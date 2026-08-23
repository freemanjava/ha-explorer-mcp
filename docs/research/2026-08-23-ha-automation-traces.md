# HA automation config & trace retrieval — observed

**Task:** `P0-05` · resolves **F-3**
**Observed:** 2026-08-23, against a live Home Assistant OS installation, in two
runs — an owner/admin token (16:53 UTC) and a **Users**-group, `is_admin: false`
token (16:54 UTC)
**HA version:** `2026.8.3` (reported by both REST `GET /api/config` and WS `get_config`)
**Vehicle:** `cmd/spike` (throwaway, Phase 00) over `http://<host>:8123` with a
long-lived access token, run once per principal.

> **Scope limit — read this before deriving App permissions from it.** The runs
> used a *user's* long-lived token, not `SUPERVISOR_TOKEN` through the
> Supervisor proxy. Command names, response schemas, sizes and the admin/
> non-admin split below are established facts. **Whether the App's principal is
> admin is not established here** — that is `P0-06` / F-4, and this file's
> central conclusion is conditional on it.
>
> Response shapes are field names and types only — the probe withholds every
> value from the installation by construction (`cmd/spike/shape.go`).

## Summary

Every automation command the doc §9 needs **exists and works on 2026.8.3** —
including full trace retrieval, which §6 suspected might require `.storage`
access. It does not: `trace/get` returns the complete stored trace over the
normal WebSocket API.

**But the entire automation surface is admin-gated.** Not one command in it
answered a non-admin principal. This is a sharper line than the registries
showed in `P0-04`, where only `config_entries/get_single` required admin.

| command | admin | non-admin | size / time (admin) | notes |
|---|---|---|---|---|
| `get_states` | ✅ | ✅ | 212 884 B, 31 ms | 47 automations; **the fallback source** |
| `automation/config` | ✅ | ⛔ `unauthorized` | 949 B, 6 ms | by `entity_id`, over WS |
| `trace/list` (unfiltered) | ✅ | ⛔ `unauthorized` | 48 644 B, 9 ms | 145 traces, whole domain, one call |
| `trace/list` (one automation) | ✅ | ⛔ `unauthorized` | 1679 B, 6 ms | 5 traces for the target |
| `trace/get` | ✅ | ⛔ (skipped — no run_id obtainable) | 3528 B, 6 ms | the full run trace |
| `trace/contexts` | ✅ | ⛔ `unauthorized` | 616 B, 6 ms | context_id → run_id, ULID-keyed map |
| `logbook/get_events` | ✅ | ✅ | 3730 B, 68 ms | 12 events / 24 h / 1 automation |
| REST `/api/config/automation/config/<id>` | ✅ HTTP 200 | ⛔ **HTTP 401** | 938 B, 63 ms | config-panel route |

## What this means for the two tools

**`get_automation` is implementable read-only, for an admin principal, from two
independent sources.** `automation/config` (WS, by `entity_id`) returns
`{config: {...}}`; REST `/api/config/automation/config/<id>` returns the same
object unwrapped, addressed by `attributes.id`. The WS form is preferable: it
needs no second transport, is 10× faster here (6 ms vs 63 ms), and is keyed by
the id the rest of the system already uses.

**`get_automation_traces` is implementable read-only, for an admin principal.**
`trace/list` gives the index; `trace/get` gives one run in full. No `/config`
mount, no Recorder SQL, no `.storage` — ADR-004 and ADR-005 stand unchallenged.
The §13.1 investigation workflow is buildable as designed **iff the App is
admin**.

**If the App's principal is not admin, both tools are unimplementable and the
workflow needs re-planning** onto the fallbacks below. That branch is decided by
`P0-06`, not by this file.

### The fallback, if the App is not admin

Two sources survive a non-admin principal and both were observed working:

- **`get_states` → `attributes.last_triggered`** — present on 46 of 47
  automations (the 47th also lacks `mode` and `current`, so it is an automation
  that has never run). Also carries `mode` and `current` (how many runs are in
  flight). This answers "did it fire, and when" but nothing about *why not*.
- **`logbook/get_events`** — answered the non-admin run identically to the admin
  one (3730 B, 12 events over 24 h). Each event carries `context_id`, which is
  the same id `trace/contexts` indexes; correlation by context survives even
  though the trace behind it does not.

Neither yields the step-level condition results that make trace analysis
valuable. A degraded `get_automation_traces` must return `unsupported` with the
reason, never a thinner answer dressed as the real one (CLAUDE.md rule 7).

## Details worth carrying into Phase 01

- **`trace/list` unfiltered is a cheap domain-wide index.** 145 traces across all
  47 automations in one 9 ms call, 48 KB. The architecture doc assumes per-
  automation trace queries; a single unfiltered call is the better primitive for
  "which automations are misbehaving" and should be preferred over N calls.
  Every summary field the detection layer needs is in the index — `state`,
  `script_execution`, `last_step`, `trigger`, `timestamp.{start,finish}` — so
  `trace/get` is only needed once a specific run is under investigation.
- **`trace/get` embeds whole state objects.** The `trace` map's
  `trigger/N.changed_variables` carries `this` (the automation's own state) and
  `trigger.from_state` / `to_state` — each a full state object with
  `attributes.friendly_name`, `icon`, `context.user_id`. A trace is therefore
  *not* a low-sensitivity payload: it can contain arbitrary entity data and a
  user id. `internal/policy` must classify traces, and `internal/redact` must
  strip them, on the same footing as entity attributes. Filed as **F-12**.
- **`trace` is keyed by execution path**, not by an array: keys observed were
  `trigger/1`, `condition/0`, `condition/0/entity_id/0`. Each value is an array
  of step records (a step can execute more than once). Mapping this to
  `internal/model` needs a path-keyed structure, not a list.
- **`trace/contexts` is a map keyed by `context_id`** (ULID). `shape.go`'s
  id-key withholding (F-9) correctly suppressed the keys — confirmation that
  fix works against a payload it was not written for.
- **`blueprint_inputs: null`** is present on a non-blueprint automation, so the
  field exists unconditionally and marks blueprint-derived automations.
- The modern schema is `triggers` / `conditions` / `actions` (plural, with
  `trigger:` and `action:` keys inside), not the legacy
  `trigger` / `condition` / `action`. `2026.8.3` returns the new form. Which
  older releases return the legacy form is **not established** — it depends on
  the version policy still open as a `needs-decision` in Phase 00.

## Not established

- **Anything about the App's principal.** Every ⛔ above describes a *user* in
  the Users group. `SUPERVISOR_TOKEN` may or may not behave as admin; `P0-06`
  decides, and it now decides whether two tools and one workflow exist.
- **Non-admin `trace/get`.** Skipped rather than refused, because `trace/list`
  was refused first and no `run_id` could be obtained. Its refusal is a safe
  inference from the other four trace commands, but it is an inference, not an
  observation.
- **Trace retention.** 145 traces for 47 automations suggests a per-automation
  cap (HA's default is 5 stored traces; the target automation had exactly 5),
  but no probe varied it. How far back traces reach is unmeasured, and it bounds
  what the §13.1 workflow can conclude.
- **Behaviour on any release other than 2026.8.3.**

## Probe correction

The first admin run (16:50 UTC) reported `trace/get` as `not_found`. That was a
defect in the probe, not in HA: `run_id` was taken from the *unfiltered*
`trace/list`, so the target automation's `item_id` was paired with another
automation's run. Fixed in `cmd/spike/automation.go` (`runIDFor`), with a
regression test, and both runs above use the corrected probe. Recorded here
because the false negative was one step from being written up as "traces
unreadable" — which would have re-planned Phase 05 for no reason.
