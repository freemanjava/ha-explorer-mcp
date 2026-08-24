# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P1-02` — WebSocket command allow-list
· [`phases/01-ha-access-gateway.md`](phases/01-ha-access-gateway.md) · model: **claude-opus-5** · flags: 🧠

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P1-02` | WebSocket command allow-list | 01 | claude-opus-5 | 🧠 |
| 2 | `P1-07` | Deny privileged escape hatches by name | 01 | claude-sonnet-5 | `blocked:P1-02` |
| 3 | `P1-03` | REST reader with route/method allow-list | 01 | claude-opus-5 | 🧠 |
| 4 | `P1-04` | Typed HA errors and graceful degradation | 01 | claude-sonnet-5 | |

**Order rationale:** `P1-01` closed 2026-08-24, so the queue continues Phase 01
bottom-up: the allow-list now has a single send path to sit in front of
(`Manager.callOn` → `session.write`), `P1-07`'s deny set needs that table to
precede, and the REST reader follows because it reuses their denial contract.
`P1-04` is pulled in to keep the queue at four rows: `Manager` already returns
`ErrUpstreamUnavailable` / `*CommandError` / `context` errors as three distinct
answers, which is the taxonomy's starting shape. Phase 02 stays unqueued —
`P2-02` / `P2-03` need `P1-05`'s domain model, and the phase is planned as a
block once Phase 01's readers exist. `P0-08` left **F-16** (`addon/Dockerfile`
cannot build under a real Supervisor App build) unqueued pending `devflow plan`.

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
| 00 | Spike & Foundations | 10 / 12 |
| 01 | HA Access & Read-Only Gateway | 2 / 9 |
| 02 | Policy, Privacy, Budget & Audit | 0 / 7 |
| 03 | MCP Server & Inventory Tools | 0 / 8 |
| 04 | History, Statistics & Detection | 0 / 5 |
| 05 | Diagnostics & Evidence Engine | 0 / 1 |
| 06 | Proposal Mode — gated | 0 / 1 |
| 07 | Controlled Change (Admin) — gated | 0 / 1 |

Counts include each phase's `needs-decision` entries, which are boxes too — one
of Phase 01's two ticks is its deny-list decision, not a task.

Phases 00–04 are milestone M1 (v1 observer). Phase 05 is M2. Phases 06–07 are
gated: they open only on an explicit owner decision plus a fresh security review.
Phases 05–07 carry no task boxes yet — theirs are written by `devflow plan` when
the phase before them closes.

Last refreshed: 2026-08-24 (`devflow next` · `P1-01`)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 5 · `defer` 2 · `unknown` 2 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

No findings were filed or closed by `P1-01`. F-14 resolved 2026-08-24 by `P0-09` and spawned **F-17** (a batched statistics
answer is ~30% larger than the same ids fetched singly — unexplained, `defer`,
conservative in the direction that matters). Remaining open `queue-next`:
F-13 → `P1-07` · F-10 and F-12 → `P2-02` / `P2-03` DoDs · F-11 → `P3-07` DoD plus
a Phase 05 degraded-workflow bullet · F-16 → unqueued, needs a `devflow plan` to
pick one of its three named fixes. Open `unknown`s, both `defer`: F-6 (Phase 05
owns it, no boxes yet) and F-17.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-08-24 · `P1-01` — WebSocket connection manager landed in `internal/ha/manager.go`: one long-lived authenticated connection, monotonic id correlation for concurrent out-of-order replies, a per-call deadline with a 30s backstop, bounded backoff-with-jitter reconnect behind a `Reconnects()` counter, and a typed `Command` interface that is the single send path P1-02's allow-list will guard. A flaky reconnect test turned out to be a real race and is fixed; `make check` green.
- 2026-08-24 · `P0-09` — multi-entity cost measured against live HA 2026.8.3 at 1/10/50/200 ids over 24h and 7d: one batched call beats N single-entity ones at every rung (1.4×–50× on time, identical bytes for history), cost tracks recorded rows rather than entity count, and statistics stay 8× smaller and 26× faster than history at 200 ids; `MaxBytes` 512 KB/1 MB, `MaxHistoryPoints` 13k/26k, `MaxEntities` 200 named for `P2-01`; `docs/research/2026-08-24-ha-multi-entity-query-cost.md`.
- 2026-08-24 · `P0-08` — Supervisor App builder's build context verified from `home-assistant/supervisor@main` source (`supervisor/apps/build.py`): the builder mounts only the App's own folder read-only as context, never the repo root, so `addon/Dockerfile`'s `COPY go.mod`/`cmd/`/`internal/` cannot resolve under a real Supervisor build; fix filed as F-16, unapplied; `docs/research/2026-08-24-supervisor-addon-build-context.md`.
- 2026-08-23 · `P0-07` — recorder history & statistics verified against live HA 2026.8.3: statistics are 1–3 orders of magnitude cheaper than history (7d of one entity: 794 B vs 3.5 MB unfiltered REST); source order is statistics → WS `history/history_during_period` → REST `/api/history/period` as fallback, `minimal_response` always set; `docs/research/2026-08-23-ha-history-statistics.md`.
- 2026-08-23 · `P0-06` — Supervisor permission matrix derived at pinned tags (Supervisor 2026.08.0 / Core 2026.8.3): `list_apps` needs `hassio_api: true` at the *default* role, `get_system_health` is partial without it; the App's Core principal is **admin**; `docs/research/2026-08-23-supervisor-permissions.md`.

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
