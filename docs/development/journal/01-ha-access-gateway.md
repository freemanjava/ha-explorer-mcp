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
