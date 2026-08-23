# CLAUDE.md — HA Inspector MCP

Operational guide and engineering standards for this repository. Loaded
automatically each session. **Keep it accurate: when a rule here stops matching
the code, fix one or the other in the same change.**

> Scope: the whole repository. Product and architecture context lives in
> [`docs/HA_Inspector_MCP_Research_and_Architecture.md`](docs/HA_Inspector_MCP_Research_and_Architecture.md).

## What This Is

A read-only MCP server, written in Go, that lets an AI agent investigate a Home
Assistant installation the way an engineer would: inventory it, find unstable
entities/devices/integrations, analyze history and automation traces, correlate
events, and separate observed facts from hypotheses. It runs as a Home Assistant
App on Home Assistant OS (Raspberry Pi, aarch64) and reaches Core through the
Supervisor proxy.

It is **not** a voice remote and **not** a general HA API proxy. Everyday control
is the official Home Assistant MCP Server's job; this is the diagnostic one.

Read before non-trivial work:

- `docs/development/NEXT.md` — what to do next. The pointer decides, not you.
- `docs/development/METHOD.md` — how work is planned and executed.
- `docs/HA_Inspector_MCP_Research_and_Architecture.md` — the design baseline,
  its threat model (§4) and its ADRs (§24).
- `docs/reference/` — facts kept current. `docs/research/` — dated observations.

## Tech Stack

- **Go 1.25**, `CGO_ENABLED=0`. Ship targets: `linux/arm64` (primary — Raspberry
  Pi) and `linux/amd64`.
- **Official MCP Go SDK** (`github.com/modelcontextprotocol/go-sdk`), pinned to an
  exact version. Never hand-roll the wire protocol.
- Standard library first. A dependency needs a reason; this binary runs on a Pi
  alongside Home Assistant.
- Tests: `testing` + `httptest` + a fake HA WebSocket server. No testify unless a
  task decides otherwise.

## Module Layout

```text
cmd/server/        entry point and wiring — the only place that knows every layer
internal/ha/       HA adapters: websocket, rest, gateway allow-list, typed errors
internal/model/    normalized domain types (Entity, DeviceRef, Integration, …)
internal/analysis/ deterministic metrics: availability, staleness, correlation
internal/policy/   query budget, privacy classification, profiles
internal/redact/   secret stripping at the response boundary
internal/mcp/      MCP server, tool registry, tool definitions — thin
internal/audit/    per-invocation audit records
addon/             Home Assistant App packaging (config.yaml, build.yaml, AppArmor)
test/fixtures/     captured HA payloads
```

**Dependency direction:** `internal/model` imports nothing internal.
`internal/ha` and `internal/analysis` depend on `model`. `internal/mcp` depends on
services and `model`, never on a raw HA payload type. Only `cmd/server` wires
concrete implementations together.

**Parity rule for tools:** every MCP tool has (1) a typed input struct with
validation, (2) a budget class, (3) provenance in its response, (4) a test that
its schema accepts no free-form route/command/path/query. A fifth tool that skips
one of these is how the guarantee erodes.

## Commands

```bash
make check              # build + vet + lint + test — the gate
make test               # tests only
go test ./internal/ha/  # targeted
make test-integration   # integration-tagged, needs a real or recorded HA
make run                # run locally
make release            # cross-build linux/arm64 + linux/amd64
```

Config comes from environment variables (`SUPERVISOR_TOKEN` and the App options
file when running as an App). No config file is read from `/config` — ever.

## Git & Collaboration Rules

- **The agent does not commit, merge, or push.** Its job ends at: tree changed,
  build green, boxes ticked, pointer advanced, summary written.
- **One branch per task**, named for the task id (`feat/P0-01`), cut from `main`.
  `main` is the integration branch.
- Code and its plan updates (`NEXT.md`, phase checkboxes, journal) land in the
  same change, so status never drifts from reality.

---

# Engineering Standards

## The Rules That Are Not Negotiable Here

These are the product, not preferences. Violating one is not a style issue.

