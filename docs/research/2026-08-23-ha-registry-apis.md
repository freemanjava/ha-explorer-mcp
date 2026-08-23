# HA registry & config-entry read APIs — observed

**Task:** `P0-04` · resolves **F-1**, **F-2**
**Observed:** 2026-08-23, against a live Home Assistant OS installation, in two
runs — an owner/admin token (16:00 UTC) and a **Users**-group, `is_admin: false`
token (16:19 UTC)
**HA version:** `2026.8.3` (reported by both REST `GET /api/config` and WS `get_config`)
**Vehicle:** `cmd/spike` (throwaway, Phase 00) over `http://<host>:8123` with a
long-lived access token, run once per principal.

> **Scope limit — read this before deriving permissions from it.** The run used
> a *user's* long-lived token, not `SUPERVISOR_TOKEN` through the Supervisor
> proxy. Command names, response schemas and payload sizes below are established
> facts. What the **App** may reach is *not* established here; that is `P0-06` /
> F-4. Nothing in this file may be cited as evidence about App permissions.
>
> Response shapes are field names and types only — the probe withholds every
> value from the installation by construction (`cmd/spike/shape.go`).

## Summary

All twelve candidate commands exist and answered successfully on 2026.8.3. No
command in the Phase 01 candidate set is unavailable. Three returned empty
collections on this installation, so their **element schema is unobserved** —
recorded below as such rather than guessed.

| command | admin | non-admin | size / time (admin) | notes |
|---|---|---|---|---|
| `get_config` | ✅ | ✅ | 3095 B, 7 ms | version source |
| `auth/current_user` | ✅ | ✅ | 241 B, 6 ms | reports the caller's `is_admin` |
| `config/entity_registry/list` | ✅ | ✅ | 584 456 B, 44 ms | 952 entries |
| `config/entity_registry/list_for_display` | ✅ | ✅ | 64 333 B, 9 ms | 469 entries — **different population** |
| `config/entity_registry/get` | ✅ | ✅ | 693 B, 4 ms | superset of the list entry |
| `config/device_registry/list` | ✅ | ✅ | 51 591 B, 7 ms | 69 devices |
| `config/area_registry/list` | ✅ | ✅ | 1246 B, 3 ms | 6 areas |
| `config/floor_registry/list` | ✅ | ✅ | 2 B, 4 ms | **empty — schema unobserved** |
| `config/label_registry/list` | ✅ | ✅ | 2 B, 4 ms | **empty — schema unobserved** |
| `config/category_registry/list` | ✅ | ✅ | 2 B, 4 ms | `scope: automation`; **empty — schema unobserved** |
| `config_entries/get` | ✅ | ✅ | 18 115 B, 6 ms | 35 entries |
| `config_entries/get_single` | ✅ | ⛔ **`unauthorized`** | 521 B, 5 ms | **the only admin-gated command** |

**Exactly one command in the candidate set requires admin:**
`config_entries/get_single`, which answered a non-admin caller with
`success: false`, error code `unauthorized`, message `Unauthorized`. Every other
command answered a non-admin identically to an admin.

Both runs reported HA `2026.8.3` and returned **byte-identical payload sizes**
for every shared command (584 456 / 64 333 / 51 591 / 18 115 / 1246 B). Home
Assistant applies no per-user filtering to registry reads: a non-admin sees the
whole registry, not a subset.

## Findings that change Phase 01

### 1. `list_for_display` is not a cheaper drop-in for `list`

It is 9× smaller (64 KB vs 584 KB) and 5× faster, but returned **469 entities
where `list` returned 952**. It is a display-filtered population, not the same
data compressed, so an inventory tool built on it would silently omit roughly
half the registry — including, on the evidence of the field set, disabled and
hidden entities, which are exactly what a diagnostic tool cares about.

**Consequence:** inventory and diagnostics use `config/entity_registry/list`.
`list_for_display` may only back a tool that explicitly means "what a user
sees", and any such tool must say so in its provenance.

Its keys are abbreviated and one is an index, not a value:

