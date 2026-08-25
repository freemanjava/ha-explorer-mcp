package ha

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

func readFixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return data
}

func TestMapEntityRegistryList_WellFormed_MapsAllFields(t *testing.T) {
	entities, err := MapEntityRegistryList(readFixture(t, "entity_registry_list.json"))
	if err != nil {
		t.Fatalf("MapEntityRegistryList: %v", err)
	}
	if len(entities) != 3 {
		t.Fatalf("got %d entities, want 3", len(entities))
	}

	kitchen := entities[0]
	if kitchen.ID != "sensor.kitchen_temperature" {
		t.Errorf("ID = %q", kitchen.ID)
	}
	if kitchen.Domain != "sensor" {
		t.Errorf("Domain = %q, want sensor", kitchen.Domain)
	}
	if kitchen.Platform != "zha" {
		t.Errorf("Platform = %q, want zha", kitchen.Platform)
	}
	if kitchen.DeviceID != "dev-kitchen-sensor-1" {
		t.Errorf("DeviceID = %q", kitchen.DeviceID)
	}
	if kitchen.DeviceClass != "temperature" {
		t.Errorf("DeviceClass = %q, want temperature (from original_device_class)", kitchen.DeviceClass)
	}
	if kitchen.Partial {
		t.Errorf("well-formed entity marked Partial: %s", kitchen.PartialReason)
	}
	if kitchen.CreatedAt.IsZero() {
		t.Error("CreatedAt not parsed")
	}

	lamp := entities[1]
	if lamp.Name != "Reading Lamp" {
		t.Errorf("Name = %q", lamp.Name)
	}

	disabled := entities[2]
	if disabled.DisabledBy != "user" {
		t.Errorf("DisabledBy = %q, want user", disabled.DisabledBy)
	}
	if disabled.EntityCategory != "diagnostic" {
		t.Errorf("EntityCategory = %q", disabled.EntityCategory)
	}
}

func TestMapEntityRegistryList_Malformed_MarksPartialWithoutPanicking(t *testing.T) {
	entities, err := MapEntityRegistryList(readFixture(t, "entity_registry_malformed.json"))
	if err != nil {
		t.Fatalf("MapEntityRegistryList: %v", err)
	}
	if len(entities) != 3 {
		t.Fatalf("got %d entities, want 3", len(entities))
	}

	for i, e := range entities {
		if !e.Partial {
			t.Errorf("entity[%d] not marked Partial, got %+v", i, e)
		}
		if e.PartialReason == "" {
			t.Errorf("entity[%d] Partial but no PartialReason", i)
		}
	}

	// entity_id wrong JSON type: the id must not silently become "12345" or
	// a zero value that looks like a real, absent entity.
	if entities[0].ID != "" {
		t.Errorf("entity[0].ID = %q, want empty for a non-string entity_id", entities[0].ID)
	}

	// missing platform, entity_id itself well-formed: the good field is not
	// discarded just because a sibling field was missing.
	if entities[1].ID != "sensor.no_platform" {
		t.Errorf("entity[1].ID = %q, want sensor.no_platform preserved", entities[1].ID)
	}
	if entities[1].Platform != "" {
		t.Errorf("entity[1].Platform = %q, want empty", entities[1].Platform)
	}
}

