# Phase 03 — MCP Server & Inventory Tools

**Milestone:** M1 (roadmap "Phase 1 — Observer") · **Target version:** v0.4

## Goal

The server is a working MCP server: an AI client connects, discovers a typed
read-only tool catalog, and can build a structured map of the Home Assistant
installation — system overview, integrations, devices, entities, areas,
automations, repairs and Apps — without any tool accepting a route, command,
path, query or code.

## Depends On

Phase 01 (access layer) and Phase 02 (policy envelope). Tools are registered
only after the envelope exists — a tool added before its budget is a tool that
ships without one.

## Add Under

```text
internal/mcp/      # server.go, system_tools.go, entity_tools.go, automation_tools.go
```

## Design Notes

- **Keep the MCP layer thin** (doc §14.2). A tool function validates input,
  calls a domain service, and formats the result. Domain logic in a tool file is
  logic that cannot be tested without the transport.
- **Tools express the engineer's task, not the upstream endpoint** (ADR-007).
  `find_unavailable_entities` is a tool; `ws_call` is not, and no argument for
  adding one is accepted at this level — it is a change to ADR-007, which is a
  decision, not an implementation choice.
- **Every response carries provenance**: source, observation time, cache age
  where applicable, and an explicit marker when data is partial or a source was
  unsupported (doc §9.1).
- **Unsupported is a first-class result.** A compatibility-sensitive API that is
  unavailable on this HA version returns `unsupported` with a reason. It never
  returns an empty list, and it never returns a plausible reconstruction — a
  fabricated answer is worse than no answer (doc §18).
- Registration is a static table. A tool that is not in it does not exist; there
  is no dynamic registration path an input could reach.
- Start with **Tools** only. Resources are deliberately out of scope (doc §19).

## Tasks

- [x] **`P3-01` — MCP server bootstrap and tool registry** 🧠 — wire the official
  Go SDK over **stdio** (Phase 01's transport decision, 2026-08-25 — no listening
  socket, no auth middleware), the static tool table, and per-invocation budget +
  audit + panic recovery middleware. Route every log sink to **stderr**: stdout
  carries the MCP framing, so a stray write there corrupts the protocol stream.
  **DoD:** a client completes initialize and `tools/list` over stdio; a test
  asserts the registry exposes exactly the expected tool names and that **every**
  registered tool is annotated read-only; a test asserts the configured logger
  writes nothing to stdout at any level, so a future log line cannot corrupt the
  stream; a panic inside a tool returns an error to the client and is audited,
  without killing the server; a test asserts every registered tool receives a
  budget (no tool can be registered without one).

- [x] **`P3-02` — `get_system_overview` and `get_system_health`**
  — root discovery snapshot (version, installation, inventory counts, headline
  health) and the Supervisor-backed resource/service health, built on `P1-08`'s
  reader at the default role: component versions, hostname, machine, arch, Core
  state, own resource use, plus `/os/info`, `/host/info` disk and
  `/resolution/info`. No `/core/stats` — the 2026-08-25 decision does not grant
  it, so Core CPU/RAM is absent by design, not missing by accident.
  **DoD:** overview returns counts without dumping entities (assert the response
  contains no per-entity list); `get_system_health` degrades to `unsupported`
  with a reason when Supervisor is unreachable, and the overview still succeeds
  (Appendix B: Supervisor absent while Core is available); a test asserts the
  response never claims a Core CPU/RAM figure — the field is absent or explicitly
  `unsupported`, never zero.

- [x] **`P3-03` — `list_integrations` and `get_integration`** — config-entry
  summary with entity/device/unavailable counts, and the per-integration
  drill-down.
  **DoD:** filtering and cursor pagination honor the Phase 02 contract; counts
  are computed server-side, not by returning the underlying lists; an integration
  in a failed setup state is represented with its state, not omitted.

- [x] **`P3-04` — `list_devices` and `get_device`** — filtered/paginated device
  inventory and device metadata with related entities and via/parent topology.
  **DoD:** `DeviceRef` is what leaves the boundary, and a test asserts the
  response does not present `device_id` as a stable physical identity (doc §8);
  a device whose entities span availability states reports them accurately; the
  device-disappeared-between-list-and-get case returns `ErrNotFound`, not a
  partially-populated object (Appendix B).

- [x] **`P3-05` — `list_entities` and `get_entity`** — the Appendix A.1 filter
  set (domain, integration, device_id, area_id, state, availability, category,
  disabled, search) with cursor pagination, plus current state enriched with
  registry, device and area metadata.
  **DoD:** each filter is tested independently and in combination; `limit`
  defaults to 50 and clamps at 200; a `PRIVATE`-classified entity is handled per
  the Phase 02 profile; `search` matching an attribute containing prompt-like
  instruction text returns it as inert data — a test asserts it is never
  interpreted (threat T2).

