# Phase 00 — Spike & Foundations

**Milestone:** M0 (roadmap "Phase 0 — Spike") · **Target version:** v0.1

> 🧠 **Stronger model recommended for the verify tasks.** This phase decides
> which Home Assistant APIs the whole v1 surface is allowed to rest on. Every
> allow-list entry, every tool schema and every compatibility adapter in phases
> 01–04 inherits what is established here. Getting a schema wrong is cheap to fix
> now and expensive to fix after twenty tools depend on it.

## Goal

Prove that a Go process running as a Home Assistant App can reach Core over the
Supervisor proxy, and replace the architecture doc's *assumed* API surface with
an *observed* one. When this phase closes, the exact WebSocket commands, REST
routes and Supervisor endpoints available to a minimal-permission App on the
target HA release are written down as dated evidence in `docs/research/`, and
the gateway allow-list of Phase 01 can be derived from that file rather than
guessed.

No MCP exposure is required in this phase. A throwaway `cmd/spike` binary or an
integration-tagged test is an acceptable vehicle for the verification tasks.

## Depends On

Nothing. This is the first phase.

## Add Under

```text
cmd/server/          # entry point (stub already present)
internal/ha/         # first real connection code
addon/               # Home Assistant App packaging
.github/workflows/   # CI
docs/research/       # dated evidence produced by the verify tasks
```

## Design Notes

- **Evidence, not opinion.** A verify task closes by writing a dated file under
  `docs/research/` containing the actual request and the actual response shape
  (redacted), plus the HA version it was observed against. "The docs say X" is
  not a result; "HA 2026.8.x returned this" is.
- **Never store a long-lived HA token in the App.** Authentication is
  `SUPERVISOR_TOKEN` from the App environment, read once at startup, never
  logged, never returned by any tool, never written to research files.
- **The spike is throwaway; its findings are not.** Code written here may be
  discarded in Phase 01. The research files must survive.
- **A negative result is a result.** If an API is unavailable to a
  minimal-permission App, that is the answer, and it changes the Phase 01 tool
  catalog. Record it and route the affected tool to `unsupported`, do not widen
  App permissions to make a tool possible without an explicit decision.
- Cross-compilation target is `linux/arm64` (Raspberry Pi, aarch64) first,
  `linux/amd64` second. CGO stays disabled.

## Tasks

- [x] **`P0-01` — Pin the official MCP Go SDK and add CI** — add
  `github.com/modelcontextprotocol/go-sdk` to `go.mod` at a pinned version
  supporting protocol 2026-07-28, and a GitHub Actions workflow running
  `make check` plus `make release` (linux/amd64 + linux/arm64 cross-build).
  **DoD:** `go.mod`/`go.sum` pin an exact SDK version, not a branch; a test
  asserts the SDK's advertised protocol version matches the constant the project
  expects (`TestSDKProtocolVersion`), so an SDK bump that changes the wire
  version fails the build instead of shipping silently; `make release` produces
  both binaries; CI is green on a clean checkout with no golangci-lint installed.

- [x] **`P0-02` — Home Assistant App packaging skeleton** — `addon/config.yaml`,
  `addon/build.yaml`, `Dockerfile`, `rootfs/` run script and an AppArmor profile,
  with the security posture from the architecture doc §15.2: `homeassistant_api:
  true`, `hassio_api: false`, no `docker_api`, no `host_network`, no
  `full_access`, no privileged caps, protection mode on, **no `/config`
  mapping**, arch list `aarch64` + `amd64`.
  **DoD:** a test (`TestAddonManifestSecurityPosture`) parses `addon/config.yaml`
  and asserts each forbidden key is absent-or-false and that `map:` contains no
  `config` entry — so a future edit that quietly grants filesystem or Docker
  access fails the build; the image builds for `linux/arm64`.

- [x] **`P0-03` — Supervisor proxy connectivity** — connect to
  `ws://supervisor/core/websocket`, complete the `auth` handshake with
  `SUPERVISOR_TOKEN`, issue one trivial read command, and perform one
  `GET http://supervisor/core/api/config`. Include reconnect-with-backoff at its
  simplest form; the full connection manager is `P1-01`.
  **DoD:** integration-tagged test against a real or recorded HA proving auth
  success, one successful WS command round-trip and one successful REST GET;
  unit test proving the token never appears in any log line emitted during the
  handshake (`TestHandshakeDoesNotLogToken`); unit test proving a wrong token
  produces a typed auth error rather than a retry storm.

