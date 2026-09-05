# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P3-09` — fallback logbook events go through the privacy profile (F-23)
· [`phases/03-inventory-tools.md`](phases/03-inventory-tools.md) · model: **claude-opus-5** · flags: 🧠

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P3-09` | fallback logbook events go through the privacy profile (F-23) | 03 | claude-opus-5 | 🧠 |
| 2 | `P4-05` | `find_unavailable_entities` / `find_stale_entities` | 04 | claude-sonnet-5 | |

**Order rationale (2026-09-05 `plan` #3 — inbox only, no new scope).** Nothing
was scoped and no row moved: the one open `queue-next` (F-23) already has its
task in the queue as the active `P3-09`, so this pass added no boxes. It exists
to answer the question the last status block left standing — F-17's trigger had
fired — and the answer is evidence, not a fix: `P4-04` closed history-backed
(`policy.SourceHistory`), so `P2-01`'s *statistics* estimate still never bound,
and nothing in production pre-flights `SourceStatistics` at all. F-17 is
deferred again on a code condition rather than a task id (the first production
`Preflight(SourceStatistics, …)` call site), and reading for it surfaced **F-25**
— the gateway allow-lists three recorder-statistics commands nothing calls —
filed `defer` on that same trigger. F-6 stays deferred a fifth time, now one
box from its own trigger. Order below is unchanged and unchanged for the
original reason: `P3-09` is a privacy-guarantee gap and outranks a new feature;
`P4-05` last, because closing it closes Phase 04 and unblocks Phase 05's
breakdown and F-6.

**Four decisions settled 2026-08-25.** *Transport:* **stdio only** — no
listening port, no client-auth subsystem, and every log line goes to stderr
because stdout carries the framing (phase 01). *Supervisor:* **`hassio_api:
true` at the default role** — `list_apps` becomes implementable and
`get_system_health` stops being partial; still no `*/stats` and no write-capable
path (phase 00). *Catalog:* **the full twenty before release** — no reduced first
cut; a tool the evidence rules out ships answering `unsupported`, not missing
(phase 03). *HA versions:* **current release only** — one adapter variant, one CI
target, `unsupported` with the detected version outside it (phase 00). Two more
were settled 2026-09-05 by this `plan`: `P4-06`'s fix direction (correct §12.1 to
the fixture, not the fixture to §12.1) and `P3-09`'s masking design (mask the
fallback event whole, keyed by its own entity's classification, never search its
text for substrings). Rationale and rejected alternatives in each phase file.

**One decision remains open**, not blocking this queue: Phase 02's Q10
(persistence), deliberately not asked yet — it waits on Phase 05 producing a
diagnostic memory-only cannot deliver.

`cmd/spike` is the probe vehicle these tasks reuse: `HA_URL` + `HA_TOKEN`, it
reports field names and types only. The owner runs it and pastes the report; no
HA token reaches the agent (owner's choice, 2026-08-23).

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
| 03 | MCP Server & Inventory Tools | 10 / 11 |
| 04 | History, Statistics & Detection | 5 / 6 |
| 05 | Diagnostics & Evidence Engine | 0 / 1 |
| 06 | Proposal Mode — gated | 0 / 1 |
| 07 | Controlled Change (Admin) — gated | 0 / 1 |

Counts include each phase's decision entries, which are boxes too. Phases 00 and
01 are fully ticked. Phase 02's one open box is the Q10 persistence decision.
**Phase 03 is open again at 10/11** — not reopened, it simply has a box again:
`P3-09`, the F-23 privacy fix, plus its already-settled decision record. Phase 04
is 5/6: `P4-06` (F-24's doc fix) and `P4-04` (`get_entity_statistics`) both
closed 2026-09-05, leaving `P4-05` as the phase's last box. F-17's trigger fired
with `P4-04` and was re-triaged by 2026-09-05's third `plan` — see Open
findings.

Phases 00–04 are milestone M1 (v1 observer). Phase 05 is M2. Phases 06–07 are
gated: they open only on an explicit owner decision plus a fresh security review.
Phases 05–07 carry no task boxes yet — theirs are written by `devflow plan` when
the phase before them closes.

Last refreshed: 2026-09-05 (`plan` #3 — inbox drained; no boxes added)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 1 · `defer` 3 · `unknown` 2 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

The one `queue-next` (F-23) has its task in the queue above (`P3-09`) and
closes when that does. Three `defer`s, all re-triaged by 2026-09-05's third
`plan`: **F-17** (batched statistics ~30% larger) had its trigger fire and was
deferred again *with evidence* — `P4-04` closed history-backed, so `P2-01`'s
statistics estimate still never bound; its trigger is now the first production
`Preflight(SourceStatistics, …)` call site rather than a task id · **F-25** (the
gateway allow-lists three recorder-statistics commands nothing calls) shares
that trigger, filed by the same pass · **F-6** (Zigbee metric normalization)
waits on `P4-05` closing, which both closes Phase 04 and gives Phase 05 boxes to
consume the answer. The two open `unknown`s are F-6 and F-17.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

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
- 2026-09-05 · `P4-02` — availability and outage analysis opens
  `internal/analysis`; fixture `entity_history_7d.json` reproduces §12.1's
  outage numbers through the real mapper. Found: its ratio is 0.98095 (F-24).
- 2026-09-05 · `P4-01` — `get_entity_history` over
  `history/history_during_period`, first user of `CheckHistoryScope` and the
  query budget; `resolution` deferred to `P4-04` by decision record.
