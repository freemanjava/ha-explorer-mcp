package ha

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestConnectWithBackoff_TransientFailure_RetriesThenSucceeds(t *testing.T) {
	var attempts atomic.Int32
	srv := newFakeHAServer(t, func(ctx context.Context, conn *websocket.Conn) {
		n := attempts.Add(1)
		if n <= 2 {
			// Simulate a dropped connection before the handshake starts.
			_ = conn.Close(websocket.StatusNormalClosure, "simulated drop")
			return
		}
		if !serveAuthHandshake(ctx, conn, testToken) {
			return
		}
		servePingLoop(ctx, conn)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := ConnectWithBackoff(ctx, wsURL(srv), testToken, slog.Default(), 5)
	if err != nil {
		t.Fatalf("ConnectWithBackoff: unexpected error: %v", err)
	}
	defer client.Close()

	if got := attempts.Load(); got != 3 {
		t.Fatalf("server saw %d connection attempts, want exactly 3 (2 failures + 1 success)", got)
	}
}

func TestConnectWithBackoff_AuthFailure_NoRetry(t *testing.T) {
	var attempts atomic.Int32
	srv := newFakeHAServer(t, func(ctx context.Context, conn *websocket.Conn) {
		attempts.Add(1)
		serveAuthHandshake(ctx, conn, testToken)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := ConnectWithBackoff(ctx, wsURL(srv), "wrong-token", slog.Default(), 5)
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("ConnectWithBackoff: got %v, want ErrAuthFailed", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("server saw %d connection attempts, want exactly 1 (auth errors are never retried)", got)
	}
}

func TestConnectWithBackoff_BoundedAttempts_GivesUp(t *testing.T) {
	srv := newFakeHAServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = conn.Close(websocket.StatusNormalClosure, "always drop")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := ConnectWithBackoff(ctx, wsURL(srv), testToken, slog.Default(), 3)
	if !errors.Is(err, ErrUpstreamUnavailable) {
		t.Fatalf("ConnectWithBackoff: got %v, want ErrUpstreamUnavailable after exhausting attempts", err)
	}
}
