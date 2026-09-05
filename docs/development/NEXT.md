# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P2-06` — measure the invocation rate limit
· [`phases/02-policy-privacy-audit.md`](phases/02-policy-privacy-audit.md) · model: **claude-sonnet-5** · flags: `needs-verify`

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P2-06` | measure the invocation rate limit | 02 | claude-sonnet-5 | `needs-verify` |
| 2 | `P4-01` | `get_entity_history` | 04 | claude-sonnet-5 | |
| 3 | `P4-02` | availability and outage analysis | 04 | claude-opus-5 | 🧠 |
| 4 | `P4-03` | update-cadence and staleness analysis | 04 | claude-opus-5 | 🧠 |
| 5 | `P4-04` | `get_entity_statistics` | 04 | claude-sonnet-5 | |
| 6 | `P4-05` | `find_unavailable_entities` / `find_stale_entities` | 04 | claude-sonnet-5 | |

**Order rationale (2026-09-05 `plan`, `P3-07` since closed and dropped).**
Phase 03 finishes first: `P3-08` — the F-21 fix, the only box this `plan`
still leaves in Phase 03 — closes it so the phase does not linger with a
known non-zero exit in it. `P2-06` then sits between the phases on purpose: it
is the F-20
measurement, whose revisit point the 2026-08-25 `plan` fixed as "the `plan`
after `P3-01` closes", and Phase 04's tools are the heaviest recorder callers
this server will ever make — writing them against an arrival limit nobody
measured is exactly the guess the finding names. Phase 04's five then run in
their written order, which is a real dependency chain: `P4-01` fetches,
`P4-02`/`P4-03` compute over what it fetches, `P4-04` exposes both as one tool,
and `P4-05` applies them installation-wide. Nothing here is flagged
`needs-verify` on `P2-06`'s result: the rate limit bounds how fast invocations
arrive, not what any Phase 04 tool computes, so a changed constant re-tunes the
envelope without re-opening a box.

**Four decisions settled 2026-08-25.** *Transport:* **stdio only** — no
listening port, no client-auth subsystem, and every log line goes to stderr
because stdout carries the framing (phase 01). *Supervisor:* **`hassio_api:
true` at the default role** — `list_apps` becomes implementable and
`get_system_health` stops being partial; still no `*/stats` and no write-capable
path (phase 00). *Catalog:* **the full twenty before release** — no reduced first
cut; a tool the evidence rules out ships answering `unsupported`, not missing
(phase 03). *HA versions:* **current release only** — one adapter variant, one CI
target, `unsupported` with the detected version outside it (phase 00). Rationale
and rejected alternatives in each phase file.

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
| 02 | Policy, Privacy, Budget & Audit | 6 / 8 |
| 03 | MCP Server & Inventory Tools | 9 / 9 |
| 04 | History, Statistics & Detection | 0 / 5 |
| 05 | Diagnostics & Evidence Engine | 0 / 1 |
| 06 | Proposal Mode — gated | 0 / 1 |
| 07 | Controlled Change (Admin) — gated | 0 / 1 |

Counts include each phase's `needs-decision` entries, which are boxes too. Phase
00 is now fully ticked — its last two were the HA-version and Supervisor
decisions, both settled 2026-08-25. **Phase 01 is now fully ticked** — `P1-08`
closed 2026-08-25: `SupervisorClient` and its own route allow-list in
`internal/ha`, `addon/config.yaml` flipped to `hassio_api: true`. Phase 02's one
remaining open box is the Q10 persistence decision — `P2-05` closed 2026-08-25.
**Phase 03 is now fully ticked** — its catalog-scope decision, `P3-01` through
`P3-08`, the last (`P3-08`, the F-21 exit-code fix) closed 2026-09-05. Both
totals grew by one at the 2026-09-05 `plan`, which wrote `P2-06` (the F-20
rate-limit measurement) and `P3-08`: new work does not reopen a phase, it
simply leaves it with open boxes again — Phase 02 is that phase now, at its
one remaining box.

Phases 00–04 are milestone M1 (v1 observer). Phase 05 is M2. Phases 06–07 are
gated: they open only on an explicit owner decision plus a fresh security review.
Phases 05–07 carry no task boxes yet — theirs are written by `devflow plan` when
the phase before them closes.

Last refreshed: 2026-09-05 (`P3-08` closed — Phase 03 now 9/9, complete —
pointer advanced to `P2-06`; F-21 resolved)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 2 · `defer` 2 · `unknown` 3 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

**F-21 resolved 2026-09-05** by `P3-08`'s close (row 1 dropped from the
queue). Two `queue-next` remain: **F-20** with `P2-06` (row 1, now active) and
**F-23**, filed while closing `P3-07` (the logbook fallback it added bypasses
`internal/redact`'s privacy classification), not yet queued as a task — the
next `plan` should give it one.

Two `defer`s remain, re-triaged and deferred again on grounds that have not
changed: **F-6** — Phase 04 has not closed, so the statistics layer its
verification needs still does not exist · **F-17** — nothing has read statistics
yet, so `P2-01`'s conservative estimate has never bound; `P4-04` is the first
place it can, and that is where to revisit it.

The three open `unknown`s are F-6, F-17 and F-20; only F-20 has a task.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-09-05 · `P3-08` — session end is a shutdown, not a crash, closing
  F-21 and Phase 03: `internal/mcp.Run` stops delegating to
  `(*sdkmcp.Server).Run`, which cannot tell a session-end error from a
  startup failure, and instead calls `srv.Connect` itself — a non-nil error
  from that call is the only case treated as real, every other way an
  established session ends (cancelled context, clean disconnect, a client
  dying mid-request) logs "stopped" at INFO and returns nil; the new
  unexported `run(ctx, srv, logger, transport)` takes an already-built
  `*sdkmcp.Server` so a test can drive a probe tool table over a raw
  `net.Pipe` — `NewInMemoryTransports`'s `ClientSession.Close()` was tried
  first and rejected, since it waits for the client's own in-flight call to
  retire before closing, which deadlocks exactly the "still in flight"
  scenario under test; `cmd/server`'s `run()` lost its
  `io.EOF`/`context.Canceled` special-casing, since `mcp.Run` now owns that
  distinction entirely. Live-verified: a hand-driven real stdio client that
  closed stdin mid-`tools/call` left the process at exit 0, logging
  `"stopped" "reason":"session ended" "detail":"server is closing: EOF"`.
- 2026-09-05 · `P3-07` — `get_automation`/`get_automation_traces` land,
  closing F-11: `internal/ha/automation_commands.go` adds the typed
  `automation/config`/`trace/list`/`logbook/get_events` commands (already
  allow-listed since P0-05) and three `CoreReader` methods;
  `get_automation` maps `automation/config` through the already-existing
  `MapAutomation`; `get_automation_traces` reads `trace/list` scoped to one
  automation into `AutomationTraceSummary`, newest first, through a
  strictly-typed `traceSummaryWire` so a retyped field fails loudly
  (`ErrUnexpectedMessage`) rather than degrading to `Partial`, unlike a
  registry entry; `classifyAutomationError` turns a permission refusal
  (`unauthorized`) into `Unsupported` naming the fallback, and a version
  gap (`unknown_command`) into `Unsupported` naming the detected version —
  kept distinct from each other and from a real Go error (`ErrNotFound`
  included); the permission branch additionally fetches `last_triggered`
  plus `logbook/get_events` live into the response's `fallback_*` fields,
  not merely documented. One gap surfaced closing this box: the logbook
  fallback bypasses `internal/redact`'s privacy classification — filed as
  F-23, left for the next `plan` rather than folded in.
- 2026-09-05 · `P3-06` — `list_areas`, `list_automations`, `list_repairs` and
  `list_apps` land, closing the phase's last inventory-breadth task:
  `list_repairs` was the piece left after F-22's `devflow verify` confirmed
  `repairs/list_issues` reachable at any principal
  ([`2026-09-05-ha-repairs-api.md`](../research/2026-09-05-ha-repairs-api.md));
  `gateway.go` allow-lists it, `internal/model.Repair`/`RepairList` and
  `internal/ha.MapRepairs`/`MapRepair` unwrap its `{"issues": [...]}` object
  shape (not a bare array) and mark an element `Partial` on a missing
  `issue_id`/`domain`/`severity`/`created`; `translation_placeholders` is
  carried as opaque `map[string]any`, defaulted to `{}` rather than `nil`
  because the MCP SDK's schema validation requires the declared `object` type
  even when HA sent nothing; `internal/mcp/repair_tools.go` sorts by
  `issue_id` and pages like every other `list_*` tool, with no filter beyond
  pagination; a failed upstream call is a real tool error, not degraded to
  `Unsupported` — unlike `list_apps`, this surface has no permission-refused
  branch.
- 2026-09-05 · `P3-05` — `list_entities` and `get_entity` land: `internal/ha`
  gains `MapEntityStateValues`/`CoreReader.States`, a per-entity current-state
  read deliberately unlike the aggregate-only readers P3-02/P3-03 added,
  because reporting one entity's state is this pair's whole job;
  `internal/mcp/entity_tools.go` implements the full Appendix A.1 filter set
  (domain, integration — resolved through the entity's config entry —
  device_id, area_id, state, availability, category, disabled, search) over
  the entity registry, pages through `internal/page.Paginate`, and masks each
  returned `State` by handing a synthetic HA-shaped payload to
  `internal/redact.Redactor.Payload` rather than re-implementing
  classification; `get_entity` adds device/area/integration name resolution,
  degrading a dangling reference to an empty name rather than failing; a
  malformed `availability` value is rejected rather than silently matching
  nothing; `search` is a plain case-insensitive substring test, so
  instruction-shaped entity names are never interpreted (threat T2).
- 2026-09-05 · `P3-04` — `list_devices` and `get_device` land: `internal/mcp/device_tools.go`
  filters (area_id/config_entry_id/disabled) and pages `RegistryCache.Devices`
  through `internal/page.Paginate`, returning `DeviceRef` itself — no derived
  counts, no related lists — as `list_devices`' items; `get_device` drills
  into one device by id (`ha.ErrNotFound` when absent, never a partial
  object), joins the entity registry to report each related entity's
  domain/name plus availability computed the same way `P3-03`'s counts were
  (membership in `UnavailableEntityIDs`, never the underlying state list),
  and resolves both directions of `ViaDeviceID` topology (`ViaDevice` up,
  `ChildDevices` down) from the device registry already in hand; a dangling
  `ViaDeviceID` degrades the topology field to nil rather than failing the
  response; both tools' schemas are left to the SDK's struct-based inference.

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
