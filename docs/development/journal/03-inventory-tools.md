# Journal — Phase 03 MCP Server & Inventory Tools

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

### 2026-09-05 · P3-01
`internal/mcp`: static twenty-row catalog with a budget class per row, stdio
server, and one receiving middleware carrying rate limit, budget, panic
recovery and audit for every invocation; `cmd/server` now actually runs it.
**Surprise:** the SDK never recovers a panicking handler — nothing in the module
calls `recover()` — so the middleware's recovery is what keeps a nil-map bug
from taking the whole App down. Its `ErrServerClosing` sentinel is also in an
`internal/` package, so a mid-request disconnect cannot be matched with
`errors.Is` (F-21).
**Left open:** every row is bound to a not-implemented handler; `P3-02` onward
replace them one at a time.
