// Command spike is P0-04's throwaway verification vehicle: it asks a live Home
// Assistant which registry and config-entry read commands actually exist, what
// they answer with, and which of them a non-admin user is refused.
//
// It is deliberately not built on internal/ha's Client. That package has no
// generic "send any command" method and must not grow one: Phase 01's gateway
// allow-list is what decides which commands may ever leave the process, and a
// general sender added before it exists is exactly the escape hatch CLAUDE.md
// rule 2 forbids. The spike dials and authenticates on its own, and is deleted
// when Phase 00 closes.
//
// Output is a markdown report containing field names and types only — never a
// value from the installation, and never the token. See shape.go.
//
// Usage:
//
//	HA_URL=http://homeassistant.local:8123 HA_TOKEN=<long-lived token> \
//	  go run ./cmd/spike > report.md
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// A full entity registry on a large installation is a few megabytes; this
// bounds a misbehaving peer without truncating a legitimate answer.
const maxFrame = 64 << 20

const requestTimeout = 30 * time.Second

// probe is one WebSocket command to try. args are literal, non-sensitive
// values; needsID marks the two commands that must be fed an id discovered
// from an earlier list, since neither accepts a wildcard.
type probe struct {
	command string
	args    map[string]any
	needsID string // "entity" or "config_entry"
	note    string
}

// The commands Phase 01's allow-list is expected to contain, per the
// architecture doc §9. Every one of them must appear in the research file
// either with an observed response or explicitly marked unavailable.
var probes = []probe{
	{command: "get_config", note: "HA version and unit system"},
	{command: "auth/current_user", note: "establishes whether this run is admin"},
	{command: "config/entity_registry/list"},
	{command: "config/entity_registry/list_for_display", note: "cheaper list variant, may not exist on older releases"},
	{command: "config/entity_registry/get", needsID: "entity"},
	{command: "config/device_registry/list"},
	{command: "config/area_registry/list"},
	{command: "config/floor_registry/list"},
	{command: "config/label_registry/list"},
	{command: "config/category_registry/list", args: map[string]any{"scope": "automation"}},
	{command: "config_entries/get"},
	{command: "config_entries/get_single", needsID: "config_entry"},
}

type result struct {
	ID      uint64          `json:"id"`
	Type    string          `json:"type"`
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "spike: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	baseURL := strings.TrimSuffix(os.Getenv("HA_URL"), "/")
	token := os.Getenv("HA_TOKEN")
	if baseURL == "" || token == "" {
		return fmt.Errorf("set HA_URL (e.g. http://homeassistant.local:8123) and HA_TOKEN")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	out := &report{}
	out.writef("# P0-04 probe — registry & config-entry read APIs\n\n")
	out.writef("Run at %s (UTC)\n\n", time.Now().UTC().Format(time.RFC3339))

	if err := probeREST(ctx, out, baseURL, token); err != nil {
		out.writef("REST `GET /api/config` failed: %v\n\n", err)
	}

	if err := probeWebSocket(ctx, out, baseURL, token); err != nil {
		return err
	}

	fmt.Print(out.String())
	return nil
}

func probeREST(ctx context.Context, out *report, baseURL, token string) error {
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/api/config", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	var body any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("status %d, undecodable body: %w", resp.StatusCode, err)
	}

	out.writef("## REST `GET /api/config`\n\nHTTP %d\n\n```\n%s```\n\n", resp.StatusCode, renderShape(shapeOf(body)))

	// The HA version is the one value the research file is required to carry,
	// and it identifies a release, not the owner.
	if m, ok := body.(map[string]any); ok {
		if v, ok := m["version"].(string); ok {
			out.writef("**Observed HA version: `%s`**\n\n", v)
		}
	}
	return nil
}

