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
