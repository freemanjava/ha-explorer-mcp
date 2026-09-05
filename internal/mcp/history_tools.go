package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
	"github.com/freemanjava/ha-explorer-mcp/internal/redact"
)

// historyReader is get_entity_history's read surface:
// history/history_during_period through the WebSocket connection — the
// source P0-07 recommends over the REST fallback
// (docs/research/2026-08-23-ha-history-statistics.md).
type historyReader interface {
	History(ctx context.Context, entityID model.EntityID, from, to time.Time, minimal bool) ([]model.HistoryPoint, error)
}

// maxHistoryWindow bounds a get_entity_history request's from..to span,
// independent of the point/byte budget: doc §10's threat T1 ("AI asks for
// all entity history for a year") wants a hard range cap that holds even for
// an entity too quiet for the point/byte budget to ever catch it. 7 days is
// the widest window the 2026-08-24 multi-entity measurement actually
// exercised (docs/research/2026-08-24-ha-multi-entity-query-cost.md);
// nothing wider has evidence behind it, so nothing wider is allowed.
const maxHistoryWindow = 7 * 24 * time.Hour

// historyEntityIDPattern mirrors internal/ha's own entity-id shape check
// (gateway.go's entityIDPattern, unexported to that package). Validating here
// too means a malformed id is refused before it is spent as a policy
// classification lookup or an upstream round trip (CLAUDE.md rule 6: HA data,
// and caller-supplied ids that will be echoed through HA data, are
// untrusted).
var historyEntityIDPattern = regexp.MustCompile(`^[a-z0-9_]+\.[a-z0-9_]+$`)

// GetEntityHistoryInput is get_entity_history's typed input (Appendix A.2):
// an explicit entity id and a bounded, explicit time range — no field accepts
// a route, command, path or query (rule 2), and there is no wildcard
// "give me everything" shape.
type GetEntityHistoryInput struct {
	EntityID string    `json:"entity_id" jsonschema:"the entity id to read history for"`
	From     time.Time `json:"from" jsonschema:"start of the window, inclusive"`
	To       time.Time `json:"to" jsonschema:"end of the window, exclusive"`
	// Minimal requests HA's minimal_response + no_attributes shape:
	// attributes are dropped and consecutive identical states collapse
	// (P0-07). Defaults true; ask for false only when attribute values
	// themselves matter, at several times the response size.
	Minimal *bool `json:"minimal,omitempty" jsonschema:"reduced shape (default true): drops attributes, collapses repeated states"`
}

// minimal returns the input's effective minimal flag, true by default
// (Appendix A.2: "minimal: true by default").
func (in GetEntityHistoryInput) minimal() bool {
	if in.Minimal == nil {
		return true
	}
	return *in.Minimal
}

// withHistoryTools returns tools with get_entity_history's handler bound,
// when opts supplies a reader for it. A row whose reader is absent keeps its
// bindNotImplemented default.
func withHistoryTools(tools []Tool, opts Options) []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	if opts.History == nil {
		return out
	}
	for i := range out {
		if out[i].Name == "get_entity_history" {
			out[i].bind = bindGetEntityHistory(opts.History, opts.Profile, opts.Secrets)
		}
	}
	return out
}

// bindGetEntityHistory registers get_entity_history's typed handler.
func bindGetEntityHistory(reader historyReader, profile policy.Profile, secrets []string) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in GetEntityHistoryInput) (*sdkmcp.CallToolResult, model.EntityHistory, error) {
			out, err := getEntityHistory(ctx, reader, profile, secrets, in)
			return nil, out, err
		})
	}
}

