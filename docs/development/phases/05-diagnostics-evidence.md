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

- [x] **`P5-01` · Verify mesh/Zigbee metric normalization (F-6, Q9)** —
  `needs-verify` · **done 2026-09-05** →
  `docs/research/2026-09-05-zigbee-mesh-metric-normalization.md`, decided as
  **D-05-5** (flat analyzer + hint table). Raised **F-27**.
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

- [x] **`P5-02` · Evidence, hypothesis and missing-evidence model** — 🧠 ·
  **done 2026-10-03**
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

- [x] **`P5-03` · Derived confidence** — 🧠 · **done 2026-10-03**
  Built as `ConfidenceFor(cited ...model.Evidence)` — the three D-05-2 inputs
  read off each cited `Evidence` (`SampleSize`, `Coverage`, `Degraded`, the
  fields `P5-02` added for this), so a multi-citation `Hypothesis` gets one
  level: the **weakest** of its citations'. Ladder per Evidence: high ≥20
  samples and ≥0.9 coverage; medium ≥5 and ≥0.5; else low; `Degraded` then
  demotes one step. Thresholds are named, explained defaults, not
  measurements — `P5-10` is where they get revisited.
  `internal/analysis/confidence.go`: one exported `ConfidenceFor` that maps
  sample size, period coverage and source reliability to a confidence level,
  per D-05-2. No other code produces a confidence value.
  **DoD:** table-driven tests over every boundary of the ladder, including the
  degraded-source and under-covered-period paths; a test asserts confidence
  drops (never rises) when coverage falls or a source degrades; and a
  package-scanning test in the spirit of `deps_test.go` asserts no confidence
  literal is assigned outside `confidence.go`.

- [x] **`P5-04` · Cross-entity outage clustering** — 🧠
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
  **Done 2026-10-03.** `ClusterOutages` sorts all windows and sweeps once
  (`outageClusterTolerance` = 2 min); a one-entity group is not a cluster.
  **F-27 settled:** a shared `via_device` is *withheld* — listed in
  `Withheld`, not `Shared` — when, in each member's config entry, every
  device but the parent names it (the coordinator star). It then says no more
  than the shared config entry, which stays annotated. A parent of only part
  of its entry is named. The rule reads registry structure only, never an
  integration name. **Also decided here:** a window both `TruncatedStart` and
  `OpenEnded` (down for the whole observed period) overlaps everything by
  construction, so it is reported in `UnavailableThroughout`, never chained.

- [x] **`P5-05` · `analyze_entity_health`** — `blocked:P5-02,P5-03`
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

- [x] **`P5-06` · `analyze_integration_health`**
  Config-entry setup state, entity/device counts and unavailable ratio, open
  repairs for the integration, and the P5-04 outage clusters restricted to its
  entities.
  **DoD:** same four parity assertions; cluster evidence is cited by the
  hypotheses that rest on it, and a hypothesis with no surviving evidence is
  absent rather than present with low confidence; `missing_evidence` names the
  Supervisor-derived evidence when Supervisor is absent (doc §3.2 degradation),
  and the call still answers.

**Re-planned 2026-10-03 (F-30).** `P5-07` as first written presumed an
automation hypothesis producer and a dependency source; neither existed. It is
now three building boxes — `P5-11` (dependencies), `P5-12` (analysis),
`P5-13` (tool) — plus `P5-07` reduced to the end-to-end test it always named.
Decided as **D-05-6** and **D-05-7** below.

