# Phase 02 — Policy, Privacy, Budget & Audit

**Milestone:** M1 (roadmap "Phase 1 — Observer") · **Target version:** v0.3

> 🧠 **Stronger model recommended.** This phase is the answer to threats T1, T3,
> T4 and T8. Its failure modes are invisible in normal operation: a redaction
> gap looks exactly like working software until a token reaches a cloud LLM.

## Goal

Every MCP invocation runs inside an enforced envelope: a query budget it cannot
exceed, a privacy policy applied before any bytes are formatted into a response,
and an audit record that describes the cost of the call without becoming a copy
of the household's private history.

This phase lands **before** the tool surface widens (doc §23 step 6). Building
twenty tools first and retrofitting policy afterwards is how a gap gets shipped.

## Depends On

Phase 01 — the budget is charged against real upstream calls, and redaction runs
over the normalized model.

## Add Under

```text
internal/policy/   # budget.go, privacy.go, profile.go
internal/redact/   # redact.go
internal/audit/    # logger.go
```

## Design Notes

- **Redaction happens at the source boundary, before response construction**
  (doc §5). A redaction step bolted onto serialization will eventually be
  bypassed by a code path that formats its own output.
- **Deny by class, not by field name spelling.** `SECRET` covers tokens,
  passwords, API keys and credentials in any attribute; matching only the exact
  key `token` misses `access_token`, `api_key` and `Authorization`.
- **Limits are measured, not chosen.** Doc §10's numbers are starting defaults
  and say so (§26). `P0-09` measures multi-entity history and statistics cost
  against the real recorder; `MaxEntities`, `MaxHistoryPoints` and `MaxBytes`
  come from that report, and a limit nobody measured is a guess wearing a
  constant's name.
- **The budget is charged, not checked once.** Every upstream request, every
  history point and every appended byte decrements it. A check at tool entry
  cannot know what the tool will do.
- **Budget exhaustion is an explicit, well-formed result**, never a truncated
  response that looks complete. The agent must be able to tell "here is
  everything" from "here is what fit".
- **Audit records metadata, never bodies** (threat T8): tool name, redacted
  parameters, upstream request count, duration, result bytes, status. No token,
  no full response, no raw entity history.
- **Home Assistant filters nothing by principal** (F-10, observed in
  `../research/2026-08-23-ha-registry-apis.md`): a non-admin connection receives
  the byte-identical entity registry, device registry and config-entry list an
  admin does, and `get_config` hands it the installation's latitude, longitude
  and location name. Every privacy decision this server makes is therefore the
  *only* one made — there is no upstream filter behind it to fall back on.
- **Sensitivity travels with embedded payloads, not with the endpoint** (F-12).
  An automation trace is diagnostic in shape and personal in content: its
  `changed_variables` carry whole state objects — `friendly_name`, `icon` — and
  a `context.user_id`. Classification follows the entities a payload embeds, at
  whatever depth they appear.
- **Masking is not redaction, and the split is deliberate.** Redaction removes a
  value because it must never be seen; masking removes a value's *meaning* while
  preserving its *shape in time*, because the shape is the diagnostic signal and
  the meaning is the exposure. They share the response boundary and nothing else:
  `internal/policy` decides both, `internal/redact` applies both, and a response
  marks the two differently so the agent never reasons about `state_A` as a real
  state name.
- `SUPERVISOR_TOKEN` is never returned and never logged, at any level, in any
  build.

## Tasks

