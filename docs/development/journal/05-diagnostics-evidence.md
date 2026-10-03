# Journal — Phase 05 Diagnostics & Evidence Engine

Append-only. One entry per closed task, **at most ~5 lines**. Never read whole —
`NEXT.md` carries the last few; this file answers "why on earth is it like that"
months later.

What belongs here is the **surprise**: the environment quirk, the API that
ignores its own documented parameters, the test that had to be shaped oddly. What
changed is already in the diff and the commit message; why it is designed that
way belongs in the phase file's decision record. Only the surprise is
unrecoverable anywhere else — so if there was none, the entry is one line and
that is correct.

---

### 2026-09-05 · `P5-01`
Q9/F-6 answered: mesh metrics need a flat analyzer plus a name/`device_class`
hint table, not a per-integration plugin seam (D-05-5). `cmd/spike` gained
`probeMesh`; evidence in `docs/research/2026-09-05-zigbee-mesh-metric-normalization.md`.
**Surprise:** twice, the same trap. ZHA's LQI/RSSI look absent — they are real
entities (`LQISensor`/`RSSISensor`) that ship `entity_registry_enabled_default =
False`, so they never reach `get_states`; and `via_device_id` looks like parent
topology but is a coordinator star on *both* integrations, so "shares a parent"
is true of the entire Zigbee network (F-27). A first pass on the default model
concluded the opposite of both by grepping HA core's `zha/sensor.py`, which no
longer holds the entity classes at all — they moved to the `zha` library.
**Left open:** F-27 (vacuous shared-parent annotation) for `P5-04`/`P5-08`; no
live ZHA installation exists to confirm the source-read half.

### 2026-10-03 · P5-02
`internal/model/evidence.go`: `Evidence`, `Hypothesis` (unexported fields, `NewHypothesis` refuses zero citations), `MissingEvidence` with a typed `MissingReason` (incl. D-05-5's `entity_disabled` vs `not_exposed`), `NextAction`, `HealthAnalysis`; a source scan over `internal/` forbids any `cause`/`root_cause` field or json tag.
**Surprise:** "assert by reflection over the package" is not possible in Go — reflection cannot enumerate a package's types — so the no-cause check is a `go/parser` scan, which also catches json tags reflection on names alone would miss.
**Left open:** `Confidence` levels (low/medium/high) defined here so `Hypothesis` can hold one; the ladder mapping evidence onto them is `P5-03`'s. Citations are not checked to resolve within a `HealthAnalysis` — `P5-05`/`P5-06` own that.

### 2026-10-03 · P5-03
`internal/analysis/confidence.go`: `ConfidenceFor(cited ...model.Evidence)` — per-Evidence ladder (high ≥20 samples & ≥0.9 coverage, medium ≥5 & ≥0.5, else low), `Degraded` demotes one step, several citations take the weakest. A `go/parser` scan over `cmd/` and `internal/` refuses any `ConfidenceX` reference or `Confidence(...)` conversion outside `confidence.go` and `model/evidence.go`.
**Surprise:** D-05-2 wrote the signature as three scalars, but a `Hypothesis` cites *several* Evidence — taking `Evidence` values lets the one function also own the combining rule, which would otherwise have been a second, unscanned producer of confidence at every call site.
**Left open:** thresholds are explained defaults, not measurements; `P5-10` revisits them against a real recorder.

### 2026-10-03 · P5-04
`internal/analysis/correlation.go`: `ClusterOutages` — sort + one sweep over all outage windows (2-min tolerance), clusters of ≥2 entities, annotated afterwards with shared device/via_device/config entry/area, each cluster a citable `Evidence`. F-27 settled: a star parent goes to `Withheld`, not `Shared`.
**Surprise:** the vacuity rule needed no integration knowledge at all — "every other device of the entry names this parent" is pure registry structure, so it also catches Hue-style bridges. And a whole-period outage overlaps everything, so it had to leave the sweep (`UnavailableThroughout`) or one dead sensor would merge and strip every cluster.
**Left open:** partial long outages still chain (F-28); area/config-entry vacuity in a one-area or one-integration home is not handled.

### 2026-10-03 · P5-05
`analysis.AnalyzeEntityHealth` composes availability, cadence, registry/integration state and open repairs into Evidence, ranked Hypotheses (confidence only via `ConfidenceFor`) and MissingEvidence; `internal/mcp/entity_health_tools.go` reads each source independently, so a failed one becomes `missing_evidence` and not an error. No score.
**Surprise:** `cmd/server/main.go` never set `Options.History`, so `get_entity_history`, `get_entity_statistics` and `find_stale_entities` all answered "not implemented" in the shipped binary (F-29); wired here because this tool needs it. Snapshot evidence (setup state, repairs) has one sample, so the ladder caps any hypothesis citing it at low — accurate, not a bug.
**Left open:** `golangci-lint` not installed locally, so `make check` skipped lint; not observed against a live HA.


### 2026-10-03 · P5-06
`analysis.AnalyzeIntegrationHealth` + `internal/mcp/integration_health_tools.go`: setup state, inventory/unavailable ratio, domain repairs, outage clusters over the entry's entities, Supervisor resolution counts; Supervisor absent/refused/down is a named gap and the call still answers. Shared `ledger`, `healthWindow`, `HealthResponse` with P5-05.
**Surprise:** one integration can own hundreds of entities and history is one HA request each against a budget of 50, so clustering reads at most 25 (down-now first) and names the rest in `missing_evidence` — P5-10 must measure whether 25 is right.
**Left open:** `golangci-lint` still not installed; not observed against a live HA.

### 2026-10-03 · P5-11
`MapAutomation` walks the config body structurally (sorted keys, depth cap 32) and collects `entity_id`/`device_id`/`area_id` values by grammar; `get_automation` applies the deny profile to entities.
**Surprise:** a device trigger/condition's `entity_id` is a registry uuid, not an entity id — it fails the grammar and is counted unextracted, so such automations under-report dependencies (F-32's trigger evidence).