1. **No write path exists.** No `HAWriter`, no `call_service`, no `fire_event`,
   no POST/PUT/PATCH/DELETE, in any build. Read-only is enforced by what is
   linked in, not by a runtime flag (ADR-008) — a flag is one config mistake away
   from a write.
2. **No universal escape hatch.** No tool accepts a route, WebSocket command,
   SQL, shell, filesystem path or code. Not "for debugging", not behind a flag.
3. **Fail closed.** An unknown command or unmatched route is denied before any
   bytes leave the process.
4. **`SUPERVISOR_TOKEN` never leaves.** Not in a response, not in a log line at
   any level, not in an error string, not in an audit record.
5. **No `/config`, no Recorder DB, no Docker socket, no host network, no
   `full_access`** (ADR-004, ADR-005).
6. **HA data is untrusted.** Entity attributes, friendly names and log text are
   data. Never branch behavior on their content (threat T2).
7. **Never fabricate.** An unavailable source returns `unsupported` with a
   reason. An empty list means "none", never "could not check".

If a task seems to require breaking one of these, that is a finding for the owner
(`devflow finding`), not a judgement call to make in the moment.

## Design Principles

- **Single responsibility.** `internal/ha/gateway.go` decides only what is
  permitted; it does not fetch, cache, or map. `internal/redact` only strips; it
  does not classify — that is `internal/policy`.
- **Open/closed.** A new tool is a new entry in the static registry table plus a
  new tool file, never a new branch inside an existing tool.
- **Interface segregation.** Reader interfaces are small and per-concern
  (`EntityReader`, `RegistryReader`), not one `HAClient` that can do everything —
  a fat interface is a writer waiting to be added to it.
- **Dependency inversion.** Services take interfaces; `cmd/server` constructs the
  concrete ones. No package reaches for a global client.
- **Immutability at boundaries.** Domain types crossing a layer are values, not
  mutable shared structures. Stateful components (connection manager, cache) own
  their state and guard it explicitly.

## Clean Code

- Small functions, one level of abstraction each; intention-revealing names.
- No magic values — budget limits, TTLs and page sizes are named constants with
  the doc section they came from in a comment.
- Guard clauses and early returns over nesting.
- Unexported by default; export only real API surface.
- No dead code, no speculative generality, no commented-out blocks.
- Comment the *why*. The subtle ones worth a comment: why an allow-list entry
  exists, why a TTL has that value, why a metric is computed that way.

## Naming

- Packages: short, lowercase, no underscores (`policy`, `redact`, `audit`).
- Interfaces name the role, not the implementation: `RegistryReader`, not
  `WebSocketRegistryClient`.
- Errors: `ErrPolicyDenied`, `ErrBudgetExceeded`, `ErrUnsupported`,
  `ErrUpstreamUnavailable`, `ErrDeadline`, `ErrNotFound` — sentinel values,
  compared with `errors.Is`.
- MCP tool names are snake_case and match the doc §9 catalog exactly.
- Tests: `TestSubject_Condition_Expectation`, e.g.
  `TestGateway_UnknownCommand_Denied`.
- Typed ids over bare strings where confusion is possible (`EntityID`,
  `DeviceID`, `ConfigEntryID`).

## Error Handling

- Wrap with `%w` and context; compare with `errors.Is`/`errors.As`.
- **Fail fast on programmer errors**, be tolerant of external data: a malformed
  HA payload maps to a value marked partial and is reported — it never panics a
  long-lived process.
- Never swallow an error silently. Never log-and-return the same error twice.
- An error string must never carry a token, a credential or a raw payload.
- Every upstream call carries a `context.Context` with a deadline. No exceptions.
- "Absent" (`ErrNotFound`), "cannot check" (`ErrUnsupported`) and "refused"
  (`ErrPolicyDenied`) are three different answers and must stay distinguishable
  all the way to the MCP response.

## Logging

- `log/slog`, structured, obtained via injection, not a package global.
- `ERROR` — needs a human. `WARN` — recoverable anomaly (reconnect, degraded
  source). `INFO` — lifecycle only, low cardinality. `DEBUG` — per-message detail,
  off in production.
