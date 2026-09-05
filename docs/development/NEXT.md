# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** none queued. Phase 04 is closed — run `devflow plan` to write
Phase 05's task breakdown.

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|

**Empty (2026-09-05, after `P4-05` closed).** `P4-05` closed Phase 04 — M1's
whole implementation surface is now built. Two things feed the next `plan`:
**F-6** (Zigbee metric normalization), which was waiting on exactly this
closure to give Phase 05 boxes to consume it, and **F-26** (`find_stale_entities`'
budget class has no real-installation measurement), filed by `P4-05` itself.
Neither is `blocks-active`, so this is a `plan` pass, not a blocked queue.

**Four decisions settled 2026-08-25.** *Transport:* **stdio only** — no
listening port, no client-auth subsystem, and every log line goes to stderr
because stdout carries the framing (phase 01). *Supervisor:* **`hassio_api:
true` at the default role** — `list_apps` becomes implementable and
`get_system_health` stops being partial; still no `*/stats` and no write-capable
path (phase 00). *Catalog:* **the full twenty before release** — no reduced first
cut; a tool the evidence rules out ships answering `unsupported`, not missing
(phase 03). *HA versions:* **current release only** — one adapter variant, one CI
target, `unsupported` with the detected version outside it (phase 00). Two more
were settled 2026-09-05: `P4-06`'s fix direction (correct §12.1 to the fixture,
not the fixture to §12.1) and `P3-09`'s masking design (mask the fallback event
whole, keyed by its own entity's classification, never search its text for
substrings). `P4-05` added a third: a PRIVATE entity is excluded outright from
both `find_*` tools under the deny profile, never masked — there is no state
value to mask, only membership in "unavailable"/"stale", and an
installation-wide scan of that over a private domain is the same bulk-
correlation exposure `CheckHistoryScope` already refuses for named history.
Rationale and rejected alternatives in each phase file.

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
| 03 | MCP Server & Inventory Tools | 11 / 11 |
| 04 | History, Statistics & Detection | 6 / 6 |
| 05 | Diagnostics & Evidence Engine | 0 / 1 |
| 06 | Proposal Mode — gated | 0 / 1 |
| 07 | Controlled Change (Admin) — gated | 0 / 1 |

Counts include each phase's decision entries, which are boxes too. **Phases 00,
01, 03 and now 04 are fully ticked** — `P4-05` closed Phase 04 with
`find_unavailable_entities` and `find_stale_entities`, completing M1's
implementation surface. Phase 02's one open box is the Q10 persistence decision.

Phases 00–04 are milestone M1 (v1 observer) and are now **fully implemented**.
Phase 05 is M2. Phases 06–07 are gated: they open only on an explicit owner
decision plus a fresh security review. Phases 05–07 carry no task boxes yet —
theirs are written by `devflow plan` when the phase before them closes, which is
exactly where this pointer is now.

Last refreshed: 2026-09-05 (`next` — `P4-05` closed)

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

**One `queue-next`: F-26** (`find_stale_entities`'s `ClassNormalRead` budget has
no real-installation measurement behind the choice not to widen it), filed by
`P4-05`. Three `defer`s, unchanged since the third 2026-09-05 `plan`: **F-17**
(batched statistics ~30% larger) and **F-25** (the gateway allow-lists three
recorder-statistics commands nothing calls) share a trigger — the first
production `Preflight(SourceStatistics, …)` call site — that has still not
fired · **F-6** (Zigbee metric normalization) was waiting on `P4-05` closing
Phase 04, which it now has; it feeds the next `plan`'s Phase 05 breakdown
rather than resolving on its own. The two open `unknown`s are F-6 and F-17.

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
