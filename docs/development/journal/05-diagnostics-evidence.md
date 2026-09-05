# Journal — Phase 05 Diagnostics & Evidence Engine

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

### 2026-09-05 · `P5-01`
Q9/F-6 answered: mesh metrics need a flat analyzer plus a name/`device_class`
hint table, not a per-integration plugin seam (D-05-5). `cmd/spike` gained
`probeMesh`; evidence in `docs/research/2026-09-05-zigbee-mesh-metric-normalization.md`.
**Surprise:** twice, the same trap. ZHA's LQI/RSSI look absent — they are real
entities (`LQISensor`/`RSSISensor`) that ship `entity_registry_enabled_default =
False`, so they never reach `get_states`; and `via_device_id` looks like parent
topology but is a coordinator star on *both* integrations, so "shares a parent"
is true of the entire Zigbee network (F-27). A first pass on the default model
concluded the opposite of both by grepping HA core's `zha/sensor.py`, which no
longer holds the entity classes at all — they moved to the `zha` library.
**Left open:** F-27 (vacuous shared-parent annotation) for `P5-04`/`P5-08`; no
live ZHA installation exists to confirm the source-read half.