- [x] **`P0-04` — Verify registry & config-entry read APIs** 🧠 `needs-verify` —
  resolves **F-1** and **F-2**. Establish, against the target HA release, the
  exact WebSocket command names, request payloads, response schemas and
  permission requirements for: entity registry list/get, device registry list,
  areas, floors, labels, and config entries (list + get).
  **DoD:** `docs/research/<date>-ha-registry-apis.md` records, per command, the
  verbatim command name, a redacted sample response, whether it required admin,
  and the HA version observed; every command that Phase 01's allow-list will
  contain appears there or is explicitly marked unavailable.

- [x] **`P0-05` — Verify automation config & trace retrieval** 🧠 `needs-verify` —
  resolves **F-3**. Determine what an external App can actually read about
  automations: config, traces, trace lists, and what requires admin or
  `/config`.
  **DoD:** `docs/research/<date>-ha-automation-traces.md` states, with observed
  evidence, whether `get_automation` and `get_automation_traces` are
  implementable read-only from an App; if not, it names what the fallback
  evidence source is (logbook, state history, `last_triggered`) and the tools are
  re-scoped by a follow-up `devflow plan`, not silently weakened.

- [x] **`P0-06` — Verify Supervisor endpoints under minimal permissions** 🧠
  `needs-verify` — resolves **F-4**. Establish which Supervisor endpoints (host
  info, OS info, Core stats, App list) respond with `hassio_api: false`, which
  need it, and which need a role above the default.
  **DoD:** `docs/research/<date>-supervisor-permissions.md` maps each endpoint
  the doc's `get_system_health` and `list_apps` tools need to the exact minimum
  permission it requires; the App manifest of `P0-02` is either confirmed
  sufficient or a specific, justified permission delta is proposed as a
  `needs-decision` entry — never applied inside this task.

- [x] **`P0-07` — Verify recorder history & statistics APIs** 🧠 `needs-verify` —
  resolves **F-5**. Establish the behavior of REST `/api/history/period` under
  bounded ranges and of the recorder statistics WebSocket API on the target
  release, including which is cheaper on a Raspberry Pi-sized recorder DB.
  **DoD:** `docs/research/<date>-ha-history-statistics.md` records observed
  response shapes, `minimal_response`/`no_attributes` parameter behavior,
  measured latency for a 24h and a 7d single-entity query against a real
  recorder, and a recommendation for which source the statistics tools should
  prefer with the other as documented fallback.

- [x] **`P0-08` — Verify the Supervisor App builder's build context** `needs-verify`
  — resolves **F-8**. `addon/Dockerfile` compiles the Go binary in its build
  stage, so it needs the repository root as build context; it was only ever
  verified by hand with `docker buildx build -f addon/Dockerfile .` from the
  root. Establish what Supervisor's own builder actually passes as context for a
  local App, and whether the `P0-02` layout builds under it unchanged.
  **DoD:** `docs/research/<date>-supervisor-addon-build-context.md` states, from
  an observed local App build on a real Supervisor (or from the builder's own
  published behavior with the exact source cited), which directory becomes the
  build context and whether `addon/Dockerfile` resolves `go.mod`, `cmd/` and
  `internal/` from it; if it does not, the file names the concrete layout change
  required (relocated Dockerfile, vendored build, or a prebuilt image reference)
  and that change is planned as its own task, never applied inside this one.

- [x] **`P0-09` — Verify multi-entity history & statistics cost** 🧠 `needs-verify`
  — resolves **F-14**. `P0-07` measured every query against exactly one entity,
  so the cost of the queries a fleet-wide detector actually issues is unknown:
  whether `history/history_during_period` and
  `recorder/statistics_during_period` scale linearly in the number of entity
  ids, and whether one batched call beats N single-entity ones. Extend
  `cmd/spike` with an entity-id list; the owner runs it, as before.
  **DoD:** `docs/research/<date>-ha-multi-entity-query-cost.md` reports, against
  the same live HA and recorder, wall-clock latency and response bytes for both
  commands at **1, 10, 50 and 200 entity ids** over a 24h and a 7d window, each
  compared against the sum of the equivalent single-entity calls; it states
  whether batching wins, and at which entity count a single call crosses one
  second and the doc §10 byte cap. From those numbers it names concrete starting
  values for `MaxEntities`, `MaxHistoryPoints` and `MaxBytes` per budget class,
  attributed to the measurement — so `P2-01` cites evidence rather than doc §10's
  admitted guesses (§26). No id from the installation appears in the report.