func probeWebSocket(ctx context.Context, out *report, baseURL, token string) error {
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/api/websocket"

	// The handshake response is discarded deliberately: coder/websocket
	// documents that its Body never needs closing, so an analyser reporting a
	// leak here is applying a net/http rule this package does not follow.
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", wsURL, err)
	}
	// Close's error is not actionable: the report is already written and the
	// process is exiting either way.
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
	conn.SetReadLimit(maxFrame)

	if err := authenticate(ctx, conn, token); err != nil {
		return err
	}

	out.writef("## WebSocket commands\n\n")

	var nextID uint64
	var entityID, configEntryID string

	for _, p := range probes {
		payload := map[string]any{"type": p.command}
		for k, v := range p.args {
			payload[k] = v
		}

		argNote := describeArgs(p.args)
		switch p.needsID {
		case "entity":
			if entityID == "" {
				out.writef("### `%s`\n\nSKIPPED — no entity_id available from the list command.\n\n", p.command)
				continue
			}
			payload["entity_id"] = entityID
			argNote = `entity_id: <one id taken from the list result>`
		case "config_entry":
			if configEntryID == "" {
				out.writef("### `%s`\n\nSKIPPED — no entry_id available from the list command.\n\n", p.command)
				continue
			}
			payload["entry_id"] = configEntryID
			argNote = `entry_id: <one id taken from config_entries/get>`
		}

		nextID++
		payload["id"] = nextID

		start := time.Now()
		res, err := call(ctx, conn, nextID, payload)
		elapsed := time.Since(start).Round(time.Millisecond)

		out.writef("### `%s`\n\n", p.command)
		if p.note != "" {
			out.writef("_%s_\n\n", p.note)
		}
		if argNote != "" {
			out.writef("Arguments: `%s`\n\n", argNote)
		}

		if err != nil {
			out.writef("TRANSPORT FAILURE after %s: %v\n\n", elapsed, err)
			return fmt.Errorf("%s: %w", p.command, err)
		}
		if !res.Success {
			code, msg := "unknown", ""
			if res.Error != nil {
				code, msg = res.Error.Code, res.Error.Message
			}
			out.writef("FAILED after %s — error code `%s`: %s\n\n", elapsed, code, msg)
			continue
		}

		var decoded any
		if err := json.Unmarshal(res.Result, &decoded); err != nil {
			out.writef("OK after %s but result did not decode: %v\n\n", elapsed, err)
			continue
		}

		out.writef("OK after %s (%d bytes)\n\n```\n%s```\n\n", elapsed, len(res.Result), renderShape(shapeOf(decoded)))

		reportCurrentUser(out, p.command, decoded)
		if entityID == "" {
			entityID = firstString(decoded, "entity_id", p.command == "config/entity_registry/list")
		}
		if configEntryID == "" {
			configEntryID = firstString(decoded, "entry_id", p.command == "config_entries/get")
		}
	}

	return nil
}

// reportCurrentUser surfaces the admin flag, which decides how every other
// line in this report must be read: the same command can succeed for an admin
// and be refused for everyone else.
func reportCurrentUser(out *report, command string, decoded any) {
	if command != "auth/current_user" {
		return
	}
	m, ok := decoded.(map[string]any)
	if !ok {
		return
	}
	admin, _ := m["is_admin"].(bool)
	out.writef("**This run authenticated as an admin user: `%t`**\n\n", admin)
}

// firstString pulls one id out of a list result so the two by-id commands have
// something to ask for. The value is used only as a request argument and is
// never written to the report.
func firstString(decoded any, key string, enabled bool) string {
	if !enabled {
		return ""
	}
	list, ok := decoded.([]any)
	if !ok {
		return ""
	}
	for _, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func describeArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(args))
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s: %v", k, v))
	}
	return strings.Join(parts, ", ")
}

func call(ctx context.Context, conn *websocket.Conn, id uint64, payload map[string]any) (*result, error) {
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	if err := wsjson.Write(reqCtx, conn, payload); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	// Nothing is subscribed here, but HA may still interleave frames; read
	// until the id matches rather than assuming the next frame is the answer.
	for {
		var res result
		if err := wsjson.Read(reqCtx, conn, &res); err != nil {
			return nil, fmt.Errorf("read: %w", err)
		}
		if res.Type == "result" && res.ID == id {
			return &res, nil
		}
	}
}

func authenticate(ctx context.Context, conn *websocket.Conn, token string) error {
	var required struct {
		Type string `json:"type"`
	}
	if err := wsjson.Read(ctx, conn, &required); err != nil {
		return fmt.Errorf("reading auth_required: %w", err)
	}
	if required.Type != "auth_required" {
		return fmt.Errorf("expected auth_required, got %q", required.Type)
	}

	if err := wsjson.Write(ctx, conn, map[string]any{"type": "auth", "access_token": token}); err != nil {
		return fmt.Errorf("sending auth: %w", err)
	}

	var res struct {
		Type string `json:"type"`
	}
	if err := wsjson.Read(ctx, conn, &res); err != nil {
		return fmt.Errorf("reading auth result: %w", err)
	}
	if res.Type != "auth_ok" {
		// The token is not in scope here and must not be echoed into the error.
		return fmt.Errorf("authentication rejected (%q) — check HA_TOKEN", res.Type)
	}
	return nil
}
