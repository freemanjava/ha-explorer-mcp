package main

import (
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// arrivalRates are three points spanning the current sustained invocation
// limit (internal/policy.invocationInterval = 500ms, i.e. 2/s): below, at,
// and above it. P2-06's DoD asks for three or more rates spanning the current
// 2/s so the constant's provenance is a measured curve, not one guessed point
// (F-20).
var arrivalRates = []float64{1, 2, 4}

// arrivalSamplesPerRate bounds each rate's run to a fixed call count rather
// than a fixed wall-clock duration, so the report's total runtime is
// predictable regardless of how fast — or slowly, if the recorder is already
// degrading — the answers come back.
const arrivalSamplesPerRate = 20

// arrivalReadTimeout bounds how long the drain loop waits for one answer
// before counting it a failure. Generous like requestTimeout: a slow recorder
// answering late is exactly the degradation this probe exists to find, and a
// tight timeout would misreport it as a transport failure instead.
const arrivalReadTimeout = requestTimeout

// arrivalPercentiles are printed for every rate: the p50 shows the typical
// call, p95/p99 show whether a stream at that rate has a long tail — the
// shape a token-bucket limiter cannot see, since it only counts arrivals.
var arrivalPercentiles = []float64{50, 95, 99}

// arrivalBatchSize and arrivalWindow describe the call this probe streams:
// deliberately NOT maxEntities (200) at the 7d window. The 2026-08-24 run
// measured that exact call at 7.63 MB
// (docs/research/2026-08-24-ha-multi-entity-query-cost.md) — about 15× over
// the 512 KB `QueryBudget` normal-read cap `internal/policy.normalMaxBytes`
// enforces, so `Preflight` would refuse it before it ever reached the
// recorder; no shipped tool can ever issue it. 10 ids over 24h is that same
// run's largest rung that stayed inside both the byte cap (80 656 B) and the
// point cap (2 189 pts, against `normalMaxHistoryPoints` 13 000) — a call
// `get_entity_history` could actually be allowed to make. Measuring what a
// *legitimate* stream of these does is the question the rate limiter exists
// to answer; measuring an over-budget call's cost is Preflight's job, not
// RateLimiter's, and conflates the two (F-20's own framing: the limiter
// bounds arrivals, not what one arrival may spend).
const (
	arrivalBatchSize = 10
	arrivalWindow    = dayWindow
)

// probeArrivalRate answers F-20 / P2-06: what a sustained stream of
// legitimately-budgeted history calls costs the recorder — and, best-effort,
// Core's CPU — at rates spanning the current 2/s sustained limit.
// `invocationBurst`/`invocationInterval` in internal/policy/ratelimit.go are
// derived from a single-call measurement, never from a stream; this is that
// stream.
//
// Within one rate, calls are pipelined deliberately: a paced writer goroutine
// sends on schedule without waiting for the previous answer, because that is
// what a real arrival stream at that rate looks like. A client that waited
// for each reply before sending the next would only ever measure round-trip
// time, never contention — the thing a rate limit exists to bound.
func probeArrivalRate(ctx context.Context, out *report, conn *websocket.Conn, idSeq *idSeq, entityIDs []string) {
	out.writef("## Arrival-rate stream (F-20 / P2-06)\n\n")
	if len(entityIDs) == 0 {
		out.writef("No entity ids were available; skipped.\n\n")
		return
	}
	batch := entityIDs
	if len(batch) > arrivalBatchSize {
		batch = batch[:arrivalBatchSize]
	}
	payload := map[string]any{
		"type":                     "history/history_during_period",
		"start_time":               time.Now().UTC().Add(-arrivalWindow).Format(time.RFC3339),
		"end_time":                 time.Now().UTC().Format(time.RFC3339),
		"entity_ids":               batch,
		"minimal_response":         true,
		"no_attributes":            true,
		"significant_changes_only": false,
	}
	out.writef("Each call: `history/history_during_period`, %d ids, %s window, "+
		"`minimal_response`+`no_attributes` — the largest single call the "+
		"2026-08-24 measurement found still inside the normal-read `QueryBudget` "+
		"(512 KB / 13 000 points), i.e. one a shipped tool could actually issue. "+
		"%d calls per rate.\n\n",
		len(batch), windows[0].name, arrivalSamplesPerRate)

	cpuEntityFound := false
	out.writef("| rate (calls/s) | calls | failures | p50 | p95 | p99 | Core CPU (start→end avg) |\n")
	out.writef("|--:|--:|--:|--:|--:|--:|---|\n")
	for _, rate := range arrivalRates {
		cpuBefore, cpuBeforeOK := sampleCPUPercent(ctx, conn, idSeq)
		latencies, failures := runArrivalRate(ctx, conn, idSeq, payload, rate, arrivalSamplesPerRate)
		cpuAfter, cpuAfterOK := sampleCPUPercent(ctx, conn, idSeq)

		cpuCell := "unsupported (no `sensor.*` with a `cpu`/`processor` name and `%` unit was found)"
		if cpuBeforeOK && cpuAfterOK {
			cpuEntityFound = true
			cpuCell = fmt.Sprintf("%.1f%% → %.1f%%", cpuBefore, cpuAfter)
		}

		p := percentiles(latencies, arrivalPercentiles)
		out.writef("| %.0f | %d | %d | %s | %s | %s | %s |\n",
			rate, arrivalSamplesPerRate, failures, p[0], p[1], p[2], cpuCell)
	}
	out.writef("\n")
	if !cpuEntityFound {
		out.writef("Core CPU was not observable through any entity's state on this " +
			"installation; the recorder-latency columns above are the measurement " +
			"P2-06's DoD can be closed on regardless.\n\n")
	}
}

// runArrivalRate paces arrivalSamplesPerRate writes at the given rate and
// drains the same number of answers, matching each to its send time by
// WebSocket message id. A write failure stops issuing further calls for this
// rate — a broken connection is not a rate the recorder was ever asked to
// sustain — and every call still owed a drained answer counts as a failure.
func runArrivalRate(ctx context.Context, conn *websocket.Conn, idSeq *idSeq, payload map[string]any, rate float64, n int) (latencies []time.Duration, failures int) {
	interval := time.Duration(float64(time.Second) / rate)

	var mu sync.Mutex
	sendTimes := make(map[uint64]time.Time, n)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range n {
			<-ticker.C
			id := idSeq.next()
			frame := make(map[string]any, len(payload)+1)
			maps.Copy(frame, payload)
			frame["id"] = id

			mu.Lock()
			sendTimes[id] = time.Now()
			mu.Unlock()

			if err := wsjson.Write(ctx, conn, frame); err != nil {
				return
			}
		}
	}()

	for range n {
		reqCtx, cancel := context.WithTimeout(ctx, arrivalReadTimeout)
		var res result
		err := wsjson.Read(reqCtx, conn, &res)
		cancel()
		if err != nil {
			failures++
			continue
		}

		mu.Lock()
		start, ok := sendTimes[res.ID]
		delete(sendTimes, res.ID)
		mu.Unlock()
		if !ok || !res.Success {
			failures++
			continue
		}
		latencies = append(latencies, time.Since(start).Round(time.Millisecond))
	}

	slices.Sort(latencies)
	return latencies, failures
}

