# Journal — Phase 08 v1.0 Release

Append-only. One entry per closed task, **at most ~5 lines**. Never read whole —
`NEXT.md` carries the last few; this file answers "why on earth is it like that"
months later.

What belongs here is the **surprise**: the environment quirk, the API that
ignores its own documented parameters, the test that had to be shaped oddly. What
changed is already in the diff and the commit message; why it is designed that
way belongs in the phase file's decision record. Only the surprise is
unrecoverable anywhere else — so if there was none, the entry is one line and
that is correct.

### 2026-10-03 · P8-01
Observed the App off-box and on the Pi: exits 0 ~65 ms after start, stays stopped; stdio serves when stdin is held. F-37 confirmed; D-08-1 unblocked.
**Surprise:** I first read the log as a restart loop — wrong (7 min apart, watchdog off); the SSH App has no `docker` as shipped; the binary reports `0.0.0-dev` (F-39).
**Left open:** `stdin: true` untried on the Pi; Core reachability seen only as "connection manager ready".

### 2026-10-03 · P8-03
Dropped the three recorder statistics commands from the allow-list; added `TestGateway_AllowList_EveryEntryHasACaller` (go/parser over `internal/`, shown red) and a denial test.
**Surprise:** six other allow-listed commands are also uncalled (F-40); exempted by name in a shrink-only set rather than widening the box.
**Left open:** F-40; REST routes still pass the check only because `rest.go` references them (P8-07).

### 2026-10-03 · P8-07
Deleted `rest.go`/`rest_test.go`, the five `Core` routes, `allowedRoutes`, `checkRoute`, `validateEntityID`; CLAUDE.md layout line updated. `make check` green.
**Surprise:** `SupervisorClient` depended on `rest.go` for its size cap, timeout backstop and test helpers; ported those tests to it rather than losing the coverage.
**Left open:** `golangci-lint` still not installed (P8-04).


### 2026-10-03 · P8-04
golangci-lint 2.14.0, 13 issues fixed (12 errcheck — mostly deferred test `Close`/`CloseNow` — and one tagged-switch in `supervisorStatusError`). `make check` and `-race` green.
**Surprise:** the linter prints issues in capped batches: 11 on the first run, 2 more only after fixing those.

### 2026-10-03 · F-41 verify → D-08-1
Read HA Core `mcp_server`, Supervisor `apps.py`/`docker/app.py`/`ingress.py`, both SSH App manifests, `ha-mcp`. Owner chose (c): HTTP in the App, LAN, port closed by default, owner-set secret, fail closed, stdio kept. Re-planned into P8-08…P8-11; D-08-3 drops F-40's six entries.
**Surprise:** Supervisor's `/apps/{app}/stdin` writes and hangs up — no response channel, so `stdin: true` was never a client path; and the official SSH App has no Docker at any protection level.
**Left open:** whether the tools work for a non-admin HA user (irrelevant to (c); noted in the research file).

### 2026-10-03 · P8-08
Security review of D-08-1's HTTP transport: D-08-4…D-08-11, ADR-013, doc §4 T5/§15.2, SDK facts in `docs/research/2026-10-03-go-sdk-streamable-http.md`; `P8-02`/`P8-09` amended.
**Surprise:** go-sdk passes the *whole* HTTP header set to server code (`RequestExtra.Header`), `Authorization` included — so the middleware must strip it; and the SDK has no default Origin check, its deprecated option admits same-origin (= DNS-rebinding) requests.
**Left open:** "port closed" doesn't stop other Apps on the `hassio` network (P8-09 observes); privacy profile/log level unsettable in the App (F-42); TLS (F-43, defer).

### 2026-10-03 · P8-02
HTTP transport per D-08-4…D-08-11: `internal/mcp/transport_http.go` (gate + `Run` switch), `cmd/server/config.go`, audit `transport`, `policy.CompositeDeadline`. Smoked the binary: 401 without secret, tools/list with it, secret absent from logs, start refused without `http_secret`.
**Surprise:** none — the SDK behaved as `P8-08`'s research note said (stateless POST works with raw JSON-RPC, no initialize needed).
**Left open:** addon packaging (port, schema, `run.sh`, AppArmor) is `P8-09`; privacy/log-level options are `P8-12`.

### 2026-10-03 · P8-12
`loadSettings` in `cmd/server/config.go`: `privacy_profile` / `log_level` under the one-source rule, shared `readOptions` with the secret; `addon/config.yaml` declares both as closed lists; startup log reports the effective profile; CLAUDE.md "Configuration" corrected (budgets are constants).
**Surprise:** none.

### 2026-10-03 · P8-11
Dropped six uncalled allow-list entries from `gateway.go`, deleted `uncalledAllowListEntries`; `TestGateway_UncalledCommands_Denied` asserts each is refused before transmission. Closes F-40.
**Surprise:** none.

