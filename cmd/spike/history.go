package main

import (
	"net/url"
	"strconv"
	"strings"
	"time"
)

// historyTarget is the one entity every history and statistics probe is aimed
// at. The id comes from the owner's installation and is used only as a request
// argument — it is never written to the report.
type historyTarget struct {
	entityID string
	// hasStateClass says whether the recorder is expected to keep long-term
	// statistics for this entity at all. Without it an empty
	// statistics_during_period answer reads exactly like "the API is
	// unavailable", which is the wrong answer to F-5.
	hasStateClass bool
}

// pickHistoryTarget chooses the entity the history and statistics probes ask
// about.
//
// Preference order: a numeric sensor carrying state_class (the only kind the
// recorder compiles into long-term statistics), then any numeric sensor (still
// a fair history-cost sample), then anything at all so the REST probes still
// run on an installation with no sensors.
func pickHistoryTarget(states []any) historyTarget {
	var numeric, any_ historyTarget
	for _, e := range states {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["entity_id"].(string)
		if id == "" {
			continue
		}
		if any_.entityID == "" {
			any_ = historyTarget{entityID: id}
		}
		if !strings.HasPrefix(id, "sensor.") || !isNumeric(m["state"]) {
			continue
		}
		attrs, _ := m["attributes"].(map[string]any)
		if sc, ok := attrs["state_class"].(string); ok && sc != "" {
			return historyTarget{entityID: id, hasStateClass: true}
		}
		if numeric.entityID == "" {
			numeric = historyTarget{entityID: id}
		}
	}
	if numeric.entityID != "" {
		return numeric
	}
	return any_
}

func isNumeric(state any) bool {
	s, ok := state.(string)
	if !ok {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// historyOpts are the two response-shaping parameters F-5 asks about. Both are
// presence flags in HA's REST API, not key/value pairs.
type historyOpts struct {
	minimalResponse bool
	noAttributes    bool
}

func (o historyOpts) label() string {
	switch {
	case o.minimalResponse && o.noAttributes:
		return "minimal_response + no_attributes"
	case o.minimalResponse:
		return "minimal_response"
	case o.noAttributes:
		return "no_attributes"
	default:
		return "no parameters (full states)"
	}
}

// historyPath builds a bounded single-entity /api/history/period request.
//
// The window is always explicit: without end_time HA answers one hour, and
// without filter_entity_id it answers for every recorded entity — the query
// this project must never issue on a Pi.
func historyPath(entityID string, start, end time.Time, opts historyOpts) string {
	q := url.Values{}
	q.Set("filter_entity_id", entityID)
	q.Set("end_time", end.UTC().Format(time.RFC3339))

	path := "/api/history/period/" + start.UTC().Format(time.RFC3339) + "?" + q.Encode()
	if opts.minimalResponse {
		path += "&minimal_response"
	}
	if opts.noAttributes {
		path += "&no_attributes"
	}
	return path
}

// pickStatisticID takes one statistic id out of a recorder/list_statistic_ids
// answer, preferring the entity the history probes used so both halves of the
// report describe the same data.
//
// The fallback skips entries with neither has_mean nor has_sum: those carry no
// compiled statistics, and querying one yields an empty result that cannot be
// told apart from an unsupported API.
func pickStatisticID(list []any, preferred string) string {
	fallback := ""
	for _, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["statistic_id"].(string)
		if id == "" {
			continue
		}
		if id == preferred {
			return id
		}
		mean, _ := m["has_mean"].(bool)
		sum, _ := m["has_sum"].(bool)
		if fallback == "" && (mean || sum) {
			fallback = id
		}
	}
	return fallback
}

// redactor strips ids belonging to the owner's installation out of the report.
//
// It is not only the request lines that carry them: HA keys both
// history/history_during_period and statistics_during_period results *by
// entity id*, so the rendered shape of those answers contains the id as an
// object key. Observed on 2026-08-23 against 2026.8.3 — the first run of this
// probe printed one.
type redactor struct{ ids []string }

func (r *redactor) add(id string) {
	if id == "" {
		return
	}
	r.ids = append(r.ids, id)
}

func (r *redactor) apply(s string) string {
	for _, id := range r.ids {
		s = strings.ReplaceAll(s, id, "<target entity>")
	}
	return s
}
