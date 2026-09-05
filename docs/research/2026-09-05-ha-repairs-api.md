# HA `repairs/list_issues` — observed

**Task:** `devflow verify` · resolves **F-22**
**Observed:** 2026-09-05, against a live Home Assistant installation, in two
runs — an owner/admin token (13:03 UTC) and a **Users**-group, `is_admin: false`
token (13:04 UTC)
**HA version:** `2026.9.0` (reported by REST `GET /api/config`) — note this is
newer than the `2026.8.3` every other Phase 00 research doc ran against; the
2026-08-25 decision is "current release only", so this is simply what "current"
now means, not a discrepancy to reconcile.
**Vehicle:** `cmd/spike` (`probeRepairs`, added this session) over
`http://<host>:8123` with a long-lived access token, run once per principal.
`SPIKE_COST_LADDER` was left unset — the P0-09 cost ladder did not re-run.

> Response shape is field names and types only — the probe withholds every
> value from the installation by construction (`cmd/spike/shape.go`).

## Summary

**`repairs/list_issues` exists on `2026.9.0` and is reachable by both an admin
and a non-admin principal.** This is the opposite gating from the automation
surface (P0-05: every automation/trace command refused a non-admin token
outright). Both runs returned the identical status (`OK`, 588 bytes, 4-5 ms) and
the identical shape:

| principal | `is_admin` | status | bytes | time |
|---|---|---|---|---|
| owner token | `true` | OK | 588 | 5 ms |
| Users-group token | `false` | OK | 588 | 4 ms |

## What this means for `list_repairs`

**`list_repairs` is implementable read-only, at whatever principal the App
runs as — admin or not.** Unlike `get_automation`/`get_automation_traces`
(P3-07), this tool's DoD does not need to special-case a permission-refused
branch: F-4/P0-06 already established the App's Supervisor-proxied Core
principal is not less privileged than a non-admin Users-group user for every
other command this project allow-lists, and this command asks nothing extra of
it.

`gateway.go`'s allow-list may now add `repairs/list_issues` on the strength of
this observation — the same standard every other entry there was held to.

## Response shape

The response is `{"issues": [...]}`, an object wrapping the array — not a bare
array the way `config/entity_registry/list` or `get_states` are. One element's
fields, merged across the one issue present on this installation:

```
issues: array[1]
  breaks_in_ha_version: null
  created: string
  dismissed_version: string
  domain: string
  ignored: bool
  is_fixable: bool
  issue_domain: null
  issue_id: string
  learn_more_url: null
  severity: string
  translation_key: string
  translation_placeholders: object
    edit: string
    entity_id: string
    error: string
    name: string
```

`severity` and `issue_id` are exactly what the P3-06 DoD asks list_repairs to
report. `domain` names which integration raised it — useful for the same
per-integration correlation `analyze_integration_health` (doc §9) will want
later. `translation_placeholders` is free-form per-issue metadata (this sample
carries an `entity_id` and an error string inside it) — HA data, not schema,
and per CLAUDE.md rule 6 must be treated as opaque, never parsed for content or
branched on.

`breaks_in_ha_version`, `issue_domain` and `learn_more_url` are `null` on this
installation's one sample — present unconditionally as fields, so a mapper
should treat them as optional strings, not assume they are always absent.

## Not established

- **Only one issue was present on the sampled installation.** A field that is
  always non-null in this one element could still be nullable in general;
  `MapRepairs` should map every field permissively (the same convention as
  `MapArea`/`MapEntity`), not assume this one sample is exhaustive.
- **Whether `ignored`/`dismissed_version` behave as a filter dimension** the
  DoD might want (e.g. "exclude dismissed issues") — the field exists but its
  semantics were not exercised (nothing was dismissed during this probe).
- **Repairs volume at scale.** One issue on this installation says nothing
  about payload size with many; `list_repairs`' pagination (already required by
  every other `list_*` tool) covers this regardless.
- **Behavior on any release other than `2026.9.0`.**
