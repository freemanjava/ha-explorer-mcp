// Command spike is Phase 00's throwaway verification vehicle: it asks a live
// Home Assistant which read commands actually exist, what they answer with, and
// which of them a non-admin user is refused.
//
// This revision serves P0-05 (F-3): what can be read about automations —
// their config, their traces, and the fallback evidence if traces are refused.
// The P0-04 registry probe set it replaces is already recorded in
// docs/research/2026-08-23-ha-registry-apis.md.
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
	"io"
	"net/http"
	"net/url"
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

// How far back logbook/get_events is asked to look. Long enough that a daily
// automation shows up, short enough that the query stays cheap on a Pi-sized
// recorder — this probe characterises the shape, not the retention.
const logbookWindow = 24 * time.Hour

// probe is one WebSocket command to try. args are literal, non-sensitive
// values; needs names the ids that must be injected from an earlier answer,
// since none of the trace commands accepts a wildcard.
type probe struct {
	label   string // heading, when one command is probed more than once
	command string
	args    map[string]any
	needs   []string
	note    string
}

// The commands the doc §9 automation tools would need, plus the fallbacks that
// have to be characterised if traces turn out to be unreachable. Every one of
// them must appear in the research file either with an observed response or
// explicitly marked unavailable.
var probes = []probe{
	{command: "get_config", note: "HA version and unit system"},
	{command: "auth/current_user", note: "establishes whether this run is admin"},
	{command: "get_states", note: "where the target automation is discovered, and the last_triggered fallback"},
	{command: "automation/config", needs: []string{"automation_entity"}, note: "the automation's own config, addressed by entity_id"},
	{
		label:   "trace/list (unfiltered)",
		command: "trace/list",
		args:    map[string]any{"domain": "automation"},
		note:    "does the whole domain's trace index come back without naming an automation?",
	},
	{
		label:   "trace/list (one automation)",
		command: "trace/list",
		args:    map[string]any{"domain": "automation"},
		needs:   []string{"automation_numeric"},
		note:    "the trace index for the target automation; source of the run_id below",
	},
	{
		command: "trace/get",
		args:    map[string]any{"domain": "automation"},
		needs:   []string{"automation_numeric", "trace_run"},
		note:    "the full stored trace of one run — the §13.1 workflow's evidence",
	},
	{
		command: "trace/contexts",
		args:    map[string]any{"domain": "automation"},
		needs:   []string{"automation_numeric"},
		note:    "context_id to run_id index, links an observed event back to a run",
	},
	{
		command: "logbook/get_events",
		needs:   []string{"automation_entity_ids", "window"},
		note:    "fallback evidence source if traces are refused",
	},
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
	out.writef("# P0-05 probe — automation config & trace retrieval\n\n")
	out.writef("Run at %s (UTC)\n\n", time.Now().UTC().Format(time.RFC3339))

	if err := probeREST(ctx, out, baseURL, token); err != nil {
		out.writef("REST `GET /api/config` failed: %v\n\n", err)
	}

	target, err := probeWebSocket(ctx, out, baseURL, token)
	if err != nil {
		return err
	}

	// Deliberately last: the config-panel route is addressed by the numeric id
	// the WebSocket run discovers, and it is the one candidate that reads
	// automations from HA's own config storage rather than from state.
	probeAutomationConfigREST(ctx, out, baseURL, token, target)

	fmt.Print(out.String())
	return nil
}

