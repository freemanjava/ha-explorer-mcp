# Findings

One inbox for everything that arrives mid-work and does not belong to the task in
hand: bugs, wrong assumptions, documents that disagree, scope wider than its box —
and your own ideas. One inbox, not two: a second list is the one that gets
forgotten.

**Log here instead of fixing opportunistically.** A finding fixed inside an
unrelated task produces a diff nobody can review and status nobody can trust.

**This file is a pipeline, not a graveyard.** Three things keep it moving:
`NEXT.md` shows open counts by state, so every session sees them; `queue-next`
items are drained at every `plan`, and always before the queue runs dry; and a
finding closes only when the tasks it became close — not when they are queued.

Triage states:

| state | meaning | next step |
|---|---|---|
| `blocks-active` | the current task cannot honestly close | stop, take it to the owner |
| `queue-next` | real work, not urgent enough to interrupt | turn into tasks via `devflow plan` |
| `defer` | real, but not now | stays here, revisited at planning |
| `wont-fix` | accepted as-is | record why, so it is not re-litigated |
| `resolved` | the work it became has landed | nothing — it leaves the open counts |

`resolved` is the only state `NEXT.md`'s open-findings block does not count. Move
a finding here when the tasks it became have closed, and fill its **Outcome**;
never before, or the pipeline quietly loses it.

An `unknown` is the one kind whose **impact cannot be assessed until it is
verified** — an unchecked assumption the work already rests on. It is resolved by
`devflow verify`, not by planning around it, and its impact line says so rather
than carrying a guess. Verification either establishes the fact, or voids the
question — in which case the tasks built on it are re-planned, not patched.

Closed findings stay, with their outcome — the reasoning is worth more than the
tidiness.

**The triage line is a grep contract.** `NEXT.md`'s open-findings counts are
derived by matching it, so write it exactly as
``**Triage:** `<state>` `` — at column 0, state backticked, nothing else on the
line. A finding whose triage line is phrased freely is a finding no session will
count, and an uncounted `queue-next` is exactly the captured work this file exists
to stop losing. Same contract as the checkboxes: `^- [x]` at column 0.

---

### F-1 · Exact read-only WebSocket registry commands are unverified · 2026-08-23

**Kind:** `unknown`

**What:** Q1 of `docs/HA_Inspector_MCP_Research_and_Architecture.md` §22. The doc
§11.1 lists `config/entity_registry/list`, `config/entity_registry/get`,
`config/device_registry/list`, `config_entries/get`, `repairs/list_issues` as an
illustrative allow-list, and §26 says explicitly that this is a design baseline,
not a guarantee that every listed command is stable. Nobody has run these against
the target HA release. Areas, floors and labels have no commands listed at all.

**Impact:** Unknown pending verification. It is the input to the Phase 01 gateway
allow-list, which is the project's primary security boundary — an allow-list
written from an unverified list is either too narrow (tools silently fail) or too
wide (a command nobody checked the semantics of).

**Triage:** `resolved`

**Outcome:** Closed 2026-08-23 by `P0-04`. All twelve candidate commands —
including the areas, floors and labels the doc listed none for — were run
against live HA `2026.8.3` and all exist. Evidence, with response schemas and
per-command admin requirements, in
[`../research/2026-08-23-ha-registry-apis.md`](../research/2026-08-23-ha-registry-apis.md).
Phase 01's allow-list is now derivable from observation rather than from §11.1's
illustrative list.

### F-2 · Config-entry read API behavior and admin requirement unverified · 2026-08-23

**Kind:** `unknown`

**What:** Q2 of the architecture doc §22. §6 marks config entries as REST
"partial", WebSocket "✓". Some HA config-entry WebSocket commands historically
require an admin-authenticated connection. Whether an App using
`SUPERVISOR_TOKEN` through the Core proxy satisfies that is unverified.

**Impact:** Unknown pending verification. If admin is required and unavailable,
`list_integrations` and `get_integration` (`P3-03`) must be re-scoped or dropped,
which changes the Phase 03 catalog and the `get_system_overview` counts.

**Triage:** `resolved`

**Outcome:** **Partially** closed by `P0-04`, deliberately not fully. What is now
observed: of the config-entry commands, only `config_entries/get_single` requires
admin — `config_entries/get` returns all entries with the full field set to a
non-admin user, so `list_integrations` / `get_integration` (`P3-03`) can be built
on the list and need not be re-scoped.

What remains open is this finding's actual question: both `P0-04` runs used a
*user's* long-lived token, so whether the App's `SUPERVISOR_TOKEN` principal
through the Core proxy counts as admin is still unverified. That residue belongs
to **F-4** / `P0-06` and must not be assumed settled by the evidence above.

**Closed 2026-08-23 by `P0-06`.** That residue is answered: Supervisor's Core
proxy forwards every App request with Home Assistant's own Supervisor token, and
Core 2026.8.3 creates that user in `GROUP_ID_ADMIN`
(`docs/research/2026-08-23-supervisor-permissions.md`, result 1). The App's Core
principal *is* admin, so `config_entries/get_single` is reachable and nothing in
`P3-03` needs re-scoping.

### F-3 · Automation config and trace retrieval may be unreachable from an App · 2026-08-23

**Kind:** `unknown`

**What:** Q3 of the architecture doc §22. §6 marks automation config and traces
as "internal/special" and "adapter/verify", with traces possibly living in
`.storage` — which v1 has decided not to mount (ADR-004). Two of the twenty v1
tools (`get_automation`, `get_automation_traces`) and the entire §13.1
investigation workflow rest on this.

