# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P0-04` — Verify registry & config-entry read APIs
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
| 1 | `P0-04` | Verify registry & config-entry read APIs | 00 | claude-opus-5 | 🧠 `needs-verify` |
| 2 | `P0-05` | Verify automation config & traces | 00 | claude-opus-5 | 🧠 `needs-verify` |
| 3 | `P0-06` | Verify Supervisor endpoints under minimal perms | 00 | claude-opus-5 | 🧠 `needs-verify` |
| 4 | `P0-07` | Verify recorder history & statistics APIs | 00 | claude-opus-5 | 🧠 `needs-verify` |

**Order rationale:** build the vehicle before driving it — `P0-01`–`P0-03` make a
binary that can actually reach Core, which is what the four verify tasks need in
order to observe anything. The verifications then run in the order the Phase 01
allow-list consumes them: registries first (they gate almost every tool), then
the two compatibility-sensitive areas, then the history source choice that Phase
04 depends on. Nothing from Phase 01 is queued yet: its allow-list is derived
from `P0-04`'s evidence, and queueing it now would invite guessing.

## Status

<!-- DERIVED — do not hand-edit. Regenerate:
for f in docs/development/phases/*.md; do
  printf '%s %s/%s\n' "$(basename "$f")" \
    "$(grep -c '^- \[x\]' "$f")" "$(grep -c '^- \[[ x]\]' "$f")"
done
-->

| phase | theme | done / total |
|------:|-------|:------------:|
| 00 | Spike & Foundations | 4 / 9 |
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

Last refreshed: 2026-08-23 (P0-03 closed)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 6 · `defer` 1 · `unknown` 7

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

F-1 … F-5 are the architecture doc's own open questions (§22), and they are
already assigned to `P0-04`–`P0-07`. That is why the queue is not blocked on them
today — but nothing in Phase 01 may be planned until they are closed.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-08-23 · `P0-03` — Supervisor proxy connectivity: `internal/ha` WS auth handshake + REST GET(`github.com/coder/websocket`), bounded backoff reconnect, integration test against a recorded-HA fake server.
- 2026-08-23 · `P0-02` — App packaging skeleton (config.yaml, build.yaml, Dockerfile, run.sh, AppArmor); arm64 image build verified.
- 2026-08-23 · `P0-01` — pinned `go-sdk` v1.7.0 (protocol 2026-07-28), added CI.

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
