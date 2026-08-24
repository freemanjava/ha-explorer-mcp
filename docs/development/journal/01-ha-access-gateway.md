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
