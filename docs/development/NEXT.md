# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P9-02` · Tool `get_automation_logic` · `docs/development/phases/09-automation-logic.md` · claude-sonnet-5-5
v1.0 is cut in the tree; the owner tags `v1.0.0` and pushes (independent of phase 09).

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Suspended

None. (`P9-01` closed 2026-10-05.)

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P9-02` | Tool `get_automation_logic` | 09 | claude-sonnet-5-5 | |
| 2 | `P9-03` | Trace-run mapper: `trace/get` → typed steps | 09 | claude-opus-5-5 | 🧠 `blocked:F-54` |
| 3 | `P9-04` | Tool `get_automation_trace` | 09 | claude-sonnet-5-5 | `blocked:P9-03` |
| 4 | `P9-05` | Observe on the Pi, measure, ship v1.1 | 09 | claude-sonnet-5-5 | `live-verify` `blocked:P9-04` |

**Ordering rationale (2026-10-04, `plan` for F-53).** Logic before traces: `P9-03` reuses `P9-01`'s `TypedValue`
grammar, and logic alone already answers "what are the thresholds". Each tool follows its mapper. `P9-05` last: one Pi
deploy observes both tools and measures the caps. `defer`s re-triaged, unchanged (F-32 is not needed: D-09-2 walks
the config itself; F-34 is unaffected because both new tools are admin-gated too).

**Earlier (2026-10-04, `plan` for F-51).** One box, nothing to order. D-08-20 (owner): short landing
page. Missing LICENSE filed as F-52, `defer` (owner). Other `defer`s re-triaged, unchanged.

**Earlier (2026-10-04, `plan` for F-50).** `P8-18` before `P8-06`: v1.0 should not ship an audit trail that
records every in-tool failure as `success`. `defer`s re-triaged, unchanged.

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

**Open decision:** none. D-09-1…D-09-5 (owner, 2026-10-04 `plan` for F-53): logic + trace steps; grammar values plus raw template text marked untrusted (a knowing widening of rule 6/T2); two new tools; under `deny` PRIVATE ids masked and templates withheld. D-08-20 (owner, 2026-10-04 `plan`): root README is a short landing page; licensing deferred (F-52). D-08-19 (2026-10-04 `plan`): a tool's `IsError` result is classified and redacted from `GetError()` like a returned error — owner may overturn at review. D-08-16…D-08-18 (2026-10-04 `plan`): requests counted at the wire seams through a context meter, `result_bytes` measured from the result, traces keyed by config id — owner may overturn at review. D-08-13…D-08-15 (2026-10-04 `plan`): Supervisor envelope unwrapped in `get`, mappers require their key, uncalled Supervisor routes dropped — owner may overturn at review. D-08-12 (owner, 2026-10-03): App options = privacy profile + log level; budget
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
| 08 | v1.0 Release | 39 / 39 |
| 09 | Automation Logic & Trace Steps | 6 / 10 |

Counts include each phase's decision entries, which are boxes too. Phase 08's
ticks are D-08-1…D-08-20, P8-01, P8-02, P8-03, P8-04, P8-05, P8-07, P8-08, P8-09, P8-10, P8-11, P8-12, P8-13, P8-14, P8-15, P8-16, P8-17, P8-18, P8-06 and P8-19.

Phases 00–04 are milestone M1 (v1 observer); phase 05 (M2) is complete. Phase 08
ships them as v1.0 and runs before 06–07, which stay gated: they open only on an
explicit owner decision plus a fresh security review, and need v1 usage data. Phase 09
(automation logic, F-53) is v1.1, read-only, and not gated: it adds two read tools, no write path.

Last refreshed: 2026-10-05 (`P9-01`)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 3 · `defer` 7 · `unknown` 1 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

**F-54** `unknown` (2026-10-05, from `P9-01`): logic node paths assume HA's trace-path convention — `verify` before `P9-03` (it blocks that row, not `P9-02`). **F-55** `queue-next`: top-level `variables` and blueprint inputs unmapped. **F-53** `queue-next`, planned as phase 09 (`P9-01`…`P9-05`); closes with `P9-05`. **F-51** closed `done` by `P8-19`. No `blocks-active` (**F-45** closed `done` by `P8-13`, 2026-10-04). **F-46** closed `done` by `P8-16`. **F-47** closed `done` by `P8-17`. **F-50** closed `done` by `P8-18`; **F-49** closed `done` by `P8-14`. Seven `defer`s: **F-52** (no LICENSE; owner deferred 2026-10-04), **F-32** (D-05-7's
`search/related` fallback), **F-33** (phase 06 topic), **F-34** (non-admin gets
no automation hypotheses), **F-36** (statistics-based staleness; re-triage on v1
usage data), **F-43** (TLS; re-triage with remote access), **F-44** (configurable budget limits; v1 usage data). No open `unknown` (**F-48** `verify`d 2026-10-04, closed `done`).

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-10-05 · `P9-01` — logic mapper: grammar-typed nodes, both schema forms, trace-style paths, caps; blueprint configs partial. Filed F-54 (`unknown`), F-55.
- 2026-10-04 · `P8-19` — root `README.md` landing page; drift tests for tool names and version. Closes F-51.
- 2026-10-04 · `plan F-51` — D-08-20 (owner: short landing page); `P8-19`. No LICENSE in the public repo → F-52, `defer`.
- 2026-10-04 · `P8-06` — `addon/config.yaml` bumped to 1.0.0; `make check` green; README clause unmet (no README, F-51). Owner tags.
- 2026-10-04 · `P8-18` — an `IsError` tool result is classified (error/denied/budget) and redacted like a returned error; text scrubbed only when changed. Closes F-50.
