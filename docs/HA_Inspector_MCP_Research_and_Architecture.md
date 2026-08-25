**HA Inspector MCP**

Research, Requirements, Threat Model & Architecture

*AI systems engineer for Home Assistant*

**Baseline target: Raspberry Pi · Home Assistant OS · Go · read-only v1**

Research snapshot verified: 23 August 2026

Document status: Design baseline / implementation input

# 1. Executive Summary

Цель проекта — создать специализированный MCP-сервер, который позволит AI-агенту исследовать Home Assistant как инженер: инвентаризировать систему, находить нестабильные entities/devices/integrations, анализировать историю и automation traces, коррелировать события, отличать факты от гипотез и предлагать улучшения. Это не «голосовой пульт» и не универсальный remote shell.

> Базовое архитектурное решение:

AI Agent  
│ MCP  
▼  
HA Inspector MCP (Go, Home Assistant App)  
│  
├─ Policy / query budget / redaction / audit  
├─ Domain services & diagnostic engine  
├─ HA WebSocket client (primary)  
└─ HA REST client (secondary)  
│  
▼  
Supervisor proxy  
│  
▼  
Home Assistant Core

| **Decision**       | **Baseline**                                                                                               |
|--------------------|------------------------------------------------------------------------------------------------------------|
| Primary language   | Go; official MCP Go SDK. Java/Spring Boot remains a viable secondary option.                               |
| Deployment         | Home Assistant App on Home Assistant OS; same container/binary should remain runnable in Docker/local dev. |
| v1 capability      | Strictly read-only observer/diagnostic server.                                                             |
| Core access        | REST + WebSocket through Supervisor proxy using SUPERVISOR_TOKEN.                                          |
| Filesystem         | No /config mount in v1.                                                                                    |
| Database           | No direct Recorder DB access in v1.                                                                        |
| Privileges         | No Docker socket, host network, host filesystem, shell, full_access or privileged capabilities.            |
| MCP API style      | Typed domain tools; never arbitrary WebSocket/REST/shell proxy.                                            |
| Safety enforcement | Read-only at tool registry + HA gateway allow-list + minimal HA/Supervisor permissions.                    |
| Analysis           | Server performs aggregations/correlations; LLM receives bounded evidence, not massive raw datasets.        |

Первый релиз считается успешным, если AI может безопасно ответить на вопросы типа: «что в системе выглядит нестабильно?», «почему эта automation иногда не срабатывает?», «какие устройства/интеграции дают общие outage-кластеры?» — без права изменять Home Assistant.

# 2. Project Goals and Scope

## 2.1 Primary goals

- Получить структурированную карту Home Assistant: Core/OS/Supervisor, integrations, config entries, devices, entities, areas, automations, Apps и repairs.

- Позволить AI расследовать нестабильность на основе истории, availability, update cadence, registry metadata, traces, repairs и системных метрик.

- Перенести тяжёлые и повторяемые вычисления (aggregation, outage detection, statistics, correlation) из LLM в deterministic server-side code.

- Сформировать evidence-backed выводы, где observation/fact отделены от inference/hypothesis.

- Создать безопасный фундамент для будущего режима proposal/write, не включая его в v1.

## 2.2 Explicit non-goals for v1

- Управление светом, замками, climate и другими entities; для everyday control уже существует официальный Home Assistant MCP Server через Assist.

- Call service, fire event, restart/reload/update/install/delete.

- Редактирование automation, dashboard, YAML или .storage.

- Raw shell, arbitrary HTTP, arbitrary WebSocket command, arbitrary SQL, code execution.

- Доступ к /config, secrets.yaml, Docker socket, host kernel/dmesg.

- Полная observability-платформа или замена Prometheus/Grafana; проект ориентирован на AI-assisted diagnostics.

## 2.3 Relationship with official Home Assistant MCP

Home Assistant уже имеет официальный MCP Server, который предоставляет MCP-клиентам Assist API, real-time context snapshot и ограничение exposed entities. Наш проект не должен дублировать эту функцию. Предлагаемая граница ответственности:

| **Component**               | **Responsibility**                                                                                                        |
|-----------------------------|---------------------------------------------------------------------------------------------------------------------------|
| Official Home Assistant MCP | Everyday interaction/control через Assist; entities ограничиваются exposure settings.                                     |
| HA Inspector MCP            | Engineering/diagnostics: inventory, history, topology, traces, repairs, stability analysis, evidence and recommendations. |

# 3. Requirements Model

## 3.1 Functional requirements

