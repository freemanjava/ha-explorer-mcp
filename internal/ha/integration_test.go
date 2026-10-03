//go:build integration

package ha

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// TestSupervisorProxyConnectivity is P0-03's DoD vehicle: it proves auth
// success, and one WS command round trip against Core through the
// Supervisor proxy shape.
//
// Against a live Supervisor, set HA_TEST_WS_URL and
// HA_TEST_TOKEN (or leave HA_TEST_TOKEN unset to fall back to
// SUPERVISOR_TOKEN, matching production). With none of those set, it runs
// against a "recorded HA": a local server that replays the exact documented
// request/response shapes from docs/HA_Inspector_MCP_Research_and_Architecture.md
// §15.1, so `make test-integration` verifies something deterministic on a
// machine with no Supervisor reachable, per Phase 00's "recorded HA is an
// acceptable vehicle" note.
func TestSupervisorProxyConnectivity(t *testing.T) {
	wsURL := os.Getenv("HA_TEST_WS_URL")
	token := os.Getenv("HA_TEST_TOKEN")
	if token == "" {
		token = os.Getenv("SUPERVISOR_TOKEN")
	}

	if wsURL == "" {
		wsURL, token = startRecordedHA(t)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := Connect(ctx, wsURL, token, slog.Default())
	if err != nil {
		t.Fatalf("Connect: auth handshake failed: %v", err)
	}
	defer client.Close()

	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping: WS round trip failed: %v", err)
	}
}

// startRecordedHA stands up a local server replaying the documented
// Supervisor-proxy shapes (ws://supervisor/core/websocket auth handshake)
// and returns its WS URL and token.
func startRecordedHA(t *testing.T) (wsURL, token string) {
	t.Helper()
	const recordedToken = "recorded-supervisor-token"

	mux := http.NewServeMux()
	mux.HandleFunc("/core/websocket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		if !serveAuthHandshake(r.Context(), conn, recordedToken) {
			return
		}
		servePingLoop(r.Context(), conn)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/core/websocket",
		recordedToken
}
