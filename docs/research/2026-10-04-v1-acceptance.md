# v1 acceptance walk — doc §21 on the Pi (P8-05)

**Date:** 2026-10-04 · **Build:** App `0.9.2`, HAOS on Raspberry Pi (aarch64), HA Core `2026.9.4`, HTTP transport, privacy profile `mask`.
**Method:** tests by name (`make check`); live calls through a client on the App's HTTP port; the App log pasted by the owner; the HA UI read in Chrome. The owner restarted Core. No HA identifiers are recorded here.

Verdicts: **pass** = asserted by named tests and, where §21 is about behaviour on the device, observed live. **finding** = a gap, filed in `FINDINGS.md`. Nothing reads "assumed".

| # | §21 criterion | Evidence | Verdict |
|--:|---|---|---|
| 1 | Runs as an App on aarch64 Pi under protection mode | Live: App `Running` on the Pi, answering tool calls (log 2026-10-04 14:17–14:37Z). Protection mode: the UI on 2026.9.4 shows no switch and no "protection disabled" warning; AppArmor badge present — indirect only. | **finding F-48** |
| 2 | No `/config`, Docker socket, host network, `full_access`/privileged | `TestAddonManifestSecurityPosture`, `TestAppArmor_NetworkIsStreamOnly`, `TestAppArmor_AllowsReadingOnlyTheOptionsFile`, `TestAddonManifest_Port_ClosedByDefault`. Asserts the repo manifest; Supervisor's view of the installed App was not read (F-48). | pass (manifest) |
| 3 | System overview and filtered inventory | `TestSystemOverview_ReturnsCountsWithoutEntityList`, `TestListEntities_InvalidAvailability_Rejected`, area/device/integration list tests. Live: `get_system_overview` (counts), `list_integrations` (35 entries, one page). | pass |
| 4 | Bounded history, availability/outage metrics, no DB access | `TestGetEntityHistory_WindowExceedsMaximum_RefusedNamingMaximum`, `TestComputeAvailability_Fixture7d_MatchesDocExample`, `TestComputeAvailability_RecorderGap_NotAnOutage`. Live: `analyze_integration_health` over 7d cited `recorder_history` evidence (8 entities unavailable throughout). | pass |
| 5 | Repairs and supported automation execution evidence | `TestListRepairs_ReportsSeverityAndIssueID`, `TestGetAutomationTraces_ReturnsRunsNewestFirst`, `TestGetAutomationTraces_PermissionRefused_AttachesFallbackEvidence`. Live: `list_repairs` returned one issue with severity and id. Live `get_automation_traces` on an automation triggered two minutes earlier returned no items and no unsupported marker. | **finding F-47** |
| 6 | Every upstream command allow-listed; mutations denied before transmission | `TestUnknownCommandDenied`, `TestMutatingCommandDenied`, `TestSession_Write_DeniedCommand_NeverReachesSocket`, `TestGateway_UncalledCommands_Denied`, `TestGateway_AllowList_EveryEntryHasACaller`. | pass |
| 7 | Budgets stop oversized work with explicit errors | `TestQueryBudget_EachDimension_TripsIndependently`, `TestQueryBudget_Exceeded_ReportsUsageSoFarAndDoesNotApplyCharge`, `TestGetEntityHistory_BudgetExceeded_ReturnsBudgetError`. Not exercised live (would need an oversized request on the Pi). | pass (tests) |
| 8 | Redaction: tokens/secrets cannot be returned | `TestSupervisorTokenNeverReturned`, `TestManager_Errors_NeverCarryTheToken`, `TestManager_TokenNeverLogged`, `TestLogHandler_TokenScrubbedFromMessageAndAttrs`, `TestAuditNeverContainsSecrets`. Live: the pasted App log (Oct 3–4) contains no token. | pass |
| 9 | Audit records cost/metadata, no full private history | `TestEmit_RecordsCostFields`, `TestEmit_NoBodyPersistedByDefault`, `TestEmit_BodyPersistedOnlyWhenOptedIn`. Live: audit lines carry tool, parameters, duration, status, transport and no body — but `result_bytes:0` for `list_integrations`, `get_system_overview`, `list_repairs`, `get_system_health`, and `ha_requests:0` for tools that read Core; only `analyze_integration_health` shows real figures (10 requests, 2167 bytes). Reproduced after the restart. | **finding F-46** |
| 10 | HA restart ⇒ safe reconnect | `TestManager_HARestart_ReconnectsReauthenticatesAndServesNextRequest`, `TestConnectWithBackoff_*`, `TestBackoffDelay_GrowsAndIsBounded`. Live: owner restarted Core 14:35Z; log shows 14 `websocket connect failed, backing off` warnings (203 ms growing to ~5.5 s, jittered, no tight loop), then `websocket reconnected reconnects:1` at 14:36:43Z; `get_system_overview` succeeded at 14:37:00Z with the App never restarted. | pass |
| 11 | Unsupported APIs fail explicitly, no fabrication | `TestSystemHealth_SupervisorUnreachable_DegradesToUnsupported_OverviewStillSucceeds`, `TestListApps_SupervisorUnreachable_ReportsUnsupportedNotEmpty`, `TestGetAutomation_PermissionRefused_ReportsUnsupportedWithFallback`. Live: `get_system_health` returned every field empty/zero with `Unsupported:false`, no reason, `Partial:false`; the call took 1044 ms and audited `success`. | **finding F-45** |
| 12 | At least three end-to-end investigations ⇒ evidence-backed ranked hypotheses | `TestDocCriterion_ThreeInvestigationsProduceEvidenceBackedRankedHypotheses`, `TestInvestigation1…3_*`. Live: one `analyze_integration_health` run on an integration in `setup_retry` returned three hypotheses (medium, medium, low), each citing evidence ids, plus a next action. | pass |