| **ID** | **Requirement**                                                                                     |
|--------|-----------------------------------------------------------------------------------------------------|
| FR-01  | System inventory summary without dumping all raw entities.                                          |
| FR-02  | Filtered/paginated discovery of integrations, devices, entities and areas.                          |
| FR-03  | Current entity state enriched with registry and device metadata.                                    |
| FR-04  | Bounded entity history retrieval for focused investigations.                                        |
| FR-05  | Server-side entity statistics: availability, state-change count, outage duration, update intervals. |
| FR-06  | Detection of unavailable/stale entities.                                                            |
| FR-07  | Automation inventory and execution evidence/traces where supported.                                 |
| FR-08  | Home Assistant Repairs/issues discovery.                                                            |
| FR-09  | System/App health summary when available with minimal Supervisor rights.                            |
| FR-10  | Composite health analysis for entity and integration.                                               |
| FR-11  | Evidence model: source, period/timestamp, observation, confidence and inference separation.         |
| FR-12  | Audit every MCP tool invocation without logging secrets/raw sensitive payloads.                     |
| FR-13  | Apply privacy/redaction policy before data leaves the MCP server.                                   |
| FR-14  | Enforce query budgets, response limits and deadlines.                                               |
| FR-15  | Reconnect safely to HA WebSocket and tolerate HA restarts.                                          |

## 3.2 Non-functional requirements

- aarch64 support and efficient operation on Raspberry Pi.

- Low idle RAM/CPU and fast startup; no heavyweight framework requirement.

- Timeouts/cancellation for every upstream request.

- Pagination and bounded response size.

- No secret/token logging; structured audit and operational logs.

- Back-pressure/rate limiting against accidental LLM request storms.

- Compatibility adapter around undocumented/unstable HA APIs.

- Unit/integration tests and contract tests against supported HA versions.

- Graceful degradation: an unavailable optional source should reduce confidence, not break all diagnostics.

# 4. Threat Model

Trust boundary: MCP instructions/schema/code are trusted; Home Assistant entity attributes, MQTT-originated strings, device names, log text and external integration payloads are untrusted data. The MCP client/LLM is not treated as a trusted administrator.

| **Threat**                     | **Example**                                                          | **Mitigation**                                                                                                |
|--------------------------------|----------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------|
| T1 Expensive query             | AI asks for all entity history for a year.                           | Query budget, max range/entities/points/bytes, timeout, aggregation tools.                                    |
| T2 Prompt injection in HA data | friendly_name/log text contains instructions to the LLM.             | Treat HA content as data, structured schemas, no tool invocation based on raw strings, explicit policy layer. |
| T3 Secret exfiltration         | Token/password/secrets.yaml reaches cloud LLM.                       | No /config; server-side redaction; deny known secret fields; never return SUPERVISOR_TOKEN.                   |
| T4 Privacy leakage             | person/device_tracker/lock history reveals occupancy patterns.       | Data classification, privacy policy, optional masking/denial of private domains, bounded history.             |
| T5 Network exposure            | MCP endpoint exposed directly to Internet/LAN without adequate auth. | Private/controlled transport, authenticated client path, do not expose random unauthenticated port.           |
| T6 Compromised App             | RCE in MCP container escalates to host.                              | No host network, Docker socket, full_access, privileged caps; AppArmor; protection mode; minimal API role.    |
| T7 Compromised MCP client      | Client intentionally calls unexpected HA operation.                  | Typed tool registry + HA Gateway allow-list; no arbitrary API proxy; v1 build lacks writer implementation.    |
| T8 Sensitive audit logs        | Operational logs become secondary data leak.                         | Store tool names/metadata only; redact params; never log token or full response by default.                   |
| T9 API drift                   | HA changes internal/undocumented command shape.                      | Compatibility adapters, feature detection, version matrix and graceful unsupported status.                    |
| T10 Diagnostic overconfidence  | AI converts correlation into claimed root cause.                     | Evidence model, confidence, alternative hypotheses, require explicit fact/inference distinction.              |

# 5. Security Principles and Boundaries

- Least privilege: obtain only the API access required for read diagnostics.

- Defense in depth: HA permissions are not the only barrier; MCP layer and HA Gateway enforce read-only independently.

- No universal escape hatch: no execute_shell, arbitrary_http_request, websocket(command,payload), run_python or read_file(path).

- Fail closed: unknown API routes/commands are denied.

- Secrets never enter the LLM context; redaction happens before MCP response creation.

- Write capability must be a separate later architecture decision; preferably separate process/server (Observer vs Admin).

## 5.1 Privacy classification

| **Class** | **Examples**                                                                           | **Default handling**                                        |
|-----------|----------------------------------------------------------------------------------------|-------------------------------------------------------------|
| NORMAL    | temperature, CPU, generic sensor states                                                | Allowed subject to query limits.                            |
| PRIVATE   | person.\*, device_tracker.\*, locks, alarm, precise zones/location, presence/occupancy | Restricted/masked; policy configurable; avoid bulk history. |
| SECRET    | tokens, passwords, API keys, credentials, secrets.yaml                                 | Never returned; deny/redact at source boundary.             |

# 6. Home Assistant Data Access Matrix

Preferred v1 approach: use Home Assistant typed APIs. Do not read internal files or database schemas if the required data can be obtained through supported interfaces. WebSocket is primary for registries/live data; REST is useful for states/history/logbook; Supervisor is optional for host/App health.

