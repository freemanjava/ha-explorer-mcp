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

- [x] **`P8-08` · Security review of the HTTP transport (D-08-1)** — 🧠
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

- [x] **`P8-02` · Streamable HTTP transport, selectable beside stdio**
  Home: `internal/mcp` gains a transport choice behind `Run` (today
  `server.go:143` hard-wires `StdioTransport`) — a new `transport_http.go`, not
  a branch inside a tool; config parsing stays in `cmd/server`, the only place
  that knows every layer — in a new `cmd/server/config.go` (environment and
  options-file reading, validation), so `main.go` stays wiring only. Implements exactly P8-08's records (D-08-4…D-08-11).
  **DoD (tests first, network-free, `httptest`):** no `Authorization` ⇒ 401 and
  the MCP server never sees the request; wrong secret ⇒ 401; a malformed or
  duplicated `Authorization` ⇒ 401; **any** `Origin` header ⇒ 403, even one
  equal to `Host` and even with the right secret (D-08-5); HTTP selected with
  the secret absent, too short, too long or containing a space ⇒ start refused
  with a non-zero exit and a message that names `http_secret` and does not echo
  any value; `HA_INSPECTOR_TRANSPORT` unknown ⇒ start refused; secret read from
  the options file when it exists and the environment variable then ignored, a
  malformed options file ⇒ start refused (D-08-9); correct secret ⇒
  `initialize` and `tools/list` succeed and list the same tools as stdio; a tool
  handler sees **no `Authorization`** in `RequestExtra.Header` (D-08-4); GET ⇒
  405, a path other than `/mcp` ⇒ 404; body over 128 KiB ⇒ 413, and the
  largest legal tool input (200 ids at 255 characters) fits (D-08-7); a fifth
  concurrent authenticated request ⇒ 503 with `Retry-After`; two HTTP requests
  draw on **one** invocation rate limiter (D-08-7); `WriteTimeout` derived from
  the composite deadline, not a literal; audit records carry `transport`
  (D-08-10); the stdio path still passes its existing tests; the secret appears
  in no log line, error string, audit record or response, and no log line
  contains `Bearer ` (assertion, like token-never-returned); `SUPERVISOR_TOKEN`
  likewise. `make check` green, `-race` included.

- [x] **`P8-12` · App options carry the privacy profile and log level (F-42,
  D-08-12)** — `blocked:P8-02`
  Home: `cmd/server/config.go` (from `P8-02`) gains two keys under D-08-9's
  one-source rule — options file when it exists, environment otherwise, never
  merged; `addon/config.yaml` declares them. CLAUDE.md "Configuration" is
  amended in the same change to say budget limits are measured constants, not
  configurable, in v1 (D-08-12).
  **DoD (tests first):** options file with `privacy_profile: deny` and
  `log_level: debug` ⇒ that profile and level, the environment ignored; options
  file without the keys ⇒ `mask` / `info`; an unknown value in either ⇒ start
  refused, message names the key (as `HA_INSPECTOR_PRIVACY_PROFILE` does today);
  no options file ⇒ today's environment behavior unchanged;
  `addon/config_test.go` asserts both keys in `schema` as closed lists
  (`list(mask|allow|deny)`, `list(debug|info|warn|error)`) with defaults `mask`
  and `info` in `options`; the startup log's `privacy_profile` is the effective
  value, not the raw environment variable. `make check` green.

- [x] **`P8-09` · Package and connect: the App on the Pi with a real client**
  — `blocked:P8-02`, `blocked:P8-12`, `live-verify`
  `addon/config.yaml`: `http_secret` as a `password` option (D-08-9), `ports:`
  mapping `8790/tcp` to `null` (closed by default, D-08-6); `run.sh` sets
  `HA_INSPECTOR_TRANSPORT=http` (D-08-9 — no transport option); `apparmor.txt`
  allows accepting on that socket and nothing more; `addon/config_test.go`
  asserts port-closed-by-default, `host_network: false`, no `ingress`, and the
  secret declared as `password`. INSTALL.md also states that the secret
  crosses the LAN in clear (D-08-11) and sits in HA backups, and how to rotate
  it (D-08-9).
  `docs/INSTALL.md`: set the secret, open the port, connect Claude Code (HTTP
  with header) and a stdio-only client via `mcp-proxy`, step by step.
  **DoD:** on the Pi, with an image the owner publishes: App stays **Started**;
  a real MCP client completes `initialize`, `tools/list` and one tool call over
  the LAN; the same request without the secret gets 401; with the port left
  closed the client cannot connect from the LAN — and whether another App on
  the `hassio` network still reaches `:8790` is observed and recorded either
  way (D-08-6 expects it can). Observations in
  `docs/research/<date>-http-transport-on-pi.md`.
  **Done 2026-10-03:** four addon tests written red first; `0.9.1` failed live (AppArmor denied `/data/options.json`),
  `0.9.2` passed every DoD observation incl. `hassio`-network reachability (401). Evidence:
  `docs/research/2026-10-03-http-transport-on-pi.md`. **Left open:** `mcp-proxy` flags in INSTALL.md §4 unverified.