func TestMapEntity_OversizedMalformedUnicodeName_RoundTripsSafely(t *testing.T) {
	entities, err := MapEntityRegistryList(readFixture(t, "entity_registry_unicode.json"))
	if err != nil {
		t.Fatalf("MapEntityRegistryList: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("got %d entities, want 1", len(entities))
	}
	got := entities[0]

	var fixture []map[string]any
	if err := json.Unmarshal(readFixture(t, "entity_registry_unicode.json"), &fixture); err != nil {
		t.Fatalf("decoding fixture for comparison: %v", err)
	}
	wantName := fixture[0]["name"].(string)
	wantOriginalName := fixture[0]["original_name"].(string)

	if got.Name != wantName {
		t.Errorf("Name mangled: got %d bytes, want %d bytes", len(got.Name), len(wantName))
	}
	if got.OriginalName != wantOriginalName {
		t.Errorf("OriginalName mangled: got %d bytes, want %d bytes", len(got.OriginalName), len(wantOriginalName))
	}
	if got.Partial {
		t.Errorf("valid (if unusual) Unicode name should not mark the entity Partial: %s", got.PartialReason)
	}

	// The value must itself still be valid JSON when re-encoded — the "round
	// trips safely" half of the DoD, not just "the bytes are unchanged".
	if _, err := json.Marshal(got); err != nil {
		t.Fatalf("re-marshalling mapped entity: %v", err)
	}
}

func TestMapDeviceRegistryList_WellFormed_MapsAllFields(t *testing.T) {
	devices, err := MapDeviceRegistryList(readFixture(t, "device_registry_list.json"))
	if err != nil {
		t.Fatalf("MapDeviceRegistryList: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2", len(devices))
	}

	sensor := devices[0]
	if sensor.ID != "dev-kitchen-sensor-1" {
		t.Errorf("ID = %q", sensor.ID)
	}
	if sensor.ConfigEntryID != "entry-zha-1" {
		t.Errorf("ConfigEntryID = %q, want entry-zha-1 from config_entries[0]", sensor.ConfigEntryID)
	}
	if sensor.Name != "Kitchen Sensor" {
		t.Errorf("Name = %q", sensor.Name)
	}
	if len(sensor.Connections) != 1 || sensor.Connections[0] != [2]string{"zigbee", "00:11:22:33:44:55"} {
		t.Errorf("Connections = %v", sensor.Connections)
	}
	if sensor.Partial {
		t.Errorf("well-formed device marked Partial: %s", sensor.PartialReason)
	}
}

func TestMapDeviceRegistryList_Malformed_MarksPartialWithoutPanicking(t *testing.T) {
	devices, err := MapDeviceRegistryList(readFixture(t, "device_registry_malformed.json"))
	if err != nil {
		t.Fatalf("MapDeviceRegistryList: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2", len(devices))
	}
	if !devices[0].Partial {
		t.Error("device with non-string id not marked Partial")
	}

	// Bad connection pairs are dropped individually; the one good pair
	// survives instead of the whole field being discarded.
	if devices[1].Partial {
		t.Errorf("device[1] should not be Partial (id present); got reason %q", devices[1].PartialReason)
	}
	if len(devices[1].Connections) != 1 || devices[1].Connections[0] != [2]string{"type", "value"} {
		t.Errorf("Connections = %v, want only the well-formed pair", devices[1].Connections)
	}
}

func TestMapAreaRegistryList_WellFormed(t *testing.T) {
	areas, err := MapAreaRegistryList(readFixture(t, "area_registry_list.json"))
	if err != nil {
		t.Fatalf("MapAreaRegistryList: %v", err)
	}
	if len(areas) != 2 {
		t.Fatalf("got %d areas, want 2", len(areas))
	}
	if areas[0].ID != "area-kitchen" || areas[0].Name != "Kitchen" {
		t.Errorf("areas[0] = %+v", areas[0])
	}
	if areas[0].Partial {
		t.Errorf("well-formed area marked Partial: %s", areas[0].PartialReason)
	}
}

func TestMapConfigEntriesGet_WellFormed(t *testing.T) {
	integrations, err := MapConfigEntriesGet(readFixture(t, "config_entries_get.json"))
	if err != nil {
		t.Fatalf("MapConfigEntriesGet: %v", err)
	}
	if len(integrations) != 3 {
		t.Fatalf("got %d integrations, want 3", len(integrations))
	}

	hue := integrations[1]
	if hue.State != "setup_error" {
		t.Errorf("State = %q", hue.State)
	}
	if hue.Reason != "cannot_connect" {
		t.Errorf("Reason = %q", hue.Reason)
	}
	if integrations[0].Partial {
		t.Errorf("well-formed integration marked Partial: %s", integrations[0].PartialReason)
	}
}

func TestMapAutomation_WellFormed(t *testing.T) {
	raw := readFixture(t, "automation_config.json")
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}

	a := MapAutomation(model.EntityID("automation.porch_light_at_sunset"), decoded)
	if a.EntityID != "automation.porch_light_at_sunset" {
		t.Errorf("EntityID = %q", a.EntityID)
	}
	if a.Alias != "Turn on porch light at sunset" {
		t.Errorf("Alias = %q", a.Alias)
	}
	if a.TriggerCount != 1 || a.ConditionCount != 0 || a.ActionCount != 1 {
		t.Errorf("counts = trigger:%d condition:%d action:%d, want 1/0/1",
			a.TriggerCount, a.ConditionCount, a.ActionCount)
	}
	if a.Partial {
		t.Errorf("well-formed automation marked Partial: %s", a.PartialReason)
	}
}

func TestMapAutomation_MissingRequiredFields_MarksPartial(t *testing.T) {
	a := MapAutomation(model.EntityID("automation.broken"), map[string]any{
		"mode": "single",
	})
	if !a.Partial {
		t.Error("automation missing id and alias not marked Partial")
	}
}
