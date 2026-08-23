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
