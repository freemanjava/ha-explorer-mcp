# Recorder history & statistics APIs — observed behavior and cost

Evidence for `P0-07`, resolving **F-5** (Q5 of the architecture doc §22).
Dated snapshot: it says what was true on this date, with the method that
re-checks it. Curated conclusions belong in `docs/reference/`, not here.

### 2026-08-23 · Which source should the history and statistics tools read, and what does each cost on a real recorder?

**Kind:** world-discoverable

**Method:** `cmd/spike` at this revision, run by the owner against their live
installation over the LAN:
`HA_URL=http://homeassistant.local:8123 HA_TOKEN=<long-lived token> go run ./cmd/spike`.
Core **2026.8.3**, 478 entities, 286 statistic ids, authenticated as **admin**.
Every query is bounded to one entity and an explicit window; each was issued
twice (cold, then warm) at a fixed end instant. The target was a numeric
`sensor.*` carrying `state_class` — a chatty one, ~1548 recorded state changes
in 24h, which makes these upper-bound numbers for a single entity rather than
typical ones. Timings are wall clock from the owner's workstation and include
LAN and JSON decode; they are not in-Core query times.

## Found — REST `GET /api/history/period`

Shape is `array[1]` (one array per requested entity) of state objects
(`entity_id`, `state`, `last_changed`, `last_updated`, `attributes`).

| window | parameters | bytes | cold | warm |
|---|---|--:|--:|--:|
| 24h | none (full states) | 510 561 | 392 ms | 382 ms |
| 24h | `minimal_response` | 105 205 | 124 ms | 97 ms |
| 24h | `no_attributes` | 272 263 | 272 ms | 285 ms |
| 24h | both | 105 051 | 90 ms | 125 ms |
| 7d | none (full states) | 3 566 221 | 2.811 s | 1.654 s |
| 7d | `minimal_response` | 733 699 | 1.535 s | 483 ms |
| 7d | `no_attributes` | 1 901 917 | 1.604 s | 1.584 s |
| 7d | both | 733 545 | 454 ms | 385 ms |

Parameter behavior, observed rather than documented:

- **`minimal_response` is the parameter that matters.** It emits the first state
  in full and reduces every subsequent one to `last_changed` + `state` — the
  probe's shape counts show `attributes`, `entity_id` and `last_updated` present
  in **1 of 1546** elements. 4.9× smaller at 24h, 4.9× at 7d.
- **`no_attributes` alone is the weak one.** `attributes` is still emitted, as an
  *empty object*, on every element: 272 KB versus 510 KB at 24h, and it did not
  reliably reduce latency (272/285 ms cold/warm, versus 392/382 ms unfiltered).
- **Together they are `minimal_response` plus ~150 bytes.** `no_attributes`
  suppresses the attributes of the one full state that `minimal_response` keeps.
  Latency was consistently the best of the four at both windows.
- Element counts differ by two between the modes (1548 vs 1546): `minimal_response`
  collapses consecutive same-state rows. It is a summary, not a lossless subset.

## Found — WebSocket `history/history_during_period`

Same data, materially cheaper, and it is the route an App already holds a
connection for. Result is an **object keyed by entity id**, whose elements use
the short keys `lu` (last updated, epoch seconds) and `s` (state).

| window | bytes | cold | warm |
|---|--:|--:|--:|
| 24h (`minimal_response`, `no_attributes`, all changes) | 58 159 | 129 ms | 157 ms |
| 7d (same) | 406 101 | 240 ms | 293 ms |

**1.8× smaller than the best REST variant, and 1.6–3.8× faster at 7d.** The gap
widens with the window: at 7d, WS answered in 240 ms what REST needed 454 ms
(both flags) to 2.8 s (unfiltered) to deliver.

## Found — recorder statistics WebSocket API

All three commands exist and answer at Core 2026.8.3.

`recorder/list_statistic_ids` — `array[286]`, 65 561 bytes, 68 ms cold / 38 ms
warm. Per entry: `statistic_id`, `source`, `name` (null throughout this
installation), `unit_class`, `display_unit_of_measurement` and
`statistics_unit_of_measurement` (both `null|string`), `has_mean`, `has_sum`,
and **`mean_type` (a number)**. `has_mean` and `mean_type` are both present —
the newer field has not replaced the older one at this release.

