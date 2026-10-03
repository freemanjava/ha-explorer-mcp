# Phase 08 — v1.0 Release

**Milestone:** M2 close (roadmap "Phase 2 — Diagnostics", shipped) · **Target version:** v1.0

> Numbered 08 only because 06 and 07 are taken by the gated milestones; it runs
> **before** them. Order lives in `NEXT.md`'s queue, not in phase numbers.

## Goal

Ship v1.0: the doc §21 acceptance criteria met **on the real target** — an
aarch64 Raspberry Pi running Home Assistant OS, the App under protection mode —
with each criterion mapped to a test or a live observation, and an MCP client
actually able to reach the server. Phase 06's gate needs "v1 usage data", which
only exists after this.

## Depends On

Phase 05 (complete 2026-10-03). Opened by the owner at the 2026-10-03 `plan`.

## Add Under

```text
addon/                 # version bump; whatever D-08-1 requires of the manifest
docs/INSTALL.md        # how a client connects — absent today (F-37)
docs/research/         # the P8-01 observation and the P8-05 acceptance report
internal/ha/           # gateway_test.go — P8-03's reachability assertion
```

## Design Notes

- **Nothing has been observed running as an App with a client attached.** Every
  journal entry through phase 05 says "not observed against a live HA", and
  `cmd/measure` ran the tools in-process off-box. The stdio-only decision
  (phase 01) says the client "is connected to the process's stdin/stdout by
  whoever starts it" — but under Supervisor the starter is Supervisor, and no
  client is attached (F-37). That is the first thing this phase establishes,
  by observation, before anyone decides how to fix it.
- **Mapping, not re-testing.** Most §21 criteria are already asserted by tests
  (`TestGateway_*`, token-never-returned, budget refusals, the three
  `investigation_test.go` walks). The acceptance report cites those tests by
  name and adds live observations only where a test cannot stand in: the
  platform, protection mode, an HA restart, a real investigation.
- **The agent does not tag or push.** A release box ends at a tree ready to tag
  (`addon/config.yaml` version bumped, docs updated); the owner pushes the tag
  that triggers `release.yml`.

## Tasks

Ordered by dependency. `P8-01` comes first because its answer may change what
every later box has to do; `P8-03`, `P8-07` and `P8-04` are independent of the Pi
and run while the owner is busy there.

- [x] **`P8-01` · Observe the App under Supervisor — does it stay up, can a
  client reach it (F-37)** — `needs-verify`
  Cheap half first, off-box: run the published-image build locally with stdin
  **not** attached (`docker run` without `-i`) and record what the process does
  (expected, unverified: reads EOF at once and exits 0). Then on the Pi, with an
  image of current `main` installed (the owner publishes a pre-release, e.g.
  `0.9.0`; the agent prepares the version bump only): App state after start,
  Supervisor log lines, restart/watchdog behaviour, and whether any client path
  exists today — `docker exec -i` from the SSH App, Supervisor's `stdin: true`.
  The owner runs the Pi half and pastes the output; no token reaches the agent.
  **DoD:** a dated report in `docs/research/` stating, for each observation,
  what was run and what happened, and listing the candidate client paths with
  what each one was observed to require. No code change.
  **Done 2026-10-03:** exits 0 ~65 ms after start and stays stopped (F-37 confirmed
  on the Pi); stdio works with stdin held (off-box); `docker exec -i` unavailable
  from the SSH App as shipped; `stdin: true` untried (needs an image change).
  Report: `docs/research/2026-10-03-app-under-supervisor.md`.

- [x] **`P8-03` · The allow-list matches the reachable surface (F-25)**
  Delete `recorder/list_statistic_ids`, `recorder/get_statistics_metadata` and
  `recorder/statistics_during_period` from `allowedCommands` and their
  constants from `internal/ha/gateway.go` (no production code sends them; F-17
  closed `wont-fix`, F-36 deferred). Home: `internal/ha/gateway_test.go`,
  because the property is the gateway's. `cmd/spike` keeps its own copies — it
  does not go through this gateway.
  **DoD:** `TestGateway_AllowList_EveryEntryHasACaller` parses the non-test Go
  files of `internal/` (`go/parser`, no shelling out) and fails if any
  allow-listed command or route constant is referenced nowhere outside
  `gateway.go`; it is shown red by temporarily removing one real caller. The
  three statistics commands are denied by `checkCommand` (existing
  not-allow-listed path, asserted). `policy.SourceStatistics` stays: it is
  policy, not gateway surface, and removing it is not this box. `make check`
  green.
  **Done 2026-10-03:** the reachability test also found six more uncalled
  entries (F-40); they sit in a shrink-only exemption set in the test.

