# Phase 04 — History, Statistics & Detection

**Milestone:** M1 (roadmap "Phase 1 — Observer") · **Target version:** v0.5

## Goal

Move the expensive, repeatable computation out of the LLM and into deterministic
Go (ADR-009). The agent asks for availability, outage and cadence metrics over a
bounded period and receives a small, exact answer — not ten thousand raw history
points it has to reduce itself, on a Raspberry Pi's recorder budget.

## Depends On

Phase 03 (tool surface) and P0-07 (which history/statistics source to prefer).

## Add Under

```text
internal/analysis/   # availability.go, staleness.go
internal/mcp/        # entity_tools.go (history/statistics tools)
```

## Design Notes

- **Aggregation happens server-side; the LLM receives evidence, not datasets**
  (doc §1). If a tool's honest answer is large, that is a signal the tool is
  under-aggregated, not that the cap should be raised.
- **Statistics are deterministic and unit-tested against fixtures.** Availability
  ratio, outage counts and percentiles must be reproducible from a captured
  history file — this is the layer the whole evidence model's credibility rests on.
- **Bounded by contract, not by convention** (doc §9.1): explicit entity ids, an
  explicit time range, a maximum range, maximum points and a deadline. There is
  no wildcard and no year-long history, and no parameter combination that
  produces one.
- **State semantics matter.** `unavailable` and `unknown` are not the same as a
  missing sample, and a gap in recorder data is not an outage. Conflating them
  manufactures evidence, which is the failure ADR-010 exists to prevent.
- Cache history aggregates a few minutes, keyed by entity + period + resolution
  (doc §16), and report the cache age.

## Tasks

- [x] **`P4-01` — `get_entity_history`** — Appendix A.2: explicit `entity_id`,
  `from`/`to`, optional `resolution`, `minimal: true` by default, using the
  source P0-07 recommended.
  **DoD:** a range exceeding the configured maximum is refused with an explicit
  policy error naming the maximum, not silently clamped; a point count exceeding
  budget returns `ErrBudgetExceeded` with what was retrieved; a slow or timing-out
  recorder query returns `ErrDeadline` and cancels upstream (Appendix B);
  `minimal` demonstrably reduces the response size.

- [x] **`P4-02` — Availability and outage analysis** 🧠 — deterministic
  computation of availability ratio, unavailable-period count, total and longest
  outage, over a bounded window.
  **DoD:** fixture-driven tests producing the exact numbers in doc §12.1's
  example shape; explicit, asserted handling of — an entity unavailable at the
  window's start (partial leading outage); one still unavailable at its end
  (open-ended outage); a recorder gap distinguished from an outage; an entity
  with zero state changes; a window shorter than one update interval.

- [x] **`P4-03` — Update-cadence and staleness analysis** 🧠 — median and p95
  update intervals, state-change rate, and a staleness judgement relative to the
  entity's own observed cadence rather than a global constant.
  **DoD:** tests over regular, irregular and bursty fixtures; an entity that
  legitimately updates hourly is not flagged stale while one that normally
  updates every 30s and has been silent for an hour is; percentile computation is
  tested at small sample sizes where naive implementations misbehave.

- [ ] **`P4-04` — `get_entity_statistics`** — expose P4-02 and P4-03 as one tool
  with a bounded `period` parameter, provenance and cache age.
  **DoD:** response matches the doc §12.1 shape; period is validated against the
  maximum; the source (recorder history vs statistics API) is named in the
  response so a reader knows what the numbers came from.

- [ ] **`P4-05` — `find_unavailable_entities` and `find_stale_entities`** —
  installation-wide detection with filters and pagination.
  **DoD:** entity counts and ranking are computed server-side; both respect the
  entity budget and return an explicit `truncated` marker rather than an
  arbitrary subset presented as complete; a `PRIVATE` entity is included or
  excluded per the Phase 02 profile, and the response says which happened, so
  absence is never mistaken for health.

## Decisions

*(the source-selection question is answered by P0-07's evidence, and the budget
defaults by P2-01. Below: real design forks hit during implementation.)*

