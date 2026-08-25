# Journal — Phase 00 Spike & Foundations

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

### 2026-08-23 · P0-01
Pinned `github.com/modelcontextprotocol/go-sdk` v1.7.0 (protocol 2026-07-28) and
added `.github/workflows/ci.yml` running `make check` + `make release`.
**Surprise:** the SDK exposes no public protocol-version constant, so
`TestSDKProtocolVersion` asserts it behaviorally — connect an in-memory
client/server pair and check the negotiated `InitializeResult.ProtocolVersion`.

### 2026-08-23 · P0-02
Added `addon/{config.yaml,build.yaml,Dockerfile,apparmor.txt,rootfs/run.sh}` with
`TestAddonManifestSecurityPosture` (hand-rolled scanner, no YAML dependency —
the assertions only need a handful of top-level scalars and the `map:` list).
**Surprise:** `homeassistant/aarch64-base:3.19` doesn't exist on Docker Hub
(latest pinned tag is `3.18`); verified the arm64 image actually builds with
`docker buildx build --platform linux/arm64` before trusting the manifest.
**Left open:** whether Supervisor's own App builder uses the addon folder or
the repo root as build context — this Dockerfile needs repo root (it compiles
from `cmd/`/`internal/`), verified manually, not through the Supervisor builder.

### 2026-08-23 · P0-03
Added `internal/ha` (WebSocket auth handshake, ping round trip, REST GET,
bounded-backoff `ConnectWithBackoff`) on `github.com/coder/websocket` — stdlib
has no WS client, so this is the one added dependency. Auth failure returns
`ErrAuthFailed` and is never retried; only transient dial/read failures back off.
**Surprise:** no live Supervisor was reachable from this environment, so the
integration test (`-tags=integration`) defaults to a local server replaying the
documented `auth_required`/`auth_ok`/`auth_invalid` and `/api/config` shapes
(doc §15.1) when `HA_TEST_WS_URL`/`HA_TEST_REST_URL` aren't set — a real
Supervisor can still be pointed at via those env vars.
**Left open:** command allow-list, request correlation for concurrent
in-flight requests, and live reconnect-on-drop are all P1-01; this client
handles one request at a time.

### 2026-08-23 · P0-04
Registry & config-entry APIs verified against live HA 2026.8.3 in two runs
(admin, non-admin) via a throwaway `cmd/spike` that reports field names and
types only; evidence in `docs/research/2026-08-23-ha-registry-apis.md`.
**Surprise:** `config_entries/get` is readable by a non-admin while
`config_entries/get_single` — the same data, one entry — is refused
`unauthorized`; and both principals got byte-identical payloads, so HA does no
per-user filtering of registries at all (F-10). Also `list_for_display`
returned 469 entities where `list` returned 952: a different population, not a
compression.
**Left open:** three registries were empty so their element schema is
unobserved; the `aliases: [null]` result is stable but unexplained.

### 2026-08-23 · P0-05
Automation config and traces verified against live HA 2026.8.3, admin and
non-admin: every command exists and works, and every one of them is admin-gated.
Evidence in `docs/research/2026-08-23-ha-automation-traces.md`; F-3 closed.
**Surprise:** `trace/get` returns whole `from_state`/`to_state` objects and a
`context.user_id` — a trace is personal data, not just diagnostic structure (F-12).
**Left open:** whether the App's principal is admin at all — `P0-06` now decides whether two tools exist.

### 2026-08-23 · P0-05
Automation config and traces verified against live HA 2026.8.3, admin and
non-admin: every command exists and works, and every one of them is admin-gated.
Evidence in `docs/research/2026-08-23-ha-automation-traces.md`; F-3 closed.
**Surprise:** `trace/get` returns whole `from_state`/`to_state` objects and a `context.user_id` — a trace is personal data, not just diagnostic structure (F-12).
**Left open:** whether the App's principal is admin — `P0-06` now decides whether two tools exist at all.

