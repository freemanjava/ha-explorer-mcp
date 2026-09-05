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

**Order rationale.** `P3-01` closed 2026-09-05: the static catalog, the stdio
server and the per-invocation envelope (rate limit, budget, panic recovery,
audit) all exist, and every one of doc §9's twenty rows is registered against a
not-implemented handler. Each remaining Phase 03 task now swaps one or two of
those rows for a real handler — a strictly additive change to a table, never a
branch inside an existing tool — so the queue keeps its written order and
`P3-07` stays last as the most compatibility-sensitive surface. Phase 04's five
follow this queue unbroken: the catalog decision below rules out an interleaved
release cut, so there is no reason to reorder them against Phase 03.

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
| 02 | Policy, Privacy, Budget & Audit | 6 / 7 |
| 03 | MCP Server & Inventory Tools | 7 / 8 |
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
`P3-03`, `P3-04`, `P3-05` and `P3-06`, closed 2026-09-05.

Phases 00–04 are milestone M1 (v1 observer). Phase 05 is M2. Phases 06–07 are
gated: they open only on an explicit owner decision plus a fresh security review.
Phases 05–07 carry no task boxes yet — theirs are written by `devflow plan` when
the phase before them closes.

Last refreshed: 2026-09-05 (`P3-06` closed — `list_repairs` lands; pointer
advanced to `P3-07`)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 2 · `defer` 3 · `unknown` 3 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

**F-22 closed 2026-09-05** by `devflow verify`: `repairs/list_issues` is
confirmed reachable and not admin-gated on `2026.9.0`. See the Active
pointer's note above and
[`2026-09-05-ha-repairs-api.md`](../research/2026-09-05-ha-repairs-api.md).

Two `queue-next` are open. **F-11** was already planned into `P3-07` (now queue
row 6) plus a Phase 05 §13.1 bullet — it closes when `P3-07` closes, not now.
**F-21** is new from `P3-01`: a client that disconnects mid-request makes the
App exit non-zero, and the SDK sentinel that would identify it is in an
`internal/` package. It belongs with the App lifecycle work, not with a tool
task, so it waits for the next `plan`.

All three `defer`s were re-triaged and stay deferred: **F-6** (Phase 05 owns
it) · **F-17** — `P2-01` landed with the conservative batched figure and a
comment saying so, exactly as the deferral anticipated · **F-20** — the
invocation rate limit's constants are still unmeasured, and `P3-01` is what
first wires the limiter into a running server, so the earliest honest point to
measure them is the `plan` after `P3-01` closes — which is now the next one.

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
