# Journal — Phase 03 MCP Server & Inventory Tools

Append-only. One entry per closed task, **at most ~5 lines**. Never read whole —
`NEXT.md` carries the last few; this file answers "why on earth is it like that"
months later.

What belongs here is the **surprise**: the environment quirk, the API that
ignores its own documented parameters, the test that had to be shaped oddly. What
changed is already in the diff and the commit message; why it is designed that
way belongs in the phase file's decision record. Only the surprise is
unrecoverable anywhere else — so if there was none, the entry is one line and
that is correct.

---

### 2026-09-05 · P3-01
`internal/mcp`: static twenty-row catalog with a budget class per row, stdio
server, and one receiving middleware carrying rate limit, budget, panic
recovery and audit for every invocation; `cmd/server` now actually runs it.
**Surprise:** the SDK never recovers a panicking handler — nothing in the module
calls `recover()` — so the middleware's recovery is what keeps a nil-map bug
from taking the whole App down. Its `ErrServerClosing` sentinel is also in an
`internal/` package, so a mid-request disconnect cannot be matched with
`errors.Is` (F-21).
**Left open:** every row is bound to a not-implemented handler; `P3-02` onward
replace them one at a time.

### 2026-09-05 · P3-02
`get_system_overview` and `get_system_health` land on `internal/ha`'s new
`CoreReader` and five typed `SupervisorClient` methods; `internal/mcp`
depends on narrow reader interfaces it defines itself, never on `ha` types
directly, so `cmd/server` stays the only place a concrete `ha.Manager` /
`RegistryCache` / `SupervisorClient` gets constructed.
**Surprise:** this is the first task in the project that needed `cmd/server`
to actually connect to Home Assistant — every prior tool row answered
"not implemented" without touching the network, so there was no Core
WebSocket URL constant anywhere yet (`ws://supervisor/core/websocket`, added
here).
**Left open:** `get_system_health` treats any single Supervisor endpoint
failing as the whole tool being unsupported, rather than reporting a
partially-filled health record — simpler, and matches every Supervisor
failure mode this project has evidence for (all-or-nothing at the granted
role), but a future finding if that assumption turns out wrong on a real
deployment.

### 2026-09-05 · P3-03
`list_integrations` and `get_integration` land: per-integration entity/device/
unavailable counts are joined server-side from `RegistryCache`'s config
entries, entities and devices plus a new `CoreReader.UnavailableEntityIDs`,
keyed by `ConfigEntryID` — no per-entity/device list ever leaves the tool.
**Surprise:** with a real typed input struct (the first non-empty one in this
project), the SDK's own `jsonschema` inference closes the schema with
`additionalProperties:false` by default — no explicit schema-pinning needed,
unlike the empty-input tools' belt-and-suspenders `emptyObjectSchema`.
**Left open:** list_integrations filters on domain/disabled only; a richer
filter set (state, search) waits until a task actually needs it.

### 2026-09-05 · P3-04
`list_devices` and `get_device` land: reused the same
`entityAvailabilityReader` from P3-03 to mark each related entity
available/unavailable by set membership rather than adding a second
state-reading path; via/parent topology resolves both directions
(`ViaDevice` up, `ChildDevices` down) from one pass over the already-fetched
device slice.
**Surprise:** none — the P3-03 pattern (typed input, SDK schema inference,
server-side aggregation from registries already in `Options.Inventory`)
transferred directly; `deviceRegistryReader` is a strict subset of
`systemInventoryReader`'s methods, so no `Options` field changed.
**Left open:** list_devices filters on area_id/config_entry_id/disabled only,
same rationale as P3-03's domain/disabled.

### 2026-09-05 · P3-05
`list_entities` and `get_entity` land: the first tool pair whose response
carries a live per-entity value the Phase 02 profile must act on, so each
returned `State` is built by handing `internal/redact.Redactor.Payload` a
synthetic `{entity_id, device_class, state}` object shaped like a real HA
state, rather than re-deriving classification/masking logic in `internal/mcp`.
**Surprise:** the response-boundary redactor existed since Phase 02 but
nothing had called it outside the audit trail — every prior tool's DoD
avoided exposing a raw state value at all (counts, availability booleans),
so this is the first place PRIVATE masking actually runs on a tool response.
**Left open:** `area_id` matches only the entity's own registry field, not
HA's own area-inherited-from-device behavior — the DoD names the filter, not
that inheritance rule, and adding it unasked would be scope beyond the box.

### 2026-09-05 · P3-06
`list_repairs` lands, closing the task: `repairs/list_issues` returns
`{"issues": [...]}`, an object wrapping the array unlike every other
allow-listed command, so `MapRepairs` unwraps it before mapping elements
permissively the same way `MapArea`/`MapAutomationStates` do.
**Surprise:** the MCP SDK's schema validation rejects a `nil` map field
(marshals to JSON `null`) against a declared `object` type — the first model
field of map type to reach a tool response — so `optObject` now defaults to
`{}` rather than `nil` on a missing/malformed source field.
**Left open:** `ignored`/`dismissed_version` are mapped but not exposed as a
filter dimension — the DoD's "not established" note flagged their semantics
as unexercised; add one only when a task actually needs it.

### 2026-09-05 · P3-07
`get_automation`/`get_automation_traces` land, closing F-11: both branches
(permission-refused vs. version-absent `unsupported`) are built regardless of
which one this deployment ever hits, and the permission branch fetches
`last_triggered` + `logbook/get_events` live into the response rather than
only documenting that the fallback works.
**Surprise:** `MapAutomation` already existed, fully tested, since P3-06 —
written ahead of the command that would call it and left unwired until now,
the same way `automation_config.json`'s fixtures predated this task.
**Left open:** `trace/get`'s full per-step detail stays allow-listed but
unused — `trace/list`'s index already carries every field the DoD's
"execution outcome" needs, so a per-run drill-down waits for a task that
actually needs it (likely Phase 05); the logbook fallback bypasses
`internal/redact`'s classification (F-23), left for the next `plan`.

### 2026-09-05 · P3-08
Session end is a shutdown, not a crash (F-21), closing Phase 03: `mcp.Run`
stops delegating to `(*sdkmcp.Server).Run` and calls `srv.Connect` itself, so
any error after a session was established — not just a clean EOF — logs
"stopped" at INFO and returns nil.
**Surprise:** the obvious pipe-based test (`NewInMemoryTransports` +
`ClientSession.Close()`) deadlocks: `Close()` waits for the client's own
in-flight outgoing call to retire before it closes the connection, which
never happens while the corresponding server handler is deliberately parked
to simulate "still in flight" — a raw `net.Pipe` severed directly, bypassing
the SDK's client object, was needed to model a process actually dying.
**Left open:** nothing — DoD's four assertions plus `live-verify` all landed
in this task.
