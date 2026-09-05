# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P4-06` — doc §12.1's example statistics made self-consistent
· [`phases/04-history-statistics.md`](phases/04-history-statistics.md) · model: **claude-sonnet-5** · flags: —

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P4-06` | doc §12.1 made self-consistent (F-24) | 04 | claude-sonnet-5 | |
| 2 | `P4-04` | `get_entity_statistics` | 04 | claude-sonnet-5 | |
| 3 | `P3-09` | fallback logbook events go through the privacy profile (F-23) | 03 | claude-opus-5 | 🧠 |
| 4 | `P4-05` | `find_unavailable_entities` / `find_stale_entities` | 04 | claude-sonnet-5 | |

**Order rationale (2026-09-05 `plan` #2 — the inbox-draining one).** Nothing new
was scoped; this `plan` queued the two open findings and re-triaged the two
`unknown`s. `P4-06` goes **first** and costs minutes: doc §12.1 is the shape
`P4-04` implements against, and both halves of its example are impossible on
their own terms (F-24) — removing the trap before that session reads the section
is cheaper than the confusion it otherwise causes, and the fix direction is
already settled in the phase file. `P4-04` then keeps its place: `P4-01` fetches,
`P4-02`/`P4-03` compute, `P4-04` exposes both as one tool. `P3-09` (F-23) sits
between `P4-04` and `P4-05` because it is a privacy-guarantee gap and so outranks
a new feature, but it reaches an agent only on a non-admin deployment — not
today's default (F-2/F-4) — so it does not displace an in-flight phase. `P4-05`
last, as written: it applies the per-entity metrics installation-wide, and
closing it closes Phase 04, which is what unblocks Phase 05's breakdown and F-6.

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
| 04 | History, Statistics & Detection | 3 / 6 |
| 05 | Diagnostics & Evidence Engine | 0 / 1 |
| 06 | Proposal Mode — gated | 0 / 1 |
| 07 | Controlled Change (Admin) — gated | 0 / 1 |

Counts include each phase's decision entries, which are boxes too. Phases 00 and
01 are fully ticked. Phase 02's one open box is the Q10 persistence decision.
**Phase 03 is open again at 10/11** — not reopened, it simply has a box again:
`P3-09`, the F-23 privacy fix, plus its already-settled decision record. Phase 04
gained `P4-06` (F-24's doc fix), so it is 3/6, not 3/5.

Phases 00–04 are milestone M1 (v1 observer). Phase 05 is M2. Phases 06–07 are
gated: they open only on an explicit owner decision plus a fresh security review.
Phases 05–07 carry no task boxes yet — theirs are written by `devflow plan` when
the phase before them closes.

Last refreshed: 2026-09-05 (`plan` — inbox drained: F-23 → `P3-09`, F-24 →
`P4-06`; pointer moved to `P4-06`)

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

Both `queue-next`s now have tasks in the queue above (F-23 → `P3-09`, F-24 →
`P4-06`); each closes when its task does, not now. The two `defer`s are the two
open `unknown`s, re-triaged this `plan` and left deferred on trigger conditions
that have not fired: **F-17** (batched statistics ~30% larger) is revisited when
`P4-04` closes — the first consumer of statistics, and the first place `P2-01`'s
estimate can bind · **F-6** (Zigbee metric normalization) waits on Phase 04
closing and on Phase 05 having boxes to consume the answer.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-09-05 · `P4-03` — cadence and staleness: median/p95 update intervals,
  state-change rate, and a stale verdict at `3 × p95` against the entity's own
  cadence. Found: §12.1's cadence half is impossible too (appended to F-24).
- 2026-09-05 · `P4-02` — availability and outage analysis opens
  `internal/analysis`; fixture `entity_history_7d.json` reproduces §12.1's
  outage numbers through the real mapper. Found: its ratio is 0.98095 (F-24).
- 2026-09-05 · `P4-01` — `get_entity_history` over
  `history/history_during_period`, first user of `CheckHistoryScope` and the
  query budget; `resolution` deferred to `P4-04` by decision record.
- 2026-09-05 · `P2-06` — measured the invocation rate limit (F-20): no
  degradation to 4 calls/s, double the sustained limit; both constants unchanged.
- 2026-09-05 · `P3-08` — session end is a shutdown, not a crash (F-21):
  `mcp.Run` calls `srv.Connect` itself; client killed mid-request exits 0.
