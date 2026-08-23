// Command spike is Phase 00's throwaway verification vehicle: it asks a live
// Home Assistant which read commands actually exist, what they answer with, and
// what they cost on the installation the owner actually runs.
//
// This revision serves P0-07 (F-5): where history and statistics should come
// from — REST /api/history/period under bounded ranges, the WebSocket history
// command, and the recorder statistics API — including which of them is cheap
// enough for a Raspberry Pi-sized recorder DB. The P0-05 automation probe set
// it replaces is already recorded in
// docs/research/2026-08-23-ha-automation-traces.md.
//
// It is deliberately not built on internal/ha's Client. That package has no
// generic "send any command" method and must not grow one: Phase 01's gateway
// allow-list is what decides which commands may ever leave the process, and a
// general sender added before it exists is exactly the escape hatch CLAUDE.md
// rule 2 forbids. The spike dials and authenticates on its own, and is deleted
// when Phase 00 closes.
//
// Output is a markdown report containing field names, types, sizes and
// timings only — never a value from the installation, and never the token.
// See shape.go.
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
	"os"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// A week of full-attribute history for a chatty sensor is the largest answer
// this probe can provoke; this bounds a misbehaving peer without truncating a
// legitimate one.
const maxFrame = 64 << 20

// Deliberately generous: the point of this run is to find out how slow the
// expensive variants are on a Pi, and a timeout tuned for the fast ones would
// report "transport failure" where the honest answer is a measured number.
const requestTimeout = 120 * time.Second

// The two windows P0-07's DoD names. 24h is what a diagnostic tool asks for by
// default; 7d is the widest an interactive investigation is likely to want.
const (
	dayWindow  = 24 * time.Hour
	weekWindow = 7 * 24 * time.Hour
)

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

// measurement is one row of the closing comparison table — the part of this
// report the recommendation is actually derived from.
type measurement struct {
	source  string
	variant string
	window  string
	bytes   int
	cold    time.Duration
	warm    time.Duration
	status  string
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

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	out := &report{}
	out.writef("# P0-07 probe — recorder history & statistics\n\n")
	out.writef("Run at %s (UTC)\n\n", time.Now().UTC().Format(time.RFC3339))
	out.writef("Every query below is bounded to one entity and an explicit window; "+
		"timings are wall clock from this machine, measured twice (cold, then warm) "+
		"to separate a first-query recorder read from a cached one. Windows: %s and %s.\n\n",
		dayWindow, weekWindow)

	if err := probeConfigREST(ctx, out, baseURL, token); err != nil {
		out.writef("REST `GET /api/config` failed: %v\n\n", err)
	}

	conn, err := dial(ctx, baseURL, token)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	ids := &idSeq{}
	target, err := probeTarget(ctx, out, conn, ids)
	if err != nil {
		return err
	}
	if target.entityID == "" {
		out.writef("No entity was found in `get_states`; every probe below is skipped.\n\n")
		fmt.Print(out.String())
		return nil
	}

	// end is fixed once so every window below is measured against the same
	// instant — otherwise the later, larger queries silently cover a slightly
	// different range than the earlier ones.
	end := time.Now().UTC()
	var table []measurement

	red := &redactor{}
	red.add(target.entityID)

	table = append(table, probeHistoryREST(ctx, out, baseURL, token, target, end, red)...)
	table = append(table, probeHistoryWS(ctx, out, conn, ids, target, end, red)...)
	table = append(table, probeStatistics(ctx, out, conn, ids, target, end, red)...)

	writeComparison(out, table)

	fmt.Print(out.String())
	return nil
}

