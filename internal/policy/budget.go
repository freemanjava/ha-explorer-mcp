package policy

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Class is the cost class of an invocation. Doc §10 defines two: a normal
// read tool and a composite diagnostic that fans out over several sources.
type Class int

const (
	// ClassNormalRead is a single-source read: one list, one history window,
	// one entity's statistics.
	ClassNormalRead Class = iota
	// ClassComposite is a diagnostic that correlates several sources within
	// one invocation and is allowed roughly double the normal read's spend.
	ClassComposite
)

func (c Class) String() string {
	switch c {
	case ClassComposite:
		return "composite_diagnostic"
	default:
		return "normal_read"
	}
}

// Limits are the ceilings one invocation may spend, in the shape of doc §10's
// QueryBudget. The values below come from the 2026-08-24 measurement against
// a live installation (docs/research/2026-08-24-ha-multi-entity-query-cost.md,
// resolving F-14), not from doc §10's illustrative numbers, which that
// document itself marks as guesses (§26).
type Limits struct {
	MaxHARequests    int
	MaxHistoryPoints int
	MaxEntities      int
	MaxBytes         int64
	Deadline         time.Duration
}

// Measured budget defaults. Each constant names the observation it came from;
// a limit nobody measured is a guess wearing a constant's name (phase 02
// design notes).
const (
	// Doc §10's values, confirmed by measurement as the constraint that binds
	// first: every call that stayed within 512 KB completed in <= 339 ms cold,
	// so the byte cap is reached before the deadline and before the entity cap
	// in every case measured.
	normalMaxBytes    int64 = 512 * 1024
	compositeMaxBytes int64 = 1024 * 1024

	// MaxBytes / 37 B per history point, the measured mean for
	// minimal_response + no_attributes. Doc §10's 50 000 is unreachable
	// within 1 MB and would therefore never fire.
	normalMaxHistoryPoints    = 13_000
	compositeMaxHistoryPoints = 26_000

	// The widest count measured: both commands answered 200 ids without
	// failure or truncation, and nothing above 200 was measured, so doc §10's
	// composite 500 is deliberately not carried forward for either class.
	measuredMaxEntities = 200

	// Doc §10's request counts. Unmeasured — the 2026-08-24 run measured the
	// cost of single batched calls, not how many an invocation needs — so
	// they stand as doc defaults and are configurable.
	normalMaxHARequests    = 20
	compositeMaxHARequests = 50

	// Doc §10's deadlines, which stand with margin: no call within its byte
	// cap exceeded 339 ms cold, and the widest measured call of all (7.63 MB
	// of history) took 5.3 s.
	normalDeadline    = 10 * time.Second
	compositeDeadline = 30 * time.Second
)

// LimitsFor returns the measured defaults for a class. An unrecognized class
// falls back to the tightest one: failing closed is the rule, and a zero
// Limits would be an unlimited budget in disguise.
func LimitsFor(c Class) Limits {
	if c == ClassComposite {
		return Limits{
			MaxHARequests:    compositeMaxHARequests,
			MaxHistoryPoints: compositeMaxHistoryPoints,
			MaxEntities:      measuredMaxEntities,
			MaxBytes:         compositeMaxBytes,
			Deadline:         compositeDeadline,
		}
	}
	return Limits{
		MaxHARequests:    normalMaxHARequests,
		MaxHistoryPoints: normalMaxHistoryPoints,
		MaxEntities:      measuredMaxEntities,
		MaxBytes:         normalMaxBytes,
		Deadline:         normalDeadline,
	}
}

// Dimension names one budget ceiling. It appears in the error text and in the
// audit record, so the values are stable snake_case identifiers.
type Dimension string

const (
	DimensionHARequests    Dimension = "ha_requests"
	DimensionHistoryPoints Dimension = "history_points"
	DimensionEntities      Dimension = "entities"
	DimensionBytes         Dimension = "bytes"
)

// Usage is what an invocation has actually spent. A tool that finishes inside
// its budget reports this alongside its result.
type Usage struct {
	HARequests    int
	HistoryPoints int
	Entities      int
	Bytes         int64
}

// BudgetError says which limit was hit, by how much, and what had already
// been retrieved when it was — everything the agent needs to decide whether
// to narrow the query or accept a partial picture.
type BudgetError struct {
	Dimension Dimension
	Limit     int64
	Requested int64
	Used      Usage
	// Estimated marks an error raised by a pre-flight estimate rather than by
	// a charge for data already received.
	Estimated bool
}

func (e *BudgetError) Error() string {
	what := "would exceed"
	if e.Estimated {
		what = "is estimated to exceed"
	}
	return fmt.Sprintf("policy: query budget exceeded: %s %s limit %d (requested %d, already used %d)",
		what, e.Dimension, e.Limit, e.Requested, e.usedOn())
}

