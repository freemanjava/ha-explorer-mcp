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

- [ ] **`P3-01` — MCP server bootstrap and tool registry** 🧠 — wire the official
  Go SDK, the transport chosen in Phase 01's decision, the static tool table, and
  per-invocation budget + audit + panic recovery middleware.
  **DoD:** a client completes initialize and `tools/list`; a test asserts the
  registry exposes exactly the expected tool names and that **every** registered
  tool is annotated read-only; a panic inside a tool returns an error to the
  client and is audited, without killing the server; a test asserts every
  registered tool receives a budget (no tool can be registered without one).

- [ ] **`P3-02` — `get_system_overview` and `get_system_health`** — root
  discovery snapshot (version, installation, inventory counts, headline health)
  and the Supervisor-backed resource/service health from P0-06's evidence.
  **DoD:** overview returns counts without dumping entities (assert the response
  contains no per-entity list); `get_system_health` degrades to `unsupported`
  with a reason when the Supervisor permission established in P0-06 is absent,
  and the overview still succeeds (Appendix B: Supervisor absent while Core is
  available).

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
  `list_apps`** — area/floor/label topology, automation inventory with enabled
  state and `last_triggered`, native HA Repairs/issues, and the Supervisor App
  inventory where permitted.
  **DoD:** each is paginated and provenance-stamped; `list_apps` returns
  `unsupported` (not empty) when Supervisor access is unavailable; repairs are
  returned with their severity/issue id so an agent can cite them as evidence.

- [ ] **`P3-07` — `get_automation` and `get_automation_traces`** — implemented
  strictly to the scope P0-05 proved reachable, behind a compatibility adapter
  with feature detection.
  **DoD:** on an HA version where the API is present, traces are returned with
  their execution outcome; on one where it is not, the tool returns
  `unsupported` with the detected version and the reason; a test with a mutated
  response shape (simulating an HA upgrade) fails loudly rather than mapping
  garbage into the domain model (Appendix B).

## Decisions

- [ ] **`needs-decision` — Tool catalog scope for the first usable release**
  The doc lists twenty tools. Whether the first release the owner actually points
  an agent at is the full twenty, or a smaller set exercised end-to-end first,
  changes the order of this phase and Phase 04. Revisit once Phase 00's evidence
  says which tools are implementable at all — some may be re-scoped by P0-05/P0-06
  before this question is even well-formed.

## Phase Definition of Done

- An MCP client connects, lists tools, and completes a full inventory walk:
  overview → integrations → devices → entities → areas → automations → repairs.
- No tool accepts a route, command, SQL, shell, path or code parameter — asserted
  by a test over the registered schemas, so a future tool cannot introduce one.
- Every compatibility-sensitive tool either works or reports `unsupported` with a
  reason; none fabricates.
- `make check` is green.