// probeTarget establishes who this run is and which entity the rest of the
// report is about.
func probeTarget(ctx context.Context, out *report, conn *websocket.Conn, ids *idSeq) (historyTarget, error) {
	out.writef("## Run context\n\n")

	user, err := wsCall(ctx, conn, ids, map[string]any{"type": "auth/current_user"})
	if err != nil {
		return historyTarget{}, err
	}
	if m, ok := user.decoded.(map[string]any); ok {
		admin, _ := m["is_admin"].(bool)
		out.writef("Authenticated as an admin user: `%t`\n\n", admin)
	}

	states, err := wsCall(ctx, conn, ids, map[string]any{"type": "get_states"})
	if err != nil {
		return historyTarget{}, err
	}
	list, _ := states.decoded.([]any)
	target := pickHistoryTarget(list)

	out.writef("%d entities present; `get_states` answered %d bytes in %s.\n\n", len(list), states.bytes, states.elapsed)
	out.writef("Target entity is a `sensor.*` with a numeric state: `%t`; it carries `state_class`: `%t`.\n\n",
		strings.HasPrefix(target.entityID, "sensor."), target.hasStateClass)
	if !target.hasStateClass {
		out.writef("> The target has no `state_class`, so the recorder compiles no long-term statistics for it. " +
			"An empty statistics answer below therefore means *nothing recorded for this entity*, not *API unavailable*.\n\n")
	}
	return target, nil
}

// probeHistoryREST answers the two halves of F-5's REST question: what the
// response looks like under each parameter combination, and what each costs.
func probeHistoryREST(ctx context.Context, out *report, baseURL, token string, target historyTarget, end time.Time, red *redactor) []measurement {
	out.writef("## REST `GET /api/history/period`\n\n")

	variants := []historyOpts{
		{},
		{minimalResponse: true},
		{noAttributes: true},
		{minimalResponse: true, noAttributes: true},
	}

	var table []measurement
	for _, w := range []struct {
		name string
		size time.Duration
	}{{"24h", dayWindow}, {"7d", weekWindow}} {
		out.writef("### Window %s\n\n", w.name)
		for _, opts := range variants {
			path := historyPath(target.entityID, end.Add(-w.size), end, opts)
			out.writef("**%s** — `%s`\n\n", opts.label(), red.apply(path))

			first := restCall(ctx, baseURL, token, path)
			second := restCall(ctx, baseURL, token, path)

			m := measurement{source: "REST history", variant: opts.label(), window: w.name,
				bytes: first.bytes, cold: first.elapsed, warm: second.elapsed, status: first.status}
			table = append(table, m)

			out.writef("%s — %d bytes, cold %s, warm %s\n\n", first.status, first.bytes, first.elapsed, second.elapsed)

			// The shape is what distinguishes the parameters; rendering it once
			// per variant on the smaller window is enough, and keeps a week of
			// history from being described four more times.
			if w.name == "24h" && first.decoded != nil {
				out.writef("```\n%s```\n\n", red.apply(renderShape(shapeOf(first.decoded))))
			}
		}
	}
	return table
}

// probeHistoryWS asks the same question of the WebSocket command, which is the
// route an App already holds a connection for.
func probeHistoryWS(ctx context.Context, out *report, conn *websocket.Conn, ids *idSeq, target historyTarget, end time.Time, red *redactor) []measurement {
	out.writef("## WebSocket `history/history_during_period`\n\n")

	var table []measurement
	for _, w := range []struct {
		name string
		size time.Duration
	}{{"24h", dayWindow}, {"7d", weekWindow}} {
		payload := map[string]any{
			"type":                     "history/history_during_period",
			"start_time":               end.Add(-w.size).Format(time.RFC3339),
			"end_time":                 end.Format(time.RFC3339),
			"entity_ids":               []string{target.entityID},
			"minimal_response":         true,
			"no_attributes":            true,
			"significant_changes_only": false,
		}
		out.writef("### Window %s — `minimal_response`, `no_attributes`, all changes\n\n", w.name)
		m, _ := measure(ctx, out, conn, ids, red, "WS history", "minimal+no_attributes", w.name, payload, w.name == "24h")
		table = append(table, m)
	}
	return table
}