**`P4-01`, 2026-09-05 — `resolution` deferred, not implemented.** Appendix A.2
lists `resolution?: string` on `get_entity_history`, but the phase's own DoD
for this task names only range/point/byte/deadline refusal and `minimal`'s
size effect — nothing exercises a resolution value. `get_entity_history`'s
only source is `history/history_during_period`, which has no bucket/period
parameter; a resolution coarser than raw points is a statistics-API concern,
which `P4-04` (`get_entity_statistics`) owns. Implementing a `resolution` field
here would mean silently switching sources mid-tool, which doc §9's two-tool
split (raw history vs. computed statistics) does not intend. The field is
left off `GetEntityHistoryInput` entirely rather than accepted and ignored
(CLAUDE.md rule 7: an accepted-but-ignored parameter is a subtler way to
fabricate an answer than a missing one). Revisit if `P4-04` finds a use for a
shared vocabulary.

**`P4-01`, 2026-09-05 — a fixed 7-day range cap, not a configurable one.**
The DoD wants "a range exceeding the configured maximum" refused explicitly.
No prior task set a maximum time span (only point/byte/entity/request budgets
exist in `internal/policy`), and doc §10 names no number. `internal/mcp`'s new
`maxHistoryWindow` (7 days) is pinned to the widest window the 2026-08-24
multi-entity measurement actually exercised
(`docs/research/2026-08-24-ha-multi-entity-query-cost.md`) — evidence-backed
rather than a guess, matching §26's rule, but a constant rather than an
environment-configurable limit like the query budget, since nothing yet
needs it to vary per deployment. Revisit as a real `policy.Limits` field if a
later task does.

**`P4-01`, 2026-09-05 — no `Unsupported` branch.** Unlike `get_automation`/
`get_automation_traces` (P3-07), `history/history_during_period` is not
documented as admin-gated or compatibility-sensitive (doc §18), and P0-07's
probe did not observe a permission or version-shaped refusal. A real failure
(bad shape, upstream error) is returned as a Go error, not silently degraded
to "unsupported" — CLAUDE.md rule 7 forbids inventing that answer without
evidence it can happen. Revisit if a future HA release, or a non-admin
principal, is observed refusing this command.

