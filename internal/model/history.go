package model

import "time"

// HistoryPoint is one recorded state change for an entity, from
// history/history_during_period — get_entity_history's raw point, before any
// aggregation (P4-02/P4-03 build aggregate metrics on top of these).
// Attributes is only ever populated when the caller asked for the full,
// non-minimal shape: the minimal shape requests no_attributes alongside
// minimal_response and drops attributes entirely (P0-07).
type HistoryPoint struct {
	Timestamp  time.Time
	State      string
	Attributes map[string]any
}

// EntityHistory is get_entity_history's response: one entity's raw recorded
// states over an explicit, bounded window (Appendix A.2).
type EntityHistory struct {
	Source     string
	ObservedAt time.Time

	EntityID EntityID
	From     time.Time
	To       time.Time
	Minimal  bool

	Points []HistoryPoint
}
