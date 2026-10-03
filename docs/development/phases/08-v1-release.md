# Phase 08 — v1.0 Release

**Milestone:** M2 close (roadmap "Phase 2 — Diagnostics", shipped) · **Target version:** v1.0

> Numbered 08 only because 06 and 07 are taken by the gated milestones; it runs
> **before** them. Order lives in `NEXT.md`'s queue, not in phase numbers.

## Goal

Ship v1.0: the doc §21 acceptance criteria met **on the real target** — an
aarch64 Raspberry Pi running Home Assistant OS, the App under protection mode —
with each criterion mapped to a test or a live observation, and an MCP client
actually able to reach the server. Phase 06's gate needs "v1 usage data", which
only exists after this.

## Depends On

Phase 05 (complete 2026-10-03). Opened by the owner at the 2026-10-03 `plan`.

## Add Under

```text
addon/                 # version bump; D-08-1's port (closed by default), secret option, AppArmor
cmd/server/            # transport selection and config (P8-02)
internal/mcp/          # transport_http.go — Streamable HTTP beside stdio (P8-02)
Dockerfile             # -X main.version from addon/config.yaml (P8-10)
docs/INSTALL.md        # how a client connects — absent today (F-37)
docs/research/         # the P8-01 observation and the P8-05 acceptance report
internal/ha/           # gateway_test.go — P8-03's reachability assertion
```

## Design Notes

- **Nothing has been observed running as an App with a client attached.** Every
  journal entry through phase 05 says "not observed against a live HA", and
  `cmd/measure` ran the tools in-process off-box. The stdio-only decision
  (phase 01) says the client "is connected to the process's stdin/stdout by
  whoever starts it" — but under Supervisor the starter is Supervisor, and no
  client is attached (F-37). That is the first thing this phase establishes,
  by observation, before anyone decides how to fix it.
- **Mapping, not re-testing.** Most §21 criteria are already asserted by tests
  (`TestGateway_*`, token-never-returned, budget refusals, the three
  `investigation_test.go` walks). The acceptance report cites those tests by
  name and adds live observations only where a test cannot stand in: the
  platform, protection mode, an HA restart, a real investigation.
- **The agent does not tag or push.** A release box ends at a tree ready to tag
  (`addon/config.yaml` version bumped, docs updated); the owner pushes the tag
  that triggers `release.yml`.

## Tasks

Ordered by dependency. `P8-01` comes first because its answer may change what
every later box has to do; `P8-03`, `P8-07` and `P8-04` are independent of the Pi
and run while the owner is busy there.

- [x] **`P8-01` · Observe the App under Supervisor — does it stay up, can a
  client reach it (F-37)** — `needs-verify`
  Cheap half first, off-box: run the published-image build locally with stdin
  **not** attached (`docker run` without `-i`) and record what the process does
  (expected, unverified: reads EOF at once and exits 0). Then on the Pi, with an
  image of current `main` installed (the owner publishes a pre-release, e.g.
  `0.9.0`; the agent prepares the version bump only): App state after start,
  Supervisor log lines, restart/watchdog behaviour, and whether any client path
  exists today — `docker exec -i` from the SSH App, Supervisor's `stdin: true`.
  The owner runs the Pi half and pastes the output; no token reaches the agent.
  **DoD:** a dated report in `docs/research/` stating, for each observation,
  what was run and what happened, and listing the candidate client paths with
  what each one was observed to require. No code change.
  **Done 2026-10-03:** exits 0 ~65 ms after start and stays stopped (F-37 confirmed
  on the Pi); stdio works with stdin held (off-box); `docker exec -i` unavailable
  from the SSH App as shipped; `stdin: true` untried (needs an image change).
  Report: `docs/research/2026-10-03-app-under-supervisor.md`.

- [x] **`P8-03` · The allow-list matches the reachable surface (F-25)**
  Delete `recorder/list_statistic_ids`, `recorder/get_statistics_metadata` and
  `recorder/statistics_during_period` from `allowedCommands` and their
  constants from `internal/ha/gateway.go` (no production code sends them; F-17
  closed `wont-fix`, F-36 deferred). Home: `internal/ha/gateway_test.go`,
  because the property is the gateway's. `cmd/spike` keeps its own copies — it
  does not go through this gateway.
  **DoD:** `TestGateway_AllowList_EveryEntryHasACaller` parses the non-test Go
  files of `internal/` (`go/parser`, no shelling out) and fails if any
  allow-listed command or route constant is referenced nowhere outside
  `gateway.go`; it is shown red by temporarily removing one real caller. The
  three statistics commands are denied by `checkCommand` (existing
  not-allow-listed path, asserted). `policy.SourceStatistics` stays: it is
  policy, not gateway surface, and removing it is not this box. `make check`
  green.
  **Done 2026-10-03:** the reachability test also found six more uncalled
  entries (F-40); they sit in a shrink-only exemption set in the test.