- [x] **`P0-10` — Verify Supervisor can pull an App image from a private registry**
  `needs-verify` — resolves **F-19**. The App-distribution decision below chose a
  **private** GHCR package, which is only implementable if Supervisor can
  authenticate to a private registry when pulling an App's `image:`. Nobody has
  established that it can, or how the credentials are supplied on Home Assistant
  OS. World-discoverable: read Supervisor's own source (`supervisor/docker/`,
  `supervisor/store/`, the registry/credential handling around
  `DockerAPI.pull_image` and `ATTR_REGISTRIES`) at a pinned tag, exactly as
  `P0-08` read `apps/build.py`, and confirm against the published Supervisor
  documentation.
  **DoD:** `docs/research/<date>-supervisor-private-registry-pull.md` states,
  citing the exact Supervisor source paths and version read: whether Supervisor
  supports pulling an App image from an authenticated registry at all; if it
  does, **where the credential lives** (the store's registries file, its shape,
  and the UI path an owner uses to enter it), whether it survives a Supervisor
  restart and an OS update, and what the failure looks like when the credential
  is absent or expired — specifically whether the App shows a clear auth error
  or an indistinguishable "image not found". If it does **not** support it, the
  file says so plainly, because that overturns the private half of the decision
  below and sends it back to the owner. No credential, token or hostname from
  the owner's installation appears in the report.

- [x] **`P0-11` — Ship the App as a published multi-arch image, not a local build**
  `live-verify` — resolves **F-16**. Unblocked 2026-08-24: `P0-10` confirmed
  Supervisor can pull from a private registry (see its DoD/research doc).
  Implements the
  App-distribution decision below. Delete `addon/Dockerfile` and
  `addon/build.yaml`; add `image:` to `addon/config.yaml` with the `{arch}`
  placeholder so Supervisor substitutes `aarch64`/`amd64`; add a release
  workflow that cross-builds both architectures and pushes them to GHCR under a
  tag equal to `config.yaml`'s `version:`. Document the install path — including
  the registry credential step `P0-10` establishes — in `docs/`, since a private
  package cannot be installed by pasting a repository URL alone.
  **DoD:** `TestAddonManifestSecurityPosture` still passes unchanged — the
  security posture is not what this task touches; a new test
  (`TestAddonManifestImageIsPinnedToVersion`) parses `addon/config.yaml` and
  asserts that `image:` is present, contains the `{arch}` placeholder, and that
  the tag the release workflow pushes is derived from the same `version:` field
  rather than written twice — so a version bump that forgets the image tag fails
  the build instead of leaving Supervisor pulling a stale binary; a further test
  asserts `addon/Dockerfile` and `addon/build.yaml` no longer exist, so the dead
  build path cannot quietly return. `make check` green. **Live:** the owner
  installs the App on the real Raspberry Pi from the published private image and
  reports that it pulls and starts; the task does not close on green tests alone,
  because the whole point of the change is a deploy path that only real
  Supervisor exercises.

## Decisions

