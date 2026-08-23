package ha

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const testToken = "s3cr3t-supervisor-token-do-not-log-me"

func TestConnect_ValidToken_HandshakeSucceedsAndPingRoundTrips(t *testing.T) {
	srv := newFakeHAServer(t, func(ctx context.Context, conn *websocket.Conn) {
		if !serveAuthHandshake(ctx, conn, testToken) {
			return
		}
		servePingLoop(ctx, conn)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := Connect(ctx, wsURL(srv), testToken, slog.Default())
	if err != nil {
		t.Fatalf("Connect: unexpected error: %v", err)
	}
	defer client.Close()

	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping: unexpected error: %v", err)
	}
}

func TestConnect_WrongToken_TypedAuthErrorNoRetryStorm(t *testing.T) {
	var attempts atomic.Int32
	srv := newFakeHAServer(t, func(ctx context.Context, conn *websocket.Conn) {
		attempts.Add(1)
		serveAuthHandshake(ctx, conn, testToken)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := Connect(ctx, wsURL(srv), "wrong-token", slog.Default())
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("Connect: got %v, want ErrAuthFailed", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("server saw %d connection attempts, want exactly 1 (no retry storm)", got)
	}
}

func TestHandshakeDoesNotLogToken(t *testing.T) {
	tests := []struct {
		name        string
		serverToken string
		dialToken   string
	}{
		{"auth_ok", testToken, testToken},
		{"auth_invalid", testToken, "wrong-token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newFakeHAServer(t, func(ctx context.Context, conn *websocket.Conn) {
				serveAuthHandshake(ctx, conn, tt.serverToken)
			})

			var logBuf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logBuf, nil))

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			client, err := Connect(ctx, wsURL(srv), tt.dialToken, logger)
			if client != nil {
				defer client.Close()
			}
			_ = err

			for _, token := range []string{tt.serverToken, tt.dialToken} {
				if bytes.Contains(logBuf.Bytes(), []byte(token)) {
					t.Fatalf("log output contains the token: %s", logBuf.String())
				}
			}
		})
	}
}

func TestConnect_ServerUnreachable_ReturnsUpstreamUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := Connect(ctx, "ws://127.0.0.1:1/core/websocket", testToken, slog.Default())
	if !errors.Is(err, ErrUpstreamUnavailable) {
		t.Fatalf("Connect: got %v, want ErrUpstreamUnavailable", err)
	}
}