// probeAutomationConfigREST asks the config panel's own route for an
// automation's stored YAML. Whether this answers an App is the difference
// between get_automation returning the real config and returning only what the
// state machine exposes.
func probeAutomationConfigREST(ctx context.Context, out *report, baseURL, token string, target automationTarget) {
	out.writef("## REST `GET /api/config/automation/config/<id>`\n\n")
	if target.numericID == "" {
		out.writef("SKIPPED — no automation with an `attributes.id` was found.\n\n")
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/api/config/automation/config/"+url.PathEscape(target.numericID), nil)
	if err != nil {
		out.writef("request could not be built: %v\n\n", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	elapsed := time.Since(start).Round(time.Millisecond)
	if err != nil {
		out.writef("TRANSPORT FAILURE after %s: %v\n\n", elapsed, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFrame))
	if err != nil {
		out.writef("HTTP %d after %s, body unreadable: %v\n\n", resp.StatusCode, elapsed, err)
		return
	}

	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		// A refusal here is frequently HTML or a bare message, and its status
		// code is the result — the body itself must not be echoed, it is the
		// owner's automation config.
		out.writef("HTTP %d after %s (%d bytes, not JSON)\n\n", resp.StatusCode, elapsed, len(body))
		return
	}

	out.writef("HTTP %d after %s (%d bytes)\n\n```\n%s```\n\n", resp.StatusCode, elapsed, len(body), renderShape(shapeOf(decoded)))
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

func probeWebSocket(ctx context.Context, out *report, baseURL, token string) (automationTarget, error) {
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/api/websocket"

	// The handshake response is discarded deliberately: coder/websocket
	// documents that its Body never needs closing, so an analyser reporting a
	// leak here is applying a net/http rule this package does not follow.
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return automationTarget{}, fmt.Errorf("dial %s: %w", wsURL, err)
	}
	// Close's error is not actionable: the report is already written and the
	// process is exiting either way.
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
	conn.SetReadLimit(maxFrame)

	if err := authenticate(ctx, conn, token); err != nil {
		return automationTarget{}, err
	}

	out.writef("## WebSocket commands\n\n")

	var nextID uint64
	var target automationTarget
	var runID string

	// The logbook window is generated here, not taken from the installation,
	// so printing it leaks nothing and makes the query reproducible.
	windowStart := time.Now().UTC().Add(-logbookWindow).Format(time.RFC3339)

	for _, p := range probes {
		heading := p.command
		if p.label != "" {
			heading = p.label
		}
		out.writef("### `%s`\n\n", heading)
		if p.note != "" {
			out.writef("_%s_\n\n", p.note)
		}

		payload := map[string]any{"type": p.command}
		for k, v := range p.args {
			payload[k] = v
		}

		notes := describeArgs(p.args)
		skipped := ""
		for _, need := range p.needs {
			switch need {
			case "automation_entity":
				if target.entityID == "" {
					skipped = "no automation entity was found in get_states"
					break
				}
				payload["entity_id"] = target.entityID
				notes = appendNote(notes, "entity_id: <the target automation>")
			case "automation_entity_ids":
				if target.entityID == "" {
					skipped = "no automation entity was found in get_states"
					break
				}
				payload["entity_ids"] = []string{target.entityID}
				notes = appendNote(notes, "entity_ids: [<the target automation>]")
			case "automation_numeric":
				if target.numericID == "" {
					skipped = "the target automation has no attributes.id"
					break
				}
				payload["item_id"] = target.numericID
				notes = appendNote(notes, "item_id: <the target automation's attributes.id>")
			case "trace_run":
				if runID == "" {
					skipped = "trace/list yielded no run_id"
					break
				}
				payload["run_id"] = runID
				notes = appendNote(notes, "run_id: <one id from trace/list>")
			case "window":
				payload["start_time"] = windowStart
				notes = appendNote(notes, fmt.Sprintf("start_time: %s", windowStart))
			}
			if skipped != "" {
				break
			}
		}
		if skipped != "" {
			out.writef("SKIPPED — %s.\n\n", skipped)
			continue
		}
		if notes != "" {
			out.writef("Arguments: `%s`\n\n", notes)
		}

		nextID++
		payload["id"] = nextID

		start := time.Now()
		res, err := call(ctx, conn, nextID, payload)
		elapsed := time.Since(start).Round(time.Millisecond)

		if err != nil {
			out.writef("TRANSPORT FAILURE after %s: %v\n\n", elapsed, err)
			return target, fmt.Errorf("%s: %w", p.command, err)
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

		out.writef("OK after %s (%d bytes)\n\n", elapsed, len(res.Result))

		if p.command == "get_states" {
			target = reportAutomationStates(out, decoded)
			continue
		}

		out.writef("```\n%s```\n\n", renderShape(shapeOf(decoded)))

		reportCurrentUser(out, p.command, decoded)
		if p.command == "trace/list" && runID == "" {
			runID = runIDFor(decoded, target.numericID)
		}
	}

	return target, nil
}

// reportAutomationStates describes only the automation entities in a
// get_states answer. The whole-installation shape would be tens of thousands
// of merged fields and would bury the schema this task is about; the count and
// the presence ratios are what decide whether last_triggered is a usable
// fallback.
func reportAutomationStates(out *report, decoded any) automationTarget {
	states := findAutomations(decoded)
	target, triggered := pickTarget(states)

	out.writef("%d automation entities present. Shape of those states only:\n\n", len(states))
	if len(states) == 0 {
		out.writef("_none — every automation probe below is skipped._\n\n")
		return target
	}
	out.writef("```\n%s```\n\n", renderShape(shapeOf(states)))
	out.writef("Target automation carries `attributes.id`: `%t`; has been triggered before: `%t`.\n\n",
		target.numericID != "", triggered)
	if !triggered {
		out.writef("> No automation on this installation reports `last_triggered`. An empty `trace/list` below therefore means *no runs*, not *traces unavailable*.\n\n")
	}
	return target
}

// appendNote joins argument notes without inventing a separator per call site.
func appendNote(existing, note string) string {
	if existing == "" {
		return note
	}
	return existing + ", " + note
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