- [x] **`P8-07` · Remove the unwired Core REST adapter (F-38)** — per D-08-2,
  `blocked:P8-03`
  Delete `internal/ha/rest.go` (`RESTClient`, `HistoryOptions`,
  `LogbookOptions`), the five `Route*` constants, `allowedRoutes`, `checkRoute`
  and `validateEntityID` with their tests; keep `ha.SupervisorClient`'s own
  GET-only check untouched. Update CLAUDE.md's module layout line
  (`internal/ha/` "websocket, rest, …") in the same change.
  **DoD:** `go build ./...` shows no Core REST client is linked in (`grep -rn
  RESTClient internal cmd` empty); `TestGateway_AllowList_EveryEntryHasACaller`
  still green; mutation-denied-before-transmission still asserted for the
  WebSocket path and the Supervisor adapter; `make check` green.
  **Done 2026-10-03:** `entityIDPattern` stays (the mapper uses it) and moved to
  `mapping.go`; the Supervisor client took over the size cap and deadline backstop
  (renamed `maxSupervisorResponseBytes`, `defaultSupervisorTimeout`) and the three
  tests that asserted them. The recorded-HA integration test lost its REST half.

- [x] **`P8-04` · `golangci-lint` clean**
  The linter has never run on this codebase — every phase 05 journal entry
  notes it skipped. Install it locally, run `golangci-lint run`, fix what it
  reports. A finding that would need a design change is filed, not fixed here.
  **DoD:** `golangci-lint run` exits 0 on the branch; the journal entry names
  the linter version and how many issues were fixed. CI is unchanged — it stays
  lint-free by design (`ci.yml`, CLAUDE.md "Commands").
  **Done 2026-10-03:** golangci-lint 2.14.0; 13 issues fixed (12 `errcheck`, 1
  `staticcheck` QF1002), none needing a design change.

- [ ] **`P8-08` · Security review of the HTTP transport (D-08-1)** — 🧠
  The fresh review phase 01's decision demanded before any listener. Design
  only; no code. Settle, as decision records in this file, each with the
  rejected alternative: the auth check (header form, constant-time compare,
  minimum secret length, what a missing/short secret does at start); the Origin
  policy (absent vs present-and-foreign; which origins, if any, are allowed);
  bind address and port number; request limits (body cap, header/read/idle
  timeouts, max concurrent requests) and how they meet the existing query
  budget; stateless vs sessions; how the transport is selected (option vs env)
  and where config is read; what the audit record carries for an HTTP caller;
  and what is never logged (the secret, the `Authorization` header). Verify,
  don't assume, what go-sdk `v1.7.0`'s Streamable HTTP handler already does
  (stateless mode, Origin/cross-origin checks, body limits) — record it in
  `docs/research/`. Update the architecture doc: T5 (§4), §15.2 (a published
  port, closed by default), and a new **ADR-013** in §24.
  **DoD:** every item above has a decision record or a research entry it links;
  doc §4/§15.2/§24 updated; the `P8-02` box below is amended where the review
  changes it; any rule-level conflict filed as a finding, not decided here.

- [ ] **`P8-02` · Streamable HTTP transport, selectable beside stdio** —
  `blocked:P8-08`
  Home: `internal/mcp` gains a transport choice behind `Run` (today
  `server.go:143` hard-wires `StdioTransport`) — a new `transport_http.go`, not
  a branch inside a tool; config parsing stays in `cmd/server`, the only place
  that knows every layer. Implements exactly P8-08's records.
  **DoD (tests first, network-free, `httptest`):** no `Authorization` ⇒ 401 and
  the MCP server never sees the request; wrong secret ⇒ 401; foreign `Origin` ⇒
  403; HTTP selected with the secret absent or too short ⇒ start refused with a
  non-zero exit and a message that does not echo any value; correct secret ⇒
  `initialize` and `tools/list` succeed and list the same tools as stdio; the
  stdio path still passes its existing tests; body over the cap ⇒ rejected; the
  secret appears in no log line, error string, audit record or response
  (assertion, like token-never-returned); `SUPERVISOR_TOKEN` likewise. `make
  check` green, `-race` included.

