# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P3-07` — `get_automation` / `get_automation_traces`
· [`phases/03-inventory-tools.md`](phases/03-inventory-tools.md) · model: **claude-sonnet-5** · flags: —

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P3-07` | `get_automation` / `get_automation_traces` | 03 | claude-sonnet-5 | |
| 2 | `P3-08` | session end is a shutdown, not a crash | 03 | claude-sonnet-5 | `live-verify` |
| 3 | `P2-06` | measure the invocation rate limit | 02 | claude-sonnet-5 | `needs-verify` |
| 4 | `P4-01` | `get_entity_history` | 04 | claude-sonnet-5 | |
| 5 | `P4-02` | availability and outage analysis | 04 | claude-opus-5 | 🧠 |
| 6 | `P4-03` | update-cadence and staleness analysis | 04 | claude-opus-5 | 🧠 |
| 7 | `P4-04` | `get_entity_statistics` | 04 | claude-sonnet-5 | |
| 8 | `P4-05` | `find_unavailable_entities` / `find_stale_entities` | 04 | claude-sonnet-5 | |

**Order rationale (2026-09-05 `plan`).** Phase 03 finishes first: `P3-07` keeps
its written last-in-phase position as the most compatibility-sensitive surface,
and `P3-08` — the F-21 fix, the only new box this `plan` wrote for Phase 03 —
follows it so the phase closes complete rather than with a known non-zero exit
in it. `P2-06` then sits between the phases on purpose: it is the F-20
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
| 03 | MCP Server & Inventory Tools | 7 / 9 |
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
Phase 03's seven ticks are its catalog-scope decision, `P3-01`, `P3-02`,
`P3-03`, `P3-04`, `P3-05` and `P3-06`, closed 2026-09-05. Both totals grew by
one at the 2026-09-05 `plan`, which wrote `P2-06` (the F-20 rate-limit
measurement) and `P3-08` (the F-21 non-zero exit): new work does not reopen a
phase, it simply leaves it with open boxes again.

Phases 00–04 are milestone M1 (v1 observer). Phase 05 is M2. Phases 06–07 are
gated: they open only on an explicit owner decision plus a fresh security review.
Phases 05–07 carry no task boxes yet — theirs are written by `devflow plan` when
the phase before them closes.

Last refreshed: 2026-09-05 (`plan` — Phase 04's five queued behind Phase 03,
two new boxes written from the findings inbox; pointer unchanged at `P3-07`)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 3 · `defer` 2 · `unknown` 3 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

**Inbox drained 2026-09-05.** All three `queue-next` are now planned and each
closes with its task, not before: **F-11** with `P3-07` (row 1), **F-21** with
the new `P3-08` (row 2), **F-20** with the new `P2-06` (row 3). F-20 was
promoted out of `defer` at exactly the revisit point its own outcome named —
`P3-01` closed, so the rate limiter finally runs somewhere a storm can reach it.

Two `defer`s remain, re-triaged and deferred again on grounds that have not
changed: **F-6** — Phase 04 has not closed, so the statistics layer its
verification needs still does not exist · **F-17** — nothing has read statistics
yet, so `P2-01`'s conservative estimate has never bound; `P4-04` is the first
place it can, and that is where to revisit it.

The three open `unknown`s are F-6, F-17 and F-20; only F-20 has a task.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

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
- 2026-09-05 · `P3-03` — `list_integrations` and `get_integration` land:
  `internal/ha` gains `MapUnavailableEntityIDs` and `CoreReader.UnavailableEntityIDs`,
  aggregating `get_states` into the set of unavailable-or-unknown entity ids
  in-process, so the two tools never see the underlying per-entity state
  list; `internal/mcp/integration_tools.go` joins that set with
  `RegistryCache`'s config entries, entities and devices — keyed by
  `ConfigEntryID` — to compute each integration's entity/device/unavailable
  counts server-side; `list_integrations` filters by domain/disabled and
  pages through `internal/page.Paginate` (default 50, max 200, cursor,
  truncated/clamped reported explicitly); `get_integration` returns
  `ha.ErrNotFound` for an unknown id rather than a partial object; a config
  entry in a failed setup state (`State`/`Reason` set) is filtered, sorted
  and returned like any other, never dropped; both tools' schemas are left
  to the SDK's struct-based inference, which closes them with
  `additionalProperties:false` on its own.
- 2026-09-05 · `P3-02` — `get_system_overview` and `get_system_health` land:
  `internal/ha` gains `CoreReader` (get_config/get_states, aggregated
  in-process into `model.StateCounts` so no per-entity list ever leaves the
  boundary) and five typed `SupervisorClient` methods
  (`CoreInfo`/`OSHealth`/`HostDisk`/`ResolutionSummary`/`SelfStats`), each with
  its own strictly-typed mapper; `internal/mcp/system_tools.go` assembles both
  responses behind narrow reader interfaces the two tools depend on, so
  `cmd/server` is the only place that wires concrete `ha` types in;
  `get_system_health` never carries a Core CPU/RAM field (none exists on
  `model.SystemHealth`) and degrades the whole response to
  `Unsupported`+reason the moment any one Supervisor endpoint fails, rather
  than serving a partially-filled report; `cmd/server/main.go` now actually
  connects to Core (`ws://supervisor/core/websocket`) and Supervisor for the
  first time in this project's life.

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
