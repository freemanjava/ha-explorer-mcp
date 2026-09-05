# Journal — Phase 04 History, Statistics & Detection

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

### 2026-09-05 · `P4-01`
`get_entity_history` lands: `internal/ha` gains `historyDuringPeriodCommand`
and `CoreReader.History` over the already-allow-listed
`history/history_during_period`, `MapHistoryDuringPeriod` reading both the
minimal_response short keys (`s`/`lu`) and the full long ones so neither mode
needs its own mapper; `internal/mcp/history_tools.go` validates entity-id
shape and `to > from`, refuses over a fixed 7-day range cap before touching
policy or budget, calls the existing (but previously unused by any tool)
`Profile.CheckHistoryScope` and `QueryBudget.Preflight`/`Charge*` machinery
P2-01/P2-03 had already built, and masks the whole point series in one
`redact.Payload` call so equal states keep the same token across the
timeline.
**Surprise:** `internal/policy` already had `HistoryScope`/`CheckHistoryScope`
and `Preflight` fully built and tested from Phase 02, anticipating this exact
tool — implementing P4-01 was mostly wiring, not designing new policy.
**Left open:** `resolution` (Appendix A.2) not implemented — see the phase
file's decision record; belongs to `P4-04`'s statistics source instead.

### 2026-09-05 · `P4-02`
`internal/analysis` opens with `availability.go`: `ComputeAvailability` reduces
one entity's mapped history over a bounded window to availability ratio, outage
count, total and longest outage, sorting/collapsing/clamping untrusted points
itself; a leading recorder gap reduces `Covered` instead of counting as
downtime, and no recorded state at all leaves `Computable` false rather than a
0.0 ratio. Fixture `entity_history_7d.json` (413 points, minimal `s`/`lu`
shape) reproduces doc §12.1's outage numbers through the real mapper.
**Surprise:** doc §12.1's example is not self-consistent — 3h12m of 7d is a
0.98095 ratio, not the 0.982 it prints; the fixture matches the outage numbers
and the test asserts the computed ratio instead.
**Left open:** cadence/staleness (`P4-03`) and the tool surface (`P4-04`); the
report type stays in `internal/analysis` until `P4-04` needs a joined shape.

### 2026-09-05 · P4-03
`internal/analysis/staleness.go`: `ComputeCadence` reduces one entity's window
to nearest-rank median/p95 update intervals, min/max, state-change rate (on
`segmentsIn`'s collapsed basis, so it agrees with `ComputeAvailability`), and a
staleness verdict at `staleIntervalFactor(3) × p95` — entity-relative, no
global constant, no absolute floor.
**Surprise:** doc §12.1's cadence half is impossible on its own terms too —
412 changes at a 31s median is ~3.5h of samples, not the 7d it claims; appended
to F-24, which had only caught the ratio.
**Left open:** the factor 3 is a starting default; `P4-05` is the first place
its false-positive rate can be measured.

### 2026-09-05 · P4-06
Doc-only: §12.1's `availability_ratio`, `median_update_interval_s` and
`p95_update_interval_s` corrected to the values the fixture yields through the
real mapper (0.98095 / 1376.5 / 2578.8), leaving `state_changes` and the other
observed fields untouched.
**Left open:** none — F-24 closes with this task.

### 2026-09-05 · P4-04
`get_entity_statistics` lands: one recorder read, reduced through both
`ComputeAvailability` and `ComputeCadence`, joined into `model.Health` —
Phase 00's stub type, unused until now, matched doc §12.1 field-for-field
already. `period` accepts Appendix A.3's "Nd" shorthand or a plain Go
duration, capped at `maxHistoryWindow` (reused from `P4-01`, not duplicated).
**Surprise:** `model.Health` already existed with almost the exact shape this
tool needed — a Phase 00 scaffold nobody had wired up yet.
**Left open:** F-17's "revisit when P4-04 closes" trigger has now fired;
needs a `plan`/`finding` pass to re-triage rather than sitting deferred.

### 2026-09-05 · P4-05
`find_unavailable_entities` pages a cheap aggregate scan (registry +
`UnavailableEntityIDs`, no per-entity cost). `find_stale_entities` scans
filtered candidates in id order, judging each with `ComputeCadence` against
one recorder read; `Truncated` means "candidates remain unexamined" (the
HA-request budget binds at 20, well below a real installation's size), not
the usual "more results exist" — `NextCursor` resumes the scan itself. Both
exclude PRIVATE entities outright under the deny profile rather than masking
a value neither tool has, and report the count via `PrivateExcluded`.
**Surprise:** `analysis.CadenceReport.StalenessRatio` already existed,
commented "for ranking entities against each other (P4-05)" — P4-03 had
already reserved the ranking key this task needed.
**Left open:** filed F-26 — `find_stale_entities` stays `ClassNormalRead`
rather than the wider composite budget `catalog.go`'s own comment invites,
for lack of a real-installation measurement to justify it.
