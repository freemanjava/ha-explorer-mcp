# Phase 01 — HA Access & Read-Only Gateway

**Milestone:** M1 (roadmap "Phase 1 — Observer") · **Target version:** v0.2

> 🧠 **Stronger model recommended for the gateway tasks.** The allow-list is the
> security boundary the whole project rests on (ADR-008). It is the one component
> where a plausible-looking shortcut — a regex instead of an exact set, a
> pass-through for "harmless" commands — silently removes the guarantee the
> product is sold on.

## Goal

A reusable, resilient Home Assistant access layer that **cannot** issue a
mutating call, because no code path capable of one is linked into the binary.
When this phase closes, every upstream request goes through a gateway that
matches it against an explicit allow-list derived from Phase 00's research
files, and an HA restart mid-request causes a clean reconnect rather than a
process failure or a hung tool call.

## Depends On

Phase 00 — the allow-list is derived from `docs/research/`, not from the
architecture doc's illustrative lists.

## Add Under

```text
internal/ha/       # websocket.go, rest.go, gateway.go, errors.go
internal/model/    # normalized domain types
```

## Design Notes

- **Read-only by construction, not by flag** (ADR-008, doc §11). The observer
  build exposes `HAReader` interfaces only. There is no `HAWriter`
  implementation in the tree, and no `write_enabled=false` runtime toggle — a
  toggle is one config mistake away from a write.
- **Fail closed.** An unknown WebSocket command or an unmatched REST route is
  `ErrPolicyDenied` before any bytes leave the process. Never "allow unless
  known-bad".
- **Exact match, never pattern match.** Allow-list entries are exact command
  names and exact route templates with typed parameters. A prefix or regex rule
  is how `config/entity_registry/list` quietly authorizes
  `config/entity_registry/update`.
- **A named deny-list sits in front of the allow-list** (decision below, F-13).
  Fail-closed already denies everything unlisted; the deny-list exists so that
  the commands which are *known* escape hatches — `supervisor/api` above all —
  cannot be admitted by a later allow-list edit, and so the reason they are
  forbidden is written down and asserted instead of implied by absence.
- **The App manifest is not a wall** (F-13). `hassio_api: false` declares intent
  and bounds a future bug's blast radius; it does not stop anything. Core's
  `supervisor/api` WebSocket command reaches Supervisor with Core's own token
  and is gated only on `is_admin`, which this App's principal is. The gateway is
  the only enforcement point that actually holds.
- **Every upstream call carries a context deadline.** No unbounded wait, ever.
  `Manager.Call` applies `defaultCallTimeout` when a caller supplies none — a
  backstop, not a licence to omit one.
- **A connection lost *before* a command is transmitted is not a failure**
  (P1-01). `Call` waits for the next connection and sends there; once bytes are
  on the wire the outcome is final and never re-sent. The distinction is what
  makes an HA restart a non-event for a caller instead of a spurious error, and
  it is safe here only because every command in this binary is a read — a write
  path would have to re-derive it, which is one more reason there is none.
- **HA data is untrusted** (threat T2). Entity attributes, friendly names and log
  text are data. Never branch tool behavior on their content; never let a
  returned string select the next call.
- **The normalized model shields the MCP contract from HA internals** (doc §8).
  Notably an HA `device_id` is not a stable physical-device identity — Core
  2026.8 lets composite devices split. Model it as `DeviceRef`, never as an
  identity key the agent is told to rely on.
- Reconnect uses exponential backoff with jitter, and every recovery action
  increments a counter, so silent degradation is impossible.

## Tasks

- [x] **`P1-01` — WebSocket connection manager** 🧠 — long-lived connection to
  `ws://supervisor/core/websocket`: auth handshake, monotonic message-id
  correlation, per-request context and deadline, concurrent in-flight requests,
  exponential backoff with jitter on drop, and a recovery counter.
  **DoD:** tests against a fake HA WS server covering — a normal request/response
  round-trip; two concurrent requests whose replies arrive out of order resolving
  to the correct callers; a request in flight when the socket closes returning a
  typed error rather than hanging; reconnect after HA restart re-authenticating
  and serving the next request; backoff growing and being bounded; a caller's
  context cancellation freeing the pending slot.

