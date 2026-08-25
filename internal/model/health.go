package model

import "time"

// Health is a deterministic health summary for one subject (an entity,
// device, integration, automation or the system as a whole), computed by
// internal/analysis — not mapped from a single raw HA payload the way
// Entity/DeviceRef/Integration/Area/Automation are (architecture doc §12).
// Fields not meaningful for a given subject are left zero.
type Health struct {
	SubjectID string
	Period    time.Duration

	AvailabilityRatio    float64
	StateChanges         int
	UnavailablePeriods   int
	TotalUnavailable     time.Duration
	LongestUnavailable   time.Duration
	MedianUpdateInterval time.Duration
	P95UpdateInterval    time.Duration

	Provenance
}
