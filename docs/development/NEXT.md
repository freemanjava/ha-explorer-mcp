# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P4-05` — `find_unavailable_entities` / `find_stale_entities`
· [`phases/04-history-statistics.md`](phases/04-history-statistics.md) · model: **claude-sonnet-5** · flags: —

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P4-05` | `find_unavailable_entities` / `find_stale_entities` | 04 | claude-sonnet-5 | |

**Order rationale (2026-09-05, after `P3-09` closed).** One row left, and it is
the row the last three `plan` passes already put last: `P4-05` closes Phase 04,
which is what gives Phase 05 boxes to write and unblocks F-6. The queue is now
below three rows but no `queue-next` finding is open — F-23, the only one, closed
with `P3-09` — so there is nothing to drain; the next `plan` writes Phase 05's
breakdown rather than pulling from the inbox.

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
| 03 | MCP Server & Inventory Tools | 11 / 11 |
| 04 | History, Statistics & Detection | 5 / 6 |
| 05 | Diagnostics & Evidence Engine | 0 / 1 |
| 06 | Proposal Mode — gated | 0 / 1 |
| 07 | Controlled Change (Admin) — gated | 0 / 1 |

Counts include each phase's decision entries, which are boxes too. **Phases 00,
01 and 03 are fully ticked** — `P3-09` closed Phase 03 for the second time, this
time with the F-23 privacy gap shut. Phase 02's one open box is the Q10
persistence decision. Phase 04 is 5/6 with `P4-05` as its last box; closing it
closes M1's implementation surface. F-17's trigger fired with `P4-04` and was
re-triaged by 2026-09-05's third `plan` — see Open findings.

Phases 00–04 are milestone M1 (v1 observer). Phase 05 is M2. Phases 06–07 are
gated: they open only on an explicit owner decision plus a fresh security review.
Phases 05–07 carry no task boxes yet — theirs are written by `devflow plan` when
the phase before them closes.

Last refreshed: 2026-09-05 (`next` — `P3-09` closed)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 0 · `defer` 3 · `unknown` 2 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

**No `queue-next` remains**: F-23 closed with `P3-09`, which masks the fallback
logbook events whole through the profile. Three `defer`s, all re-triaged by
2026-09-05's third `plan`: **F-17** (batched statistics ~30% larger) had its
trigger fire and was deferred again *with evidence* — `P4-04` closed
history-backed, so `P2-01`'s statistics estimate still never bound; its trigger
is now the first production `Preflight(SourceStatistics, …)` call site rather
than a task id · **F-25** (the gateway allow-lists three recorder-statistics
commands nothing calls) shares that trigger, filed by the same pass · **F-6**
(Zigbee metric normalization) waits on `P4-05` closing, which both closes Phase
04 and gives Phase 05 boxes to consume the answer. The two open `unknown`s are
F-6 and F-17.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

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
- 2026-09-05 · `P4-02` — availability and outage analysis opens
  `internal/analysis`; fixture `entity_history_7d.json` reproduces §12.1's
  outage numbers through the real mapper. Found: its ratio is 0.98095 (F-24).
