//go:build integration

package ha

import (
	"context"
	"encoding/json"
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
// success, one WS command round trip, and one REST GET against Core through
// the Supervisor proxy shape.
//
// Against a live Supervisor, set HA_TEST_WS_URL, HA_TEST_REST_URL and
// HA_TEST_TOKEN (or leave HA_TEST_TOKEN unset to fall back to
// SUPERVISOR_TOKEN, matching production). With none of those set, it runs
// against a "recorded HA": a local server that replays the exact documented
// request/response shapes from docs/HA_Inspector_MCP_Research_and_Architecture.md
// §15.1, so `make test-integration` verifies something deterministic on a
// machine with no Supervisor reachable, per Phase 00's "recorded HA is an
// acceptable vehicle" note.
func TestSupervisorProxyConnectivity(t *testing.T) {
	wsURL := os.Getenv("HA_TEST_WS_URL")
	restURL := os.Getenv("HA_TEST_REST_URL")
	token := os.Getenv("HA_TEST_TOKEN")
	if token == "" {
		token = os.Getenv("SUPERVISOR_TOKEN")
	}

	if wsURL == "" || restURL == "" {
		wsURL, restURL, token = startRecordedHA(t)
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

	body, err := NewRESTClient(restURL, token, http.DefaultClient, slog.Default()).Config(ctx)
	if err != nil {
		t.Fatalf("Config: REST GET failed: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(body, &cfg); err != nil {
		t.Fatalf("Config: response is not a JSON object: %v", err)
	}
	if _, ok := cfg["version"]; !ok {
		t.Fatalf("Config: response missing expected \"version\" field: %v", cfg)
	}
}

// startRecordedHA stands up a local server replaying the documented
// Supervisor-proxy shapes (ws://supervisor/core/websocket auth handshake and
// GET /api/config) and returns its WS URL, REST base URL and token.
func startRecordedHA(t *testing.T) (wsURL, restURL, token string) {
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
	mux.HandleFunc("/core/api/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+recordedToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// Shape observed for GET /api/config, redacted of instance-specific
		// values per docs/HA_Inspector_MCP_Research_and_Architecture.md §15.1.
		_, _ = w.Write([]byte(`{"version":"2026.8.0","location_name":"Home","components":["config","websocket_api"]}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/core/websocket",
		srv.URL + "/core",
		recordedToken
}
