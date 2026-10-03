# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P5-05` — `analyze_entity_health` · phase 05 · default model.
Compose P4-02 availability, P4-03 cadence, registry/device context,
integration setup state and repairs into the Appendix A.3 shape: `Evidence`,
ranked `Hypothesis` via `ConfidenceFor`, `MissingEvidence`. No `score`
(D-05-4). Replaces its `bindNotImplemented` catalog row.

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P5-05` | `analyze_entity_health` | 05 | default | |
| 2 | `P5-06` | `analyze_integration_health` | 05 | default | `blocked:P5-05` |
| 3 | `P5-07` | investigation 1 — doc §13.1 e2e + degraded branch | 05 | default | `blocked:P5-05` |
| 4 | `P5-08` | investigation 2 — doc §13.2 e2e (F-27's rule now in its DoD) | 05 | default | `blocked:P5-06` |
| 5 | `P5-09` | investigation 3 — correlated mass unavailability (observe F-28 first) | 05 | default | `blocked:P5-06` |
| 6 | `P5-10` | measure composite budget, re-class `find_stale_entities` (F-26) | 05 | default | `needs-verify` `blocked:P5-06` |

**Ordering rationale (2026-09-05 `plan`).** Verify → model → analysis
primitives → tools → workflows → measurement. `P5-01` went first because its
answer was structural; it closed 2026-09-05, so `P5-04`/`P5-06` no longer wait
on it. `P5-10` is last because measuring composite cost needs the composite
tools to exist.

**Five design decisions now govern this phase's boxes**, D-05-1…5 in the phase
file, so implementation follows a spec rather than making judgment calls:
fact/inference/recommendation are **separate types**, not fields on one struct ·
**confidence comes from one `ConfidenceFor` function** or does not exist ·
outage clusters are **overlap-with-tolerance, annotated afterwards**, never a
correlation coefficient · **no health score in v1** · mesh metrics are read by
a **flat analyzer over a name/`device_class` hint table**, never a
per-integration plugin seam (D-05-5, from `P5-01`). Rationale and rejected
alternatives in the phase file.

**F-27's annotation half is settled (`P5-04`).** A shared `via_device` that
every other device of its config entry names (the Zigbee coordinator star) is
listed in a cluster's `Withheld`, not `Shared`; a parent of part of its entry
is named. `P5-08`'s DoD now carries the rule; F-27 closes with `P5-08`, which
also owns its second question (is a real neighbour table reachable at all).

**Earlier decisions, still standing.** *Transport:* stdio only (phase 01).
*Supervisor:* `hassio_api: true` at the default role (phase 00). *Catalog:* the
full twenty before release (phase 03). *HA versions:* current release only
(phase 00). `P4-05`: a PRIVATE entity is excluded outright from both `find_*`
tools under the deny profile, never masked.

**No decision is open.** Phase 02's Q10 closed 2026-10-03: **memory-only in
v1** (owner), because `P5-04` — the expected trigger — needed no store of its
own. Reopens only on a diagnostic memory-only demonstrably cannot deliver.

`cmd/spike` is the probe vehicle `P5-10` reuses: `HA_URL` + `HA_TOKEN`, it
reports field names and types only. The owner runs it and pastes the report; no
HA token reaches the agent (owner's choice, 2026-08-23). `P5-01` added
`probeMesh` to it, which now also reports the **distinct** `via_device_id`
count per domain — the one measurement that would confirm F-27's star on the
owner's installation, free on the next run.

## Status

<!-- DERIVED — do not hand-edit. Regenerate:
for f in docs/development/phases/*.md; do
  printf '%s %s/%s\n' "$(basename "$f")" \
    "$(grep -c '^- \[x\]' "$f")" "$(grep -c '^- \[[ x]\]' "$f")"
done
-->

| phase | theme | done / total |
|------:|-------|:------------:|
| 00 | Spike & Foundations | 15 / 15 |
| 01 | HA Access & Read-Only Gateway | 10 / 10 |
| 02 | Policy, Privacy, Budget & Audit | 8 / 8 |
| 03 | MCP Server & Inventory Tools | 11 / 11 |
| 04 | History, Statistics & Detection | 6 / 6 |
| 05 | Diagnostics & Evidence Engine | 9 / 15 |
| 06 | Proposal Mode — gated | 0 / 1 |
| 07 | Controlled Change (Admin) — gated | 0 / 1 |

Counts include each phase's decision entries, which are boxes too. Phase 05's
9 ticked are D-05-1…5 and the `P5-01`…`P5-04` task boxes; its six remaining task boxes
are open, and no decision entry in the phase is open any more. Phase 02 is
complete: its last box, the Q10 persistence decision, closed 2026-10-03.

Phases 00–04 are milestone M1 (v1 observer) and are **fully implemented**.
Phase 05 is M2, and is where the last two catalog rows
(`analyze_entity_health`, `analyze_integration_health` — today bound to
`bindNotImplemented`) become real. Phases 06–07 are gated: they open only on an
explicit owner decision plus a fresh security review, and carry no task boxes.

Last refreshed: 2026-10-03 (`P5-04` closed — outage clustering, F-27 settled)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 3 · `defer` 2 · `unknown` 2 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

Three `queue-next`, all attached to boxes already in the queue and closing
when those close: **F-26** → `P5-10`; **F-27** → `P5-08` (its `P5-04` half
settled); and the new **F-28** (`unknown`) — long outages chain unrelated ones
into one cluster; observe on `P5-10`'s live run, before `P5-09` asserts on a
mass-outage cluster. Two `defer`s remain, both on
the same unfired trigger — the first production
`Preflight(policy.SourceStatistics, …)` call site: **F-17** (batched
statistics ~30% larger) and **F-25** (three allow-listed recorder commands
nothing calls). No Phase 05 box creates that call site, so a standing decision
stands in place of a sixth deferral: **if Phase 05 closes with still no such
call site, F-17 becomes `wont-fix` and F-25 becomes a deletion task, at that
`plan`.** The open `unknown`s are F-17 and F-28.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-10-03 · `P5-04` — `ClusterOutages`: sort + one sweep, 2-min tolerance,
  ≥2-entity clusters annotated afterwards, each a citable `Evidence`. F-27: a
  star parent goes to `Withheld`, never `Shared`. Whole-period outages leave
  the sweep (`UnavailableThroughout`). Found: partial long outages still chain
  (F-28).

- 2026-10-03 · `P5-03` — `ConfidenceFor(cited ...model.Evidence)`: ladder
  high ≥20 samples & ≥0.9 coverage, medium ≥5 & ≥0.5, `Degraded` demotes one
  step, several citations take the weakest. A source scan forbids naming a
  confidence level anywhere but `confidence.go`/`model/evidence.go`.
- 2026-10-03 · `P5-02` — D-05-1's four types plus `HealthAnalysis` in
  `internal/model/evidence.go`; `NewHypothesis` refuses zero citations;
  `MissingReason` separates `entity_disabled` from `not_exposed` (D-05-5). The
  no-`cause` rule is a `go/parser` scan over `internal/`, json tags included.
- 2026-09-05 · `P5-01` — Q9/F-6 answered: mesh metrics get a flat analyzer plus
  a name/`device_class` hint table, not a per-integration plugin seam
  (D-05-5). Both integrations expose LQI/RSSI as ordinary entities; they differ
  only in name, in whether a `device_class` exists, and in whether the entity
  is enabled — ZHA ships both disabled, Zigbee2MQTT has no RSSI. Found:
  `via_device_id` is a coordinator star on both, making D-05-3's shared-parent
  annotation vacuous for Zigbee (F-27).
- 2026-09-05 · `P4-05` — `find_unavailable_entities` (cheap aggregate scan,
  paginated) and `find_stale_entities` (per-entity cadence scan bounded by the
  HA-request budget, `Truncated` meaning "candidates remain unexamined").
  PRIVATE entities are excluded outright under the deny profile in both,
  counted via `PrivateExcluded`. Found: `find_stale_entities`' budget class
  has no measurement behind it (F-26).
