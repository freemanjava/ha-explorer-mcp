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

- [ ] **`P3-03` — `list_integrations` and `get_integration`** — config-entry
  summary with entity/device/unavailable counts, and the per-integration
  drill-down.
  **DoD:** filtering and cursor pagination honor the Phase 02 contract; counts
  are computed server-side, not by returning the underlying lists; an integration
  in a failed setup state is represented with its state, not omitted.

- [ ] **`P3-04` — `list_devices` and `get_device`** — filtered/paginated device
  inventory and device metadata with related entities and via/parent topology.
  **DoD:** `DeviceRef` is what leaves the boundary, and a test asserts the
  response does not present `device_id` as a stable physical identity (doc §8);
  a device whose entities span availability states reports them accurately; the
  device-disappeared-between-list-and-get case returns `ErrNotFound`, not a
  partially-populated object (Appendix B).

- [ ] **`P3-05` — `list_entities` and `get_entity`** — the Appendix A.1 filter
  set (domain, integration, device_id, area_id, state, availability, category,
  disabled, search) with cursor pagination, plus current state enriched with
  registry, device and area metadata.
  **DoD:** each filter is tested independently and in combination; `limit`
  defaults to 50 and clamps at 200; a `PRIVATE`-classified entity is handled per
  the Phase 02 profile; `search` matching an attribute containing prompt-like
  instruction text returns it as inert data — a test asserts it is never
  interpreted (threat T2).

- [ ] **`P3-06` — `list_areas`, `list_automations`, `list_repairs`,
  `list_apps`** — area/floor/label topology, automation inventory
  with enabled state and `last_triggered`, native HA Repairs/issues, and the
  Supervisor App inventory. `list_apps` is implementable rather than permanently
  `unsupported` because the 2026-08-25 decision grants `/supervisor/info`, whose
  payload embeds the installed-App inventory; that embedded list is its only
  enumeration path, and no `*/stats` is available, so per-App resource use is out
  of scope.
  **DoD:** each is paginated and provenance-stamped; `list_apps` enumerates from
  `/supervisor/info` and returns `unsupported` (not empty) when Supervisor is
  unreachable — a test asserts the two cases are distinguishable in the response;
  repairs are returned with their severity/issue id so an agent can cite them as
  evidence.

- [ ] **`P3-07` — `get_automation` and `get_automation_traces`** — implemented
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

## Phase Definition of Done

- An MCP client connects, lists tools, and completes a full inventory walk:
  overview → integrations → devices → entities → areas → automations → repairs.
- No tool accepts a route, command, SQL, shell, path or code parameter — asserted
  by a test over the registered schemas, so a future tool cannot introduce one.
- Every compatibility-sensitive tool either works or reports `unsupported` with a
  reason; none fabricates.
- `make check` is green.
