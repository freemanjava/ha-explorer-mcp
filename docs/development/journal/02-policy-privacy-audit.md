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

---

### 2026-08-25 · P2-02
`Sensitivity` (normal/private/secret) is decided by five readable tables — private domains, occupancy-shaped device classes, secret key fragments (substring, case-insensitive, so `access_token` and `Authorization` are caught by class rather than by spelling), private attribute keys, and `get_config`'s coordinate fields — plus `ClassifyPayload`, which walks a decoded payload and reports the strictest class it embeds at any depth, so a `trace/get` response classifies by the entities inside its `changed_variables` rather than by its diagnostic endpoint (F-12), asserted against a new `test/fixtures/automation_trace_get.json`. `Profile` (`mask` default as the zero value, `allow`, `deny`) turns a class into an `Action`, and `CheckHistoryScope` refuses with `ErrPolicyDenied` before the recorder is asked.
**Surprise:** the DoD's "bulk history over a PRIVATE domain is refused under the default profile" and the 2026-08-25 mask-by-default decision only agree once *bulk* is read as scope shape, not volume — a named `lock.front_door` is masked and served, a `device_tracker` domain sweep is refused, because correlating N masked trackers reconstructs the day that masking one does not. `HistoryScope{Entities, Domains}` exists to carry that distinction. Also: doc §5.1's "presence/occupancy" has no domain to put in a table — an occupancy sensor sits in `binary_sensor` beside a power meter — so device class had to become a second classification axis, including inside the payload walk. That reads an HA-supplied value, which rule 6 forbids branching on; it is safe only because the lookup can escalate a class and never lower it.
**Left open:** classification only. Nothing masks, redacts or coarsens yet — `CoordinateDecimals` is a policy constant with no applier until `P2-03`, and `Action`/`Sensitivity` have no caller at the response boundary. `ClassifyEntityWithClass` is likewise unwired: the registry `Entity.DeviceClass` that feeds it reaches the tools in Phase 03.

### 2026-08-25 · `P2-03`
`internal/redact` applies at the response boundary what `internal/policy`
decides: secret keys and secret literals become `[redacted]`, private states
become `[masked:state_<nonce>X]`, get_config coordinates coarsen, every
withheld field is marked, and a `slog.Handler` closes the log-line route.
**Surprise:** "stable within one response" is too coarse — two private entities
in the same state got the same token, which withholds the meaning and hands
over the correlation. Tokens are now scoped per entity; the decision record was
amended in the same change.

### 2026-08-25 · `P2-04`
`internal/page` cuts one page from a caller-sorted list, cutting at the first
of resolved limit / cumulative byte size / list end, always keeping a whole
record even when one alone exceeds `maxBytes`.
**Surprise:** an index-based cursor was the obvious first design and the wrong
one — resuming from "record 3" breaks the moment record 2 is deleted. Keying
the cursor to the last-returned sort key instead makes duplicate/skip
structurally impossible rather than something to detect and flag, so the DoD's
"without saying so" clause needed no extra signal in the result at all.
**Left open:** no `list_*` tool exists yet to call it (Phase 03); `keyOf` and
`byteSize` are caller-supplied rather than JSON-marshal defaults, a choice to
revisit once a real tool response shape exists to measure against.