- [x] **`P8-10` · The binary reports the image's version (F-39)**
  `addon/config.yaml`'s `version:` stays the single source: `release.yml`
  already reads it; pass it as a Docker build arg into `-ldflags "-X
  main.version=…"`. No second literal anywhere.
  **DoD:** a test (beside `addon/config_test.go` or a `Dockerfile` assertion)
  fails if the Dockerfile stops setting `-X main.version`; a local `docker
  build --build-arg` run logs that version at start (observed, pasted in the
  journal). `make check` green.
  **Done 2026-10-03:** two `addon/config_test.go` tests written red first (Dockerfile `ARG VERSION` + `-X`; `release.yml` passes it for both images).

- [x] **`P8-11` · Drop the six uncalled allow-list entries (F-40, D-08-3)**
  Remove `CommandAuthCurrentUser`, `CommandEntityRegistryListForDisplay`,
  `CommandEntityRegistryGet`, `CommandCategoryRegistryList`, `CommandTraceGet`,
  `CommandTraceContexts` from `internal/ha/gateway.go` and delete
  `uncalledAllowListEntries`.
  **DoD:** `TestGateway_AllowList_EveryEntryHasACaller` passes with no exemption
  set; each dropped command is denied before transmission (existing
  unknown-command test pattern, one case per command); `make check` green.
  **Done 2026-10-03:** `TestGateway_UncalledCommands_Denied` (six cases) written red first.

- [x] **`P8-05` · §21 acceptance walk on the Pi** — `live-verify`
  For each of the twelve doc §21 criteria: the test(s) that assert it, by name,
  or a live observation on the Pi — at least: App running under protection mode
  on aarch64 with the §15.2 flags (`addon/config.yaml` as installed); Core
  restarted while a client is connected, then a tool call succeeding without
  restarting the App; one `analyze_*` investigation run against the real
  installation. A criterion with neither is a finding, not a pass.
  **DoD:** `docs/research/<date>-v1-acceptance.md` with one row per criterion
  (evidence, pass/finding); every gap filed in `FINDINGS.md`; no row reads
  "assumed".
  **Done 2026-10-04:** `docs/research/2026-10-04-v1-acceptance.md` — 8 pass, 4 findings (F-45, F-46, F-47, F-48); Core restart observed live. F-48 `verify`d the same day: rows 1–2 now observed on the installed App (`protected: true`) — 10 pass, F-46/F-47 open.

- [x] **`P8-13` · Supervisor responses are unwrapped, and a blank body fails loudly (F-45)** — `live-verify`
  Per D-08-13 and D-08-14. Home: `internal/ha/supervisor.go` `get` (the one place every Supervisor body passes,
  so the envelope is handled once) and `internal/ha/mapping.go` (each Supervisor mapper names its required keys).
  Fixture `test/fixtures/supervisor_info.json`: the `{"result","data"}` envelope with exactly the `data` keys
  observed on the Pi (`docs/research/2026-10-04-supervisor-response-shape.md`), **invented values only** — no
  hostname, machine id or version string from the real installation enters the repo. The existing hand-written
  flat test bodies are rewritten into the envelope; none may stay flat.
  **DoD:** written red first — (1) the fixture through `CoreInfo` yields non-empty `CoreVersion`, `Hostname`,
  `Arch`; (2) a flat (un-enveloped) body, `{"result":"error"}` on 200, and an envelope with no `data` each return
  an error per D-08-13, with no Supervisor `message` text in it; (3) per mapper, a `data` object missing its
  required key (D-08-14) returns `ErrUnexpectedMessage`, and a present-but-empty `addons: []` maps to zero Apps
  with no error; (4) at the MCP layer, a Supervisor answering a flat body makes `get_system_health` and
  `list_apps` report `Unsupported` with a reason — never empty-and-unmarked. `make check` green.
  **Live:** on the Pi, after the owner installs the build: `get_system_health` returns a Core version and disk
  figures, `list_apps` lists the installed Apps (counts only in the journal, no names). If the owner can, they
  also paste `jq 'keys, (.data | keys)'` of `ha supervisor info`, `ha os info`, `ha host info` and
  `ha resolution info` into the research doc, settling the inner keys D-08-14 relies on.
  **Done 2026-10-04:** envelope unwrapped in `get`, `requireKeys` in six mappers, fixture `supervisor_info.json`; observed on the Pi (0.9.3): `get_system_health` fully populated, `list_apps` lists 7 Apps. Inner-key paste not needed — every mapper's required key was present.

