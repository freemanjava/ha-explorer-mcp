# Journal — Phase 02 Policy, Privacy, Budget & Audit

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

### 2026-08-25 · P2-01
`internal/policy` now exists: `QueryBudget` charges four dimensions (HA requests, history points, entities, bytes) against per-class `Limits` whose constants each cite the 2026-08-24 measurement they came from; `WithBudget` attaches it to the invocation context *and* bounds that context by the class deadline, so an in-flight upstream call is cancelled rather than left running. `Preflight` refuses an over-budget query from the per-entity-day means before the recorder is asked, charging nothing. A separate token-bucket `RateLimiter` answers the request-storm half of the DoD — a budget bounds one invocation, and a client inside every budget can still storm by looping max-page calls.
**Surprise:** the research's "the byte cap binds first, everywhere" does not survive contact with its own derived constants. `MaxHistoryPoints` was derived as `MaxBytes ÷ 37`, and the measured history mean is 5 600 B / 151 points ≈ 37.1 B/point — so for history the two caps are the same cap to within rounding (86 vs 94 entity-days), and the point cap actually trips marginally first. The byte cap only genuinely binds first for statistics, at 110 B/point. Both are enforced, and `Preflight` checks bytes before points so the reported dimension matches the report's framing.
**Left open:** nothing charges the budget yet — the charging sites are inside the tools (Phase 03/04) and the limiter's `Allow()` belongs at the MCP invocation boundary (`P3-01`); `internal/policy` ships as decision-only, with no caller. The rate limiter's own constants are the one unmeasured pair in the task, filed as **F-20** (`defer`).