**Impact:** Unknown pending verification. If traces are unreachable read-only, it
is not a small degradation: the flagship "why did this automation sometimes not
run?" workflow needs a different evidence source (logbook, `last_triggered`,
dependency history) and `P3-07` plus Phase 05 are re-planned.

**Triage:** `resolved`

**Outcome:** **Resolved** by `P0-05` —
`docs/research/2026-08-23-ha-automation-traces.md`. Automation config and full
traces *are* reachable read-only over the normal WebSocket API on 2026.8.3; no
`.storage` mount is needed and ADR-004 stands. They are, however, **admin-gated
without exception**, so whether the two tools exist for an App is now decided by
F-4 / `P0-06`. The fallback path, if not: `last_triggered` +
`logbook/get_events` + `context_id` correlation, both observed working for a
non-admin principal. Consequences filed as F-11 (degraded mode) and F-12
(trace privacy).

**Closed 2026-08-23 by `P0-06`.** The open dependency resolves in the tools'
favour: the App's Core principal is admin (see F-2's closing note), so
`get_automation` and `get_automation_traces` exist in v1. F-11's degraded branch
stays required for non-Supervisor deployments — it is now the exception, not the
expected path.

### F-4 · Supervisor endpoints available under minimal permissions unverified · 2026-08-23

**Kind:** `unknown`

**What:** Q4 of the architecture doc §22. §15.2 sets `hassio_api: false` as the
desired baseline, while §6 marks Core CPU/RAM, OS/Supervisor info and App state
as "v1 if permitted". These two statements have not been reconciled against a
running Supervisor.

**Impact:** Unknown pending verification. Determines whether `get_system_health`
and `list_apps` exist in v1 at all, and whether the App manifest must request a
permission the security posture currently forbids — a trade the owner decides,
not the implementation.

**Triage:** `resolved`

**Outcome:** **Resolved** by `P0-06` —
`docs/research/2026-08-23-supervisor-permissions.md` maps every endpoint the two
tools need to the role that grants it, derived from Supervisor `2026.08.0`'s
security middleware at a pinned tag. The reconciliation §15.2 and §6 lacked:
under `hassio_api: false` an App reaches `/info`, `/addons/self/*` and
`/supervisor/ping` — enough for a *partial* `get_system_health` and not enough
for `list_apps`, which has no enumeration path. One line
(`hassio_api: true`, role left at its `default`) reaches `/supervisor/info`,
whose embedded `addons[]` makes `list_apps` implementable without the `manager`
role the `/addons` collection would need. The trade is written up as a
`needs-decision` entry in phase 00 and deliberately not applied. Two results
outside the question came with it: the App's Core principal is admin (closing
F-2's and F-3's residues) and `hassio_api: false` is not an enforced ceiling
(**F-13**).

### F-5 · Recorder statistics API stability and cost unverified · 2026-08-23

**Kind:** `unknown`

**What:** Q5 of the architecture doc §22. §6 footnote (*) warns that recorder
statistics WebSocket APIs are documented mainly through developer change notices,
not a stable public reference, and §26 warns the query-budget numbers in §10 are
starting defaults, not measured Raspberry Pi limits.

**Impact:** Unknown pending verification. Picking the wrong source makes the
Phase 04 statistics tools either brittle across HA upgrades or slow enough to
strain the recorder on a Pi — and the budget defaults in `P2-01` are guesses
until something is measured.

**Triage:** `resolved`

**Outcome:** **Resolved** by `P0-07` — evidence in
[`docs/research/2026-08-23-ha-history-statistics.md`](../research/2026-08-23-ha-history-statistics.md).
All three recorder statistics commands exist and answer at Core 2026.8.3, and
statistics are 1–3 orders of magnitude cheaper than history for the same window.
Source order: statistics first, WS `history/history_during_period` for raw
states, REST `/api/history/period` as documented fallback. Multi-entity cost
remains unmeasured and is filed as **F-14**.

### F-6 · Zigbee metric normalization across ZHA and Zigbee2MQTT unverified · 2026-08-23

**Kind:** `unknown`

**What:** Q9 of the architecture doc §22. The §13.2 investigation workflow
("Zigbee devices intermittently disappear") assumes LQI/RSSI and parent topology
can be read in a comparable way regardless of which Zigbee integration is in use.

**Impact:** Unknown pending verification. Decides whether Phase 05 needs a
per-integration diagnostic plugin architecture or a flat analyzer — a structural
difference, but one that costs nothing to defer until Phase 04 closes.

**Triage:** `defer`

**Outcome:** Re-triaged at the second 2026-09-05 `plan` and **left deferred** a
fourth time on unchanged grounds: Phase 04 still has three open boxes
(`P4-04`, `P4-05`, `P4-06`), so the statistics layer this verification needs
still does not exist and Phase 05 still has no boxes to consume the answer.
Re-triaged at the first 2026-09-05 `plan` and **left deferred** a third
time: Phase 04 still has not closed — this `plan` is the one that queues its
five boxes — so the statistics layer the verification needs does not exist yet.
Re-triaged at the 2026-08-25 `plan` and **left deferred** on
unchanged grounds. Previously re-triaged 2026-08-24 on the same grounds: Phase 04 has not closed and Phase 05
still has no boxes. Originally re-triaged at the 2026-08-23 `plan`. It is
owned by Phase 05's `needs-decision — Zigbee/mesh metric normalization` entry,
Phase 05 has no boxes yet, and verifying it needs a running Zigbee installation
plus the statistics layer Phase 04 has not built. Verifying now would produce
evidence nothing can consume.