- [x] **`P8-14` · Drop the uncalled Supervisor routes and raw readers (F-49)**
  Per D-08-15. Remove `SupervisorRouteNetworkInfo`, `…HardwareInfo`, `…JobsInfo`, `…AddonSelfInfo` and
  `SupervisorRoutePing` from `allowedSupervisorRoutes`, and the raw-`json.RawMessage` methods with no production
  caller (`Info`, `OSInfo`, `HostInfo`, `ResolutionInfo`, `NetworkInfo`, `HardwareInfo`, `JobsInfo`,
  `AddonSelfInfo`, `AddonSelfStats`, `Ping`). Tests that drove `get` through `Info` switch to a typed reader.
  Home: `internal/ha/gateway.go`, `internal/ha/supervisor.go`; the property stays asserted in
  `gateway_test.go`.
  **DoD:** each dropped route is denied by `checkSupervisorRoute` before a request is built (one case per
  route); `TestGateway_AllowList_EveryEntryHasACaller` is tightened so a Supervisor route counts as called only
  through a method that a non-test file outside `internal/ha` calls — shown red against today's tree before the
  removal; `make check` green.
  **Done 2026-10-04:** red on exactly the five routes, then green after removal; the tests that drove `get` through
  `Info` now use `CoreInfo`.

- [x] **`P8-15` · Every upstream request is counted where it leaves, not where a tool remembers to (F-46)**
  Per D-08-16. `internal/ha` gains a narrow `RequestMeter` interface (`ChargeHARequests(n int) error`) and
  `WithRequestMeter(ctx, m)` / its lookup; `Manager.Call` and `SupervisorClient.get` charge it once per request
  that has passed the allow-list, before any bytes are written. No meter in the context is a no-op (probes,
  unit tests). The invocation middleware attaches the invocation's `*policy.QueryBudget` as the meter. Every
  `ChargeHARequests` call in `internal/mcp` is removed — the seam now counts them, and keeping both would
  double-count. Home: a new `internal/ha/meter.go` (the interface and context helpers), the two call sites, and
  `internal/mcp/middleware.go`; `internal/ha` does not import `internal/policy`.
  **DoD:** a denied command charges nothing and sends nothing; a meter that refuses (budget exhausted) makes
  `Call`/`get` return an error that `errors.Is(…, ErrBudgetExceeded)` and the fake HA receives zero frames /
  requests for it; a request that fails or times out upstream is still counted; `list_integrations` through
  the middleware on a cold registry cache audits `HARequests` equal to the commands the fake HA received, and
  on a warm cache audits 0 (a refill under a detached context must still reach the meter — test it); a
  table test over the catalog drives every tool against its fixtures and asserts its counted requests fit its
  class's `MaxHARequests`; a source-scan test asserts no non-test file in `internal/mcp` calls
  `ChargeHARequests`; `make check` green.
  **Done 2026-10-04:** `internal/ha/meter.go`; the catalog table asserts audited `ha_requests` equals what the fake
  HA and fake Supervisor received, for all 21 tools.

