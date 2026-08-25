package policy

import (
	"fmt"
	"time"
)

// Source is a recorder-backed data source whose cost can be estimated before
// the query is issued.
type Source int

const (
	// SourceHistory is history/history_during_period with minimal_response and
	// no_attributes — the only variant P0-07 found worth measuring.
	SourceHistory Source = iota
	// SourceStatistics is recorder/statistics_during_period with hour buckets.
	SourceStatistics
)

func (s Source) String() string {
	switch s {
	case SourceStatistics:
		return "statistics"
	case SourceHistory:
		return "history"
	default:
		return "unknown"
	}
}

// Per-entity-day means over the 200-entity set measured 2026-08-24
// (docs/research/2026-08-24-ha-multi-entity-query-cost.md). They exist so the
// budget can refuse *before* issuing: MaxBytes enforced on a received response
// is enforced after the Pi has already paid for the recorder read, the
// serialization and the transfer.
//
// The statistics byte figure is the batched one, which F-17 records as ~30%
// larger than the same ids fetched singly for reasons not yet established —
// so the estimate errs conservative, in the direction of refusing early.
//
// These are means over a skewed set, not bounds: a handful of very chatty
// entities can exceed them, which is why ChargeBytes on the received response
// stays as the backstop.
const (
	historyBytesPerEntityDay  = 5_600
	historyPointsPerEntityDay = 151
	statsBytesPerEntityDay    = 670
	statsPointsPerEntityDay   = 6
	hoursPerDay               = 24
)

// EstimateBytes returns the expected response size for a query over entities
// across window. See the per-entity-day constants for what it is and is not.
func EstimateBytes(src Source, entities int, window time.Duration) int64 {
	switch src {
	case SourceHistory:
		return int64(entityDays(entities, window) * historyBytesPerEntityDay)
	case SourceStatistics:
		return int64(entityDays(entities, window) * statsBytesPerEntityDay)
	default:
		return 0
	}
}

// EstimatePoints returns the expected number of recorded points for a query
// over entities across window.
func EstimatePoints(src Source, entities int, window time.Duration) int {
	switch src {
	case SourceHistory:
		return int(entityDays(entities, window) * historyPointsPerEntityDay)
	case SourceStatistics:
		return int(entityDays(entities, window) * statsPointsPerEntityDay)
	default:
		return 0
	}
}

func entityDays(entities int, window time.Duration) float64 {
	return float64(entities) * window.Hours() / hoursPerDay
}

// Preflight refuses a query that the remaining budget cannot afford, before it
// is issued. It charges nothing: what the query actually costs is charged when
// the answer arrives.
//
// It is checked against what is left, not against the class ceiling, so a
// composite diagnostic's second query cannot spend the first one's budget.
func (b *QueryBudget) Preflight(src Source, entities int, window time.Duration) error {
	if src != SourceHistory && src != SourceStatistics {
		return fmt.Errorf("policy: cannot estimate the cost of source %d", int(src))
	}
	if entities <= 0 {
		return fmt.Errorf("policy: pre-flight over %d entities is not a query", entities)
	}
	if window <= 0 {
		return fmt.Errorf("policy: pre-flight over a %v window is not a query", window)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Entities first — it is the one dimension known exactly rather than
	// estimated. Bytes next: the measurement found the byte cap binds before
	// the point cap and before the deadline in every case observed.
	if err := b.estimateLocked(DimensionEntities, int64(entities), int64(b.used.Entities), int64(b.limits.MaxEntities)); err != nil {
		return err
	}
	if err := b.estimateLocked(DimensionBytes, EstimateBytes(src, entities, window), b.used.Bytes, b.limits.MaxBytes); err != nil {
		return err
	}
	return b.estimateLocked(DimensionHistoryPoints, int64(EstimatePoints(src, entities, window)), int64(b.used.HistoryPoints), int64(b.limits.MaxHistoryPoints))
}

func (b *QueryBudget) estimateLocked(dim Dimension, estimate, used, limit int64) error {
	if used+estimate > limit {
		return &BudgetError{Dimension: dim, Limit: limit, Requested: estimate, Used: b.used, Estimated: true}
	}
	return nil
}