// getEntityHistory validates the request, refuses what the range cap and the
// privacy profile will not serve, reads the recorder-backed source, and
// charges the invocation's budget for what actually came back.
//
// Refusal order matters: shape and range are checked before anything reaches
// the profile or the budget, so a malformed request is never charged against
// either; the privacy check runs before the query is issued, because a deny
// refusal must never spend a recorder read to produce nothing (phase 02
// design notes); the budget pre-flight estimate runs last of the "before"
// checks, over the one entity and the already-validated window.
func getEntityHistory(ctx context.Context, reader historyReader, profile policy.Profile, secrets []string, in GetEntityHistoryInput) (model.EntityHistory, error) {
	if !historyEntityIDPattern.MatchString(in.EntityID) {
		return model.EntityHistory{}, fmt.Errorf("get_entity_history: %q is not a valid entity id", in.EntityID)
	}
	entityID := model.EntityID(in.EntityID)

	if !in.To.After(in.From) {
		return model.EntityHistory{}, fmt.Errorf("get_entity_history: to (%s) must be after from (%s)", in.To, in.From)
	}
	window := in.To.Sub(in.From)
	if window > maxHistoryWindow {
		return model.EntityHistory{}, fmt.Errorf("%w: get_entity_history: requested window %s exceeds the maximum %s",
			policy.ErrPolicyDenied, window, maxHistoryWindow)
	}

	if err := profile.CheckHistoryScope(policy.HistoryScope{Entities: []model.EntityID{entityID}}); err != nil {
		return model.EntityHistory{}, err
	}

	if budget, ok := policy.BudgetFrom(ctx); ok {
		if err := budget.Preflight(policy.SourceHistory, 1, window); err != nil {
			return model.EntityHistory{}, err
		}
	}

	minimal := in.minimal()
	points, err := reader.History(ctx, entityID, in.From, in.To, minimal)
	if err != nil {
		return model.EntityHistory{}, err
	}

	redactor := redact.New(profile, secrets...)
	masked := maskHistoryPoints(redactor, entityID, points)

	if budget, ok := policy.BudgetFrom(ctx); ok {
		if err := budget.ChargeHARequests(1); err != nil {
			return model.EntityHistory{}, err
		}
		if err := budget.ChargeHistoryPoints(len(masked)); err != nil {
			return model.EntityHistory{}, err
		}
		if err := budget.ChargeBytes(historyByteSize(masked)); err != nil {
			return model.EntityHistory{}, err
		}
	}

	return model.EntityHistory{
		Source:     "home_assistant_core",
		ObservedAt: time.Now().UTC(),
		EntityID:   entityID,
		From:       in.From,
		To:         in.To,
		Minimal:    minimal,
		Points:     masked,
	}, nil
}

// maskHistoryPoints applies the Phase 02 profile to one entity's raw history,
// reusing internal/redact's own classification and masking rather than
// re-implementing it (mirrors entity_tools.go's maskEntityState). The whole
// series is redacted in one Payload call, not point by point: the walker
// scopes its mask tokens per response, and a series masked one point at a
// time would mint a fresh Redactor per point and lose the "equal states share
// a token" property masking depends on to keep transitions countable.
func maskHistoryPoints(redactor *redact.Redactor, entityID model.EntityID, points []model.HistoryPoint) []model.HistoryPoint {
	if len(points) == 0 {
		return points
	}

	raw := make([]any, len(points))
	for i, p := range points {
		elem := map[string]any{
			"entity_id": string(entityID),
			"state":     p.State,
		}
		if len(p.Attributes) > 0 {
			elem["attributes"] = p.Attributes
		}
		raw[i] = elem
	}

	res := redactor.Payload(raw)
	arr, ok := res.Value.([]any)
	if !ok {
		return points
	}

	out := make([]model.HistoryPoint, len(points))
	for i, p := range points {
		out[i] = p
		elem, ok := arr[i].(map[string]any)
		if !ok {
			continue
		}
		if s, ok := elem["state"].(string); ok {
			out[i].State = s
		}
		if attrs, ok := elem["attributes"].(map[string]any); ok {
			out[i].Attributes = attrs
		} else {
			// A nil map marshals to JSON null, but the MCP SDK's inferred
			// output schema declares Attributes an object unconditionally
			// (mirrors MapRepair's translation_placeholders, mapping.go
			// optObject) — never present, this must still be an empty
			// object rather than null.
			out[i].Attributes = map[string]any{}
		}
	}
	return out
}

// historyByteSize approximates a masked history's serialized size for the
// byte dimension of the query budget — cheap enough to run once per
// invocation without re-serializing the response afterward.
func historyByteSize(points []model.HistoryPoint) int64 {
	b, err := json.Marshal(points)
	if err != nil {
		return 0
	}
	return int64(len(b))
}
