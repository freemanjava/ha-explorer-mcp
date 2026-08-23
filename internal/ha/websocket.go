// Package ha adapts to Home Assistant Core and Supervisor: the WebSocket
// and REST clients that reach Core through the Supervisor proxy. Only the
// connection and auth handshake live here in phase 00 — the command
// allow-list gateway is Phase 01.
package ha

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// maxHandshakeFrame bounds the auth_required/auth_ok/auth_invalid frames.
// The documented payloads are a few dozen bytes; this only guards against a
// misbehaving peer, not variability in a legitimate one.
const maxHandshakeFrame = 4096

// Client is a single, already-authenticated WebSocket connection to Home
// Assistant Core through the Supervisor proxy. It handles one request in
// flight at a time; the long-lived, multiplexing, auto-reconnecting
// connection manager is P1-01 — this is the "simplest form" the spike task
// calls for.
type Client struct {
	conn   *websocket.Conn
	logger *slog.Logger
	nextID atomic.Uint64
}

// haMessage covers the handshake and ping/pong envelope fields. Command
// result envelopes (id/type/success/result/error) belong to the gateway in
// Phase 01, not this spike.
type haMessage struct {
	Type      string `json:"type"`
	ID        uint64 `json:"id,omitempty"`
	Message   string `json:"message,omitempty"`
	HAVersion string `json:"ha_version,omitempty"`
}

type authMessage struct {
	Type        string `json:"type"`
	AccessToken string `json:"access_token"`
}

type pingMessage struct {
	ID   uint64 `json:"id"`
	Type string `json:"type"`
}

// Connect dials the Home Assistant WebSocket endpoint and completes the
// documented auth handshake (auth_required -> auth -> auth_ok|auth_invalid)
// with token. token is read once by the caller from SUPERVISOR_TOKEN and is
// never logged or embedded in any error Connect returns.
func Connect(ctx context.Context, url string, token string, logger *slog.Logger) (*Client, error) {
	if logger == nil {
		logger = slog.Default()
	}

	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: dial %s: %w", ErrUpstreamUnavailable, url, err)
	}
	conn.SetReadLimit(maxHandshakeFrame)

	c := &Client{conn: conn, logger: logger}
	if err := c.authenticate(ctx, token); err != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "auth failed")
		return nil, err
	}

	logger.InfoContext(ctx, "ha: websocket connected")
	return c, nil
}

func (c *Client) authenticate(ctx context.Context, token string) error {
	var required haMessage
	if err := wsjson.Read(ctx, c.conn, &required); err != nil {
		return fmt.Errorf("%w: reading auth_required", ErrUpstreamUnavailable)
	}
	if required.Type != "auth_required" {
		return fmt.Errorf("%w: expected auth_required, got %q", ErrUnexpectedMessage, required.Type)
	}

	if err := wsjson.Write(ctx, c.conn, authMessage{Type: "auth", AccessToken: token}); err != nil {
		return fmt.Errorf("%w: sending auth", ErrUpstreamUnavailable)
	}

	var result haMessage
	if err := wsjson.Read(ctx, c.conn, &result); err != nil {
		return fmt.Errorf("%w: reading auth result", ErrUpstreamUnavailable)
	}

	switch result.Type {
	case "auth_ok":
		return nil
	case "auth_invalid":
		return ErrAuthFailed
	default:
		return fmt.Errorf("%w: expected auth_ok or auth_invalid, got %q", ErrUnexpectedMessage, result.Type)
	}
}

// Ping issues one trivial read round-trip (HA's ping/pong command) over the
// authenticated connection, proving it is live end to end.
func (c *Client) Ping(ctx context.Context) error {
	id := c.nextID.Add(1)
	if err := wsjson.Write(ctx, c.conn, pingMessage{ID: id, Type: "ping"}); err != nil {
		return fmt.Errorf("%w: sending ping", ErrUpstreamUnavailable)
	}

	var resp haMessage
	if err := wsjson.Read(ctx, c.conn, &resp); err != nil {
		return fmt.Errorf("%w: reading pong", ErrUpstreamUnavailable)
	}
	if resp.Type != "pong" || resp.ID != id {
		return fmt.Errorf("%w: expected pong id=%d, got type=%q id=%d", ErrUnexpectedMessage, id, resp.Type, resp.ID)
	}
	return nil
}

// Close closes the connection with a normal closure code.
func (c *Client) Close() error {
	return c.conn.Close(websocket.StatusNormalClosure, "")
}
