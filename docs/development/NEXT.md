# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `verify F-48` — protection mode as installed (owner's SSH read) ·
[phase 08](phases/08-v1-release.md) · **default** model · `needs-verify` (run `devflow verify`).

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Suspended

None. (`P8-14` closed 2026-10-04.)

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | verify F-48 | protection mode as installed (owner's SSH read) | 08 | default | `needs-verify` |
| 2 | `P8-06` | cut v1.0 (version bump; owner tags) | 08 | default | |

**Ordering rationale (2026-10-04, `plan` after F-45's `verify`).** `P8-13` first: it clears the only
`blocks-active`, and F-46's audit figures are worth reading only on a build whose Supervisor tools return data.
`P8-14` (same file) closed 2026-10-04. F-46…F-48 stay `verify`s — none has an
established cause — and run before `P8-06` because phase 08's DoD wants every §21 row a pass, not a finding;
any of them may add a box. `defer`s re-triaged, unchanged: F-32, F-33, F-34, F-36, F-43, F-44.

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

**Open decision:** none. D-08-13…D-08-15 (2026-10-04 `plan`): Supervisor envelope unwrapped in `get`, mappers require their key, uncalled Supervisor routes dropped — owner may overturn at review. D-08-12 (owner, 2026-10-03): App options = privacy profile + log level; budget
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
| 08 | v1.0 Release | 28 / 29 |

Counts include each phase's decision entries, which are boxes too. Phase 08's
ticks are D-08-1…D-08-15, P8-01, P8-02, P8-03, P8-04, P8-05, P8-07, P8-08, P8-09, P8-10, P8-11, P8-12, P8-13 and P8-14; its one open box is `P8-06`.

Phases 00–04 are milestone M1 (v1 observer); phase 05 (M2) is complete. Phase 08
ships them as v1.0 and runs before 06–07, which stay gated: they open only on an
explicit owner decision plus a fresh security review, and need v1 usage data.

Last refreshed: 2026-10-04 (`P8-14` closed)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 3 · `defer` 6 · `unknown` 1 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

No `blocks-active` (**F-45** closed `done` by `P8-13`, 2026-10-04). `queue-next` **F-46** (defect, awaits `plan`), **F-47, F-48** (unknowns, queued as `verify`s); **F-49** closed `done` by `P8-14`. Six `defer`s: **F-32** (D-05-7's
`search/related` fallback), **F-33** (phase 06 topic), **F-34** (non-admin gets
no automation hypotheses), **F-36** (statistics-based staleness; re-triage on v1
usage data), **F-43** (TLS; re-triage with remote access), **F-44** (configurable budget limits; v1 usage data). One open `unknown`: **F-48**. **F-47** `verify`d 2026-10-04 into a `defect` (trace/list keyed by object id, HA keys by config id; needs a `plan` task before `P8-06`). **F-46** was `verify`d into a `defect` (audit cost counted only where a handler charges); it needs a `plan` task before `P8-06`.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-10-04 · `P8-14` — five uncalled Supervisor routes and ten raw readers dropped; reachability test now follows a route to a caller outside `internal/ha`. Closes F-49.
- 2026-10-04 · `P8-13` — Supervisor envelope unwrapped in `get`, mappers require their key; observed on the Pi (0.9.3): health populated, 7 Apps listed. Closes F-45.
- 2026-10-04 · `P8-05` — §21 walk on the Pi: 8 pass, 4 findings (F-45…F-48); Core restart → backoff → `reconnected`, next call OK. Report `docs/research/2026-10-04-v1-acceptance.md`.
- 2026-10-03 · `P8-09` — App packaged for HTTP (`http_secret`, port closed, `run.sh`, AppArmor `/data/options.json`); `0.9.2` observed on the Pi: 401/initialize/Claude Code OK. Closes F-37.
- 2026-10-03 · `P8-10` — Dockerfile `ARG VERSION` → `-X main.version`; `release.yml` passes config.yaml's version to both builds. Closes F-39.