- [x] **`P8-16` · `result_bytes` is the size of the result, measured (F-46)**
  Per D-08-17. The middleware sets `audit.Record.ResultBytes` from the `*CallToolResult` the handler returned:
  `len(StructuredContent)` plus the length of every `TextContent` text — the bytes the SDK already marshalled,
  so nothing is re-serialized. `budget.Usage().Bytes` stops feeding the audit; `ChargeBytes` stays exactly as it
  is, as the budget's backstop. The comment at `middleware.go` that justified the old source is rewritten.
  Home: `internal/mcp/middleware.go`.
  **DoD:** `list_integrations` and `get_system_overview` through the middleware audit `ResultBytes` equal to
  the summed lengths of the returned result's structured and text content, and > 0; a tool that charges bytes
  (`get_entity_history`) audits the measured size, not its charge; an error result audits 0; `make check` green.
  **Done 2026-10-04:** `resultBytes` in `middleware.go`; an `IsError` result measures 0.

- [ ] **`P8-17` · `get_automation_traces` asks HA's trace store by the automation's config id (F-47)** —
  `live-verify`
  Per D-08-18. `CoreReader.AutomationTraces` first reads `automation/config` for the entity (the read
  `AutomationDetail` already makes) and sends `trace/list{domain:"automation", item_id:<config id>}`. A config
  with no `id` → `ErrUnsupported` with a fixed reason saying HA keys traces by config id and this automation
  has none, so the tool answers `unsupported` and attaches its logbook fallback rather than an empty list.
  `splitEntityID` stops being used for traces; the doc comment on `traceListCommand` that says "object id" is
  corrected. `analyze_automation_health` reads traces through the same method and inherits the fix. Home:
  `internal/ha/corereader.go`, `internal/ha/automation_commands.go`.
  **DoD:** the fake HA's `trace/list` handler answers `[]` for any `item_id` but the config id and the
  fixture's traces for the config id — the existing test, rerun against it, is red today and green after;
  a config without `id` yields `Unsupported:true` with that reason and no `trace/list` frame sent; a
  non-admin principal still reaches the logbook fallback unchanged; `make check` green; **live:** on the Pi,
  `get_automation_traces` for one of the three automations sampled in F-47's `verify` returns `Items` with
  at least one run whose start is after its `LastTriggered` minus a minute — or, if still empty, that is a
  new finding, not a pass.

- [ ] **`P8-06` · Cut v1.0**
  `addon/config.yaml` `version: "1.0.0"`; `docs/INSTALL.md` current; README's
  status line says v1.0. The owner tags `v1.0.0` and pushes; `release.yml`
  publishes both architectures.
  **DoD:** `TestAddonManifestImageIsPinnedToVersion` green at 1.0.0; no
  `blocks-active` finding open; `make check` green. The tag itself is the
  owner's action and is not part of this box.

## Decisions

D-08-4…D-08-11 are `P8-08`'s security review of D-08-1's HTTP transport, decided 2026-10-03 on Opus. What the
SDK already does is recorded in `docs/research/2026-10-03-go-sdk-streamable-http.md`; the architecture doc carries
the summary as **ADR-013** (§24), T5 (§4) and §15.2. The owner may overturn any of them at review; none is
implemented yet (`P8-02`).

D-08-16…D-08-18 were decided at the 2026-10-04 `plan` after F-46's and F-47's `verify`s, on the stronger model.

- [x] **D-08-16 — Upstream requests are counted at the two wire seams, through a meter carried in the context** —
  `P8-15` (F-46)
  `Manager.Call` (WebSocket) and `SupervisorClient.get` (REST) are the only places a request leaves the
  process; each charges a `RequestMeter` found in the context, after the allow-list and before the write. The
  meter is a one-method interface defined in `internal/ha`; `*policy.QueryBudget` satisfies it structurally, so
  `internal/ha` does not import `internal/policy`. A cache hit costs nothing and is counted as nothing; a refill
  is counted against the invocation that triggered it. Per-tool `ChargeHARequests` goes. Consequence, wanted:
  `MaxHARequests` now bounds every tool, not only the six that opted in. **Why:** the `verify` showed 8 of 14
  tool files never charge — opt-in counting drifts by construction, and the seam is the one place that cannot
  forget. **Rejected:** *charge in every tool* — the status quo, which is how F-46 happened; *`internal/ha`
  imports `policy`* — ties the adapter to the budget's concrete type for one method; *a decorator in
  `cmd/server`* — the wiring package gains behaviour; *count in `gateway.go`* — it decides only what is
  permitted (CLAUDE.md, single responsibility).

