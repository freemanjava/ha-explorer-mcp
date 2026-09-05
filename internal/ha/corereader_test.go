package ha

import (
	"encoding/json"
	"testing"
)

func TestCoreReader_CoreConfig_MapsFields(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandGetConfig, json.RawMessage(`{"version":"2026.8.3","location_name":"Home","time_zone":"UTC","state":"RUNNING"}`))

	cfg, err := NewCoreReader(fc).CoreConfig(testCtx(t))
	if err != nil {
		t.Fatalf("CoreConfig: %v", err)
	}
	if cfg.Version != "2026.8.3" || cfg.LocationName != "Home" || cfg.TimeZone != "UTC" || cfg.State != "RUNNING" {
		t.Fatalf("CoreConfig mapped %+v unexpectedly", cfg)
	}
	if cfg.Partial {
		t.Errorf("well-formed get_config marked Partial: %s", cfg.PartialReason)
	}
}

func TestCoreReader_CoreConfig_MissingVersion_MarksPartial(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandGetConfig, json.RawMessage(`{"location_name":"Home"}`))

	cfg, err := NewCoreReader(fc).CoreConfig(testCtx(t))
	if err != nil {
		t.Fatalf("CoreConfig: %v", err)
	}
	if !cfg.Partial {
		t.Fatal("get_config missing version was not marked Partial")
	}
}

func TestCoreReader_StateCounts_AggregatesWithoutExposingEntities(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandGetStates, json.RawMessage(`[
		{"entity_id":"light.kitchen","state":"on"},
		{"entity_id":"light.hallway","state":"unavailable"},
		{"entity_id":"sensor.attic","state":"unknown"},
		{"entity_id":"sensor.basement","state":"unknown"}
	]`))

	counts, err := NewCoreReader(fc).StateCounts(testCtx(t))
	if err != nil {
		t.Fatalf("StateCounts: %v", err)
	}
	if counts.Total != 4 || counts.Unavailable != 1 || counts.Unknown != 2 {
		t.Fatalf("StateCounts = %+v, want Total 4, Unavailable 1, Unknown 2", counts)
	}
}

func TestCoreReader_UnavailableEntityIDs_AggregatesWithoutExposingStates(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandGetStates, json.RawMessage(`[
		{"entity_id":"light.kitchen","state":"on"},
		{"entity_id":"light.hallway","state":"unavailable"},
		{"entity_id":"sensor.attic","state":"unknown"}
	]`))

	ids, err := NewCoreReader(fc).UnavailableEntityIDs(testCtx(t))
	if err != nil {
		t.Fatalf("UnavailableEntityIDs: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("ids = %v, want exactly 2 entries", ids)
	}
}

func TestCoreReader_States_ReturnsPerEntityState(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandGetStates, json.RawMessage(`[
		{"entity_id":"light.kitchen","state":"on"},
		{"entity_id":"light.hallway","state":"unavailable"}
	]`))

	states, err := NewCoreReader(fc).States(testCtx(t))
	if err != nil {
		t.Fatalf("States: %v", err)
	}
	if states["light.kitchen"] != "on" || states["light.hallway"] != "unavailable" {
		t.Fatalf("States = %v, want light.kitchen=on and light.hallway=unavailable", states)
	}
}

func TestCoreReader_Automations_FiltersToAutomationDomain(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandGetStates, json.RawMessage(`[
		{"entity_id":"automation.morning","state":"on","attributes":{"friendly_name":"Morning","last_triggered":"2026-09-01T12:00:00+00:00"}},
		{"entity_id":"light.kitchen","state":"on","attributes":{}}
	]`))

	automations, err := NewCoreReader(fc).Automations(testCtx(t))
	if err != nil {
		t.Fatalf("Automations: %v", err)
	}
	if len(automations) != 1 || automations[0].EntityID != "automation.morning" {
		t.Fatalf("Automations = %+v, want only automation.morning", automations)
	}
	if !automations[0].Enabled {
		t.Errorf("automation.morning Enabled = false, want true")
	}
}

func TestCoreReader_Repairs_MapsIssues(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandRepairsListIssues, json.RawMessage(`{
		"issues": [
			{"issue_id": "deprecated_setting", "domain": "sun", "severity": "warning", "created": "2026-09-01T12:00:00+00:00"}
		]
	}`))

	repairs, err := NewCoreReader(fc).Repairs(testCtx(t))
	if err != nil {
		t.Fatalf("Repairs: %v", err)
	}
	if len(repairs) != 1 || repairs[0].IssueID != "deprecated_setting" {
		t.Fatalf("Repairs = %+v, want one deprecated_setting entry", repairs)
	}
}

func TestCoreReader_UpstreamError_Propagates(t *testing.T) {
	fc := newFakeCaller()
	fc.err = ErrUpstreamUnavailable

	if _, err := NewCoreReader(fc).CoreConfig(testCtx(t)); err == nil {
		t.Fatal("CoreConfig swallowed the upstream error")
	}
	if _, err := NewCoreReader(fc).StateCounts(testCtx(t)); err == nil {
		t.Fatal("StateCounts swallowed the upstream error")
	}
	if _, err := NewCoreReader(fc).UnavailableEntityIDs(testCtx(t)); err == nil {
		t.Fatal("UnavailableEntityIDs swallowed the upstream error")
	}
	if _, err := NewCoreReader(fc).States(testCtx(t)); err == nil {
		t.Fatal("States swallowed the upstream error")
	}
	if _, err := NewCoreReader(fc).Automations(testCtx(t)); err == nil {
		t.Fatal("Automations swallowed the upstream error")
	}
	if _, err := NewCoreReader(fc).Repairs(testCtx(t)); err == nil {
		t.Fatal("Repairs swallowed the upstream error")
	}
}
