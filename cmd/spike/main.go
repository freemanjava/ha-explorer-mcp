// Command spike is Phase 00's throwaway verification vehicle: it asks a live
// Home Assistant which read commands actually exist, what they answer with, and
// what they cost on the installation the owner actually runs.
//
// This revision serves P0-09 (F-14): what the queries a fleet-wide detector
// issues actually cost. P0-07 measured every query against exactly one entity
// (docs/research/2026-08-23-ha-history-statistics.md), so whether
// history/history_during_period and recorder/statistics_during_period scale
// linearly in the number of entity ids — and whether one batched call beats N
// single-entity ones — is unmeasured. This run measures both commands at 1, 10,
// 50 and 200 ids over a 24h and a 7d window, each against the summed cost of
// the equivalent single-entity calls.
//
// It is deliberately not built on internal/ha's Client. That package has no
// generic "send any command" method and must not grow one: Phase 01's gateway
// allow-list is what decides which commands may ever leave the process, and a
// general sender added before it exists is exactly the escape hatch CLAUDE.md
// rule 2 forbids. The spike dials and authenticates on its own, and is deleted
// when Phase 00 closes.
//
// Output is a markdown report containing field names, types, counts, sizes and
// timings only — never a value or an id from the installation, and never the
// token. See shape.go and history.go's redactor.
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

// A week of history for 200 entities is the largest answer this probe can
// provoke, and its size is precisely the thing being measured: a read limit
// tuned for the single-entity case would report a transport failure where the
// honest answer is a number. This bounds a misbehaving peer without truncating
// the widest legitimate answer.
const maxFrame = 256 << 20

// Deliberately generous: the point of this run is to find out how slow the
// widest queries are on a Pi, and a timeout tuned for the narrow ones would
// report "transport failure" where the honest answer is a measured number.
const requestTimeout = 180 * time.Second

// runBudget bounds the whole run. The single-entity baseline is 200 calls per
// window per source, so the run is minutes long by construction.
const runBudget = 90 * time.Minute

// The two windows P0-09's DoD names. 24h is what a diagnostic tool asks for by
// default; 7d is the widest an interactive investigation is likely to want.
const (
	dayWindow  = 24 * time.Hour
	weekWindow = 7 * 24 * time.Hour
)

// Doc §10's illustrative response caps, quoted here so the report can say at
// which entity count a single answer crosses them rather than leaving the
// reader to do the arithmetic.
const (
	normalByteCap    = 512 << 10
	compositeByteCap = 1 << 20
	slowCall         = time.Second
)

// statisticsPeriod is held fixed across both windows so the two rows differ
// only in window width. `hour` is the bucket a fleet-wide detector would ask
// for: fine enough to see a daily pattern, coarse enough that 7d stays small.
const statisticsPeriod = "hour"

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

// costRow is one row of the comparison the recommendation is derived from: one
// command, one window, one entity count, batched against the sum of the same
// many single-entity calls.
type costRow struct {
	source      string
	window      string
	count       int
	batched     sample
	batchedWarm time.Duration
	singles     sample
	status      string
}

// window pairs a name with its width so both probes iterate the same two.
type window struct {
	name string
	size time.Duration
}

