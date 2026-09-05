package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// fakeAutomationReader is an automationReader test double.
type fakeAutomationReader struct {
	automations []model.AutomationSummary
}

func (f *fakeAutomationReader) Automations(context.Context) ([]model.AutomationSummary, error) {
	return f.automations, nil
}

func automationOptions(reader automationReader) Options {
	opts := testOptions()
	opts.Automations = reader
	return opts
}

// TestListAutomations_ReportsEnabledStateAndLastTriggered is the P3-06 DoD's
// core promise for this tool.
func TestListAutomations_ReportsEnabledStateAndLastTriggered(t *testing.T) {
	triggered := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	reader := &fakeAutomationReader{automations: []model.AutomationSummary{
		{EntityID: "automation.morning", Alias: "Morning", Enabled: true, LastTriggered: &triggered, Mode: "single"},
		{EntityID: "automation.never_run", Alias: "Never Run", Enabled: false},
	}}
	client := connect(t, newServer(automationOptions(reader), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "list_automations", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var list model.AutomationList
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list.Items) != 2 {
		t.Fatalf("Items = %+v, want 2", list.Items)
	}
	byID := map[model.EntityID]model.AutomationSummary{}
	for _, a := range list.Items {
		byID[a.EntityID] = a
	}
	morning := byID["automation.morning"]
	if !morning.Enabled {
		t.Errorf("automation.morning should be reported enabled")
	}
	if morning.LastTriggered == nil || !morning.LastTriggered.Equal(triggered) {
		t.Errorf("automation.morning LastTriggered = %v, want %v", morning.LastTriggered, triggered)
	}
	neverRun := byID["automation.never_run"]
	if neverRun.Enabled {
		t.Errorf("automation.never_run should be reported disabled")
	}
	if neverRun.LastTriggered != nil {
		t.Errorf("automation.never_run LastTriggered = %v, want nil", neverRun.LastTriggered)
	}
}

// TestListAutomations_EnabledFilter exercises the enabled filter
// independently, the same way list_devices' filters are pinned per-value.
func TestListAutomations_EnabledFilter(t *testing.T) {
	reader := &fakeAutomationReader{automations: []model.AutomationSummary{
		{EntityID: "automation.a", Enabled: true},
		{EntityID: "automation.b", Enabled: false},
	}}
	client := connect(t, newServer(automationOptions(reader), Catalog()))

	enabled := true
	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "list_automations", Arguments: map[string]any{"enabled": enabled}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var list model.AutomationList
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].EntityID != "automation.a" {
		t.Fatalf("Items = %+v, want only automation.a", list.Items)
	}
}

