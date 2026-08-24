# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P0-09` — Verify multi-entity history & statistics cost
· [`phases/00-spike-foundations.md`](phases/00-spike-foundations.md) · model: **claude-opus-5** · flags: 🧠 `needs-verify`

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P0-09` | Verify multi-entity history & statistics cost | 00 | claude-opus-5 | 🧠 `needs-verify` |
| 2 | `P1-01` | WebSocket connection manager | 01 | claude-opus-5 | 🧠 |
| 3 | `P1-02` | WebSocket command allow-list | 01 | claude-opus-5 | 🧠 |
| 4 | `P1-07` | Deny privileged escape hatches by name | 01 | claude-sonnet-5 | `blocked:P1-02` |
| 5 | `P1-03` | REST reader with route/method allow-list | 01 | claude-opus-5 | 🧠 |

**Order rationale:** the remaining Phase 00 unknown comes first because it is an
input to work below it — `P0-09` gates `P2-01`, whose budget limits CLAUDE.md
forbids guessing. `P0-08` closed 2026-08-24: Supervisor's builder confirmed to
use the App's own folder as build context, not the repo root, so
`addon/Dockerfile` cannot build under a real Supervisor App build; the fix is
**F-16**, unqueued pending `devflow plan`. Phase 01 then builds bottom-up:
the connection manager before the allow-list that guards it, the allow-list
before `P1-07`'s deny set (which is meaningless without a table to sit in front
of), and the REST reader after both because it reuses their denial contract.
Phase 02 is deliberately not queued: `P2-01` waits on `P0-09`, and `P2-02` /
`P2-03` need `P1-05`'s domain model to classify and redact.

**Two decisions are waiting**, neither blocking today's queue: the App's
Supervisor permission level (phase 00 — gates Phase 03's tool catalog) and the
MCP transport and client authentication (phase 01 — gates whether Phase 03 needs
an auth subsystem at all).

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
| 00 | Spike & Foundations | 9 / 12 |
| 01 | HA Access & Read-Only Gateway | 1 / 9 |
| 02 | Policy, Privacy, Budget & Audit | 0 / 7 |
| 03 | MCP Server & Inventory Tools | 0 / 8 |
| 04 | History, Statistics & Detection | 0 / 5 |
| 05 | Diagnostics & Evidence Engine | 0 / 1 |
| 06 | Proposal Mode — gated | 0 / 1 |
| 07 | Controlled Change (Admin) — gated | 0 / 1 |

Counts include each phase's `needs-decision` entries, which are boxes too — the
Phase 01 tick is its deny-list decision, not a task.

Phases 00–04 are milestone M1 (v1 observer). Phase 05 is M2. Phases 06–07 are
gated: they open only on an explicit owner decision plus a fresh security review.
Phases 05–07 carry no task boxes yet — theirs are written by `devflow plan` when
the phase before them closes.

Last refreshed: 2026-08-24 (`devflow next` · `P0-08`)

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

F-8 resolved 2026-08-24 by `P0-08` and spawned **F-16** (`addon/Dockerfile`
cannot build under Supervisor's real App builder — `queue-next`, unqueued,
needs a `devflow plan` to pick one of its three named fixes). Remaining open
`queue-next`: F-14 → `P0-09` (the queue's next row) · F-13 → `P1-07` · F-10 and
F-12 → `P2-02` / `P2-03` DoDs · F-11 → `P3-07` DoD plus a Phase 05
degraded-workflow bullet · F-16 → unqueued. F-6 stays `defer` by decision —
Phase 05 owns it and has no boxes yet. Open `unknown`s: F-6 (defer), F-14 (the
queue's next row).

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-08-24 · `P0-08` — Supervisor App builder's build context verified from `home-assistant/supervisor@main` source (`supervisor/apps/build.py`): the builder mounts only the App's own folder read-only as context, never the repo root, so `addon/Dockerfile`'s `COPY go.mod`/`cmd/`/`internal/` cannot resolve under a real Supervisor build; fix filed as F-16, unapplied; `docs/research/2026-08-24-supervisor-addon-build-context.md`.
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