- [x] **`P1-02` — WebSocket command allow-list** 🧠 — an exact-match set of
  permitted read commands, populated from `docs/research/` (P0-04, P0-05), and
  enforced in the only code path that can send a command.
  **DoD:** `TestUnknownCommandDenied` — an unlisted command returns
  `ErrPolicyDenied` and **no bytes are written to the socket** (assert on the
  fake server, not just the return value); `TestMutatingCommandDenied` covers
  `call_service`, `fire_event`, `config/entity_registry/update` and a
  supervisor restart command; a test asserts the allow-list contains no entry
  whose name matches a mutation verb (`update|create|delete|remove|set|call`),
  so a future addition cannot slip one in.

- [x] **`P1-03` — REST reader with route/method allow-list** 🧠 — `GET`-only
  client for `http://supervisor/core/api/`, exact route templates
  (`/config`, `/states`, `/states/{entity_id}`, `/history/period/{ts}`, optional
  logbook), `SUPERVISOR_TOKEN` auth, deadline per request, bounded response read.
  **DoD:** `TestNonGetMethodDenied` for POST/PUT/PATCH/DELETE — denied before the
  request is issued; `TestUnlistedRouteDenied`; a path-traversal-shaped
  `entity_id` (`../../config`) is rejected at parameter validation, not escaped
  and sent; a response exceeding the byte cap is truncated with an explicit
  error, not buffered whole.

- [x] **`P1-04` — Typed HA errors and graceful degradation** — one error taxonomy
  the layers above can branch on: `ErrPolicyDenied`, `ErrBudgetExceeded`,
  `ErrUnsupported`, `ErrUpstreamUnavailable`, `ErrDeadline`, `ErrNotFound`.
  **DoD:** each maps to a distinct MCP-visible status; a test asserts an
  unavailable optional source (Supervisor absent) yields `ErrUnsupported` that a
  caller can degrade on, and never a generic failure that would abort a whole
  diagnostic; a test asserts no error string carries a token or a raw payload.

- [x] **`P1-05` — Normalized domain model** — `Entity`, `DeviceRef`,
  `Integration`, `Area`, `Automation`, `Health`, `Evidence` in `internal/model`,
  with explicit mapping from raw HA payloads. No HA JSON struct is exposed above
  `internal/ha`.
  **DoD:** mapping tests from captured fixtures (`test/fixtures/`) for each type;
  a malformed/partial payload maps to a value with an explicit "partial" marker
  rather than panicking or silently zeroing fields; an oversized or malformed
  Unicode `friendly_name` round-trips safely (Appendix B); `internal/model`
  imports nothing from `internal/ha`.

- [ ] **`P1-06` — Registry cache with TTL and observation time** — TTL caching
  per doc §16 (entity registry 30–60s, device registry / areas ~5min, config
  entries 1–5min, system info ~30s), with every cached value carrying its
  observation timestamp.
  **DoD:** a test asserts a served value exposes its age; a test asserts expiry
  refetches; a test asserts a rename observed after expiry is reflected (the
  stale-registry failure mode in Appendix B); concurrent readers do not trigger
  a thundering herd of upstream fetches.