- [x] **`P3-06` — `list_areas`, `list_automations`, `list_repairs`,
  `list_apps`** — area/floor/label topology, automation inventory
  with enabled state and `last_triggered`, native HA Repairs/issues, and the
  Supervisor App inventory. `list_apps` is implementable rather than permanently
  `unsupported` because the 2026-08-25 decision grants `/supervisor/info`, whose
  payload embeds the installed-App inventory; that embedded list is its only
  enumeration path, and no `*/stats` is available, so per-App resource use is out
  of scope.

  **2026-09-05: `list_areas`, `list_automations` and `list_apps` landed.**
  `list_repairs` was briefly suspended on F-22 (`repairs/list_issues` had no
  P0 probe evidence at all — not allow-listed, not in `cmd/spike`, not in any
  research doc, unlike every other command this phase needed) and is now
  unblocked: `devflow verify` confirmed the command exists on `2026.9.0` and
  answers identically for an admin and a non-admin principal — unlike the
  automation surface, it is **not** admin-gated. Evidence:
  [`2026-09-05-ha-repairs-api.md`](../../research/2026-09-05-ha-repairs-api.md).
  `gateway.go` may now allow-list `repairs/list_issues` on the same footing as
  every other entry; `list_repairs` itself remains to implement.

  **2026-09-05: `list_repairs` lands, closing `P3-06`.** `gateway.go`
  allow-lists `repairs/list_issues`; `internal/model.Repair`/`RepairList` and
  `internal/ha.MapRepairs`/`MapRepair` unwrap the `{"issues": [...]}` object
  the research doc observed — not a bare array — and map each element
  permissively, marking it `Partial` on a missing `issue_id`/`domain`/
  `severity`/`created` rather than aborting the scan; `translation_placeholders`
  is carried as an opaque `map[string]any` (rule 6), defaulted to `{}` rather
  than `nil` so the field survives the MCP SDK's schema validation, which
  requires the declared `object` type even when HA sent nothing (a `nil` map
  marshals to JSON `null`); `CoreReader.Repairs` fetches and maps it;
  `internal/mcp/repair_tools.go` sorts by `issue_id` and pages like every
  other `list_*` tool, with no filter beyond pagination (the DoD asks only for
  provenance and per-item severity/issue id); a failed upstream call is a real
  tool error (`IsError`), not degraded to `Unsupported` — unlike `list_apps`,
  this surface has no permission-refused branch to distinguish.
  **DoD:** each is paginated and provenance-stamped; `list_apps` enumerates from
  `/supervisor/info` and returns `unsupported` (not empty) when Supervisor is
  unreachable — a test asserts the two cases are distinguishable in the response;
  repairs are returned with their severity/issue id so an agent can cite them as
  evidence.

