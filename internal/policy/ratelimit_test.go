package policy

import (
	"errors"
	"testing"
	"time"
)

// testClock makes the token bucket deterministic: no sleeping in tests.
type testClock struct{ t time.Time }

func (c *testClock) now() time.Time          { return c.t }
func (c *testClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestLimiter(burst int, interval time.Duration) (*RateLimiter, *testClock) {
	c := &testClock{t: time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)}
	r := NewRateLimiter(burst, interval)
	r.now = c.now
	return r, c
}

// Appendix B: "MCP client requests maximum page repeatedly / request storm"
// (threat T1). The storm must be refused, not served.
func TestRateLimiter_RequestStorm_RefusedAfterBurst(t *testing.T) {
	r, _ := newTestLimiter(3, time.Second)

	served := 0
	var lastErr error
	for range 50 {
		if err := r.Allow(); err == nil {
			served++
		} else {
			lastErr = err
		}
	}

	if served != 3 {
		t.Fatalf("served %d of 50 storm requests, want the 3-request burst", served)
	}
	if !errors.Is(lastErr, ErrRateLimited) {
		t.Fatalf("lastErr = %v, want ErrRateLimited", lastErr)
	}
}

func TestRateLimiter_AfterRefillInterval_AllowsAgain(t *testing.T) {
	r, clock := newTestLimiter(2, time.Second)

	for range 2 {
		if err := r.Allow(); err != nil {
			t.Fatalf("burst request refused: %v", err)
		}
	}
	if err := r.Allow(); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited once the burst is spent", err)
	}

	clock.advance(time.Second)
	if err := r.Allow(); err != nil {
		t.Fatalf("one token should have refilled after the interval: %v", err)
	}
	if err := r.Allow(); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want only one refilled token", err)
	}
}

// An idle client must not bank unlimited credit for a later storm.
func TestRateLimiter_LongIdle_CreditCappedAtBurst(t *testing.T) {
	r, clock := newTestLimiter(3, time.Second)
	for range 3 {
		if err := r.Allow(); err != nil {
			t.Fatalf("burst request refused: %v", err)
		}
	}
	clock.advance(time.Hour)

	served := 0
	for range 10 {
		if err := r.Allow(); err == nil {
			served++
		}
	}
	if served != 3 {
		t.Fatalf("served %d after an hour idle, want the %d-token cap", served, 3)
	}
}

func TestRateLimiter_Error_NamesTheRetryDelay(t *testing.T) {
	r, _ := newTestLimiter(1, 2*time.Second)
	if err := r.Allow(); err != nil {
		t.Fatalf("first request: %v", err)
	}

	err := r.Allow()
	var rl *RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("err = %v, want a *RateLimitError", err)
	}
	if rl.RetryAfter <= 0 || rl.RetryAfter > 2*time.Second {
		t.Fatalf("RetryAfter = %v, want a positive delay no longer than the refill interval", rl.RetryAfter)
	}
}

func TestNewInvocationLimiter_Defaults_AllowAnInteractiveBurst(t *testing.T) {
	r := NewInvocationLimiter()
	served := 0
	for range invocationBurst {
		if err := r.Allow(); err == nil {
			served++
		}
	}
	if served != invocationBurst {
		t.Fatalf("served %d of %d, want the whole default burst", served, invocationBurst)
	}
	if err := r.Allow(); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited past the default burst", err)
	}
}

func TestRateLimiter_ConcurrentCallers_NeverExceedBurst(t *testing.T) {
	r, _ := newTestLimiter(5, time.Hour)

	results := make(chan error, 40)
	for range 40 {
		go func() { results <- r.Allow() }()
	}

	served := 0
	for range 40 {
		if err := <-results; err == nil {
			served++
		}
	}
	if served != 5 {
		t.Fatalf("served %d concurrent requests, want exactly the 5-token burst", served)
	}
}
