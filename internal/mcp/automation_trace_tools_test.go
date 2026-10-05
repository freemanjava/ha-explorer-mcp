package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// fakeAutomationTraceReader is an automationTraceReader test double. It
// records the ids it was asked for so a test can see what reached the reader.
type fakeAutomationTraceReader struct {
	run   model.AutomationTraceRun
	err   error
	calls int
	runID string
}

func (f *fakeAutomationTraceReader) AutomationTraceRun(_ context.Context, _ model.EntityID, runID string) (model.AutomationTraceRun, error) {
	f.calls++
	f.runID = runID
	return f.run, f.err
}

// stoppedRun is an invented run that a condition stopped: a PRIVATE tracker
// on one condition's sub-step and a plain result with a false verdict.
func stoppedRun() model.AutomationTraceRun {
	return model.AutomationTraceRun{
		RunID: "01JABCDEF",
		State: "stopped",
		Steps: []model.TraceStep{
			{Path: "trigger/0", Values: []model.TypedValue{
				{Key: "entity_id", Kind: model.ValueEntity, Value: logicPrivateID},
			}},
			{Path: "condition/0", Values: []model.TypedValue{
				{Key: "result", Kind: model.ValueBool, Value: false},
			}, SubSteps: []model.TraceStep{
				{Path: "condition/0/entity_id/0", Values: []model.TypedValue{
					{Key: "entity_id", Kind: model.ValueEntity, Value: logicPrivateID},
					{Key: "result", Kind: model.ValueBool, Value: false},
				}},
			}},
		},
	}
}

func traceOptions(reader *fakeAutomationTraceReader, versions *fakeCoreReader, profile policy.Profile) Options {
	opts := testOptions()
	opts.AutomationTrace = reader
	if versions != nil {
		opts.Core = versions
	}
	opts.Profile = profile
	opts.Secrets = []string{"tok-secret-value"}
	return opts
}

func callTrace(t *testing.T, opts Options, args map[string]any) (model.AutomationTraceRun, string) {
	t.Helper()
	client := connect(t, newServer(opts, Catalog()))
	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "get_automation_trace", Arguments: args})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	var out model.AutomationTraceRun
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out, string(raw)
}

// DoD (1): the schema is a closed object holding entity_id and run_id only.
func TestGetAutomationTrace_Schema_AcceptsNoFreeFormParameter(t *testing.T) {
	client := connect(t, newServer(traceOptions(&fakeAutomationTraceReader{}, nil, policy.Profile{}), Catalog()))
	res, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name != "get_automation_trace" {
			continue
		}
		raw, _ := json.Marshal(tool.InputSchema)
		var schema struct {
			Type                 string                    `json:"type"`
			AdditionalProperties any                       `json:"additionalProperties"`
			Properties           map[string]map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("unmarshal schema: %v", err)
		}
		if schema.Type != "object" || schema.AdditionalProperties != false {
			t.Errorf("schema = %s, want a closed object", raw)
		}
		if len(schema.Properties) != 2 || schema.Properties["entity_id"] == nil || schema.Properties["run_id"] == nil {
			t.Errorf("properties = %v, want entity_id and run_id only", schema.Properties)
		}
		return
	}
	t.Fatal("get_automation_trace is not in the tool listing")
}

// DoD (1): a run id that could be a path, a sentence or a flood is rejected
// before the reader is reached.
func TestGetAutomationTrace_InvalidInput_RejectedBeforeReader(t *testing.T) {
	cases := map[string]GetAutomationTraceInput{
		"path traversal": {EntityID: "automation.a", RunID: "../config"},
		"space":          {EntityID: "automation.a", RunID: "run 1"},
		"empty":          {EntityID: "automation.a", RunID: ""},
		"too long":       {EntityID: "automation.a", RunID: strings.Repeat("a", 65)},
		"slash":          {EntityID: "automation.a", RunID: "a/b"},
		"wrong domain":   {EntityID: "light.hall", RunID: "01JABCDEF"},
		"empty entity":   {EntityID: "automation.", RunID: "01JABCDEF"},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			reader := &fakeAutomationTraceReader{}
			if _, err := getAutomationTrace(t.Context(), reader, nil, policy.Profile{}, in); err == nil {
				t.Errorf("input %+v accepted", in)
			}
			if reader.calls != 0 {
				t.Errorf("reader called %d times for rejected input", reader.calls)
			}
		})
	}
}