- [x] **`P3-07` — `get_automation` and `get_automation_traces`** — implemented
  strictly to the scope P0-05 proved reachable, behind a compatibility adapter
  with feature detection. **Both branches are built regardless of how `P0-06`
  lands** (F-11): `automation/config`, `trace/list`, `trace/get` and
  `trace/contexts` are admin-gated without exception on 2026.8.3, and an App
  that is admin on the owner's installation can be deployed against one where it
  is not.
  **DoD:** on an HA version and principal where the API is present, traces are
  returned with their execution outcome; where the command exists but the
  principal is refused, the tool returns `unsupported` naming *permission* as the
  reason — distinct from the version-unsupported case and from an empty result —
  and points at the degraded evidence path; where the API is absent, the tool
  returns `unsupported` with the detected version and the reason; the degraded
  path itself (`last_triggered` + `logbook/get_events`, both observed working for
  a non-admin principal) is exercised by a test, not merely documented; a test
  with a mutated response shape (simulating an HA upgrade) fails loudly rather
  than mapping garbage into the domain model (Appendix B).

  **2026-09-05: lands, closing F-11.** `gateway.go`'s existing allow-list
  already covered all four commands (P0-05 anticipated them); this task adds
  the typed WS commands (`automationConfigCommand`, `traceListCommand`,
  `logbookGetEventsCommand`, `internal/ha/automation_commands.go`) and three
  `CoreReader` methods. `get_automation` maps `automation/config`'s
  `{"config": {...}}` envelope through the already-existing `MapAutomation`
  (written ahead of this task, at P3-06, and only now wired to a command);
  `get_automation_traces` reads `trace/list` scoped to one automation
  (`domain`/`item_id`, `item_id` derived from the entity id, not
  `trace/get`'s full per-step detail — that command stays allow-listed for a
  future per-run drill-down, out of this box's scope) into
  `model.AutomationTraceSummary`, newest run first, through a strictly-typed
  `traceSummaryWire` so a retyped field fails the call (`ErrUnexpectedMessage`)
  rather than degrading to `Partial` the way a registry entry would — traces
  are evidence, not configuration, and Appendix B's mutated-shape test targets
  this mapper. `classifyAutomationError` (`internal/mcp/automation_tools.go`)
  is the one place that turns a `*ha.CommandError` into the three-way outcome:
  `errors.Is(err, ha.ErrUnsupported)` (HA's `unauthorized`, already unwrapped
  by `manager.go`) is the permission case, naming the fallback tool for
  `get_automation` or this tool's own `fallback_*` fields for
  `get_automation_traces`; a raw `*CommandError` with `Code == "unknown_command"`
  is the version-absent case, naming the version `Options.Core.CoreConfig`
  reports (or "unknown" if `Core` is nil or itself fails — a detail, not a
  reason to fail the tool); anything else propagates as a real Go error
  (`ha.ErrNotFound` for a deleted automation included), never dressed up as a
  fact about the installation. The permission branch additionally fetches the
  degraded evidence live — `last_triggered` from `Options.Automations` (the
  same source `list_automations` reads) and `logbook/get_events` since a 24h
  window via the new `CoreReader.LogbookEvents`/`MapLogbookEvents` — into
  `AutomationTraceList.FallbackLastTriggered`/`FallbackEvents`, satisfying the
  DoD's "exercised by a test, not merely documented" clause; the version-absent
  branch fetches neither, since a dropped `trace/list` does not also change
  `last_triggered` or the logbook. One gap surfaced closing this box: the
  logbook fallback's `Message`/`Name`/`EntityID` cross the boundary without
  `internal/redact`'s classification, unlike every other entity-derived field
  — filed as **F-23** rather than folded in here, since it is absent from this
  DoD and would have widened the box.

- [x] **`P3-08` — Session end is a shutdown, not a crash** `live-verify` — a
  client that dies *while a request is in flight* closes stdin mid-session;
  `Server.Run` then returns the SDK's `jsonrpc2.ErrServerClosing`, `cmd/server`'s
  `run()` does not recognize it, and the App exits 1 (F-21). Under the Supervisor
  a non-zero exit reads as a crash and, with a restart policy, triggers a
  restart. The sentinel is unreachable — it lives in the SDK's `internal/`
  package — so the fix is to treat *any* session-end error after a session was
  established as a normal shutdown logged at INFO, not to match a message string
  or the JSON-RPC code, both of which an SDK release can change under us.
  **DoD:** a test drives the server over a pipe, closes the client end with a
  request in flight, and asserts `run()` returns nil and logs the shutdown at
  INFO; the clean between-requests disconnect still returns nil; an error that is
  *not* a session end (a startup or transport failure before any session was
  established) still returns non-nil, so the fix cannot swallow a real failure;
  observed running (`live-verify`) — a real client killed mid-request leaves the
  process at exit 0.

  **Closed 2026-09-05.** `internal/mcp.Run` no longer delegates to
  `(*sdkmcp.Server).Run`, which conflates the two cases it cannot tell apart;
  it now calls `srv.Connect` itself and treats a non-nil error from that call
  (no session ever established) as the only real failure, while any way an
  established session ends — cancelled context, clean disconnect, or a
  client dying mid-request — logs "stopped" at INFO and returns nil. The new
  unexported `run(ctx, srv, logger, transport)` takes an already-built
  `*sdkmcp.Server` and a `sdkmcp.Transport` rather than `Options`, so a test
  can drive a server built from `probeTable`'s tool table over a raw
  `net.Pipe` — `NewInMemoryTransports()`'s `ClientSession.Close()` was tried
  first and rejected: it waits for the client's own in-flight outgoing call to
  retire before closing, which never happens once the corresponding server
  handler is deliberately parked to simulate "mid-request", so the test
  deadlocked on the wrong half of the scenario. `cmd/server`'s `run()` lost
  its `io.EOF`/`context.Canceled` special-casing entirely — `mcp.Run` now
  owns that distinction, so `cmd/server` only ever sees a non-nil error for a
  real startup failure. Live-verified by hand-driving the built binary over a
  real stdio pipe (`initialize` → `tools/call` → `stdin.Close()` while the
  call is still outstanding): the process logged `"stopped" "reason":
  "session ended" "detail":"server is closing: EOF"` at INFO and exited 0.

- [x] **`P3-09` — `get_automation_traces`' fallback evidence goes through the
  privacy profile** 🧠 — F-23. `attachAutomationFallback`
  (`internal/mcp/automation_tools.go`) returns `model.LogbookEvent`s whose
  `Message`, `Name` and `EntityID` cross the response boundary untouched, while
  every other entity-derived text in the catalog runs through
  `internal/redact.Redactor` keyed by the entity's classification
  (`maskEntityState`, `maskHistoryPoints`). A logbook message embeds the
  human-readable transition of the entity that triggered the run ("… changed to
  home"), so a PRIVATE-classified trigger entity leaks through the degraded path
  exactly where the profile is meant to mask it. `bindGetAutomationTraces` does
  not currently receive `Profile`/`Secrets` at all — the plumbing is part of the
  box.
  **DoD:** a test with a PRIVATE-classified entity (e.g. `device_tracker.*`) in
  the fallback events asserts, under the default mask profile, that neither its
  state text nor its friendly name survives in `FallbackEvents` while `When` and
  `ContextID` do; the same test under the deny profile asserts the placeholder
  path; a PUBLIC entity's message is unchanged except that a configured secret
  appearing in it is stripped; masking is stable within one response (equal
  values share a token) as `maskHistoryPoints` already requires; the existing
  `P3-07` fallback tests still pass.

## Decisions

- [x] **Tool catalog scope for the first usable release — the full twenty** — decided 2026-08-25

  **Decision:** The first release the owner points an agent at is the **full
  twenty-tool catalog**: Phase 03 complete, then Phase 04 complete. No reduced
  first cut, and no tool is dropped to reach a release date. Tools whose evidence
  says they are not implementable at the permitted level are not silently
  dropped either — they ship answering `unsupported` with a reason.

  **Why:** The product is a diagnostic that separates what it observed from what
  it could not check. A partial catalog degrades that promise in a way the agent
  cannot see: an investigation that cannot reach history or statistics does not
  return a smaller answer, it returns a differently-shaped one, and the owner has
  no way to tell which. Shipping the whole catalog keeps "no answer" a real
  answer rather than a missing tool.

  **Rejected:** *Inventory core first* (`P3-01`–`P3-06`, then traces and Phase
  04) — reaches a usable agent sooner and exercises the pipeline end-to-end
  earlier, but produces exactly the half-visible degradation above and invites a
  release that quietly becomes permanent. *A minimal probe set*
  (`P3-01`/`P3-02`/`P3-05`) — fastest to a real session, most re-planning
  afterward, and the least representative of how the tools behave together.

  **Consequences:** Phase 03 keeps its written order — `P3-01` first because
  everything registers into it, `P3-07` last because it is the most
  compatibility-sensitive — and Phase 04 follows without an interleaved release
  cut. Both phases' Definitions of Done stand as written. The tool-count claim is
  a fact that will otherwise drift: `P3-01`'s registry test asserting the exact
  expected tool names is what keeps "twenty" true rather than aspirational.
  `list_apps` and `get_system_health` are in scope at the level the 2026-08-25
  Supervisor decision permits, not above it.

- [x] **`P3-09` — the fallback event is masked whole, not searched for
  substrings** — decided 2026-09-05

  **Decision:** Classify each fallback `model.LogbookEvent` by its own
  `EntityID` through `internal/policy`, and where the classification is PRIVATE
  apply the profile to the event's free text as a unit — `Message` and `Name`
  are replaced by the profile's mask token or denied placeholder, while `When`,
  `ContextID` and the event's presence survive. Every event, at any
  classification, still passes through `Redactor` so a configured secret cannot
  ride out in prose (CLAUDE.md rule 4). Classification stays in
  `internal/policy` and masking in `internal/redact`; `internal/mcp` only keys
  one into the other, as `maskEntityState` already does.

  **Why:** a logbook message is prose HA composed from a friendly name and a
  state, with no field boundary inside it to redact. The PRIVATE-handling
  decision's own principle — the shape in time survives, the meaning does not —
  maps exactly onto this shape: an agent still learns that the automation fired
  and when, and can still correlate by `ContextID`, without reading who was
  where.

  **Rejected:** *substring-matching the entity's friendly name or state inside
  `Message`* — unreliable against HA's phrasings and a direct violation of rule
  6 (never branch behavior on untrusted content). *Dropping PRIVATE events from
  `FallbackEvents`* — silently removes evidence that the automation ran, turning
  "masked" into "did not happen", which is the fabrication rule 7 forbids.
  *Suppressing the whole fallback under a restrictive profile* — removes F-11's
  degraded evidence in precisely the non-admin deployment it was built for.

  **Consequences:** `bindGetAutomationTraces` gains `policy.Profile` and
  `secrets`, matching every other privacy-touching binder, so the tool table
  stays uniform; `get_automation_traces` under a restrictive profile returns
  events whose count and timing are usable and whose text is not.

## Phase Definition of Done

- An MCP client connects, lists tools, and completes a full inventory walk:
  overview → integrations → devices → entities → areas → automations → repairs.
- No tool accepts a route, command, SQL, shell, path or code parameter — asserted
  by a test over the registered schemas, so a future tool cannot introduce one.
- Every compatibility-sensitive tool either works or reports `unsupported` with a
  reason; none fabricates.
- `make check` is green.