| **Information**               | **REST**         | **WS**            | **Supervisor** | **/config** | **DB** | **Decision**    |
|-------------------------------|------------------|-------------------|----------------|-------------|--------|-----------------|
| HA version/config             | ✓                | —                 | ✓              | —           | —      | v1              |
| Current states                | ✓                | ✓                 | —              | —           | —      | v1              |
| Live state events             | —                | ✓                 | —              | —           | —      | v1              |
| Entity Registry               | —                | ✓                 | —              | —           | —      | v1              |
| Device Registry               | —                | ✓                 | —              | —           | —      | v1              |
| Areas/Floors/Labels           | —                | ✓                 | —              | —           | —      | v1              |
| Config Entries / integrations | partial          | ✓                 | —              | —           | —      | v1              |
| History                       | ✓                | possible/internal | —              | —           | —      | v1              |
| Long-term statistics          | partial/internal | ✓\*               | —              | —           | —      | v1/adapt        |
| Logbook                       | ✓                | —                 | —              | —           | —      | optional        |
| Repairs/issues                | —                | ✓                 | —              | —           | —      | v1              |
| Automations as entities       | ✓                | ✓                 | —              | —           | —      | v1              |
| Automation config             | special/internal | special/internal  | —              | ✓           | —      | adapter/verify  |
| Automation traces             | internal/special | internal/special  | —              | storage     | —      | adapter/verify  |
| Core CPU/RAM                  | —                | —                 | ✓              | —           | —      | v1 if permitted |
| OS/Supervisor info            | —                | —                 | ✓              | —           | —      | v1 if permitted |
| Apps state/info               | —                | —                 | ✓              | —           | —      | v1 if permitted |
| Core/App logs                 | —                | —                 | ✓              | —           | —      | v1.1            |
| Raw YAML/.storage             | —                | —                 | —              | ✓           | —      | NO v1           |
| Recorder SQL                  | —                | —                 | —              | —           | ✓      | NO v1           |
| Kernel/dmesg/USB internals    | —                | —                 | privileged     | —           | —      | NO v1           |

(\*) Some recorder/statistics WebSocket APIs are documented primarily through developer change notices rather than a stable public API reference. Treat them as compatibility-sensitive and isolate them behind adapters.

# 7. Explicit Data-Source Decisions

## 7.1 No /config mount in v1

Even read-only /config access creates avoidable risk: secrets.yaml, .storage, certificates, custom component data and implementation details become reachable. It also forces us to implement path traversal/symlink/allow-list logic. Typed HA APIs are a better boundary.

AI → typed MCP tool → domain service → typed HA API  
  
NOT:  
AI → read_file(path) → /config

## 7.2 No direct Recorder DB in v1

Direct SQLite/MariaDB access couples the project to recorder schema/migrations and credentials, and can cause expensive queries. Prefer REST history and statistics APIs plus server-side aggregation. Direct DB can be reconsidered only if a concrete diagnostic capability cannot be implemented otherwise.

# 8. Internal Data Architecture

Home Assistant  
├─ REST API  
└─ WebSocket API  
│  
▼  
Raw HA adapters  
│  
▼  
Normalized domain model  
Entity / Device / Integration / Automation / Health / Evidence  
│  
▼  
Analysis & correlation services  
│  
▼  
MCP tools

The normalized model shields the MCP contract from Home Assistant internal representation changes. In particular, Home Assistant Core 2026.8 changed device ownership: ordinary devices are restricted to a single config entry (and at most one subentry), and previously composite devices can be split. Therefore an HA device_id must not be treated as a permanent physical-device identity.

type DeviceRef struct {  
HADeviceID string  
ConfigEntryID string  
Platform string  
// Optional privacy-redacted external identity metadata  
}

# 9. MCP v1 Tool Catalog

Public tools should express the engineer’s task, not mirror upstream endpoints. list\_\* tools are filtered and paginated; composite tools aggregate multiple HA sources. Proposed first release:

| **\#** | **Tool**                   | **Purpose**                                                                        |
|--------|----------------------------|------------------------------------------------------------------------------------|
| 1      | get_system_overview        | Root discovery snapshot: version, installation, inventory counts, headline health. |
| 2      | get_system_health          | Core/OS/Supervisor resource and service health where safely available.             |
| 3      | list_integrations          | Integration/config-entry summary with entity/device/unavailable counts.            |
| 4      | get_integration            | Drill-down for one integration/config entry.                                       |
| 5      | list_devices               | Filtered/paginated device inventory.                                               |
| 6      | get_device                 | Device metadata + related entities/topology.                                       |
| 7      | list_entities              | Filtered/paginated entity inventory.                                               |
| 8      | get_entity                 | Current state + entity registry + device/area metadata.                            |
| 9      | get_entity_history         | Bounded raw history for focused time ranges.                                       |
| 10     | get_entity_statistics      | Server-side availability/update/outage statistics.                                 |
| 11     | find_unavailable_entities  | Entities currently unavailable/unknown or with recent availability issues.         |
| 12     | find_stale_entities        | Detect entities whose updates are unexpectedly old/irregular.                      |
| 13     | list_areas                 | Area/topology context; optional floors/labels mapping.                             |
| 14     | list_automations           | Automation inventory/status.                                                       |
| 15     | get_automation             | Automation details through supported/safe adapter.                                 |
| 16     | get_automation_traces      | Execution evidence; compatibility-sensitive adapter.                               |
| 17     | list_repairs               | Native Home Assistant Repairs/issues.                                              |
| 18     | list_apps                  | Supervisor App inventory/state when permitted.                                     |
| 19     | analyze_entity_health      | Composite deterministic entity health analysis.                                    |
| 20     | analyze_integration_health | Composite integration health/outage-correlation analysis.                          |

