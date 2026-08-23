package main

import (
	"testing"
	"time"
)

func TestPickHistoryTarget_PrefersNumericSensorWithStateClass(t *testing.T) {
	states := []any{
		state("light.kitchen", "on", map[string]any{}),
		state("sensor.free_text", "idle", map[string]any{"unit_of_measurement": "°C"}),
		state("sensor.humidity", "51.0", map[string]any{"unit_of_measurement": "%"}),
		state("sensor.power", "812.4", map[string]any{"unit_of_measurement": "W", "state_class": "measurement"}),
	}

	got := pickHistoryTarget(states)

	if got.entityID != "sensor.power" {
		t.Fatalf("entityID = %q, want sensor.power", got.entityID)
	}
	if !got.hasStateClass {
		t.Errorf("hasStateClass = false, want true")
	}
}

func TestPickHistoryTarget_NoStateClass_FallsBackToNumericSensor(t *testing.T) {
	states := []any{
		state("light.kitchen", "on", map[string]any{}),
		state("sensor.humidity", "51.0", map[string]any{"unit_of_measurement": "%"}),
	}

	got := pickHistoryTarget(states)

	if got.entityID != "sensor.humidity" {
		t.Fatalf("entityID = %q, want sensor.humidity", got.entityID)
	}
	if got.hasStateClass {
		t.Errorf("hasStateClass = true, want false — an empty statistics answer must stay readable")
	}
}

func TestPickHistoryTarget_NoNumericSensor_FallsBackToAnyEntity(t *testing.T) {
	states := []any{state("light.kitchen", "on", map[string]any{})}

	got := pickHistoryTarget(states)

	if got.entityID != "light.kitchen" {
		t.Fatalf("entityID = %q, want light.kitchen", got.entityID)
	}
}

func TestPickHistoryTarget_Empty_ReturnsZero(t *testing.T) {
	if got := pickHistoryTarget(nil); got.entityID != "" {
		t.Fatalf("entityID = %q, want empty", got.entityID)
	}
}

func TestHistoryPath_Bounded_CarriesWindowAndFilter(t *testing.T) {
	start := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	got := historyPath("sensor.power", start, end, historyOpts{})

	want := "/api/history/period/2026-08-23T00:00:00Z" +
		"?end_time=2026-08-24T00%3A00%3A00Z&filter_entity_id=sensor.power"
	if got != want {
		t.Fatalf("historyPath() = %q, want %q", got, want)
	}
}

func TestHistoryPath_Flags_AppendedAsBareParameters(t *testing.T) {
	start := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)

	got := historyPath("sensor.power", start, start.Add(time.Hour), historyOpts{
		minimalResponse: true,
		noAttributes:    true,
	})

	// HA reads both as presence flags; "=true" is not the documented form.
	want := "/api/history/period/2026-08-23T00:00:00Z" +
		"?end_time=2026-08-23T01%3A00%3A00Z&filter_entity_id=sensor.power" +
		"&minimal_response&no_attributes"
	if got != want {
		t.Fatalf("historyPath() = %q, want %q", got, want)
	}
}

func TestPickStatisticID_PrefersTheTargetEntity(t *testing.T) {
	list := []any{
		map[string]any{"statistic_id": "sensor.other", "has_mean": true},
		map[string]any{"statistic_id": "sensor.power", "has_mean": true},
	}

	if got := pickStatisticID(list, "sensor.power"); got != "sensor.power" {
		t.Fatalf("pickStatisticID() = %q, want sensor.power", got)
	}
}

func TestPickStatisticID_TargetAbsent_FallsBackToFirstWithData(t *testing.T) {
	list := []any{
		map[string]any{"statistic_id": "sensor.no_data"},
		map[string]any{"statistic_id": "sensor.energy", "has_sum": true},
	}

	if got := pickStatisticID(list, "sensor.power"); got != "sensor.energy" {
		t.Fatalf("pickStatisticID() = %q, want sensor.energy", got)
	}
}

func TestPickStatisticID_NoCandidates_ReturnsEmpty(t *testing.T) {
	if got := pickStatisticID([]any{}, "sensor.power"); got != "" {
		t.Fatalf("pickStatisticID() = %q, want empty", got)
	}
}

func state(entityID, value string, attrs map[string]any) any {
	return map[string]any{"entity_id": entityID, "state": value, "attributes": attrs}
}

func TestRedactor_ReplacesEveryKnownID(t *testing.T) {
	var r redactor
	r.add("sensor.power")
	r.add("sensor.energy")

	got := r.apply("sensor.power: array[3]\nsensor.energy: array[3]\n")

	want := "<target entity>: array[3]\n<target entity>: array[3]\n"
	if got != want {
		t.Fatalf("apply() = %q, want %q", got, want)
	}
}

func TestRedactor_EmptyID_Ignored(t *testing.T) {
	var r redactor
	r.add("")

	// An empty needle would otherwise match between every character.
	if got := r.apply("abc"); got != "abc" {
		t.Fatalf("apply() = %q, want abc", got)
	}
}
