# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P5-02` — evidence / hypothesis / missing-evidence model · phase
05 · stronger model · 🧠. Replace the unused `internal/model/evidence.go` stub
with D-05-1's four distinct types plus the `HealthAnalysis` envelope. Note
D-05-5 (new): `MissingEvidence` is what carries an unreadable mesh metric, so
it needs to name *why* a source was unreadable, not merely that it was.

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P5-02` | evidence / hypothesis / missing-evidence model | 05 | stronger | 🧠 |
| 2 | `P5-03` | derived confidence (`ConfidenceFor`) | 05 | stronger | 🧠 `blocked:P5-02` |
| 3 | `P5-04` | cross-entity outage clustering (**must settle F-27**) | 05 | stronger | 🧠 `blocked:P5-02,P5-03` |
| 4 | `P5-05` | `analyze_entity_health` | 05 | default | `blocked:P5-02,P5-03` |
| 5 | `P5-06` | `analyze_integration_health` | 05 | default | `blocked:P5-04,P5-05` |
| 6 | `P5-07` | investigation 1 — doc §13.1 e2e + degraded branch | 05 | default | `blocked:P5-05` |
| 7 | `P5-08` | investigation 2 — doc §13.2 e2e (**F-27 changes its DoD**) | 05 | default | `blocked:P5-06` |
| 8 | `P5-09` | investigation 3 — correlated mass unavailability | 05 | default | `blocked:P5-06` |
| 9 | `P5-10` | measure composite budget, re-class `find_stale_entities` (F-26) | 05 | default | `needs-verify` `blocked:P5-06` |

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

**F-27 is the live consequence of D-05-5 and lands on two queued boxes.**
`via_device_id` is a coordinator star on both Zigbee integrations, so D-05-3's
"members share a `via_device` parent" annotation is vacuous for Zigbee — it is
true of the whole network. `P5-04` decides whether that annotation is emitted
at all when its cardinality is 1; `P5-08`'s DoD, which asserts a shared-parent
cluster, is satisfiable by a fixture while unreachable in reality and needs
rewriting there. Neither is a re-plan — both boxes stand, with F-27 named in
them.

**Earlier decisions, still standing.** *Transport:* stdio only (phase 01).
*Supervisor:* `hassio_api: true` at the default role (phase 00). *Catalog:* the
full twenty before release (phase 03). *HA versions:* current release only
(phase 00). `P4-05`: a PRIVATE entity is excluded outright from both `find_*`
tools under the deny profile, never masked.

**One decision remains open**, not blocking this queue: Phase 02's Q10
(persistence), deliberately not asked yet — it waits on Phase 05 producing a
diagnostic memory-only cannot deliver. `P5-04`'s clustering over historical
outages is the candidate; ask when it lands, not before.

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
| 02 | Policy, Privacy, Budget & Audit | 7 / 8 |
| 03 | MCP Server & Inventory Tools | 11 / 11 |
| 04 | History, Statistics & Detection | 6 / 6 |
| 05 | Diagnostics & Evidence Engine | 6 / 15 |
| 06 | Proposal Mode — gated | 0 / 1 |
| 07 | Controlled Change (Admin) — gated | 0 / 1 |

Counts include each phase's decision entries, which are boxes too. Phase 05's
6 ticked are D-05-1…5 and the `P5-01` task box; its nine remaining task boxes
are open, and no decision entry in the phase is open any more. Phase 02's one
open box is the Q10 persistence decision.

Phases 00–04 are milestone M1 (v1 observer) and are **fully implemented**.
Phase 05 is M2, and is where the last two catalog rows
(`analyze_entity_health`, `analyze_integration_health` — today bound to
`bindNotImplemented`) become real. Phases 06–07 are gated: they open only on an
explicit owner decision plus a fresh security review, and carry no task boxes.

Last refreshed: 2026-09-05 (`P5-01` closed — Q9 answered, D-05-5 written)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 2 · `defer` 2 · `unknown` 1 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

**F-6 closed with `P5-01`** (Q9 answered; see D-05-5), leaving two
`queue-next`, both attached to boxes already in the queue and closing when
those close: **F-26** → `P5-10`, and the new **F-27** → `P5-04` (the vacuous
shared-parent annotation) and `P5-08` (its DoD). Two `defer`s remain, both on
the same unfired trigger — the first production
`Preflight(policy.SourceStatistics, …)` call site: **F-17** (batched
statistics ~30% larger) and **F-25** (three allow-listed recorder commands
nothing calls). No Phase 05 box creates that call site, so a standing decision
stands in place of a sixth deferral: **if Phase 05 closes with still no such
call site, F-17 becomes `wont-fix` and F-25 becomes a deletion task, at that
`plan`.** The one open `unknown` is F-17.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

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
