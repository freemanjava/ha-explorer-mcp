package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// deviceOptions is testOptions wired with the given readers.
func deviceOptions(inventory systemInventoryReader, avail entityAvailabilityReader) Options {
	opts := testOptions()
	opts.Inventory = inventory
	opts.Availability = avail
	return opts
}

// TestListDevices_Filters exercises the area_id, config_entry_id and
// disabled filters independently.
func TestListDevices_Filters(t *testing.T) {
	inventory := &fakeInventoryReader{
		devices: []model.DeviceRef{
			{ID: "dev-a", AreaID: "kitchen", ConfigEntryID: "entry-hue"},
			{ID: "dev-b", AreaID: "hallway", ConfigEntryID: "entry-hue"},
			{ID: "dev-c", AreaID: "kitchen", ConfigEntryID: "entry-mqtt", DisabledBy: "user"},
		},
	}
	avail := &fakeAvailabilityReader{unavailable: unavailableSet()}
	client := connect(t, newServer(deviceOptions(inventory, avail), Catalog()))

	cases := []struct {
		name string
		args map[string]any
		want []string
	}{
		{"area_id", map[string]any{"area_id": "kitchen"}, []string{"dev-a", "dev-c"}},
		{"config_entry_id", map[string]any{"config_entry_id": "entry-hue"}, []string{"dev-a", "dev-b"}},
		{"disabled true", map[string]any{"disabled": true}, []string{"dev-c"}},
		{"disabled false", map[string]any{"disabled": false}, []string{"dev-a", "dev-b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "list_devices", Arguments: tc.args})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			var list model.DeviceList
			raw, _ := json.Marshal(res.StructuredContent)
			if err := json.Unmarshal(raw, &list); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			got := make([]string, len(list.Items))
			for i, d := range list.Items {
				got[i] = string(d.ID)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("ids = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("ids = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// TestListDevices_CursorPagination_HonorsPhase02Contract pins default limit,
// clamping and the truncated/next_cursor pair together.
func TestListDevices_CursorPagination_HonorsPhase02Contract(t *testing.T) {
	var devices []model.DeviceRef
	for i := range 3 {
		devices = append(devices, model.DeviceRef{ID: model.DeviceID("dev-" + string(rune('a'+i)))})
	}
	inventory := &fakeInventoryReader{devices: devices}
	avail := &fakeAvailabilityReader{unavailable: unavailableSet()}
	client := connect(t, newServer(deviceOptions(inventory, avail), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name:      "list_devices",
		Arguments: map[string]any{"limit": 2},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var list model.DeviceList
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list.Items) != 2 || !list.Truncated || list.NextCursor == "" {
		t.Fatalf("first page = %+v, want 2 items, Truncated true, a NextCursor", list)
	}

	res2, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name:      "list_devices",
		Arguments: map[string]any{"limit": 2, "cursor": list.NextCursor},
	})
	if err != nil {
		t.Fatalf("CallTool (page 2): %v", err)
	}
	var list2 model.DeviceList
	raw2, _ := json.Marshal(res2.StructuredContent)
	if err := json.Unmarshal(raw2, &list2); err != nil {
		t.Fatalf("unmarshal page 2: %v", err)
	}
	if len(list2.Items) != 1 || list2.Truncated {
		t.Fatalf("second page = %+v, want the last remaining item and Truncated false", list2)
	}
}

// TestGetDevice_ReportsRelatedEntityAvailabilityAndTopology is the P3-04
// DoD's second and first lines: a device whose entities span availability
// states reports them accurately, and via/parent topology (both directions)
// is populated.
func TestGetDevice_ReportsRelatedEntityAvailabilityAndTopology(t *testing.T) {
	inventory := &fakeInventoryReader{
		devices: []model.DeviceRef{
			{ID: "dev-hub", Name: "Hub"},
			{ID: "dev-bulb", Name: "Bulb", ViaDeviceID: "dev-hub"},
		},
		entities: []model.Entity{
			{ID: "light.kitchen", Domain: "light", Name: "Kitchen", DeviceID: "dev-bulb"},
			{ID: "sensor.kitchen_battery", Domain: "sensor", Name: "Kitchen Battery", DeviceID: "dev-bulb"},
			{ID: "light.hallway", Domain: "light", Name: "Hallway", DeviceID: "dev-hub"},
		},
	}
	avail := &fakeAvailabilityReader{unavailable: unavailableSet("sensor.kitchen_battery")}
	client := connect(t, newServer(deviceOptions(inventory, avail), Catalog()))

	r, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name:      "get_device",
		Arguments: map[string]any{"id": "dev-bulb"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	raw, _ := json.Marshal(r.StructuredContent)
	var detail model.DeviceDetail
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(detail.RelatedEntities) != 2 {
		t.Fatalf("RelatedEntities = %+v, want 2", detail.RelatedEntities)
	}
	byID := map[model.EntityID]model.DeviceEntityRef{}
	for _, e := range detail.RelatedEntities {
		byID[e.ID] = e
	}
	if !byID["light.kitchen"].Available {
		t.Errorf("light.kitchen should be reported available")
	}
	if byID["sensor.kitchen_battery"].Available {
		t.Errorf("sensor.kitchen_battery should be reported unavailable")
	}

	if detail.ViaDevice == nil || detail.ViaDevice.ID != "dev-hub" {
		t.Fatalf("ViaDevice = %+v, want dev-hub", detail.ViaDevice)
	}
	if len(detail.ChildDevices) != 0 {
		t.Fatalf("ChildDevices = %+v, want none for a leaf device", detail.ChildDevices)
	}

	r2, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name:      "get_device",
		Arguments: map[string]any{"id": "dev-hub"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	raw2, _ := json.Marshal(r2.StructuredContent)
	var hub model.DeviceDetail
	if err := json.Unmarshal(raw2, &hub); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if hub.ViaDevice != nil {
		t.Fatalf("ViaDevice = %+v, want nil for a device with no parent", hub.ViaDevice)
	}
	if len(hub.ChildDevices) != 1 || hub.ChildDevices[0].ID != "dev-bulb" {
		t.Fatalf("ChildDevices = %+v, want [dev-bulb]", hub.ChildDevices)
	}
}

// TestGetDevice_UnknownID_ReturnsNotFound is Appendix B's "gone between list
// and get" case.
func TestGetDevice_UnknownID_ReturnsNotFound(t *testing.T) {
	inventory := &fakeInventoryReader{devices: []model.DeviceRef{{ID: "dev-a"}}}
	avail := &fakeAvailabilityReader{unavailable: unavailableSet()}
	client := connect(t, newServer(deviceOptions(inventory, avail), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name:      "get_device",
		Arguments: map[string]any{"id": "dev-does-not-exist"},
	})
	if err != nil {
		t.Fatalf("CallTool transport error: %v", err)
	}
	if !res.IsError {
		t.Fatal("get_device with an unknown id did not return an error result")
	}
}

// TestGetDevice_DoesNotClaimStableIdentity is the P3-04 DoD's first line: the
// response must not present device_id as a stable physical identity (doc
// §8) by introducing a field that implies permanence beyond the registry id
// itself.
func TestGetDevice_DoesNotClaimStableIdentity(t *testing.T) {
	inventory := &fakeInventoryReader{
		devices: []model.DeviceRef{{ID: "dev-a", SerialNumber: "SN123"}},
	}
	avail := &fakeAvailabilityReader{unavailable: unavailableSet()}
	client := connect(t, newServer(deviceOptions(inventory, avail), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name:      "get_device",
		Arguments: map[string]any{"id": "dev-a"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, forbidden := range []string{"physical_id", "PhysicalID", "stable_id", "StableID", "hardware_id", "HardwareID", "permanent_id", "PermanentID"} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("get_device response contains %q — it must not present device_id as a stable physical identity", forbidden)
		}
	}
}

// TestDeviceTools_SchemaRejectsFreeFormParameter pins the parity rule's
// fourth clause: an implemented list_* / get_* tool's schema still closes
// off any parameter beyond its own typed fields.
func TestDeviceTools_SchemaRejectsFreeFormParameter(t *testing.T) {
	client := connect(t, newServer(deviceOptions(&fakeInventoryReader{}, &fakeAvailabilityReader{}), Catalog()))
	res, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name != "list_devices" && tool.Name != "get_device" {
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
		if schema.AdditionalProperties != false {
			t.Errorf("%s: schema additionalProperties = %v, want false", tool.Name, schema.AdditionalProperties)
		}
	}
}