### F-7 · Go module path is an assumption, not a fact · 2026-08-23

**Kind:** `inconsistency`

**What:** `go.mod:1` declared `a placeholder module path from an early username`. This
was chosen during scaffolding because the repository had no remote; the owner's
actual repository URL was never stated. The project's own name is also
inconsistent: the directory is `ha-explorer-mcp`, the architecture doc calls the
product "HA Inspector MCP", and the Makefile builds `ha-inspector-mcp`.

**Impact:** Cheap now, annoying later — changing a module path rewrites every
import in the tree. Fixing it before `P0-01` pins dependencies costs one edit.

**Triage:** `resolved`

**Outcome:** Closed 2026-08-23. The owner supplied the repository
(`https://github.com/freemanjava/ha-explorer-mcp.git`), so the module path is now
`github.com/freemanjava/ha-explorer-mcp` — corrected before any internal import
existed, at a cost of one line. Recorded as a decision in
`phases/00-spike-foundations.md`.

The naming inconsistency it also raised is **left standing deliberately**: the
repository and module are `ha-explorer-mcp`, the product and binary are
`ha-inspector-mcp`. That is a normal split (repo name vs product name) and
renaming either now would churn the remote or the architecture doc for nothing.
Revisit only if it starts confusing someone.

### F-8 · Supervisor App builder's build context is unverified · 2026-08-23

**Kind:** `unknown`

**What:** `addon/Dockerfile` (from `P0-02`) compiles the Go binary in its build
stage, so it needs the whole repo — `go.mod`, `cmd/`, `internal/` — as build
context, not just the `addon/` folder. It was verified manually with
`docker buildx build -f addon/Dockerfile .` from the repo root. Whether Home
Assistant Supervisor's own App builder invokes `docker build` with the addon
folder as context (the conventional layout) or the repository root was never
checked against real Supervisor behavior.

**Impact:** Unknown pending verification. If Supervisor builds with
`addon/` as context, the image never builds on a real installation despite
passing this repo's local check — a deploy-time failure the current DoD
(image builds via `docker buildx` from repo root) does not catch.

**Triage:** `resolved`

**Outcome:** Verified by `P0-08`, confirmed true: Supervisor's own builder
(`supervisor/apps/build.py`, `home-assistant/supervisor@main`) mounts only the
App's own folder read-only as build context — not the repo root. `addon/Dockerfile`
does not build under a real Supervisor App build. Evidence:
`docs/research/2026-08-24-supervisor-addon-build-context.md`. The required
layout change is filed separately as **F-16**.

### F-9 · Probe report leaks installation ids through map-shaped object keys · 2026-08-23

**Kind:** `defect`
**What:** `cmd/spike/shape.go` renders every object as "keys are schema". When a
payload uses an object as a *map keyed by id*, those ids are values, not field
names, and reach the report. Observed in the `P0-04` admin run:
`config/device_registry/list` → `config_entries_subentries` emitted 27 literal
config-entry ids (ULIDs and 32-char hex) from the owner's installation.
`cmd/spike/shape_test.go`'s `TestShape_PayloadValues_NeverEmitted` did not catch
it because its fixture has no map-shaped object.
**Impact:** Defeats the one guarantee the probe output rests on — that a report
pasted into a chat and committed to `docs/research/` carries no data from the
installation. Ids are not secrets, but they identify a specific install and the
redactor is either trustworthy or it is not.
**Triage:** `resolved`
**Outcome:** Fixed inside `P0-04` — the defect is in the vehicle that task
built and would have re-leaked on the task's own second (non-admin) run, so it
is in scope rather than unrelated work. The task closed 2026-08-23, so the
finding closes with it; state corrected from `queue-next` at the 2026-08-23
`plan` (it had been left open while its work was already landed).

### F-10 · HA applies no per-user filtering to registry reads · 2026-08-23

**Kind:** `scope`
**What:** `P0-04`'s two runs (admin and `is_admin: false`) returned
**byte-identical** payload sizes for every shared command — entity registry
584 456 B / 952 entities, device registry 51 591 B / 69 devices, config entries
18 115 B / 35 entries. A non-admin sees the entire registry. `get_config`
likewise returns `latitude`, `longitude` and `location_name` to a non-admin.
Evidence: [`docs/research/2026-08-23-ha-registry-apis.md`](../research/2026-08-23-ha-registry-apis.md).
**Impact:** Phase 02's privacy classification cannot lean on Home Assistant
having already filtered by principal — it has not. Every privacy decision this
server makes is the *only* one made. Home coordinates in particular arrive from
`get_config` on a path that has nothing to do with the history tools where the
doc's privacy profiles currently focus.
**Triage:** `resolved`
**Outcome:** Planned 2026-08-23 into **`P2-02`**, which now carries it as a DoD
assertion (installation coordinates classify `PRIVATE` on the classification
table) plus a Phase 02 design note stating that HA filters nothing by principal.
No new box: this sharpens the classification task rather than adding a separate
reviewable change. **Closed 2026-08-25:** `P2-02` classified the coordinates
and `P2-03` applied it — `Redactor.Config` coarsens `latitude` / `longitude` to
one decimal and leaves `location_name` untouched, asserted by test.

### F-11 · Automation tools need a defined degraded mode, not just a happy path · 2026-08-23

**Kind:** `scope`

