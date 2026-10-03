# App lifecycle and client paths (P8-01, F-37)

Dated snapshot. Re-check by repeating the commands below.

### 2026-10-03 · Does the server stay up with no stdin attached? (off-box half)

**Kind:** world-discoverable
**Method:** `docker build -t hai-p801 .` of `main` (the published-image Dockerfile, colima, Docker 28.5.1, host arch); then
`docker run hai-p801 </dev/null` and `docker run -d hai-p801` (no `-i`, as Supervisor starts an App without `stdin: true`).
**Found:** both runs log `starting` → `server session connected` → `server session disconnected` → `stopped reason="session ended"`
and exit **0** about 60 ms after start (`State.Status=exited ExitCode=0`). The process does not wait for Core: its first WebSocket
connect attempt was still in backoff when it stopped. Expectation of F-37 confirmed.
**Not established:** Supervisor's reaction (App state shown, whether a watchdog or `boot: auto` restarts it, restart-loop behaviour) —
needs the Pi; this cannot be reproduced with plain Docker.
**Means:** as packaged, the App starts and stops itself immediately; no client can use it.

### 2026-10-03 · Does the server work if a client holds stdin open? (off-box half)

**Kind:** world-discoverable
**Method:** `docker run --rm -i hai-p801` fed `initialize`, `notifications/initialized`, `tools/list` as JSON-RPC lines, stdin held 3 s.
**Found:** `initialize` and `tools/list` both answered over stdout (tools incl. `analyze_automation_health`, all `readOnlyHint:true`);
log shows `session initialized`; exit 0 once stdin closed. The Core WebSocket was not reachable off-box (connect backoff WARN), as expected.
**Not established:** the same under Supervisor — whether `stdin: true` in `config.yaml` gives a usable attach path, and whether
`docker exec -i` from the SSH App (needs `docker_api`/host access there) works. `ha addons` CLI has no documented stdio attach.
**Means:** the stdio server itself is sound; the missing piece is purely *who holds its stdin*. Candidate paths, each still to be
observed on the Pi: (a) `stdin: true` + Supervisor API attach, (b) `docker exec -i` from outside the App, (c) a network transport
(contradicts the phase 01 stdio-only decision and adds client authentication — owner's call, D-08-1).
