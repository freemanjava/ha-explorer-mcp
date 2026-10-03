# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P5-11` — automation dependency extraction (D-05-7) · phase 05 ·
default model. `MapAutomation` collects `entity_id`/`device_id`/`area_id`
values from the `automation/config` body — grammar-validated, de-duplicated,
capped; templates counted, never extracted; ids only leave `internal/ha`.

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Suspended

None. `P5-07`'s suspension (F-30) was re-planned 2026-10-03 into `P5-11`…`P5-13`
plus a reduced `P5-07`. Its branch `feat/P5-07` holds no work.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P5-11` | automation dependency extraction (D-05-7) | 05 | default | |
| 2 | `P5-12` | `AnalyzeAutomationHealth` — runs × dependency windows | 05 | stronger | 🧠 |
| 3 | `P5-13` | `analyze_automation_health` tool — the twenty-first | 05 | default | |
| 4 | `P5-07` | investigation 1 — doc §13.1 e2e + degraded branch | 05 | default | |
| 5 | `P5-08` | investigation 2 — doc §13.2 e2e (F-27's rule in its DoD) | 05 | default | |
| 6 | `P5-10` | measure composite budget (F-26); observe F-28, F-31 | 05 | default | `needs-verify` |
| 7 | `P5-09` | investigation 3 — mass unavailability vs. HA restart | 05 | default | |

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

**Earlier ordering (2026-09-05 `plan`).** Verify → model → analysis
primitives → tools → workflows → measurement. **Ordering rationale (2026-09-05 `plan`).** Verify → model → analysis
primitives → tools → workflows → measurement. `P5-10` follows the composite
tools because measuring them needs them to exist.

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
| 05 | Diagnostics & Evidence Engine | 13 / 20 |
| 06 | Proposal Mode — gated | 0 / 1 |
| 07 | Controlled Change (Admin) — gated | 0 / 1 |

Counts include each phase's decision entries, which are boxes too. Phase 05's
13 ticked are D-05-1…7 and the `P5-01`…`P5-06` task boxes; its seven remaining
are task boxes (`P5-07`…`P5-13`), and no decision entry in the phase is open. Phase 02 is
complete: its last box, the Q10 persistence decision, closed 2026-10-03.

Phases 00–04 are milestone M1 (v1 observer) and are **fully implemented**.
Phase 05 is M2, and is where the last two catalog rows
(`analyze_entity_health`, `analyze_integration_health`) became real. Phases 06–07 are gated: they open only on an
explicit owner decision plus a fresh security review, and carry no task boxes.

Last refreshed: 2026-10-03 (`plan` — F-30 split `P5-07` into `P5-11`…`P5-13` + e2e)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 5 · `defer` 4 · `unknown` 3 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

Five `queue-next`, all attached to queued boxes and closing with them:
**F-26** → `P5-10`; **F-27** → `P5-08`; **F-28** (`unknown`) and **F-31**
(`unknown`, no evidence for "HA restarted") → observed in `P5-10`'s live run,
before `P5-09`; **F-30** → `P5-11`…`P5-13`, closes with `P5-07`. Four
`defer`s: **F-17** and **F-25** wait on the first production
`Preflight(policy.SourceStatistics, …)` call site — **if Phase 05 closes with
still none, F-17 becomes `wont-fix` and F-25 a deletion task, at that
`plan`**; **F-32** is D-05-7's recorded fallback (`search/related`),
triggered by evidence the mapper misses dependencies; **F-33** is the owner's
phase 06 topic (help writing fixes/new automations), noted at that gate. The
open `unknown`s are F-17, F-28 and F-31.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-10-03 · `P5-06` — `analyze_integration_health`: setup state, inventory
  ratio, domain repairs, clusters over the entry's entities, Supervisor
  resolution counts (absence named, call still answers). History read for ≤25
  entities, down-now first, the rest named in `missing_evidence` (P5-10 to
  measure).
- 2026-10-03 · `P5-05` — `analyze_entity_health`: each source (history,
  registry, repairs) read independently, a failed one becomes
  `missing_evidence`; hypotheses via `ConfidenceFor`, no score. Found and fixed:
  `History` was never wired in `cmd/server` (F-29).
- 2026-10-03 · `P5-04` — `ClusterOutages`: sort + one sweep, 2-min tolerance,
  ≥2-entity clusters annotated afterwards, each a citable `Evidence`. F-27: a
  star parent goes to `Withheld`, never `Shared`. Whole-period outages leave
  the sweep (`UnavailableThroughout`). Found: partial long outages still chain
  (F-28).
- 2026-10-03 · `P5-03` — `ConfidenceFor(cited ...model.Evidence)`: ladder
  high ≥20 samples & ≥0.9 coverage, medium ≥5 & ≥0.5, `Degraded` demotes one
  step, several citations take the weakest. A source scan forbids naming a
  confidence level anywhere but `confidence.go`/`model/evidence.go`.
