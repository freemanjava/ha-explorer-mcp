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

- [ ] **`P4-01` — `get_entity_history`** — Appendix A.2: explicit `entity_id`,
  `from`/`to`, optional `resolution`, `minimal: true` by default, using the
  source P0-07 recommended.
  **DoD:** a range exceeding the configured maximum is refused with an explicit
  policy error naming the maximum, not silently clamped; a point count exceeding
  budget returns `ErrBudgetExceeded` with what was retrieved; a slow or timing-out
  recorder query returns `ErrDeadline` and cancels upstream (Appendix B);
  `minimal` demonstrably reduces the response size.

- [ ] **`P4-02` — Availability and outage analysis** 🧠 — deterministic
  computation of availability ratio, unavailable-period count, total and longest
  outage, over a bounded window.
  **DoD:** fixture-driven tests producing the exact numbers in doc §12.1's
  example shape; explicit, asserted handling of — an entity unavailable at the
  window's start (partial leading outage); one still unavailable at its end
  (open-ended outage); a recorder gap distinguished from an outage; an entity
  with zero state changes; a window shorter than one update interval.

- [ ] **`P4-03` — Update-cadence and staleness analysis** 🧠 — median and p95
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

*(none open — the source-selection question is answered by P0-07's evidence, and
the budget defaults by P2-01. Record here anything that turns out to be a real
design fork during implementation.)*

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
