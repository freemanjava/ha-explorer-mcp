# Home Assistant trace step paths vs. the logic mapper's node paths (F-54)

### 2026-10-05 · Do `P9-01`'s `LogicNode.Path` strings equal the step keys `trace/get` reports?

**Kind:** world-discoverable. HA Core's source builds the path strings, so reading it settles the question. The owner can't answer it.
**Method:** read Home Assistant Core source at tag `2026.9.4` (the current release, the only one supported per phase 00) and at `dev` `df45506` (2026-10-05). I compared every `trace_path(` / `async_trace_path(` / `trigger_path =` line in the three files below: they are identical in both. The files are `homeassistant/helpers/trace.py` (`trace_path_get` joins the path stack with `/`), `helpers/script.py` (`_ScriptRun`), `helpers/condition.py`, `components/automation/__init__.py` (`async_trigger`) and `helpers/config_validation.py` (`_parallel_sequence_action`). I compared them against `internal/ha/automation_logic.go` (`logicWalker.section/action/option/condition/repeat`) and the expectations in `internal/ha/automation_logic_test.go` (`TestMapAutomationLogic_NestedStructures_ChildrenWithTracePaths`).
**Found:**

| construct | HA pushes (source) | HA step key | mapper path | match |
|---|---|---|---|---|
| trigger | `trigger/{idx}`, or `trigger` with no idx (automation `async_trigger`) | `trigger/N` | `trigger/N` | yes |
| top-level condition | `["condition", i]` (`ConditionsChecker`, used by `_async_process_if`) | `condition/N` | `condition/N` | yes |
| top-level action | `"action"` then `str(self._step)` | `action/N` | `action/N` | yes |
| and/or/not children | `["conditions", i]` | `…/conditions/J` | `…/conditions/J` | yes |
| `choose` option conditions | `choose`, `str(idx)`, `_test_conditions(…,"conditions")` → `conditions`, `str(j)` | `…/choose/I/conditions/J` | same | yes |
| `choose` option sequence | `choose`, `str(idx)`, `sequence`, step | `…/choose/I/sequence/J` | same | yes |
| `default` | `["default"]`, step | `…/default/J` | same | yes |
| `if` conditions | `if`, then `_test_conditions(…,"if","condition")` → `condition`, `str(j)` | `…/if/condition/J` | same | yes |
| `then` / `else` | `then` / `else`, step | `…/then/J`, `…/else/J` | same | yes |
| `repeat` sequence | `@async_trace_path("repeat")`, `sequence`, step | `…/repeat/sequence/J` | same | yes |
| `repeat` `while` / `until` | `repeat`, `_test_conditions(…,"while"/"until")`, `str(j)` | `…/repeat/while/J`, `…/repeat/until/J` | same | yes |
| `sequence` action | `@async_trace_path("sequence")`, step | `…/sequence/J` | same | yes |
| `parallel` branch given as `{sequence: [...]}` | `@async_trace_path("parallel")`, `[str(idx), "sequence"]`, step | `…/parallel/I/sequence/J` | branch node `…/parallel/I` (no step of its own), children `…/parallel/I/sequence/J` | yes |
| `parallel` branch given as a **bare action** | config validation wraps it as `{sequence: [action]}` (`_parallel_sequence_action`), so the same push applies | `…/parallel/I/sequence/0` | `…/parallel/I` | **no** |
| state / numeric_state with several `entity_id`s | `["entity_id", i]` plus a `trace_condition` per entity | `…/entity_id/I` sub-steps under the condition's key | no node | no node, by design |
| `wait_for_trigger` triggers | no `trace_path` push; triggers are not traced | none | `…/wait_for_trigger/J` | never joined (harmless) |

`automation/config` returns `raw_config`, which is the config *before* validation. So the wrapping of a bare parallel branch is invisible in the payload the mapper reads, and the mapper has to apply it itself.

**Not established:** this was not observed on a live `trace/get`. No token reaches the agent, and the only captured trace fixture is a simple automation. The reading relies on the `trace_path` stack being the only source of step keys, which `trace_path_get` and every `TraceElement(…, path)` call site confirm (`action_trace_append`, `condition_trace_append`, the automation's trigger element). A nested-automation capture through `cmd/spike` would confirm it end to end.
**Means:** F-54 is answered. Every shape matches except one: a bare action inside `parallel` gets `…/parallel/I` from the mapper, but HA reports `…/parallel/I/sequence/0`. The current test asserts the wrong value (`service@action/3/parallel/0`). If this is not fixed, `P9-03`'s step-to-node join misses every step under a bare parallel branch. That is a defect in `P9-01`'s mapper, filed as F-57. `P9-03`'s join must also handle two cases. First, the `…/entity_id/I` sub-steps, which belong to their parent condition and are not "outside the grammar". Second, a bare `trigger` key, which appears when a run has no trigger idx (a manual run).