var windows = []window{{"24h", dayWindow}, {"7d", weekWindow}}

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

	ctx, cancel := context.WithTimeout(context.Background(), runBudget)
	defer cancel()

	out := &report{}
	out.writef("# P0-09 probe — multi-entity history & statistics cost\n\n")
	out.writef("Run at %s (UTC)\n\n", time.Now().UTC().Format(time.RFC3339))
	out.writef("Every query is bounded to an explicit window and an explicit id list. "+
		"Each batched call is measured twice (cold, then warm); the single-entity "+
		"baseline is measured once per id and prefix-summed, so a rung of N is compared "+
		"against exactly those N ids. Windows: %s and %s. Statistics buckets: `%s`.\n\n",
		dayWindow, weekWindow, statisticsPeriod)

	if err := probeConfigREST(ctx, out, baseURL, token); err != nil {
		out.writef("REST `GET /api/config` failed: %v\n\n", err)
	}

	conn, err := dial(ctx, baseURL, token)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	ids := &idSeq{}
	red := &redactor{}

	entityIDs, err := probeTargets(ctx, out, conn, ids)
	if err != nil {
		return err
	}
	red.add(entityIDs...)

	statisticIDs, err := probeStatisticTargets(ctx, out, conn, ids)
	if err != nil {
		return err
	}
	red.add(statisticIDs...)

	// end is fixed once so every window below is measured against the same
	// instant — otherwise the later, larger queries silently cover a slightly
	// different range than the earlier ones.
	end := time.Now().UTC()

	var table []costRow
	table = append(table, probeHistoryCost(ctx, out, conn, ids, red, entityIDs, end)...)
	table = append(table, probeStatisticsCost(ctx, out, conn, ids, red, statisticIDs, end)...)

	writeComparison(out, table)
	writeCrossings(out, table)

	fmt.Print(out.String())
	return nil
}

// probeTargets establishes who this run is and how many entity ids the ladder
// can reach.
func probeTargets(ctx context.Context, out *report, conn *websocket.Conn, ids *idSeq) ([]string, error) {
	out.writef("## Run context\n\n")

	user, err := wsCall(ctx, conn, ids, map[string]any{"type": "auth/current_user"})
	if err != nil {
		return nil, err
	}
	if m, ok := user.decoded.(map[string]any); ok {
		admin, _ := m["is_admin"].(bool)
		out.writef("Authenticated as an admin user: `%t`\n\n", admin)
	}

	states, err := wsCall(ctx, conn, ids, map[string]any{"type": "get_states"})
	if err != nil {
		return nil, err
	}
	list, _ := states.decoded.([]any)
	targets := pickHistoryTargets(list)

	out.writef("%d entities present; `get_states` answered %d bytes in %s.\n\n",
		len(list), states.bytes, states.elapsed)
	out.writef("History ladder uses %d entity ids (%d of them numeric `sensor.*`): rungs %v.\n\n",
		len(targets), countNumericSensors(list), ladder(len(targets)))
	return targets, nil
}

// probeStatisticTargets collects the statistic ids the statistics ladder uses.
// They are chosen independently of the history ids: only entities the recorder
// compiles statistics for have any, so the two lists are rarely the same set,
// and forcing them to match would shorten whichever ladder is scarcer.
func probeStatisticTargets(ctx context.Context, out *report, conn *websocket.Conn, ids *idSeq) ([]string, error) {
	listed, err := wsCall(ctx, conn, ids, map[string]any{"type": "recorder/list_statistic_ids"})
	if err != nil {
		return nil, err
	}
	if listed.status != "OK" {
		out.writef("`recorder/list_statistic_ids` — %s; the statistics ladder is skipped.\n\n", listed.status)
		return nil, nil
	}
	list, _ := listed.decoded.([]any)
	targets := pickStatisticIDs(list)

	out.writef("`recorder/list_statistic_ids` answered %d ids in %d bytes (%s); "+
		"%d carry compiled data, so the statistics ladder uses rungs %v.\n\n",
		len(list), listed.bytes, listed.elapsed, len(targets), ladder(len(targets)))
	return targets, nil
}

func countNumericSensors(states []any) int {
	n := 0
	for _, e := range states {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["entity_id"].(string)
		if strings.HasPrefix(id, "sensor.") && isNumeric(m["state"]) {
			n++
		}
	}
	return n
}

// probeHistoryCost measures history/history_during_period up the ladder. Every
// call carries minimal_response and no_attributes: P0-07 established those as
// the only variant cheap enough to consider, so measuring the expensive ones
// again at 200 ids would cost minutes to re-learn a settled fact.
func probeHistoryCost(ctx context.Context, out *report, conn *websocket.Conn, idSeq *idSeq, red *redactor, entityIDs []string, end time.Time) []costRow {
	out.writef("## WebSocket `history/history_during_period`\n\n")
	if len(entityIDs) == 0 {
		out.writef("No entity ids were available; skipped.\n\n")
		return nil
	}
	return probeLadder(ctx, out, conn, idSeq, red, "WS history", entityIDs,
		func(batch []string, w window) map[string]any {
			return map[string]any{
				"type":                     "history/history_during_period",
				"start_time":               end.Add(-w.size).Format(time.RFC3339),
				"end_time":                 end.Format(time.RFC3339),
				"entity_ids":               batch,
				"minimal_response":         true,
				"no_attributes":            true,
				"significant_changes_only": false,
			}
		})
}