**What:** `P0-05` established that `automation/config`, `trace/list`,
`trace/get` and `trace/contexts` are all admin-gated on 2026.8.3
(`docs/research/2026-08-23-ha-automation-traces.md`). Whether the App's
`SUPERVISOR_TOKEN` principal clears that bar is F-4 / `P0-06`. Phase 03's
`get_automation` and Phase 05's §13.1 workflow are currently written assuming
the data is there.

**Impact:** Both branches must be built regardless of how `P0-06` lands: an App
that is admin today can be deployed against an installation where it is not.
`get_automation_traces` needs an `unsupported` path carrying the reason, and the
investigation workflow needs a documented degraded mode over `last_triggered` +
`logbook/get_events` + `context_id` correlation. Not planning it now means
discovering it in Phase 05, where it is a rewrite rather than a branch.

**Triage:** `resolved`

**Outcome:** Planned 2026-08-23 into **`P3-07`**, whose DoD now requires both
branches and a *permission*-flavored `unsupported` distinct from the
version-flavored one, with the degraded path exercised by a test rather than
only documented; Phase 05's candidate scope gained the matching §13.1 degraded
workflow bullet. Closes when `P3-07` closes — re-confirmed at the 2026-08-25
`plan`, which queued `P3-07` as row 9; no further box is needed.

**Closed 2026-09-05 by `P3-07`.** `get_automation`/`get_automation_traces`
build both branches: a permission refusal (`automation/config`/`trace/list`
answering `unauthorized`) degrades to `Unsupported` naming the fallback, and
`get_automation_traces` additionally fetches that fallback live —
`last_triggered` from `list_automations`' source and `logbook/get_events`,
correlated by `context_id` — into the response's `fallback_last_triggered`/
`fallback_events` fields, rather than only documenting that the fallback
works. A version-absent command (`unknown_command`) degrades separately,
naming the detected HA version, with no fallback fetch attempted. Exercised
by `internal/ha`'s `TestCoreReader_LogbookEvents_MapsEvents` and
`internal/mcp`'s `TestGetAutomationTraces_PermissionRefused_AttachesFallbackEvidence`/
`TestGetAutomationTraces_VersionAbsent_NoFallbackFetched`. One gap surfaced
during the close: the fallback's logbook events bypass `internal/redact`'s
privacy classification, unlike every other entity-derived field this project
returns — filed as **F-23** rather than folded into this task, since it is
absent from `P3-07`'s DoD and widens this box's scope.

### F-12 · Automation traces embed whole entity states and a user id · 2026-08-23

**Kind:** `inconsistency`

**What:** A `trace/get` response carries, under
`trace["trigger/N"][i].changed_variables`, the automation's own state and the
trigger's `from_state` / `to_state` — each a complete state object including
`attributes.friendly_name`, `attributes.icon` and `context.user_id`
(`docs/research/2026-08-23-ha-automation-traces.md`). The architecture doc treats
traces as diagnostic structure and classifies sensitivity at the entity level.

**Impact:** A trace is as sensitive as the entities it touched, plus it names a
user. Classifying traces as low-sensitivity diagnostic data would route personal
data past the privacy profile that exists to hold it — threat T2's neighbour, and
a straight contradiction of the doc §4 privacy model. `internal/policy` must
classify trace payloads by their embedded entities, and `internal/redact` must
walk into `changed_variables`, not only into top-level attributes.

**Triage:** `resolved`

**Outcome:** Planned 2026-08-23 into **`P2-02`** (payloads classify by the
entities they embed, asserted with a captured trace fixture) and **`P2-03`** (the
redaction walk descends into nested state objects, asserted at
`changed_variables` depth), plus a Phase 02 design note that sensitivity travels
with embedded payloads, not with the endpoint. **Closed 2026-08-25:** both
landed — `ClassifyPayload` walks embedded entities, and `P2-03`'s redaction walk
comes back clean at `changed_variables` depth against a planted-secret trace
fixture.

### F-13 · `hassio_api: false` does not prevent Supervisor access · 2026-08-23

**Kind:** `inconsistency`