- [x] **`P1-07` — Deny privileged escape hatches by name** — resolves **F-13**
  and **F-18** (`P1-02` unblocked this on 2026-08-24). Add the deny set decided
  above to `internal/ha/gateway.go`, checked before the allow-list, with
  `supervisor/api` as its first entry and the finding id in the constant's
  comment. Move the gateway check to the point where a frame becomes *sendable*
  rather than where a call begins (**F-18**): today it sits at the top of
  `Manager.Call`, and that `Call` is the only route to `session.write` is
  asserted in a comment, not enforced — a second send site added later would
  bypass both tables silently. Keep the early check in `Call` as well, so a
  denial stays independent of whether HA is reachable. Correct the two places
  that read as though the App manifest were the enforcement point: architecture
  doc §15.2 and the `# Security posture` comment in `addon/config.yaml`.
  **DoD:** `TestGateway_SupervisorAPICommand_Denied` asserts `supervisor/api` is
  refused with `ErrPolicyDenied` and **no bytes reach the fake server**, and that
  it is refused identically whether or not the allow-list is empty — the deny
  does not depend on allow-list contents; a test asserts no deny-set entry also
  appears in the allow-list, so the two tables cannot contradict each other; a
  test asserts the denial reason distinguishes "denied by name" from "not
  allow-listed", so an audit record says which; a test asserts the chokepoint
  property **without going through `Call`** (**F-18**) — a denied command cannot
  be turned into a sendable frame at all, so the guarantee survives a future
  second caller inside `internal/ha`; §15.2 and `addon/config.yaml`
  state that `hassio_api: false` declares intent and bounds blast radius but does
  not prevent Supervisor access, naming the gateway as the enforcement point.

## Decisions

- [x] **Gateway carries a named deny-list, checked before the allow-list** —
  decided 2026-08-23 (planning, from **F-13**)

  **Decision:** `internal/ha/gateway.go` holds a small, explicit deny set of
  known privileged escape hatches — `supervisor/api` first — consulted before
  the allow-list. A denied command returns `ErrPolicyDenied` before any bytes
  are written, exactly as an unlisted one does; the two paths differ only in the
  reason recorded.

  **Why:** An allow-list alone already denies `supervisor/api`, so this is not
  about today's behavior — it is about the edit that comes later. `supervisor/api`
  accepts a free-form `endpoint` *and* a free-form `method` and runs as Core's
  Supervisor user, which Core 2026.8.3 puts in `GROUP_ID_ADMIN`: a single
  command that is both a universal escape hatch and a write path, the two shapes
  CLAUDE.md rules 1 and 2 forbid outright. A guarantee that holds only because
  nobody has typed a name into a table is not a guarantee; a named deny with a
  test makes re-admitting it a deliberate, visible act.

  **Rejected:** *Rely on fail-closed alone* — leaves the project's sharpest
  known hazard undocumented in the code that exists to stop it, and invisible to
  the reviewer of a future allow-list addition. *Enforce it in the App manifest*
  — `hassio_api: false` does not prevent this path at all (F-13); believing it
  does is the mistake, not the fix. *A regex deny on `supervisor/*`* — pattern
  rules on the deny side invite pattern rules on the allow side, which the
  exact-match note above rules out.

  **Consequences:** The deny set is a named constant with the finding id in its
  comment. `P1-07` implements and asserts it; the architecture doc §15.2 wording
  is corrected in the same change.

- [ ] **`needs-decision` — MCP transport and client authentication**
  Q7 from the architecture doc. This decides the network exposure of the whole
  server (threat T5) and cannot be inferred from the code. Options: **stdio only**
  (no listening port; the MCP client must run beside the App — safest, most
  restrictive); **HTTP on the App's internal network with a shared secret**
  (reachable from other Apps / the LAN, needs an auth story); **HTTP behind HA
  Ingress** (authenticated by HA's own session, but constrains which clients can
  connect). The answer determines whether Phase 03 needs an auth subsystem at
  all. Depends on which AI client the owner actually intends to point at this.

## Phase Definition of Done

- No `HAWriter` implementation exists anywhere in the tree (assert by test or by
  a grep in CI, so it stays true).
- Every upstream WebSocket command and REST route is allow-listed, and denial
  happens before transmission — proven by tests that assert on the wire, not on
  the return value.
- `supervisor/api` — and every other entry of the deny set — is refused by name,
  independently of the allow-list and of the App manifest (F-13).
- An HA restart during an in-flight request produces a reconnect and a typed
  error, not a hang and not a crash.
- `make check` is green.
