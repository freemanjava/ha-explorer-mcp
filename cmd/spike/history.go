package main

import (
	"strconv"
	"strings"
)

// maxEntities is the top rung of the measurement ladder and the doc §10 cap on
// entities per read tool: no probe below asks about more ids than a real tool
// would ever be allowed to.
const maxEntities = 200

// pickHistoryTargets chooses the entity ids the history probes ask about, in a
// stable order, at most maxEntities of them.
//
// Preference order: numeric `sensor.*` entities first (the kind a fleet-wide
// detector actually queries, and the kind the recorder keeps rows for), then
// anything else, so the ladder still reaches its upper rungs on an
// installation with few sensors. Ids leave this function only as request
// arguments — never into the report.
func pickHistoryTargets(states []any) []string {
	var numeric, other []string
	for _, e := range states {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["entity_id"].(string)
		if id == "" {
			continue
		}
		if strings.HasPrefix(id, "sensor.") && isNumeric(m["state"]) {
			numeric = append(numeric, id)
			continue
		}
		other = append(other, id)
	}

	picked := append(numeric, other...)
	if len(picked) > maxEntities {
		picked = picked[:maxEntities]
	}
	return picked
}

func isNumeric(state any) bool {
	s, ok := state.(string)
	if !ok {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// pickStatisticIDs takes up to maxEntities statistic ids out of a
// recorder/list_statistic_ids answer.
//
// Entries with neither has_mean nor has_sum are skipped: they carry no
// compiled statistics, so including them would understate the cost per id and
// make an empty answer indistinguishable from an unsupported API.
func pickStatisticIDs(list []any) []string {
	var ids []string
	for _, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["statistic_id"].(string)
		if id == "" {
			continue
		}
		mean, _ := m["has_mean"].(bool)
		sum, _ := m["has_sum"].(bool)
		if !mean && !sum {
			continue
		}
		ids = append(ids, id)
		if len(ids) == maxEntities {
			break
		}
	}
	return ids
}

// redactor strips ids belonging to the owner's installation out of the report.
//
// It is not only request lines that carry them: HA keys both
// history/history_during_period and statistics_during_period results *by
// entity id*, so the rendered shape of those answers contains ids as object
// keys. Observed on 2026-08-23 against 2026.8.3 — the first run of this probe
// printed one.
type redactor struct{ ids []string }

func (r *redactor) add(ids ...string) {
	for _, id := range ids {
		if id != "" {
			r.ids = append(r.ids, id)
		}
	}
}

func (r *redactor) apply(s string) string {
	for _, id := range r.ids {
		s = strings.ReplaceAll(s, id, "<entity>")
	}
	return s
}