- [ ] **`P2-01` — Query budget** 🧠 — `QueryBudget{MaxHARequests,
  MaxHistoryPoints, MaxEntities, MaxBytes, Deadline}` attached to each
  invocation's context, with the two classes from doc §10 (normal read /
  composite diagnostic) as defaults **taken from `P0-09`'s measurement**, not
  from doc §10's admitted guesses (§26, F-14), each constant carrying the
  measured number it came from in its comment. The measurement landed
  2026-08-24 in
  [`docs/research/2026-08-24-ha-multi-entity-query-cost.md`](../../research/2026-08-24-ha-multi-entity-query-cost.md):
  `MaxBytes` 512 KB / 1 MB, `MaxHistoryPoints` 13 000 / 26 000 (bytes ÷ 37 B per
  history point), `MaxEntities` 200 for both classes (nothing above 200 was
  measured, so doc §10's composite 500 is not carried forward), `Deadline` 10 s
  / 30 s. That file also gives the entity-day pre-flight estimate the budget
  needs in order to refuse **before** issuing a query — enforcing `MaxBytes` on a
  received response is enforcing it after the Pi has already paid.
  **DoD:** tests that each dimension independently trips `ErrBudgetExceeded`; the
  error names which limit was hit and what was retrieved so far; a request storm
  (repeated max-page calls) is rate-limited rather than served (threat T1,
  Appendix B); a tool that finishes inside budget reports its consumption; the
  deadline cancels in-flight upstream calls rather than leaving them running.

- [ ] **`P2-02` — Privacy classification and profiles** 🧠 — classify entities
  `NORMAL` / `PRIVATE` / `SECRET` per doc §5.1, with a configurable profile
  controlling how `PRIVATE` is handled (allow / mask / deny).
  **DoD:** `person.*`, `device_tracker.*`, `lock.*`, `alarm_control_panel.*` and
  precise location attributes classify as `PRIVATE`; a test asserts bulk history
  over a `PRIVATE` domain is refused under the default profile with an explicit
  policy error (Appendix B); classification is driven by a table a reviewer can
  read, not by scattered conditionals; **the installation's own coordinates**
  (`get_config` → `latitude` / `longitude` / `location_name`) classify as
  `PRIVATE` on that same table, asserted by a test — HA hands them to any
  principal (F-10), and they arrive on a path unrelated to the history tools;
  **a payload is classified by the entities it embeds**, so a trace whose
  `changed_variables` contain a `PRIVATE` entity's state, or a `context.user_id`,
  classifies `PRIVATE` even though `trace/get` is a diagnostic endpoint (F-12),
  asserted with a captured trace fixture; the three profiles `mask` (default),
  `allow` and `deny` are selectable and the **default resolves to `mask`**,
  asserted by a test that constructs a profile from empty configuration.

- [ ] **`P2-03` — Redaction** 🧠 — strip `SECRET` values from anything crossing
  the response boundary, including nested attributes and error messages.
  **DoD:** `TestSupervisorTokenNeverReturned` — the live token value planted in
  an entity attribute, a device name, an error string and a log line is absent
  from the rendered MCP response in all four; key-pattern coverage for
  `token|password|secret|api_key|credential|authorization` case-insensitively;
  a redacted field is visibly marked redacted rather than silently dropped, so
  the agent knows something was withheld; the walk descends into **nested state
  objects**, not only top-level attributes — a fixture trace with secrets and a
  `context.user_id` planted inside `trace["trigger/N"][i].changed_variables`
  comes back redacted at that depth (F-12); **masking of `PRIVATE` values is
  applied at this same boundary** per the *PRIVATE handling* decision — a test
  asserts a `person.*` history comes back with opaque state tokens while its
  timestamps and transition count match the unmasked input exactly, that the
  same underlying state maps to the same token within one response and to a
  *different* token in a second response over the same data, that
  `get_config` latitude/longitude are coarsened to one decimal while
  `location_name` is untouched, and that every masked field is visibly marked
  masked and distinguishable from a redacted one.

- [ ] **`P2-04` — Response size cap and pagination contract** — a single place
  that enforces the byte cap and emits cursor pagination for every `list_*`
  tool: default limit 50, max 200.
  **DoD:** a response approaching `MaxBytes` after aggregation is cut at a record
  boundary with `truncated: true` and a usable cursor, never mid-structure
  (Appendix B); `limit > 200` is clamped with an explicit note, not silently
  honored; a cursor from a changed underlying list does not produce duplicate or
  skipped records without saying so.

- [ ] **`P2-05` — Audit logger** — one structured record per MCP invocation in
  the shape of doc §17, plus operational logs with no secrets and no full
  payloads at INFO.
  **DoD:** a record is emitted for success, for policy denial and for budget
  exhaustion alike; `TestAuditNeverContainsSecrets` plants secrets in parameters
  and asserts the record carries redacted parameters; a test asserts no result
  body is persisted by default; audit emission failure never fails the tool call.

## Decisions

- [x] **PRIVATE handling — mask by default** — decided 2026-08-25

  **Decision.** The default privacy profile **masks** `PRIVATE` data rather than
  denying or allowing it. A `PRIVATE` entity's states are replaced with stable
  opaque tokens (`state_A`, `state_B`, …) while **timestamps, ordering and
  transition counts are preserved verbatim**. The installation's own coordinates
  (`get_config` → `latitude` / `longitude`) are **coarsened to one decimal
  place** (~11 km) rather than withheld, and `location_name` passes through
  unchanged. `allow` and `deny` remain selectable profiles for owners who want
  either end.

  **Rationale.** The two properties are separable: availability, flapping,
  outage and correlation analysis need *when a value changed and how often*,
  while occupancy reconstruction needs *what the value was*. Masking keeps the
  first and destroys the second, so the diagnostic purpose of this server
  survives without handing an occupancy timeline to whatever cloud model is on
  the other end of the MCP connection (threat T3). Coarsened coordinates keep
  sun-elevation, weather and timezone correlation working — real diagnostic
  inputs — while removing address-level identification; withholding them would
  degrade those correlations to buy nothing masking does not already buy, since
  `location_name` is the owner's own label and carries no address.

  **Rejected.** *Deny by default* — safest, but a flaky presence sensor or an
  unreliable lock is exactly the kind of fault this server exists to diagnose,
  and denying it makes the default profile one every owner immediately loosens;
  a default that gets turned off is not a default. *Allow with bounded history* —
  a 24 h window is still a full occupancy timeline, and bounding the window
  bounds the volume rather than the exposure.

  **Consequences.** `internal/policy` decides *that* a value is masked and
  *which* token it gets (classification and profile); `internal/redact` applies
  the substitution at the response boundary, the same boundary `SECRET` stripping
  runs at. Token assignment must be **stable within one response** — the same
  underlying state maps to the same token throughout — or transition counting
  becomes unreadable; it must **not** be stable across responses, or the tokens
  become a de-facto stable identifier that leaks the value by correlation. A
  masked field is marked masked, exactly as a redacted one is marked redacted
  (`P2-03`): the agent must be able to tell a masked state from a real one, or
  it will reason about `state_A` as though it were a state name.

- [ ] **`needs-decision` — Persistence beyond cache and audit**
  Q10. Memory-only keeps the App small on a Raspberry Pi and makes every restart
  a clean slate. An embedded store (SQLite/Bolt/Badger) buys durable audit and
  historical baselines for anomaly detection in Phase 05 — at the cost of a
  writable data volume and a schema to maintain. Do not decide this ahead of
  evidence: revisit when Phase 05 has a concrete diagnostic that memory-only
  cannot deliver.

## Phase Definition of Done

- No tool can exceed its budget, and exhaustion is always an explicit error
  naming the limit — never a silently short answer.
- Redaction tests demonstrate that a planted secret cannot reach a response,
  a log line or an audit record by any of the tested routes.
- Audit answers "what did the agent request and how expensive was it?" and
  nothing more.
- `make check` is green.
