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
