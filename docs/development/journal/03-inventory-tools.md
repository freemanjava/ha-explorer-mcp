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