func (e *BudgetError) Unwrap() error { return ErrBudgetExceeded }

func (e *BudgetError) usedOn() int64 {
	switch e.Dimension {
	case DimensionHARequests:
		return int64(e.Used.HARequests)
	case DimensionHistoryPoints:
		return int64(e.Used.HistoryPoints)
	case DimensionEntities:
		return int64(e.Used.Entities)
	default:
		return e.Used.Bytes
	}
}

// QueryBudget is one invocation's charged budget. It is charged, not checked
// once: every upstream request, every history point and every appended byte
// decrements it, because a check at tool entry cannot know what the tool will
// go on to do (phase 02 design notes).
//
// It is safe for concurrent use — a composite diagnostic fans out.
//
// The zero value is not usable; construct with NewQueryBudget.
type QueryBudget struct {
	limits Limits

	mu   sync.Mutex
	used Usage
}

// NewQueryBudget returns a budget with the measured defaults for a class.
func NewQueryBudget(c Class) *QueryBudget {
	return NewQueryBudgetWith(LimitsFor(c))
}

// NewQueryBudgetWith returns a budget with owner-configured limits. Limits are
// configurable; read-only-ness is not (CLAUDE.md, Configuration). A limit left
// at zero means zero — nothing may be spent on that dimension — except the
// deadline, where zero would mean an already-expired context rather than a
// bound, so it falls back to the tightest class default.
func NewQueryBudgetWith(l Limits) *QueryBudget {
	if l.Deadline <= 0 {
		l.Deadline = LimitsFor(ClassNormalRead).Deadline
	}
	return &QueryBudget{limits: l}
}

// Limits returns the ceilings this budget enforces.
func (b *QueryBudget) Limits() Limits { return b.limits }

// Usage returns what has been spent so far.
func (b *QueryBudget) Usage() Usage {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used
}

// ChargeHARequests charges n upstream requests.
func (b *QueryBudget) ChargeHARequests(n int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.checkLocked(DimensionHARequests, int64(n), int64(b.used.HARequests), int64(b.limits.MaxHARequests)); err != nil {
		return err
	}
	b.used.HARequests += n
	return nil
}

// ChargeHistoryPoints charges n recorded data points. Statistics buckets count
// here too: doc §10 names the dimension for history, but both sources return
// points and neither cap implies the other at 37 B and 110 B per point.
func (b *QueryBudget) ChargeHistoryPoints(n int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.checkLocked(DimensionHistoryPoints, int64(n), int64(b.used.HistoryPoints), int64(b.limits.MaxHistoryPoints)); err != nil {
		return err
	}
	b.used.HistoryPoints += n
	return nil
}

// ChargeEntities charges n distinct entities touched by the invocation.
func (b *QueryBudget) ChargeEntities(n int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.checkLocked(DimensionEntities, int64(n), int64(b.used.Entities), int64(b.limits.MaxEntities)); err != nil {
		return err
	}
	b.used.Entities += n
	return nil
}

// ChargeBytes charges n bytes of upstream payload or formatted response.
func (b *QueryBudget) ChargeBytes(n int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.checkLocked(DimensionBytes, n, b.used.Bytes, b.limits.MaxBytes); err != nil {
		return err
	}
	b.used.Bytes += n
	return nil
}

// checkLocked reports whether requested more of one dimension fits, without
// applying it. A refused charge is not counted: the error reports what was
// actually retrieved, and a budget that counted its own refusals could not
// say that.
func (b *QueryBudget) checkLocked(dim Dimension, requested, used, limit int64) error {
	if requested < 0 {
		return fmt.Errorf("policy: negative charge of %d on %s", requested, dim)
	}
	if used+requested > limit {
		return &BudgetError{Dimension: dim, Limit: limit, Requested: requested, Used: b.used}
	}
	return nil
}

// budgetKey is unexported so no other package can plant or read a budget
// through a colliding context key.
type budgetKey struct{}

// WithBudget attaches b to ctx and bounds the context by b's deadline, so an
// upstream call issued under it is cancelled rather than left running when the
// invocation's time is up. The caller must call the returned cancel.
func WithBudget(ctx context.Context, b *QueryBudget) (context.Context, context.CancelFunc) {
	ctx = context.WithValue(ctx, budgetKey{}, b)
	return context.WithTimeout(ctx, b.limits.Deadline)
}

// BudgetFrom returns the budget attached to ctx. The second result is false
// when there is none: a caller that forgot to attach one must fail closed, not
// receive an unlimited budget.
func BudgetFrom(ctx context.Context) (*QueryBudget, bool) {
	b, ok := ctx.Value(budgetKey{}).(*QueryBudget)
	return b, ok
}