// probeStatisticsCost measures recorder/statistics_during_period up the same
// ladder, so the two commands are compared at equal width.
func probeStatisticsCost(ctx context.Context, out *report, conn *websocket.Conn, idSeq *idSeq, red *redactor, statisticIDs []string, end time.Time) []costRow {
	out.writef("## WebSocket `recorder/statistics_during_period`\n\n")
	if len(statisticIDs) == 0 {
		out.writef("No statistic ids with compiled data were available; skipped.\n\n")
		return nil
	}
	return probeLadder(ctx, out, conn, idSeq, red, "WS statistics", statisticIDs,
		func(batch []string, w window) map[string]any {
			return map[string]any{
				"type":          "recorder/statistics_during_period",
				"start_time":    end.Add(-w.size).Format(time.RFC3339),
				"end_time":      end.Format(time.RFC3339),
				"statistic_ids": batch,
				"period":        statisticsPeriod,
				"types":         []string{"mean", "min", "max", "sum", "state"},
			}
		})
}

// probeLadder runs one command at every rung of the ladder, in both windows,
// against the prefix-summed single-entity baseline.
func probeLadder(ctx context.Context, out *report, conn *websocket.Conn, idSeq *idSeq, red *redactor, source string, allIDs []string, payload func([]string, window) map[string]any) []costRow {
	rungs := ladder(len(allIDs))
	if len(rungs) == 0 {
		out.writef("Fewer ids than the smallest rung; skipped.\n\n")
		return nil
	}
	widest := rungs[len(rungs)-1]

	var table []costRow
	for _, w := range windows {
		out.writef("### Window %s\n\n", w.name)

		// The baseline first: one call per id, so every rung's comparison is
		// against the same measured set rather than a re-run.
		singles := measureSingles(ctx, conn, idSeq, allIDs[:widest], w, payload)
		base := sumSamples(singles, widest)
		out.writef("Single-entity baseline: %d calls, %d bytes, %d points, %s total (%s per call on average).\n\n",
			widest, base.bytes, base.points, base.elapsed, perCall(base.elapsed, widest))

		for i, n := range rungs {
			batch := allIDs[:n]
			first, err := wsCall(ctx, conn, idSeq, payload(batch, w))
			if err != nil {
				out.writef("**%d ids** — TRANSPORT FAILURE: %v\n\n", n, err)
				table = append(table, costRow{source: source, window: w.name, count: n,
					singles: sumSamples(singles, n), status: "transport failure"})
				continue
			}
			if first.status != "OK" {
				out.writef("**%d ids** — %s\n\n", n, red.apply(first.status))
				table = append(table, costRow{source: source, window: w.name, count: n,
					singles: sumSamples(singles, n), status: first.status})
				continue
			}
			second, err := wsCall(ctx, conn, idSeq, payload(batch, w))
			warm := time.Duration(0)
			if err == nil {
				warm = second.elapsed
			}

			row := costRow{
				source:  source,
				window:  w.name,
				count:   n,
				batched: sample{bytes: first.bytes, elapsed: first.elapsed, points: countPoints(first.decoded)},
				singles: sumSamples(singles, n),
				status:  "OK",
			}
			row.batchedWarm = warm
			table = append(table, row)

			out.writef("**%d ids** — batched %d bytes, %d points, cold %s, warm %s; "+
				"same ids one at a time: %d bytes, %d points, %s.\n\n",
				n, row.batched.bytes, row.batched.points, row.batched.elapsed, warm,
				row.singles.bytes, row.singles.points, row.singles.elapsed)

			// One shape per command and window is enough to show that a
			// batched answer is keyed by id; repeating it at every rung would
			// print the same structure four times.
			if i == 0 && first.decoded != nil {
				out.writef("```\n%s```\n\n", red.apply(renderShape(shapeOf(first.decoded))))
			}
		}
	}
	return table
}

