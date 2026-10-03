# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** none — phase 05's task boxes are all ticked. Run `devflow plan`
to triage the open findings (F-17/F-25 re-triage at Phase 05 close, F-36) and
queue what follows. `P5-09` is on branch `feat/P5-09`, awaiting the owner's merge.

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Suspended

None.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|

**Ordering rationale (2026-10-03 second `plan`, F-35).** Producers before the
e2e that walks them, as for `P5-07`. `P5-14` goes first because `P5-16`'s host
row hangs on "a cluster exists" in the response. `P5-15` comes before `P5-16`
because the neighbour-table row is conditioned on resolved mesh metrics.
`P5-10` stays after `P5-08`: it measures `analyze_integration_health` with the
mesh reads included. **Two decisions taken (owner, F-35):** **D-05-8**:
topology ships as a `clusters` list beside `evidence`, and `Evidence` stays
measurement-only. **D-05-9**: mesh metrics are evidence only, with no LQI
threshold hypothesis in v1.

**Ordering rationale (2026-10-03 `plan`).** Dependencies → analysis → tool →
the §13.1 e2e that walks them. `P5-10` now runs **before** `P5-09`: one
`cmd/spike` session measures all three composite tools and observes the two
unknowns `P5-09` rests on (F-28 chaining, F-31 restart evidence), which the
old order asked for but did not schedule.

**Two decisions taken 2026-10-03 (owner, F-30):** **D-05-6** — automation
hypotheses ship as a **twenty-first tool**, `analyze_automation_health`
(amends phase 03's "full twenty" in `P5-13`); **D-05-7** — dependencies come
from **our mapper, ids only**, with HA's `search/related` recorded as the
fallback (F-32, `defer`).

**Five earlier design decisions govern this phase's boxes**, D-05-1…5 in the phase
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
| 05 | Diagnostics & Evidence Engine | 26 / 26 |
| 06 | Proposal Mode — gated | 0 / 1 |
| 07 | Controlled Change (Admin) — gated | 0 / 1 |

Counts include each phase's decision entries, which are boxes too. Phase 05's
26 ticked are D-05-1…10 and the `P5-01`…`P5-16` task boxes; nothing in the phase is open. Phase 02 is
complete: its last box, the Q10 persistence decision, closed 2026-10-03.

Phases 00–04 are milestone M1 (v1 observer) and are **fully implemented**.
Phase 05 is M2, and is where the last two catalog rows
(`analyze_entity_health`, `analyze_integration_health`) became real. Phases 06–07 are gated: they open only on an
explicit owner decision plus a fresh security review, and carry no task boxes.

Last refreshed: 2026-10-03 (`P5-09`)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 0 · `defer` 6 · `unknown` 1 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

No `queue-next`. F-31 closed with `P5-09`; F-26 and F-28 closed with `P5-10` (D-05-10).
Six `defer`s: **F-17** and **F-25** wait
on the first production `Preflight(policy.SourceStatistics, …)` call site — **if
Phase 05 closes with still none, F-17 becomes `wont-fix` and F-25 a deletion task,
at that `plan`**; **F-32** is D-05-7's recorded fallback (`search/related`);
**F-33** is the owner's phase 06 topic; **F-34** (non-admin principals get no
automation hypotheses) waits on a non-admin deployment mattering; **F-36**
(statistics-based staleness, one batched call instead of one read per entity)
re-triages after Phase 05. The one open `unknown` is F-17.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-10-03 · `P5-09` — restart evidence: `analyze_integration_health` reads ±5 min of logbook
  per cluster onset (max 3), keeps only core lifecycle rows; restart ⇒ restart hypothesis,
  none ⇒ upstream citing the absence, unread ⇒ missing. Closes F-31; §21 criterion pinned.
- 2026-10-03 · `P5-10` — `cmd/measure` (real tools in-process) + logbook probe; owner ran both. D-05-10:
  classes stand (`find_stale_entities` 20 req/page, `analyze_*` max 36/50). Closes F-26, F-28.
- 2026-10-03 · `P5-08` — §13.2 end to end in `investigation_test.go`: partial parent ⇒
  topology claim, none without a parent, star ⇒ `withheld`; mesh evidence vs
  `entity_disabled`; host row `privileged`. Closes F-27, F-35.
- 2026-10-03 · `P5-16` — `addUnreachableEvidence`: cluster ⇒ `MissingPrivileged` host
  row + tool-less `NextAction`; `MeshResolved` input ⇒ `MissingNotExposed`
  neighbour-table row. No integration name read; no hypothesis changes.
- 2026-10-03 · `P5-15` — `analysis/mesh.go`: `ResolveMeshMetrics` (device_class
  first, entity-id hint table second; never platform), `MeshEvidence` (min/mean/
  samples, no zero for absence); tool reads metrics of affected devices only,
  cap 10. `entity_disabled`/`not_exposed` rows; no hypothesis cites mesh evidence.
