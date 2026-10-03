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
