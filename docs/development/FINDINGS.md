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

**Triage:** `queue-next`

**Outcome:** Assigned to `P0-04`.

### F-2 · Config-entry read API behavior and admin requirement unverified · 2026-08-23

**Kind:** `unknown`

**What:** Q2 of the architecture doc §22. §6 marks config entries as REST
"partial", WebSocket "✓". Some HA config-entry WebSocket commands historically
require an admin-authenticated connection. Whether an App using
`SUPERVISOR_TOKEN` through the Core proxy satisfies that is unverified.

**Impact:** Unknown pending verification. If admin is required and unavailable,
`list_integrations` and `get_integration` (`P3-03`) must be re-scoped or dropped,
which changes the Phase 03 catalog and the `get_system_overview` counts.

**Triage:** `queue-next`

**Outcome:** Assigned to `P0-04`.

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

**Triage:** `queue-next`

**Outcome:** Assigned to `P0-05`.

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

**Triage:** `queue-next`

**Outcome:** Assigned to `P0-06`.

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

**Triage:** `queue-next`

**Outcome:** Assigned to `P0-07`.

### F-6 · Zigbee metric normalization across ZHA and Zigbee2MQTT unverified · 2026-08-23

**Kind:** `unknown`

**What:** Q9 of the architecture doc §22. The §13.2 investigation workflow
("Zigbee devices intermittently disappear") assumes LQI/RSSI and parent topology
can be read in a comparable way regardless of which Zigbee integration is in use.

**Impact:** Unknown pending verification. Decides whether Phase 05 needs a
per-integration diagnostic plugin architecture or a flat analyzer — a structural
difference, but one that costs nothing to defer until Phase 04 closes.

**Triage:** `defer`

**Outcome:** —

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