## 9.1 Tool design rules

- No tool accepts arbitrary route, command, SQL, shell, path or code.

- list\_\* has default limit (e.g. 50), maximum limit (e.g. 200) and cursor pagination.

- History requires explicit entity IDs and bounded time range; no wildcard year-long history.

- Composite tools return summaries/evidence and only necessary raw samples.

- Tool responses include source/period metadata and indicate unsupported/partial data explicitly.

# 10. Query Budget and Resource Protection

Every MCP invocation receives a budget enforced by the application layer, independent of LLM behavior.

type QueryBudget struct {  
MaxHARequests int  
MaxHistoryPoints int  
MaxEntities int  
MaxBytes int64  
Deadline time.Time  
}

| **Class**            | **Illustrative limits**                                                            |
|----------------------|------------------------------------------------------------------------------------|
| Normal read tool     | ≤20 HA requests; ≤10k history points; ≤200 entities; ≤512 KB; ~10 s deadline.      |
| Composite diagnostic | ≤50 HA requests; ≤50k history points; ≤500 entities; ≤1 MB; ~30 s deadline.        |
| Policy               | Actual limits configurable; defaults should protect Raspberry Pi and Recorder I/O. |

# 11. Read-Only Enforcement

Do not rely solely on Home Assistant permissions. Enforce observer semantics twice:

1.  MCP Tool Registry registers only read/analysis tools.

2.  HA Gateway contains an explicit allow-list of permitted REST routes/methods and WebSocket commands.

## 11.1 WebSocket gateway

allowedWS := {  
"config/entity_registry/list",  
"config/entity_registry/get",  
"config/device_registry/list",  
"config_entries/get",  
"repairs/list_issues",  
// plus explicitly reviewed read commands only  
}  
  
unknown or mutating command → ErrPolicyDenied

## 11.2 REST gateway

Observer build should allow only GET against an explicit route allow-list (for example /api/config, /api/states\[/...\], /api/history/period\[/...\], optional logbook). POST/PUT/PATCH/DELETE are denied before reaching Home Assistant.

Prefer a code-level separation: v1 exposes HAReader interfaces only; no HAWriter implementation is linked into the observer build. This is safer than a runtime flag write_enabled=false.

# 12. Diagnostic / Health Engine

This layer differentiates HA Inspector from a thin API wrapper. Deterministic metrics should be calculated in Go; the LLM interprets evidence and generates hypotheses/recommendations.

| **Health object**  | **Candidate signals**                                                                                                                                          |
|--------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Entity Health      | availability ratio; unavailable periods; total/longest outage; update cadence; state-change rate; staleness; anomaly flags; related device/integration health. |
| Device Health      | aggregate entity availability; battery if present; RSSI/LQI if applicable; firmware metadata; via/parent topology; correlated dropouts.                        |
| Integration Health | config-entry state; unavailable-entity ratio; Repairs; reconnection/reload/error evidence where accessible; shared outage clusters.                            |
| Automation Health  | enabled state; last trigger/run; trace failures/condition stops; dependency availability around execution; execution duration when available.                  |
| System Health      | HA uptime/version; resource pressure; recorder/query symptoms; Supervisor/App state; restart correlation.                                                      |

## 12.1 Example entity statistics

{  
"entity_id": "sensor.example",  
"period": "7d",  
"availability_ratio": 0.982,  
"state_changes": 412,  
"unavailable_periods": 7,  
"total_unavailable": "3h12m",  
"longest_unavailable": "54m",  
"median_update_interval_s": 31,  
"p95_update_interval_s": 104  
}

## 12.2 Evidence model

{  
"observation": "entity experienced repeated outages",  
"source": "recorder_history",  
"period": {"from": "...", "to": "..."},  
"evidence": {"outages": 7},  
"confidence": "high",  
"inference": "may be caused by an upstream integration outage"  
}

The system should explicitly preserve: FACT/OBSERVATION ≠ INFERENCE ≠ RECOMMENDATION. An outage correlation is evidence, not automatically a proven root cause.

# 13. Investigation Workflows

## 13.1 “Why did this automation sometimes not run?”

get_automation  
↓  
get_automation_traces  
↓  
identify trigger / condition / action dependencies  
↓  
get_entity_history / statistics for relevant entities  
↓  
check unavailable/stale windows and Repairs  
↓  
correlate timestamps  
↓  
produce ranked hypotheses + evidence + next diagnostic action

