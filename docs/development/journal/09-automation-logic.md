# Journal — Phase 09

### 2026-10-05 · P9-01
`model.AutomationLogic` + `ha.MapAutomationLogic`/`MapAutomationLogicResult`: grammar-typed nodes, both schema forms, trace-style paths, node/depth/value/template caps.
**Surprise:** HA's `HH:MM` serves both time-of-day and duration (which may exceed 24h or be negative) — only the key (`for`/`delay`/`timeout`/`offset`) tells them apart. Blueprint configs carry no triggers at all, so empty logic would have read as "none": mapped partial instead.
**Left open:** node paths vs real trace keys unverified (F-54, blocks P9-03); top-level `variables` and blueprint inputs unmapped (F-55).

### 2026-10-05 · P9-02
`get_automation_logic` (`ClassNormalRead`): deny masks PRIVATE entity ids (`withheld`, `IdsWithheld`) and drops templates (`TemplatesWithheld`); other profiles ship templates as `untrusted_template`, token-scrubbed; byte cap drops trailing top-level nodes. Catalog 22, doc §9, README, CLAUDE.md rule 6 note.
**Surprise:** the SDK panics at registration on a self-nested output type (`LogicNode.Children`), so the handler returns `any` and the output schema is omitted. The first byte-cap test overshot the cap: the size estimate omitted the array commas.
**Left open:** `mask` profile ships templates (D-09-5 names only `deny`/`allow`) — F-56.

### 2026-10-05 · P9-06
`logicWalker.parallel`: a branch that is not a `{sequence: [...]}` object maps at `…/parallel/I/sequence/0`, as HA's validation wraps it; no wrapper node. Nested-structures test corrected, new bare+sequence case and a depth-cap case.
