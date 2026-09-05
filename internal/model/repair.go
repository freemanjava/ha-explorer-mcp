package model

import "time"

// Repair is the normalized view of one Home Assistant Repairs/issue entry,
// read through repairs/list_issues (P3-06) — reachable at any principal,
// admin or not (docs/research/2026-09-05-ha-repairs-api.md), unlike the
// automation/trace surface. TranslationPlaceholders is free-form per-issue
// metadata HA fills in; per CLAUDE.md rule 6 it is carried as opaque data,
// never parsed for content or branched on.
type Repair struct {
	IssueID  string
	Domain   string
	Severity string

	Created          time.Time
	IsFixable        bool
	Ignored          bool
	DismissedVersion string

	// BreaksInHAVersion, IssueDomain and LearnMoreURL are observed null on
	// every sampled installation but present unconditionally as fields
	// (2026-09-05 research doc) — optional, not assumed absent.
	BreaksInHAVersion string
	IssueDomain       string
	LearnMoreURL      string

	TranslationKey          string
	TranslationPlaceholders map[string]any

	Provenance
}

// RepairList is list_repairs' page: repair entries plus the cursor-pagination
// envelope every list_* tool shares (doc §9.1).
type RepairList struct {
	Source     string
	ObservedAt time.Time

	Items        []Repair
	NextCursor   string
	Truncated    bool
	LimitClamped bool

	Provenance
}