// probeStatistics characterises the recorder statistics API — the one §6 of the
// architecture doc warns is documented mainly through change notices.
func probeStatistics(ctx context.Context, out *report, conn *websocket.Conn, ids *idSeq, target historyTarget, end time.Time, red *redactor) []measurement {
	out.writef("## WebSocket recorder statistics\n\n")

	var table []measurement

	out.writef("### `recorder/list_statistic_ids`\n\n")
	listed, decoded := measure(ctx, out, conn, ids, red, "WS statistics", "list_statistic_ids", "-",
		map[string]any{"type": "recorder/list_statistic_ids"}, true)
	table = append(table, listed)

	statID := ""
	if res, ok := decoded.([]any); ok {
		statID = pickStatisticID(res, target.entityID)
	}
	if statID == "" {
		out.writef("No statistic id with compiled data was found; the period queries below are skipped.\n\n")
		return table
	}
	out.writef("Statistic id used below is the history target's own: `%t`\n\n", statID == target.entityID)
	red.add(statID)

	out.writef("### `recorder/get_statistics_metadata`\n\n")
	meta, _ := measure(ctx, out, conn, ids, red, "WS statistics", "get_statistics_metadata", "-",
		map[string]any{"type": "recorder/get_statistics_metadata", "statistic_ids": []string{statID}}, true)
	table = append(table, meta)

	periods := []struct {
		window string
		size   time.Duration
		period string
	}{
		{"24h", dayWindow, "5minute"},
		{"24h", dayWindow, "hour"},
		{"7d", weekWindow, "hour"},
		{"7d", weekWindow, "day"},
	}
	for _, p := range periods {
		out.writef("### `recorder/statistics_during_period` — %s window, `%s` buckets\n\n", p.window, p.period)
		payload := map[string]any{
			"type":          "recorder/statistics_during_period",
			"start_time":    end.Add(-p.size).Format(time.RFC3339),
			"end_time":      end.Format(time.RFC3339),
			"statistic_ids": []string{statID},
			"period":        p.period,
			"types":         []string{"mean", "min", "max", "sum", "state"},
		}
		m, _ := measure(ctx, out, conn, ids, red, "WS statistics", "period="+p.period, p.window, payload, true)
		table = append(table, m)
	}

	// The negative case: types is a comparatively recent parameter, and an
	// adapter must know whether omitting it is still accepted before relying on
	// either form.
	out.writef("### `recorder/statistics_during_period` — 24h, `hour` buckets, `types` omitted\n\n")
	noTypes, _ := measure(ctx, out, conn, ids, red, "WS statistics", "hour, no types", "24h",
		map[string]any{
			"type":          "recorder/statistics_during_period",
			"start_time":    end.Add(-dayWindow).Format(time.RFC3339),
			"end_time":      end.Format(time.RFC3339),
			"statistic_ids": []string{statID},
			"period":        "hour",
		}, false)
	table = append(table, noTypes)

	return table
}

// measure runs one WebSocket command twice and writes its shape, size and both
// timings. The decoded first answer is returned for the two call sites that
// need a value out of it, and is nil unless the command succeeded.
func measure(ctx context.Context, out *report, conn *websocket.Conn, ids *idSeq, red *redactor, source, variant, window string, payload map[string]any, renderShapeToo bool) (measurement, any) {
	first, err := wsCall(ctx, conn, ids, payload)
	if err != nil {
		out.writef("TRANSPORT FAILURE: %v\n\n", err)
		return measurement{source: source, variant: variant, window: window, status: "transport failure"}, nil
	}
	if first.status != "OK" {
		out.writef("%s\n\n", first.status)
		return measurement{source: source, variant: variant, window: window, status: first.status}, nil
	}

	second, err := wsCall(ctx, conn, ids, payload)
	warm := time.Duration(0)
	if err == nil {
		warm = second.elapsed
	}

	out.writef("OK — %d bytes, cold %s, warm %s\n\n", first.bytes, first.elapsed, warm)
	if renderShapeToo && first.decoded != nil {
		out.writef("```\n%s```\n\n", red.apply(renderShape(shapeOf(first.decoded))))
	}
	return measurement{source: source, variant: variant, window: window,
		bytes: first.bytes, cold: first.elapsed, warm: warm, status: "OK"}, first.decoded
}

func writeComparison(out *report, table []measurement) {
	out.writef("## Comparison\n\n")
	out.writef("| source | variant | window | bytes | cold | warm | status |\n")
	out.writef("|---|---|---|--:|--:|--:|---|\n")
	for _, m := range table {
		out.writef("| %s | %s | %s | %d | %s | %s | %s |\n",
			m.source, m.variant, m.window, m.bytes, m.cold, m.warm, m.status)
	}
	out.writef("\n")
}

