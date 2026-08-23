package main

import "testing"

func TestFindAutomations_SelectsOnlyTheAutomationDomain(t *testing.T) {
	states := mustUnmarshal(t, `[
	  {"entity_id": "light.kitchen", "state": "on", "attributes": {}},
	  {"entity_id": "automation.morning", "state": "on", "attributes": {"id": "1699"}},
	  {"entity_id": "automation_helper.not_one", "state": "on", "attributes": {}},
	  {"entity_id": "automation.evening", "state": "off", "attributes": {"id": "1700"}}
	]`)

	got := findAutomations(states)

	if len(got) != 2 {
		t.Fatalf("expected 2 automation states, got %d", len(got))
	}
}

func TestFindAutomations_NotAList_ReturnsNone(t *testing.T) {
	if got := findAutomations(mustUnmarshal(t, `{"error": "nope"}`)); got != nil {
		t.Fatalf("expected nil for a non-list result, got %v", got)
	}
}

// A never-triggered automation has no traces, so probing one would produce an
// empty trace/list and the false conclusion that traces are unavailable.
func TestPickTarget_PrefersATriggeredAutomation(t *testing.T) {
	states := findAutomations(mustUnmarshal(t, `[
	  {"entity_id": "automation.never_run", "attributes": {"id": "1", "last_triggered": null}},
	  {"entity_id": "automation.has_run", "attributes": {"id": "2", "last_triggered": "2026-08-23T10:00:00+00:00"}}
	]`))

	target, triggered := pickTarget(states)

	if !triggered {
		t.Error("expected the chosen target to be marked as previously triggered")
	}
	if target.entityID != "automation.has_run" {
		t.Errorf("entityID = %q, want automation.has_run", target.entityID)
	}
	if target.numericID != "2" {
		t.Errorf("numericID = %q, want 2", target.numericID)
	}
}

func TestPickTarget_NoneTriggered_FallsBackToTheFirst(t *testing.T) {
	states := findAutomations(mustUnmarshal(t, `[
	  {"entity_id": "automation.first", "attributes": {"id": "1"}},
	  {"entity_id": "automation.second", "attributes": {"id": "2"}}
	]`))

	target, triggered := pickTarget(states)

	if triggered {
		t.Error("expected triggered=false when no automation carries last_triggered")
	}
	if target.entityID != "automation.first" {
		t.Errorf("entityID = %q, want automation.first", target.entityID)
	}
}

// attributes.id is what trace/list and the REST config route are keyed by. A
// YAML automation without one must still yield an entity_id, so the probes
// that need only that still run and the rest are reported as skipped.
func TestPickTarget_AutomationWithoutNumericID_StillYieldsTheEntity(t *testing.T) {
	states := findAutomations(mustUnmarshal(t, `[
	  {"entity_id": "automation.yaml_defined", "attributes": {"last_triggered": "2026-08-23T10:00:00+00:00"}}
	]`))

	target, _ := pickTarget(states)

	if target.entityID != "automation.yaml_defined" {
		t.Errorf("entityID = %q, want automation.yaml_defined", target.entityID)
	}
	if target.numericID != "" {
		t.Errorf("numericID = %q, want empty", target.numericID)
	}
}

func TestPickTarget_NoAutomations_ReturnsEmpty(t *testing.T) {
	if target, _ := pickTarget(nil); target.entityID != "" {
		t.Errorf("expected an empty target, got %q", target.entityID)
	}
}

func TestRunIDFor_TakesTheFirstRunOfThatAutomation(t *testing.T) {
	traces := mustUnmarshal(t, `[
	  {"item_id": "1699", "run_id": "01JABCDE", "timestamp": {"start": "2026-08-23T10:00:00+00:00"}},
	  {"item_id": "1699", "run_id": "01JZZZZZ"}
	]`)

	if got := runIDFor(traces, "1699"); got != "01JABCDE" {
		t.Errorf("runIDFor = %q, want 01JABCDE", got)
	}
}

// The regression the first admin run hit: an unfiltered trace/list carries
// every automation's runs, and pairing one automation's item_id with another's
// run_id makes trace/get answer not_found for a reason that has nothing to do
// with whether traces are readable.
func TestRunIDFor_IgnoresAnotherAutomationsRun(t *testing.T) {
	traces := mustUnmarshal(t, `[
	  {"item_id": "1700", "run_id": "01JOTHER"},
	  {"item_id": "1699", "run_id": "01JMINE"}
	]`)

	if got := runIDFor(traces, "1699"); got != "01JMINE" {
		t.Errorf("runIDFor = %q, want 01JMINE", got)
	}
}

func TestRunIDFor_NoRunForThatAutomation_ReturnsEmpty(t *testing.T) {
	traces := mustUnmarshal(t, `[{"item_id": "1700", "run_id": "01JOTHER"}]`)

	if got := runIDFor(traces, "1699"); got != "" {
		t.Errorf("runIDFor = %q, want empty", got)
	}
}

func TestRunIDFor_EmptyList_ReturnsEmpty(t *testing.T) {
	if got := runIDFor(mustUnmarshal(t, `[]`), "1699"); got != "" {
		t.Errorf("runIDFor = %q, want empty", got)
	}
}