- [x] **D-08-17 — `result_bytes` is measured from the returned result, not taken from budget charges** —
  `P8-16` (F-46)
  The SDK has already marshalled the typed output into `StructuredContent` (and its text copy) by the time the
  middleware sees the result (go-sdk v1.7.0, `mcp/server.go` typed-handler wrapper), so its length is free.
  The audit field means "what this invocation returned"; the budget's byte charges stay as the enforcement
  backstop and stop doubling as a report. **Why:** the old source was correct only where a tool charged, and
  its charges mix upstream size and output size. **Rejected:** *re-marshal in the middleware* — the cost the
  original comment rightly refused, and unnecessary; *make every tool charge its output* — opt-in again.

- [x] **D-08-18 — Traces are read by the automation's config id; no id is `unsupported`, never `[]`** —
  `P8-17` (F-47)
  The id comes from `automation/config` (observed live on the Pi: `get_automation` shows a config `ID` unlike
  the object id; fixtures' trace `item_id` is a config id). Same admin gate as `trace/list`, so no new failure
  mode; one extra small request. **Why:** rule 7 — an empty, unmarked list for a key HA never used reads as
  "never ran". **Rejected:** *entity-registry `unique_id`* — free from the cache, but "unique_id equals config
  id" is not established by any fixture or observation here; *try the config id, fall back to the object id*
  — whether HA stores traces for id-less automations at all is not established either, and a fallback that
  may also return a meaningless `[]` reintroduces the defect. If the live check shows id-less automations do
  carry traces, that is a finding that reopens this record.

D-08-13…D-08-15 were decided at the 2026-10-04 `plan` after F-45's `verify`, on the stronger model.

- [x] **D-08-13 — The Supervisor envelope is unwrapped once, in `get`, and only `result:"ok"` passes** —
  `P8-13` (F-45)
  `get` decodes `{"result": string, "data": raw}` and returns `data`. `result` other than `"ok"` on a 200 →
  `ErrUnsupported` with a fixed string naming the route; no `data` key, or `data` not an object →
  `ErrUnexpectedMessage`. Supervisor's `message` field is never copied into the error: it is upstream text
  (rule 6) and may quote anything. **Why:** every Supervisor route carries the same envelope (observed on
  `/info`; Supervisor's documented API contract for the rest), so it is a transport property, and `get` is
  already the single choke point for Supervisor bytes. **Rejected:** *each mapper unwraps* — six copies of one
  rule, and the seventh mapper forgets; *accept both flat and enveloped bodies* — the flat shape was never
  observed, and tolerating it is what hid this defect.

- [x] **D-08-14 — Each Supervisor mapper requires its identifying key; absence is an error, emptiness is not** —
  `P8-13` (F-45)
  Before decoding, a mapper checks that `data` holds its required key(s): `/info` → `homeassistant`;
  `/supervisor/info` → `version`, `addons`; `/os/info` → `version`; `/host/info` → `disk_total`;
  `/resolution/info` → `issues`; `/addons/self/stats` → `memory_percent`. Missing → `ErrUnexpectedMessage`.
  A present empty value (`"addons": []`) is a real answer. **Why:** `encoding/json` turns any unexpected shape
  into zero values with a nil error, and on these routes a zero value reads as a fact ("no Apps", "0 issues") —
  the rule-7 breach F-45 observed. The envelope fix alone would leave the same hole for the next shape change.
  The inner keys are the existing wire structs' (Supervisor's docs); only `/info`'s were observed — any that is
  wrong now surfaces as `Unsupported`, which `P8-13`'s live check catches. **Rejected:** *require every field* —
  a new Supervisor dropping a minor field would blank a whole tool; *only check non-zero results* — "0 issues"
  and "no Apps" are legitimate answers.

- [x] **D-08-15 — Supervisor routes and readers with no caller are dropped, as D-08-3 dropped commands** —
  `P8-14` (F-49)
  D-08-3's reasoning, applied to the Supervisor half of the allow-list it did not cover: five routes are
  reached only through raw readers nothing outside tests calls, and the reachability test counted the constant's
  mention, not a caller. **Rejected:** *keep the raw readers for `cmd/spike`* — the spike has its own client;
  *keep `/supervisor/ping` for a health probe* — no box plans one, and re-adding it with its caller is one line.
  Owner may overturn at review.

- [x] **D-08-12 — In v1 the App configures the privacy profile and log level; budget limits stay constants** —
  decided by the owner 2026-10-03 (`plan`, F-42)
  **Decision:** `/data/options.json` carries `privacy_profile` (`mask`|`allow`|`deny`, default `mask`) and
  `log_level` (`debug`|`info`|`warn`|`error`, default `info`), read under D-08-9's one-source rule. **Budget limits
  are not configurable in v1** — they stay the measured constants in `internal/policy/budget.go` and
  `ratelimit.go`, and CLAUDE.md's "Budget limits and the privacy profile are configurable" is corrected to match the
  code (CLAUDE.md: fix one or the other). **Why:** the limits are what protects the Pi's recorder, set from
  measurements (doc §10/§26); an option would let an installation lift them above anything measured, and no v1 user
  has asked. The profile and level are the two settings an owner actually needs on the Pi, and today neither can be
  set there. **Rejected:** *budget limits as App options in v1* — more surface, and a way to switch off the Pi's
  protection; revisit on v1 usage data (F-44); *fold F-42 into `P8-02`* — `P8-02` is already the largest box in the
  phase and security-critical; a separate reviewable change keeps its diff about the transport.

- [x] **D-08-4 — Client authentication: one owner-set bearer secret, compared in constant time** — `P8-08`
  **Header:** `Authorization: Bearer <secret>`, scheme case-insensitive, exactly one `Authorization` header;
  anything else is **401** with `WWW-Authenticate: Bearer` and a fixed body that does not say which check failed.
  **Compare:** SHA-256 of the presented value against SHA-256 of the configured one with
  `crypto/subtle.ConstantTimeCompare` — equal-length digests, so neither content nor length leaks through timing.
  **Secret rules:** 32–256 characters, printable ASCII without spaces (header-safe; 32 is the floor of `openssl rand
  -hex 16`, and INSTALL.md will recommend `-hex 32`). HTTP selected with the secret absent or breaking a rule ⇒
  **the process refuses to start**, non-zero exit, a message naming the option (`http_secret`) and the rule — never
  the value, never its length. **After a match** the middleware **deletes `Authorization` from the request** before
  the SDK handler sees it: the SDK passes every header to server code as `RequestExtra.Header` (research note), and
  a secret that never enters `internal/mcp` cannot be logged from there. The secret is also registered with the
  redactor (`Options.Secrets`), as `SUPERVISOR_TOKEN` is. No lockout: a lockout is a denial of service any LAN host
  can trigger, and a 128-bit secret is not brute-forced over HTTP.
  **Rejected:** *an HA token, or passing the client's credential to Core* — the MCP spec forbids token passthrough,
  and D-08-1 chose an App-issued secret precisely so a leak reaches no write right; *the SDK's
  `auth.RequireBearerToken`* — OAuth-shaped (scopes, mandatory expiry, verifier error text in the body) and it leaves
  the header in `Extra.Header`; *HTTP Basic* — the same secret with a username nobody needs; *a query-string secret* —
  lands in client history and proxy logs; *OAuth* — no authorization server in v1; *plain `==`* — timing-dependent.

- [x] **D-08-5 — Origin: any `Origin` header is refused; absent is allowed** — `P8-08`
  A request carrying `Origin` — any value, including one equal to the `Host` — is **403** before the secret is
  checked. A request without `Origin` proceeds to D-08-4. No origin allow-list exists. **Why:** every client v1
  supports (Claude Code, `mcp-proxy`, SDK clients) is a non-browser process that sends no `Origin`; a browser always
  sends one on a cross-site POST. Refusing them all meets the MCP spec's "validate Origin" with the smallest rule, and
  needs no notion of "our" origin, which the App does not have (the owner picks the host port and reaches it by
  whatever LAN name). The SDK's localhost protection stays enabled but is inert on the `hassio` interface (research
  note); it is not counted as a control. **Rejected:** *`http.CrossOriginProtection` or the SDK's deprecated
  option* — admits same-origin browser requests, which is what a DNS-rebinding page is; *same-origin by comparing
  `Origin` with `Host`* — the same hole; *a configurable allow-list* — no browser client in v1, and a knob nobody
  needs is one more thing to misconfigure. A browser-based client (MCP Inspector) is a new decision, not an option.