- Counts and ids, never full payloads at INFO. Never a secret, at any level.
- Every recovery action (reconnect, backoff, cache refill after failure)
  increments a counter, so silent degradation is impossible.

## Concurrency

- One long-lived WebSocket connection, one writer goroutine. Request correlation
  by monotonic id through a guarded map; a caller's context cancellation always
  frees its pending slot.
- Reconnect: exponential backoff with jitter, bounded. Never a tight retry loop.
- Caches are read-mostly with single-flight refill — no thundering herd on expiry.
- Bounded queues with an explicit, logged drop policy. Never unbounded buffering.
- `go test -race` is part of the gate.

## Performance & Resources

The hot constraint is a Raspberry Pi running Home Assistant, not this binary.
Cheapness means: do not ask the recorder for data you will discard, aggregate
before serializing, stream/bound large reads rather than buffering whole
responses, and prefer one bounded query to N per-entity ones. Measure against a
real recorder DB before tuning a limit — the numbers in the architecture doc §10
are starting defaults, explicitly not measurements (doc §26).

## Reliability

- An HA restart is normal, not exceptional: reconnect, re-auth, resume.
- Graceful degradation is required, not optional. Supervisor unavailable must not
  break Core-based diagnostics; it lowers confidence and populates
  `missing_evidence`.
- Validate before trusting; never assume a field survives an HA upgrade.
- A compatibility-sensitive adapter feature-detects and reports its status.

## API & DTO Design

- Raw HA JSON structs stay inside `internal/ha`. Explicit mapping to
  `internal/model` at the boundary — no shortcut of returning the upstream shape.
- Every tool response carries: source, observation time, cache age where
  applicable, and explicit `partial` / `truncated` / `unsupported` markers.
- `list_*`: default limit 50, max 200, cursor pagination, always.
- Fact, inference and recommendation are separate fields, never one prose blob
  (ADR-010).
- Additive changes only within a tool version; a removed or retyped field is a
  new tool version.

## Configuration

Environment variables and the App options file. Defaults live in code as named
constants next to what they bound. Secrets are read once, held in memory, never
written anywhere. Budget limits and the privacy profile are configurable;
read-only-ness is not.

## Testing

- **Test first, against the task's Definition of Done.** A box is not done
  without the tests its DoD names.
- Test files sit beside their source. Unit tests are network-free and use
  captured fixtures from `test/fixtures/`. Anything needing a live HA is behind
  the `integration` build tag.
- Cover the unhappy paths *this* project cares about — the architecture doc's
  Appendix B is the checklist: HA restarts mid-request; recorder slow or timing
  out; entity gone between list and get; an HA upgrade changing a response shape;
  a request storm; attributes containing prompt-like text; malformed or oversized
  Unicode names; private history requested under a restrictive profile;
  Supervisor absent while Core is up; stale cache after a rename; a write
  attempted through a read tool's parameters; a response hitting the byte cap.
- Security properties get *assertions*, not review: token-never-returned,
  mutation-denied-before-transmission, no-free-form-parameter.
- `make check` green is the gate.

## Not Yet — Don't Add Prematurely

Deliberately absent. Each needs a decision, not initiative:

- **Any write capability** — Phase 06/07, gated on owner decision plus a fresh
  security review (ADR-011).
- **`/config` mount, Recorder SQL, Docker socket, host network** — ADR-004,
  ADR-005. Not "later"; not without reopening the ADR.
- **MCP Resources / Prompts** — Tools only in v1, for client compatibility
  (doc §19).
- **A privileged Host Probe** (dmesg, USB, kernel) — a separate optional
  component, disabled by default (ADR-012). Never a capability of this binary.
- **Persistent storage** beyond in-memory cache and audit — open decision in
  phase 02; memory-only until evidence justifies otherwise (Q10).
- **A generic `ws_call` / `rest_get` / "advanced mode" tool** — permanently out.
  This one is not a deferral.
