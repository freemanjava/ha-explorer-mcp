# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P8-06` — cut v1.0 (version bump; owner tags) ·
[phase 08](phases/08-v1-release.md) · **default** model.

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Suspended

None. (`P8-18` closed 2026-10-04.)

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P8-06` | cut v1.0 (version bump; owner tags) | 08 | default | |

**Ordering rationale (2026-10-04, `plan` for F-50).** `P8-18` before `P8-06`: v1.0 should not ship an audit trail that
records every in-tool failure as `success`. `defer`s re-triaged, unchanged.

**Earlier (2026-10-04, `plan` for F-46, F-47).** `P8-15` before `P8-16`: both edit `middleware.go`, and
P8-16's "measured, not charged" assertion reads cleaner once request charges have left the tools. `P8-17` is
independent but goes third so a single Pi deploy observes its live trace check *and* the corrected audit figures.
All three precede `P8-06`: phase 08's DoD wants every §21 row a pass, not a finding. `defer`s re-triaged, unchanged:
F-32, F-33, F-34, F-36, F-43, F-44.

**Phase 05's design decisions** (D-05-1…10) stand in its phase file: separate
fact/inference/recommendation types, one `ConfidenceFor`, overlap-with-tolerance
clusters, no health score, flat mesh analyzer, `analyze_automation_health` as
the twenty-first tool, mapper-side dependencies, budget classes as measured.

**Earlier decisions, still standing.** *Transport:* stdio only (phase 01) — **superseded in part by D-08-1**
(HTTP in the App, LAN, port closed by default, owner-set secret, fail closed; stdio kept for development).
*HTTP transport security:* D-08-4…D-08-11 / ADR-013 (`P8-08`) — bearer secret with constant-time compare and
`Authorization` stripped before the SDK; any `Origin` refused; `:8790` `POST /mcp`; 128 KiB body, 4 in flight;
stateless JSON; transport by env fixed in `run.sh`, secret from `/data/options.json`; no TLS in v1.
*Supervisor:* `hassio_api: true` at the default role (phase 00). *Catalog:* the
full twenty before release (phase 03). *HA versions:* current release only
(phase 00). `P4-05`: a PRIVATE entity is excluded outright from both `find_*`
tools under the deny profile, never masked.

**Open decision:** none. D-08-19 (2026-10-04 `plan`): a tool's `IsError` result is classified and redacted from `GetError()` like a returned error — owner may overturn at review. D-08-16…D-08-18 (2026-10-04 `plan`): requests counted at the wire seams through a context meter, `result_bytes` measured from the result, traces keyed by config id — owner may overturn at review. D-08-13…D-08-15 (2026-10-04 `plan`): Supervisor envelope unwrapped in `get`, mappers require their key, uncalled Supervisor routes dropped — owner may overturn at review. D-08-12 (owner, 2026-10-03): App options = privacy profile + log level; budget
limits stay constants, CLAUDE.md corrected in `P8-12`. D-08-1 decided 2026-10-03 (evidence: `docs/research/2026-10-03-mcp-client-paths.md`). Q10 (persistence) closed
2026-10-03: memory-only in v1.

**Probes:** `cmd/spike` / `cmd/measure` take `HA_URL` + `HA_TOKEN` and report
shapes and costs only; the owner runs them and pastes the report — no HA token
reaches the agent (owner's choice, 2026-08-23).

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
| 02 | Policy, Privacy, Budget & Audit | 8 / 8 |
| 03 | MCP Server & Inventory Tools | 11 / 11 |
| 04 | History, Statistics & Detection | 6 / 6 |
| 05 | Diagnostics & Evidence Engine | 26 / 26 |
| 06 | Proposal Mode — gated | 0 / 1 |
| 07 | Controlled Change (Admin) — gated | 0 / 1 |
| 08 | v1.0 Release | 36 / 37 |

Counts include each phase's decision entries, which are boxes too. Phase 08's
ticks are D-08-1…D-08-19, P8-01, P8-02, P8-03, P8-04, P8-05, P8-07, P8-08, P8-09, P8-10, P8-11, P8-12, P8-13, P8-14, P8-15, P8-16, P8-17 and P8-18; its one open box is `P8-06`.

Phases 00–04 are milestone M1 (v1 observer); phase 05 (M2) is complete. Phase 08
ships them as v1.0 and runs before 06–07, which stay gated: they open only on an
explicit owner decision plus a fresh security review, and need v1 usage data.

Last refreshed: 2026-10-04 (`P8-18`)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 0 · `defer` 6 · `unknown` 0 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

No `blocks-active` (**F-45** closed `done` by `P8-13`, 2026-10-04). **F-46** closed `done` by `P8-16`. **F-47** closed `done` by `P8-17`. **F-50** closed `done` by `P8-18`; **F-49** closed `done` by `P8-14`. Six `defer`s: **F-32** (D-05-7's
`search/related` fallback), **F-33** (phase 06 topic), **F-34** (non-admin gets
no automation hypotheses), **F-36** (statistics-based staleness; re-triage on v1
usage data), **F-43** (TLS; re-triage with remote access), **F-44** (configurable budget limits; v1 usage data). No open `unknown` (**F-48** `verify`d 2026-10-04, closed `done`).

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-10-04 · `P8-18` — an `IsError` tool result is classified (error/denied/budget) and redacted like a returned error; text scrubbed only when changed. Closes F-50.
- 2026-10-04 · `plan F-50` — D-08-19; `P8-18`. F-50 is wider than filed: every in-tool error arrives as an `IsError` result, never classified or redacted.
- 2026-10-04 · `P8-17` — `trace/list` keyed by the automation's config id; none → `unsupported` with its own reason. Observed on the Pi (0.9.4). Closes F-47.
- 2026-10-04 · `P8-16` — `result_bytes` measured from the returned result (structured + text; error result 0), not budget charges. Closes F-46.
- 2026-10-04 · `P8-15` — requests charged at `Manager.Call` / `SupervisorClient.get` through a context meter; per-tool charges removed; audit equals the wire for all 21 tools.
