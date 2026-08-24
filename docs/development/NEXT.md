# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P0-11` — Ship the App as a published multi-arch image
· [`phases/00-spike-foundations.md`](phases/00-spike-foundations.md) · model: **claude-sonnet-5** · flags: `live-verify`

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P0-11` | Ship the App as a published multi-arch image | 00 | claude-sonnet-5 | `live-verify` |
| 2 | `P1-03` | REST reader with route/method allow-list | 01 | claude-opus-5 | 🧠 |
| 3 | `P1-04` | Typed HA errors and graceful degradation | 01 | claude-sonnet-5 | |
| 4 | `P1-05` | Normalized domain model | 01 | claude-sonnet-5 | |

**Order rationale:** `P1-07` closed 2026-08-24 — `supervisor/api` and the rest
of the deny set are now refused by name, in front of the allow-list, at the
point a frame becomes sendable, independently of whether the allow-list holds
anything. `P0-10` / `P0-11` are next because the 2026-08-24 `plan` drained
**F-16** — the App
cannot be installed on real hardware at all, and every task after this one adds
code to a package that has no working deploy path. Both are small and depend on
nothing in Phase 01, so they cost one short detour rather than a reordering;
`P0-10` precedes `P0-11` because the private-registry half of the distribution
decision rests on an unverified fact (**F-19**). Phase 01 then resumes in its
previous order: `P1-03` reuses the denial contract both allow-list tables
establish, `P1-04` consolidates the error taxonomy the gateway and manager
already return between them, and `P1-05` is pulled forward because Phase 02's
`P2-02` / `P2-03` are blocked on its domain model. Phase 02 stays otherwise
unqueued, planned as a block once Phase 01's readers exist.

**Two decisions are waiting**, neither blocking today's queue: the App's
Supervisor permission level (phase 00 — gates Phase 03's tool catalog) and the
MCP transport and client authentication (phase 01 — gates whether Phase 03 needs
an auth subsystem at all). A third was **answered 2026-08-24**: the App ships as
a prebuilt multi-arch image pulled from a private GHCR package, never built by
Supervisor — recorded as **App distribution** in `phases/00-spike-foundations.md`.

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
| 00 | Spike & Foundations | 12 / 15 |
| 01 | HA Access & Read-Only Gateway | 4 / 9 |
| 02 | Policy, Privacy, Budget & Audit | 0 / 7 |
| 03 | MCP Server & Inventory Tools | 0 / 8 |
| 04 | History, Statistics & Detection | 0 / 5 |
| 05 | Diagnostics & Evidence Engine | 0 / 1 |
| 06 | Proposal Mode — gated | 0 / 1 |
| 07 | Controlled Change (Admin) — gated | 0 / 1 |

Counts include each phase's `needs-decision` entries, which are boxes too — one
of Phase 01's three ticks is its deny-list decision, not a task.

Phases 00–04 are milestone M1 (v1 observer). Phase 05 is M2. Phases 06–07 are
gated: they open only on an explicit owner decision plus a fresh security review.
Phases 05–07 carry no task boxes yet — theirs are written by `devflow plan` when
the phase before them closes.

Last refreshed: 2026-08-24 (`devflow verify` — `P0-10`)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 7 · `defer` 2 · `unknown` 2 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

The 2026-08-24 `plan` drained the inbox: **F-16** (the App cannot build under a
real Supervisor) became `P0-11` and spawned **F-19** (can Supervisor pull a
private-registry image at all?), which became `P0-10`. The two `defer`s were
re-triaged and left deferred with the reason recorded on each: F-6 (Phase 05
owns it, no boxes yet) and F-17 (`P2-01` is unwritten, and the error is
conservative). Everything else `queue-next` is already pinned into a DoD:
F-13 and F-18 → `P1-07` (closed 2026-08-24) · F-10 and F-12 → `P2-02` / `P2-03` ·
F-11 → `P3-07` plus a Phase 05 degraded-workflow bullet. Nothing is unqueued.
Open `unknown`s: F-6 and F-17 (both `defer`). F-19 answered 2026-08-24 by
`P0-10` — Supervisor does support private-registry App pulls; see
`docs/research/2026-08-24-supervisor-private-registry-pull.md`. `P0-11` is now
unblocked.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-08-24 · `P0-10` — read `home-assistant/supervisor@main`'s `docker/manager.py`, `docker/interface.py`, `const.py`, `validate.py`, `api/docker.py` plus the frontend's `panels/config/apps/`: Supervisor does support pulling an App image from a private registry, credentials keyed by hostname in `/data/docker.json`, entered at Settings → Add-ons → Registries — confirms the private half of the App-distribution decision and unblocks `P0-11`; a *missing* credential (vs. a wrong one) degrades to a generic, untyped pull error worth a troubleshooting-doc line; `docs/research/2026-08-24-supervisor-private-registry-pull.md`. F-19 answered.
- 2026-08-24 · `P1-07` — named deny set landed in `internal/ha/gateway.go`: `supervisor/api` (F-13) refused before the allow-list, identically whether the allow-list is empty or populated. The chokepoint moved from `Manager.Call` (an unenforced comment, F-18) to `session.write` itself — the last function before `conn.Write` — so a denial no longer depends on `Call` being the only send site; a test constructs a `session` directly, bypassing `Call` entirely, and proves a denied frame still never reaches the socket. Architecture doc §15.2 and `addon/config.yaml` corrected: `hassio_api: false` bounds blast radius but is not the enforcement point. `make check` green.
- 2026-08-24 · `P1-02` — WebSocket command allow-list landed in `internal/ha/gateway.go`: 21 exact-match command names, every one observed answering live HA 2026.8.3 in P0-04/P0-05/P0-07, enforced at the top of `Manager.Call` ahead of session acquisition and frame encoding, so a denial never depends on HA being reachable and no denied command is ever encoded into a frame. New `ErrPolicyDenied` sentinel; the denial tests assert on the fake server's wire rather than the return value, and a guard test rejects any allow-list entry containing a mutation verb; `make check` green.
- 2026-08-24 · `P1-01` — WebSocket connection manager landed in `internal/ha/manager.go`: one long-lived authenticated connection, monotonic id correlation for concurrent out-of-order replies, a per-call deadline with a 30s backstop, bounded backoff-with-jitter reconnect behind a `Reconnects()` counter, and a typed `Command` interface that is the single send path P1-02's allow-list will guard. A flaky reconnect test turned out to be a real race and is fixed; `make check` green.
- 2026-08-24 · `P0-09` — multi-entity cost measured against live HA 2026.8.3 at 1/10/50/200 ids over 24h and 7d: one batched call beats N single-entity ones at every rung (1.4×–50× on time, identical bytes for history), cost tracks recorded rows rather than entity count, and statistics stay 8× smaller and 26× faster than history at 200 ids; `MaxBytes` 512 KB/1 MB, `MaxHistoryPoints` 13k/26k, `MaxEntities` 200 named for `P2-01`; `docs/research/2026-08-24-ha-multi-entity-query-cost.md`.

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
