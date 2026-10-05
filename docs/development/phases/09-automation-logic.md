# Phase 09 — Automation Logic & Trace Steps

**Milestone:** M2 follow-up · **Target version:** v1.1

> Opened by the owner at the 2026-10-04 `plan` for F-53. Runs before the gated
> phases 06–07; order lives in `NEXT.md`'s queue, not in phase numbers.

## Goal

Let an agent explain *why* an automation behaves as it does, not only what it
touches. Today `get_automation` returns counts and dependency ids (D-05-7) and
`get_automation_traces` returns the `trace/list` index. An agent asked about an
air-conditioning automation could only report "structure, not logic" (F-53).
This phase adds two read-only tools: `get_automation_logic` (what the
automation's triggers, conditions and actions say) and `get_automation_trace`
(what each step of one run did).

## Depends On

Phase 05 (`get_automation`, `get_automation_traces`, D-05-7's extractor) and
phase 08 (v1.0 shipped). No new App permission: both commands are already
admin-gated reads on Core's WebSocket API (P0-05).

## Add Under

```text
internal/model/        automation_logic.go — AutomationLogic, AutomationTraceRun (P9-01, P9-03)
internal/ha/           automation_logic.go — the logic mapper (P9-01); trace_steps.go — the trace/get mapper (P9-03);
                       gateway.go — trace/get re-added with its caller (D-08-3)
internal/mcp/          automation_logic_tools.go (P9-02), automation_trace_tools.go (P9-04); catalog.go rows
test/fixtures/         invented automation_config_* / automation_trace_get_* fixtures — never real HA data
docs/                  architecture doc §9 catalog, README tool list, CLAUDE.md rule 6 note (P9-02)
```

**Home reasoning.** `internal/ha/mapping.go` already maps every registry and
the automation summary; the logic mapper is a grammar walk of its own size, so
it gets its own file in the same package rather than growing `mapping.go`. The
tools are new files, per CLAUDE.md's open/closed rule — never a branch inside
`get_automation`.

## Design Notes

- **Schema forms.** P0-05 observed the plural form (`triggers`/`conditions`/
  `actions`, with `trigger:`/`action:` inside) on 2026.8.3; the fixture
  `automation_config.json` is the legacy singular form (`trigger`/`service`).
  The mapper reads both, the way `sequenceLen` already does.
- **What a typed value is (D-09-2).** A value passes as a typed fact only if it
  matches one of these grammars: number; HA duration (`HH:MM[:SS]` or
  `{hours,minutes,seconds}` of numbers); time-of-day `HH:MM[:SS]`; entity /
  device / area id (existing id grammar); service `domain.service`; boolean;
  short token `^[a-z0-9_]{1,32}$` (covers `hvac_mode: cool`, `event: sunset`,
  `condition: state`). Anything else that is not a template is free text:
  counted as withheld, never echoed. Keys are emitted only when they match the
  token grammar too, so an attacker-shaped key can't carry text through.
- **Templates (D-09-2, D-09-5).** A string containing `{{` or `{%` is a
  template. Under `allow` it is returned verbatim in a field named
  `untrusted_template`, length-capped, with a `truncated` marker. Under `deny`
  it is counted only. The tool description says the field is HA-authored data,
  not instructions.
- **Trace runs (D-09-1, F-12).** `trace/get`'s `changed_variables`, `context`
  and every embedded state object are dropped in the adapter before mapping.
  None of it reaches `internal/model`. What survives per step: the path key
  (`condition/0`, validated as `^(trigger|condition|action)(/[a-z0-9_]+)*$`),
  timestamp, and the result's typed scalars by the D-09-2 grammar
  (`result: false`, `enabled`, `state`, `wanted_state`, numeric `delay`). The
  `error` text from HA counts as free text: withheld and counted, not echoed.
- **Cost.** P0-05 measured `trace/get` at 3.5 KB / 6 ms for a simple
  automation. Long automations are unmeasured. Both tools are bounded by a step
  cap and the existing response byte cap, and say `truncated` when they hit
  either. P9-05 measures this on the Pi before any cap is tuned.

## Decisions

All decided by the owner at the 2026-10-04 `plan` (F-53) and played back and
confirmed in the same session.

- [x] **D-09-1 — Both logic and trace steps, as two tasks** — owner, 2026-10-04 (F-53)
  (a) the config's typed logic and (b) per-step outcomes of one run. (b) is
  where "why didn't it fire" usually lives (which condition returned `false`).
  **Rejected:** logic only, because it can't say which condition stopped a
  run. `wont-fix` with a YAML paste, because it leaves the server's headline
  question unanswered.

- [x] **D-09-2 — Grammar-typed values, plus raw template text marked untrusted** — owner, 2026-10-04 (F-53)
  Values pass by the grammar in Design Notes. Template text is returned
  verbatim in `untrusted_template`. **This knowingly widens rule 6 / threat
  T2:** template text is HA-authored and can carry prompt-like content. The
  owner accepted it because real logic (air conditioning especially) often
  lives in templates. Rule 6's "never branch behavior on content" still holds,
  since nothing in the server reads the template. CLAUDE.md gains a note saying
  so (P9-02). Other free text (`alias`, `description`, `error`, notification
  messages) is still withheld and counted. **Rejected:** grammar only, which
  is blind to templated logic; types and ids only, which can't see the
  threshold.

- [x] **D-09-3 — Two new tools, `get_automation_logic` and `get_automation_trace`** — owner, 2026-10-04 (F-53)
  The catalog goes from 21 to 23 tools. Architecture doc §9, README and the
  catalog drift tests move with them. Both are `ClassNormalRead`, admin-gated,
  and degrade to `unsupported` with F-11's reason for a non-admin principal.
  **Rejected:** an optional `logic` field on `get_automation` plus an optional
  `run_id` on `get_automation_traces`. That keeps the catalog at 21, but
  mixes a cheap summary with a large payload in one tool, and the owner prefers
  the cleaner split.

- [x] **D-09-4 — Under `deny`, a PRIVATE entity's id is masked; the element stays** — owner, 2026-10-04 (F-53)
  A trigger, condition, action or step that names a PRIVATE entity keeps its
  kind and typed values (threshold, time). The id is replaced by a
  `withheld` marker and counted. **Rejected:** dropping the whole element, which
  is safer but hides the threshold that is usually the answer. The risk the
  owner accepted: a value can itself be revealing (a zone radius, a presence
  time).

- [x] **D-09-5 — Under `deny`, template text is withheld entirely** — owner, 2026-10-04 (F-53)
  A template can name a PRIVATE entity inside its text (`states('person.x')`,
  or indirectly through a variable), and masking ids in it reliably would mean
  parsing Jinja. Under `deny` templates are counted only, as today. Under
  `allow` they ship per D-09-2. **Rejected:** string-searching the template for
  PRIVATE ids, which misses indirect references. Always returning templates,
  which leaks PRIVATE ids under `deny`.

## Tasks

- [x] **`P9-01` · Logic mapper: config → typed logic** 🧠 (D-09-2, D-09-4)
  New `internal/model/automation_logic.go`: `AutomationLogic{Triggers, Conditions, Actions []LogicNode}` with
  `LogicNode{Kind string; Path string; Values []TypedValue; Children []LogicNode; Templates []Template;
  Withheld int}`. `TypedValue{Key, Kind, Value}` holds Kind ∈ number/duration/time/entity/device/area/service/
  bool/token. `Template{Key, Text, Truncated}`. New `internal/ha/automation_logic.go` `MapAutomationLogic`
  walks both schema forms, recursing into `choose`/`if`/`then`/`else`/`sequence`/`repeat`/`parallel`, with a
  node cap and a depth cap (named constants). The mapper does no privacy work. The tool applies D-09-4/D-09-5.
  **DoD:** written red first, against invented fixtures. (1) Plural and singular forms map to the same nodes.
  (2) `numeric_state` `above: 26` → `TypedValue{above, number, 26}`; `hvac_mode: cool` → token; `at: "22:00"`
  → time. (3) A template string → `Templates`, never `Values`. (4) Free text (`alias`, a notify `message`) and
  a key outside the token grammar → `Withheld` only; no fixture string appears in the output (assert by
  marshalling and searching). (5) Prompt-like text in a non-template field never appears. In a template it
  appears only under `Templates`. (6) Nested `choose` deeper than the cap → `truncated`, no panic. (7) A
  malformed body (wrong types, oversized Unicode) maps partial, no panic. `make check` green.

- [x] **`P9-02` · Tool `get_automation_logic`** (D-09-3, D-09-4, D-09-5)
  New `internal/mcp/automation_logic_tools.go` and a catalog row (`ClassNormalRead`). Input: `entity_id` only,
  validated by the automation id grammar. Reads `automation/config` through the existing admin-gated reader and
  applies the profile: under `deny`, a PRIVATE entity id is masked and counted (D-09-4) and templates are
  counted only (D-09-5). Response carries provenance, `partial`/`truncated`/`unsupported`. The description
  states that `untrusted_template` is HA-authored data. Moves with it: catalog count 22, architecture doc §9
  row, README list, and a CLAUDE.md rule 6 note on D-09-2.
  **DoD:** written red first. (1) Parity: schema accepts no free-form route/command/path/query. (2) Under
  `deny`, a PRIVATE id never appears, its threshold does, the template text never appears and the template
  count does. (3) Under `allow`, the template is returned in `untrusted_template`. (4) A non-admin principal
  → `unsupported` with F-11's reason, not an empty logic. (5) Not found → `ErrNotFound`, distinct from
  unsupported. (6) The response byte cap → `truncated`. (7) Token never in response. `make check` green.

- [ ] **`P9-03` · Trace-run mapper: `trace/get` → typed steps** 🧠 (D-09-1, D-09-2, F-12) `blocked:F-54`
  Re-add `trace/get` to `internal/ha/gateway.go`'s allow-list with its caller (D-08-3), keyed like
  `trace/list` by config id (D-08-18). A new `CoreReader.AutomationTraceRun(ctx, entityID, runID)`. New
  `internal/ha/trace_steps.go` `MapAutomationTraceRun` drops `changed_variables`, `context` and every state
  object before mapping. It keeps run state, `script_execution`, `last_step`, timestamps and per-path steps
  with their result scalars by the D-09-2 grammar. Error text is withheld and counted. Reuses P9-01's
  `TypedValue`.
  **DoD:** written red first, on `automation_trace_get.json` and `automation_trace_secrets.json`. (1) No
  value from any `changed_variables`/`from_state`/`to_state`/`context` appears in the output (assert by
  marshalling and searching for every leaf string of those subtrees: friendly names, coordinates, user id).
  (2) `condition/0` with `result: false` maps to a step with `{result, bool, false}`. (3) A path outside the
  grammar is dropped and counted. (4) Gateway: `trace/get` is allowed, and an unlisted `trace/*` command is
  still denied before transmission. (5) Step cap → `truncated`. `make check` green.

- [ ] **`P9-04` · Tool `get_automation_trace`** (D-09-3, D-09-4) `blocked:P9-03`
  New `internal/mcp/automation_trace_tools.go` and a catalog row (`ClassNormalRead`). Input: `entity_id` and
  `run_id`, each validated by grammar (run id: the ULID/opaque-token grammar `get_automation_traces` already
  emits). Under `deny`, a PRIVATE id in a step result is masked (D-09-4). Moves with it: catalog count 23,
  doc §9, README.
  **DoD:** written red first. (1) Parity: no free-form parameter, and `run_id` rejects `../`, spaces and
  over-length values. (2) A run id that doesn't belong to the automation → `ErrNotFound`. (3) Non-admin →
  `unsupported` with reason. (4) Under `deny`, a masked id plus a visible result. (5) Token never in
  response. `make check` green.

- [ ] **`P9-05` · Observe on the Pi, measure, ship v1.1** `live-verify` `blocked:P9-04`
  Deploy to the Pi. Ask a client "why does the air conditioning automation (not) turn on" and record whether
  the answer cites a threshold or a failed condition. Measure `trace/get` bytes and latency on the longest
  automation, and set or confirm the step and node caps from that. Bump `addon/config.yaml` to `1.1.0`, with
  the README drift test following. Evidence in `docs/research/2026-10-xx-automation-logic-on-pi.md`, with ids
  and values withheld.
  **DoD:** the research note exists with measured sizes; caps are constants citing it; both tools return
  non-empty logic/steps on the Pi under `allow` and the masked form under `deny`; `make check` green.