**Totals:** 8 pass (two of them test-only, noted), 4 findings (F-45, F-46, F-47, F-48 — F-45 and F-47 share a symptom: an empty answer on the live build where the tests assert an explicit marker).

## What this says

- F-45 and F-47 are both "tests green, live build empty with no marker", but they are not one cause: F-45 is the Supervisor path (see `2026-10-04-supervisor-response-shape.md`); F-47 goes through the Core WebSocket and is still unexplained.
- `supervisor_resolution` evidence in the live `analyze_integration_health` run reported zero issues from the same Supervisor path as the empty `get_system_health`; treat it as unconfirmed until F-45 is settled.
- Criterion 1 cannot be closed by reading the UI on this HA version.

## Not done

Row 7 live (oversized request) and row 2 as installed. Both are covered by tests and F-48 respectively.

### 2026-10-04 · Why do most tools audit `result_bytes:0` and `ha_requests:0`? (F-46)

**Kind:** world-discoverable
**Method:** read `internal/mcp/middleware.go` (`invoke`), `internal/policy/budget.go`, and counted `Charge*` calls per `internal/mcp/*_tools.go`; grepped `internal/ha` for any `policy.` use. Compared with the Pi's 0.9.2 audit lines already in the acceptance walk. No new live call (the audit log is on the Pi; no token reaches the agent).
**Found:**
- The audit record takes `HARequests` and `ResultBytes` from `budget.Usage()` (`middleware.go:97,102`). Nothing measures the call itself; the figures are only what a handler explicitly charged.
- Tools that charge (`ChargeHARequests`/`ChargeBytes`): `find_*`, `get_entity_history`, `get_entity_statistics`, `analyze_entity_health`, `analyze_integration_health`, `analyze_automation_health`. Tools with zero charges: `app_tools`, `area_tools`, `automation_tools`, `device_tools`, `entity_tools`, `integration_tools`, `repair_tools`, `system_tools` — i.e. every `list_*`/`get_*` inventory tool and `get_system_overview`/`get_system_health`/`list_repairs`.
- `internal/ha` does not import `policy`, so upstream requests are never counted at the adapter either.
- This matches the Pi: the four zero-figure tools are all uncharging ones; the one tool with real figures (`analyze_integration_health`, 10 / 2167) is a charging one. Not a cache effect and not a structured-vs-text effect.
**Not established:** whether the uncharged tools stay within their budget in practice (they are bounded by page limits, not by charges); the audit line was not re-observed after reading the code.
**Means:** F-46 is a defect, not an unknown: audit cost is accurate only for tools that opt in. Fix is a design choice (count in one place — the HA gateway/adapter for requests, the middleware for result size — versus per-tool charges); that is a `plan` matter.

### 2026-10-04 · Why does `get_automation_traces` return no items for a just-triggered automation? (F-47)

**Kind:** world-discoverable
**Method:** live ha-inspector (Pi, 0.9.3, 15:25Z): `list_automations` (45 automations, many with `LastTriggered` minutes old), then `get_automation_traces` on three recently triggered ones (`active_inbed_bedroom_off`, `air_cond_toggle_bedroom_s_air_conditioner`, `ha_dashboard_1_stopcharge`); `get_automation` on the second; read `internal/ha/corereader.go:AutomationTraces` and `splitEntityID`; fixtures `test/fixtures/automation_trace_*.json`.
**Found:** all three traces calls: `Items:[]`, `Unsupported:false`, `Partial:false`. `get_automation` reports config `ID:"1659800012906"` for an entity whose object id is `air_cond_toggle_bedroom_s_air_conditioner`. The adapter sends `trace/list{domain:"automation", item_id:<object id>}` (`splitEntityID`); the captured fixtures show HA's trace `item_id` is the config id (`"1724500000000"`). HA answers `[]` for an `item_id` it has no store for, so the adapter reports a legitimate-looking empty list.
**Not established:** the wire reply itself — no tool here issues `trace/list` with the config id, so "traces exist under the config id" is inferred from fixtures and the 2026-08-23 probe (which paired by `item_id`), not observed on this Pi. Storage-off / trace-limit-0 is not excluded, but would not explain why the id mismatch exists. Automations without a config id (YAML) key by object id and may work.
**Means:** the premise "HA keeps no trace" is probably void; the likely defect is the wrong key. The tool's tests passed because the fake server accepts whatever `item_id` the test uses. Rule 7 is at stake: empty, unmarked, wrong. Fix needs the config id (from `automation/config`, already read by `get_automation`) and a test whose fake HA rejects the object id.
