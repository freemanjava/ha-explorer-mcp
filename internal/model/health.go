package model

import "time"

// Health is a deterministic health summary for one subject (an entity,
// device, integration, automation or the system as a whole), computed by
// internal/analysis — not mapped from a single raw HA payload the way
// Entity/DeviceRef/Integration/Area/Automation are (architecture doc §12).
// Fields not meaningful for a given subject are left zero.
//
// get_entity_statistics (P4-04) is its first user, joining P4-02's
// availability analysis and P4-03's cadence analysis into the one shape doc
// §12.1 describes.
type Health struct {
	// Source names the recorder API the numbers came from — "recorder_history"
	// today, the only source P4-01 wired up (doc §12.2's evidence model uses
	// the same field for the same purpose). Unlike other tools' Source field,
	// which names the HA subsystem that answered, this one names which
	// recorder endpoint computed the numbers, because history and the
	// statistics API answer at different resolutions and a diagnostic reader
	// needs to know which one it is trusting.
	Source     string
	ObservedAt time.Time

	SubjectID string
	Period    time.Duration
	From      time.Time
	To        time.Time

	// AvailabilityComputable is false when nothing was recorded in the
	// window (analysis.AvailabilityReport.Computable) — AvailabilityRatio is
	// then meaningless and must never be read as "0% available".
	AvailabilityComputable bool
	AvailabilityRatio      float64
	StateChanges           int
	UnavailablePeriods     int
	TotalUnavailable       time.Duration
	LongestUnavailable     time.Duration

	// CadenceComputable is false when fewer than two points were observed
	// (analysis.CadenceReport.Computable) — the interval fields are then
	// meaningless and must never be read as an instant cadence.
	CadenceComputable    bool
	MedianUpdateInterval time.Duration
	P95UpdateInterval    time.Duration

	// StaleJudgeable is false when cadence is not computable: without an
	// observed interval there is nothing to call the silence long against,
	// so Stale must never be read as "not stale" (analysis.CadenceReport).
	StaleJudgeable bool
	Stale          bool

	Provenance
}
