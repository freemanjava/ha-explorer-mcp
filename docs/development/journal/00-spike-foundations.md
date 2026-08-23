# Journal — Phase 00 Spike & Foundations

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

### 2026-08-23 · P0-01
Pinned `github.com/modelcontextprotocol/go-sdk` v1.7.0 (protocol 2026-07-28) and
added `.github/workflows/ci.yml` running `make check` + `make release`.
**Surprise:** the SDK exposes no public protocol-version constant, so
`TestSDKProtocolVersion` asserts it behaviorally — connect an in-memory
client/server pair and check the negotiated `InitializeResult.ProtocolVersion`.