## 13.2 “Zigbee devices intermittently disappear”

system overview / health  
↓  
integration health (ZHA or Zigbee-related integration)  
↓  
find unavailable entities/devices  
↓  
cluster outages by time / device / parent topology  
↓  
coordinator / parent device evidence  
↓  
correlate with HA/Supervisor resource/restart evidence  
↓  
rank hypotheses and identify missing privileged evidence, if any

Important boundary: kernel USB resets/dmesg may require a future optional privileged Host Probe. The primary MCP server should not gain host privileges just to investigate rare low-level failures.

# 14. Technology Choice

## 14.1 Preferred: Go

Go is the baseline for this project: small static-ish binary, strong concurrency model, low startup overhead, straightforward cross-compilation for aarch64 and a Tier 1 official MCP SDK supporting the 2026-07-28 protocol.

| **Criterion**                        | **Go**    | **Java/Spring Boot**                          |
|--------------------------------------|-----------|-----------------------------------------------|
| Raspberry Pi footprint               | Excellent | Good; heavier baseline                        |
| Startup/single binary                | Excellent | Good; JVM/native-image choices add complexity |
| Concurrency / WebSocket service      | Excellent | Excellent                                     |
| Official MCP SDK status (Aug 2026)   | Tier 1    | Tier 2                                        |
| Enterprise framework ecosystem       | Good      | Excellent                                     |
| Complex RBAC/data platform evolution | Good      | Excellent                                     |
| v1 recommendation                    | Primary   | Secondary option                              |

## 14.2 Suggested Go package layout

cmd/  
server/main.go  
internal/  
ha/  
rest.go  
websocket.go  
registry.go  
history.go  
automations.go  
supervisor.go  
model/  
entity.go  
device.go  
integration.go  
automation.go  
health.go  
evidence.go  
analysis/  
availability.go  
staleness.go  
correlation.go  
entity_health.go  
integration_health.go  
policy/  
gateway.go  
budget.go  
privacy.go  
redact/  
redact.go  
mcp/  
server.go  
system_tools.go  
entity_tools.go  
automation_tools.go  
diagnostic_tools.go  
audit/  
logger.go

Keep the MCP layer thin. Domain/analysis code should be testable independently of MCP transport and Home Assistant adapters.

# 15. Deployment on Home Assistant OS

Primary deployment target: Home Assistant App managed by Supervisor. The same application should also support standalone Docker/local development to simplify testing and portability.

Raspberry Pi  
└─ Home Assistant OS  
└─ Supervisor  
├─ Home Assistant Core  
├─ MQTT / Zigbee / other Apps  
└─ HA Inspector MCP  
└─ Go binary

## 15.1 Core communication

A Home Assistant App can use the Supervisor proxy to Core. Current official documentation exposes the Core REST proxy and WebSocket proxy, with SUPERVISOR_TOKEN supplied to the App environment. This avoids storing a user-created long-lived token inside the App.

REST: http://supervisor/core/api/  
WebSocket: ws://supervisor/core/websocket  
Auth: SUPERVISOR_TOKEN (never returned/logged)

## 15.2 App permissions baseline

homeassistant_api: true  
hassio_api: true \# default role (hassio_role unset) — decided 2026-08-25, see below  
  
\# Desired security posture:  
\# no docker_api  
\# no host_network  
\# no full_access  
\# protection enabled  
\# AppArmor profile  
\# no /config mapping

The default role is designed around info calls: it grants every Supervisor
path ending in `/info` (`/supervisor/info`, `/os/info`, `/host/info`,
`/resolution/info`, `/network/info`, `/hardware/info`, `/jobs/info`), and
nothing above it — no `*/stats` beyond this App's own, no `/addons`
collection, no `/core/.+` or `/host/.+` write-capable surface. `manager`/`admin`
must not be used for observer v1 (docs/research/2026-08-23-supervisor-permissions.md;
phase 00 "Supervisor permission level" decision).

`hassio_api` (at any role) declares intent and bounds blast radius — it is not
the enforcement point. The App manifest is not a wall: Core still reaches the
Supervisor API through the `supervisor/api` WebSocket command routed over
`homeassistant_api: true` regardless of this flag (F-13). The refusal that
actually matters is `internal/ha/gateway.go`'s named deny set, checked ahead
of the allow-list and enforced at the point a frame becomes sendable (P1-07),
plus the Supervisor REST route allow-list `internal/ha/gateway.go` holds
separately for `SupervisorClient`, which permits only the endpoints above and
denies everything else — including anything the default role's `/.+/info$`
regex would otherwise admit but this project chooses not to request (P1-08).

# 16. Caching, Resilience and Performance

