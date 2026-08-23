# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P0-07` — Verify recorder history & statistics APIs
· [`phases/00-spike-foundations.md`](phases/00-spike-foundations.md) · model: **claude-opus-5** · flags: 🧠 `needs-verify`

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

> Module path is settled (2026-08-23): `github.com/freemanjava/ha-explorer-mcp`,
> remote `origin` wired. `P0-01` pins dependencies against it.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P0-07` | Verify recorder history & statistics APIs | 00 | claude-opus-5 | 🧠 `needs-verify` |
| 2 | `P0-08` | Verify the Supervisor App builder's build context | 00 | claude-sonnet-5 | `needs-verify` |

**Order rationale:** the verifications run in the order the Phase 01 allow-list
consumes them. `P0-06` is done: the App's Core principal is admin and the
Supervisor permission matrix is written down, so the remaining Phase 00 unknowns
are the history source (`P0-07`, which Phase 04 is written against) and the
build context (`P0-08`, which gates deployment rather than design and is the
only queue row on the default model). Phase 01 can now be planned as far as F-5
allows — its allow-list still needs `P0-07`'s answer, and it has gained a
required deny-list entry from **F-13**.

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
| 00 | Spike & Foundations | 7 / 11 |
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

Last refreshed: 2026-08-23 (`P0-06` closed)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 6 · `defer` 1 · `unknown` 2 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

Every open `queue-next` finding names the task it became, except the newest:
F-5 → `P0-07` · F-8 → `P0-08` · F-10 and F-12 → `P2-02` / `P2-03` DoDs ·
F-11 → `P3-07` DoD plus a Phase 05 degraded-workflow bullet. **F-13 is new and
unplanned** — Core's `supervisor/api` WebSocket command is a write path and a
free-form escape hatch reachable by any admin, which this App is, so Phase 01's
gateway must deny it by name; it becomes a task at the next `devflow plan`.
F-1, F-2, F-3, F-4, F-7 and F-9 are **resolved** — `P0-06` closed F-4 and with it
the residues F-2 and F-3 were waiting on. F-6 deliberately stays `defer`:
Phase 05 owns it and has no boxes yet.

Nothing in Phase 01 may be planned until F-5 closes: its allow-list is the
project's primary security boundary and would otherwise be written from an
unverified list.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-08-23 · `P0-06` — Supervisor permission matrix derived at pinned tags (Supervisor 2026.08.0 / Core 2026.8.3): `list_apps` needs `hassio_api: true` at the *default* role, `get_system_health` is partial without it; the App's Core principal is **admin**; `docs/research/2026-08-23-supervisor-permissions.md`.
- 2026-08-23 · `P0-05` — automation config & traces verified against live HA 2026.8.3: all commands exist and work, all are **admin-gated**; fallback is `last_triggered` + `logbook/get_events`; `docs/research/2026-08-23-ha-automation-traces.md`.
- 2026-08-23 · `P0-04` — registry & config-entry APIs verified against live HA 2026.8.3, admin and non-admin; only `config_entries/get_single` needs admin; `docs/research/2026-08-23-ha-registry-apis.md`.
- 2026-08-23 · `P0-03` — Supervisor proxy connectivity: `internal/ha` WS auth handshake + REST GET(`github.com/coder/websocket`), bounded backoff reconnect, integration test against a recorded-HA fake server.
- 2026-08-23 · `P0-02` — App packaging skeleton (config.yaml, build.yaml, Dockerfile, run.sh, AppArmor); arm64 image build verified.

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
