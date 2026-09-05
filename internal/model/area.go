package model

import "time"

// Area is the normalized view of one Home Assistant area.
type Area struct {
	ID      AreaID
	Name    string
	FloorID string
	Icon    string
	Labels  []string

	Provenance
}

// Floor is the normalized view of one Home Assistant floor registry entry.
// Its element schema was unobserved by the 2026-08-23 probe — every
// installation sampled had an empty floor registry
// (docs/research/2026-08-23-ha-registry-apis.md finding 8) — so MapFloor
// assumes the field names HA's floor_registry component documents and marks
// a Floor Partial the moment one is missing, rather than trusting the
// assumption silently.
type Floor struct {
	ID   string
	Name string
	Icon string

	Provenance
}

// Label is the normalized view of one Home Assistant label registry entry.
// Same unverified-schema caveat as Floor.
type Label struct {
	ID    string
	Name  string
	Icon  string
	Color string

	Provenance
}

// AreaSummary is one list_areas row: the area itself plus its floor and
// label names, resolved from the floor/label registries best-effort. An
// unresolved FloorID or label id degrades only the name fields, never the
// area's own registry data (P3-06 DoD: "optional floor and label mapping").
type AreaSummary struct {
	Area

	FloorName  string
	LabelNames []string
}

// AreaList is list_areas' page: area summaries plus the cursor-pagination
// envelope every list_* tool shares (doc §9.1).
type AreaList struct {
	Source     string
	ObservedAt time.Time

	Items        []AreaSummary
	NextCursor   string
	Truncated    bool
	LimitClamped bool

	Provenance
}
