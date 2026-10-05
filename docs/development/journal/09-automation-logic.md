# Journal — Phase 09

### 2026-10-05 · P9-01
`model.AutomationLogic` + `ha.MapAutomationLogic`/`MapAutomationLogicResult`: grammar-typed nodes, both schema forms, trace-style paths, node/depth/value/template caps.
**Surprise:** HA's `HH:MM` serves both time-of-day and duration (which may exceed 24h or be negative) — only the key (`for`/`delay`/`timeout`/`offset`) tells them apart. Blueprint configs carry no triggers at all, so empty logic would have read as "none": mapped partial instead.
**Left open:** node paths vs real trace keys unverified (F-54, blocks P9-03); top-level `variables` and blueprint inputs unmapped (F-55).
