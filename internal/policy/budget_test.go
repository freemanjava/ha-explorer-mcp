package policy

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLimitsFor_NormalRead_MatchesMeasuredValues(t *testing.T) {
	l := LimitsFor(ClassNormalRead)
	if l.MaxHARequests != 20 || l.MaxHistoryPoints != 13_000 || l.MaxEntities != 200 {
		t.Fatalf("normal read limits: %+v", l)
	}
	if l.MaxBytes != 512*1024 || l.Deadline != 10*time.Second {
		t.Fatalf("normal read limits: %+v", l)
	}
}

func TestLimitsFor_Composite_MatchesMeasuredValues(t *testing.T) {
	l := LimitsFor(ClassComposite)
	if l.MaxHARequests != 50 || l.MaxHistoryPoints != 26_000 || l.MaxEntities != 200 {
		t.Fatalf("composite limits: %+v", l)
	}
	if l.MaxBytes != 1024*1024 || l.Deadline != 30*time.Second {
		t.Fatalf("composite limits: %+v", l)
	}
}

// The composite class must not inherit doc §10's illustrative 500 entities:
// nothing above 200 ids was measured (research 2026-08-24).
func TestLimitsFor_Composite_EntityCapNotWidenedBeyondMeasurement(t *testing.T) {
	if got := LimitsFor(ClassComposite).MaxEntities; got != LimitsFor(ClassNormalRead).MaxEntities {
		t.Fatalf("composite MaxEntities = %d, want the measured 200 both classes", got)
	}
}

func TestLimitsFor_UnknownClass_FallsBackToNormalRead(t *testing.T) {
	if LimitsFor(Class(99)) != LimitsFor(ClassNormalRead) {
		t.Fatal("unknown class must fall back to the tightest class, not to zero limits")
	}
}

// Each dimension must trip independently: exhausting one says nothing about
// the others.
func TestQueryBudget_EachDimension_TripsIndependently(t *testing.T) {
	tests := []struct {
		name  string
		limit Limits
		spend func(*QueryBudget) error
		dim   Dimension
	}{
		{
			name:  "ha requests",
			limit: Limits{MaxHARequests: 2, MaxHistoryPoints: 1e6, MaxEntities: 1e6, MaxBytes: 1e9},
			spend: func(b *QueryBudget) error { return b.ChargeHARequests(3) },
			dim:   DimensionHARequests,
		},
		{
			name:  "history points",
			limit: Limits{MaxHARequests: 1e6, MaxHistoryPoints: 10, MaxEntities: 1e6, MaxBytes: 1e9},
			spend: func(b *QueryBudget) error { return b.ChargeHistoryPoints(11) },
			dim:   DimensionHistoryPoints,
		},
		{
			name:  "entities",
			limit: Limits{MaxHARequests: 1e6, MaxHistoryPoints: 1e6, MaxEntities: 5, MaxBytes: 1e9},
			spend: func(b *QueryBudget) error { return b.ChargeEntities(6) },
			dim:   DimensionEntities,
		},
		{
			name:  "bytes",
			limit: Limits{MaxHARequests: 1e6, MaxHistoryPoints: 1e6, MaxEntities: 1e6, MaxBytes: 100},
			spend: func(b *QueryBudget) error { return b.ChargeBytes(101) },
			dim:   DimensionBytes,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := NewQueryBudgetWith(tc.limit)
			err := tc.spend(b)
			if !errors.Is(err, ErrBudgetExceeded) {
				t.Fatalf("err = %v, want ErrBudgetExceeded", err)
			}
			var be *BudgetError
			if !errors.As(err, &be) {
				t.Fatalf("err = %v, want a *BudgetError", err)
			}
			if be.Dimension != tc.dim {
				t.Fatalf("Dimension = %q, want %q", be.Dimension, tc.dim)
			}
			if !strings.Contains(err.Error(), string(tc.dim)) {
				t.Fatalf("error text %q does not name the limit that was hit", err.Error())
			}
		})
	}
}

// The error must carry what was retrieved so far, and the refused charge must
// not be applied — a budget that counts what it refused cannot be reported on.
func TestQueryBudget_Exceeded_ReportsUsageSoFarAndDoesNotApplyCharge(t *testing.T) {
	b := NewQueryBudgetWith(Limits{MaxHARequests: 10, MaxHistoryPoints: 10, MaxEntities: 10, MaxBytes: 1000})

	if err := b.ChargeHARequests(2); err != nil {
		t.Fatalf("first charge: %v", err)
	}
	if err := b.ChargeBytes(400); err != nil {
		t.Fatalf("byte charge: %v", err)
	}

	err := b.ChargeBytes(700)
	var be *BudgetError
	if !errors.As(err, &be) {
		t.Fatalf("err = %v, want a *BudgetError", err)
	}
	if be.Used.Bytes != 400 || be.Used.HARequests != 2 {
		t.Fatalf("Used = %+v, want the 400 bytes and 2 requests already retrieved", be.Used)
	}
	if be.Limit != 1000 || be.Requested != 700 {
		t.Fatalf("Limit/Requested = %d/%d, want 1000/700", be.Limit, be.Requested)
	}
	if got := b.Usage().Bytes; got != 400 {
		t.Fatalf("Usage().Bytes = %d after a refused charge, want the pre-charge 400", got)
	}
}