- [x] **`P5-11` · Automation dependency extraction** — per D-05-7
  Home: `internal/ha/mapping.go`, beside `MapAutomation` — the only unit that
  sees the raw `automation/config` body, so the body never crosses into
  `internal/model`. `model.Automation` gains, additively, `DependsOn` (typed
  `EntityID`/`DeviceID`/area ids, de-duplicated, sorted), `DependsTruncated`
  and `UnextractedRefs` (a count, never the text: a template or any value under
  a dependency key that fails the id grammar). Resolution of a device or area to
  its entities is **not** the mapper's job — it happens where the registry is
  read (`P5-13`).
  **DoD:** fixtures over both config syntaxes HA accepts (`trigger`/`triggers`,
  `service`/`action`, `platform`/`trigger` keys, bare object vs. list) and
  nesting (`choose`, `if`/`then`/`else`, `repeat`, `parallel`, `sequence`,
  `target`, `data`); a template in `value_template` and a templated
  `entity_id` are counted in `UnextractedRefs`, never extracted; a
  prompt-like string under `entity_id` is rejected by the grammar (Appendix B:
  attributes containing prompt-like text); more than
  `maxAutomationDependencies` (a named constant) sets `DependsTruncated`; a
  test asserts `get_automation`'s serialized response carries ids only — no
  trigger/condition/action body text; under the deny profile a PRIVATE
  dependency is withheld and counted, as `P4-05` does for `find_*`.

- [x] **`P5-12` · Automation run analysis** — 🧠
  Home: new `internal/analysis/automation_health.go`,
  `AnalyzeAutomationHealth(AutomationHealthInput)` — the third analyzer,
  same shape as `entity_health.go`/`integration_health.go` (reuses the P5-06
  `ledger`/`healthWindow`); not grown into either of them, which analyze a
  different subject. Input arrives already read: the automation, its trace
  summaries **or** the F-11 fallback (`last_triggered` + logbook events by
  `context_id`), each dependency's history, repairs. Evidence: run outcomes
  (stopped at a condition, error, aborted), dependency unavailable/stale
  windows, and the overlap of a failed run with a dependency window (within
  `outageClusterTolerance`, D-05-3 — overlap, never "caused by").
  Fallback-derived evidence is `Degraded`, so `ConfidenceFor` demotes it; no
  confidence is set anywhere else (D-05-2).
  **DoD:** a failed run inside a dependency's unavailable window yields a
  hypothesis citing both pieces of evidence, ranked above one without the
  overlap; the same scenario through the fallback has **strictly lower**
  confidence — asserted as a comparison, not a fixed level; traces absent,
  `UnextractedRefs > 0`, a truncated dependency list and an unread dependency
  history each appear in `missing_evidence` with why; a hypothesis with no
  surviving evidence is absent; no `cause` field (the D-05-1 reflection test
  covers the new types).

- [ ] **`P5-13` · `analyze_automation_health`**
  Home: new `internal/mcp/automation_health_tools.go` plus one catalog row at
  `ClassComposite` (open/closed: a new file and a table entry). Reads
  `automation/config` (dependencies), `trace/list` or the fallback, registry
  (device/area → entities), history for at most the `P5-06` cap of
  dependencies (the rest named in `missing_evidence`), repairs; then
  `AnalyzeAutomationHealth`.
  **DoD:** the parity rule's four, each asserted; a non-admin principal still
  answers (fallback branch, `missing_evidence` names `automation/config` and
  `trace/list`); the catalog test moves from twenty to **twenty-one**, and in
  the same change doc §9 gains the row and the phase 03 "full twenty"
  decision record gains a one-line amendment pointing at D-05-6.

- [ ] **`P5-07` · Investigation 1 — doc §13.1, end to end** — `blocked:P5-13`
  An integration-level test walking `get_automation` → `get_automation_traces`
  → dependency history/statistics → repairs → `analyze_automation_health`,
  against a fixture installation.
  **DoD:** the happy path produces ranked hypotheses each citing evidence; the
  **degraded branch** (F-11 — traces unavailable to the principal) produces
  hypotheses from `last_triggered` + logbook + `context_id` correlation, names
  the absent traces in `missing_evidence`, and carries strictly lower
  confidence than the same scenario with traces present — asserted as a
  comparison, not as a fixed level. Closes F-30.