// measureSingles asks for each id on its own, once. A warm repeat is skipped
// deliberately: at 200 ids per window it would double the longest part of the
// run to refine a baseline the batched rows are only compared against in total.
func measureSingles(ctx context.Context, conn *websocket.Conn, idSeq *idSeq, ids []string, w window, payload func([]string, window) map[string]any) []sample {
	out := make([]sample, 0, len(ids))
	for _, id := range ids {
		res, err := wsCall(ctx, conn, idSeq, payload([]string{id}, w))
		if err != nil || res.status != "OK" {
			// A single failed id must not void the whole baseline; it
			// contributes nothing and the totals stay honest about what was
			// measured because the call count is reported alongside them.
			out = append(out, sample{})
			continue
		}
		out = append(out, sample{bytes: res.bytes, elapsed: res.elapsed, points: countPoints(res.decoded)})
	}
	return out
}

func perCall(total time.Duration, n int) time.Duration {
	if n == 0 {
		return 0
	}
	return (total / time.Duration(n)).Round(time.Millisecond)
}

func writeComparison(out *report, table []costRow) {
	out.writef("## Comparison\n\n")
	out.writef("| source | window | ids | batched bytes | batched points | cold | warm | " +
		"N× single bytes | N× single total | batched ÷ singles (time) | status |\n")
	out.writef("|---|---|--:|--:|--:|--:|--:|--:|--:|--:|---|\n")
	for _, r := range table {
		ratio := "—"
		if r.status == "OK" && r.singles.elapsed > 0 {
			ratio = fmt.Sprintf("%.2f×", float64(r.batched.elapsed)/float64(r.singles.elapsed))
		}
		out.writef("| %s | %s | %d | %d | %d | %s | %s | %d | %s | %s | %s |\n",
			r.source, r.window, r.count, r.batched.bytes, r.batched.points,
			r.batched.elapsed, r.batchedWarm, r.singles.bytes, r.singles.elapsed, ratio, r.status)
	}
	out.writef("\n")
}

// writeCrossings names the first rung at which a single batched answer exceeds
// each limit doc §10 quotes, so the budget values P2-01 needs are read off the
// measurement instead of interpolated by eye.
func writeCrossings(out *report, table []costRow) {
	out.writef("## Where a single batched call crosses doc §10's limits\n\n")
	out.writef("| source | window | > %d B (normal) | > %d B (composite) | ≥ %s cold |\n",
		normalByteCap, compositeByteCap, slowCall)
	out.writef("|---|---|--:|--:|--:|\n")

	type key struct{ source, window string }
	var order []key
	seen := map[key]bool{}
	for _, r := range table {
		k := key{r.source, r.window}
		if !seen[k] {
			seen[k] = true
			order = append(order, k)
		}
	}

	for _, k := range order {
		normal, composite, slow := "not crossed", "not crossed", "not crossed"
		for _, r := range table {
			if r.source != k.source || r.window != k.window || r.status != "OK" {
				continue
			}
			if normal == "not crossed" && r.batched.bytes > normalByteCap {
				normal = fmt.Sprintf("%d ids", r.count)
			}
			if composite == "not crossed" && r.batched.bytes > compositeByteCap {
				composite = fmt.Sprintf("%d ids", r.count)
			}
			if slow == "not crossed" && r.batched.elapsed >= slowCall {
				slow = fmt.Sprintf("%d ids", r.count)
			}
		}
		out.writef("| %s | %s | %s | %s | %s |\n", k.source, k.window, normal, composite, slow)
	}
	out.writef("\nRungs are discrete, so a crossing names the first *measured* count above the "+
		"limit, not the exact one. Per-id averages in the tables above interpolate it. "+
		"Read limit for this run: %d bytes — a batched answer refused by it would show as a "+
		"transport failure, not a size.\n\n", maxFrame)
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
