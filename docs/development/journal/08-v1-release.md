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

