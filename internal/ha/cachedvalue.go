package ha

import (
	"context"
	"sync"
	"time"
)

// cachedValue holds one TTL-cached value with single-flight refill: while a
// fetch is in progress, concurrent callers wait on its result instead of
// starting one each (CLAUDE.md, Concurrency — "no thundering herd on
// expiry"). The zero value is not usable; construct with newCachedValue.
type cachedValue[T any] struct {
	ttl time.Duration
	now func() time.Time // overridden by tests; time.Now in production

	mu         sync.Mutex
	value      T
	observedAt time.Time
	fetching   chan struct{} // non-nil and open while a fetch is in flight
	fetchErr   error
}

func newCachedValue[T any](ttl time.Duration) *cachedValue[T] {
	return &cachedValue[T]{ttl: ttl, now: time.Now}
}

// Get returns the cached value and the time it was observed, calling fetch to
// refill it if the cache is empty or past its TTL. A caller that arrives
// while a refill from another caller is already in flight waits on that same
// fetch instead of starting its own — the cache is a load-control mechanism,
// not a source of truth (doc §16), so it never serves a value it did not
// itself observe.
func (c *cachedValue[T]) Get(ctx context.Context, fetch func(context.Context) (T, error)) (T, time.Time, error) {
	c.mu.Lock()
	if !c.observedAt.IsZero() && c.now().Sub(c.observedAt) < c.ttl {
		v, t := c.value, c.observedAt
		c.mu.Unlock()
		return v, t, nil
	}
	if ch := c.fetching; ch != nil {
		c.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			var zero T
			return zero, time.Time{}, wrapDeadline(ctx.Err())
		}
		c.mu.Lock()
		v, t, err := c.value, c.observedAt, c.fetchErr
		c.mu.Unlock()
		if err != nil {
			var zero T
			return zero, time.Time{}, err
		}
		return v, t, nil
	}

	ch := make(chan struct{})
	c.fetching = ch
	c.mu.Unlock()

	v, err := fetch(ctx)

	c.mu.Lock()
	if err == nil {
		c.value = v
		c.observedAt = c.now()
		c.fetchErr = nil
	} else {
		c.fetchErr = err
	}
	c.fetching = nil
	close(ch)
	c.mu.Unlock()

	if err != nil {
		var zero T
		return zero, time.Time{}, err
	}
	return v, c.observedAt, nil
}