// A tool that finishes inside budget must be able to report what it cost.
func TestQueryBudget_WithinBudget_ReportsConsumption(t *testing.T) {
	b := NewQueryBudget(ClassNormalRead)
	for _, err := range []error{
		b.ChargeHARequests(3),
		b.ChargeEntities(12),
		b.ChargeHistoryPoints(1_800),
		b.ChargeBytes(65_536),
	} {
		if err != nil {
			t.Fatalf("charge inside budget: %v", err)
		}
	}

	want := Usage{HARequests: 3, Entities: 12, HistoryPoints: 1_800, Bytes: 65_536}
	if got := b.Usage(); got != want {
		t.Fatalf("Usage() = %+v, want %+v", got, want)
	}
	if got := b.Limits(); got != LimitsFor(ClassNormalRead) {
		t.Fatalf("Limits() = %+v, want the class defaults", got)
	}
}

func TestQueryBudget_ExactLimit_Allowed(t *testing.T) {
	b := NewQueryBudgetWith(Limits{MaxHARequests: 2, MaxHistoryPoints: 2, MaxEntities: 2, MaxBytes: 2})
	if err := b.ChargeBytes(2); err != nil {
		t.Fatalf("charging exactly the limit must be allowed, got %v", err)
	}
	if err := b.ChargeBytes(1); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("one byte past the limit: err = %v, want ErrBudgetExceeded", err)
	}
}

func TestQueryBudget_NegativeCharge_Rejected(t *testing.T) {
	b := NewQueryBudget(ClassNormalRead)
	if err := b.ChargeBytes(-1); err == nil {
		t.Fatal("a negative charge must be refused, not credited back to the budget")
	}
	if got := b.Usage().Bytes; got != 0 {
		t.Fatalf("Usage().Bytes = %d after a refused negative charge, want 0", got)
	}
}

func TestQueryBudget_ConcurrentCharges_TotalNeverExceedsLimit(t *testing.T) {
	const limit = 100
	b := NewQueryBudgetWith(Limits{MaxHARequests: 1e6, MaxHistoryPoints: 1e6, MaxEntities: 1e6, MaxBytes: limit})

	var wg sync.WaitGroup
	var granted int64
	var mu sync.Mutex
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := b.ChargeBytes(10); err == nil {
				mu.Lock()
				granted += 10
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if granted != limit {
		t.Fatalf("granted %d bytes, want exactly the %d-byte limit", granted, limit)
	}
	if got := b.Usage().Bytes; got != limit {
		t.Fatalf("Usage().Bytes = %d, want %d", got, limit)
	}
}

func TestWithBudget_Deadline_CancelsInFlightUpstreamCall(t *testing.T) {
	b := NewQueryBudgetWith(Limits{MaxHARequests: 10, MaxHistoryPoints: 10, MaxEntities: 10, MaxBytes: 10, Deadline: 20 * time.Millisecond})

	ctx, cancel := WithBudget(context.Background(), b)
	defer cancel()

	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("WithBudget must put the budget's deadline on the context")
	}

	// Stands in for an upstream call that is already in flight: it returns
	// only when its context is done.
	done := make(chan error, 1)
	go func() {
		<-ctx.Done()
		done <- ctx.Err()
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("in-flight call ended with %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("deadline did not cancel the in-flight call")
	}
}

func TestBudgetFrom_ContextCarriesBudget_SameInstanceReturned(t *testing.T) {
	b := NewQueryBudget(ClassComposite)
	ctx, cancel := WithBudget(context.Background(), b)
	defer cancel()

	got, ok := BudgetFrom(ctx)
	if !ok || got != b {
		t.Fatalf("BudgetFrom() = %v, %v; want the budget that was attached", got, ok)
	}
}

// Fail closed: a caller that forgot to attach a budget must not be handed an
// unlimited one.
func TestBudgetFrom_NoBudget_ReportsAbsence(t *testing.T) {
	if got, ok := BudgetFrom(context.Background()); ok || got != nil {
		t.Fatalf("BudgetFrom(bare ctx) = %v, %v; want nil, false", got, ok)
	}
}

func TestWithBudget_ZeroDeadline_FallsBackToClassDefault(t *testing.T) {
	b := NewQueryBudgetWith(Limits{MaxBytes: 1})
	ctx, cancel := WithBudget(context.Background(), b)
	defer cancel()

	dl, ok := ctx.Deadline()
	if !ok {
		t.Fatal("a budget with no configured deadline must still bound the call")
	}
	if d := time.Until(dl); d <= 0 || d > LimitsFor(ClassNormalRead).Deadline+time.Second {
		t.Fatalf("deadline in %v, want the normal-read default", d)
	}
}
