package model

import "time"

// Evidence separates an observed fact from its inference, per ADR-010: fact,
// inference and recommendation are never one prose blob. Confidence names how
// strongly Evidence supports Inference, not how certain Observation is —
// Observation is a deterministic measurement.
type Evidence struct {
	Observation string
	Source      string
	PeriodFrom  time.Time
	PeriodTo    time.Time

	Facts      map[string]int
	Confidence string
	Inference  string
}