### 2026-10-03 · P5-12
`AnalyzeAutomationHealth`: trace outcomes (or degraded logbook runs), per-dependency outage + stale windows, run overlap within `outageClusterTolerance`; config/trace/extraction/history gaps each named in `missing_evidence`. Result wraps `HealthAnalysis` with `DependencyEvidence` (numbered evidence id → entity), since ids may not be HA-derived.
**Surprise:** trace coverage is only knowable from the oldest stored trace (HA keeps ~5), so a busy automation's run evidence covers a sliver of the period and lands `low` honestly. "Overlap ranked above no-overlap" holds by citation count only while the dependency history is at least as strong as the runs — the shared ladder ranks confidence first.
**Left open:** `P5-13` must render `DependencyEvidence` and resolve device/area deps to entities; `golangci-lint` still not installed.

### 2026-10-03 · P5-13
`analyze_automation_health` (`internal/mcp/automation_health_tools.go`): config → traces → logbook fallback only when traces are unread; device/area deps resolved through the cached registries; history capped at `maxClusterEntities`, with private (deny) and over-cap dependencies passed to the analysis as unread-with-reason so they are counted, never named. Catalog, doc §9 and phase 03 moved to twenty-one.
**Surprise:** a fallback-vs-traces confidence comparison is vacuous on a small fixture (both `low`); it needs ≥5 runs, a trace older than the period (trace coverage) and a period the 24h logbook window covers.
**Left open:** F-26 (composite cost) still unmeasured — `P5-10`; `golangci-lint` still not installed.

### 2026-10-03 · P5-07
`internal/mcp/investigation_test.go`: one server session walks `get_automation` → traces → history → `list_repairs` → `analyze_automation_health`; happy path cites existing evidence, traces-refused branch ranks strictly below it, fully non-admin names both gated sources and yields no hypothesis.
**Surprise:** config and traces are gated together, so the F-11 fallback can't produce hypotheses for a real non-admin principal (F-34).
**Left open:** F-34 (`defer`); `golangci-lint` still not installed.