| **Data**                           | **Suggested cache strategy**                                    |
|------------------------------------|-----------------------------------------------------------------|
| Entity registry                    | 30–60 s TTL initially; later event-driven invalidation.         |
| Device registry                    | ~5 min TTL / event-driven invalidation.                         |
| Areas/floors/labels                | ~5 min TTL.                                                     |
| Config entries/integration summary | 1–5 min TTL.                                                    |
| System info                        | ~30 s TTL.                                                      |
| Current states                     | Little/no cache; optional short snapshot cache.                 |
| History aggregates                 | Few-minute TTL keyed by entity+period+resolution.               |
| WebSocket                          | Long-lived connection; reconnect with backoff after HA restart. |

Cache is a load-control mechanism, not a source of truth. Responses should include observation time and optionally cache age so the agent can judge freshness.

# 17. Audit and Operational Observability

{  
"time": "...",  
"tool": "get_entity_history",  
"parameters": {"entity_id": "sensor.example", "period": "24h"},  
"ha_requests": 1,  
"duration_ms": 83,  
"result_bytes": 18342,  
"status": "success"  
}

Do not persist full result bodies by default. Audit should answer “what did the agent request and how expensive was it?” without becoming a copy of private smart-home history.

# 18. Home Assistant API Stability and Compatibility Strategy

Not all useful frontend/internal WebSocket APIs have the same stability/documentation guarantees. Recorder statistics and automation trace/config endpoints require special care. The project should distinguish supported public APIs from compatibility-sensitive adapters.

| **Category**                               | **Policy**                                                                                                     |
|--------------------------------------------|----------------------------------------------------------------------------------------------------------------|
| Stable/documented API                      | Use directly behind typed adapter; regression tests.                                                           |
| Documented only via developer/change notes | Feature-detect and version-test; isolate adapter.                                                              |
| Frontend/internal API                      | Use only when the capability is high value and no safer public API exists; mark experimental; fail gracefully. |
| Filesystem/storage implementation          | Avoid in v1.                                                                                                   |
| Breaking HA release                        | CI compatibility test; clear unsupported feature status rather than silent wrong results.                      |

# 19. MCP Protocol Considerations

As of 2026-08-23, MCP specification 2026-07-28 introduces a stateless protocol core and updated authorization/discovery mechanics. Official Go SDK is Tier 1 and supports this protocol version. The server should use the official SDK rather than implement wire protocol manually, while preserving backward compatibility where the SDK provides it.

Home Assistant’s built-in MCP integration currently supports Tools but not all MCP feature categories when HA acts as an MCP client. For HA Inspector, start with Tools for maximum client compatibility. Resources can be considered later for inspectable URIs if client support becomes valuable.

# 20. Delivery Roadmap

| **Phase**                   | **Scope**                                                                                                        | **Security posture**                                              |
|-----------------------------|------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------|
| Phase 0 — Spike             | Connect Go App to Core REST/WS; prove registry/history/repairs access; enumerate exact permissions/API commands. | No MCP exposure required; local test only.                        |
| Phase 1 — Observer          | 20 read-only tools, policy gateway, budgets, redaction, audit, caching.                                          | No write path, no /config, no DB, minimal privileges.             |
| Phase 2 — Diagnostics       | Entity/integration/device/automation health and cross-event correlation.                                         | Still read-only; evidence model enforced.                         |
| Phase 3 — Proposal          | Generate automation/config/dashboard proposals + validation/diffs but do not apply.                              | No direct apply; explicit proposal artifact.                      |
| Phase 4 — Controlled Change | Separate admin capability: approve → backup → apply → verify → rollback.                                         | Prefer separate Admin MCP/process and stronger confirmation/auth. |
| Optional Host Probe         | Low-level kernel/USB/network diagnostics only if justified.                                                      | Separate optional privileged component, disabled by default.      |

# 21. v1 Acceptance Criteria

- Runs as a Home Assistant App on aarch64 Raspberry Pi under protection mode.

- No /config mount, no Docker socket, no host network, no full_access/privileged permissions.

- Can build system overview and filtered integration/device/entity inventory.

- Can retrieve bounded history and calculate availability/outage/update metrics without direct DB access.

- Can expose Repairs and supported automation execution evidence.

- Every upstream HA command/method is allow-listed; mutation attempts are denied before transmission.

- Query budgets stop oversized workloads and produce explicit budget/limit errors.

- Redaction/privacy tests demonstrate that tokens/secrets cannot be returned.

- Audit records tool invocation cost/metadata without storing full private history.

- HA restart causes safe reconnect rather than process failure.

- Unsupported compatibility-sensitive APIs fail explicitly and do not fabricate data.

- At least three end-to-end investigations produce evidence-backed ranked hypotheses.

# 22. Open Questions / Required Spikes

