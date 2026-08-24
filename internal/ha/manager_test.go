package ha

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// pendingCount reports how many request slots the manager's current session
// holds. Tests use it to assert that a finished call — however it finished —
// leaves no slot behind; a leaked slot is a leaked goroutine's worth of state
// on a process meant to run for months.
func (m *Manager) pendingCount() int {
	m.mu.Lock()
	s := m.current
	m.mu.Unlock()
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending)
}

// startManager wires a manager to srv, starts it, and stops it on cleanup.
func startManager(t *testing.T, url string, logger *slog.Logger) *Manager {
	t.Helper()
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	m := NewManager(url, testToken, logger)
	m.Start(context.Background())
	t.Cleanup(m.Close)
	return m
}

func TestManager_Request_RoundTripsResult(t *testing.T) {
	srv := newFakeHAServer(t, func(ctx context.Context, conn *websocket.Conn) {
		if !serveAuthHandshake(ctx, conn, testToken) {
			return
		}
		serveCommands(ctx, conn, func(cmd commandFrame) {
			_ = writeResult(ctx, conn, cmd.ID, map[string]any{"echoed": cmd.Type})
		})
	})

	m := startManager(t, wsURL(srv), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	raw, err := m.Call(ctx, BareCommand("get_config"))
	if err != nil {
		t.Fatalf("Call: unexpected error: %v", err)
	}
	var got struct {
		Echoed string `json:"echoed"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decoding result: %v", err)
	}
	if got.Echoed != "get_config" {
		t.Fatalf("result echoed %q, want %q", got.Echoed, "get_config")
	}
	if n := m.pendingCount(); n != 0 {
		t.Fatalf("%d pending slots left after a completed call, want 0", n)
	}
}

func TestManager_ErrorResult_ReturnsCommandError(t *testing.T) {
	srv := newFakeHAServer(t, func(ctx context.Context, conn *websocket.Conn) {
		if !serveAuthHandshake(ctx, conn, testToken) {
			return
		}
		serveCommands(ctx, conn, func(cmd commandFrame) {
			_ = writeErrorResult(ctx, conn, cmd.ID, "not_found", "Entity not found")
		})
	})

	m := startManager(t, wsURL(srv), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := m.Call(ctx, BareCommand("get_states"))
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("Call: got %v, want *CommandError", err)
	}
	if cmdErr.Code != "not_found" {
		t.Fatalf("CommandError.Code = %q, want %q", cmdErr.Code, "not_found")
	}
	if n := m.pendingCount(); n != 0 {
		t.Fatalf("%d pending slots left after an error result, want 0", n)
	}
}

func TestManager_ConcurrentRequests_OutOfOrderReplies_ResolveCorrectCallers(t *testing.T) {
	const requests = 4

	var mu sync.Mutex
	var collected []commandFrame
	release := make(chan struct{})

	srv := newFakeHAServer(t, func(ctx context.Context, conn *websocket.Conn) {
		if !serveAuthHandshake(ctx, conn, testToken) {
			return
		}
		serveCommands(ctx, conn, func(cmd commandFrame) {
			mu.Lock()
			collected = append(collected, cmd)
			ready := len(collected) == requests
			batch := collected
			mu.Unlock()
			if !ready {
				return
			}
			// Reply in reverse arrival order: correlation must come from the
			// message id, never from the order replies happen to arrive in.
			for i := len(batch) - 1; i >= 0; i-- {
				_ = writeResult(ctx, conn, batch[i].ID, map[string]any{"for": batch[i].Type})
			}
			close(release)
		})
	})

	m := startManager(t, wsURL(srv), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	commands := []string{"cmd_a", "cmd_b", "cmd_c", "cmd_d"}

	var wg sync.WaitGroup
	errs := make([]error, requests)
	results := make([]string, requests)
	for i, name := range commands {
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, err := m.Call(ctx, BareCommand(name))
			if err != nil {
				errs[i] = err
				return
			}
			var got struct {
				For string `json:"for"`
			}
			errs[i] = json.Unmarshal(raw, &got)
			results[i] = got.For
		}()
	}
	wg.Wait()

	select {
	case <-release:
	default:
		t.Fatalf("server never saw all %d requests concurrently in flight", requests)
	}

	for i, name := range commands {
		if errs[i] != nil {
			t.Fatalf("Call(%s): unexpected error: %v", name, errs[i])
		}
		if results[i] != name {
			t.Fatalf("Call(%s) resolved to result for %q — replies crossed callers", name, results[i])
		}
	}
	if n := m.pendingCount(); n != 0 {
		t.Fatalf("%d pending slots left after %d completed calls, want 0", n, requests)
	}
}

func TestManager_SocketClosesInFlight_ReturnsTypedErrorNotHang(t *testing.T) {
	srv := newFakeHAServer(t, func(ctx context.Context, conn *websocket.Conn) {
		if !serveAuthHandshake(ctx, conn, testToken) {
			return
		}
		serveCommands(ctx, conn, func(cmd commandFrame) {
			// Drop the connection with the request in flight, as an HA
			// restart does — never send a reply.
			conn.CloseNow()
		})
	})

	m := startManager(t, wsURL(srv), nil)

	// Deliberately generous: the point is that Call returns on the close, not
	// on this deadline. A hang would fail the test as a timeout, not as a
	// typed error.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { _, err := m.Call(ctx, BareCommand("get_states")); done <- err }()

	select {
	case err := <-done:
		if !errors.Is(err, ErrUpstreamUnavailable) {
			t.Fatalf("Call: got %v, want ErrUpstreamUnavailable", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Call hung after the socket closed with the request in flight")
	}
}

func TestManager_HARestart_ReconnectsReauthenticatesAndServesNextRequest(t *testing.T) {
	var handshakes atomic.Int32
	srv := newFakeHAServer(t, func(ctx context.Context, conn *websocket.Conn) {
		if !serveAuthHandshake(ctx, conn, testToken) {
			return
		}
		generation := handshakes.Add(1)
		serveCommands(ctx, conn, func(cmd commandFrame) {
			if generation == 1 {
				// First connection dies mid-request: the HA restart.
				conn.CloseNow()
				return
			}
			_ = writeResult(ctx, conn, cmd.ID, map[string]any{"generation": generation})
		})
	})

	m := startManager(t, wsURL(srv), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if _, err := m.Call(ctx, BareCommand("get_states")); !errors.Is(err, ErrUpstreamUnavailable) {
		t.Fatalf("first Call: got %v, want ErrUpstreamUnavailable", err)
	}

	raw, err := m.Call(ctx, BareCommand("get_states"))
	if err != nil {
		t.Fatalf("Call after reconnect: unexpected error: %v", err)
	}
	var got struct {
		Generation int `json:"generation"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decoding result: %v", err)
	}
	if got.Generation < 2 {
		t.Fatalf("served by generation %d, want a re-authenticated connection (>= 2)", got.Generation)
	}
	if n := handshakes.Load(); n < 2 {
		t.Fatalf("server completed %d auth handshakes, want at least 2 (reconnect re-authenticates)", n)
	}
	if m.Reconnects() == 0 {
		t.Fatal("Reconnects() = 0 after a reconnect: recovery must be counted, never silent")
	}
}

func TestManager_CallerContextCancelled_FreesPendingSlot(t *testing.T) {
	received := make(chan struct{}, 1)
	srv := newFakeHAServer(t, func(ctx context.Context, conn *websocket.Conn) {
		if !serveAuthHandshake(ctx, conn, testToken) {
			return
		}
		serveCommands(ctx, conn, func(cmd commandFrame) {
			// Accept the command and never answer it.
			select {
			case received <- struct{}{}:
			default:
			}
		})
	})

	m := startManager(t, wsURL(srv), nil)

	callCtx, cancelCall := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := m.Call(callCtx, BareCommand("get_states")); done <- err }()

	select {
	case <-received:
	case <-time.After(10 * time.Second):
		t.Fatal("server never received the command")
	}

	cancelCall()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Call: got %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Call did not return after its context was cancelled")
	}

	if n := m.pendingCount(); n != 0 {
		t.Fatalf("%d pending slots left after cancellation, want 0", n)
	}
}