- [x] **D-08-6 — Bind `:8790`, path `/mcp`, port closed by default; the secret is the gate, not the port** —
  `P8-08`
  The listener binds `:8790` (all container interfaces — under `host_network: false` the container has only its
  `hassio` bridge interface) and serves exactly `POST /mcp`; any other path is **404**, other methods **405** (the
  SDK's stateless rule). The container port is a named constant with no override: the owner chooses the *host* port
  on the App's Network tab, and `addon/config.yaml` maps `8790/tcp: null` so nothing is published until they do.
  **What "closed" does not mean:** Apps share Supervisor's `hassio` Docker network and reach each other by container
  name without any `ports:` mapping (how every App reaches `core-mosquitto`), so with the host port closed the
  listener is still reachable from **other Apps and from Core**. Third-party App code is therefore a client
  population from the first start, and D-08-4 holds for it exactly as for the LAN — which is why a missing secret
  refuses start instead of "it's closed anyway". `P8-09` observes this rather than assuming it.
  **Rejected:** *bind loopback only* — unreachable through Docker's port mapping, so the transport could not work;
  *an option for the container port* — the Network tab already remaps the host side; *8099* — the conventional
  Ingress port, and this App has no Ingress (D-08-1); *serve on `/`* — a fixed path is one more thing a scanner must
  guess and keeps room for nothing else to ever be added there by accident.

- [x] **D-08-7 — Request limits sit outside the query budget, not instead of it** — `P8-08`
  **Body:** `MaxRequestBodyBytes` = **128 KiB** (SDK default 4 MiB). The largest legal tool input is ~52 KiB — 200
  entity ids (`measuredMaxEntities`, `internal/policy/budget.go`) at HA's 255-character id ceiling plus JSON
  framing — so 128 KiB is ~2.5× headroom; `P8-02` asserts that input fits. **`http.Server`:** `MaxHeaderBytes` 8
  KiB; `ReadHeaderTimeout` 5 s; `ReadTimeout` 10 s (a ≤128 KiB body on a LAN); `WriteTimeout` = the composite
  deadline (30 s, `policy.compositeDeadline`) **+ 15 s**, derived from that constant, not a second literal, so a
  composite tool's own deadline always fires first and returns `partial` rather than a cut connection; `IdleTimeout`
  60 s. **Concurrency:** at most **4** authenticated requests in flight; the fifth is **503** with `Retry-After: 1`,
  checked *after* D-08-5/D-08-4 so unauthenticated traffic cannot occupy the slots. **Budget:** unchanged and per
  invocation; the invocation rate limiter (`policy.NewInvocationLimiter`, 2/s, burst 10) stays **process-wide**
  because `getServer` returns the **one** `*sdkmcp.Server` built at start — never one per request, which would give
  every POST a fresh limiter. **Why 4:** composite tools fan out to several HA reads each, and the measured binding
  constraint is the recorder on the Pi (`2026-10-03-composite-budget-measurement.md`); four concurrent composites
  already exceed what the rate limiter lets arrive in two seconds. **Rejected:** *rely on the SDK default body cap* —
  32× more than any legal input; *no concurrency cap, the rate limiter is enough* — it bounds arrivals of tool calls
  only, not `initialize`/`tools/list` or slow requests holding goroutines; *cap before auth* — lets an
  unauthenticated flood lock out the real client; *a connection-count limiter* — needs `x/net/netutil` or our own
  listener wrapper, and the in-flight cap plus timeouts already bound the work.

- [x] **D-08-8 — Stateless, JSON responses** — `P8-08`
  `StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: …, Logger: <redacting logger>}`.
  No session table, no `Mcp-Session-Id`, no GET stream; each POST is one request and one `application/json`
  response. **Why:** sessions are per-client server state with no idle timeout by default (`SessionTimeout` zero ⇒
  never closed) and an id that, if leaked, addresses someone else's session; nothing this server does needs
  server→client requests (research note), and HA's own MCP server runs stateless for the same reasons
  (`2026-10-03-mcp-client-paths.md`). JSON over SSE because no tool streams progress, and a plain response is what
  `curl` and `mcp-proxy --stateless` handle without surprises. **Rejected:** *stateful with a `SessionTimeout`* —
  state to bound and an id to protect for no feature we use; *`MCPGODEBUG=allowsessionsinstateless`* — a
  compatibility path the SDK deletes in v1.9.0.

- [x] **D-08-9 — Transport by environment, fixed by the image; the secret from the App options file** — `P8-08`
  **Selection:** `HA_INSPECTOR_TRANSPORT` = `stdio` (default) | `http`; any other value refuses start, like
  `HA_INSPECTOR_PRIVACY_PROFILE` today. `addon/rootfs/run.sh` sets `http` — the App always serves HTTP, and the owner
  cannot switch it to stdio (which would only reproduce F-37's immediate exit). Development and `cmd/measure` keep
  the stdio default. **Secret:** read once at start from the App options file **`/data/options.json`**, key
  `http_secret` (declared `password` in `addon/config.yaml`'s schema, so the UI masks it); when that file does not
  exist (development), from `HA_INSPECTOR_HTTP_SECRET`. One source per run, never merged: if the options file exists,
  the environment variable is ignored. A malformed options file refuses start. Both are read in `cmd/server`, the
  only place that knows every layer; `internal/mcp` receives the validated secret, never a path or a variable name.
  `/data/options.json` is a fixed constant, not configurable — not `/config` (ADR-004). **Known exposure, accepted:**
  Supervisor stores App options in plain text in `/data/options.json` and in HA backups, so a backup reveals the
  secret; INSTALL.md (`P8-09`) says so and says how to rotate (change the option, restart the App). **Rejected:** *a
  `transport` App option* — a choice whose only other value breaks the App; *auto-detect from `SUPERVISOR_TOKEN`* —
  implicit, and a developer with the variable set would get a listener they did not ask for; *the secret in an
  environment variable set by Supervisor* — App options reach the process only through the options file; *`bashio`
  in `run.sh` to export options* — adds a shell dependency to an image that has none, and puts the secret in the
  process environment, visible in `/proc/<pid>/environ`.

- [x] **D-08-10 — What is logged and audited for an HTTP caller; what never is** — `P8-08`
  **Audit:** the existing per-invocation record (doc §17), plus a `transport` field (`stdio`|`http`) set once from
  options. No caller address: the SDK gives server code no remote address (research note), and with one shared
  secret every authenticated caller is the same principal. **Rejected requests** never reach a tool, so they make no
  audit record: each is logged at **DEBUG** (remote IP, reason class `origin`|`missing`|`malformed`|`mismatch`) and
  counted (`http_auth_rejections`); a **WARN** is emitted at most **once a minute** with the count since the last and
  the last remote IP — a probing LAN host is visible without flooding the log (CLAUDE.md: every recovery or refusal
  path has a counter). Listener start/stop is **INFO** with the port, never the secret. **Never logged, at any
  level:** the secret; the `Authorization` header in any form; request headers as a set; request or response bodies
  at INFO or above (existing rule). **Rejected:** *remote IP in every audit record* — needs smuggling it through a
  header or context the SDK does not define, for no principal it could distinguish; *a WARN per rejection* — one
  scanner fills the Supervisor log; *no log of rejections* — a leaked secret being tried from a new host would be
  invisible.

- [x] **D-08-11 — No TLS in v1; the secret crosses the LAN in clear** — `P8-08`
  The listener is plain HTTP. Anyone who can sniff the LAN segment between client and Pi can read the secret.
  **Accepted because:** D-08-1 bounds the transport to the LAN with the port closed by default, and the blast radius
  of a stolen secret is the read-only tools under the privacy profile — no write path exists to reach (rule 1,
  ADR-008); rotation is one option change. INSTALL.md (`P8-09`) states the exposure plainly. **Revisit when:** remote
  access (out of v1 per D-08-1) is reopened — a reverse proxy or Nabu Casa path would terminate TLS there, and that
  is where it belongs (F-43). **Rejected:** *a self-signed certificate generated into `/data`* — every client must be
  taught to trust it (Claude Code via `NODE_EXTRA_CA_CERTS`, `mcp-proxy` via its own flags), and an unpinned
  self-signed cert stops a passive sniffer but not an active LAN attacker; *mapping `ssl:ro` for HA's certificates* —
  widens `map: []`, and those certificates are usually issued for the public name, not the LAN address.

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