| **ID** | **Question / spike**                                                                                                               | **Why it matters**                                     |
|--------|------------------------------------------------------------------------------------------------------------------------------------|--------------------------------------------------------|
| Q1     | Exact read-only WebSocket commands and schemas for entity/device/area/floor/label registries on the target HA version.             | Finalize Gateway allow-list.                           |
| Q2     | Exact config-entry/integration read API behavior and admin requirements.                                                           | Determine list/get integration implementation.         |
| Q3     | Automation config and trace retrieval APIs available from an external App in current HA.                                           | May change tool scope or mark experimental.            |
| Q4     | Which Supervisor metrics/App endpoints are available without hassio_api and which require default role.                            | Finalize minimal permissions.                          |
| Q5     | Recorder statistics API stability and best fallback to REST history aggregation.                                                   | Avoid brittle implementation.                          |
| Q6     | Privacy defaults for person/device_tracker/lock/alarm domains.                                                                     | Cloud LLM safety and usability balance.                |
| Q7     | MCP transport/authentication path for the intended AI client(s).                                                                   | Defines network exposure and auth architecture.        |
| Q8     | Supported HA version policy (current only vs N previous releases).                                                                 | Determines CI matrix and adapter complexity.           |
| Q9     | Whether Zigbee-specific metrics (LQI/RSSI/topology) can be normalized across ZHA/Zigbee2MQTT without integration-specific plugins. | Determines diagnostic plugin architecture.             |
| Q10    | Persistence needs beyond audit/cache: SQLite/Bolt/Badger vs memory-only.                                                           | Keep v1 lightweight unless evidence justifies storage. |

# 23. Recommended Implementation Sequence

**1.** Create repository, Go module, CI for linux/amd64 and linux/arm64; pin official MCP Go SDK.

**2.** Build Home Assistant App skeleton with minimal permissions and Supervisor proxy connectivity.

**3.** Implement HA WebSocket connection manager, authentication, reconnect/backoff and request correlation.

**4.** Implement REST reader with route/method allow-list.

**5.** Implement normalized Entity/Device/Integration/Area models and inventory services.

**6.** Add policy engine, query budgets, response-size cap, privacy/redaction and audit before broadening tool set.

**7.** Implement get_system_overview, list/get entities/devices/integrations, list_repairs.

**8.** Implement bounded history + deterministic statistics and stale/unavailable detection.

**9.** Spike automation config/traces and Supervisor metrics; expose only proven read operations.

**10.** Implement analyze_entity_health and analyze_integration_health with evidence model.

**11.** Add contract/integration tests against a real or fixture HA instance and failure-mode tests.

**12.** Only after v1 usage data exists, design proposal/write subsystem separately.

# 24. Architecture Decision Log (ADR Summary)

| **ADR** | **Decision**                                   | **Rationale**                                                            |
|---------|------------------------------------------------|--------------------------------------------------------------------------|
| ADR-001 | Go is the primary implementation language.     | RPi efficiency, simple deployment, Tier 1 MCP SDK.                       |
| ADR-002 | Deploy primarily as Home Assistant App.        | Best fit for HA OS and Supervisor proxy/token lifecycle.                 |
| ADR-003 | v1 is read-only.                               | Build trust and diagnostic value before actuator/config changes.         |
| ADR-004 | No /config mount in v1.                        | Avoid secrets/filesystem attack surface and HA internals coupling.       |
| ADR-005 | No direct Recorder DB.                         | Avoid schema/credential coupling and expensive SQL.                      |
| ADR-006 | WebSocket primary, REST secondary.             | Registries/live model via WS; states/history via REST where appropriate. |
| ADR-007 | Domain-oriented MCP tools.                     | Reduce LLM errors and eliminate arbitrary API proxy.                     |
| ADR-008 | Double read-only enforcement.                  | Tool registry + HA Gateway allow-list.                                   |
| ADR-009 | Server-side analysis first.                    | Reduce LLM context/cost and Pi load; improve determinism.                |
| ADR-010 | Evidence model is first-class.                 | Prevent overconfident root-cause claims.                                 |
| ADR-011 | Observer/Admin separation for future writes.   | Security boundary stronger than a single all-powerful server.            |
| ADR-012 | Optional privileged Host Probe stays separate. | Do not weaken main MCP for rare low-level diagnostics.                   |

# 25. Primary References (verified 2026-08-23)