- [ ] **`P5-08` · Investigation 2 — doc §13.2, end to end**
  Overview/health → integration health → `find_unavailable_entities` → P5-04
  clustering by time and parent topology → coordinator/parent evidence →
  ranked hypotheses, with the privileged host evidence (USB resets, dmesg —
  ADR-012, never this binary's) named in `missing_evidence`.
  **DoD:** the mesh-metric evidence is read the way `P5-01` established;
  a fixture where two devices share a parent produces a topology-annotated
  cluster, and one where they do not produces the same time cluster *without*
  the topology claim. **F-27 (settled in `P5-04`):** the shared-parent
  fixture must be a parent of *part* of its config entry; a coordinator star
  must yield the time cluster with `via_device` in `Withheld`, not `Shared`.

- [ ] **`P5-09` · Investigation 3 — correlated mass unavailability**
  The third of doc §21's three: a batch of entities goes unavailable together;
  the chain is `find_unavailable_entities` → clustering → shared config entry →
  `analyze_integration_health` → repairs, ending in ranked hypotheses that
  distinguish "one integration failed" from "HA restarted".
  **DoD:** both fixtures are distinguished by evidence, not by a heuristic
  name match; the doc §21 criterion ("at least three end-to-end investigations
  produce evidence-backed ranked hypotheses") is asserted by a test naming all
  three, so the criterion cannot silently regress.
  **Before building (F-31, F-28):** nothing in `internal/analysis` today
  observes an HA restart, so "HA restarted" has no evidence to rest on yet.
  `P5-10`'s live run checks whether the logbook's start/stop events are
  readable and observes F-28; this box's design starts from that report.

- [ ] **`P5-10` · Measure the composite budget and re-class `find_stale_entities`
  (F-26)** — `needs-verify`
  One measurement session on a real installation covering both unmeasured
  request budgets at once: `find_stale_entities` at `ClassNormalRead`, and the
  three `analyze_*` tools at `ClassComposite` (`P5-13` added the third).
  The same `cmd/spike` session observes F-28 (long outages chaining clusters)
  and F-31 (is an HA restart visible in logbook/get_events). Record actual HA requests, bytes
  and wall time per call at realistic installation width.
  **DoD:** a dated report in `docs/research/` with the measured numbers, and a
  decision record here that either keeps the current classes with the
  measurement behind them or re-classes with it — never a class changed
  without the report. Deliberately last: measuring composite cost requires the
  composite tools to exist, and one session covers both halves of the question.

## Decisions

- [x] **D-05-5 — Mesh metrics are read by a flat analyzer over a name/
  `device_class` hint table, not a per-integration plugin seam**
  Q9, F-6, decided with `P5-01`. **Evidence:**
  `docs/research/2026-09-05-zigbee-mesh-metric-normalization.md` — the owner's
  live Zigbee2MQTT installation plus pinned-source reading of `zha` 2.1.0 and
  core 2026.8.3 for the ZHA half, which the owner's installation cannot answer
  (it has zero `zha` devices).

  The axis that would have forced a plugin — *where the value lives* — does not
  disagree: both integrations expose link quality and signal strength as
  ordinary **entities in the state machine**, with recorder history, not behind
  an integration-private API. ZHA's `LQISensor`/`RSSISensor` are registered for
  every device; Zigbee2MQTT's `linkquality` sensors were observed live. What
  disagrees is only data: the entity **name** (`_lqi` vs `_linkquality`),
  whether a **semantic marker** exists (RSSI carries `device_class:
  signal_strength`; LQI deliberately carries none on either side), and whether
  the entity is **enabled** (ZHA ships both `entity_registry_enabled_default =
  False`; Zigbee2MQTT has no RSSI equivalent at all).

  Decided: one flat analyzer resolves the metric by `device_class` first and a
  named hint table (`lqi`, `linkquality`, `link_quality`, `rssi`,
  `signal_strength`) second; the hint table is a constant with this research
  file cited beside it. A metric that is absent — ZHA's disabled diagnostics,
  Zigbee2MQTT's missing RSSI — is reported as `MissingEvidence` (D-05-1)
  naming *why*, never as a zero and never as a reason to fail the call.

  **Rejected:** a per-integration diagnostic plugin seam (an interface, a
  registry and two implementations to absorb what is five strings and one
  `device_class` lookup — speculative generality, and it would have to be
  written against ZHA with no ZHA installation to test it on); keying off the
  entity registry's `platform` field to branch behavior (it makes the analyzer
  wrong for the third Zigbee integration nobody has installed yet, and CLAUDE.md
  rule 6 already says HA-supplied strings are data, not control flow);
  reading ZHA's richer `zha/devices` WebSocket payload (it carries `lqi`/`rssi`
  even when the entities are disabled, but it is ZHA-only, admin-gated, and
  outside the gateway allow-list — a genuinely per-integration path, which is
  the thing being rejected).

  **Consequence outside this decision:** `via_device_id` turned out to be a
  coordinator/bridge **star** on both integrations (ZHA sets it to the
  coordinator explicitly; the owner's 27-of-28 count is the same signature), so
  "these cluster members share a `via_device` parent" is vacuous for Zigbee —
  every device in the network shares it. That is **F-27**, against D-05-3 and
  `P5-04`/`P5-08`, not a revision of this entry.

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

- [x] **D-05-6 — Automation hypotheses ship as a twenty-first tool,
  `analyze_automation_health`** — owner, 2026-10-03 (F-30)
  Doc §13.1 ends in ranked hypotheses, and nothing produced them: the two
  existing analyzers never see traces, `last_triggered` or logbook. Decided: a
  third composite tool at `ClassComposite`, built like the other two
  (`P5-12` analysis, `P5-13` tool). This amends phase 03's "full twenty"
  decision to twenty-one; the catalog test and doc §9 move with it in
  `P5-13`. **Rejected:** hypotheses inside `get_automation_traces`' response
  (a read tool emitting inference mixes fact and inference in one tool's
  output — ADR-010's separation, eroded at the tool level); composing them only
  inside the e2e test (an analyzer no tool calls is dead code, and §13.1's
  answer would never reach a real agent).

- [x] **D-05-7 — Dependencies are extracted by our mapper from
  `automation/config`, ids only; `search/related` is the recorded fallback** —
  owner, 2026-10-03 (F-30)
  §13.1's "identify trigger / condition / action dependencies" needs ids to
  walk to history. Decided: `internal/ha`'s mapper collects the values under
  `entity_id`, `device_id` and `area_id` keys anywhere in the config body,
  each validated by the id grammar, de-duplicated and capped; templates and
  grammar failures are counted, never extracted or echoed. Nothing else of the
  body leaves `internal/ha`. **Rule 6 reading, made explicit:** these values
  are used only as lookup keys — the same use `via_device_id` already has in
  `P5-04` — and no behavior branches on their content; a value that is not a
  valid id is data that failed validation, not an instruction. The command is
  already allow-listed and admin-gated, so the surface does not grow.
  **Rejected for now — recorded fallback (F-32, `defer`):** HA's
  `search/related`, which lets HA do the parsing and returns devices, areas and
  integrations ready-made. Its admin gate, response shape and cost on a Pi are
  unverified; it adds an allow-list command F-25 already counts against us; it
  misses templates just as our mapper does; and "related" is likely wider than
  "depends on" (neighbour entities of the same device or area). Reopen it if
  real automations show our mapper missing constructions. **Also rejected:** no
  dependencies in v1 (every §13.1 answer would carry "dependencies: not
  checked", which is the diagnostic's whole value missing).

## Phase Definition of Done

- `analyze_entity_health` and `analyze_integration_health` return evidence in the
  doc §12.2 shape with fact and inference structurally separated.
- Three end-to-end investigations produce ranked hypotheses backed by cited
  evidence and an explicit `missing_evidence` list.
- No analysis path presents a correlation as a cause — asserted by test over the
  response types, not left to review.
- Still strictly read-only. `make check` is green.
