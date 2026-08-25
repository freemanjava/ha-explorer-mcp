# Journal — Phase 01 HA Access & Read-Only Gateway

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

### 2026-08-24 · P1-01
`internal/ha/manager.go`: long-lived multiplexing WS connection — id-correlated concurrent requests, per-call deadline, bounded backoff+jitter reconnect with a `Reconnects()` counter, typed `Command`/`CommandError` on the one send path P1-02 will guard.
**Surprise:** the reconnect test's flake was a real defect — `Call` could be handed a session that died a microsecond earlier and report `ErrUpstreamUnavailable` while the manager was already reconnecting. Fixed by separating "transmitted" (final) from "not transmitted" (retry on the next connection).
**Left open:** nothing observed against a live HA (no token reaches the agent); the manager is not yet wired into `cmd/server`.

### 2026-08-24 · P1-02
`internal/ha/gateway.go` holds the exact-match allow-list — 21 commands, each observed answering live HA 2026.8.3 in P0-04/P0-05/P0-07 — enforced at the top of `Manager.Call`, before a session is acquired or a frame encoded.
**Surprise:** the check cannot live in `callOn`: its `transmitted=false` means "retry on the next connection", so a denial there would loop forever instead of failing. Denial must precede the retry loop, which also makes it independent of whether HA is reachable.
**Left open:** `supervisor/api` is denied today only by absence — the named deny set and its distinct reason are `P1-07`. `config_entries/get_single` is deliberately unlisted (P0-04's consequence).

### 2026-08-24 · P1-07
Named deny set (`supervisor/api`, F-13) added to `internal/ha/gateway.go`'s `checkCommand`, ahead of the allow-list. The chokepoint moved from `Manager.Call` into `session.write` — the function that actually calls `conn.Write` — so the guarantee no longer rests on "`Call` is the only caller" being true forever.
**Surprise:** enforcing in `session.write` meant `callOn`'s `write` error handling needed a third case: a policy denial there is final (`transmitted=true`), not a connection failure to retry on — reusing the existing tri-state signal correctly required naming *why* in a comment, since "transmitted" no longer literally means bytes went out.
**Left open:** none — `TestSession_Write_DeniedCommand_NeverReachesSocket` dials and authenticates a raw connection outside `Manager`/`Call` entirely to prove the chokepoint holds without them.

### 2026-08-25 · P1-03
`internal/ha/rest.go` is now a `RESTClient` with typed per-route methods (`Config`, `States`, `State`, `HistoryPeriod`, `LogbookPeriod`); `gateway.go` gained the exact-match route-template table, a GET-only method check ahead of it, and `validateEntityID`. Query filters are typed option structs, never a caller-supplied map.
**Surprise:** the method check is nearly moot by construction — the client has no method parameter at all, so `TestNoNonGetRequestPathExists` (a source assertion that `http.MethodPost`/`Put`/`Patch`/`Delete` never appear in `rest.go`) is the stronger guarantee, and `checkRoute`'s method arm is the backstop for whatever calls it next. Matching the *template*, not the expanded path, is what keeps the table from becoming a pattern rule once `{entity_id}` is substituted.
**Left open:** `ErrNotFound` and `ErrResponseTooLarge` were added here because 404 and an oversized body have no honest home in the existing sentinels; `P1-04` owns consolidating the taxonomy. The client is not wired into `cmd/server` yet — no caller exists until Phase 03.

### 2026-08-25 · P1-04
Added `ErrUnsupported` and `ErrDeadline` to close the taxonomy. `CommandError.Unwrap()` maps HA's wire codes onto it (`unauthorized`→`ErrUnsupported`, `not_found`→`ErrNotFound`, evidenced by P0-05's admin-gate probe) while `errors.As` still recovers the original code/message. A `wrapDeadline` helper wraps `context.DeadlineExceeded` as `ErrDeadline` at every site that previously returned `ctx.Err()` raw, in both `manager.go` and `rest.go`.
**Surprise:** "Supervisor absent" in the DoD had no literal Supervisor-only code path to exercise yet (this binary only reaches Core through the one proxy) — the closest real, evidenced scenario is the admin-gate `unauthorized` result P0-05 already observed, which is exactly the "optional source, degrade don't abort" case the sentinel is for.
**Left open:** `ErrBudgetExceeded` is not defined here — it is `P2-01`'s, in `internal/policy`, once that package exists.

