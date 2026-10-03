# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P8-02` — Streamable HTTP transport beside stdio · [phase 08](phases/08-v1-release.md) ·
run `devflow next` on the **default** model. Implements exactly D-08-4…D-08-11; its DoD lists each assertion.
Config reading goes into a new `cmd/server/config.go`, which `P8-12` then extends.

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Suspended

None.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P8-02` | Streamable HTTP transport beside stdio (D-08-4…D-08-11) | 08 | default | — |
| 2 | `P8-12` | App options carry privacy profile + log level (F-42, D-08-12) | 08 | default | `blocked:P8-02` |
| 3 | `P8-11` | drop six uncalled allow-list entries (F-40, D-08-3) | 08 | default | — |
| 4 | `P8-10` | binary reports the image version (F-39) | 08 | default | — |
| 5 | `P8-09` | package + real client on the Pi | 08 | default | `blocked:P8-02`, `blocked:P8-12`, `live-verify` |
| 6 | `P8-05` | §21 acceptance walk on the Pi | 08 | default | `blocked:P8-09`, `live-verify` |
| 7 | `P8-06` | cut v1.0 (version bump; owner tags) | 08 | default | `blocked:P8-05` |

**Ordering rationale (2026-10-03, `plan` after D-08-1).** The owner chose HTTP inside the App (D-08-1); phase 01
required a fresh security review before any listener, so `P8-08` gates `P8-02`. `P8-11` and `P8-10` need no Pi and
must land before the image the owner publishes for `P8-09`, so the acceptance walk runs the final build.
`defer`s re-triaged, unchanged: F-32, F-33, F-34, F-36 are untouched by the transport.
**2026-10-03, `plan` after `P8-08`:** `P8-12` (F-42) follows `P8-02` because it extends the `config.go` that
`P8-02` creates, and precedes `P8-09` so the image on the Pi has the options. `defer`s re-triaged, unchanged:
F-32…F-36 as above, F-43 (TLS, with remote access), F-44 (configurable budgets, on v1 usage data).

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

**Open decision:** none. D-08-12 (owner, 2026-10-03): App options = privacy profile + log level; budget
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
| 08 | v1.0 Release | 17 / 24 |

Counts include each phase's decision entries, which are boxes too. Phase 08's
ticks are D-08-1…D-08-12, P8-01, P8-03, P8-04, P8-07 and P8-08; its open boxes are seven tasks.

Phases 00–04 are milestone M1 (v1 observer); phase 05 (M2) is complete. Phase 08
ships them as v1.0 and runs before 06–07, which stay gated: they open only on an
explicit owner decision plus a fresh security review, and need v1 usage data.

Last refreshed: 2026-10-03 (`plan` after `P8-08`)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 4 · `defer` 6 · `unknown` 0 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

Four `queue-next`: **F-37** → `P8-08` ✓/`P8-02`/`P8-09`; **F-39** → `P8-10`;
**F-40** → `P8-11`; **F-42** → `P8-12`. Six `defer`s: **F-32** (D-05-7's
`search/related` fallback), **F-33** (phase 06 topic), **F-34** (non-admin gets
no automation hypotheses), **F-36** (statistics-based staleness; re-triage on v1
usage data), **F-43** (TLS; re-triage with remote access), **F-44** (configurable budget limits; v1 usage data). No open `unknown`: **F-41** closed `done` 2026-10-03 by `verify`. F-37 is a confirmed defect.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-10-03 · `P8-08` — HTTP security review: D-08-4…D-08-11, ADR-013. The SDK hands `Authorization` to server code
  (strip it); no default Origin check. "Port closed" doesn't cover the `hassio` network. Filed F-42, F-43.
- 2026-10-03 · `F-41` verify — (d) void (Supervisor stdin is write-only); official SSH App has no Docker; HA's own
  MCP is a Core integration. Owner chose (c): D-08-1 = HTTP in the App. Re-planned P8-08…P8-11.
- 2026-10-03 · `P8-04` — golangci-lint 2.14.0 clean: 13 issues fixed (errcheck in tests, one
  tagged switch); no design-level findings.
- 2026-10-03 · `P8-07` — Core REST adapter deleted (`rest.go`, five routes, `validateEntityID`);
  Supervisor client keeps its own size cap/deadline tests. Closes F-38.
- 2026-10-03 · `P8-03` — three statistics commands dropped from the allow-list; reachability
  test over `internal/` (red shown). It found six more uncalled entries: F-40, exempted by name.
