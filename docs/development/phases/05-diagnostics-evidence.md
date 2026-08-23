# Phase 05 — Diagnostics & Evidence Engine

**Milestone:** M2 (roadmap "Phase 2 — Diagnostics") · **Target version:** v1.0

> 🧠 **Stronger model recommended throughout.** This is the phase that
> differentiates HA Inspector from a thin API wrapper, and the one where a
> plausible-but-wrong design (correlation presented as causation, a health score
> nobody can justify) produces confidently misleading output.

## Goal

Composite, deterministic health analysis over entities, devices, integrations
and automations, returning **evidence** in the doc §12.2 shape — observation,
source, period, confidence, inference — with fact, inference and recommendation
kept structurally distinct (ADR-010). The agent gets ranked hypotheses with the
evidence behind each and an explicit list of what evidence is *missing*, not a
claimed root cause.

Closing this phase means the doc §21 criterion "at least three end-to-end
investigations produce evidence-backed ranked hypotheses" is met, including the
two workflows in doc §13.

## Depends On

Phase 04. Health analysis composes the statistics built there; building it
earlier means inventing the metrics twice.

## Add Under

```text
internal/analysis/   # correlation.go, entity_health.go, integration_health.go
internal/model/      # evidence.go, health.go
```

## Design Notes

- **FACT ≠ INFERENCE ≠ RECOMMENDATION**, structurally, in the response type — not
  as prose an LLM is asked to keep straight. An outage correlation is evidence,
  never an established cause.
- **Confidence must be derived from something.** A confidence field the code
  assigns by feel is worse than no field, because it launders a guess as a
  measurement. Say what the level means in terms of sample size, period covered
  and source reliability.
- **`missing_evidence` is a first-class output** (Appendix A.3). What could not
  be observed — because Supervisor access was absent, because traces are
  unsupported, because the recorder does not go back far enough — is exactly what
  keeps the agent from over-concluding, and it names the next diagnostic action.
- **A health score is optional and must be explainable.** If a single number
  cannot be traced back to named observations, ship the observations without it.
- Degrading a source reduces confidence; it does not break the analysis (doc §3.2).

## Tasks

*Not yet broken down.* Task boxes are written by `devflow plan` once Phase 04
closes — the shape of the correlation engine depends on what the statistics
layer actually produces and on what Phase 00 established about trace and
Supervisor availability. Candidate scope, from the architecture doc:

- The evidence model and its serialization (doc §12.2, Appendix A.3).
- `analyze_entity_health` — composite entity analysis.
- `analyze_integration_health` — config-entry state, unavailable ratio, repairs,
  shared outage clusters.
- Cross-entity outage clustering by time and by device/parent topology.
- Device and automation health composition.
- The doc §13.1 and §13.2 investigation workflows, end to end, as integration
  tests with a fixture installation.
- **A documented degraded mode for §13.1** when traces are unavailable to the
  App's principal (F-11): `last_triggered` + `logbook/get_events` +
  `context_id` correlation, with the missing traces named in
  `missing_evidence` and the confidence lowered accordingly — a branch of the
  workflow, not a rewrite discovered here.

## Decisions

- [ ] **`needs-decision` — Zigbee/mesh metric normalization**
  Q9. Whether LQI/RSSI/topology can be normalized across ZHA and Zigbee2MQTT
  without integration-specific plugins determines whether this phase needs a
  diagnostic plugin architecture or a flat analyzer. Do not answer from the doc:
  this is reality's to reveal against the owner's actual Zigbee stack — run
  `devflow verify` before it is planned, not after.

## Phase Definition of Done

- `analyze_entity_health` and `analyze_integration_health` return evidence in the
  doc §12.2 shape with fact and inference structurally separated.
- Three end-to-end investigations produce ranked hypotheses backed by cited
  evidence and an explicit `missing_evidence` list.
- No analysis path presents a correlation as a cause — asserted by test over the
  response types, not left to review.
- Still strictly read-only. `make check` is green.
