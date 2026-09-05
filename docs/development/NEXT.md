# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P4-03` — update-cadence and staleness analysis
· [`phases/04-history-statistics.md`](phases/04-history-statistics.md) · model: **claude-opus-5** · flags: 🧠

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P4-03` | update-cadence and staleness analysis | 04 | claude-opus-5 | 🧠 |
| 2 | `P4-04` | `get_entity_statistics` | 04 | claude-sonnet-5 | |
| 3 | `P4-05` | `find_unavailable_entities` / `find_stale_entities` | 04 | claude-sonnet-5 | |

**Order rationale (2026-09-05 `plan`, `P3-07` since closed and dropped).**
Phase 03 finished first: `P3-08` — the F-21 fix — closed it so the phase does
not linger with a known non-zero exit in it. `P2-06` sat between the phases on
purpose: it was the F-20 measurement, whose revisit point the 2026-08-25 `plan`
fixed as "the `plan` after `P3-01` closes", and Phase 04's tools are the
heaviest recorder callers this server will ever make — writing them against an
arrival limit nobody measured was exactly the guess the finding named. It
closed 2026-09-05 confirming both constants unchanged
([`2026-09-05-ha-invocation-rate-limit.md`](../research/2026-09-05-ha-invocation-rate-limit.md)).
Phase 04's five now run in their written order, which is a real dependency
chain: `P4-01` fetches, `P4-02`/`P4-03` compute over what it fetches, `P4-04`
exposes both as one tool, and `P4-05` applies them installation-wide. None of
them is flagged `needs-verify` on `P2-06`'s result: the rate limit bounds how
fast invocations arrive, not what any Phase 04 tool computes, and the
constants came out unchanged besides.

**Four decisions settled 2026-08-25.** *Transport:* **stdio only** — no
listening port, no client-auth subsystem, and every log line goes to stderr
because stdout carries the framing (phase 01). *Supervisor:* **`hassio_api:
true` at the default role** — `list_apps` becomes implementable and
`get_system_health` stops being partial; still no `*/stats` and no write-capable
path (phase 00). *Catalog:* **the full twenty before release** — no reduced first
cut; a tool the evidence rules out ships answering `unsupported`, not missing
(phase 03). *HA versions:* **current release only** — one adapter variant, one CI
target, `unsupported` with the detected version outside it (phase 00). Rationale
and rejected alternatives in each phase file.

**One decision remains open**, not blocking this queue: Phase 02's Q10
(persistence), deliberately not asked yet — it waits on Phase 05 producing a
diagnostic memory-only cannot deliver.

`cmd/spike` is the probe vehicle these tasks reuse: `HA_URL` + `HA_TOKEN`, it
reports field names and types only. The owner runs it and pastes the report; no
HA token reaches the agent (owner's choice, 2026-08-23).

## Status

<!-- DERIVED — do not hand-edit. Regenerate:
for f in docs/development/phases/*.md; do
  printf '%s %s/%s\n' "$(basename "$f")" \
    "$(grep -c '^- \[x\]' "$f")" "$(grep -c '^- \[[ x]\]' "$f")"
done
-->

| phase | theme | done / total |
|------:|-------|:------------:|
| 00 | Spike & Foundations | 15 / 15 |
| 01 | HA Access & Read-Only Gateway | 10 / 10 |
| 02 | Policy, Privacy, Budget & Audit | 7 / 8 |
| 03 | MCP Server & Inventory Tools | 9 / 9 |
| 04 | History, Statistics & Detection | 2 / 5 |
| 05 | Diagnostics & Evidence Engine | 0 / 1 |
| 06 | Proposal Mode — gated | 0 / 1 |
| 07 | Controlled Change (Admin) — gated | 0 / 1 |

Counts include each phase's `needs-decision` entries, which are boxes too. Phase
00 is now fully ticked — its last two were the HA-version and Supervisor
decisions, both settled 2026-08-25. **Phase 01 is now fully ticked** — `P1-08`
closed 2026-08-25: `SupervisorClient` and its own route allow-list in
`internal/ha`, `addon/config.yaml` flipped to `hassio_api: true`. **`P2-06`
closed 2026-09-05** — the F-20 rate-limit measurement, `invocationBurst`/
`invocationInterval` confirmed unchanged. Phase 02's one remaining open box is
the Q10 persistence decision — `P2-05` closed 2026-08-25. **Phase 03 is now
fully ticked** — its catalog-scope decision, `P3-01` through `P3-08`, the last
(`P3-08`, the F-21 exit-code fix) closed 2026-09-05.

Phases 00–04 are milestone M1 (v1 observer). Phase 05 is M2. Phases 06–07 are
gated: they open only on an explicit owner decision plus a fresh security review.
Phases 05–07 carry no task boxes yet — theirs are written by `devflow plan` when
the phase before them closes.

Last refreshed: 2026-09-05 (`P4-02` closed — Phase 04 now 2/5 — pointer
advanced to `P4-03`)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 1 · `defer` 3 · `unknown` 2 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

**F-20 resolved 2026-09-05** by `P2-06`'s close (row dropped from the queue,
pointer advanced to `P4-01`). One `queue-next` remains: **F-23**, filed while
closing `P3-07` (the logbook fallback it added bypasses `internal/redact`'s
privacy classification), not yet queued as a task — the next `plan` should
give it one.

Three `defer`s now. **F-24** is new, filed while closing `P4-02`: doc §12.1's
example statistics are not self-consistent (its `0.982` ratio contradicts its
own `3h12m`/`7d`), harmless to the server but a trap for whoever implements
`P4-04` against that shape — a one-line doc fix. Two older ones stand,
re-triaged and deferred again on grounds that have not changed: **F-6** — Phase 04 has not closed, so the statistics layer its
verification needs still does not exist · **F-17** — nothing has read statistics
yet, so `P2-01`'s conservative estimate has never bound; `P4-04` is the first
place it can, and that is where to revisit it.

The two open `unknown`s are F-6 and F-17; neither has a task yet.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-09-05 · `P4-02` — availability and outage analysis lands, opening
  `internal/analysis`: `ComputeAvailability` reduces one entity's mapped
  history over a bounded window to availability ratio, unavailable-period
  count, total and longest outage, normalizing untrusted points (sorting,
  collapsing repeats, clamping to the window) itself. `unavailable` and
  `unknown` both count as not-available, matching the two-way split the rest
  of the catalog already uses, with a contiguous mixed run counted as **one**
  outage and the `unknown` share preserved as `UnknownDuration`. A leading
  recorder gap reduces `Covered` rather than being charged as downtime, and
  nothing recorded leaves `Computable` false instead of a 0.0 ratio that
  reads as total failure. Outages carry `TruncatedStart`/`OpenEnded` for the
  window's two edges. New fixture `entity_history_7d.json` (413 points,
  minimal `s`/`lu` shape) reproduces doc §12.1's outage numbers through the
  real mapper; `deps_test.go` asserts the package's non-test code imports
  only `internal/model`. **Found:** doc §12.1's example is not
  self-consistent — 3h12m over 7d is 0.98095, not the 0.982 it prints.
- 2026-09-05 · `P4-01` — `get_entity_history` lands, Phase 04's first task:
  `internal/ha` gains `CoreReader.History` over the already-allow-listed
  `history/history_during_period`, mapping both the minimal_response short
  keys (`s`/`lu`) and the full long ones; `internal/mcp/history_tools.go`
  validates entity-id shape and the range, refuses over a fixed 7-day window
  cap before touching policy or budget, and is the first tool to actually use
  Phase 02's `Profile.CheckHistoryScope` and `QueryBudget.Preflight`/
  `Charge*` machinery. `resolution` (Appendix A.2) deliberately not
  implemented — left to `P4-04`'s statistics source (phase file decision
  record).
- 2026-09-05 · `P2-06` — measured the invocation rate limit, closing F-20:
  `cmd/spike/arrival.go`'s new `probeArrivalRate` streams a budget-compliant
  `history/history_during_period` call (10 ids, 24h — the 2026-08-24 run's
  largest rung inside both the byte and point caps) at 1/2/4 calls/s, pipelined
  via a paced writer goroutine + a draining reader matched by WS message id so
  it measures contention rather than round-trip time; best-effort Core CPU via
  a `sensor.*` name/unit heuristic, `unsupported` when none matches
  (`/core/stats` needs a role this App doesn't request). **Found:** no
  measurable latency or CPU degradation at any tested rate, including 4/s —
  double the current sustained limit
  ([`2026-09-05-ha-invocation-rate-limit.md`](../research/2026-09-05-ha-invocation-rate-limit.md));
  `invocationBurst`/`invocationInterval` confirmed unchanged, comment now citing
  the note. `internal/policy/ratelimit_test.go` extended to assert the
  post-burst refusal's `RetryAfter` is consistent with `invocationInterval`. A
  first probe cut mis-sized "max-page" as the cost ladder's widest rung
  (200 ids/7d, 7.63 MB, 15× over the byte cap) and was caught before its severe
  degradation under load was mistaken for the answer.
- 2026-09-05 · `P3-08` — session end is a shutdown, not a crash, closing
  F-21 and Phase 03: `internal/mcp.Run` stops delegating to
  `(*sdkmcp.Server).Run`, which cannot tell a session-end error from a
  startup failure, and instead calls `srv.Connect` itself — a non-nil error
  from that call is the only case treated as real, every other way an
  established session ends (cancelled context, clean disconnect, a client
  dying mid-request) logs "stopped" at INFO and returns nil; the new
  unexported `run(ctx, srv, logger, transport)` takes an already-built
  `*sdkmcp.Server` so a test can drive a probe tool table over a raw
  `net.Pipe` — `NewInMemoryTransports`'s `ClientSession.Close()` was tried
  first and rejected, since it waits for the client's own in-flight call to
  retire before closing, which deadlocks exactly the "still in flight"
  scenario under test; `cmd/server`'s `run()` lost its
  `io.EOF`/`context.Canceled` special-casing, since `mcp.Run` now owns that
  distinction entirely. Live-verified: a hand-driven real stdio client that
  closed stdin mid-`tools/call` left the process at exit 0, logging
  `"stopped" "reason":"session ended" "detail":"server is closing: EOF"`.
- 2026-09-05 · `P3-07` — `get_automation`/`get_automation_traces` land,
  closing F-11: `internal/ha/automation_commands.go` adds the typed
  `automation/config`/`trace/list`/`logbook/get_events` commands (already
  allow-listed since P0-05) and three `CoreReader` methods;
  `get_automation` maps `automation/config` through the already-existing
  `MapAutomation`; `get_automation_traces` reads `trace/list` scoped to one
  automation into `AutomationTraceSummary`, newest first, through a
  strictly-typed `traceSummaryWire` so a retyped field fails loudly
  (`ErrUnexpectedMessage`) rather than degrading to `Partial`, unlike a
  registry entry; `classifyAutomationError` turns a permission refusal
  (`unauthorized`) into `Unsupported` naming the fallback, and a version
  gap (`unknown_command`) into `Unsupported` naming the detected version —
  kept distinct from each other and from a real Go error (`ErrNotFound`
  included); the permission branch additionally fetches `last_triggered`
  plus `logbook/get_events` live into the response's `fallback_*` fields,
  not merely documented. One gap surfaced closing this box: the logbook
  fallback bypasses `internal/redact`'s privacy classification — filed as
  F-23, left for the next `plan` rather than folded in.
## Project facts

| | |
|---|---|
| default model | `claude-sonnet-5` |
| stronger model | `claude-opus-5` |
| repository | `https://github.com/freemanjava/ha-explorer-mcp.git` (`origin`) |
| module path | `github.com/freemanjava/ha-explorer-mcp` |
| base branch | `main` |
| branch per task | `<type>/<task-id>` — e.g. `feat/P0-01` |
| build / test | `make check` |
| run locally | `make run` |
| architecture | [`../HA_Inspector_MCP_Research_and_Architecture.md`](../HA_Inspector_MCP_Research_and_Architecture.md) |
| standards | [`CLAUDE.md`](../../CLAUDE.md) |
| method | [`METHOD.md`](METHOD.md) |

## Loop

`devflow next` — one task, start to finish. `devflow help` for everything else
(planning, findings, status, the standalone prompts).

Without the skill: read this file, read the active task's phase file and
`CLAUDE.md`, check the model gate, branch, write the test, implement, verify
green, close atomically, stop without committing.
