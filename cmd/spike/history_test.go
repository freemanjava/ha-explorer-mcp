package main

import "testing"

func TestPickHistoryTargets_NumericSensorsFirst(t *testing.T) {
	states := []any{
		state("light.kitchen", "on", map[string]any{}),
		state("sensor.free_text", "idle", map[string]any{}),
		state("sensor.power", "812.4", map[string]any{"state_class": "measurement"}),
		state("sensor.humidity", "51.0", map[string]any{}),
	}

	got := pickHistoryTargets(states)

	want := []string{"sensor.power", "sensor.humidity", "light.kitchen", "sensor.free_text"}
	assertIDs(t, got, want)
}

func TestPickHistoryTargets_MoreThanTheCap_Truncated(t *testing.T) {
	var states []any
	for i := 0; i < maxEntities+37; i++ {
		states = append(states, state("sensor.n"+itoa(i), "1.0", map[string]any{}))
	}

	if got := len(pickHistoryTargets(states)); got != maxEntities {
		t.Fatalf("len = %d, want %d — no probe may exceed doc §10's entity cap", got, maxEntities)
	}
}

func TestPickHistoryTargets_Empty_ReturnsNone(t *testing.T) {
	if got := pickHistoryTargets(nil); len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestPickStatisticIDs_SkipsEntriesWithoutCompiledData(t *testing.T) {
	list := []any{
		map[string]any{"statistic_id": "sensor.no_data"},
		map[string]any{"statistic_id": "sensor.energy", "has_sum": true},
		map[string]any{"statistic_id": "sensor.power", "has_mean": true},
	}

	assertIDs(t, pickStatisticIDs(list), []string{"sensor.energy", "sensor.power"})
}

func TestPickStatisticIDs_MoreThanTheCap_Truncated(t *testing.T) {
	var list []any
	for i := 0; i < maxEntities+5; i++ {
		list = append(list, map[string]any{"statistic_id": "sensor.n" + itoa(i), "has_mean": true})
	}

	if got := len(pickStatisticIDs(list)); got != maxEntities {
		t.Fatalf("len = %d, want %d", got, maxEntities)
	}
}

func TestPickStatisticIDs_NoCandidates_ReturnsNone(t *testing.T) {
	if got := pickStatisticIDs([]any{}); len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestRedactor_ReplacesEveryKnownID(t *testing.T) {
	var r redactor
	r.add("sensor.power", "sensor.energy")

	got := r.apply("sensor.power: array[3]\nsensor.energy: array[3]\n")

	want := "<entity>: array[3]\n<entity>: array[3]\n"
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

func state(entityID, value string, attrs map[string]any) any {
	return map[string]any{"entity_id": entityID, "state": value, "attributes": attrs}
}

func assertIDs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids = %v, want %v", got, want)
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