// DoD (2): a run id HA does not hold under this automation is ErrNotFound,
// distinct from unsupported.
func TestGetAutomationTrace_UnknownRun_IsErrNotFoundNotUnsupported(t *testing.T) {
	cases := map[string]error{
		"reader sentinel":     fmt.Errorf("run: %w", ha.ErrNotFound),
		"HA not_found answer": &ha.CommandError{Code: "not_found", Message: "No trace found"},
	}
	for name, readErr := range cases {
		t.Run(name, func(t *testing.T) {
			reader := &fakeAutomationTraceReader{err: readErr}
			out, err := getAutomationTrace(t.Context(), reader, nil, policy.Profile{}, GetAutomationTraceInput{EntityID: "automation.a", RunID: "other"})
			if !errors.Is(err, ha.ErrNotFound) {
				t.Fatalf("err = %v, want ErrNotFound", err)
			}
			if out.Unsupported {
				t.Errorf("not found must not read as unsupported: %+v", out)
			}
		})
	}
}

// DoD (3): a refused principal is unsupported with a reason, never an empty run.
func TestGetAutomationTrace_PermissionRefused_ReportsUnsupported(t *testing.T) {
	opts := traceOptions(&fakeAutomationTraceReader{err: fmt.Errorf("trace/get: %w", ha.ErrUnsupported)}, nil, policy.Profile{})
	out, _ := callTrace(t, opts, map[string]any{"entity_id": "automation.a", "run_id": "01JABCDEF"})

	if !out.Unsupported || !strings.Contains(out.UnsupportedReason, "permission denied") {
		t.Fatalf("response = %+v, want unsupported naming permission", out)
	}
	if len(out.Steps) != 0 {
		t.Errorf("unsupported response carries steps: %+v", out)
	}
}

func TestGetAutomationTrace_VersionAbsent_NamesDetectedVersion(t *testing.T) {
	reader := &fakeAutomationTraceReader{err: &ha.CommandError{Code: "unknown_command", Message: "Unknown command."}}
	opts := traceOptions(reader, &fakeCoreReader{cfg: model.CoreConfig{Version: "2026.8.3"}}, policy.Profile{})
	out, _ := callTrace(t, opts, map[string]any{"entity_id": "automation.a", "run_id": "01JABCDEF"})
	if !out.Unsupported || !strings.Contains(out.UnsupportedReason, "2026.8.3") {
		t.Fatalf("response = %+v, want unsupported naming the version", out)
	}
}

// DoD (4): under deny a PRIVATE id is masked wherever it sits, including a
// condition's sub-steps, and the verdict beside it stays visible (D-09-4).
func TestGetAutomationTrace_DenyProfile_MasksPrivateIDKeepsResult(t *testing.T) {
	opts := traceOptions(&fakeAutomationTraceReader{run: stoppedRun()}, nil, policy.Profile{Private: policy.HandlingDeny})
	out, raw := callTrace(t, opts, map[string]any{"entity_id": "automation.a", "run_id": "01JABCDEF"})

	if strings.Contains(raw, logicPrivateID) {
		t.Errorf("response leaks the PRIVATE id under deny: %s", raw)
	}
	if len(out.Steps) != 2 || len(out.Steps[1].SubSteps) != 1 {
		t.Fatalf("steps = %+v, want both steps and the sub-step kept", out.Steps)
	}
	if got := out.Steps[1].Values[0]; got.Key != "result" || got.Value != false {
		t.Errorf("condition verdict = %+v, want result false", got)
	}
	if got := out.Steps[1].SubSteps[0].Values[1]; got.Key != "result" || got.Value != false {
		t.Errorf("sub-step verdict = %+v, want result false", got)
	}
	if out.IdsWithheld != 2 {
		t.Errorf("IdsWithheld = %d, want 2", out.IdsWithheld)
	}
	if out.Steps[0].Withheld != 1 || out.Steps[1].SubSteps[0].Withheld != 1 {
		t.Errorf("per-step Withheld not counted: %+v", out.Steps)
	}
}