- [ ] **`P8-09` · Package and connect: the App on the Pi with a real client**
  — `blocked:P8-02`, `live-verify`
  `addon/config.yaml`: the secret as a `password` option, the transport option,
  `ports:` mapping P8-08's port to `null` (closed by default); `apparmor.txt`
  allows accepting on that socket and nothing more; `addon/config_test.go`
  asserts port-closed-by-default, `host_network: false`, no `ingress`.
  `docs/INSTALL.md`: set the secret, open the port, connect Claude Code (HTTP
  with header) and a stdio-only client via `mcp-proxy`, step by step.
  **DoD:** on the Pi, with an image the owner publishes: App stays **Started**;
  a real MCP client completes `initialize`, `tools/list` and one tool call over
  the LAN; the same request without the secret gets 401; with the port left
  closed the client cannot connect. Observations in
  `docs/research/<date>-http-transport-on-pi.md`.

- [ ] **`P8-10` · The binary reports the image's version (F-39)**
  `addon/config.yaml`'s `version:` stays the single source: `release.yml`
  already reads it; pass it as a Docker build arg into `-ldflags "-X
  main.version=…"`. No second literal anywhere.
  **DoD:** a test (beside `addon/config_test.go` or a `Dockerfile` assertion)
  fails if the Dockerfile stops setting `-X main.version`; a local `docker
  build --build-arg` run logs that version at start (observed, pasted in the
  journal). `make check` green.

- [ ] **`P8-11` · Drop the six uncalled allow-list entries (F-40, D-08-3)**
  Remove `CommandAuthCurrentUser`, `CommandEntityRegistryListForDisplay`,
  `CommandEntityRegistryGet`, `CommandCategoryRegistryList`, `CommandTraceGet`,
  `CommandTraceContexts` from `internal/ha/gateway.go` and delete
  `uncalledAllowListEntries`.
  **DoD:** `TestGateway_AllowList_EveryEntryHasACaller` passes with no exemption
  set; each dropped command is denied before transmission (existing
  unknown-command test pattern, one case per command); `make check` green.

- [ ] **`P8-05` · §21 acceptance walk on the Pi** — `blocked:P8-09`, `live-verify`
  For each of the twelve doc §21 criteria: the test(s) that assert it, by name,
  or a live observation on the Pi — at least: App running under protection mode
  on aarch64 with the §15.2 flags (`addon/config.yaml` as installed); Core
  restarted while a client is connected, then a tool call succeeding without
  restarting the App; one `analyze_*` investigation run against the real
  installation. A criterion with neither is a finding, not a pass.
  **DoD:** `docs/research/<date>-v1-acceptance.md` with one row per criterion
  (evidence, pass/finding); every gap filed in `FINDINGS.md`; no row reads
  "assumed".

- [ ] **`P8-06` · Cut v1.0** — `blocked:P8-05`
  `addon/config.yaml` `version: "1.0.0"`; `docs/INSTALL.md` current; README's
  status line says v1.0. The owner tags `v1.0.0` and pushes; `release.yml`
  publishes both architectures.
  **DoD:** `TestAddonManifestImageIsPinnedToVersion` green at 1.0.0; no
  `blocks-active` finding open; `make check` green. The tag itself is the
  owner's action and is not part of this box.

## Decisions

- [x] **D-08-3 — The six uncalled allow-list entries are dropped, not kept for
  later** — decided at the 2026-10-03 second `plan` (F-40)
  No production reader calls them and no open box plans one. **Why:** the same
  reasoning as D-08-2 — the allow-list is the reachable surface, and an entry
  without a caller overstates it; CLAUDE.md "no speculative generality". A
  future reader (`trace/get` for trace detail, `auth/current_user` for admin
  detection) re-adds its entry in the same change as its caller. **Rejected:**
  *keep `trace/get` and `auth/current_user` as "likely soon"* — that is the
  speculative generality the rule names; *decide per entry later* — leaves the
  exemption set alive with no owner. Owner may overturn at review.