// percentiles returns one duration per requested percentile using the
// nearest-rank method, formatted as "—" when there is nothing to report.
func percentiles(sorted []time.Duration, ps []float64) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		if len(sorted) == 0 {
			out[i] = "—"
			continue
		}
		idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
		idx = max(0, min(idx, len(sorted)-1))
		out[i] = sorted[idx].String()
	}
	return out
}

// sampleCPUPercent asks get_states once and looks for a `sensor.*` entity
// that plausibly reports Core's CPU load: name containing "cpu" or
// "processor" and a `%` unit. This is a heuristic, not a contract — no HA
// version is guaranteed to expose Core CPU as an entity state at all (the
// authoritative source, Supervisor's `/core/stats`, needs the `homeassistant`
// role this App does not request — docs/research/2026-08-23-supervisor-permissions.md)
// — so a miss is reported as unsupported, never as zero (CLAUDE.md rule 7).
func sampleCPUPercent(ctx context.Context, conn *websocket.Conn, idSeq *idSeq) (float64, bool) {
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	res, err := wsCall(reqCtx, conn, idSeq, map[string]any{"type": "get_states"})
	if err != nil || res.decoded == nil {
		return 0, false
	}
	list, _ := res.decoded.([]any)
	for _, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["entity_id"].(string)
		if !strings.HasPrefix(id, "sensor.") {
			continue
		}
		lower := strings.ToLower(id)
		if !strings.Contains(lower, "cpu") && !strings.Contains(lower, "processor") {
			continue
		}
		attrs, _ := m["attributes"].(map[string]any)
		if unit, _ := attrs["unit_of_measurement"].(string); unit != "%" {
			continue
		}
		state, _ := m["state"].(string)
		v, err := strconv.ParseFloat(state, 64)
		if err != nil {
			continue
		}
		return v, true
	}
	return 0, false
}
