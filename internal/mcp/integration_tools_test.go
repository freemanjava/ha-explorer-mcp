package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// fakeAvailabilityReader is an entityAvailabilityReader test double.
type fakeAvailabilityReader struct {
	unavailable map[model.EntityID]struct{}
	err         error
}

func (f *fakeAvailabilityReader) UnavailableEntityIDs(context.Context) (map[model.EntityID]struct{}, error) {
	return f.unavailable, f.err
}

// integrationOptions is testOptions wired with the given readers.
func integrationOptions(inventory systemInventoryReader, avail entityAvailabilityReader) Options {
	opts := testOptions()
	opts.Inventory = inventory
	opts.Availability = avail
	return opts
}

func unavailableSet(ids ...model.EntityID) map[model.EntityID]struct{} {
	out := make(map[model.EntityID]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}

// TestListIntegrations_ComputesCountsWithoutExposingLists is the P3-03 DoD's
// second line: counts computed server-side, and the raw response never
// carries the underlying entity or device ids.
func TestListIntegrations_ComputesCountsWithoutExposingLists(t *testing.T) {
	inventory := &fakeInventoryReader{
		integrations: []model.Integration{
			{ID: "entry-hue", Domain: "hue", Title: "Philips Hue"},
			{ID: "entry-mqtt", Domain: "mqtt", Title: "Mosquitto"},
		},
		entities: []model.Entity{
			{ID: "light.kitchen", ConfigEntryID: "entry-hue"},
			{ID: "light.hallway", ConfigEntryID: "entry-hue"},
			{ID: "sensor.mqtt_temp", ConfigEntryID: "entry-mqtt"},
		},
		devices: []model.DeviceRef{
			{ID: "dev-hue-bridge", ConfigEntryID: "entry-hue"},
		},
	}
	avail := &fakeAvailabilityReader{unavailable: unavailableSet("light.hallway")}

	client := connect(t, newServer(integrationOptions(inventory, avail), Catalog()))

	var list model.IntegrationList
	res := callStructured(t, client, "list_integrations", &list)

	if len(list.Items) != 2 {
		t.Fatalf("Items = %d, want 2", len(list.Items))
	}
	byID := map[model.ConfigEntryID]model.IntegrationSummary{}
	for _, it := range list.Items {
		byID[it.ID] = it
	}
	hue := byID["entry-hue"]
	if hue.EntityCount != 2 || hue.DeviceCount != 1 || hue.UnavailableEntities != 1 {
		t.Fatalf("hue summary = %+v, want EntityCount 2, DeviceCount 1, UnavailableEntities 1", hue)
	}
	mqtt := byID["entry-mqtt"]
	if mqtt.EntityCount != 1 || mqtt.DeviceCount != 0 || mqtt.UnavailableEntities != 0 {
		t.Fatalf("mqtt summary = %+v, want EntityCount 1, DeviceCount 0, UnavailableEntities 0", mqtt)
	}

	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, id := range []string{"light.kitchen", "light.hallway", "sensor.mqtt_temp", "dev-hue-bridge"} {
		if strings.Contains(string(raw), id) {
			t.Errorf("list_integrations response contains %q — it must report counts only, never the per-entity/device list", id)
		}
	}
}

// TestListIntegrations_DomainFilter exercises the Phase 02 filtering
// contract.
func TestListIntegrations_DomainFilter(t *testing.T) {
	inventory := &fakeInventoryReader{
		integrations: []model.Integration{
			{ID: "entry-hue", Domain: "hue"},
			{ID: "entry-mqtt", Domain: "mqtt"},
		},
	}
	avail := &fakeAvailabilityReader{unavailable: unavailableSet()}

	client := connect(t, newServer(integrationOptions(inventory, avail), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name:      "list_integrations",
		Arguments: map[string]any{"domain": "mqtt"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var list model.IntegrationList
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Domain != "mqtt" {
		t.Fatalf("domain filter returned %+v, want exactly the mqtt entry", list.Items)
	}
}

// TestListIntegrations_CursorPagination_HonorsPhase02Contract pins default
// limit, clamping and the truncated/next_cursor pair together.
func TestListIntegrations_CursorPagination_HonorsPhase02Contract(t *testing.T) {
	var entries []model.Integration
	for i := range 3 {
		entries = append(entries, model.Integration{ID: model.ConfigEntryID("entry-" + string(rune('a'+i))), Domain: "demo"})
	}
	inventory := &fakeInventoryReader{integrations: entries}
	avail := &fakeAvailabilityReader{unavailable: unavailableSet()}

	client := connect(t, newServer(integrationOptions(inventory, avail), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name:      "list_integrations",
		Arguments: map[string]any{"limit": 2},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var list model.IntegrationList
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list.Items) != 2 || !list.Truncated || list.NextCursor == "" {
		t.Fatalf("first page = %+v, want 2 items, Truncated true, a NextCursor", list)
	}

	res2, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name:      "list_integrations",
		Arguments: map[string]any{"limit": 2, "cursor": list.NextCursor},
	})
	if err != nil {
		t.Fatalf("CallTool (page 2): %v", err)
	}
	var list2 model.IntegrationList
	raw2, _ := json.Marshal(res2.StructuredContent)
	if err := json.Unmarshal(raw2, &list2); err != nil {
		t.Fatalf("unmarshal page 2: %v", err)
	}
	if len(list2.Items) != 1 || list2.Truncated {
		t.Fatalf("second page = %+v, want the last remaining item and Truncated false", list2)
	}
}

// TestGetIntegration_FailedSetupState_NotOmitted is the DoD's third line: an
// integration in a failed setup state is represented with its state and
// reason, never dropped from the result.
func TestGetIntegration_FailedSetupState_NotOmitted(t *testing.T) {
	inventory := &fakeInventoryReader{
		integrations: []model.Integration{
			{ID: "entry-broken", Domain: "broken_thing", State: "setup_error", Reason: "cannot connect"},
		},
	}
	avail := &fakeAvailabilityReader{unavailable: unavailableSet()}

	client := connect(t, newServer(integrationOptions(inventory, avail), Catalog()))

	var detail model.IntegrationDetail
	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name:      "get_integration",
		Arguments: map[string]any{"id": "entry-broken"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if detail.State != "setup_error" || detail.Reason != "cannot connect" {
		t.Fatalf("get_integration dropped the failed setup state: %+v", detail)
	}
}

// TestGetIntegration_UnknownID_ReturnsNotFound is Appendix B's "gone between
// list and get" case.
func TestGetIntegration_UnknownID_ReturnsNotFound(t *testing.T) {
	inventory := &fakeInventoryReader{integrations: []model.Integration{{ID: "entry-a"}}}
	avail := &fakeAvailabilityReader{unavailable: unavailableSet()}

	client := connect(t, newServer(integrationOptions(inventory, avail), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name:      "get_integration",
		Arguments: map[string]any{"id": "entry-does-not-exist"},
	})
	if err != nil {
		t.Fatalf("CallTool transport error: %v", err)
	}
	if !res.IsError {
		t.Fatal("get_integration with an unknown id did not return an error result")
	}
}

// TestIntegrationTools_SchemaRejectsFreeFormParameter pins the parity rule's
// fourth clause: an implemented list_* / get_* tool's schema still closes
// off any parameter beyond its own typed fields.
func TestIntegrationTools_SchemaRejectsFreeFormParameter(t *testing.T) {
	client := connect(t, newServer(integrationOptions(&fakeInventoryReader{}, &fakeAvailabilityReader{}), Catalog()))
	res, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name != "list_integrations" && tool.Name != "get_integration" {
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