func TestManager_NoDeadlineOnCallerContext_StillBounded(t *testing.T) {
	srv := newFakeHAServer(t, func(ctx context.Context, conn *websocket.Conn) {
		if !serveAuthHandshake(ctx, conn, testToken) {
			return
		}
		serveCommands(ctx, conn, func(cmd commandFrame) {}) // never answers
	})

	m := startManager(t, wsURL(srv), nil)

	// A caller that forgets a deadline must not park a request forever: the
	// manager applies defaultCallTimeout. Shortened here so the test is fast.
	restore := defaultCallTimeout
	defaultCallTimeout = 300 * time.Millisecond
	t.Cleanup(func() { defaultCallTimeout = restore })

	_, err := m.Call(context.Background(), BareCommand("get_states"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Call: got %v, want context.DeadlineExceeded from the default deadline", err)
	}
}

func TestManager_TokenNeverLogged(t *testing.T) {
	srv := newFakeHAServer(t, func(ctx context.Context, conn *websocket.Conn) {
		if !serveAuthHandshake(ctx, conn, testToken) {
			return
		}
		serveCommands(ctx, conn, func(cmd commandFrame) { conn.CloseNow() })
	})

	var logBuf syncBuffer
	m := startManager(t, wsURL(srv), slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = m.Call(ctx, BareCommand("get_states"))
	_, _ = m.Call(ctx, BareCommand("get_states"))

	if logBuf.contains(testToken) {
		t.Fatalf("log output contains the token: %s", logBuf.String())
	}
}

// syncBuffer is a bytes.Buffer safe for the manager's goroutines to write to
// while the test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

func (b *syncBuffer) contains(s string) bool {
	return strings.Contains(b.String(), s)
}
