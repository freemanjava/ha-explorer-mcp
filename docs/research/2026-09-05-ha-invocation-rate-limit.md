# Invocation rate limit under a sustained arrival stream — observed

**Task:** `devflow verify` for `P2-06` · resolves **F-20**
**Observed:** 2026-09-05, against a live Home Assistant installation, in two
runs from a **Users**-group, `is_admin: false` token — an initial run at
14:31 UTC and a corrected one at 14:46 UTC (see "First run" below)
**HA version:** `2026.9.0` (reported by REST `GET /api/config`)
**Vehicle:** `cmd/spike` (`probeArrivalRate`, added this session): a paced
writer goroutine sends `history/history_during_period` calls on schedule
without waiting for the previous answer — a real arrival stream, not a
round-trip benchmark — while a draining reader matches each answer to its
send time by WebSocket message id. Three rates (1, 2, 4 calls/s, spanning the
current 2/s sustained limit — `internal/policy.invocationInterval` = 500 ms),
20 calls each. Core CPU sampled best-effort before/after each rate via a
`sensor.*` entity whose name contains `cpu`/`processor` and whose unit is `%`
— the authoritative source, Supervisor's `/core/stats`, needs the
`homeassistant` role this App does not request
([`2026-08-23-supervisor-permissions.md`](2026-08-23-supervisor-permissions.md)).

> Report is field names, types, counts and timings only — no value or id from
> the installation, per `cmd/spike`'s standing convention.

## First run — an out-of-budget payload, corrected before drawing conclusions

The probe's first cut sized "max-page" as `maxEntities` (200) ids over the 7d
window — the widest rung the 2026-08-24 cost-ladder run measured
([`2026-08-24-ha-multi-entity-query-cost.md`](2026-08-24-ha-multi-entity-query-cost.md)).
That run already recorded this exact call at **7.63 MB**, ~15× over the
512 KB `normalMaxBytes` a normal-read `QueryBudget` enforces
(`internal/policy/budget.go`) — `Preflight` would refuse it before it ever
reached the recorder, so no shipped tool can ever actually issue it.

Streamed anyway, it produced a severe, still-climbing backlog: p50 latency
1m33s → 1m36s and p99 2m27s → 2m44s across 1/2/4 calls/s, with sampled Core
CPU climbing 2% → 57% → 70% → 74% and not plateaued by the run's end. This is
kept as context, not as P2-06's answer — it is the shape of what a `Preflight`
bypass would cost under load, evidence that the byte-budget check is
load-bearing, not evidence about the arrival limit itself. The probe was
corrected (10 ids over 24h — see below) and re-run before writing the rest of
this note.

## Found — no measurable degradation up to double the current sustained rate

At 10 ids over 24h — the 2026-08-24 run's largest rung that stayed inside
**both** the byte cap (80,656 B against 512 KB) and the point cap (2,189 pts
against `normalMaxHistoryPoints` 13,000), i.e. a call `get_entity_history`
could actually be allowed to make — the stream shows flat latency and flat
CPU at every rate tested, including 4/s, double the current 2/s sustained
limit:

| rate (calls/s) | calls | failures | p50 | p95 | p99 | Core CPU (start→end avg) |
|--:|--:|--:|--:|--:|--:|---|
| 1 | 20 | 0 | 62 ms | 88 ms | 97 ms | 2.0% → 3.0% |
| 2 | 20 | 0 | 56 ms | 65 ms | 68 ms | 3.0% → 4.0% |
| 4 | 20 | 0 | 54 ms | 64 ms | 73 ms | 4.0% → 4.0% |

Zero failures at any rate. Latency does not grow with rate — if anything the
2/s and 4/s rows are *faster* at the tail than 1/s, consistent with warm
page-cache effects across repeated identical calls rather than any queueing.
Core CPU stays in a 2–4% band throughout, an order of magnitude below the
first run's climb into the 70s. On this Pi, at this installation, a
budget-compliant normal-read call sustained at up to 4/s shows no sign of
strain.

## Means — the constants stand, confirmed rather than derived

`invocationBurst = 10` and `invocationInterval = 500 ms` (2/s sustained) were
derived, not measured, from the 2026-08-24 single-call run (F-20). This run
measured the stream directly and found no degradation at 4/s — double the
current sustained rate — so **the values are confirmed unchanged**, now with
a measured provenance rather than a derivation. The margin (4/s clean vs. 2/s
enforced) is deliberately kept: it is headroom against installations weaker
than the one measured here, not evidence the limit could be safely raised.

## Not established

- **Only one installation was measured**, on one Raspberry Pi generation, with
  491 entities and 135 numeric sensors. A weaker Pi or a much larger
  installation's recorder could behave differently; nothing here bounds that.
- **Rates above 4/s.** The DoD's "three or more rates spanning the current
  2/s" is satisfied, but the ceiling above which an in-budget stream *does*
  degrade was not found — only that it is above 4/s.
- **Sustained duration beyond 20 calls per rate.** A longer stream (minutes,
  not ~5–20 seconds) could reveal slower-building contention this run's short
  windows would not show.
- **Core CPU's precision.** The sampled figure comes from a `sensor.*` state,
  not `/core/stats` — a heuristic match, not a contract (see Vehicle above).
  It moved in the direction and magnitude the recorder-latency numbers would
  predict in both runs, which is corroborating, not confirming.
- **What a stream at the burst size (10 immediate arrivals) does.** This run
  measured sustained rates, not an instantaneous burst; the burst dimension of
  `invocationBurst` remains as it was — sized from the single-call
  measurement's cold-call cost, not from a burst-specific stream.
