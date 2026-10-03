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

### 2026-10-03 · What does Supervisor do with the App? (Pi half, partial)

**Kind:** world-discoverable
**Method:** owner installed the `0.9.0` image on the Pi (HAOS, aarch64) and started the App; pasted the App log.
**Found:** the App **exits on its own**; whether Supervisor restarts it is settled by the next entry (an earlier version of this entry said "restarts in a loop" — that was an overreach). The log shows the same sequence as the off-box run — `starting` → `ha: websocket connection manager ready`
→ `server session connected` → `server session disconnected` → `stopped reason="session ended"` — about 65 ms after `starting`.
Unlike off-box, there is **no** `websocket connect failed, backing off` line: through the Supervisor proxy the Core connection manager
came up on the first try (readiness only; no tool call was observed). The log's first line,
`ha-inspector-mcp 0.0.0-dev: not implemented yet`, is from the phase 00 stub — that string exists in no current source
(`git log -S` finds it only in `bf41c7c`) — so it is a leftover from an earlier install, not from this image. The binary reports
`version=0.0.0-dev` although the image is tagged 0.9.0: the Dockerfile sets no `-X main.version`.
**Not established:** what restarts it (watchdog, `boot`, Supervisor's own restart policy — `config.yaml` declares none of them);
the restart interval and whether it backs off; the attach paths — `stdin: true`, `docker exec -i` from the SSH App — not yet tried.
**Means:** F-37 is confirmed on the real platform: the App exits 0 on EOF and serves nobody. The Core side
looks reachable, so the open question is only the client path (D-08-1).

### 2026-10-03 · Does Supervisor restart the exited App? (Pi, Info page + log)

**Kind:** world-discoverable
**Method:** owner read the App's Info page and pasted two consecutive `starting` log blocks.
**Found:** "Start on boot" on, "Watchdog" off. Starts at `16:56:07Z` and `17:03:28Z` — 7 min 21 s apart, each exiting within ~70 ms.
That is not a restart loop: a loop would show starts seconds apart, and with the watchdog off Supervisor has no reason to restart an App that exited 0.
**Not established:** whether the 17:03 start was manual (owner to confirm) and whether anything restarts it with the watchdog on.
**Means:** the App stays **stopped** after its immediate exit unless started again (by hand or at boot) — unusable, but not a crash-loop. The earlier "restarts (loops)" reading is withdrawn.

### 2026-10-03 · Does opening the Logs tab restart the App? (Pi, observed through the HA UI in Chrome)

**Kind:** world-discoverable
**Method:** agent drove the owner's logged-in HA UI (`/config/app/<slug>/info`, then `/logs`, then back), read-only clicks, no Start/Stop.
**Found:** the App page shows **Stopped**, version 0.9.0, Start on boot on, Watchdog off, Auto update off. After opening the Log tab and
watching it ~15 s ("Live" stream), the log held exactly the two earlier start blocks (16:56:07Z, 17:03:28Z) and no new start; back on
Info it was still Stopped. The leading `not implemented yet` line is still in the same log buffer, so the buffer persists across
image versions — that line is not evidence about 0.9.0.
**Not established:** the owner's report that the App "restarts when I go to the Logs tab" was not reproduced. The only evidence of
starts is the two blocks; whether the 17:03 start was a manual one is still unconfirmed.
**Means:** nothing restarts the App on its own with the watchdog off. It is stopped after its immediate exit, which is what F-37 predicted.

### 2026-10-03 · Can the SSH add-on reach the App container to attach to its stdin? (Pi, agent drove the HA UI Terminal)

**Kind:** world-discoverable
**Method:** agent opened the Terminal & SSH add-on's web terminal in the owner's logged-in HA UI (`/core_ssh`) and ran
`docker inspect addon_54cff362_ha_inspector_mcp …`. No settings were changed.
**Found:** `bash: docker: command not found`. The add-on's shell has no Docker CLI in its current configuration, so neither
`docker exec -i` nor `docker run -i --network hassio …` can be done from it.
**Not established:** whether the cause is the add-on's protection mode (not read — the Info page was not opened; it is a security
setting the agent does not change), and therefore whether the path would work at all with protection off. The off-box result
(server answers `initialize`/`tools/list` with stdin held open) is not repeated on the Pi.
**Means:** as the platform ships, a `docker exec -i` client path needs the owner to lower a Supervisor protection setting on a
different add-on — a weakening of the posture the App itself was built to keep (ADR-004/005). That is a cost the owner weighs in D-08-1,
not something the App can assume.
