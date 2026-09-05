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

Ordered by dependency. `P5-01` is first deliberately: F-6 asks whether mesh
metrics normalize across integrations, and that answer decides whether the
correlation and integration-health analyzers need a per-integration seam or
stay flat. Designing them first and learning the answer afterwards is how a
structural decision gets made on an unverified premise.

- [ ] **`P5-01` · Verify mesh/Zigbee metric normalization (F-6, Q9)** —
  `needs-verify`
  A `devflow verify` cycle, not an implementation one. Establish, against the
  owner's actual Zigbee stack via `cmd/spike`, whether LQI/RSSI and parent
  topology are readable in a comparable shape regardless of which Zigbee
  integration is in use (ZHA, Zigbee2MQTT, or both). Report field names, types
  and where each lives (entity attribute, device registry, diagnostic entity),
  never values.
  **DoD:** a dated report in `docs/research/` naming, per integration present:
  the entity/attribute carrying link quality, the one carrying signal strength,
  and how a parent/`via_device` relation is expressed — or stating plainly that
  one of them has no readable equivalent. The report answers the Decisions
  entry below, which is ticked in the same cycle with its record written.
  Legwork runs on the default model; if the two integrations disagree in a way
  that makes the flat-vs-plugin call contested, stop and recommend a fresh
  session on the stronger model rather than deciding mid-flight.

- [ ] **`P5-02` · Evidence, hypothesis and missing-evidence model** — 🧠
  Replace the unused `internal/model/evidence.go` stub with the full doc §12.2
  / Appendix A.3 shape as **distinct types**, per D-05-1: `Evidence` (a
  measured observation with source and period), `Hypothesis` (an inference
  citing evidence, carrying a derived confidence), `MissingEvidence` (what
  could not be observed and why), `NextAction` (a recommended diagnostic step),
  and the `HealthAnalysis` envelope that carries them plus `Provenance`.
  **DoD:** a test asserts no single type merges fact with inference — an
  `Evidence` value has no inference field and a `Hypothesis` has no field an
  agent could read as a measured fact; a `Hypothesis` with zero cited evidence
  is not constructible; and no type anywhere carries a field named `cause` or
  `root_cause` (asserted over the package by reflection, not by review).

- [ ] **`P5-03` · Derived confidence** — 🧠 `blocked:P5-02`
  `internal/analysis/confidence.go`: one exported `ConfidenceFor` that maps
  sample size, period coverage and source reliability to a confidence level,
  per D-05-2. No other code produces a confidence value.
  **DoD:** table-driven tests over every boundary of the ladder, including the
  degraded-source and under-covered-period paths; a test asserts confidence
  drops (never rises) when coverage falls or a source degrades; and a
  package-scanning test in the spirit of `deps_test.go` asserts no confidence
  literal is assigned outside `confidence.go`.

- [ ] **`P5-04` · Cross-entity outage clustering** — 🧠
  `blocked:P5-01,P5-02,P5-03`
  `internal/analysis/correlation.go`: group entities whose unavailable windows
  overlap within the D-05-3 tolerance into clusters, then annotate each cluster
  with what its members share — device, `via_device` parent, config entry,
  area. Output is `Evidence`, never a cause.
  **DoD:** deterministic output for a given input (asserted by repeated runs
  over a shuffled input); fixtures covering no overlap, one clean cluster,
  two clusters, and a coincidental overlap that must *not* become a shared-
  parent claim; cost stays linear in windows, not pairwise (asserted by a
  counting fake, not by benchmark); a test asserts a cluster's serialized form
  carries no causal field.

- [ ] **`P5-05` · `analyze_entity_health`** — `blocked:P5-02,P5-03`
  Compose P4-02 availability, P4-03 cadence, the entity's registry/device
  context, its integration's setup state and any related repairs into the
  Appendix A.3 shape. No `score` (D-05-4).
  **DoD:** returns `Evidence`, ranked `Hypothesis` values with confidence from
  `ConfidenceFor`, and a populated `missing_evidence` naming each source that
  could not be read and why; a degraded source lowers confidence and does not
  fail the call; the parity rule's four (typed input, budget class, provenance,
  no free-form parameter) each asserted; the privacy profile applies as it does
  in `get_entity_statistics`, with a PRIVATE entity under deny refused rather
  than partially analyzed.

- [ ] **`P5-06` · `analyze_integration_health`** — `blocked:P5-04,P5-05`
  Config-entry setup state, entity/device counts and unavailable ratio, open
  repairs for the integration, and the P5-04 outage clusters restricted to its
  entities.
  **DoD:** same four parity assertions; cluster evidence is cited by the
  hypotheses that rest on it, and a hypothesis with no surviving evidence is
  absent rather than present with low confidence; `missing_evidence` names the
  Supervisor-derived evidence when Supervisor is absent (doc §3.2 degradation),
  and the call still answers.

- [ ] **`P5-07` · Investigation 1 — doc §13.1, end to end** — `blocked:P5-05`
  An integration-level test walking `get_automation` → `get_automation_traces`
  → dependency history/statistics → repairs → correlated timestamps → ranked
  hypotheses, against a fixture installation.
  **DoD:** the happy path produces ranked hypotheses each citing evidence; the
  **degraded branch** (F-11 — traces unavailable to the principal) produces
  hypotheses from `last_triggered` + logbook + `context_id` correlation, names
  the absent traces in `missing_evidence`, and carries strictly lower
  confidence than the same scenario with traces present — asserted as a
  comparison, not as a fixed level.

