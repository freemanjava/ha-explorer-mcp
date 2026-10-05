package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
	"github.com/freemanjava/ha-explorer-mcp/internal/redact"
)

// automationLogicReader is get_automation_logic's read surface: the
// admin-gated automation/config command, mapped to typed logic (P9-01). A
// non-admin principal or an HA version without the command answers with an
// error the tool classifies, exactly as get_automation's does.
type automationLogicReader interface {
	AutomationLogic(ctx context.Context, entityID model.EntityID) (model.AutomationLogic, error)
}

// GetAutomationLogicInput is get_automation_logic's typed input: exactly the
// entity id, nothing an agent could use as a free-form route or query.
type GetAutomationLogicInput struct {
	EntityID string `json:"entity_id" jsonschema:"the automation entity id, e.g. automation.evening_lights"`
}

// logicEnvelopeBytes is the room kept for the response fields around the
// nodes (provenance, counters, the reason text) when the byte cap is applied.
// The cap is on the whole serialized response, and this is a generous bound on
// what is not a node, not a measurement.
const logicEnvelopeBytes = 1024

// maskedID is what replaces a PRIVATE entity id under the deny profile
// (D-09-4): the element, its kind and its sibling values stay, only the id is
// withheld, and IdsWithheld counts it.
const maskedID = "withheld"

// bindGetAutomationLogic registers get_automation_logic's typed handler. Its
// output type is any because LogicNode nests itself: the SDK cannot infer a
// JSON Schema for a recursive type and would panic at registration. The
// result is still the model.AutomationLogic value, serialized as structured
// content; only the advertised output schema is omitted.
func bindGetAutomationLogic(reader automationLogicReader, versions automationVersionReader, profile policy.Profile, secrets []string) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in GetAutomationLogicInput) (*sdkmcp.CallToolResult, any, error) {
			out, err := getAutomationLogic(ctx, reader, versions, profile, in, secrets...)
			if err != nil {
				return nil, nil, err
			}
			return nil, out, nil
		})
	}
}

// getAutomationLogic reads automation/config as typed logic and applies the
// privacy profile (D-09-4, D-09-5). Where HA refuses by permission or does not
// offer the command, the response is Unsupported with a reason rather than an
// empty logic (rule 7); an automation HA does not know is ErrNotFound.
func getAutomationLogic(ctx context.Context, reader automationLogicReader, versions automationVersionReader, profile policy.Profile, in GetAutomationLogicInput, secrets ...string) (model.AutomationLogic, error) {
	if err := validateAutomationEntityID(in.EntityID); err != nil {
		return model.AutomationLogic{}, fmt.Errorf("get_automation_logic: %w", err)
	}
	entityID := model.EntityID(in.EntityID)
	envelope := model.AutomationLogic{Source: "home_assistant_core", ObservedAt: time.Now().UTC(), EntityID: entityID}

	logic, err := reader.AutomationLogic(ctx, entityID)
	if err != nil {
		var cmdErr *ha.CommandError
		if errors.As(err, &cmdErr) && cmdErr.Code == "not_found" {
			return model.AutomationLogic{}, fmt.Errorf("%w: automation %q", ha.ErrNotFound, in.EntityID)
		}
		reason, ok := classifyAutomationError(ctx, versions, err, automationFallbackReason)
		if !ok {
			return model.AutomationLogic{}, err
		}
		envelope.Unsupported = true
		envelope.UnsupportedReason = reason
		return envelope, nil
	}

	envelope.Provenance = logic.Provenance
	envelope.Truncated = logic.Truncated
	applier := logicPrivacy{
		deny:           profile.Private == policy.HandlingDeny,
		allowTemplates: profile.Private == policy.HandlingAllow,
		redactor:       redact.New(profile, secrets...),
	}
	envelope.Triggers = applier.nodes(logic.Triggers)
	envelope.Conditions = applier.nodes(logic.Conditions)
	envelope.Actions = applier.nodes(logic.Actions)
	envelope.IdsWithheld = applier.ids
	envelope.TemplatesWithheld = applier.templates

	if capLogicBytes(&envelope, maxResponseBytes(ctx)) {
		envelope.Truncated = true
	}
	return envelope, nil
}

// logicPrivacy applies the profile to a mapped logic tree, counting what it
// removes. It works on copies: the reader's value is never mutated.
type logicPrivacy struct {
	deny           bool
	allowTemplates bool
	redactor       *redact.Redactor
	ids            int
	templates      int
}

func (p *logicPrivacy) nodes(in []model.LogicNode) []model.LogicNode {
	if in == nil {
		return nil
	}
	out := make([]model.LogicNode, len(in))
	for i, n := range in {
		out[i] = p.node(n)
	}
	return out
}

func (p *logicPrivacy) node(n model.LogicNode) model.LogicNode {
	n.Values = p.values(n.Values, &n.Withheld)
	n.Templates = p.templateTexts(n.Templates, &n.Withheld)
	n.Children = p.nodes(n.Children)
	return n
}

// values masks a PRIVATE entity id under deny (D-09-4). Only the id changes:
// the key and kind stay, so the agent still sees that a presence entity gates
// the element.
func (p *logicPrivacy) values(in []model.TypedValue, withheld *int) []model.TypedValue {
	if in == nil {
		return nil
	}
	out := make([]model.TypedValue, len(in))
	for i, v := range in {
		out[i] = v
		id, isString := v.Value.(string)
		if !p.deny || v.Kind != model.ValueEntity || !isString {
			continue
		}
		if policy.ClassifyEntityWithClass(model.EntityID(id), "") == policy.SensitivityPrivate {
			out[i].Value = maskedID
			p.ids++
			*withheld++
		}
	}
	return out
}

// templateTexts ships template text only under allow (D-09-6): masking ids
// inside Jinja reliably would mean parsing it, so mask withholds it like deny
// does. Shipped text has the supervisor token scrubbed like any response text.
func (p *logicPrivacy) templateTexts(in []model.Template, withheld *int) []model.Template {
	if len(in) == 0 {
		return nil
	}
	if !p.allowTemplates {
		p.templates += len(in)
		*withheld += len(in)
		return nil
	}
	out := make([]model.Template, len(in))
	for i, t := range in {
		out[i] = t
		out[i].Text = p.redactor.Text(t.Text)
	}
	return out
}

// capLogicBytes keeps top-level nodes in order — triggers, conditions, then
// actions — until the byte budget is spent, drops the rest of that section,
// and reports whether it dropped any. Keeping the opening of each section
// matters because that is where an automation's gate usually sits.
func capLogicBytes(l *model.AutomationLogic, max int64) bool {
	budget := max - logicEnvelopeBytes
	dropped := false
	for _, section := range []*[]model.LogicNode{&l.Triggers, &l.Conditions, &l.Actions} {
		kept := 0
		for _, n := range *section {
			size := logicNodeBytes(n)
			if size > budget {
				dropped = true
				break
			}
			budget -= size
			kept++
		}
		*section = (*section)[:kept]
	}
	return dropped
}

// logicNodeBytes approximates one node's serialized size for the byte cap,
// including the comma that separates it from the next node in its array.
func logicNodeBytes(n model.LogicNode) int64 {
	b, err := json.Marshal(n)
	if err != nil {
		return 0
	}
	return int64(len(b)) + 1
}