type callResult struct {
	decoded any
	bytes   int
	elapsed time.Duration
	status  string
}

// idSeq hands out the monotonic ids the WebSocket API requires.
type idSeq struct{ n uint64 }

func (s *idSeq) next() uint64 { s.n++; return s.n }

func wsCall(ctx context.Context, conn *websocket.Conn, ids *idSeq, payload map[string]any) (callResult, error) {
	id := ids.next()
	sent := make(map[string]any, len(payload)+1)
	for k, v := range payload {
		sent[k] = v
	}
	sent["id"] = id

	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	start := time.Now()
	if err := wsjson.Write(reqCtx, conn, sent); err != nil {
		return callResult{}, fmt.Errorf("write: %w", err)
	}

	// Nothing is subscribed here, but HA may still interleave frames; read
	// until the id matches rather than assuming the next frame is the answer.
	for {
		var res result
		if err := wsjson.Read(reqCtx, conn, &res); err != nil {
			return callResult{}, fmt.Errorf("read: %w", err)
		}
		if res.Type != "result" || res.ID != id {
			continue
		}
		elapsed := time.Since(start).Round(time.Millisecond)
		if !res.Success {
			code, msg := "unknown", ""
			if res.Error != nil {
				code, msg = res.Error.Code, res.Error.Message
			}
			return callResult{elapsed: elapsed, status: fmt.Sprintf("FAILED after %s — error code `%s`: %s", elapsed, code, msg)}, nil
		}
		var decoded any
		if err := json.Unmarshal(res.Result, &decoded); err != nil {
			return callResult{elapsed: elapsed, bytes: len(res.Result), status: fmt.Sprintf("OK after %s but result did not decode: %v", elapsed, err)}, nil
		}
		return callResult{decoded: decoded, bytes: len(res.Result), elapsed: elapsed, status: "OK"}, nil
	}
}

func restCall(ctx context.Context, baseURL, token, path string) callResult {
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return callResult{status: fmt.Sprintf("request could not be built: %v", err)}
	}
	req.Header.Set("Authorization", "Bearer "+token)

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return callResult{elapsed: time.Since(start).Round(time.Millisecond), status: fmt.Sprintf("TRANSPORT FAILURE: %v", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxFrame))
	// Timed after the body is drained: a history response streams, so
	// time-to-first-byte would understate what a consumer actually waits for.
	elapsed := time.Since(start).Round(time.Millisecond)
	if readErr != nil {
		return callResult{elapsed: elapsed, status: fmt.Sprintf("HTTP %d, body unreadable: %v", resp.StatusCode, readErr)}
	}

	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		// A refusal is frequently HTML or a bare message. Its status code is
		// the result; the body itself is the owner's data and is not echoed.
		return callResult{bytes: len(body), elapsed: elapsed, status: fmt.Sprintf("HTTP %d (not JSON)", resp.StatusCode)}
	}
	return callResult{decoded: decoded, bytes: len(body), elapsed: elapsed, status: fmt.Sprintf("HTTP %d", resp.StatusCode)}
}

func probeConfigREST(ctx context.Context, out *report, baseURL, token string) error {
	res := restCall(ctx, baseURL, token, "/api/config")
	if res.decoded == nil {
		return fmt.Errorf("%s", res.status)
	}
	out.writef("## REST `GET /api/config`\n\n%s\n\n", res.status)

	// The HA version is the one value the research file is required to carry,
	// and it identifies a release, not the owner.
	if m, ok := res.decoded.(map[string]any); ok {
		if v, ok := m["version"].(string); ok {
			out.writef("**Observed HA version: `%s`**\n\n", v)
		}
	}
	return nil
}

func dial(ctx context.Context, baseURL, token string) (*websocket.Conn, error) {
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/api/websocket"

	// The handshake response is discarded deliberately: coder/websocket
	// documents that its Body never needs closing, so an analyser reporting a
	// leak here is applying a net/http rule this package does not follow.
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", wsURL, err)
	}
	conn.SetReadLimit(maxFrame)

	if err := authenticate(ctx, conn, token); err != nil {
		_ = conn.Close(websocket.StatusInternalError, "")
		return nil, err
	}
	return conn, nil
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
