# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P8-04` — `golangci-lint` clean ·
[phase 08](phases/08-v1-release.md) · model: default · run `devflow next`. Owner's open decision meanwhile: D-08-1 (client path).

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Suspended

None.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P8-04` | `golangci-lint` clean | 08 | default | |
| 2 | `D-08-1` | owner: how a client reaches the App | 08 | — | `needs-decision` |
| 3 | `P8-02` | implement D-08-1's path (box written after the decision) | 08 | default | `blocked:D-08-1`, `live-verify` |
| 4 | `P8-05` | §21 acceptance walk on the Pi | 08 | default | `blocked:P8-02`, `live-verify` |
| 5 | `P8-06` | cut v1.0 (version bump; owner tags) | 08 | default | `blocked:P8-05` |

**Ordering rationale (2026-10-03 third `plan`, phase 08 opened by the owner).**
`P8-01` first: it is the open `unknown`, and if the App exits at start (F-37)
the client-path decision reshapes `P8-02`…`P8-06`. The three cleanups need no
Pi and run while the owner gathers `P8-01`'s Pi half; `P8-07` follows `P8-03`
because both edit `gateway.go`. **F-17** closed `wont-fix` and **F-36** stays
deferred (owner): no production statistics call site will exist in v1.

**Phase 05's design decisions** (D-05-1…10) stand in its phase file: separate
fact/inference/recommendation types, one `ConfidenceFor`, overlap-with-tolerance
clusters, no health score, flat mesh analyzer, `analyze_automation_health` as
the twenty-first tool, mapper-side dependencies, budget classes as measured.

**Earlier decisions, still standing.** *Transport:* stdio only (phase 01).
*Supervisor:* `hassio_api: true` at the default role (phase 00). *Catalog:* the
full twenty before release (phase 03). *HA versions:* current release only
(phase 00). `P4-05`: a PRIVATE entity is excluded outright from both `find_*`
tools under the deny profile, never masked.

**Open decision:** D-08-1 (client path); `P8-01`'s evidence is in its box and `docs/research/2026-10-03-app-under-supervisor.md`. Q10 (persistence) closed
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
| 08 | v1.0 Release | 4 / 9 |

Counts include each phase's decision entries, which are boxes too. Phase 08's
ticks are D-08-2, P8-01, P8-03 and P8-07; its open boxes are four tasks plus D-08-1.

Phases 00–04 are milestone M1 (v1 observer); phase 05 (M2) is complete. Phase 08
ships them as v1.0 and runs before 06–07, which stay gated: they open only on an
explicit owner decision plus a fresh security review, and need v1 usage data.

Last refreshed: 2026-10-03 (`P8-07` closed)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 3 · `defer` 4 · `unknown` 0 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

Three `queue-next`: **F-37** → `D-08-1`/`P8-02`; **F-39**
(the `0.0.0-dev` version) and **F-40** (six uncalled allow-list entries) await `plan`. Four `defer`s: **F-32** (D-05-7's
`search/related` fallback), **F-33** (phase 06 topic), **F-34** (non-admin gets
no automation hypotheses), **F-36** (statistics-based staleness; re-triage on v1
usage data). No open `unknown`: F-37 was verified 2026-10-03 and is now a confirmed defect.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-10-03 · `P8-07` — Core REST adapter deleted (`rest.go`, five routes, `validateEntityID`);
  Supervisor client keeps its own size cap/deadline tests. Closes F-38.
- 2026-10-03 · `P8-03` — three statistics commands dropped from the allow-list; reachability
  test over `internal/` (red shown). It found six more uncalled entries: F-40, exempted by name.
- 2026-10-03 · `P8-01` — App observed off-box and on the Pi: exits 0 ~65 ms after start and
  stays stopped (no restart loop); stdio serves when stdin is held; SSH App has no `docker`.
  F-37 confirmed, D-08-1 unblocked; `0.0.0-dev` version filed as F-39.
- 2026-10-03 · `P5-10` — `cmd/measure` (real tools in-process) + logbook probe; owner ran both. D-05-10:
  classes stand (`find_stale_entities` 20 req/page, `analyze_*` max 36/50). Closes F-26, F-28.
- 2026-10-03 · `P5-08` — §13.2 end to end in `investigation_test.go`: partial parent ⇒
  topology claim, none without a parent, star ⇒ `withheld`; mesh evidence vs
  `entity_disabled`; host row `privileged`. Closes F-27, F-35.
