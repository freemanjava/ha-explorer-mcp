package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

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
// fourth clause: an implemented list_* tool's schema still closes off any
// parameter beyond its own typed fields.
func TestAutomationTools_SchemaRejectsFreeFormParameter(t *testing.T) {
	client := connect(t, newServer(automationOptions(&fakeAutomationReader{}), Catalog()))
	res, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name != "list_automations" {
			continue
		}
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
}
