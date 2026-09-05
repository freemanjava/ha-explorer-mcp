# Zigbee/mesh metric normalization across ZHA and Zigbee2MQTT

**Task:** `P5-01` · resolves **F-6** / doc §22 Q9
**HA version:** `2026.9.0` (owner's installation, reported by REST `GET
/api/config` in the same `cmd/spike` run)

**Method:** two sources of different strength, because the owner's installation
runs only one of the two integrations (see *Not established*):

1. **Live probe** — `cmd/spike`'s `probeMesh` (added by this task), run by the
   owner against their own installation with a long-lived admin token. It reads
   `config/device_registry/list`, `config/entity_registry/list` and
   `get_states`, and reports identifier domains, `via_device_id` population by
   domain, and which entities match either integration's link-quality /
   signal-strength naming — field names and counts only, never an entity id or
   a value.
2. **Source reading** at pinned versions, for the ZHA half, which the owner's
   installation cannot answer at all. Same fallback and same justification as
   `2026-08-23-supervisor-permissions.md`: the claims below are about a
   client-visible contract (an entity's registry defaults, a `device_info`
   field) rather than emergent runtime behavior.
   - `zha` library (PyPI `zha`, which HA core's `zha` component wraps) at
     **`2.1.0`** — the version `requirements_all.txt` pins for core `2026.8.3`.
   - `home-assistant/core` at **`2026.8.3`**.
   ```sh
   curl -s https://raw.githubusercontent.com/zigpy/zha/2.1.0/zha/application/platforms/sensor/__init__.py
   curl -s https://raw.githubusercontent.com/home-assistant/core/2026.8.3/homeassistant/components/zha/entity.py
   ```

## Found

The owner's installation runs **Zigbee2MQTT only** — 28 devices under the `mqtt`
identifier domain, **zero** under `zha` (full domain list: `androidtv_remote`,
`backup`, `broadlink`, `cast`, `hacs`, `hassio`, `ipp`, `met`, `mobile_app`,
`mqtt`, `rpi_power`, `sun`, `systemmonitor`, `volumio`, `wyoming`).

### 1. Link quality and signal strength are entity-shaped on *both* integrations

| | Zigbee2MQTT (observed live) | ZHA (read from source) |
|---|---|---|
| link quality | `sensor.*_linkquality` — 23 entities, all matched on entity id, all `platform: mqtt` | `LQISensor`, `_unique_id_suffix`/`translation_key` `lqi` → `sensor.*_lqi` |
| `device_class` / unit for LQI | none carried | **explicitly** `device_class = None`, `native_unit_of_measurement = None` |
| signal strength | **none** — zero entities matched by id, `device_class`, unit or attribute key | `RSSISensor` — `device_class = SIGNAL_STRENGTH`, unit `dBm`, `state_class = MEASUREMENT` |
| registry default | linkquality shipped enabled (it was in `get_states`) | **`_attr_entity_registry_enabled_default = False`** on both, `EntityCategory.DIAGNOSTIC` |

Both integrations put these values in the **state machine**, as ordinary
entities with recorder history — not behind an integration-private API. ZHA's
`RSSISensor`/`LQISensor` (`zha/application/platforms/sensor/__init__.py`,
~L3208 and ~L3283) are registered against `Basic.cluster_id` with
`_is_supported()` hardcoded `True`, so *every* ZHA device gets both.

The differences are: **the name** (`_linkquality` vs `_lqi`), **whether a
semantic marker exists** (only RSSI has one, `device_class: signal_strength`;
LQI deliberately has none on either side), and **whether the entity is enabled**
(ZHA ships both disabled; Zigbee2MQTT ships linkquality enabled and has no RSSI
equivalent at all).

### 2. `via_device_id` is populated by both — as a flat star to the coordinator

ZHA sets it explicitly, in `homeassistant/components/zha/entity.py` `device_info`
(~L140–148):

```python
coordinator_ieee = str(zha_gateway.state.node_info.ieee)
if ieee != coordinator_ieee:
    device_info["via_device_id"] = dr.async_get_device_id_by_identifier(
        gateway_proxy.hass, (DOMAIN, coordinator_ieee), ...
    )
```

Every device that is not the coordinator points at the coordinator. That is
N−1 of N populated, with exactly **one** distinct value.

The owner's Zigbee2MQTT installation shows the same N−1-of-N signature: **27 of
28** populated, the one unpopulated device being (necessarily) the hub the other
27 hang from.

**So the two integrations agree here — and what they agree on carries no mesh
routing information.** `via_device_id` is a bridge/coordinator star on both, not
a parent/router relation. The real Zigbee neighbour table is not in the device
registry on either integration.

## Not established

- **No live ZHA run.** No ZHA-based installation was available; every ZHA claim
  above is pinned source, not observation. A live run would be stronger, should
  a ZHA installation ever become reachable.
- **Whether Zigbee2MQTT's 27 `via_device_id` values are all the same one.** The
  27-of-28 count is strongly consistent with a single-bridge star and matches
  ZHA's confirmed shape, but the probe reported a count, not a cardinality. The
  decisive measurement is the number of *distinct* `via_device_id` values among
  `mqtt`-domain devices — `probeMesh` now reports it, so the next `cmd/spike`
  run settles it. Filed as **F-27**, because it is `P5-04`'s problem, not this
  task's.
- **Whether any Zigbee2MQTT deployment exposes RSSI.** Absent here; not shown
  absent in general.

## Means

**Q9 answers "flat analyzer, plus a small lookup table" — not a per-integration
plugin seam.** The three axes checked disagree only in ways that are data, not
structure:

- *Where the value lives* is the same on both — an entity in the state machine
  with recorder history. This is the axis that would have forced a plugin, and
  it does not.
- *What the entity is called* differs (`_linkquality` vs `_lqi`), and only RSSI
  carries a `device_class`. A name/`device_class` hint table resolves this; it is
  a constant, not an abstraction.
- *Whether the value is readable at all* differs per installation (ZHA disabled
  by default, Zigbee2MQTT has no RSSI). This is precisely `MissingEvidence`
  (D-05-1) — an expected, reportable absence, not an architectural problem.

One consequence lands outside this task and is filed as **F-27**: because
`via_device_id` is a coordinator star on both integrations, annotating an outage
cluster with "these members share a `via_device` parent" is **vacuous for
Zigbee** — every device in the network shares it. D-05-3 and `P5-04`/`P5-08`
currently treat that annotation as meaningful evidence.
