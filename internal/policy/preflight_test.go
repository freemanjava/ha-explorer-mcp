package policy

import (
	"errors"
	"testing"
	"time"
)

const day = 24 * time.Hour

func TestEstimateBytes_History_MatchesMeasuredPerEntityDayMean(t *testing.T) {
	// 200 entities over 7d measured 7 633 159 B; the per-entity-day mean the
	// estimate is built from puts the same query near that figure.
	got := EstimateBytes(SourceHistory, 200, 7*day)
	if got < 7_000_000 || got > 8_500_000 {
		t.Fatalf("EstimateBytes(history, 200, 7d) = %d, want within ~10%% of the measured 7 633 159", got)
	}
}

func TestEstimateBytes_Statistics_MatchesMeasuredPerEntityDayMean(t *testing.T) {
	// 200 ids over 7d measured 939 984 B (the batched figure, F-17-conservative).
	got := EstimateBytes(SourceStatistics, 200, 7*day)
	if got < 850_000 || got > 1_050_000 {
		t.Fatalf("EstimateBytes(statistics, 200, 7d) = %d, want within ~10%% of the measured 939 984", got)
	}
}

func TestEstimateBytes_Statistics_CheaperThanHistoryAtFleetWidth(t *testing.T) {
	h := EstimateBytes(SourceHistory, 200, 7*day)
	s := EstimateBytes(SourceStatistics, 200, 7*day)
	if s*4 > h {
		t.Fatalf("statistics estimate %d vs history %d: measurement says statistics is ~8x cheaper", s, h)
	}
}

func TestEstimatePoints_ScalesWithWindowAndEntities(t *testing.T) {
	one := EstimatePoints(SourceHistory, 1, day)
	if one <= 0 {
		t.Fatalf("EstimatePoints(history, 1, 24h) = %d, want a positive estimate", one)
	}
	if got := EstimatePoints(SourceHistory, 10, 2*day); got != 20*one {
		t.Fatalf("EstimatePoints scales to %d, want %d — the estimate is linear in entity-days", got, 20*one)
	}
}

// The whole point of the pre-flight estimate: refuse before the Pi pays for
// the query (research 2026-08-24, "the byte cap binds first, everywhere").
func TestQueryBudget_Preflight_OverBudgetQuery_RefusedWithoutCharging(t *testing.T) {
	b := NewQueryBudget(ClassNormalRead)

	err := b.Preflight(SourceHistory, 200, 7*day)
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("err = %v, want ErrBudgetExceeded for 200 entities over 7d", err)
	}
	if got := b.Usage(); got != (Usage{}) {
		t.Fatalf("Usage() = %+v after a refused pre-flight, want nothing charged", got)
	}
}

func TestQueryBudget_Preflight_AffordableQuery_Allowed(t *testing.T) {
	b := NewQueryBudget(ClassNormalRead)
	if err := b.Preflight(SourceHistory, 10, day); err != nil {
		t.Fatalf("10 entities over 24h is well inside 512 KB, got %v", err)
	}
	if err := b.Preflight(SourceStatistics, 200, day); err != nil {
		t.Fatalf("statistics for 200 ids over 24h measured 131 530 B, got %v", err)
	}
}

// Statistics buys ~8.7x more entity-days than history for the same bytes.
func TestQueryBudget_Preflight_BytesDimension_BindsOnStatistics(t *testing.T) {
	b := NewQueryBudget(ClassNormalRead)
	if err := b.Preflight(SourceStatistics, 100, 7*day); err != nil {
		t.Fatalf("700 entity-days of statistics is inside 512 KB, got %v", err)
	}

	err := b.Preflight(SourceStatistics, 200, 5*day)
	var be *BudgetError
	if !errors.As(err, &be) {
		t.Fatalf("err = %v, want a *BudgetError for 1 000 entity-days of statistics", err)
	}
	if be.Dimension != DimensionBytes {
		t.Fatalf("Dimension = %q, want %q — statistics hits the byte cap before the point cap", be.Dimension, DimensionBytes)
	}
}

func TestQueryBudget_Preflight_TooManyEntities_RefusedOnEntityDimension(t *testing.T) {
	b := NewQueryBudget(ClassNormalRead)
	err := b.Preflight(SourceHistory, 201, time.Minute)

	var be *BudgetError
	if !errors.As(err, &be) {
		t.Fatalf("err = %v, want a *BudgetError", err)
	}
	if be.Dimension != DimensionEntities {
		t.Fatalf("Dimension = %q, want %q", be.Dimension, DimensionEntities)
	}
}

// The estimate is checked against what is left, not against the class ceiling:
// a composite diagnostic's second query cannot spend the first one's budget.
func TestQueryBudget_Preflight_AccountsForAlreadyConsumedBudget(t *testing.T) {
	b := NewQueryBudget(ClassComposite)
	if err := b.Preflight(SourceHistory, 20, day); err != nil {
		t.Fatalf("first query must fit: %v", err)
	}
	if err := b.ChargeBytes(LimitsFor(ClassComposite).MaxBytes - 1_000); err != nil {
		t.Fatalf("charging the first query's result: %v", err)
	}

	if err := b.Preflight(SourceHistory, 20, day); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("err = %v, want ErrBudgetExceeded once 1 000 bytes remain", err)
	}
}

func TestQueryBudget_Preflight_UnknownSource_Refused(t *testing.T) {
	b := NewQueryBudget(ClassNormalRead)
	if err := b.Preflight(Source(99), 1, time.Minute); err == nil {
		t.Fatal("an unestimatable source must fail closed, not be waved through")
	}
}

func TestQueryBudget_Preflight_NonPositiveWindowOrEntities_Refused(t *testing.T) {
	b := NewQueryBudget(ClassNormalRead)
	if err := b.Preflight(SourceHistory, 0, day); err == nil {
		t.Fatal("zero entities is a malformed query, not a free one")
	}
	if err := b.Preflight(SourceHistory, 5, -time.Hour); err == nil {
		t.Fatal("a negative window is a malformed query, not a free one")
	}
}