- [x] **`P8-07` · Remove the unwired Core REST adapter (F-38)** — per D-08-2,
  `blocked:P8-03`
  Delete `internal/ha/rest.go` (`RESTClient`, `HistoryOptions`,
  `LogbookOptions`), the five `Route*` constants, `allowedRoutes`, `checkRoute`
  and `validateEntityID` with their tests; keep `ha.SupervisorClient`'s own
  GET-only check untouched. Update CLAUDE.md's module layout line
  (`internal/ha/` "websocket, rest, …") in the same change.
  **DoD:** `go build ./...` shows no Core REST client is linked in (`grep -rn
  RESTClient internal cmd` empty); `TestGateway_AllowList_EveryEntryHasACaller`
  still green; mutation-denied-before-transmission still asserted for the
  WebSocket path and the Supervisor adapter; `make check` green.
  **Done 2026-10-03:** `entityIDPattern` stays (the mapper uses it) and moved to
  `mapping.go`; the Supervisor client took over the size cap and deadline backstop
  (renamed `maxSupervisorResponseBytes`, `defaultSupervisorTimeout`) and the three
  tests that asserted them. The recorded-HA integration test lost its REST half.

- [ ] **`P8-04` · `golangci-lint` clean**
  The linter has never run on this codebase — every phase 05 journal entry
  notes it skipped. Install it locally, run `golangci-lint run`, fix what it
  reports. A finding that would need a design change is filed, not fixed here.
  **DoD:** `golangci-lint run` exits 0 on the branch; the journal entry names
  the linter version and how many issues were fixed. CI is unchanged — it stays
  lint-free by design (`ci.yml`, CLAUDE.md "Commands").

- [ ] **`P8-02` · Implement the client path D-08-1 chooses** — `blocked:D-08-1`,
  `live-verify`
  *Written when D-08-1 is decided — not before.* Its DoD will at least require a
  real MCP client completing `initialize` and `tools/list` against the App on
  the Pi, and `docs/INSTALL.md` describing that connection step by step.

- [ ] **`P8-05` · §21 acceptance walk on the Pi** — `blocked:P8-02`, `live-verify`
  For each of the twelve doc §21 criteria: the test(s) that assert it, by name,
  or a live observation on the Pi — at least: App running under protection mode
  on aarch64 with the §15.2 flags (`addon/config.yaml` as installed); Core
  restarted while a client is connected, then a tool call succeeding without
  restarting the App; one `analyze_*` investigation run against the real
  installation. A criterion with neither is a finding, not a pass.
  **DoD:** `docs/research/<date>-v1-acceptance.md` with one row per criterion
  (evidence, pass/finding); every gap filed in `FINDINGS.md`; no row reads
  "assumed".

- [ ] **`P8-06` · Cut v1.0** — `blocked:P8-05`
  `addon/config.yaml` `version: "1.0.0"`; `docs/INSTALL.md` current; README's
  status line says v1.0. The owner tags `v1.0.0` and pushes; `release.yml`
  publishes both architectures.
  **DoD:** `TestAddonManifestImageIsPinnedToVersion` green at 1.0.0; no
  `blocks-active` finding open; `make check` green. The tag itself is the
  owner's action and is not part of this box.

## Decisions

- [x] **D-08-2 — The Core REST adapter is deleted, not kept as a fallback** —
  decided at the 2026-10-03 `plan` (F-38)
  `P1-03` built it per doc §23 step 4; every reader since went over the
  WebSocket, and `cmd/server` never constructs a `RESTClient`. **Why:** CLAUDE.md
  "no dead code, no speculative generality", and ADR-008's own reasoning —
  read-only-ness is enforced by what is linked in, so a linked-in HTTP client
  with its own allow-list is surface whether or not anything calls it. A future
  REST need re-adds an adapter with a caller in the same change.
  **Rejected:** *keep it as an HA-restart fallback* — the WebSocket manager
  already reconnects (`P1-02`), and a fallback with no caller is never
  exercised; *wire it somewhere so it has a caller* — inventing a consumer to
  justify code. Owner may overturn at review; the box is cheap to drop.

- [ ] **D-08-1 — How an MCP client reaches the App-hosted server** —
  `needs-decision`
  The owner's to decide, on `P8-01`'s evidence. Options known before
  observation, none chosen: *(a)* stdio through `docker exec -i` (from the SSH
  App or a host shell) — keeps phase 01's no-listener decision, but its
  prerequisites on the SSH side are unverified; *(b)* run the binary off-box
  against Core's WebSocket with a long-lived token — no App needed at all, but
  then the App packaging is not what v1 ships, and token handling (rule 4)
  moves to the client's machine; *(c)* reopen phase 01's transport decision
  (HTTP behind Ingress) — explicitly "a new decision plus a fresh security
  review, not a configuration change".
  **Evidence from `P8-01`:** (a) needs `docker` in the SSH App, which is absent
  as shipped — it takes lowering that App's protection mode, a weaker posture
  than this App keeps; (b) untouched by the observations; (c) unchanged. New
  option *(d)*: `stdin: true` in `config.yaml` with Supervisor holding stdin —
  keeps stdio and the no-listener decision, but nothing observed says a client
  can then reach it; untried (needs a new image).

## Phase Definition of Done

- Every doc §21 criterion is mapped to a named test or a live observation on the
  Pi, in a dated report.
- A real MCP client reaches the App on the Pi, and `docs/INSTALL.md` says how.
- `golangci-lint run` and `make check` are green; the allow-list has no entry
  without a caller, and no unwired adapter is linked in.
- `addon/config.yaml` is at 1.0.0, ready for the owner's tag. Still strictly
  read-only.