- [ ] **`needs-decision` — Supported Home Assistant version policy**
  Q8 from the architecture doc. The CI compatibility matrix, the number of
  adapter variants the project must carry, and how loudly an unsupported API
  fails all follow from this. Options: **current release only** (smallest
  adapter surface, users must stay current); **current + N previous** (N is the
  cost multiplier on every compatibility-sensitive adapter and on CI time);
  **best effort, no promise** (cheapest now, produces silent wrong results
  later — the failure mode the doc's §18 exists to prevent). A model cannot pick
  this: it depends on which HA version the owner's Raspberry Pi actually runs
  and how often they upgrade.

- [ ] **`needs-decision` — Supervisor permission level for the App manifest**
  Raised by `P0-06`; evidence in
  [`docs/research/2026-08-23-supervisor-permissions.md`](../../research/2026-08-23-supervisor-permissions.md).
  The current manifest (`hassio_api: false`) reaches `/info`,
  `/addons/self/info`, `/addons/self/stats` and `/supervisor/ping` — enough for a
  partial `get_system_health` (all four component versions, hostname, machine,
  arch, Core state, own resource use) and **not enough for `list_apps`**, which
  has no enumeration path at all and would answer `unsupported`. Options:
  **(a) keep `hassio_api: false`** — smallest surface, `list_apps` is dropped
  from v1 and `get_system_health` ships partial; **(b) `hassio_api: true` with
  the default role** — one line, `hassio_role` is already `default` when unset;
  adds `/supervisor/info` (which embeds the installed-App inventory, so
  `list_apps` becomes implementable), `/os/info`, `/host/info` (disk),
  `/resolution/info` (repairs/unsupported), `/network/info`, `/hardware/info`,
  `/jobs/info`, and no `*/stats` and no write-capable path whatsoever;
  **(c) a role above default** — `homeassistant` would add Core CPU/RAM
  (`/core/stats`), `manager` would add the richer `/addons` list and other Apps'
  stats, and both grant broad `/core/.+` or `/host/.+` write access that doc
  §15.2 rules out for observer v1. A model cannot pick this: it trades the
  project's stated minimal-permission posture against two of the twenty v1
  tools, and it is the owner's App on the owner's installation.

  Not a factor in the trade, but on the record: `hassio_api: false` does not
  *prevent* Supervisor access — see **F-13**. The choice is about declared
  intent and the blast radius of a future bug, not about a wall.

- [x] **App distribution — published image, not a local Supervisor build** — decided 2026-08-24

  **Decision:** The App ships as a **prebuilt multi-arch image pulled from a
  private GHCR package**, referenced from `addon/config.yaml`'s `image:` field.
  Supervisor never builds it. `addon/Dockerfile` and `addon/build.yaml` are
  deleted, so no local-build path remains. Running the same binary outside the
  App — `make run` against a networked HA with an owner-supplied long-lived
  token — is unaffected and stays supported for development, per doc §15.

  **Why:** `P0-08` established that Supervisor's builder mounts only `addon/`
  read-only as build context, so the Dockerfile's `COPY go.mod`/`cmd/`/`internal/`
  cannot resolve (F-16) — the App is currently uninstallable on real hardware.
  Of the three fixes that task named, publishing an image is the only one that
  does not either contradict `CLAUDE.md`'s module layout or introduce a
  generated source tree inside `addon/` that must be committed for a local
  install to work. It also keeps Go compilation off the Raspberry Pi entirely,
  which is the machine `CLAUDE.md`'s performance section says is the hot
  constraint.

  **Rejected:** *Relocate packaging to the repository root* — cheapest build fix
  and needs no infrastructure, but it makes the repo root the App folder,
  scatters manifest files there against the documented layout, and leaves every
  install compiling Go beside a running Home Assistant. *Vendor the Go source
  into `addon/`* — keeps the layout only on paper: Supervisor mounts the folder
  read-only and never runs a Makefile, so the vendored tree would have to be
  committed (duplicating the whole source in git) or produced by CI into a
  separate published repository.

  **Consequences:** A publish pipeline and version-tag discipline become part of
  the release: `config.yaml`'s `version:` and the pushed image tag are one fact
  and must not be written twice — `P0-11`'s DoD asserts that. The package being
  private means an installer must supply registry credentials to Supervisor
  before the pull succeeds; **whether Supervisor supports that at all is not yet
  established** (F-19), which is why `P0-10` verifies it before `P0-11` builds on
  it. If that verification comes back negative, the private half of this decision
  is void and returns to the owner — the published-image half stands either way.
  Phase 00's Definition of Done clause "the image builds for `aarch64`" is
  superseded by this decision and is restated below.

- [x] **Module path / repository home** — decided 2026-08-23

  **Decision:** The repository is `https://github.com/freemanjava/ha-explorer-mcp.git`,
  wired as `origin`, and the Go module path is
  `github.com/freemanjava/ha-explorer-mcp`.

  **Why:** The scaffolded path was an admitted placeholder (F-7). Settling it now
  cost one line in `go.mod` because no internal import existed yet; after `P0-01`
  pins dependencies and Phase 01 lands, the same change rewrites every import in
  the tree.

  **Consequences:** All `internal/` imports use this prefix. The repository name
  (`ha-explorer-mcp`) deliberately differs from the product and binary name
  (`ha-inspector-mcp`, per the architecture doc); neither is being renamed to
  match the other. CI in `P0-01` targets this remote.

## Phase Definition of Done

- A Go binary authenticates to Core through the Supervisor proxy and performs at
  least one WebSocket read command and one REST GET, with the token never logged.
- The App is installable on real Home Assistant OS from a published multi-arch
  image (`P0-11`) — superseding this phase's original "the image builds for
  `aarch64`", which the App-distribution decision made moot: nothing builds it
  locally any more. Its manifest is asserted by test to carry no `/config` map,
  no Docker socket, no host network and no `full_access`.
- Every `unknown` finding this phase owns — F-1 … F-5, F-8, F-14 and F-19 — is closed by a
  dated file in `docs/research/`, and each Phase 01 tool is marked implementable,
  re-scoped or unsupported on that evidence.
- The `needs-decision` entry above is answered and recorded here.
- `make check` is green; CI is green for linux/amd64 and linux/arm64.