### 2026-10-03 · P8-10
`Dockerfile` takes `ARG VERSION` into `-X main.version`; `release.yml` passes config.yaml's version to both image builds. Observed: `docker build --build-arg VERSION=9.9.9-test` then run logs `"version":"9.9.9-test"`. Closes F-39.
**Surprise:** none.
**Left open:** `make release` still builds with the dev version (not the image path).

### 2026-10-03 · P8-09
App packaged for HTTP: `http_secret` password option, `8790/tcp: null`, `run.sh` sets the transport, INSTALL §3–4. Observed on the Pi with `0.9.2`: Started, 401 without secret, `initialize` and Claude Code work, port closed refuses, Terminal & SSH App reaches `:8790` (401). Closes F-37.
**Surprise:** `0.9.1` died on `cannot read the App options file` — AppArmor had no `/data/options.json r,`; no unit test can see that.
**Left open:** `mcp-proxy` flags for stdio-only clients.

### 2026-10-04 · P8-05
§21 walk on the Pi (0.9.2): 12 rows, 8 pass, 4 findings; owner restarted Core — 14 backoff warnings, `reconnected`, next call OK without restarting the App. Report: `docs/research/2026-10-04-v1-acceptance.md`.
**Surprise:** on the real build `get_system_health` and `get_automation_traces` answer empty with no unsupported marker, and audit shows `result_bytes:0` — all three green in tests. HA 2026.9.4 shows no Protection mode switch at all.
**Left open:** F-45 (blocks v1.0), F-46, F-47, F-48 need `verify`; row 7 not exercised live.

### 2026-10-04 · P8-13
Supervisor `{"result","data"}` envelope unwrapped once in `get`; six mappers require their key (D-08-13/14); invented-value fixture; flat-body test at the MCP layer. Observed on the Pi (0.9.3): `get_system_health` populated, `list_apps` lists 7 Apps. Closes F-45.
**Surprise:** none — every inner key D-08-14 assumed from Supervisor's docs was present on the first live call.
**Left open:** F-46 can now be read on a build that returns data; `P8-14` next.

### 2026-10-04 · P8-14
Five Supervisor routes and ten raw readers dropped; `EveryEntryHasACaller` now follows a route through its `SupervisorClient` method to a caller outside `internal/ha` (red on the five, green after). `Info`-driven tests moved to `CoreInfo`.
**Surprise:** none — the name-mention test passed only because the raw readers themselves mentioned the constants.
**Left open:** F-46…F-48 `verify`s, then `P8-06`.

### 2026-10-04 · verify F-48
Owner's SSH read of `ha apps info` for the installed App: `protected: true`, AppArmor profile, `host_network`/`full_access`/`docker_api` false, `privileged []`, `hassio_role default`, no ports. §21 rows 1–2 now observed; F-48 closed `done`.
**Surprise:** the `ha` CLI is `apps` but its JSON still keys `.data.addons`, and the installed slug carries a repository prefix (first two probes returned all-null, not an error).

### 2026-10-04 · plan F-46, F-47
D-08-16…D-08-18 recorded; `P8-15` (requests counted at `Manager.Call`/`SupervisorClient.get` via a context meter), `P8-16` (`result_bytes` measured from the result), `P8-17` (traces keyed by config id, `live-verify`) queued before `P8-06`.
**Surprise:** the middleware's "re-serializing would double the work" premise no longer holds — go-sdk v1.7.0 hands the middleware an already-marshalled `StructuredContent`, so measuring is free.
**Left open:** whether HA stores traces for id-less automations; D-08-18 answers `unsupported` for them until observed.

### 2026-10-04 · P8-15
`ha.RequestMeter` + `WithRequestMeter`; `Manager.Call` and `SupervisorClient.get` charge once per request after the allow-list; middleware attaches the budget; all 12 per-tool `ChargeHARequests` sites removed. Catalog table test holds audited `ha_requests` to the wire count for every tool.
**Surprise:** `find_stale_entities`' scan bound read `usage.HARequests` — its test double had to charge the meter itself, since a fake reader bypasses the seam. slog returns audit ints as `int64`.
**Left open:** `result_bytes` (P8-16); F-46 closes with it.

### 2026-10-04 · P8-16
`resultBytes` in the middleware sums `StructuredContent` (the SDK's already-marshalled RawMessage) and text blocks; `budget.Usage().Bytes` no longer feeds the audit. Closes F-46.
**Surprise:** SDK argument-validation failures come back as an `IsError` result, not a Go error, so the audit status is `success` with 26 bytes of error text — measuring zero for `IsError` was needed to meet the DoD.
**Left open:** audit status for `IsError` results still reads `success`.

### 2026-10-04 · P8-17
`CoreReader.AutomationTraces` reads the config id via `automation/config` and keys `trace/list` by it; no id → `ErrAutomationHasNoConfigID` (an `ErrUnsupported`) with its own reason and the logbook fallback. Closes F-47.
**Surprise:** the reason text was hard-wired to "permission denied" for every `ErrUnsupported`, so a distinct sentinel and a branch in `classifyAutomationError` were needed.
**Left open:** deploy-only bump to 0.9.4; `P8-06` sets 1.0.0.
