# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P3-01` — MCP server bootstrap and tool registry
· [`phases/03-inventory-tools.md`](phases/03-inventory-tools.md) · model: **claude-opus-5** · flags: 🧠

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P3-01` | MCP server bootstrap and tool registry | 03 | claude-opus-5 | 🧠 |
| 2 | `P3-02` | `get_system_overview` / `get_system_health` | 03 | claude-sonnet-5 | |
| 3 | `P3-03` | `list_integrations` / `get_integration` | 03 | claude-sonnet-5 | |
| 4 | `P3-04` | `list_devices` / `get_device` | 03 | claude-sonnet-5 | |
| 5 | `P3-05` | `list_entities` / `get_entity` | 03 | claude-sonnet-5 | |
| 6 | `P3-06` | `list_areas` / `list_automations` / `list_repairs` / `list_apps` | 03 | claude-sonnet-5 | |
| 7 | `P3-07` | `get_automation` / `get_automation_traces` | 03 | claude-sonnet-5 | |

**Order rationale.** `P2-05` closed Phase 02: `internal/audit` reused
`P2-03`'s redaction rather than growing a second copy, so `P3-01` can wire
audit middleware into every invocation against a record shape that already
exists. `P1-08` closed Phase 01: `SupervisorClient` now reads the endpoints
the 2026-08-25 Supervisor decision grants, so `P3-02` and `P3-06`'s
`blocked:P1-08` is lifted — both queued unblocked. Phase 03 now runs in its
written order — `P3-01` first because everything registers into it, `P3-07`
last because it is the most compatibility-sensitive surface in the phase.
Phase 04's five follow this queue unbroken: the catalog decision below rules
out an interleaved release cut, so there is no reason to reorder them against
Phase 03.

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
| 02 | Policy, Privacy, Budget & Audit | 6 / 7 |
| 03 | MCP Server & Inventory Tools | 1 / 8 |
| 04 | History, Statistics & Detection | 0 / 5 |
| 05 | Diagnostics & Evidence Engine | 0 / 1 |
| 06 | Proposal Mode — gated | 0 / 1 |
| 07 | Controlled Change (Admin) — gated | 0 / 1 |

Counts include each phase's `needs-decision` entries, which are boxes too. Phase
00 is now fully ticked — its last two were the HA-version and Supervisor
decisions, both settled 2026-08-25. **Phase 01 is now fully ticked** — `P1-08`
closed 2026-08-25: `SupervisorClient` and its own route allow-list in
`internal/ha`, `addon/config.yaml` flipped to `hassio_api: true`. Phase 02's one
remaining open box is the Q10 persistence decision — `P2-05` closed 2026-08-25.
Phase 03's one tick is its catalog-scope decision.

Phases 00–04 are milestone M1 (v1 observer). Phase 05 is M2. Phases 06–07 are
gated: they open only on an explicit owner decision plus a fresh security review.
Phases 05–07 carry no task boxes yet — theirs are written by `devflow plan` when
the phase before them closes.

Last refreshed: 2026-08-25 (`devflow next` — `P1-08` closed, Phase 01 done, pointer advanced to `P3-01`)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 1 · `defer` 3 · `unknown` 3 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

The 2026-08-25 `plan` drained the inbox again and wrote no boxes from it. The
one `queue-next`, **F-11**, was already planned into `P3-07` (now queue row 8)
plus a Phase 05 §13.1 bullet — it closes when `P3-07` closes, not now.

All three `defer`s were re-triaged and stay deferred: **F-6** (Phase 05 owns
it) · **F-17** — `P2-01` landed with the conservative batched figure and a
comment saying so, exactly as the deferral anticipated · **F-20** — the
invocation rate limit's constants are still unmeasured, and `P3-01` is what
first wires the limiter into a running server, so the earliest honest point to
measure them is the `plan` after `P3-01` closes, not this one.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-08-25 · `P1-08` — `internal/ha/supervisor.go`: `SupervisorClient` reads
  Supervisor's own API — its own base (`http://supervisor`), its own
  exact-match GET-only route table holding only the eleven endpoints the
  default role and `api_bypass` grant; Supervisor unreachable maps to
  `ErrUnsupported`, distinct from Core's `ErrUpstreamUnavailable`, so a
  Core-based answer keeps working with Supervisor absent; `/supervisor/info`
  is mapped to `model.SupervisorInfo` through a strictly-typed decode that
  fails loudly on a retyped field rather than degrading to `Partial`;
  `addon/config.yaml` now sets `hassio_api: true` at the default role.
- 2026-08-25 · `P2-05` — `internal/audit`: `Logger.Emit` runs
  `Record.Parameters` through the invocation's `*redact.Redactor` before
  logging, so secret literals and masked-private values never reach the trail
  either; `Status` (success/denied/budget_exceeded/error) plus `Reason` keep a
  refusal, a budget cutoff and a success distinguishable; the result body is
  excluded unless `Logger.WithBody()` opts in; `Emit` recovers its own panics
  so a broken sink cannot fail the tool call it is recording.
- 2026-08-25 · `P2-04` — `internal/page`: `Paginate[T]` cuts a caller-sorted
  list at the first of resolved limit / cumulative `byteSize` vs `MaxBytes` /
  list end, always keeping a whole record even when one alone exceeds the cap;
  the cursor encodes the last-returned key (not an index), so a list changed
  between calls cannot duplicate or skip records by construction; `clamped`
  and `truncated` are reported as distinct fields.
- 2026-08-25 · `P2-03` — `internal/redact`: one `Redactor` per response applies
  policy's decisions at the boundary — SECRET keys and secret literals become
  `[redacted]`, PRIVATE states become `[masked:state_<nonce>X]` tokens scoped
  to one entity's timeline, get_config coordinates coarsen to one decimal, and
  every withheld field is marked rather than dropped; a slog handler closes the
  log-line route.
- 2026-08-25 · `P2-02` — Privacy classification and profiles in
  `internal/policy`: `Sensitivity` (normal/private/secret) decided by readable
  tables — private domains, occupancy device classes, secret key fragments,
  location attributes, `get_config` coordinates — plus `ClassifyPayload`, which
  classifies by the entities a payload embeds at any depth (F-12), and
  `Profile` (mask default / allow / deny) with `ErrPolicyDenied` refusing bulk
  history over a private domain.

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
