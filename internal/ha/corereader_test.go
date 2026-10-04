package ha

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
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

func TestCoreReader_AutomationDetail_MapsConfig(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandAutomationConfig, json.RawMessage(`{"config":{"id":"123","alias":"Evening lights","mode":"single","triggers":[{"trigger":"state"}],"conditions":[],"actions":[{"action":"light.turn_on"}]}}`))

	a, err := NewCoreReader(fc).AutomationDetail(testCtx(t), "automation.evening_lights")
	if err != nil {
		t.Fatalf("AutomationDetail: %v", err)
	}
	if a.Alias != "Evening lights" || a.TriggerCount != 1 || a.ActionCount != 1 {
		t.Fatalf("AutomationDetail mapped %+v unexpectedly", a)
	}
}

// traceStoreCaller answers trace/list the way HA's trace store does: keyed by
// the automation's config id, and an empty list for any other item_id (F-47).
type traceStoreCaller struct {
	*fakeCaller
	configID string
	traces   json.RawMessage
}

func (c *traceStoreCaller) Call(ctx context.Context, cmd Command) (json.RawMessage, error) {
	raw, err := c.fakeCaller.Call(ctx, cmd)
	if t, ok := cmd.(traceListCommand); ok {
		if t.ItemID != c.configID {
			return json.RawMessage(`[]`), nil
		}
		return c.traces, nil
	}
	return raw, err
}

func TestCoreReader_AutomationTraces_KeysTraceListByConfigID(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandAutomationConfig, json.RawMessage(`{"config":{"id":"1700000000001","alias":"Evening lights"}}`))
	store := &traceStoreCaller{
		fakeCaller: fc,
		configID:   "1700000000001",
		traces:     json.RawMessage(`[{"run_id":"r1","state":"stopped","script_execution":"finished","timestamp":{"start":"2026-08-22T19:04:11+00:00"}}]`),
	}

	traces, err := NewCoreReader(store).AutomationTraces(testCtx(t), "automation.evening_lights")
	if err != nil {
		t.Fatalf("AutomationTraces: %v", err)
	}
	if len(traces) != 1 || traces[0].RunID != "r1" {
		t.Fatalf("AutomationTraces = %+v, want one run r1", traces)
	}
}

func TestCoreReader_AutomationTraces_NoConfigID_UnsupportedWithoutTraceFrame(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandAutomationConfig, json.RawMessage(`{"config":{"alias":"YAML only"}}`))

	_, err := NewCoreReader(fc).AutomationTraces(testCtx(t), "automation.yaml_only")
	if !errors.Is(err, ErrUnsupported) || !errors.Is(err, ErrAutomationHasNoConfigID) {
		t.Fatalf("AutomationTraces error = %v, want ErrAutomationHasNoConfigID (an ErrUnsupported)", err)
	}
	if n := fc.callCount(CommandTraceList); n != 0 {
		t.Fatalf("trace/list sent %d times for an automation with no config id, want 0", n)
	}
}

func TestCoreReader_AutomationTraces_ConfigReadRefused_Propagates(t *testing.T) {
	fc := newFakeCaller()
	fc.err = &CommandError{Code: "unauthorized", Message: "unauthorized"}

	_, err := NewCoreReader(fc).AutomationTraces(testCtx(t), "automation.evening_lights")
	if !errors.Is(err, ErrUnsupported) || errors.Is(err, ErrAutomationHasNoConfigID) {
		t.Fatalf("AutomationTraces error = %v, want a plain permission ErrUnsupported", err)
	}
}

func TestCoreReader_LogbookEvents_MapsEvents(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandLogbookGetEvents, json.RawMessage(`[{"when":"2026-08-22T19:04:11+00:00","name":"Evening lights","context_id":"ctx1"}]`))

	events, err := NewCoreReader(fc).LogbookEvents(testCtx(t), "automation.evening_lights", time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("LogbookEvents: %v", err)
	}
	if len(events) != 1 || events[0].ContextID != "ctx1" {
		t.Fatalf("LogbookEvents = %+v, want one event with ContextID ctx1", events)
	}
}

func TestCoreReader_History_MapsPointsAndSetsBothFlagsFromMinimal(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandHistoryDuringPeriod, json.RawMessage(`{"sensor.x": [{"lu": 1755000000, "s": "1"}]}`))

	from := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	points, err := NewCoreReader(fc).History(testCtx(t), "sensor.x", from, to, true)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(points) != 1 || points[0].State != "1" {
		t.Fatalf("History = %+v, want one point with state 1", points)
	}
}

func TestCoreReader_AutomationDetail_PermissionRefused_ReturnsUnsupported(t *testing.T) {
	fc := newFakeCaller()
	fc.err = &CommandError{Code: "unauthorized", Message: "unauthorized"}

	if _, err := NewCoreReader(fc).AutomationDetail(testCtx(t), "automation.evening_lights"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("AutomationDetail error = %v, want errors.Is(err, ErrUnsupported)", err)
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
	if _, err := NewCoreReader(fc).AutomationDetail(testCtx(t), "automation.evening_lights"); err == nil {
		t.Fatal("AutomationDetail swallowed the upstream error")
	}
	if _, err := NewCoreReader(fc).AutomationTraces(testCtx(t), "automation.evening_lights"); err == nil {
		t.Fatal("AutomationTraces swallowed the upstream error")
	}
	if _, err := NewCoreReader(fc).LogbookEvents(testCtx(t), "automation.evening_lights", time.Now()); err == nil {
		t.Fatal("LogbookEvents swallowed the upstream error")
	}
}

func TestCoreReader_LifecycleEvents_KeepsOnlyCoreRows(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandLogbookGetEvents, json.RawMessage(`[
		{"when":"2026-10-03T10:00:00+00:00","name":"Home Assistant","message":"stopped","domain":"homeassistant"},
		{"when":"2026-10-03T10:01:00+00:00","name":"Kitchen light","message":"turned off","domain":"light","entity_id":"light.kitchen"},
		{"when":"2026-10-03T10:02:00+00:00","name":"Home Assistant","message":"started","domain":"homeassistant"},
		{"name":"Home Assistant","domain":"homeassistant"}]`))

	from := time.Date(2026, 10, 3, 9, 55, 0, 0, time.UTC)
	events, err := NewCoreReader(fc).LifecycleEvents(testCtx(t), from, from.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("LifecycleEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("LifecycleEvents = %+v, want the two timestamped core rows and nothing else", events)
	}
}

func TestCoreReader_LifecycleEvents_UpstreamError_NotSwallowed(t *testing.T) {
	fc := newFakeCaller()
	fc.err = errors.New("boom")
	if _, err := NewCoreReader(fc).LifecycleEvents(testCtx(t), time.Now().Add(-time.Minute), time.Now()); err == nil {
		t.Fatal("LifecycleEvents swallowed the upstream error")
	}
}

func TestLogbookWindowCommand_Wire_BoundedAndUnfiltered(t *testing.T) {
	from := time.Date(2026, 10, 3, 9, 55, 0, 0, time.UTC)
	b, err := json.Marshal(logbookWindowCommand{StartTime: from, EndTime: from.Add(10 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["end_time"]; !ok {
		t.Errorf("wire form %s has no end_time: the read would be unbounded", b)
	}
	if _, ok := m["entity_ids"]; ok {
		t.Errorf("wire form %s carries entity_ids", b)
	}
	if (logbookWindowCommand{}).CommandType() != CommandLogbookGetEvents {
		t.Error("window command must stay inside the allow-listed logbook command")
	}
}
