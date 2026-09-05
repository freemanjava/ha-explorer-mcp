package model

import "time"

// UnavailableEntityList is find_unavailable_entities' page: entities
// currently unavailable or unknown (P4-02's decision keeps that one bucket,
// as the rest of the catalog already does), filtered and paginated like any
// list_* tool (doc §9.1).
type UnavailableEntityList struct {
	Source     string
	ObservedAt time.Time

	Items        []Entity
	NextCursor   string
	Truncated    bool
	LimitClamped bool

	// PrivateExcluded counts PRIVATE entities withheld from Items because the
	// configured profile denies private data outright. The deny profile
	// excludes here rather than masking, as list_entities' state field does:
	// an installation-wide availability scan over a private domain is
	// exactly the bulk-correlation concern policy.Profile.CheckHistoryScope
	// already refuses for named history, applied live instead of
	// historically. The count exists so a short list under deny is never
	// mistaken for a healthy installation (CLAUDE.md rule 7).
	PrivateExcluded int

	Provenance
}

// StaleEntity is one find_stale_entities row: the entity plus the P4-03
// cadence evidence that judged it stale, so the response carries the reason
// for inclusion rather than a bare id.
type StaleEntity struct {
	Entity

	LastUpdate           time.Time
	SilentFor            time.Duration
	MedianUpdateInterval time.Duration
	P95UpdateInterval    time.Duration
	StaleThreshold       time.Duration
	// StalenessRatio is SilentFor / P95UpdateInterval — analysis.CadenceReport
	// reserves this field for ranking entities against each other here: the
	// worst offenders (most tail-intervals of silence) sort first.
	StalenessRatio float64
}

// StaleEntityList is find_stale_entities' page. Unlike a registry list_*
// tool, Truncated means "more candidate entities remain unexamined", not
// "more matching results exist": judging cadence costs one recorder read per
// entity (P4-03), so an installation-wide scan is bounded by the
// invocation's HA-request budget, not by response size alone.
type StaleEntityList struct {
	Source     string
	ObservedAt time.Time
	Period     time.Duration

	Items        []StaleEntity
	NextCursor   string
	Truncated    bool
	LimitClamped bool

	// Scanned counts candidate entities actually examined this call, so a
	// short Items list can be told apart from "the budget ran out after one".
	Scanned int
	// PrivateExcluded counts PRIVATE entities skipped without a recorder
	// read, for the same reason UnavailableEntityList.PrivateExcluded exists.
	PrivateExcluded int

	Provenance
}
