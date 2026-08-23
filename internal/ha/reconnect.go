package ha

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"time"
)

// Backoff parameters for ConnectWithBackoff. Named per the doc's "never a
// tight retry loop" rule (CLAUDE.md Concurrency); the full connection
// manager (live reconnect-on-drop, single-flight, request correlation) is
// P1-01 — this is reconnect at its simplest form.
const (
	backoffBase           = 200 * time.Millisecond
	backoffMax            = 5 * time.Second
	backoffJitterFraction = 0.2
)

// ConnectWithBackoff calls Connect, retrying a transient upstream failure
// with bounded exponential backoff and jitter. An auth rejection
// (ErrAuthFailed) is never retried: a bad token retried in a loop is a
// self-inflicted denial of service against Supervisor, not a transient
// condition backoff can fix.
func ConnectWithBackoff(ctx context.Context, url, token string, logger *slog.Logger, maxAttempts int) (*Client, error) {
	if logger == nil {
		logger = slog.Default()
	}

	var lastErr error
	for attempt := range maxAttempts {
		client, err := Connect(ctx, url, token, logger)
		if err == nil {
			return client, nil
		}
		if errors.Is(err, ErrAuthFailed) {
			return nil, err
		}
		lastErr = err

		if attempt == maxAttempts-1 {
			break
		}

		delay := backoffDelay(attempt)
		logger.WarnContext(ctx, "ha: websocket connect failed, retrying",
			"attempt", attempt+1, "max_attempts", maxAttempts, "delay_ms", delay.Milliseconds())

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, lastErr
}

func backoffDelay(attempt int) time.Duration {
	d := backoffBase * time.Duration(uint64(1)<<uint(attempt))
	if d > backoffMax || d <= 0 {
		d = backoffMax
	}
	jitter := time.Duration(rand.Float64() * backoffJitterFraction * float64(d))
	return d + jitter
}