| key | meaning |
|---|---|
| `ei` | entity_id | 
| `di` | device_id |
| `ai` | area_id |
| `en` | name |
| `pl` | platform |
| `ic` | icon |
| `tk` | translation_key |
| `lb` | labels |
| `hn` | has_entity_name |
| `hb` | hidden_by |
| `dp` | display precision |
| `ec` | **index into the sibling `entity_categories` lookup object**, not a value |

The `ec` indirection is a mapping trap: a naive adapter would emit a number
where a category name belongs.

### 2. `config_entries/get_single` wraps its result; `config_entries/get` does not

`get` returns a bare array of entries. `get_single` returns
`{"config_entry": {…}}`. Two different envelopes for the same entry type — the
Phase 01 mapper needs distinct decode paths, and a shared one will fail on the
second.

### 3. `config/entity_registry/get` returns a superset of the list entry

Fields present in `get` but absent from `list`: `aliases`, `capabilities`,
`device_class`, `original_device_class`, `original_icon`. So a per-entity detail
tool cannot be served from a cached list result — it needs the per-entity call,
which bears on the caching design in Phase 01.

### 4. `config_entries/get` is readable by a non-admin but `get_single` is not

The list returns all 35 entries with the full field set to any authenticated
user; the single-entry read of *the same data* is refused with `unauthorized`.
The asymmetry is Home Assistant's, not ours, and it is the more permissive path
that is unrestricted.

**Consequence:** the Phase 01 config-entry detail tool reads
`config_entries/get` and selects the entry in-process, rather than calling
`config_entries/get_single`. That is not a workaround for a permission check —
it returns identical data from an endpoint HA itself leaves open — and it makes
the tool work for a non-admin principal instead of failing for one. If
`get_single` is allow-listed at all, its refusal must map to `ErrPolicyDenied`,
distinct from `ErrNotFound` and `ErrUnsupported`.

### 5. WebSocket error shape for a refusal, observed

A refused command returns a normal `result` envelope with `success: false` and
`error: {code: "unauthorized", message: "Unauthorized"}` — not a transport
error and not a disconnect. Phase 01's gateway maps `code` to the sentinel
errors; `unauthorized` → `ErrPolicyDenied`.

### 6. Payload sizes bear on the budget, on a Pi

`config/entity_registry/list` is **584 KB in one frame** on a 952-entity
installation, and installations get larger. Phase 01's bounded-read limit and
Phase 02's budget class for registry reads must be set against this number, not
against the architecture doc's placeholder. The 44 ms was measured over LAN to
the Pi and excludes serialization into MCP.

### 7. Privacy-relevant fields present in the device registry

`connections` carries `[[type, value]]` pairs (MAC addresses), and
`identifiers` carries integration-scoped ids. `serial_number` was `null` on all
69 devices here, but the field exists and will not be null everywhere. These are
inputs to Phase 02's privacy classification, not fields to pass through.

### 8. Empty registries mean the schema is unverified, not absent

`floor_registry`, `label_registry` and `category_registry` each returned `[]`.
The **command names are confirmed** (they succeeded rather than erroring
`unknown_command`), but no element schema was observed. Phase 01 may allow-list
these commands; it may **not** write a mapper against an assumed element shape.
Either the owner creates one floor/label/category and the probe is re-run, or
the mapper is written defensively and marked partial.

## Open oddity

`config/entity_registry/get` reported `aliases: array[1]` whose single element
typed as `null`. Either the sampled entity genuinely holds a null alias or the
merge is mis-typing a one-element array. It reproduced identically across both
runs, so it is stable rather than a fluke — but stable and explained are
different things, and an unexplained result is not a verified one. Worth one run
against a named entity before Phase 01 maps `aliases`.

## Not established here

- Anything about `SUPERVISOR_TOKEN` or App-level permissions (`P0-06`). Both
  runs used a *user's* long-lived token. That an unprivileged user may read
  these does not establish that the App's Supervisor-proxied principal may.
- Behavior across an HA upgrade — this is one release, `2026.8.3`.
- Element schemas for the three empty registries.