- [ ] **`P5-08` · Investigation 2 — doc §13.2, end to end** — `blocked:P5-06`
  Overview/health → integration health → `find_unavailable_entities` → P5-04
  clustering by time and parent topology → coordinator/parent evidence →
  ranked hypotheses, with the privileged host evidence (USB resets, dmesg —
  ADR-012, never this binary's) named in `missing_evidence`.
  **DoD:** the mesh-metric evidence is read the way `P5-01` established;
  a fixture where two devices share a parent produces a topology-annotated
  cluster, and one where they do not produces the same time cluster *without*
  the topology claim.

- [ ] **`P5-09` · Investigation 3 — correlated mass unavailability** —
  `blocked:P5-06`
  The third of doc §21's three: a batch of entities goes unavailable together;
  the chain is `find_unavailable_entities` → clustering → shared config entry →
  `analyze_integration_health` → repairs, ending in ranked hypotheses that
  distinguish "one integration failed" from "HA restarted".
  **DoD:** both fixtures are distinguished by evidence, not by a heuristic
  name match; the doc §21 criterion ("at least three end-to-end investigations
  produce evidence-backed ranked hypotheses") is asserted by a test naming all
  three, so the criterion cannot silently regress.

- [ ] **`P5-10` · Measure the composite budget and re-class `find_stale_entities`
  (F-26)** — `needs-verify` `blocked:P5-06`
  One measurement session on a real installation covering both unmeasured
  request budgets at once: `find_stale_entities` at `ClassNormalRead`, and the
  two `analyze_*` tools at `ClassComposite`. Record actual HA requests, bytes
  and wall time per call at realistic installation width.
  **DoD:** a dated report in `docs/research/` with the measured numbers, and a
  decision record here that either keeps the current classes with the
  measurement behind them or re-classes with it — never a class changed
  without the report. Deliberately last: measuring composite cost requires the
  composite tools to exist, and one session covers both halves of the question.

## Decisions

- [ ] **`needs-verify` → decided with `P5-01` — Zigbee/mesh metric normalization**
  Q9, F-6. Whether LQI/RSSI and parent topology normalize across ZHA and
  Zigbee2MQTT decides whether this phase needs a per-integration diagnostic
  plugin seam or a flat analyzer. **Not answerable from the doc and not the
  owner's to choose** — it is reality's, so `P5-01` observes it first and this
  entry is ticked in the same cycle with the record written: the evidence, the
  flat-vs-plugin call, and the rejected alternative.

- [x] **D-05-1 — Fact, inference and recommendation are separate *types*, not
  separate fields**
  ADR-010 and this phase's first design note require the separation to survive
  contact with an LLM. A single `Evidence` struct with an `Inference` field —
  doc §12.2's sketch, and what the current stub carries — makes "correlation
  presented as cause" one assignment away, with nothing but review in the way.
  Decided: `Evidence`, `Hypothesis`, `MissingEvidence` and `NextAction` are
  distinct types; a `Hypothesis` cites evidence by reference and cannot be
  constructed citing none; no type in the tree carries a `cause` or
  `root_cause` field. **Rejected:** doc §12.2's literal one-object shape (kept
  as the serialized *rendering* of an `Evidence` plus the `Hypothesis` that
  cites it, so the doc's example still reads true); a single struct with a
  `kind` discriminator (an enum value is as easy to set wrongly as a prose
  field, and the compiler checks neither).

- [x] **D-05-2 — Confidence is computed by one function or it does not exist**
  A confidence a call site assigns by feel launders a guess as a measurement —
  worse than shipping no confidence at all. Decided: one
  `analysis.ConfidenceFor(sampleSize, coverage, source)` with a documented
  ladder, and its inputs are exactly the three things that can lower it: how
  many observations back the claim, how much of the requested period the
  recorder actually covered, and whether the source answered fully or degraded.
  Monotone by construction — coverage falling or a source degrading can only
  lower the result. **Rejected:** per-tool confidence heuristics (the same
  evidence would get different confidence from two tools, and neither would be
  explainable); a numeric 0–1 score (invites arithmetic on a level that has no
  arithmetic meaning); omitting confidence entirely (doc §12.2 requires it, and
  a derived one is defensible).

- [x] **D-05-3 — Outage clusters are overlap-with-tolerance, annotated after
  the fact**
  Decided: entities whose unavailable windows overlap, allowing a tolerance for
  clock and polling skew, form one cluster; the cluster is *then* annotated
  with what its members share (device, `via_device` parent, config entry,
  area). The tolerance is a named constant with its rationale beside it, and
  the sharing annotation is evidence about the cluster, never a claim that the
  shared thing caused it. **Rejected:** correlation coefficients over state
  series (a number nobody can trace back to named observations — D-05-2's
  objection, and this phase's design note); pairwise comparison of every entity
  against every other (quadratic on a Pi at installation width, for a result a
  sweep over sorted window edges gives linearly); clustering *by* shared parent
  first (it would find only the outages already suspected, and hide the
  cross-integration ones §13.2 is looking for).

- [x] **D-05-4 — No health score in v1**
  Appendix A.3 marks `score?` optional and this phase's design note sets the
  bar: a single number must trace back to named observations or it ships as
  observations without it. Nothing measured here weights against anything else
  in a way that survives an owner asking "why 72?". Decided: no `score` field
  in the v1 response. **Rejected:** a weighted composite (the weights would be
  the unexplainable part, and once emitted an agent would rank on it); a coarse
  good/degraded/bad verdict (the same problem, with the arithmetic hidden
  rather than absent). Revisit only if a scoring rule falls out of `P5-10`'s
  measurements, and only as an additive field.

## Phase Definition of Done

- `analyze_entity_health` and `analyze_integration_health` return evidence in the
  doc §12.2 shape with fact and inference structurally separated.
- Three end-to-end investigations produce ranked hypotheses backed by cited
  evidence and an explicit `missing_evidence` list.
- No analysis path presents a correlation as a cause — asserted by test over the
  response types, not left to review.
- Still strictly read-only. `make check` is green.
