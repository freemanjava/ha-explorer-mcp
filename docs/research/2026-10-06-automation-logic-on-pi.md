# Automation logic and trace steps on the Pi — 2026-10-06

Evidence for `P9-05` (phase 09). Ids, device ids, values and the owner's
installation details are withheld by design; only shapes, sizes and timings.

## What was asked

1. Does `get_automation_logic` / `get_automation_trace` answer "why does the air
   conditioning automation (not) turn on" from a real installation?
2. What do they cost on the Pi, and are the node/step caps right?
3. Do the three privacy profiles behave as D-09-5 / D-09-6 say?

## How it was observed

App `1.1.0-rc1` (image published by the manual `Release App image` run on
`feat/P9-05`, then merged), Home Assistant Core `2026.9.4`, Raspberry Pi
(aarch64), 41 automations. An MCP client called both tools over the App's
HTTP transport. Sizes and times are the App's own audit lines
(`ha_requests`, `duration_ms`, `result_bytes`), read from the App log. The
privacy profile was switched through the App options (restart each time) and
restored to `mask` afterwards.

## Result

**The AC question is answered with a cited threshold and a failed condition.**
For a temperature-triggered turn-on automation: the logic view reports a numeric
trigger and condition `above: 27` on the room temperature sensor, a state
condition on a zone flag, and the `turn_on` action. A stopped run
(`failed_conditions`) reports `condition/1` false with the observed value below
the threshold (25.72 against 27); a run that fired reports the value above it
(27.02), all three conditions true, then the action step.

**Cost** (`mask`, unless noted):

| call | `ha_requests` | `duration_ms` | `result_bytes` |
|---|--:|--:|--:|
| logic, 3 conditions, 1 action | 1 | 7 | 4216 |
| logic, two `choose` options (largest seen) | 1 | 8 | 5108 |
| logic, `choose` plus delay | 1 | 6 | 4276 |
| logic, time triggers, `choose` (`deny`) | 1 | 4 | 3858 |
| logic, one template (`allow`/`deny`) | 1 | 3 | 1620 |
| trace, run stopped at a condition | 2 | 8 | 2872 |
| trace, run that fired | 2 | 8 | 4152 |

20 of the 41 automations were mapped; none was `Partial` or `Truncated`. The
largest has two `choose` options, about a dozen nodes. This installation has no
automation near either cap.

**Privacy profiles.**

- `allow`: the one automation with a template returns its text in
  `untrusted_template` (a date-formatting expression).
- `mask` and `deny`: the same automation returns no template text, only
  `TemplatesWithheld: 1`.
- `deny` masking of a PRIVATE entity id was **not observed**: no automation here
  references a PRIVATE entity. It is covered by unit tests only, not live.

## What this means for the plan

- `maxLogicNodes = 500` and `maxTraceSteps = 500` are confirmed, not tuned: real
  automations use tens of nodes, and results stay near 5 KB at 3–8 ms. The
  measurements do not show where the caps bind, so there is no basis to move them.
- Latency is a non-issue: `trace/get` costs two Core requests and about 8 ms.
- The live PRIVATE-masking check stays open as an untested live case.