**What:** Core registers the WebSocket command `supervisor/api`
(`homeassistant/components/hassio/websocket_api.py` at 2026.8.3,
`WS_TYPE_API = "supervisor/api"`), which accepts a free-form `endpoint` **and a
free-form `method`** and calls Supervisor with Core's own token. It is gated only
on `connection.user.is_admin`, and everything this App sends through the Core
proxy runs as Home Assistant's Supervisor user, which is in `GROUP_ID_ADMIN`
(`docs/research/2026-08-23-supervisor-permissions.md`). Home Assistant blocks the
HTTP-shaped equivalent (`BLACKLIST` in Supervisor's security middleware) but not
this one.

**Impact:** Two things, one design and one documentation. (1) `supervisor/api` is
a universal escape hatch *and* a write path in a single WebSocket command — the
exact shape CLAUDE.md rules 1 and 2 forbid. Phase 01's gateway must deny it by
name, with a test, and the deny must not depend on the App manifest, which does
not stop it. (2) Any statement that `hassio_api: false` *prevents* Supervisor
access is false; the manifest declares intent and bounds a future bug's blast
radius, it is not a wall. The architecture doc §15.2 and the `P0-02` manifest
comment both read as though it were.

**Triage:** `resolved`

**Outcome:** Planned 2026-08-23 into **`P1-07`** (`phases/01-ha-access-gateway.md`),
which adds a named deny set in front of the gateway's allow-list — `supervisor/api`
first — asserted denied independently of both the allow-list and the App
manifest, and corrects the §15.2 and `addon/config.yaml` wording in the same
change. The design record for why a deny-list exists at all beside a fail-closed
allow-list is in that phase's *Decisions*. **Closed 2026-08-24**: `P1-07` landed
the named deny set in `internal/ha/gateway.go`, and architecture doc §15.2 and
`addon/config.yaml` now say `hassio_api: false` is not the enforcement point.

### F-14 · Multi-entity history and statistics cost is unmeasured · 2026-08-23

**Kind:** `unknown`

**What:** `P0-07` measured every history and statistics query against exactly
one entity ([`docs/research/2026-08-23-ha-history-statistics.md`](../research/2026-08-23-ha-history-statistics.md),
*Not established*). The queries that will actually strain a Pi are the ones a
fleet-wide detector issues — `history/history_during_period` or
`recorder/statistics_during_period` over dozens of entity ids at once — and
nothing observed says whether that cost is linear in the number of entities, or
whether one batched call beats N single-entity ones.

**Impact:** Unknown pending verification. It decides the shape of the Phase 04
detection queries (batched vs per-entity) and the Phase 02 budget limits, which
CLAUDE.md requires to be measured against a real recorder rather than guessed.
The source *order* recommended by `P0-07` does not depend on it — statistics win
by 1–3 orders of magnitude per entity — so this blocks tuning, not design.

**Triage:** `resolved`

**Outcome:** Closed 2026-08-24 by **`P0-09`** —
[`docs/research/2026-08-24-ha-multi-entity-query-cost.md`](../research/2026-08-24-ha-multi-entity-query-cost.md).
Neither command is linear in entity count: cost tracks recorded rows, so the
first 50 ids of a mixed 200-id set cost more than the remaining 150. One
batched call beat N single-entity ones at every rung of both commands (1.4×–50×
on time, identical bytes for history), the saving being round trips rather than
payload. Starting values named for `MaxBytes`, `MaxHistoryPoints`, `MaxEntities`
and `Deadline` per budget class, plus an entity-day pre-flight estimate, all
attributed to the measurement — `P2-01` can now cite evidence. Spawned **F-17**
(batched statistics bytes exceed the summed singles, unexplained, `defer`).

### F-15 · F-9's leak recurred: entity-id-keyed maps were not covered · 2026-08-23

**Kind:** `defect`

**What:** F-9 taught `cmd/spike/shape.go` to withhold object keys when they are
ids, but `isIDKey` recognised only a ULID and a 32-char hex digest. HA keys
`history/history_during_period` and `recorder/statistics_during_period` answers
by **entity id**, which matched neither, so the first `P0-07` probe run printed
`sensor.<…>` into `report.md` — the same guarantee F-9 exists to protect,
broken by a different key format four tasks later.

**Impact:** The redaction guarantee is what makes a probe report safe to paste
into a chat and commit to `docs/research/`. A partial one is worse than none,
because it is trusted. The leaked report was local and gitignored; no id reached
the repository, and the research file written from it carries none.

**Triage:** `resolved`

**Outcome:** Fixed inside `P0-07`, the task whose probe re-leaked it: `isIDKey`
now also matches `domain.object_id`, `TestShape_EntityIDKeyedMap_KeysWithheld`
asserts it, and the probe additionally routes every rendered shape through a
`redactor` seeded with the ids it requested — so a future key format that slips
past the pattern is still caught for the ids the probe itself chose.

### F-16 · `addon/Dockerfile` cannot build under Supervisor's real App builder · 2026-08-24

**Kind:** `defect`

**What:** Confirmed by `P0-08`
(`docs/research/2026-08-24-supervisor-addon-build-context.md`): Supervisor's
builder passes only the App's own folder (`addon/`) as Docker build context,
read-only. `addon/Dockerfile:9-11` copies `go.mod`, `go.sum`, `cmd/`,
`internal/` from the repository root, one level above that context — none of
those paths exist inside a real Supervisor build. The build stage fails at the
first `COPY` on any installation that builds the App locally through
Supervisor, even though `docker buildx build -f addon/Dockerfile .` run by hand
from the repo root (the `P0-02` DoD's check) passes.

**Impact:** The App cannot currently be installed as a local build on real
Home Assistant Supervisor — only the hand-run repo-root build works. This is a
deploy-blocking defect, not a style issue: nothing downstream of packaging
(all of Phase 03+) can be run on real hardware until it is fixed.

**Triage:** `resolved`

**Outcome:** Planned 2026-08-24 into **`P0-11`**. Of the three candidate fixes
named in `docs/research/2026-08-24-supervisor-addon-build-context.md`, the owner
chose the third — publish a prebuilt multi-arch image and reference it via
`config.yaml`'s `image:`, deleting the local-build path entirely; rationale and
rejected alternatives are recorded as the **App distribution** decision in
`phases/00-spike-foundations.md`. The private-registry half of that choice
spawned **F-19**, verified first by `P0-10`. **Closed 2026-08-25**: `P0-11` closed, live-verified on the owner's real Home
Assistant OS / Raspberry Pi — the App pulls from GHCR and starts. Three defects
surfaced only under the real Supervisor and are recorded in that task's journal
entry (missing `repository.yaml`, non-executable `COPY`'d binary, AppArmor
granting `mr` where `exec` needs `ix`).

### F-17 · A batched statistics answer is ~30% larger than the same ids fetched singly · 2026-08-24

**Kind:** `unknown`

**What:** Measured by `P0-09`
([`docs/research/2026-08-24-ha-multi-entity-query-cost.md`](../research/2026-08-24-ha-multi-entity-query-cost.md)):
`recorder/statistics_during_period` over 200 statistic ids returns 131 530 B
(24h) and 939 984 B (7d), while the *same* 200 ids fetched one per call sum to
101 793 B and 721 895 B. Point counts are identical in both directions (1 196
and 8 684), so the difference is ~25 B of extra serialization per point, not
extra data, and the ratio is stable across both windows. History shows no such
effect — batched and summed bytes agree within 200 B at every rung.

**Impact:** Unknown pending verification, and small either way: statistics
remain 8× smaller and 26× faster than history at fleet width, so the source
order and the batching recommendation do not depend on it. It costs ~30%
accuracy in the statistics half of `P2-01`'s pre-flight byte estimate. The
estimate uses the larger batched figure, so the error is conservative.

**Triage:** `defer`

**Outcome:** Open. Re-triaged at the second 2026-09-05 `plan` and **left
deferred** again, with the trigger unchanged and now one task away: `P4-04` is
the first consumer of statistics and is next in the queue, so this is revisited
when `P4-04` closes — if `P2-01`'s statistics byte estimate binds there, resolve
it; if it does not bind, defer again with that as evidence rather than as
expectation. Re-triaged at the first 2026-09-05 `plan` and **left deferred**
again: `P2-01`'s statistics estimate has still never bound in practice — nothing
between `P3-02` and `P3-06` reads statistics at all — so the trigger the
2026-08-25 deferral named has not fired. Phase 04 is the first consumer, and
`P4-04` is where it can first bind; revisit there, not before.
Re-triaged at the 2026-08-25 `plan` and **left deferred**:
`P2-01` landed with the conservative batched figure and a comment saying so,
exactly as the deferral anticipated, so nothing has changed to make resolving it
worthwhile. Re-triaged 2026-08-24 on the then-current grounds that `P2-01` was
not yet written, so nothing consumed the answer, and the error runs in the
conservative direction. It becomes worth resolving only if `P2-01`'s
statistics estimate turns out to bind in practice. Cheapest probe: request one
id alone and inside a batch and compare the rendered *field sets* per point via
`cmd/spike/shape.go` — no values from the installation need be printed.

### F-18 · The gateway check's single-chokepoint property is documented, not enforced · 2026-08-24

**Kind:** `inconsistency`

**What:** `P1-02` put `checkCommand` at the top of `Manager.Call`
([`internal/ha/manager.go:175`](../../internal/ha/manager.go)), ahead of session
acquisition and encoding, and the comment there asserts "Call is the only route
to callOn, so this is the single place a command can be sent from". That is true
today — `callOn` is unexported and has exactly one caller — but nothing fails if
it stops being true. A second caller added inside `internal/ha` would reach
`session.write` without passing the allow-list, and no test would notice: every
denial assertion goes through `Call`.

**Impact:** No defect today; the read-only guarantee holds as shipped. The cost
is that the project's central security property rests on a comment rather than
on something that breaks when violated — the same shape as the allow-list's
pattern-vs-exact-match hazard the phase notes call out. It is cheapest to close
now, while `P1-07` is already editing the denial path, and it would be a real
defect the first time this package grows a second send site.

**Triage:** `resolved`

**Outcome:** Pinned into `P1-07`'s scope and DoD 2026-08-24. **Closed 2026-08-24**:
the check moved into `session.write`, the function that actually calls
`conn.Write` — not `encodeCommand`, which only builds bytes and would still
have let a caller skip the check by writing pre-built frame bytes directly.
`Call` keeps its early check so denial stays independent of connectivity.
`TestSession_Write_DeniedCommand_NeverReachesSocket` asserts the property by
constructing a `session` directly and calling `write`, bypassing `Call`.

---

### F-19 · Supervisor's ability to pull an App image from a private registry is unverified · 2026-08-24

**Kind:** `unknown`

**What:** The 2026-08-24 App-distribution decision
(`phases/00-spike-foundations.md`) chose a **private** GHCR package referenced
from `addon/config.yaml`'s `image:`. That rests on an assumption nobody has
checked: that Supervisor can authenticate to a private registry when pulling an
App image, and that an owner has a supported way to supply the credential on
Home Assistant OS. Supervisor appears to carry a registries concept, but it was
inferred from memory during planning, not read from source — the same mistake
`P0-08` caught in `addon/Dockerfile`.

**Impact:** Unknown pending verification, and premise-overturning if negative.
If Supervisor cannot pull from an authenticated registry, the private half of
the decision is unimplementable and returns to the owner as a public-vs-private
choice; the published-image half stands either way. If it can, the credential's
shape and its behavior across restarts decide what `P0-11` must document as the
install procedure — a private App that installs with an indistinguishable
"image not found" is a support burden, not a deploy path.

**Triage:** `resolved`

**Outcome:** Planned 2026-08-24 into **`P0-10`**, queued ahead of `P0-11`, which
is `blocked:P0-10` for exactly this reason. **Answered** by `P0-10`
([`docs/research/2026-08-24-supervisor-private-registry-pull.md`](../research/2026-08-24-supervisor-private-registry-pull.md)):
Supervisor does support pulling an App's `image:` from an authenticated
registry, via `DockerConfig`'s `/data/docker.json` (hostname → username/password),
managed at Settings → Add-ons → Registries (`/docker/registries` API,
`ha-config-apps-registries.ts`), independent of the App's repository entry. The
private half of the App-distribution decision stands; `P0-11` is unblocked.
One caveat for `P0-11`'s install documentation: only a *present-but-wrong*
credential surfaces as a clear, typed `DockerRegistryAuthError`; a *missing*
one degrades to a generic `DockerError` carrying the raw registry response
text, indistinguishable at the type level from image-not-found.

### F-20 · The invocation rate limit's numbers are the only unmeasured constants in `P2-01` · 2026-08-25

**Kind:** `unknown`

**What:** `P2-01`'s budget dimensions all carry a measured provenance
([`docs/research/2026-08-24-ha-multi-entity-query-cost.md`](../research/2026-08-24-ha-multi-entity-query-cost.md)),
but the token bucket that answers the DoD's request-storm requirement does not:
`invocationBurst = 10` and `invocationInterval = 500ms`
([`internal/policy/ratelimit.go`](../../internal/policy/ratelimit.go)) are
*derived* from that run's single-call timings (worst in-budget call 339 ms cold
⇒ ~3 back-to-back calls saturate one recorder read stream), not observed. The
2026-08-24 run measured the cost of one call, never a stream of them, so what a
sustained 2/s arrival rate does to a Pi's recorder is unmeasured — the exact
shape of F-14, which the phase's own design note calls "a limit nobody measured
is a guess wearing a constant's name".

**Impact:** Unknown pending verification, and currently inert: nothing calls
`Allow()` yet — the limiter is wired at the MCP invocation boundary, which does
not exist until Phase 03. Both error directions are plausible: too permissive
lets a storm through the mitigation for threat T1, too strict refuses a normal
interactive investigation's opening fan-out. Neither can be judged without a
sustained-arrival measurement against a real recorder.

**Triage:** `resolved`

**Outcome:** Promoted 2026-09-05 at the `plan` the 2026-08-25 re-triage named
as the exact revisit point — `P3-01` closed, so the limiter now runs at a real
invocation boundary and a storm has something to hit. Planned into **`P2-06`**
(`phases/02-policy-privacy-audit.md`), flagged `needs-verify`. **Closed
2026-09-05 by `P2-06`.** `cmd/spike`'s new `probeArrivalRate` streamed a
budget-compliant `history/history_during_period` call (10 ids, 24h — the
2026-08-24 run's largest rung inside both the byte and point caps) at 1/2/4
calls/s, pipelined so it measures contention, not round-trip time
([`2026-09-05-ha-invocation-rate-limit.md`](../research/2026-09-05-ha-invocation-rate-limit.md)):
no measurable recorder-latency or Core-CPU degradation at any tested rate,
including 4/s — double the current sustained limit. `invocationBurst`/
`invocationInterval` are confirmed unchanged, their comment now citing this
note. A first probe cut mis-sized "max-page" as the widest cost-ladder rung
(200 ids/7d, 7.63 MB) — 15× over the normal-read byte cap, a call `Preflight`
would refuse before any shipped tool could issue it — and was caught and fixed
before that run's severe degradation (real, but the shape of a `Preflight`
bypass's cost, not of the arrival limit) was mistaken for the answer.

### F-21 · A client that dies mid-request makes the App exit non-zero · 2026-09-05

**Kind:** `defect`

**What:** Observed while running `P3-01`'s server over real stdio. When stdin
closes *while a request is in flight*, `Server.Run` returns the SDK's
`jsonrpc2.ErrServerClosing` ("server is closing: EOF"), `cmd/server`'s
`run()` does not recognize it, and the process exits 1. A clean disconnect
between requests exits 0. The sentinel cannot be matched: it lives in
`github.com/modelcontextprotocol/go-sdk/internal/jsonrpc2`, so `errors.Is`
against it is not available to code outside the SDK — `main.go`'s `io.EOF`
check does not catch it, and only its message or its JSON-RPC code (-32004)
identifies it.

**Impact:** Cosmetic today, operational under the Supervisor: an App that exits
non-zero is logged as a crash and, with a restart policy, restarted — so a
client that quits at the wrong moment looks like a failing App. It never
corrupts anything and never affects a live session.

**Triage:** `resolved`

**Outcome:** Planned 2026-09-05 into **`P3-08`**
(`phases/03-inventory-tools.md`), which carries the fix this finding proposed —
treat any session-end error after a session was established as a normal shutdown
at INFO, rather than match an unreachable sentinel by message or JSON-RPC code,
both of which an SDK release can change. It sits in Phase 03 because the code is
`P3-01`'s, and is flagged `live-verify`: the defect was found by running the
server, so the fix is not done until a client killed mid-request is observed
leaving exit 0. **Closed 2026-09-05 by `P3-08`.** `internal/mcp.Run` now calls
`srv.Connect` directly instead of delegating to `(*sdkmcp.Server).Run`, and
treats any error after a session was established as a normal shutdown; a real
client killed mid-request was observed leaving the process at exit 0, logging
`"stopped" "reason":"session ended"` at INFO.

### F-22 · `repairs/list_issues` has no P0 probe evidence at all · 2026-09-05

**Kind:** `unknown`

**What:** `P3-06`'s `list_repairs` DoD assumes a native HA Repairs/issues
command exists, is reachable at whatever principal the App runs as, and
returns severity/issue id fields — but unlike every other command this
project allow-lists, no Phase 00 probe ever exercised it. `gateway.go`'s own
rule (its doc comment, and the P1-02 "Depends On" it cites) is that a command
is allow-listed only once "observed answering successfully against live HA" —
`docs/research/2026-08-23-ha-registry-apis.md` and
`2026-08-23-ha-automation-traces.md` cover every other command this phase
needed, including the two (`config/floor_registry/list`,
`config/label_registry/list`) whose *element schema*, not their reachability,
was left unverified. `repairs/list_issues` (the name the architecture doc §9
and §23 use) has neither: not in the allow-list, not in `cmd/spike`, not in
any research doc. Whether it exists under that name on `2026.8.3`, whether it
is admin-gated the way the automation surface turned out to be (P0-05), and
what its element schema looks like are all unestablished.

**Impact:** Unknown pending verification. `list_repairs` cannot honestly be
implemented without either (a) evidence the command is reachable at the
App's principal, sanctioning an allow-list addition the way every other entry
in `gateway.go` is sanctioned, or (b) a decision to ship it permanently
`unsupported` — and that decision is itself premature while the command's
existence is unconfirmed one way or the other.

**Triage:** `resolved`

**Outcome:** **Closed 2026-09-05.** `cmd/spike`'s `probeRepairs`, run by the
owner once with an admin token and once with a non-admin Users-group token,
confirms `repairs/list_issues` exists on `2026.9.0`, answers identically for
both principals (`OK`, 588 bytes, 4-5 ms — unlike the automation surface,
this command is **not** admin-gated), and returns `{issues: [...]}` with
`severity`/`issue_id` among its fields, exactly what the DoD needs. Evidence:
[`docs/research/2026-09-05-ha-repairs-api.md`](../research/2026-09-05-ha-repairs-api.md).
`P3-06` is unblocked: `gateway.go` may now allow-list `repairs/list_issues` on
the same evidentiary footing as every other entry, and `list_repairs` can be
implemented on the next `devflow next` cycle.

### F-23 · `get_automation_traces`' logbook fallback bypasses privacy classification · 2026-09-05

**Kind:** `inconsistency`

**What:** `P3-07`'s `get_automation_traces` degrades, on a permission
refusal, to fallback evidence fetched live via `logbook/get_events`
(`internal/mcp/automation_tools.go`'s `attachAutomationFallback`, backed by
`internal/ha.CoreReader.LogbookEvents`/`MapLogbookEvents`). Unlike every other
tool that surfaces entity-derived text — `list_entities`/`get_entity`'s
`maskEntityState` runs the current state through `internal/redact.Redactor`
keyed by the entity's classification — a `model.LogbookEvent`'s `Message`,
`Name` and `EntityID` cross the response boundary unredacted. A logbook
message routinely embeds the human-readable state transition of whichever
entity triggered it (e.g. "Owner phone changed to home"), so a PRIVATE-
classified entity (`device_tracker`, `person`, …) referenced by an
automation's trigger leaks through the fallback exactly where the profile is
supposed to mask it.

**Impact:** Under the default (mask) or deny privacy profile, a non-admin
deployment's `get_automation_traces` can return PRIVATE entity data
`list_entities`/`get_entity` would have withheld — a real gap in the Phase 02
privacy guarantee, though narrow: it only reaches an agent (a) on an
installation where the App's principal is *not* admin (today's default
deployment is admin, per F-2/F-4's resolution) and (b) for an automation
whose recent logbook entries actually reference a PRIVATE entity.

**Triage:** `queue-next`

**Outcome:** Queued 2026-09-05 as `P3-09` (phase 03 — Phase 03 simply has open
boxes again), with the masking design settled in the same `plan` as that
phase's `P3-09` decision record: the event is masked whole, keyed by its own
entity's classification, rather than searched for substrings. Ordered after
`P4-04` and before `P4-05`: it is a privacy-guarantee gap, so it outranks a new
feature, but it reaches an agent only on a non-admin deployment (not today's
default, per F-2/F-4), so it does not displace the pointer mid-phase. Closes
when `P3-09` closes.

### F-24 · Doc §12.1's example statistics are not self-consistent · 2026-09-05

**Kind:** `inconsistency`

**What:** `docs/HA_Inspector_MCP_Research_and_Architecture.md` §12.1 prints
`"availability_ratio": 0.982` alongside `"total_unavailable": "3h12m"` over
`"period": "7d"`. 11520s of 604800s is 1.905% down, so the ratio those two
numbers imply is 0.98095, not 0.982. Found while building `P4-02`'s fixture
(`test/fixtures/entity_history_7d.json`) to reproduce the example exactly:
every other field matches, the ratio cannot.

Its cadence half does not hold either (found while building `P4-03`,
2026-09-05): `"median_update_interval_s": 31` cannot coexist with
`"state_changes": 412` over `"period": "7d"` — 412 changes a median 31s apart
span about 3.5 hours, not a week; a week at that cadence is ~19500 changes.
The two halves of the example describe different entities. A single doc fix
covers both.

**Impact:** None on the running server — `internal/analysis` computes the
ratio and `P4-02`'s test asserts the computed value, with the discrepancy
recorded in the phase file's decision record. The cost is future confusion:
§12.1 is the shape `P4-04` (`get_entity_statistics`) implements against, and
a reader treating the example as a specification would chase a phantom
rounding bug. A one-line doc fix.

**Triage:** `queue-next`

**Outcome:** Re-triaged 2026-09-05 from `defer` to `queue-next` and queued as
`P4-06`: the deferral's own trigger — "revisit when it traps whoever implements
`P4-04`" — is now, since `P4-04` is the next task and §12.1 is the shape it
implements against. Ordered *first*, ahead of `P4-04`, so the trap is gone
before that session reads the section. The fix direction is settled in phase
04's `P4-06` decision record (correct §12.1 to the fixture, not the fixture to
§12.1). Closes when `P4-06` closes.
