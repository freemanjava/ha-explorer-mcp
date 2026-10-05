# HA Inspector MCP

A read-only [MCP](https://modelcontextprotocol.io) server that lets an AI agent
investigate a Home Assistant installation the way an engineer would: inventory
it, find unstable entities, devices and integrations, read history and
automation traces, and keep observed facts apart from hypotheses.

It runs as a Home Assistant App on Home Assistant OS and reaches Core through
the Supervisor proxy. Written in Go; ships for `linux/arm64` (Raspberry Pi) and
`linux/amd64`.

**Status:** v1.0.0 — the observer is complete; nothing here can change your
installation.

## What it is not

- Not a voice remote. Turning lights on and running scripts is the job of the
  official Home Assistant MCP server — use that one for control.
- Not a general Home Assistant API proxy. There is no tool that takes a route,
  a WebSocket command, SQL, a shell command, a file path or code.

## The read-only guarantee

- **No write path is linked in.** No service calls, no events, no
  POST/PUT/PATCH/DELETE — in any build, behind no flag.
- **Fail closed.** An unknown command or route is denied before any bytes leave
  the process.
- **No free-form parameters.** Every tool takes typed, validated input.
- **The Supervisor token never leaves.** It is not in a response, a log line, an
  error or an audit record.
- **Home Assistant data is untrusted.** Names, attributes and log text are
  reported as data and never steer the server's behavior.
- An unavailable source is reported as `unsupported` with a reason; an empty
  list means "none", never "could not check".

## Tools

Overview and health

- `get_system_overview` — root snapshot: version, inventory counts, headline health.
- `get_system_health` — Core, OS and Supervisor resource and service health.

Inventory

- `list_integrations` — integrations and config entries with entity, device and unavailable counts.
- `get_integration` — one integration or config entry, including setup state.
- `list_devices` — filtered, paginated device inventory.
- `get_device` — one device with its entities and topology.
- `list_entities` — filtered, paginated entity inventory.
- `get_entity` — current state of one entity with registry, device and area metadata.
- `list_areas` — area topology, with optional floors and labels.
- `list_apps` — Supervisor App inventory and state.
- `list_repairs` — native Repairs and issues with severity.

History and detection

- `get_entity_history` — bounded raw history over an explicit time range.
- `get_entity_statistics` — availability, update cadence and outages for one entity.
- `find_unavailable_entities` — entities unavailable, unknown, or recently flapping.
- `find_stale_entities` — entities whose updates are old or irregular against their own cadence.

Automations

- `list_automations` — automation inventory with enabled state and last trigger.
- `get_automation` — one automation's details.
- `get_automation_logic` — what its triggers, conditions and actions say: thresholds, times and ids as typed values; template text, where the privacy profile allows it, marked untrusted.
- `get_automation_trace` — what each step of one run did, from a run id `get_automation_traces` reports: which condition returned false, as typed values; error text withheld.
- `get_automation_traces` — execution evidence, or the reason traces are unavailable.

Analysis — facts, inferences and recommendations kept in separate fields

- `analyze_entity_health` — deterministic health analysis of one entity.
- `analyze_integration_health` — health and outage correlation for one integration.
- `analyze_automation_health` — run outcomes overlaid on the automation's dependencies' outages.

Every response says where it came from, when it was observed, and whether it is
partial or truncated.

## Install and design

- [`docs/INSTALL.md`](docs/INSTALL.md) — installing the App, setting the secret, connecting a client.
- [`docs/HA_Inspector_MCP_Research_and_Architecture.md`](docs/HA_Inspector_MCP_Research_and_Architecture.md) — threat model, architecture and decisions.
- [`CLAUDE.md`](CLAUDE.md) — the engineering rules this repository is held to.
