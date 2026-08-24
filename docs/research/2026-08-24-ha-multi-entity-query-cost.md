# Multi-entity history & statistics cost — observed scaling and budget values

Evidence for `P0-09`, resolving **F-14**. Dated snapshot: it says what was true
on this date, with the method that re-checks it. Curated conclusions belong in
`docs/reference/`, not here.

### 2026-08-24 · Do history and statistics scale linearly in the number of entity ids, does batching win, and what budget values follow?

**Kind:** world-discoverable

**Method:** `cmd/spike` at this revision, run by the owner against their live
installation over the LAN:
`HA_URL=http://homeassistant.local:8123 HA_TOKEN=<long-lived token> go run ./cmd/spike`.
Core **2026.8.3**, 478 entities (135 numeric `sensor.*`), 286 statistic ids of
which 200 carry compiled data, authenticated as **admin** — the App's own Core
principal (`P0-06`). Both commands were measured at **1, 10, 50 and 200 entity
ids** over a **24h** and a **7d** window at one fixed end instant; history
always with `minimal_response` + `no_attributes` + `significant_changes_only:
false` (`P0-07` settled that as the only variant worth measuring), statistics
always with `hour` buckets and `types: [mean,min,max,sum,state]`. Each batched
call was issued twice (cold, then warm). The single-entity baseline is each of
the 200 ids measured once and prefix-summed, so a rung of N is compared against
exactly those N ids rather than a re-run. Timings are wall clock from the
owner's workstation and include LAN and JSON decode; they are not in-Core query
times.

## Found — batching wins on time, always, and never costs bytes for history

| source | window | ids | batched bytes | points | cold | warm | N× single bytes | N× single total | batched ÷ singles |
|---|---|--:|--:|--:|--:|--:|--:|--:|--:|
| history | 24h | 10 | 80 656 | 2 189 | 75 ms | 58 ms | 80 665 | 253 ms | 0.30× |
| history | 24h | 50 | 858 229 | 23 140 | 339 ms | 339 ms | 858 278 | 1.414 s | 0.24× |
| history | 24h | 200 | 1 118 065 | 30 260 | 1.499 s | 457 ms | 1 118 264 | 4.677 s | 0.32× |
| history | 7d | 10 | 552 365 | 15 015 | 303 ms | 302 ms | 552 374 | 540 ms | 0.56× |
| history | 7d | 50 | 6 039 937 | 163 144 | 3.418 s | 3.427 s | 6 039 986 | 4.673 s | 0.73× |
| history | 7d | 200 | 7 633 159 | 207 512 | 5.295 s | 4.322 s | 7 633 358 | 8.802 s | 0.60× |
| statistics | 24h | 10 | 10 914 | 92 | 25 ms | 14 ms | 8 721 | 215 ms | 0.12× |
| statistics | 24h | 50 | 16 356 | 138 | 19 ms | 18 ms | 13 137 | 716 ms | 0.03× |
| statistics | 24h | 200 | 131 530 | 1 196 | 50 ms | 48 ms | 101 793 | 2.607 s | 0.02× |
| statistics | 7d | 10 | 78 355 | 668 | 30 ms | 31 ms | 62 338 | 147 ms | 0.20× |
| statistics | 7d | 50 | 117 511 | 1 002 | 37 ms | 39 ms | 93 556 | 643 ms | 0.06× |
| statistics | 7d | 200 | 939 984 | 8 684 | 204 ms | 217 ms | 721 895 | 2.659 s | 0.08× |

**One batched call beat N single-entity calls at every rung of both commands**,
by 1.4× (history, 7d, 50 ids) to 50× (statistics, 24h, 200 ids). The saving is
**round trips, not payload**: history returns the same bytes either way (within
200 B of the summed singles at every rung), and the per-call floor measured here
is 13–23 ms. Batching therefore matters most where the payload per entity is
small — which is exactly the statistics case.

The 1-id rung is not in the table: the first id in the ladder happened to be an
entity with **one** recorded state in 7d, so its 70-byte answer describes that
entity, not a typical one. Per-id means over the 200-id set are the honest
single-entity figures.

## Found — cost tracks recorded rows, not the number of ids

History is **not** linear in entity count. Going from 50 to 200 ids over 24h
added 150 ids but only 260 KB (+30%), because the first 50 of this set contain
the chatty entities and ids 51–200 are nearly silent. Over 7d the same step
added 26% more bytes. **Entity count is a poor proxy for cost; bytes and points
are the real quantity**, and a budget that counts entities alone will let one
chatty sensor through and refuse 150 quiet ones.

Time does carry a per-id component that bytes do not explain: 50 → 200 ids at
24h raised bytes 30% but cold latency 4.4× (339 ms → 1.499 s), while the warm
repeat of the same call took 457 ms. That gap appears only at the widest rung
and only cold, consistent with a per-entity recorder read that the page cache
then absorbs.

Per-entity-day means over the 200-id set, for pre-flight estimation:

