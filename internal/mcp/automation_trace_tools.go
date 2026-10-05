package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
	"github.com/freemanjava/ha-explorer-mcp/internal/redact"
)

// automationTraceReader is get_automation_trace's read surface: the
// admin-gated trace/get command, mapped to typed steps (P9-03). A non-admin
// principal or an HA version without the command answers with an error the
// tool classifies, exactly as get_automation_traces' does.
type automationTraceReader interface {
	AutomationTraceRun(ctx context.Context, entityID model.EntityID, runID string) (model.AutomationTraceRun, error)
}

// GetAutomationTraceInput is get_automation_trace's typed input: an
// automation and one of its run ids, each checked by grammar. Nothing here
// could be used as a route, command or query.
type GetAutomationTraceInput struct {
	EntityID string `json:"entity_id" jsonschema:"the automation entity id, e.g. automation.evening_lights"`
	RunID    string `json:"run_id" jsonschema:"a run id as reported by get_automation_traces"`
}

// runIDPattern admits what get_automation_traces emits for a run id (a uuid
// hex or a ULID) and nothing that could be a path, a sentence or a query.
// It matches the grammar the mapper applies to the same field.
var runIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// traceEnvelopeBytes is the room kept for the response fields around the
// steps (provenance, counters, the reason text) when the byte cap is applied.
// A generous bound on what is not a step, not a measurement.
const traceEnvelopeBytes = 1024

func validateRunID(runID string) error {
	if !runIDPattern.MatchString(runID) {
		return fmt.Errorf("run_id is not a run id as get_automation_traces reports them")
	}
	return nil
}

// bindGetAutomationTrace registers get_automation_trace's typed handler. Its
// output type is any for the reason bindGetAutomationLogic's is: TraceStep
// nests itself through SubSteps, which the SDK cannot infer a schema for.
func bindGetAutomationTrace(reader automationTraceReader, versions automationVersionReader, profile policy.Profile, secrets []string) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in GetAutomationTraceInput) (*sdkmcp.CallToolResult, any, error) {
			out, err := getAutomationTrace(ctx, reader, versions, profile, in, secrets...)
			if err != nil {
				return nil, nil, err
			}
			return nil, out, nil
		})
	}
}

// getAutomationTrace reads trace/get for one run as typed steps and applies
// the privacy profile (D-09-4). Where HA refuses by permission or does not
// offer the command, the response is Unsupported with a reason rather than an
// empty run (rule 7); a run HA does not hold under this automation is
// ErrNotFound.
func getAutomationTrace(ctx context.Context, reader automationTraceReader, versions automationVersionReader, profile policy.Profile, in GetAutomationTraceInput, secrets ...string) (model.AutomationTraceRun, error) {
	if err := validateAutomationEntityID(in.EntityID); err != nil {
		return model.AutomationTraceRun{}, fmt.Errorf("get_automation_trace: %w", err)
	}
	if err := validateRunID(in.RunID); err != nil {
		return model.AutomationTraceRun{}, fmt.Errorf("get_automation_trace: %w", err)
	}
	entityID := model.EntityID(in.EntityID)
	envelope := model.AutomationTraceRun{Source: "home_assistant_core", ObservedAt: time.Now().UTC(), EntityID: entityID}

	run, err := reader.AutomationTraceRun(ctx, entityID, in.RunID)
	if err != nil {
		var cmdErr *ha.CommandError
		if errors.As(err, &cmdErr) && cmdErr.Code == "not_found" {
			return model.AutomationTraceRun{}, fmt.Errorf("%w: run %q of automation %q", ha.ErrNotFound, in.RunID, in.EntityID)
		}
		reason, ok := classifyAutomationError(ctx, versions, err, automationFallbackReason)
		if !ok {
			return model.AutomationTraceRun{}, err
		}
		envelope.Unsupported = true
		envelope.UnsupportedReason = reason
		return envelope, nil
	}

	redactor := redact.New(profile, secrets...)
	envelope.RunID = redactor.Text(run.RunID)
	envelope.State = redactor.Text(run.State)
	envelope.ScriptExecution = redactor.Text(run.ScriptExecution)
	envelope.LastStep = redactor.Text(run.LastStep)
	envelope.TimestampStart = run.TimestampStart
	envelope.TimestampFinish = run.TimestampFinish
	envelope.Provenance = run.Provenance
	envelope.Truncated = run.Truncated
	envelope.PathsDropped = run.PathsDropped
	envelope.Withheld = run.Withheld

	applier := traceMasker{privacy: logicPrivacy{deny: profile.Private == policy.HandlingDeny}, redactor: redactor}
	envelope.Steps = applier.steps(run.Steps)
	envelope.IdsWithheld = applier.privacy.ids

	if capTraceBytes(&envelope, maxResponseBytes(ctx)) {
		envelope.Truncated = true
	}
	return envelope, nil
}

// traceMasker applies the profile to mapped steps, counting what it masks. It
// works on copies: the reader's value is never mutated. The id masking is
// logicPrivacy's, so a trace and the logic it ran hide the same ids.
type traceMasker struct {
	privacy  logicPrivacy
	redactor *redact.Redactor
}

func (m *traceMasker) steps(in []model.TraceStep) []model.TraceStep {
	if in == nil {
		return nil
	}
	out := make([]model.TraceStep, len(in))
	for i, s := range in {
		s.Values = m.privacy.values(s.Values, &s.Withheld)
		s.Values = m.scrub(s.Values)
		s.SubSteps = m.steps(s.SubSteps)
		out[i] = s
	}
	return out
}

// scrub removes the supervisor token from string values, as every response
// text is scrubbed. The grammar already keeps free text out; this is the
// last line for a token-shaped value that passed it.
func (m *traceMasker) scrub(in []model.TypedValue) []model.TypedValue {
	for i, v := range in {
		if text, ok := v.Value.(string); ok {
			in[i].Value = m.redactor.Text(text)
		}
	}
	return in
}

// capTraceBytes keeps steps in execution order until the byte budget is
// spent, drops the rest, and reports whether it dropped any. The opening of a
// run is kept because the step that stopped it is usually near it, and
// last_step in the envelope names where it ended.
func capTraceBytes(r *model.AutomationTraceRun, max int64) bool {
	budget := max - traceEnvelopeBytes
	kept := 0
	for _, s := range r.Steps {
		size := traceStepBytes(s)
		if size > budget {
			break
		}
		budget -= size
		kept++
	}
	dropped := kept < len(r.Steps)
	r.Steps = r.Steps[:kept]
	return dropped
}

// traceStepBytes approximates one step's serialized size for the byte cap,
// including the comma that separates it from the next step.
func traceStepBytes(s model.TraceStep) int64 {
	b, err := json.Marshal(s)
	if err != nil {
		return 0
	}
	return int64(len(b)) + 1
}