// Under mask and allow ids are not masked (mask tokenizes elsewhere; the
// trace carries none of a template's text to withhold).
func TestGetAutomationTrace_AllowProfile_MasksNothing(t *testing.T) {
	opts := traceOptions(&fakeAutomationTraceReader{run: stoppedRun()}, nil, policy.Profile{Private: policy.HandlingAllow})
	out, raw := callTrace(t, opts, map[string]any{"entity_id": "automation.a", "run_id": "01JABCDEF"})
	if !strings.Contains(raw, logicPrivateID) || out.IdsWithheld != 0 {
		t.Errorf("allow must mask nothing: %s", raw)
	}
	if out.Source == "" || out.ObservedAt.IsZero() || out.EntityID != "automation.a" {
		t.Errorf("missing provenance: %+v", out)
	}
}

// The reader's value is never mutated by the privacy pass.
func TestGetAutomationTrace_DenyProfile_DoesNotMutateReaderValue(t *testing.T) {
	run := stoppedRun()
	reader := &fakeAutomationTraceReader{run: run}
	if _, err := getAutomationTrace(t.Context(), reader, nil, policy.Profile{Private: policy.HandlingDeny}, GetAutomationTraceInput{EntityID: "automation.a", RunID: "01JABCDEF"}); err != nil {
		t.Fatal(err)
	}
	if got := run.Steps[0].Values[0].Value; got != logicPrivateID {
		t.Errorf("reader value mutated to %v", got)
	}
}

// Mapper counters survive the tool: partial, truncated and dropped counts.
func TestGetAutomationTrace_MapperMarkers_CarriedThrough(t *testing.T) {
	run := stoppedRun()
	run.Partial, run.PartialReason = true, "a step had the wrong shape"
	run.Truncated, run.PathsDropped, run.Withheld = true, 3, 2
	_, raw := callTrace(t, traceOptions(&fakeAutomationTraceReader{run: run}, nil, policy.Profile{}), map[string]any{"entity_id": "automation.a", "run_id": "01JABCDEF"})
	var out model.AutomationTraceRun
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	if !out.Partial || !out.Truncated || out.PathsDropped != 3 || out.Withheld != 2 {
		t.Errorf("markers lost: %+v", out)
	}
}

// A run larger than the response byte cap is cut to a prefix and says so.
func TestGetAutomationTrace_OverByteCap_TruncatedAndWithinCap(t *testing.T) {
	var steps []model.TraceStep
	for i := 0; i < 6000; i++ {
		steps = append(steps, model.TraceStep{Path: fmt.Sprintf("action/%d", i), Timestamp: time.Unix(int64(i), 0).UTC(), Values: []model.TypedValue{
			{Key: "entity_id", Kind: model.ValueEntity, Value: fmt.Sprintf("light.lamp_%04d", i)},
		}})
	}
	reader := &fakeAutomationTraceReader{run: model.AutomationTraceRun{Steps: steps}}
	out, raw := callTrace(t, traceOptions(reader, nil, policy.Profile{}), map[string]any{"entity_id": "automation.big", "run_id": "01JABCDEF"})

	if !out.Truncated {
		t.Fatalf("Truncated = false for %d bytes of steps", len(raw))
	}
	if limit := policy.LimitsFor(policy.ClassNormalRead).MaxBytes; int64(len(raw)) > limit {
		t.Errorf("response is %d bytes, over the %d cap", len(raw), limit)
	}
	if len(out.Steps) == 0 || len(out.Steps) >= len(steps) {
		t.Errorf("kept %d of %d steps, want a non-empty prefix", len(out.Steps), len(steps))
	}
}

// DoD (5): the supervisor token never reaches the response, even from a value.
func TestGetAutomationTrace_TokenInValue_NeverInResponse(t *testing.T) {
	run := stoppedRun()
	run.State = "tok-secret-value"
	run.Steps[0].Values = append(run.Steps[0].Values, model.TypedValue{Key: "mode", Kind: model.ValueToken, Value: "tok-secret-value"})
	opts := traceOptions(&fakeAutomationTraceReader{run: run}, nil, policy.Profile{Private: policy.HandlingAllow})
	_, raw := callTrace(t, opts, map[string]any{"entity_id": "automation.a", "run_id": "01JABCDEF"})
	if strings.Contains(raw, "tok-secret-value") {
		t.Errorf("token reached the response: %s", raw)
	}
}
