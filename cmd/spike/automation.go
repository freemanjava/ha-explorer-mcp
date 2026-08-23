package main

import "strings"

// automationTarget is the one automation every trace probe is aimed at. Both
// ids come from the owner's installation and are used only as request
// arguments — neither is ever written to the report.
type automationTarget struct {
	entityID  string // automation.<slug>
	numericID string // attributes.id — trace/list's item_id and the REST config path segment
}

// findAutomations returns the automation states in a get_states result.
//
// get_states answers with every entity in the installation; rendering that
// whole shape would bury the one schema this task is about. The report
// describes only these.
func findAutomations(decoded any) []any {
	list, ok := decoded.([]any)
	if !ok {
		return nil
	}
	var out []any
	for _, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if id, ok := m["entity_id"].(string); ok && strings.HasPrefix(id, "automation.") {
			out = append(out, e)
		}
	}
	return out
}

// pickTarget chooses which automation the trace probes ask about, preferring
// one that has run before.
//
// Traces exist only for runs: probing a never-triggered automation returns an
// empty trace/list, which reads exactly like "traces are unavailable" and is
// the wrong answer to F-3. The returned flag says which case this run got, so
// an empty result can be read correctly.
func pickTarget(states []any) (target automationTarget, triggered bool) {
	var first automationTarget
	for _, e := range states {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		candidate := automationTarget{}
		candidate.entityID, _ = m["entity_id"].(string)
		attrs, _ := m["attributes"].(map[string]any)
		if attrs != nil {
			candidate.numericID, _ = attrs["id"].(string)
			if lt, ok := attrs["last_triggered"].(string); ok && lt != "" {
				return candidate, true
			}
		}
		if first.entityID == "" {
			first = candidate
		}
	}
	return first, false
}

// runIDFor takes one run_id out of a trace/list answer so trace/get has
// something to ask for. Like the ids above it never reaches the report.
//
// The run_id must belong to itemID. trace/list without an item_id returns every
// automation's runs, and trace/get addressed with one automation's item_id and
// another's run_id answers not_found — which reads exactly like "traces are
// unreadable" and is the wrong answer to F-3. Observed on 2026-08-23 against
// 2026.8.3; the first admin run of this probe made precisely that mistake.
func runIDFor(decoded any, itemID string) string {
	list, ok := decoded.([]any)
	if !ok {
		return ""
	}
	for _, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := m["item_id"].(string); id != itemID {
			continue
		}
		if v, ok := m["run_id"].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
