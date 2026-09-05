package policy

import (
	"fmt"
	"sync"
	"time"
)

// Default arrival limits for MCP invocations, confirmed by
// docs/research/2026-09-05-ha-invocation-rate-limit.md (P2-06, resolving
// F-20): a sustained stream of budget-compliant history calls (10 ids, 24h —
// the 2026-08-24 run's largest rung inside both the byte and point caps)
// showed no latency or Core CPU degradation at 1, 2 or 4 calls/s, double the
// interval below. The values are unchanged from their original derivation —
// the 2026-08-24 single-call run's worst in-budget cold call, 339 ms, implying
// roughly three back-to-back calls saturate one recorder read stream — but
// now carry a stream measurement's provenance, not only a single-call one.
// Sustained arrivals are held below that so a storm cannot pin the recorder,
// while the burst covers an interactive investigation's opening fan-out
// (threat T1, Appendix B "request storm").
const (
	invocationBurst    = 10
	invocationInterval = 500 * time.Millisecond // 2 invocations/second sustained
)

// RateLimitError says how long the caller should wait, so a well-behaved
// client can back off instead of retrying into the same wall.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("policy: rate limited, retry after %s", e.RetryAfter)
}

func (e *RateLimitError) Unwrap() error { return ErrRateLimited }

// RateLimiter is a token bucket bounding how fast invocations may arrive. It
// is deliberately separate from QueryBudget: a budget bounds what one
// invocation spends, and a client that stays inside every budget can still
// storm the recorder by issuing max-page calls in a loop.
//
// Safe for concurrent use. The zero value is not usable; construct with
// NewRateLimiter or NewInvocationLimiter.
type RateLimiter struct {
	burst    int
	interval time.Duration

	// now is injected so tests can advance time without sleeping.
	now func() time.Time

	mu     sync.Mutex
	tokens int
	// last is the instant the current token count was accounted for. It is
	// set on first use rather than at construction, so the limiter does not
	// start out already owing refills for the time since it was built.
	last time.Time
}

// NewRateLimiter returns a limiter allowing burst arrivals immediately and one
// further arrival per interval thereafter.
func NewRateLimiter(burst int, interval time.Duration) *RateLimiter {
	return &RateLimiter{burst: burst, interval: interval, tokens: burst, now: time.Now}
}

// NewInvocationLimiter returns the limiter guarding MCP invocations, with the
// defaults above.
func NewInvocationLimiter() *RateLimiter {
	return NewRateLimiter(invocationBurst, invocationInterval)
}

// Allow consumes one token, or returns a *RateLimitError wrapping
// ErrRateLimited. A refused arrival is refused outright rather than queued:
// buffering a storm is still serving it, just later (CLAUDE.md, Concurrency —
// bounded queues with an explicit drop policy).
func (r *RateLimiter) Allow() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	if r.last.IsZero() {
		r.last = now
	}
	if refilled := int(now.Sub(r.last) / r.interval); refilled > 0 {
		r.tokens = min(r.burst, r.tokens+refilled)
		// Advance by whole intervals only, so the fraction of an interval
		// already elapsed still counts toward the next token. Idle credit is
		// capped by burst above, not banked indefinitely.
		r.last = r.last.Add(time.Duration(refilled) * r.interval)
	}

	if r.tokens > 0 {
		r.tokens--
		return nil
	}

	retryAfter := r.last.Add(r.interval).Sub(now)
	if retryAfter <= 0 {
		retryAfter = r.interval
	}
	return &RateLimitError{RetryAfter: retryAfter}
}