### 2026-10-03 · P5-14
`model.ClusterAnnotation`/`ClusterTrait`/`TraitKind` (moved out of `analysis`); `HealthAnalysis.Clusters` filled by `AnalyzeIntegrationHealth`, rendered as `Clusters` in `HealthResponse` with `[]` for empty member/trait lists. `Evidence` untouched.
**Surprise:** responses carry no JSON tags, so the key is `Clusters` (PascalCase like `Evidence`), not the DoD's lowercase `clusters`; I kept the existing convention. `analyze_entity_health` now also emits `Clusters: []`.
**Left open:** `golangci-lint` still not installed.

### 2026-10-03 · P5-15
`analysis/mesh.go` (`ResolveMeshMetrics`, `MeshEvidence`, `IsMeshEntity`); `IntegrationHealthInput.MeshEvidence`; `readMesh` in the tool reads metric history for devices with an outage or a down entity, capped at `maxMeshMetricEntities` (10).
**Surprise:** `policy` classifies privacy by entity domain only, and mesh metrics are `sensor.*`, so the deny-profile path in `readMesh` cannot fire today; it is routed through `permittedEntities` so a future classifier change is honoured, but no test can trigger it.
**Left open:** a metric with no numeric reading is reported as `out_of_retention`, the nearest reason; `golangci-lint` still not installed.

### 2026-10-03 · P5-16
`addUnreachableEvidence` in `integration_health.go`: a `MissingPrivileged` host row plus a tool-less `NextAction` when a cluster exists; a `MissingNotExposed` neighbour-table row when `IntegrationHealthInput.MeshResolved` (set by `readMesh` when any metric entity resolved).
**Surprise:** "mesh metrics resolved" is not "mesh evidence exists" — a ZHA shape with both metrics disabled resolves entities but yields no evidence, so a new input flag was needed rather than `len(MeshEvidence)`.
**Left open:** `golangci-lint` still not installed.

### 2026-10-03 · P5-08
`investigation_test.go`: `walkInvestigation2` drives `find_unavailable_entities` → `analyze_integration_health` → `analyze_entity_health` over `layout`/`meshShape` fixtures; asserts partial-parent topology, no-parent, coordinator star (F-27), mesh evidence vs `entity_disabled`, host `privileged` row, hypotheses cite their own envelope. No production code changed.
**Left open:** the restart-evidence step (F-31, `P5-09`); `golangci-lint` still not installed.

### 2026-10-03 · P5-10
New `cmd/measure` drives the real tools in-process and reads cost from the audit record; `cmd/spike` gained a logbook restart probe. Owner ran both on HA 2026.9.4 (521 entities). D-05-10: classes stand (`find_stale_entities` 20, `analyze_*` composite, max 36/50). Closes F-26, F-28.
**Surprise:** `find_stale_entities` at `limit=200` examines only 20 entities per call (one recorder read each); and an unfiltered 7d logbook is 10.8 MB, so F-31's restart rows need a bounded window.
**Left open:** F-31 (closes with `P5-09`); F-36 (statistics-based staleness); Supervisor requests not in the `analyze_integration_health` numbers.

### 2026-10-03 · P5-09
`analyze_integration_health` probes the logbook ±5 min around each cluster onset (max 3, one request each) through a new unfiltered-window command; only `homeassistant`-domain rows survive the mapper. Restart ⇒ restart hypothesis (no upstream one); none ⇒ upstream citing a measured zero; unread ⇒ `missing_evidence`. Closes F-31; `TestDocCriterion_…` names all three investigations.
**Surprise:** the restart rows belong to no entity, so the only selector is the time window; and the cluster onset is the one instant that bounds it, which made probing after clustering (not before) the design.
**Left open:** cross-integration restart signal (a restart seen across several config entries); `cmd/measure` does not wire `Lifecycle`, so its composite numbers exclude up to 3 logbook requests; `golangci-lint` not installed.