### 2026-08-23 · P0-06
Supervisor permission matrix derived from the security middleware at pinned tags
(Supervisor 2026.08.0, Core 2026.8.3): `docs/research/2026-08-23-supervisor-permissions.md`.
`list_apps` needs one manifest line (`hassio_api: true`, default role) — filed as a `needs-decision`, not applied. F-4 closed; F-2 and F-3 residues closed with it.
**Surprise:** the App's Core principal is HA's own Supervisor user, in `GROUP_ID_ADMIN` — so it is admin, and Core's `supervisor/api` WS command lets any admin call any Supervisor endpoint with any method, making `hassio_api: false` a declaration rather than a boundary (F-13).
**Left open:** nothing was called live; the confirming three curls are written into the research file for the first real deployment.

### 2026-08-23 · `P0-07`
Rewrote `cmd/spike` as a history/statistics probe (REST `/api/history/period` ×4
parameter variants × 24h/7d, WS `history_during_period`, the three recorder
statistics commands) and wrote the evidence up in
`docs/research/2026-08-23-ha-history-statistics.md`. Statistics win by 1–3
orders of magnitude; source order is statistics → WS history → REST fallback.
**Surprise:** `no_attributes` alone barely helps — it still emits an *empty*
`attributes` object per row (272 KB vs 510 KB at 24h); `minimal_response` is the
parameter that actually shrinks the answer, and it silently collapses
consecutive same-state rows, so it is a summary rather than a subset. F-9's leak
also recurred (F-15): its `isIDKey` never covered entity ids, and history keys
its result by one.
**Left open:** multi-entity query cost (F-14) — every number here is one entity.

### 2026-08-24 · `P0-08`
Read Supervisor's own builder source (`supervisor/apps/build.py`,
`home-assistant/supervisor@main`) instead of a live build — confirms F-8:
build context is the App's own folder, read-only, never the repo root.
`docs/research/2026-08-24-supervisor-addon-build-context.md`.
**Surprise:** `addon/Dockerfile` fails at the very first `COPY` (`go.mod`)
under a real Supervisor build despite passing the repo-root manual check every
session has been running since `P0-02` — the local DoD was never testing the
real path. Filed as **F-16**, three candidate fixes named, none applied.
**Left open:** which of the three fixes (relocate / vendor / prebuilt image)
to take — deferred to the next `devflow plan`.

### 2026-08-24 · P0-09
`cmd/spike` rewritten as a multi-entity cost ladder (1/10/50/200 ids × 24h/7d,
batched against a prefix-summed single-entity baseline); owner ran it against
HA 2026.8.3. Batching wins on time at every rung (1.4×–50×) and costs nothing in
bytes; budget starting values derived in
`docs/research/2026-08-24-ha-multi-entity-query-cost.md`. F-14 closed.
**Surprise:** cost tracks recorded rows, not entity count — 150 extra quiet ids
added 30% bytes, while a batched statistics answer is inexplicably ~30% *larger*
than the same ids fetched one by one (F-17).
**Left open:** nothing above 200 ids measured, so doc §10's composite 500-entity
limit is not carried forward.

### 2026-08-24 · P0-10
Read `home-assistant/supervisor@main`'s `docker/manager.py`, `docker/interface.py`,
`const.py`, `validate.py`, `api/docker.py`, plus the frontend's
`panels/config/apps/`: Supervisor does support private-registry pulls, keyed by
hostname in `/data/docker.json`, entered at Settings → Add-ons → Registries.
`docs/research/2026-08-24-supervisor-private-registry-pull.md`. F-19 answered;
`P0-11` unblocked.
**Surprise:** the auth-error clarity is conditional — a *wrong* stored
credential raises a typed, registry-named `DockerRegistryAuthError`, but a
*missing* one (nothing configured yet) falls through to a generic `DockerError`
carrying raw registry text, indistinguishable from image-not-found.
**Left open:** did not independently confirm `docker.json` survives an OS
update (only that it lives beside other files this project already treats as
durable).

### 2026-08-25 · `P0-11`
App ships as a published multi-arch image; live-verified pulling and starting
on real Home Assistant OS on a Raspberry Pi.
**Surprise:** three failures only appeared on real Supervisor, none caught by
`make check` or a local `docker build`/`docker run`: a repository needs a root
`repository.yaml` to be recognized at all; `COPY` doesn't make a binary
executable; and AppArmor denies `exec` as a check separate from and stricter
than Unix file permissions (`mr` vs `mrix`) — a local `docker run` without the
real profile attached would never have caught the third one.
**Left open:** none — F-16 resolved, DoD's `Live:` clause satisfied.
