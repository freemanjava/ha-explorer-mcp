# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P1-04` — Typed HA errors and graceful degradation
· [`phases/01-ha-access-gateway.md`](phases/01-ha-access-gateway.md) · model: **claude-sonnet-5** · flags: —

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P1-04` | Typed HA errors and graceful degradation | 01 | claude-sonnet-5 | |
| 2 | `P1-05` | Normalized domain model | 01 | claude-sonnet-5 | |

**Order rationale:** `P1-03` closed 2026-08-25 — both allow-lists (WebSocket
commands, REST routes) now exist and share one denial contract, so `P1-04`
comes next: it consolidates the error taxonomy the gateway, the manager and
the new REST client currently return between them, and `P1-03` already added
two sentinels (`ErrNotFound`, `ErrResponseTooLarge`) that need a home in it.
`P1-05` follows because Phase 02's `P2-02` / `P2-03` are blocked on its domain
model. Phase 02 stays otherwise unqueued, planned as a block once Phase 01's
readers exist.

**The queue is down to two rows.** Run `devflow plan` after `P1-04` to write
Phase 02's block; all seven open `queue-next` findings are already pinned into
existing DoDs, so this is planning ahead, not draining an inbox.

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
| 00 | Spike & Foundations | 13 / 15 |
| 01 | HA Access & Read-Only Gateway | 5 / 9 |
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

Last refreshed: 2026-08-25 (`devflow next` — `P1-03`)

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

- 2026-08-25 · `P1-03` — REST reader landed in `internal/ha/rest.go` as a `RESTClient` with typed per-route methods (`Config`, `States`, `State`, `HistoryPeriod`, `LogbookPeriod`) and typed option structs instead of any caller-supplied query map; `gateway.go` gained the exact-match route-template table, a GET-only method check consulted ahead of it, and `validateEntityID` (Core's `domain.object_id` shape), so `../../config` is refused at parameter validation rather than escaped and sent. The method check is a backstop, not the guarantee: the client has no method parameter at all, and `TestNoNonGetRequestPathExists` asserts `http.MethodPost/Put/Patch/Delete` never appear in `rest.go`. Oversized bodies are read to cap+1 and refused with `ErrResponseTooLarge` (the test offers an unbounded body, so a client that buffered whole would never return); `ErrNotFound` added for 404. `make check` green.
- 2026-08-25 · `P0-11` — App now ships as a published multi-arch image: `addon/Dockerfile`/`build.yaml` deleted, `addon/config.yaml` carries `image:` with the `{arch}` placeholder, root `Dockerfile` (build stage pinned to `$BUILDPLATFORM` — cross-arch under QEMU segfaulted Go's own `asm`/`compile`) + `.github/workflows/release.yml` publish to GHCR tagged from `version:`. Live-verified on real Home Assistant OS/Raspberry Pi: pulls and starts. Three issues only surfaced there, none caught by `make check` or a local `docker build`: (1) Supervisor refuses a repository without a root `repository.yaml`, undocumented in the phase file's plan — added; (2) `COPY`'d binary wasn't executable — `chmod +x` added for both binary and `run.sh`; (3) even after that, `addon/apparmor.txt` granted the binary only `mr` (mmap+read) — AppArmor denies `exec` as a separate, stricter check than Unix file permissions, invisible to any test that doesn't run under the real profile; fixed to `mrix`. `docs/INSTALL.md` documents the registry-credential step. New tests `TestAddonManifestImageIsPinnedToVersion`, `TestAddonLocalBuildPathRemoved`; `make check` green. F-16 resolved.
- 2026-08-24 · `P0-10` — read `home-assistant/supervisor@main`'s `docker/manager.py`, `docker/interface.py`, `const.py`, `validate.py`, `api/docker.py` plus the frontend's `panels/config/apps/`: Supervisor does support pulling an App image from a private registry, credentials keyed by hostname in `/data/docker.json`, entered at Settings → Add-ons → Registries — confirms the private half of the App-distribution decision and unblocks `P0-11`; a *missing* credential (vs. a wrong one) degrades to a generic, untyped pull error worth a troubleshooting-doc line; `docs/research/2026-08-24-supervisor-private-registry-pull.md`. F-19 answered.
- 2026-08-24 · `P1-07` — named deny set landed in `internal/ha/gateway.go`: `supervisor/api` (F-13) refused before the allow-list, identically whether the allow-list is empty or populated. The chokepoint moved from `Manager.Call` (an unenforced comment, F-18) to `session.write` itself — the last function before `conn.Write` — so a denial no longer depends on `Call` being the only send site; a test constructs a `session` directly, bypassing `Call` entirely, and proves a denied frame still never reaches the socket. Architecture doc §15.2 and `addon/config.yaml` corrected: `hassio_api: false` bounds blast radius but is not the enforcement point. `make check` green.
- 2026-08-24 · `P1-02` — WebSocket command allow-list landed in `internal/ha/gateway.go`: 21 exact-match command names, every one observed answering live HA 2026.8.3 in P0-04/P0-05/P0-07, enforced at the top of `Manager.Call` ahead of session acquisition and frame encoding, so a denial never depends on HA being reachable and no denied command is ever encoded into a frame. New `ErrPolicyDenied` sentinel; the denial tests assert on the fake server's wire rather than the return value, and a guard test rejects any allow-list entry containing a mutation verb; `make check` green.

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