- Home Assistant — Model Context Protocol Server — [https://www.home-assistant.io/integrations/mcp_server/](https://www.home-assistant.io/integrations/mcp_server/)

- Home Assistant — Model Context Protocol client integration — [https://www.home-assistant.io/integrations/mcp/](https://www.home-assistant.io/integrations/mcp/)

- Home Assistant Developer Docs — REST API — [https://developers.home-assistant.io/docs/api/rest/](https://developers.home-assistant.io/docs/api/rest/)

- Home Assistant Developer Docs — WebSocket API — [https://developers.home-assistant.io/docs/api/websocket/](https://developers.home-assistant.io/docs/api/websocket/)

- Home Assistant Developer Docs — App communication — [https://developers.home-assistant.io/docs/apps/communication/](https://developers.home-assistant.io/docs/apps/communication/)

- Home Assistant Developer Docs — App security — [https://developers.home-assistant.io/docs/apps/security/](https://developers.home-assistant.io/docs/apps/security/)

- Home Assistant Developer Docs — Device Registry — [https://developers.home-assistant.io/docs/device_registry_index/](https://developers.home-assistant.io/docs/device_registry_index/)

- Home Assistant Developer Blog — device registry single config entry (Core 2026.8) — [https://developers.home-assistant.io/blog/2026/07/21/device-registry-single-config-entry/](https://developers.home-assistant.io/blog/2026/07/21/device-registry-single-config-entry/)

- Home Assistant Developer Blog — Recorder statistics API changes — [https://developers.home-assistant.io/blog/2025/10/16/recorder-statistics-api-changes/](https://developers.home-assistant.io/blog/2025/10/16/recorder-statistics-api-changes/)

- Model Context Protocol — 2026-07-28 specification release — [https://blog.modelcontextprotocol.io/posts/2026-07-28/](https://blog.modelcontextprotocol.io/posts/2026-07-28/)

- Model Context Protocol — official SDK tiers — [https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/docs/2026-07-28/sdk.mdx](https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/docs/2026-07-28/sdk.mdx)

- Official MCP Go SDK — protocol documentation — [https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/protocol.md](https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/protocol.md)

- Spring AI — MCP overview (alternative Java/Spring option) — [https://docs.spring.io/spring-ai/reference/api/mcp/mcp-overview.html](https://docs.spring.io/spring-ai/reference/api/mcp/mcp-overview.html)

# 26. Research Notes and Caveats

- This document is a design baseline, not a guarantee that every listed HA internal/frontend command is stable. Exact schemas must be verified during Phase 0 against the deployed Home Assistant version.

- The numbers in illustrative query budgets are starting defaults, not measured Raspberry Pi limits. Benchmark and tune them using the real Recorder database and entity count.

- Automation trace/config and some Recorder statistics interfaces are compatibility-sensitive. Keep them behind adapters and expose an explicit unsupported/experimental status when necessary.

- Home Assistant, MCP and SDKs evolve rapidly. Re-verify reference APIs before implementation milestones and before supporting a new HA release.

- Future write/admin functionality should not silently expand this observer server. Treat it as a new security review and preferably a separate MCP endpoint/process.

# Appendix A. Example Tool Schemas (conceptual)

## A.1 list_entities

list_entities(  
domain?: string,  
integration?: string,  
device_id?: string,  
area_id?: string,  
state?: string,  
availability?: string,  
category?: string,  
disabled?: bool,  
search?: string,  
limit?: int = 50, // max 200  
cursor?: string  
)

## A.2 get_entity_history

get_entity_history(  
entity_id: string,  
from: timestamp,  
to: timestamp,  
resolution?: string,  
minimal?: bool = true  
)  
  
Policy constraints:  
- explicit entity only (or tightly bounded small set in future)  
- max time range  
- max points/bytes  
- query deadline

## A.3 analyze_entity_health

analyze_entity_health(  
entity_id: string,  
period?: duration = "7d"  
) -\> {  
score?,  
observations\[\],  
metrics{},  
evidence\[\],  
hypotheses\[\],  
missing_evidence\[\],  
next_diagnostic_actions\[\]  
}

# Appendix B. Failure Modes to Test

- Home Assistant restarts while a WebSocket request is in flight.

- Recorder history query is slow or times out.

- Entity disappears from registry between list and get.

- Home Assistant upgrade changes a compatibility-sensitive WS response.

- MCP client requests maximum page repeatedly / request storm.

- Entity attributes contain strings resembling prompt instructions.

- Entity/device names contain very large or malformed Unicode strings.

- Private entity history is requested by an unapproved policy profile.

- Supervisor API permission is absent even though Core is available.

- Cache contains old registry data after entity rename/move.

- LLM asks for a write-like action using a read tool parameter trick.

- Response approaches max bytes after aggregation.

# Appendix C. Future Controlled-Change Architecture (not v1)

AI proposes change  
↓  
structured proposal + reason + risk  
↓  
validation / config check / diff  
↓  
explicit user approval  
↓  
backup / snapshot where appropriate  
↓  
apply via narrow typed operation  
↓  
reload/restart only if required  
↓  
verify expected outcome  
↓  
rollback on failed verification

High-impact operations (system update/restart, delete integration/entity, network changes, restore backup, Supervisor admin operations) require their own policy/confirmation model and should never become generic tool capabilities.

# 27. Conclusion

The recommended project is not a generic Home Assistant MCP wrapper. It is a small, controlled, read-only engineering service that converts Home Assistant’s operational data into bounded, privacy-aware evidence for an AI agent. Go + Home Assistant App + Core REST/WebSocket + minimal Supervisor access provides a strong fit for Raspberry Pi/Home Assistant OS. The design deliberately avoids filesystem/database/host privileges and puts policy enforcement between the LLM and every Home Assistant API call.

The immediate next technical milestone is Phase 0: verify the exact read APIs/permissions on the target Home Assistant release, build the Gateway allow-list, and prove the first inventory/history/repairs tool chain. Only after that should the full MCP surface be implemented.
