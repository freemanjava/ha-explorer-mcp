# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P5-01` — verify mesh/Zigbee metric normalization (F-6, Q9) ·
phase 05 · default model · `needs-verify`. Run `devflow verify`, not an
implementation cycle: the owner runs `cmd/spike` against their Zigbee stack and
pastes the report; no token or value reaches the agent.

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P5-01` | verify Zigbee/mesh metric normalization (F-6) | 05 | default | `needs-verify` |
| 2 | `P5-02` | evidence / hypothesis / missing-evidence model | 05 | stronger | 🧠 |
| 3 | `P5-03` | derived confidence (`ConfidenceFor`) | 05 | stronger | 🧠 `blocked:P5-02` |
| 4 | `P5-04` | cross-entity outage clustering | 05 | stronger | 🧠 `blocked:P5-01,P5-02,P5-03` |
| 5 | `P5-05` | `analyze_entity_health` | 05 | default | `blocked:P5-02,P5-03` |
| 6 | `P5-06` | `analyze_integration_health` | 05 | default | `blocked:P5-04,P5-05` |
| 7 | `P5-07` | investigation 1 — doc §13.1 e2e + degraded branch | 05 | default | `blocked:P5-05` |
| 8 | `P5-08` | investigation 2 — doc §13.2 e2e | 05 | default | `blocked:P5-06` |
| 9 | `P5-09` | investigation 3 — correlated mass unavailability | 05 | default | `blocked:P5-06` |
| 10 | `P5-10` | measure composite budget, re-class `find_stale_entities` (F-26) | 05 | default | `needs-verify` `blocked:P5-06` |

**Ordering rationale (2026-09-05 `plan`).** Verify → model → analysis
primitives → tools → workflows → measurement. `P5-01` is first because its
answer is structural: whether mesh metrics normalize across integrations
decides if `P5-04`/`P5-06` need a per-integration seam, and designing them
first would settle that on an unverified premise. `P5-10` is last because
measuring composite cost needs the composite tools to exist.

**Four design decisions were settled in this `plan`** and written into phase 05
as D-05-1…4, so the implementation boxes follow a spec rather than making
judgment calls: fact/inference/recommendation are **separate types**, not
fields on one struct · **confidence comes from one `ConfidenceFor` function**
or does not exist · outage clusters are **overlap-with-tolerance, annotated
afterwards**, never a correlation coefficient · **no health score in v1**.
Rationale and rejected alternatives in the phase file.

**Earlier decisions, still standing.** *Transport:* stdio only (phase 01).
*Supervisor:* `hassio_api: true` at the default role (phase 00). *Catalog:* the
full twenty before release (phase 03). *HA versions:* current release only
(phase 00). `P4-05`: a PRIVATE entity is excluded outright from both `find_*`
tools under the deny profile, never masked.

**One decision remains open**, not blocking this queue: Phase 02's Q10
(persistence), deliberately not asked yet — it waits on Phase 05 producing a
diagnostic memory-only cannot deliver. `P5-04`'s clustering over historical
outages is the candidate; ask when it lands, not before.

`cmd/spike` is the probe vehicle `P5-01` and `P5-10` reuse: `HA_URL` +
`HA_TOKEN`, it reports field names and types only. The owner runs it and pastes
the report; no HA token reaches the agent (owner's choice, 2026-08-23).

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
| 02 | Policy, Privacy, Budget & Audit | 7 / 8 |
| 03 | MCP Server & Inventory Tools | 11 / 11 |
| 04 | History, Statistics & Detection | 6 / 6 |
| 05 | Diagnostics & Evidence Engine | 4 / 15 |
| 06 | Proposal Mode — gated | 0 / 1 |
| 07 | Controlled Change (Admin) — gated | 0 / 1 |

Counts include each phase's decision entries, which are boxes too. Phase 05's
4 ticked are D-05-1…4, settled by this `plan`; its ten task boxes and the
`needs-verify` Zigbee decision are open. Phase 02's one open box is the Q10
persistence decision.

Phases 00–04 are milestone M1 (v1 observer) and are **fully implemented**.
Phase 05 is M2, and is where the last two catalog rows
(`analyze_entity_health`, `analyze_integration_health` — today bound to
`bindNotImplemented`) become real. Phases 06–07 are gated: they open only on an
explicit owner decision plus a fresh security review, and carry no task boxes.

Last refreshed: 2026-09-05 (`plan` — Phase 05 broken down)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 2 · `defer` 2 · `unknown` 2 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

**Both `queue-next` are now queued, not waiting:** **F-6** became `P5-01`
(head of the queue) and **F-26** became `P5-10`; each closes when its task
closes. Two `defer`s remain, both on the same unfired trigger — the first
production `Preflight(policy.SourceStatistics, …)` call site: **F-17**
(batched statistics ~30% larger) and **F-25** (three allow-listed recorder
commands nothing calls). No Phase 05 box creates that call site, so this
`plan` recorded a standing decision instead of a sixth deferral: **if Phase 05
closes with still no such call site, F-17 becomes `wont-fix` and F-25 becomes
a deletion task, at that `plan`.** The two open `unknown`s are F-6 and F-17.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-09-05 · `P4-05` — `find_unavailable_entities` (cheap aggregate scan,
  paginated) and `find_stale_entities` (per-entity cadence scan bounded by the
  HA-request budget, `Truncated` meaning "candidates remain unexamined").
  PRIVATE entities are excluded outright under the deny profile in both,
  counted via `PrivateExcluded`. Found: `find_stale_entities`' budget class
  has no measurement behind it (F-26).
- 2026-09-05 · `P3-09` — the fallback logbook events of
  `get_automation_traces` now go through the privacy profile (F-23 closed):
  `maskFallbackEvents` masks `Name`/`Message` whole via `redact`'s new
  `MaskedText`, keyed by the event's own entity; `When`/`ContextID` survive.
- 2026-09-05 · `P4-04` — `get_entity_statistics` joins `P4-02`/`P4-03` into one
  tool over `model.Health` (Phase 00's unused stub, extended); one range cap
  reused from `P4-01`; `Source` names the recorder endpoint, not the subsystem.
- 2026-09-05 · `P4-06` — doc §12.1 made self-consistent (F-24 resolved):
  `availability_ratio` → `0.98095`, `median_update_interval_s` → `1376.5`,
  `p95_update_interval_s` → `2578.8`, matching the fixture through the real
  mapper. Doc-only; `test/fixtures/entity_history_7d.json` untouched.
- 2026-09-05 · `P4-03` — cadence and staleness: median/p95 update intervals,
  state-change rate, and a stale verdict at `3 × p95` against the entity's own
  cadence. Found: §12.1's cadence half is impossible too (appended to F-24).
