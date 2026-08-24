package ha

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// fakeConnHandler is invoked once per accepted WebSocket connection, after
// the HTTP upgrade but before any protocol message is exchanged, so tests
// can script arbitrary handshake behavior (including dropping the
// connection before auth_required is sent).
type fakeConnHandler func(ctx context.Context, conn *websocket.Conn)

// newFakeHAServer starts an httptest server that speaks WebSocket at
// /core/websocket, driven by handler. It stands in for the "recorded HA"
// this phase's tasks are allowed to use in place of a live Supervisor.
func newFakeHAServer(t *testing.T, handler fakeConnHandler) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/core/websocket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		handler(r.Context(), conn)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// wsURL converts a httptest server's http:// URL into the ws:// URL of its
// /core/websocket endpoint.
func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/core/websocket"
}

// serveAuthHandshake plays the documented auth_required -> auth ->
// auth_ok|auth_invalid exchange and reports whether auth succeeded. It
// leaves the connection open on success so the caller can keep serving
// (e.g. ping/pong) on it.
func serveAuthHandshake(ctx context.Context, conn *websocket.Conn, expectedToken string) bool {
	if err := wsjson.Write(ctx, conn, haMessage{Type: "auth_required", HAVersion: "2026.8.0"}); err != nil {
		return false
	}

	var auth authMessage
	if err := wsjson.Read(ctx, conn, &auth); err != nil {
		return false
	}

	if auth.AccessToken != expectedToken {
		_ = wsjson.Write(ctx, conn, haMessage{Type: "auth_invalid", Message: "Invalid access token or password"})
		return false
	}

	return wsjson.Write(ctx, conn, haMessage{Type: "auth_ok", HAVersion: "2026.8.0"}) == nil
}

// servePingLoop answers ping commands with pong until the connection closes.
func servePingLoop(ctx context.Context, conn *websocket.Conn) {
	for {
		var msg pingMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			return
		}
		if msg.Type == "ping" {
			if err := wsjson.Write(ctx, conn, haMessage{Type: "pong", ID: msg.ID}); err != nil {
				return
			}
		}
	}
}

// commandFrame is a command envelope as the fake server sees it on the wire.
type commandFrame struct {
	ID   uint64 `json:"id"`
	Type string `json:"type"`
}

// serveCommands reads command frames until the connection closes, handing
// each to handle. handle owns the reply, so a test can answer out of order,
// answer late, or never answer at all.
func serveCommands(ctx context.Context, conn *websocket.Conn, handle func(cmd commandFrame)) {
	for {
		var cmd commandFrame
		if err := wsjson.Read(ctx, conn, &cmd); err != nil {
			return
		}
		handle(cmd)
	}
}

// writeResult writes the success form of HA's command result envelope.
func writeResult(ctx context.Context, conn *websocket.Conn, id uint64, result any) error {
	return wsjson.Write(ctx, conn, map[string]any{
		"id": id, "type": "result", "success": true, "result": result,
	})
}

// writeErrorResult writes the failure form of HA's command result envelope.
func writeErrorResult(ctx context.Context, conn *websocket.Conn, id uint64, code, message string) error {
	return wsjson.Write(ctx, conn, map[string]any{
		"id": id, "type": "result", "success": false,
		"error": map[string]any{"code": code, "message": message},
	})
}
