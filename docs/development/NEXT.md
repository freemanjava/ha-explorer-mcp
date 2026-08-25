# Next

<!-- BOUNDED FILE — rewritten in place, never appended to. Keep under ~100 lines.
     Anything that grows goes to journal/. This file is read by every session. -->

**▶ Active:** `P2-03` — Redaction and masking
· [`phases/02-policy-privacy-audit.md`](phases/02-policy-privacy-audit.md) · model: **claude-opus-5** · flags: 🧠

> Advancing this pointer is part of finishing a task, together with ticking the
> box, recomputing status and appending a journal entry. All four, or none.

## Queue

Ordered by dependency, not by phase number. Work strictly top to bottom, one per
cycle. Remove a row when its task closes.

| # | id | task | phase | model | flags |
|--:|----|------|-------|-------|-------|
| 1 | `P2-03` | Redaction and masking | 02 | claude-opus-5 | 🧠 |
| 2 | `P2-04` | Response size cap and pagination contract | 02 | claude-sonnet-5 | |
| 3 | `P2-05` | Audit logger | 02 | claude-sonnet-5 | |

**Order rationale.** Phase 01 finishes first: `P2-01` charges its budget against
the typed failures `P1-04` defines, and `P2-02` / `P2-03` classify and mask the
domain values `P1-05` maps — classifying raw HA JSON would put a privacy
decision inside `internal/ha`, where the dependency direction forbids it.
`P1-06` closes Phase 01 because its observation timestamp is the `cache age`
every tool response carries. Inside Phase 02, enforcement before application:
`P2-01` (its `MaxBytes` is what `P2-04` cuts against, constants already measured
by `P0-09`) → `P2-02` (decide what is sensitive) → `P2-03` (apply it at the
boundary) → `P2-04` (truncation must cut an already-masked response) → `P2-05`
(the audit record reuses `P2-03`'s redaction rather than growing a second copy).

**No new boxes.** Phase 02's five were written when the phase was drafted; this
`plan` settled the decision blocking `P2-02`, folded its consequences into the
`P2-02` / `P2-03` DoDs, and ordered the block into the queue.

**Decision settled 2026-08-25 — PRIVATE handling: mask by default.** `PRIVATE`
states become opaque tokens with timestamps and transition counts preserved;
installation coordinates coarsen to one decimal, `location_name` passes through;
`allow` / `deny` stay selectable. Rationale and rejected alternatives in
[`phases/02-policy-privacy-audit.md`](phases/02-policy-privacy-audit.md).

**Two decisions remain**, neither blocking this queue, both required before
Phase 03 is planned: the App's Supervisor permission level (phase 00 — gates the
tool catalog) and the MCP transport and client authentication (phase 01 — gates
whether Phase 03 needs an auth subsystem, and decides the server's whole network
exposure, threat T5). Phase 02's Q10 (persistence) is deliberately not asked
yet: it waits on Phase 05 producing a diagnostic memory-only cannot deliver.

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
| 00 | Spike & Foundations | 13 / 15 |
| 01 | HA Access & Read-Only Gateway | 8 / 9 |
| 02 | Policy, Privacy, Budget & Audit | 3 / 7 |
| 03 | MCP Server & Inventory Tools | 0 / 8 |
| 04 | History, Statistics & Detection | 0 / 5 |
| 05 | Diagnostics & Evidence Engine | 0 / 1 |
| 06 | Proposal Mode — gated | 0 / 1 |
| 07 | Controlled Change (Admin) — gated | 0 / 1 |

Counts include each phase's `needs-decision` entries, which are boxes too — one
of Phase 01's five ticks is its deny-list decision, not a task; of Phase 02's
three, one is its PRIVATE-handling decision, settled 2026-08-25; the other two
are `P2-01` and `P2-02`.

Phases 00–04 are milestone M1 (v1 observer). Phase 05 is M2. Phases 06–07 are
gated: they open only on an explicit owner decision plus a fresh security review.
Phases 05–07 carry no task boxes yet — theirs are written by `devflow plan` when
the phase before them closes.

Last refreshed: 2026-08-25 (`devflow next` — `P2-02` closed)

## Open findings

<!-- DERIVED from FINDINGS.md — counts only, never the findings themselves.
     grep -c '^\*\*Triage:\*\* `queue-next`' docs/development/FINDINGS.md  (etc.)
     This block exists so captured work cannot quietly rot: every session sees it. -->

`blocks-active` 0 · `queue-next` 3 · `defer` 3 · `unknown` 3 (open)

> Any `blocks-active` is stop-work. If `queue-next` is non-zero and the queue
> above has fewer than 3 rows, drain it with `devflow plan` before continuing —
> a queue that empties while findings wait is how real work gets lost.
>
> An open `unknown` outranks the queue: it is an assumption the plan already
> rests on. Run `devflow verify` before building further on it.

The 2026-08-25 `plan` closed four findings whose tasks had already landed while
their triage still read `queue-next` (**F-13**, **F-18** → `P1-07` · **F-16** →
`P0-11` · **F-19** → `P0-10`) — the count read 7, not 3. A finding closes when
its tasks close, not when they are queued.

The three open ones are each already pinned into a DoD, none needing a box:
**F-10**, **F-12** → `P2-02` (closed 2026-08-25 — installation coordinates and
embedded-entity payloads both classify now) and `P2-03`, which applies that
classification and is what closes them · **F-11** → `P3-07` plus a
Phase 05 §13.1 bullet. Both `defer`s stay deferred: **F-6** (Phase 05 owns it),
**F-17** — `P2-01` landed with the conservative batched figure in its statistics
estimate and a comment saying so, exactly as the deferral anticipated, so
nothing has changed to make resolving it worthwhile. `P2-01` filed a third:
**F-20**, the invocation rate limit's constants are derived from single-call
timings rather than measured, `defer` until Phase 03 wires the limiter.

## Recent

Last 5 closed tasks, one line each. Older entries live in `journal/`.

- 2026-08-25 · `P2-02` — Privacy classification and profiles in
  `internal/policy`: `Sensitivity` (normal/private/secret) decided by readable
  tables — private domains, occupancy device classes, secret key fragments,
  location attributes, `get_config` coordinates — plus `ClassifyPayload`, which
  classifies by the entities a payload embeds at any depth (F-12), and
  `Profile` (mask default / allow / deny) with `ErrPolicyDenied` refusing bulk
  history over a private domain.
- 2026-08-25 · `P2-01` — Query budget in `internal/policy`: `QueryBudget`
  charged per dimension against measured class limits, attached to the
  invocation context with its deadline, a pre-flight entity-day estimate that
  refuses before the recorder is asked, and a token-bucket rate limiter for
  request storms (threat T1).
- 2026-08-25 · `P1-06` — `RegistryCache` in `internal/ha`: TTL + single-flight
  refill per doc §16 for entity/device/area/config-entries registries; every
  served value carries its observation time; a fake `caller` (no real
  WebSocket) makes TTL expiry and concurrency deterministic in tests.
- 2026-08-25 · `P1-05` — Normalized domain model: `Entity`, `DeviceRef`,
  `Integration`, `Area`, `Automation`, `Health`, `Evidence` in `internal/model`;
  explicit permissive mapping in `internal/ha` marks a value `Partial` on a
  malformed/missing field instead of panicking or zeroing it.
- 2026-08-25 · `P1-04` — Full error taxonomy: `ErrUnsupported` + `ErrDeadline`
  added, `CommandError.Unwrap` maps HA's `unauthorized`/`not_found` codes, ctx
  deadlines wrapped distinctly from `ErrUpstreamUnavailable` in WS and REST.

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
