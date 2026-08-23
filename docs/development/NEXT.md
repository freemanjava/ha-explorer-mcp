# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P0-06` — Verify Supervisor endpoints under minimal permissions
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
| 1 | `P0-06` | Verify Supervisor endpoints under minimal perms | 00 | claude-opus-5 | 🧠 `needs-verify` |
| 2 | `P0-07` | Verify recorder history & statistics APIs | 00 | claude-opus-5 | 🧠 `needs-verify` |
| 3 | `P0-08` | Verify the Supervisor App builder's build context | 00 | claude-sonnet-5 | `needs-verify` |

**Order rationale:** the verifications run in the order the Phase 01 allow-list
consumes them. Registries (`P0-04`) and automations (`P0-05`) are done; next is
the Supervisor permission surface, then the history source choice Phase 04
depends on. `P0-06` is still the highest-stakes: `P0-05` proved every automation
command is admin-gated, so whether `get_automation` and `get_automation_traces`
exist at all turns on whether the App's principal is admin. `P0-08` is last of
the three because it gates deployment, not design — nothing downstream is
written against its answer, and it is the only queue row that runs on the
default model. Nothing from Phase 01 is queued yet: its allow-list still rests
on `P0-06`–`P0-07`.

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
| 00 | Spike & Foundations | 6 / 10 |
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

Last refreshed: 2026-08-23 (`devflow plan` — inbox drained)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 8 · `defer` 1 · `unknown` 5 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

Every open `queue-next` finding now names the task it became, so none is
unplanned: F-2 and F-4 → `P0-06` · F-5 → `P0-07` · F-8 → `P0-08` (new) ·
F-10 and F-12 → `P2-02` / `P2-03` DoDs · F-11 → `P3-07` DoD plus a Phase 05
degraded-workflow bullet. They stay open until those tasks close. F-1, F-3 and
F-9 are **resolved** (F-9's fix landed inside `P0-04`; its state was corrected
here). F-6 was re-triaged and deliberately stays `defer` — Phase 05 owns it and
has no boxes yet.

Nothing in Phase 01 may be planned until F-4 and F-5 close: its allow-list is
the project's primary security boundary and would otherwise be written from an
unverified list.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-08-23 · `P0-05` — automation config & traces verified against live HA 2026.8.3: all commands exist and work, all are **admin-gated**; fallback is `last_triggered` + `logbook/get_events`; `docs/research/2026-08-23-ha-automation-traces.md`.
- 2026-08-23 · `P0-04` — registry & config-entry APIs verified against live HA 2026.8.3, admin and non-admin; only `config_entries/get_single` needs admin; `docs/research/2026-08-23-ha-registry-apis.md`.
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