**`P4-02`, 2026-09-05 — `unknown` is not available, but stays countable.**
The phase's own design note warns that conflating states manufactures
evidence, and `unavailable` (the integration cannot reach it) is genuinely
not `unknown` (reachable, no value). The rest of the catalog nevertheless
already lumps both into "not available" — `internal/ha`'s entity-state
aggregate and `internal/mcp`'s `availability` filter (P3-05) — and a window
metric that disagreed with the point-in-time one would let two tools
contradict each other about the same entity. So both count against
`AvailabilityRatio`, a contiguous run mixing them is **one** outage rather
than two (splitting on flavour would inflate `unavailable_periods`, doc
§12.1's own field), and the distinction is preserved instead as
`AvailabilityReport.UnknownDuration` — how much of the outage total was
merely value-less. Nothing is lost and nothing is invented.

**`P4-02`, 2026-09-05 — a recorder gap reduces coverage; it is never an
outage, and never a zero.** From one entity's state changes alone, the only
observable gap is a *leading* one: no recorded state at or before the
window's start. A gap in the middle is indistinguishable from a state that
simply held, so it is not guessed at. The leading gap is reported
(`CoverageGap`, `CoveredFrom`, `CoverageComplete`) and the ratio is computed
over `Covered`, not over the requested window — charging unobserved time as
downtime would be exactly the fabrication CLAUDE.md rule 7 forbids. When
nothing at all was recorded, `Computable` is false and the ratio stays 0.0
as a zero value that no caller may read: "could not check" is not "0%
available". `P4-04` must surface `Computable` rather than the bare ratio.

**`P4-02`, 2026-09-05 — the fixture reproduces doc §12.1's outage numbers,
not its ratio.** `test/fixtures/entity_history_7d.json` is a captured-shape
7-day `history/history_during_period` payload (minimal `s`/`lu` encoding,
413 points) built to hit the doc's example exactly: 412 state changes, 7
unavailable periods, 3h12m total, 54m longest. Its availability ratio comes
out 0.98095, not the doc's 0.982 — 11520s of 604800s is 1.905% down, so
§12.1's own two numbers are not consistent with each other. The example is
illustrative prose; the computation is the contract, and the test asserts
the computed value with that noted.

**`P4-02`, 2026-09-05 — result types live in `internal/analysis`.**
`AvailabilityReport` and `Outage` are computed products, not normalized HA
domain types, so they stay with the code that computes them rather than
swelling `internal/model`. `internal/analysis` still imports only
`internal/model` in non-test code, asserted by `deps_test.go`; the tests
import `internal/ha` deliberately, to read the fixture through the real
mapper so the metrics are reproducible from a real payload. Revisit at
`P4-04`, which has to join this report with `P4-03`'s into one tool
response and may want a shared type in `model`.

**`P4-03`, 2026-09-05 — staleness is judged against p95, times three, with no
absolute floor.** The task's own wording rules out a global constant, so the
threshold is `staleIntervalFactor × P95UpdateInterval`, an entity-relative
number. p95 rather than the median because a bursty entity's median is one
second while its normal quiet stretch is hours — judging against the median
would call every motion sensor stale between bursts. A minimum absolute
threshold ("nothing under five minutes of silence is ever stale") was
considered and rejected: it is exactly the global constant the DoD excludes,
no evidence sets its value (doc §26), and 3× a genuinely observed p95 is
already an anomaly for a fast entity. `staleIntervalFactor = 3` is a starting
default; `P4-05` is the first task that can measure its false-positive rate on
a real installation, and the constant's comment says so.

**`P4-03`, 2026-09-05 — nearest-rank percentiles, so every reported interval
is one the entity actually exhibited.** For an even sample the classical
median averages the two central values, producing a number that never
occurred; over durations that is a small fabrication of the kind rule 7
forbids, and it buys nothing here — these are diagnostic evidence, not a
statistical estimate. One `nearestRank` function serves both the median and
p95 (`ceil(p×n)`, clamped to `[1, n]`), which is defined at every n ≥ 1 and
avoids the two ways the naive `int(p*len)` form breaks at small samples:
index −1, and index == n at p = 1. Asserted at n = 1..4.

**`P4-03`, 2026-09-05 — an update is not a state change, and both are
reported.** Doc §11 lists "update cadence" and "state-change rate" as separate
health signals, so `CadenceReport` measures intervals between *recorded
instants* (a re-record of the same value is still an update) while
`StateChanges` reuses `segmentsIn`'s collapsed segments and covered span —
the same basis as `AvailabilityReport.StateChanges`, so the two halves of doc
§12.1 cannot report different change counts for one window. Two records at the
same instant are dropped rather than admitted as a zero-length interval, which
would drag every percentile toward nothing.

**`P4-03`, 2026-09-05 — silence is measured from the last known update, even
one from before the window.** An entity whose last record predates the window
is precisely what staleness exists to catch, so `LastUpdate` considers the
carried-in state and `SilentFor` can exceed the window. That same leading
point also contributes the first observed interval, which is often the only
one a window narrower than the entity's cadence can show. It is still not
counted in `Updates`: it happened before the window. With no interval
observable at all, `Computable` and `StaleJudgeable` are false and the
percentiles stay zero values no caller may read — "could not tell" is not
"not stale", the same rule `P4-02` applied to its ratio.

## Phase Definition of Done

- Availability, outage and cadence metrics are reproducible from fixtures and
  match hand-computed values.
- No history path can be induced to exceed its range, point or byte bounds by any
  parameter combination — asserted by test.
- A recorder timeout degrades to an explicit error with partial results, never a
  hang and never a fabricated series.
- v1 acceptance criteria (doc §21) that concern inventory, history and metrics
  are demonstrably met.
- `make check` is green.