### 2026-08-25 · P1-05
`internal/model` now holds `Entity`, `DeviceRef`, `Integration`, `Area`, `Automation`, `Health`, `Evidence` — plain typed structs, zero imports. `internal/ha/mapping.go` maps `config/entity_registry`, `config/device_registry`, `config/area_registry`, `config_entries/get` and `automation/config` payloads into them: each raw element decodes to `map[string]any` first and every field read goes through a permissive accessor (`optString`/`optBool`/…) that reports absence or a type mismatch instead of a failed type assertion, so a bad element degrades to one `Partial` value with a `PartialReason` rather than aborting the batch or zeroing silently.
**Surprise:** none of the candidate commands had an observed full-element schema for `config/entity_registry/list` itself — P0-04's probe recorded field *names* (via `list_for_display`'s abbreviated keys and `get`'s superset note) but withheld a literal element, by design (F-9). The mapping and its fixtures are therefore built from HA's documented registry storage schema, not a captured payload; `Health`/`Evidence` have no HA payload to map from at all — they are analysis outputs (§12) and only got the domain type today, no mapper.
**Left open:** device registry's `config_entry_id` singular vs. `config_entries` array (Core 2026.8's device-ownership change, §8) is handled defensively (`firstConfigEntry`) but unverified against a live payload — worth a `cmd/spike` glance before `P1-06` caches it. `Automation`/`Integration`/`Area` mapping has no captured-fixture equivalent of the malformed/Unicode tests beyond Entity/Device — those two carry the DoD's required coverage; the others got well-formed-only tests since the DoD names them collectively but the failure-mode requirement (Appendix B) is written around entity/device names specifically.

### 2026-08-25 · P1-06
Added `cachedValue[T]` (generic TTL + single-flight refill) and `RegistryCache` in `internal/ha`, wrapping `Manager.Call` + `P1-05`'s `Map*` functions for entity/device/area/config-entries registries, each with its own TTL from doc §16. Every served slice comes back with its `observedAt`. Tests use a narrow `caller` interface and a fake with a controllable block channel and injectable clock, so TTL expiry and the single-flight thundering-herd guarantee are asserted deterministically, without real sleeps or a live WebSocket.
**Surprise:** the single-flight design choice (propagate error, don't fall back to a stale value on a failed refetch) meant `TestRegistryCache_RenameAfterExpiry_Reflected` needed a within-TTL assertion too, not just before/after — otherwise a caching bug that refetched on every call would still pass the "after expiry" check by accident.
**Left open:** no method yet for floor/label/category registries or system info (doc §16's other two rows) — nothing in Phase 01-03 calls them yet; add when a tool needs one.

### 2026-08-25 · P1-08
Added `SupervisorClient` (`internal/ha/supervisor.go`) alongside `RESTClient`: its own base (`http://supervisor`), its own exact-match GET-only route table (`allowedSupervisorRoutes`, `checkSupervisorRoute`) holding only the eleven endpoints the default role and `api_bypass` actually grant. `MapSupervisorInfo` decodes `/supervisor/info` with a strictly-typed struct rather than the permissive `map[string]any` style the registry mappers use, so a retyped field fails the call instead of degrading to `Partial`. `addon/config.yaml` flipped to `hassio_api: true` (role still unset/default).
**Surprise:** Supervisor unreachable maps to `ErrUnsupported`, not `ErrUpstreamUnavailable` — deliberately different from `RESTClient`'s Core-outage sentinel, since a Core-based diagnostic must keep working with Supervisor absent (CLAUDE.md, Reliability); reusing Core's sentinel would have made that degradation indistinguishable from a Core outage upstream.
**Left open:** the other ten endpoints (`Info`, `OSInfo`, `HostInfo`, …) return `json.RawMessage`, unmapped — no tool consumes them yet; `P3-02`/`P3-06` map what they actually need when they land.