- [x] **D-08-2 — The Core REST adapter is deleted, not kept as a fallback** —
  decided at the 2026-10-03 `plan` (F-38)
  `P1-03` built it per doc §23 step 4; every reader since went over the
  WebSocket, and `cmd/server` never constructs a `RESTClient`. **Why:** CLAUDE.md
  "no dead code, no speculative generality", and ADR-008's own reasoning —
  read-only-ness is enforced by what is linked in, so a linked-in HTTP client
  with its own allow-list is surface whether or not anything calls it. A future
  REST need re-adds an adapter with a caller in the same change.
  **Rejected:** *keep it as an HA-restart fallback* — the WebSocket manager
  already reconnects (`P1-02`), and a fallback with no caller is never
  exercised; *wire it somewhere so it has a caller* — inventing a consumer to
  justify code. Owner may overturn at review; the box is cheap to drop.

- [x] **D-08-1 — How an MCP client reaches the App-hosted server** — decided
  by the owner 2026-10-03: **option (c), HTTP inside this App**
  **Decision:** the App serves MCP over **Streamable HTTP** on a published port,
  **LAN only**. The port is **closed by default** (`ports:` maps it to `null`);
  the owner opens it on the App's Network tab. Clients authenticate with a
  **secret the owner sets in the App options** (a `password` field) — never an
  HA token, never passed on to Core (MCP spec: no token passthrough). Origin is
  validated; the transport is stateless; no `host_network`, no Ingress. **Both
  transports stay in the binary**, chosen by configuration: stdio for
  development and `cmd/measure`, HTTP in the App. **Fail closed:** HTTP selected
  with no secret set ⇒ the process refuses to start. Remote access (Nabu Casa,
  reverse proxy) is **out of v1**.
  **Why:** of the real options this is the only one where a leaked client
  credential reaches no more than the read-only tools under the privacy
  profile — (a) is host-root, (b) is an HA token with that user's write rights
  on the client machine, which hollows out ADR-008 through the credential
  rather than the code. (d) is void; (e) means leaving the App architecture.
  **Supersedes** phase 01's "stdio only" decision in part (its rejected
  "HTTP with a shared secret" is now chosen, with LAN-only and port-closed-by-
  default as the bound on T5). As that decision required, this is not a
  configuration change: **`P8-08` is the fresh security review**, and no HTTP
  code lands before it.
  **Rejected:** (a), (b), (d), (e) — reasons above and in the evidence below.
  *Original question:* the owner's to decide, on `P8-01`'s evidence. Options known before
  observation, none chosen: *(a)* stdio through `docker exec -i` (from the SSH
  App or a host shell) — keeps phase 01's no-listener decision, but its
  prerequisites on the SSH side are unverified; *(b)* run the binary off-box
  against Core's WebSocket with a long-lived token — no App needed at all, but
  then the App packaging is not what v1 ships, and token handling (rule 4)
  moves to the client's machine; *(c)* reopen phase 01's transport decision
  (HTTP behind Ingress) — explicitly "a new decision plus a fresh security
  review, not a configuration change".
  **Evidence from `P8-01`:** (a) needs `docker` in the SSH App, which is absent
  as shipped — it takes lowering that App's protection mode, a weaker posture
  than this App keeps; (b) untouched by the observations; (c) unchanged. New
  option *(d)*: `stdin: true` in `config.yaml` with Supervisor holding stdin —
  keeps stdio and the no-listener decision, but nothing observed says a client
  can then reach it; untried (needs a new image).
  **Evidence from F-41's verify (2026-10-03):** *(d) is void* — Supervisor's
  stdin endpoint is write-only, no response channel. *(a)* needs the community
  SSH App with `docker_api` and protection off (the official one has no Docker
  at all) — host-root on the client side. *(b)* puts an HA long-lived token,
  with that user's full write rights, on the client machine. *(c)* needs its
  own App-issued credential (no HA-token passthrough, MCP spec) and `Origin`
  validation; a leaked secret reaches only the read-only tools. New *(e)*: HA's
  own model — a Core integration on `/api/mcp` with HA auth — real only by
  reopening the architecture. Table and sources:
  `docs/research/2026-10-03-mcp-client-paths.md`.

## Phase Definition of Done

- Every doc §21 criterion is mapped to a named test or a live observation on the
  Pi, in a dated report.
- A real MCP client reaches the App on the Pi, and `docs/INSTALL.md` says how.
- `golangci-lint run` and `make check` are green; the allow-list has no entry
  without a caller, and no unwired adapter is linked in.
- `addon/config.yaml` is at 1.0.0, ready for the owner's tag. Still strictly
  read-only.