| source | bytes / entity-day | points / entity-day | bytes / point |
|---|--:|--:|--:|
| history (`minimal_response`, `no_attributes`) | ~5 600 | ~151 | **~37** |
| statistics (`hour` buckets) | ~670 | ~6 | **~110** |

Both scale near-linearly in window width: 7× the window cost 6.8× the bytes for
history and 7.1× for statistics.

## Found — statistics stay 1–2 orders of magnitude cheaper at fleet width

`P0-07`'s source order survives the widening. At the widest measured query —
200 ids over 7d — statistics cost **940 KB in 204 ms** against history's
**7.63 MB in 5.295 s**: 8× the bytes and 26× the time, for data that answers the
same fleet-wide "which of these is behaving oddly" question. Statistics never
reached 1 s at any rung of either window, and never crossed 512 KB except at
200 ids over 7d.

## Found — the byte cap binds first, everywhere

Where a single batched call crosses doc §10's illustrative limits:

| source | window | > 512 KB (normal) | > 1 MB (composite) | ≥ 1 s cold |
|---|---|--:|--:|--:|
| history | 24h | 50 ids | 200 ids | 200 ids |
| history | 7d | 10 ids | 50 ids | 50 ids |
| statistics | 24h | not crossed | not crossed | not crossed |
| statistics | 7d | 200 ids | not crossed | not crossed |

Rungs are discrete, so each entry is the first *measured* count above the limit.
The pattern is uniform: **the response-size cap is reached before the deadline
and before the entity cap in every case measured**. Every call that stayed
within 512 KB completed in ≤ 339 ms cold; the deadline never bound first.

This has a design consequence beyond the numbers. `MaxBytes` enforced on the
*received* response is enforced after the Pi has already paid for it — the
recorder read, the serialization and the transfer all happened. The budget must
therefore refuse **before issuing**, from an estimate; the per-entity-day means
above are what that estimate is built from.

## Means — starting budget values, attributed to this measurement

For `P2-01`, replacing doc §10's admitted guesses (§26). All derived from the
table above against this installation:

| field | normal read tool | composite diagnostic | derivation |
|---|--:|--:|---|
| `MaxBytes` | **512 KB** | **1 MB** | doc §10's values confirmed as the binding constraint; both are reached well inside the deadline (worst: 1 MB of history in 1.5 s cold) |
| `MaxHistoryPoints` | **13 000** | **26 000** | `MaxBytes` ÷ 37 B/point measured for history; doc §10's 50 000 is unreachable within 1 MB and would never fire |
| `MaxEntities` | **200** | **200** | the widest count measured; both commands answered 200 ids without failure or truncation. Doc §10's composite 500 is **not** supported by this run — nothing above 200 was measured |
| `Deadline` | **10 s** | **30 s** | doc §10's values stand with margin: no call within its byte cap exceeded 339 ms cold |

Pre-flight estimate to refuse an over-budget query before it is issued, in
**entity-days** (entities × window in days):

| source | ≤ 512 KB | ≤ 1 MB |
|---|--:|--:|
| history (`minimal_response`, `no_attributes`) | ~90 entity-days | ~180 entity-days |
| statistics (`hour` buckets) | ~780 entity-days | ~1 560 entity-days |

Read as: a normal read tool may ask history for 90 entities over 1 day, or 12
entities over 7 days, before it is expected to exceed 512 KB. Statistics buys
roughly **8.7× more entity-days for the same bytes**.

Because these are means over a mixed 200-entity set and the set is skewed, the
estimate is a mean and not a bound — a query of a few very chatty entities can
exceed it. `MaxBytes` on the received response stays as the backstop; the
estimate is what keeps the recorder from being asked in the first place.

Both `MaxHistoryPoints` and `MaxBytes` must be enforced: at 37 B/point for
history and 110 B/point for statistics, neither one implies the other.

**Not established:**

- **Why a batched statistics answer is ~30% larger than the same ids fetched one
  at a time** (24h: 131 530 B vs 101 793 B; 7d: 939 984 B vs 721 895 B — same
  point count both times, so it is ~25 B per point of extra serialization, not
  extra data). The ratio is stable across both windows. Filed as **F-17**; the
  larger, batched figure is the one used above, so the estimates are
  conservative either way.
- **Anything above 200 entity ids.** 200 is doc §10's own entity cap and the
  probe's ceiling, so the composite class's illustrative 500 is unmeasured. It is
  not carried forward as a recommendation.
- **Non-admin cost**, still — this run, like `P0-07`'s, was admin-only.
- **Hardware attribution.** Timings are wall clock from the owner's workstation
  over LAN against their installation; the split between recorder I/O, Core CPU
  and transfer is not separated, and a differently-sized recorder DB will move
  the absolute numbers. The *ratios* (batched vs singles, statistics vs history,
  bytes per point) are the durable part.
- **Whether the widest calls degraded the installation** while they ran. Only
  client-side wall clock was recorded; no Core CPU or recorder metric was
  observed during the run.