// TestAutomationTools_SchemaRejectsFreeFormParameter pins the parity rule's
// fourth clause: every implemented tool in this file's schema still closes
// off any parameter beyond its own typed fields.
func TestAutomationTools_SchemaRejectsFreeFormParameter(t *testing.T) {
	opts := automationOptions(&fakeAutomationReader{})
	opts.AutomationDetail = &fakeAutomationDetailReader{}
	client := connect(t, newServer(opts, Catalog()))
	res, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	checked := map[string]bool{}
	for _, tool := range res.Tools {
		if tool.Name != "list_automations" && tool.Name != "get_automation" && tool.Name != "get_automation_traces" {
			continue
		}
		checked[tool.Name] = true
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("%s: marshal schema: %v", tool.Name, err)
		}
		var schema struct {
			Type                 string `json:"type"`
			AdditionalProperties any    `json:"additionalProperties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("%s: unmarshal schema: %v", tool.Name, err)
		}
		if schema.Type != "object" || schema.AdditionalProperties != false {
			t.Errorf("%s: schema = %s, want a closed object with additionalProperties false", tool.Name, raw)
		}
	}
	for _, name := range []string{"list_automations", "get_automation", "get_automation_traces"} {
		if !checked[name] {
			t.Errorf("%s was not found in the tool listing", name)
		}
	}
}

// fakeAutomationDetailReader is an automationDetailReader test double.
type fakeAutomationDetailReader struct {
	automation    model.Automation
	automationErr error
	traces        []model.AutomationTraceSummary
	tracesErr     error
}

func (f *fakeAutomationDetailReader) AutomationDetail(context.Context, model.EntityID) (model.Automation, error) {
	return f.automation, f.automationErr
}
func (f *fakeAutomationDetailReader) AutomationTraces(context.Context, model.EntityID) ([]model.AutomationTraceSummary, error) {
	return f.traces, f.tracesErr
}

// fakeLogbookReader is a logbookReader test double.
type fakeLogbookReader struct {
	events []model.LogbookEvent
	err    error
}

func (f *fakeLogbookReader) LogbookEvents(context.Context, model.EntityID, time.Time) ([]model.LogbookEvent, error) {
	return f.events, f.err
}

func automationDetailOptions(detail *fakeAutomationDetailReader, versions *fakeCoreReader, automations automationReader, logbook logbookReader) Options {
	opts := testOptions()
	opts.AutomationDetail = detail
	if versions != nil {
		opts.Core = versions
	}
	opts.Automations = automations
	opts.Logbook = logbook
	return opts
}

// TestGetAutomation_MapsConfigDetail is the P3-07 DoD's happy path: on a
// principal and version where automation/config answers, get_automation
// returns its counts, not the trigger/condition/action bodies themselves
// (CLAUDE.md rule 6).
func TestGetAutomation_MapsConfigDetail(t *testing.T) {
	detail := &fakeAutomationDetailReader{automation: model.Automation{
		EntityID: "automation.evening_lights", Alias: "Evening lights", Mode: "single",
		TriggerCount: 1, ConditionCount: 1, ActionCount: 2,
	}}
	client := connect(t, newServer(automationDetailOptions(detail, nil, nil, nil), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "get_automation", Arguments: map[string]any{"entity_id": "automation.evening_lights"}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var a model.Automation
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.Unsupported {
		t.Fatalf("get_automation reported Unsupported: %s", a.UnsupportedReason)
	}
	if a.Alias != "Evening lights" || a.TriggerCount != 1 || a.ConditionCount != 1 || a.ActionCount != 2 {
		t.Fatalf("Automation = %+v unexpectedly", a)
	}
}

// TestGetAutomation_PermissionRefused_ReportsUnsupportedWithFallback pins the
// F-11 degraded branch: a non-admin principal must not surface as an empty
// or broken tool, and the reason must name the fallback (P3-07 DoD).
func TestGetAutomation_PermissionRefused_ReportsUnsupportedWithFallback(t *testing.T) {
	detail := &fakeAutomationDetailReader{automationErr: &ha.CommandError{Code: "unauthorized", Message: "unauthorized"}}
	client := connect(t, newServer(automationDetailOptions(detail, nil, nil, nil), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "get_automation", Arguments: map[string]any{"entity_id": "automation.evening_lights"}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var a model.Automation
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !a.Unsupported {
		t.Fatal("get_automation did not report Unsupported for a permission refusal")
	}
	if !strings.Contains(a.UnsupportedReason, "list_automations") {
		t.Errorf("UnsupportedReason = %q, want it to name list_automations", a.UnsupportedReason)
	}
	if strings.Contains(a.UnsupportedReason, "2026.") {
		t.Errorf("UnsupportedReason = %q, permission refusal must not read like a version mismatch", a.UnsupportedReason)
	}
}

// TestGetAutomation_VersionAbsent_NamesDetectedVersion pins the DoD's other
// unsupported branch: an HA release without automation/config at all is
// distinguishable from a permission refusal, and names the version.
func TestGetAutomation_VersionAbsent_NamesDetectedVersion(t *testing.T) {
	detail := &fakeAutomationDetailReader{automationErr: &ha.CommandError{Code: "unknown_command", Message: "Unknown command: automation/config"}}
	versions := &fakeCoreReader{cfg: model.CoreConfig{Version: "2025.1.0"}}
	client := connect(t, newServer(automationDetailOptions(detail, versions, nil, nil), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "get_automation", Arguments: map[string]any{"entity_id": "automation.evening_lights"}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var a model.Automation
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !a.Unsupported {
		t.Fatal("get_automation did not report Unsupported for an absent command")
	}
	if !strings.Contains(a.UnsupportedReason, "2025.1.0") {
		t.Errorf("UnsupportedReason = %q, want it to name the detected version", a.UnsupportedReason)
	}
}

// TestGetAutomation_InvalidEntityID_Rejected pins the input-validation guard:
// a non-automation id is a validation error, not a round trip to HA.
func TestGetAutomation_InvalidEntityID_Rejected(t *testing.T) {
	detail := &fakeAutomationDetailReader{}
	client := connect(t, newServer(automationDetailOptions(detail, nil, nil, nil), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "get_automation", Arguments: map[string]any{"entity_id": "light.kitchen"}})
	if err != nil {
		t.Fatalf("CallTool transport error: %v", err)
	}
	if !res.IsError {
		t.Fatal("get_automation accepted a non-automation entity id")
	}
}

// TestGetAutomationTraces_ReturnsRunsNewestFirst is the P3-07 DoD's happy
// path: traces are returned with their execution outcome.
func TestGetAutomationTraces_ReturnsRunsNewestFirst(t *testing.T) {
	older := time.Date(2026, 8, 21, 19, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 8, 22, 19, 0, 0, 0, time.UTC)
	detail := &fakeAutomationDetailReader{traces: []model.AutomationTraceSummary{
		{RunID: "r1", State: "stopped", ScriptExecution: "finished", TimestampStart: older},
		{RunID: "r2", State: "running", TimestampStart: newer},
	}}
	client := connect(t, newServer(automationDetailOptions(detail, nil, nil, nil), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "get_automation_traces", Arguments: map[string]any{"entity_id": "automation.evening_lights"}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var list model.AutomationTraceList
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if list.Unsupported {
		t.Fatalf("get_automation_traces reported Unsupported: %s", list.UnsupportedReason)
	}
	if len(list.Items) != 2 || list.Items[0].RunID != "r2" || list.Items[1].RunID != "r1" {
		t.Fatalf("Items = %+v, want r2 then r1 (newest first)", list.Items)
	}
}

// TestGetAutomationTraces_PermissionRefused_AttachesFallbackEvidence pins the
// F-11 degraded branch's central promise: the fallback is fetched into the
// response, not merely named in prose (P3-07 DoD).
func TestGetAutomationTraces_PermissionRefused_AttachesFallbackEvidence(t *testing.T) {
	triggered := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	detail := &fakeAutomationDetailReader{tracesErr: &ha.CommandError{Code: "unauthorized", Message: "unauthorized"}}
	automations := &fakeAutomationReader{automations: []model.AutomationSummary{
		{EntityID: "automation.evening_lights", LastTriggered: &triggered},
	}}
	logbook := &fakeLogbookReader{events: []model.LogbookEvent{
		{Name: "Evening lights", ContextID: "ctx1"},
	}}
	client := connect(t, newServer(automationDetailOptions(detail, nil, automations, logbook), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "get_automation_traces", Arguments: map[string]any{"entity_id": "automation.evening_lights"}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var list model.AutomationTraceList
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !list.Unsupported {
		t.Fatal("get_automation_traces did not report Unsupported for a permission refusal")
	}
	if list.FallbackLastTriggered == nil || !list.FallbackLastTriggered.Equal(triggered) {
		t.Errorf("FallbackLastTriggered = %v, want %v", list.FallbackLastTriggered, triggered)
	}
	if len(list.FallbackEvents) != 1 || list.FallbackEvents[0].ContextID != "ctx1" {
		t.Errorf("FallbackEvents = %+v, want one event with ContextID ctx1", list.FallbackEvents)
	}
}

// TestGetAutomationTraces_VersionAbsent_NoFallbackFetched pins the other half
// of the degraded-branch design: a version-absent trace/list has no fallback
// worth fetching, since dropping trace/list did not also change
// last_triggered or the logbook.
func TestGetAutomationTraces_VersionAbsent_NoFallbackFetched(t *testing.T) {
	detail := &fakeAutomationDetailReader{tracesErr: &ha.CommandError{Code: "unknown_command", Message: "Unknown command: trace/list"}}
	automations := &fakeAutomationReader{automations: []model.AutomationSummary{{EntityID: "automation.evening_lights"}}}
	logbook := &fakeLogbookReader{events: []model.LogbookEvent{{Name: "should not be fetched"}}}
	client := connect(t, newServer(automationDetailOptions(detail, &fakeCoreReader{cfg: model.CoreConfig{Version: "2025.1.0"}}, automations, logbook), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "get_automation_traces", Arguments: map[string]any{"entity_id": "automation.evening_lights"}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var list model.AutomationTraceList
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !list.Unsupported || !strings.Contains(list.UnsupportedReason, "2025.1.0") {
		t.Fatalf("UnsupportedReason = %q, want it to name the detected version", list.UnsupportedReason)
	}
	if list.FallbackEvents != nil || list.FallbackLastTriggered != nil {
		t.Errorf("version-absent response fetched a fallback it should not have: %+v", list)
	}
}
