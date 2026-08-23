# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P0-08` — Verify the Supervisor App builder's build context
· [`phases/00-spike-foundations.md`](phases/00-spike-foundations.md) · model: **claude-sonnet-5** · flags: `needs-verify`

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

> Module path is settled (2026-08-23): `github.com/freemanjava/ha-explorer-mcp`,
> remote `origin` wired. `P0-01` pins dependencies against it.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P0-08` | Verify the Supervisor App builder's build context | 00 | claude-sonnet-5 | `needs-verify` |

**Order rationale:** `P0-07` is done — F-5 is closed and the Phase 01 allow-list
now has its history and statistics entries from observation. `P0-08` is the last
Phase 00 unknown and gates deployment rather than design. **The queue is down to
one row while six `queue-next` findings are open: run `devflow plan` next.**
Phase 01 is now fully plannable — its allow-list has the verified command set
and a required deny-list entry from **F-13** — and F-14 (multi-entity query
cost) needs a verify task before Phase 02's budget limits can be more than
guesses.

**One decision is waiting** (phase 00, *Decisions*): the App's Supervisor
permission level — keep `hassio_api: false` and drop `list_apps`, or grant it at
the default role. Nothing is blocked on it today; Phase 03's tool catalog is.

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
| 00 | Spike & Foundations | 8 / 11 |
| 01 | HA Access & Read-Only Gateway | 0 / 7 |
| 02 | Policy, Privacy, Budget & Audit | 0 / 7 |
| 03 | MCP Server & Inventory Tools | 0 / 8 |
| 04 | History, Statistics & Detection | 0 / 5 |
| 05 | Diagnostics & Evidence Engine | 0 / 1 |
| 06 | Proposal Mode — gated | 0 / 1 |
| 07 | Controlled Change (Admin) — gated | 0 / 1 |

Phases 00–04 are milestone M1 (v1 observer). Phase 05 is M2. Phases 06–07 are
gated: they open only on an explicit owner decision plus a fresh security review.
Phases 05–07 carry no task boxes yet — theirs are written by `devflow plan` when
the phase before them closes.

Last refreshed: 2026-08-23 (`P0-07` closed)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 6 · `defer` 1 · `unknown` 3 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

Open `queue-next`: F-8 → `P0-08` · F-10 and F-12 → `P2-02` / `P2-03` DoDs ·
F-11 → `P3-07` DoD plus a Phase 05 degraded-workflow bullet · **F-13** (Core's
`supervisor/api` WebSocket command is a write path and a free-form escape hatch
reachable by any admin, which this App is — Phase 01's gateway must deny it by
name) and **F-14** (multi-entity history/statistics cost unmeasured, so Phase 02
budget limits stay guesses) are both **unplanned**. F-6 deliberately stays
`defer`: Phase 05 owns it and has no boxes yet. F-1 – F-5, F-7, F-9 and F-15 are
**resolved** — `P0-07` closed F-5 and, in the probe it built, F-15.

F-5 is closed, so Phase 01's allow-list can now be derived from observation
rather than guessed.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-08-23 · `P0-07` — recorder history & statistics verified against live HA 2026.8.3: statistics are 1–3 orders of magnitude cheaper than history (7d of one entity: 794 B vs 3.5 MB unfiltered REST); source order is statistics → WS `history/history_during_period` → REST `/api/history/period` as fallback, `minimal_response` always set; `docs/research/2026-08-23-ha-history-statistics.md`.
- 2026-08-23 · `P0-06` — Supervisor permission matrix derived at pinned tags (Supervisor 2026.08.0 / Core 2026.8.3): `list_apps` needs `hassio_api: true` at the *default* role, `get_system_health` is partial without it; the App's Core principal is **admin**; `docs/research/2026-08-23-supervisor-permissions.md`.
- 2026-08-23 · `P0-05` — automation config & traces verified against live HA 2026.8.3: all commands exist and work, all are **admin-gated**; fallback is `last_triggered` + `logbook/get_events`; `docs/research/2026-08-23-ha-automation-traces.md`.
- 2026-08-23 · `P0-04` — registry & config-entry APIs verified against live HA 2026.8.3, admin and non-admin; only `config_entries/get_single` needs admin; `docs/research/2026-08-23-ha-registry-apis.md`.
- 2026-08-23 · `P0-03` — Supervisor proxy connectivity: `internal/ha` WS auth handshake + REST GET(`github.com/coder/websocket`), bounded backoff reconnect, integration test against a recorded-HA fake server.

## Project facts

| | |
|---|---|
| default model | `claude-sonnet-5` |
| stronger model | `claude-opus-5` |
| repository | `https://github.com/freemanjava/ha-explorer-mcp.git` (`origin`) |
| module path | `github.com/freemanjava/ha-explorer-mcp` |
| base branch | `main` |
| branch per task | `<type>/<task-id>` — e.g. `feat/P0-01` |
| build / test | `make check` |
| run locally | `make run` |
| architecture | [`../HA_Inspector_MCP_Research_and_Architecture.md`](../HA_Inspector_MCP_Research_and_Architecture.md) |
| standards | [`CLAUDE.md`](../../CLAUDE.md) |
| method | [`METHOD.md`](METHOD.md) |

## Loop

`devflow next` — one task, start to finish. `devflow help` for everything else
(planning, findings, status, the standalone prompts).

Without the skill: read this file, read the active task's phase file and
`CLAUDE.md`, check the model gate, branch, write the test, implement, verify
green, close atomically, stop without committing.