`recorder/get_statistics_metadata` — same element shape for the named ids, 226
bytes for one.

`recorder/statistics_during_period` — object keyed by statistic id; elements
carry `start`, `end` (epoch numbers) and only the types the metric actually has:

| window | period | points | bytes | cold | warm |
|---|---|--:|--:|--:|--:|
| 24h | `5minute` | 287 | 26 593 | 63 ms | 47 ms |
| 24h | `hour` | 23 | 2 226 | 27 ms | 16 ms |
| 7d | `hour` | 167 | 15 808 | 149 ms | 46 ms |
| 7d | `day` | 8 | 794 | 28 ms | 25 ms |

- `types: ["mean","min","max","sum","state"]` was requested; the answer carried
  **`mean`, `min`, `max` only** for this `measurement` sensor. Unsupported types
  are dropped silently, not refused — a tool must read what came back, never
  assume the fields it asked for.
- **Omitting `types` is still accepted** (24h/`hour`: 2 640 bytes, 49 ms) and
  returns slightly more than the explicit request. The parameter is an optional
  narrowing, so an adapter need not feature-detect it to work.
- `5minute` data was present for the full 24h window (287 of a possible 288).

## Cost, side by side

For a 7-day view of one entity: **794 bytes** (statistics, day buckets) ·
**15.8 KB** (statistics, hour) · **406 KB** (WS history) · **734 KB** (REST,
both flags) · **3.5 MB** (REST, unfiltered). Four orders of magnitude between
the cheapest correct answer and the naive one.

## Not established

- **Multi-entity cost.** Every measurement is one entity. The queries that will
  actually strain a Pi are the ones a fleet-wide detector issues across dozens
  of entities, and nothing here says whether that cost is linear.
- **Host hardware and recorder backend.** Timings are client-side over LAN
  against the owner's installation; the DB engine (SQLite vs MariaDB), its size,
  and whether Core runs on the Pi the architecture doc assumes were not
  captured. Treat these as *relative* costs, which is what the recommendation
  rests on, not as absolute Pi latencies.
- **`5minute` retention.** Present for 24h here; the point at which it stops
  being available (and the query silently returns less) was not probed.
- **Statistics for non-`measurement` metrics.** Only a `mean`-type sensor was
  measured; `sum`/`state` behavior for energy-style totals is unverified.
- **Behavior under an HA restart mid-query**, and the shape of a refusal for a
  non-admin principal: this run was admin-only. `P0-04`/`P0-05` established that
  the App's Core principal is admin, so this is a gap in the record, not a risk
  to the plan.

## Means

For Phase 04 and the Phase 01 allow-list:

1. **Statistics are the primary source for anything aggregate or long-range.**
   `recorder/statistics_during_period` with an explicit `period` is 1–3 orders
   of magnitude cheaper than history for the same window, and its cost scales
   with bucket count, not with how chatty the entity is — the property that
   makes a budget predictable.
2. **`history/history_during_period` (WebSocket) is the raw-state source**, with
   `minimal_response` and `no_attributes` always set. It beats REST on both size
   and latency and reuses the existing connection.
3. **REST `/api/history/period` is the documented fallback**, used only when the
   WebSocket route is unavailable, and only with both flags and an explicit
   `end_time` + `filter_entity_id`. An unbounded or full-attribute history query
   is the one this project must never issue: 3.5 MB and 2.8 s for *one* entity
   over 7 days.
4. **Never branch on requested fields.** Statistics silently omit types a metric
   does not have, and `minimal_response` omits `attributes`/`entity_id` on all
   but the first element. Both are `partial`-marker territory in a tool
   response, not fields to dereference.
5. **Feature-detect `mean_type` alongside `has_mean`.** Both ship at 2026.8.3;
   an adapter reading only one of them is betting on which way that pair
   resolves in a later release.
6. The §10 budget defaults can now be anchored for the single-entity case; the
   multi-entity number remains a guess and is called out above.
