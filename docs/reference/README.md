# Reference

Curated, maintained facts read *while implementing*: the gateway allow-list as it
stands, the tool catalog contract, HA API shapes the project depends on.

Unlike `docs/research/`, files here are **kept true**. When one stops matching
the code, fix one or the other in the same change.

Expected first inhabitants, once Phase 00 produces the evidence for them:

- `ha-allowlist.md` — the exact WebSocket commands and REST routes the gateway
  permits, and why each is needed.
- `tool-catalog.md` — every MCP tool, its schema, its budget class and its
  compatibility status.
