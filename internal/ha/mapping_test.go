package ha

import (
	"encoding/json"
	"errors"
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

func TestMapCoreConfig_WellFormed(t *testing.T) {
	cfg, err := MapCoreConfig(json.RawMessage(`{
		"version": "2026.8.3",
		"location_name": "Home",
		"time_zone": "Europe/Berlin",
		"state": "RUNNING"
	}`))
	if err != nil {
		t.Fatalf("MapCoreConfig: %v", err)
	}
	if cfg.Version != "2026.8.3" || cfg.LocationName != "Home" || cfg.TimeZone != "Europe/Berlin" || cfg.State != "RUNNING" {
		t.Fatalf("MapCoreConfig mapped %+v unexpectedly", cfg)
	}
	if cfg.Partial {
		t.Errorf("well-formed get_config marked Partial: %s", cfg.PartialReason)
	}
}

func TestMapCoreConfig_MissingVersion_MarksPartial(t *testing.T) {
	cfg, err := MapCoreConfig(json.RawMessage(`{"location_name": "Home"}`))
	if err != nil {
		t.Fatalf("MapCoreConfig: %v", err)
	}
	if !cfg.Partial {
		t.Error("get_config missing version was not marked Partial")
	}
}

func TestMapCoreConfig_NotAnObject_Errors(t *testing.T) {
	if _, err := MapCoreConfig(json.RawMessage(`[1,2,3]`)); err == nil {
		t.Fatal("MapCoreConfig accepted a non-object payload")
	}
}

func TestMapStateCounts_CountsWithoutExposingEntities(t *testing.T) {
	counts, err := MapStateCounts(json.RawMessage(`[
		{"entity_id":"light.kitchen","state":"on"},
		{"entity_id":"light.hallway","state":"unavailable"},
		{"entity_id":"sensor.attic","state":"unknown"},
		{"entity_id":"sensor.basement","state":"unavailable"}
	]`))
	if err != nil {
		t.Fatalf("MapStateCounts: %v", err)
	}
	if counts.Total != 4 || counts.Unavailable != 2 || counts.Unknown != 1 {
		t.Fatalf("MapStateCounts = %+v, want Total 4, Unavailable 2, Unknown 1", counts)
	}
}

func TestMapStateCounts_MalformedElement_Skipped(t *testing.T) {
	counts, err := MapStateCounts(json.RawMessage(`[
		{"entity_id":"light.kitchen","state":"on"},
		"not an object",
		{"entity_id":"sensor.attic","state":"unknown"}
	]`))
	if err != nil {
		t.Fatalf("MapStateCounts: %v", err)
	}
	if counts.Total != 2 || counts.Unknown != 1 {
		t.Fatalf("MapStateCounts = %+v, want the malformed element skipped, not fatal", counts)
	}
}

func TestMapUnavailableEntityIDs_ReturnsOnlyUnavailableOrUnknown(t *testing.T) {
	ids, err := MapUnavailableEntityIDs(json.RawMessage(`[
		{"entity_id":"light.kitchen","state":"on"},
		{"entity_id":"light.hallway","state":"unavailable"},
		{"entity_id":"sensor.attic","state":"unknown"},
		{"entity_id":"sensor.basement","state":"off"}
	]`))
	if err != nil {
		t.Fatalf("MapUnavailableEntityIDs: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("ids = %v, want exactly 2 entries", ids)
	}
	for _, want := range []model.EntityID{"light.hallway", "sensor.attic"} {
		if _, ok := ids[want]; !ok {
			t.Errorf("ids missing %q", want)
		}
	}
}

func TestMapUnavailableEntityIDs_MalformedElement_Skipped(t *testing.T) {
	ids, err := MapUnavailableEntityIDs(json.RawMessage(`[
		{"entity_id":"light.hallway","state":"unavailable"},
		"not an object",
		{"state":"unknown"}
	]`))
	if err != nil {
		t.Fatalf("MapUnavailableEntityIDs: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("ids = %v, want the malformed and entity_id-less elements skipped", ids)
	}
}

func TestMapEntityStateValues_ReturnsPerEntityState(t *testing.T) {
	states, err := MapEntityStateValues(json.RawMessage(`[
		{"entity_id":"light.kitchen","state":"on"},
		{"entity_id":"light.hallway","state":"unavailable"}
	]`))
	if err != nil {
		t.Fatalf("MapEntityStateValues: %v", err)
	}
	if len(states) != 2 || states["light.kitchen"] != "on" || states["light.hallway"] != "unavailable" {
		t.Fatalf("states = %v, want light.kitchen=on and light.hallway=unavailable", states)
	}
}

func TestMapEntityStateValues_MalformedElement_Skipped(t *testing.T) {
	states, err := MapEntityStateValues(json.RawMessage(`[
		{"entity_id":"light.kitchen","state":"on"},
		"not an object",
		{"state":"unknown"}
	]`))
	if err != nil {
		t.Fatalf("MapEntityStateValues: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("states = %v, want the malformed and entity_id-less elements skipped", states)
	}
}

func TestMapCoreInfo_WellFormed(t *testing.T) {
	info, err := MapCoreInfo(json.RawMessage(`{
		"supervisor": "2026.08.0",
		"homeassistant": "2026.8.3",
		"hassos": "14.2",
		"hostname": "homeassistant",
		"machine": "rpi4",
		"arch": "aarch64",
		"state": "running",
		"supported": true
	}`))
	if err != nil {
		t.Fatalf("MapCoreInfo: %v", err)
	}
	if info.CoreVersion != "2026.8.3" || info.SupervisorVersion != "2026.08.0" || info.OSVersion != "14.2" {
		t.Fatalf("MapCoreInfo mapped %+v unexpectedly", info)
	}
	if info.Hostname != "homeassistant" || info.Machine != "rpi4" || info.Arch != "aarch64" {
		t.Fatalf("MapCoreInfo mapped %+v unexpectedly", info)
	}
	if info.State != "running" || !info.Supported {
		t.Fatalf("MapCoreInfo mapped %+v unexpectedly", info)
	}
}

func TestMapCoreInfo_MutatedShape_FailsLoudly(t *testing.T) {
	if _, err := MapCoreInfo(json.RawMessage(`{"supported": "yes"}`)); !errors.Is(err, ErrUnexpectedMessage) {
		t.Fatalf("MapCoreInfo: got %v, want ErrUnexpectedMessage", err)
	}
}

func TestMapHostDisk_WellFormed(t *testing.T) {
	disk, err := MapHostDisk(json.RawMessage(`{"disk_free": 10.5, "disk_total": 32, "disk_used": 21.5}`))
	if err != nil {
		t.Fatalf("MapHostDisk: %v", err)
	}
	if disk.FreeGB != 10.5 || disk.TotalGB != 32 || disk.UsedGB != 21.5 {
		t.Fatalf("MapHostDisk mapped %+v unexpectedly", disk)
	}
}

func TestMapResolutionInfo_WellFormed(t *testing.T) {
	summary, err := MapResolutionInfo(json.RawMessage(`{
		"unhealthy": ["privileged"],
		"unsupported": [],
		"issues": [{"uuid": "1", "type": "free_space"}]
	}`))
	if err != nil {
		t.Fatalf("MapResolutionInfo: %v", err)
	}
	if summary.IssueCount != 1 || len(summary.Unhealthy) != 1 || summary.Unhealthy[0] != "privileged" {
		t.Fatalf("MapResolutionInfo mapped %+v unexpectedly", summary)
	}
}

func TestMapAddonStats_WellFormed(t *testing.T) {
	stats, err := MapAddonStats(json.RawMessage(`{"cpu_percent": 1.5, "memory_percent": 4.2}`))
	if err != nil {
		t.Fatalf("MapAddonStats: %v", err)
	}
	if stats.CPUPercent != 1.5 || stats.MemoryPercent != 4.2 {
		t.Fatalf("MapAddonStats mapped %+v unexpectedly", stats)
	}
}

// TestMapFloorRegistryList_WellFormed exercises MapFloor's assumed field
// names. The floor_registry element schema was unobserved by the 2026-08-23
// probe (docs/research/2026-08-23-ha-registry-apis.md finding 8); this test
// pins the assumption, not an observation, so a defensive mapper's contract
// stays visible.
func TestMapFloorRegistryList_WellFormed(t *testing.T) {
	floors, err := MapFloorRegistryList(json.RawMessage(`[
		{"floor_id": "floor-1", "name": "Ground Floor", "icon": "mdi:home-floor-0"}
	]`))
	if err != nil {
		t.Fatalf("MapFloorRegistryList: %v", err)
	}
	if len(floors) != 1 {
		t.Fatalf("got %d floors, want 1", len(floors))
	}
	if floors[0].ID != "floor-1" || floors[0].Name != "Ground Floor" {
		t.Errorf("floors[0] = %+v", floors[0])
	}
	if floors[0].Partial {
		t.Errorf("well-formed floor marked Partial: %s", floors[0].PartialReason)
	}
}

func TestMapFloor_MissingFields_MarksPartial(t *testing.T) {
	f := MapFloor(map[string]any{"icon": "mdi:home"})
	if !f.Partial {
		t.Error("floor missing floor_id and name not marked Partial")
	}
}

// TestMapLabelRegistryList_WellFormed is TestMapFloorRegistryList_WellFormed's
// counterpart for labels; same unverified-schema caveat.
func TestMapLabelRegistryList_WellFormed(t *testing.T) {
	labels, err := MapLabelRegistryList(json.RawMessage(`[
		{"label_id": "label-a", "name": "Important", "color": "red"}
	]`))
	if err != nil {
		t.Fatalf("MapLabelRegistryList: %v", err)
	}
	if len(labels) != 1 {
		t.Fatalf("got %d labels, want 1", len(labels))
	}
	if labels[0].ID != "label-a" || labels[0].Name != "Important" || labels[0].Color != "red" {
		t.Errorf("labels[0] = %+v", labels[0])
	}
	if labels[0].Partial {
		t.Errorf("well-formed label marked Partial: %s", labels[0].PartialReason)
	}
}

func TestMapLabel_MissingFields_MarksPartial(t *testing.T) {
	l := MapLabel(map[string]any{"icon": "mdi:tag"})
	if !l.Partial {
		t.Error("label missing label_id and name not marked Partial")
	}
}

// TestMapAutomationStates_FiltersToAutomationDomainAndMapsAttributes is
// list_automations' confirmed non-admin fallback source
// (docs/research/2026-08-23-ha-automation-traces.md): enabled state and
// last_triggered from get_states, never automation/config.
func TestMapAutomationStates_FiltersToAutomationDomainAndMapsAttributes(t *testing.T) {
	automations, err := MapAutomationStates(json.RawMessage(`[
		{
			"entity_id": "automation.morning",
			"state": "on",
			"attributes": {
				"friendly_name": "Morning",
				"last_triggered": "2026-09-01T12:00:00+00:00",
				"mode": "single",
				"current": 0
			}
		},
		{
			"entity_id": "automation.never_run",
			"state": "off",
			"attributes": {"friendly_name": "Never Run"}
		},
		{"entity_id": "light.kitchen", "state": "on", "attributes": {}}
	]`))
	if err != nil {
		t.Fatalf("MapAutomationStates: %v", err)
	}
	if len(automations) != 2 {
		t.Fatalf("got %d automations, want 2 (light.kitchen excluded)", len(automations))
	}

	byID := map[model.EntityID]model.AutomationSummary{}
	for _, a := range automations {
		byID[a.EntityID] = a
	}
	morning := byID["automation.morning"]
	if !morning.Enabled {
		t.Errorf("automation.morning Enabled = false, want true")
	}
	if morning.Alias != "Morning" || morning.Mode != "single" {
		t.Errorf("morning = %+v", morning)
	}
	if morning.LastTriggered == nil || morning.LastTriggered.Year() != 2026 {
		t.Errorf("morning.LastTriggered = %v", morning.LastTriggered)
	}

	neverRun := byID["automation.never_run"]
	if neverRun.Enabled {
		t.Errorf("automation.never_run Enabled = true, want false")
	}
	if neverRun.LastTriggered != nil {
		t.Errorf("automation.never_run LastTriggered = %v, want nil", neverRun.LastTriggered)
	}
}

func TestMapAutomationStates_MissingAttributes_MarksPartial(t *testing.T) {
	automations, err := MapAutomationStates(json.RawMessage(`[
		{"entity_id": "automation.broken", "state": "on"}
	]`))
	if err != nil {
		t.Fatalf("MapAutomationStates: %v", err)
	}
	if len(automations) != 1 || !automations[0].Partial {
		t.Fatalf("automations = %+v, want one entry marked Partial", automations)
	}
}

// TestMapRepairs_UnwrapsIssuesObjectAndMapsFields exercises the shape the
// 2026-09-05 research doc observed: {"issues": [...]}, an object wrapping the
// array, not the bare array get_states/registry commands return.
func TestMapRepairs_UnwrapsIssuesObjectAndMapsFields(t *testing.T) {
	repairs, err := MapRepairs(json.RawMessage(`{
		"issues": [
			{
				"breaks_in_ha_version": null,
				"created": "2026-09-01T12:00:00+00:00",
				"dismissed_version": "2026.8.3",
				"domain": "sun",
				"ignored": false,
				"is_fixable": true,
				"issue_domain": null,
				"issue_id": "deprecated_setting",
				"learn_more_url": null,
				"severity": "warning",
				"translation_key": "deprecated_setting",
				"translation_placeholders": {"entity_id": "sun.sun", "name": "Sun"}
			}
		]
	}`))
	if err != nil {
		t.Fatalf("MapRepairs: %v", err)
	}
	if len(repairs) != 1 {
		t.Fatalf("got %d repairs, want 1", len(repairs))
	}
	r := repairs[0]
	if r.IssueID != "deprecated_setting" || r.Domain != "sun" || r.Severity != "warning" {
		t.Fatalf("repair = %+v", r)
	}
	if !r.IsFixable || r.Ignored {
		t.Errorf("repair fixable/ignored flags = %+v", r)
	}
	if r.DismissedVersion != "2026.8.3" {
		t.Errorf("DismissedVersion = %q", r.DismissedVersion)
	}
	if r.Created.Year() != 2026 {
		t.Errorf("Created = %v", r.Created)
	}
	if r.TranslationPlaceholders["entity_id"] != "sun.sun" {
		t.Errorf("TranslationPlaceholders = %v", r.TranslationPlaceholders)
	}
	if r.Partial {
		t.Errorf("well-formed repair marked Partial: %s", r.PartialReason)
	}
}

func TestMapRepairs_MissingFields_MarksPartial(t *testing.T) {
	repairs, err := MapRepairs(json.RawMessage(`{"issues": [{"domain": "sun"}]}`))
	if err != nil {
		t.Fatalf("MapRepairs: %v", err)
	}
	if len(repairs) != 1 || !repairs[0].Partial {
		t.Fatalf("repairs = %+v, want one entry marked Partial", repairs)
	}
}

func TestMapRepairs_EmptyIssuesList(t *testing.T) {
	repairs, err := MapRepairs(json.RawMessage(`{"issues": []}`))
	if err != nil {
		t.Fatalf("MapRepairs: %v", err)
	}
	if len(repairs) != 0 {
		t.Fatalf("repairs = %+v, want empty", repairs)
	}
}
