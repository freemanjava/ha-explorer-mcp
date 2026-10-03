# Composite budget measurement, outage chaining, restart evidence (P5-10)

Observed 2026-10-03 against Home Assistant **2026.9.4**, admin principal, 521
entities, 35 config entries, 45 enabled automations. Supervisor not wired (it is
unreachable outside an App). Instruments: `cmd/measure` (new; real tools
in-process, cost read from the server's own audit record) and `cmd/spike`'s
`logbook/get_events` probe. Report text carried counts and ordinal labels only.

### How much does one `find_stale_entities` call cost? (F-26)

**Kind:** world-discoverable
**Method:** `go run ./cmd/measure` — ten consecutive pages, `limit=200`, `period=7d`, following the cursor.
**Found:** every page spent exactly **20 / 20 HA requests** and examined **20** candidates (not 200), with `truncated=true`. Result size 276–1 932 B (limit 524 288). Server time 178–540 ms per page. After ten pages 200 candidates were examined and the cursor was still open. 0–2 stale entities per page, 5 in total.
**Not established:** the number of candidate entities in total (the cursor never ran out); the cost at `ClassComposite` (50 requests) was not run — it is arithmetic from the same per-entity cost, not a measurement.
**Means:** the binding limit is the **request count**, one recorder read per entity. Bytes are ~270× under their cap and 20 requests take 0.2–0.5 s against a 10 s deadline. Re-classing to composite would raise a page from 20 to 50 candidates (2.5×) and nothing else; covering the installation still takes many cursor calls either way.

### What do the `analyze_*` tools cost at real width? (F-26)

**Kind:** world-discoverable
**Method:** `go run ./cmd/measure` — widest config entries at 7d (and 24h for the widest), three unavailable entities, three enabled automations.
**Found:**

| tool | target | HA requests / 50 | bytes / 1 MiB | server ms |
|---|---|--:|--:|--:|
| `analyze_integration_health` | 284 entities, 19 unavailable, 7d | 36 | 14 222 | 945 |
| `analyze_integration_health` | same, 24h | 36 | 9 697 | 736 |
| `analyze_integration_health` | 100 entities, 2 unavailable (two entries) | 26 | 2 023 | 288–315 |
| `analyze_entity_health` | three unavailable entities | 2 | 1 160–1 447 | 28–33 |
| `analyze_automation_health` | three automations | 12 | 4 707–4 714 | 188–224 |

No call was refused or truncated.
**Not established:** the widest integration was 284 entities and the heaviest used 36 of 50 requests, so the headroom is 14, not a ceiling. What drives the 10-request step from 26 to 36 is not read from the data (it matches the mesh-read cap of 10, which is an inference). Supervisor's own requests are absent from every row and would add to them in production.
**Means:** `ClassComposite`'s byte and deadline limits are far from binding. Its request limit holds on this installation with 28% headroom at the widest integration.

### Do long outages chain clusters? (F-28)

**Kind:** world-discoverable
**Method:** per-cluster entity count, outage periods and span from the `analyze_integration_health` responses above.
**Found:** one analysed integration reported clusters: four, each of 4 entities and 4 outage periods, spans 11–149 s, **0% of the 7d period**. The other analyses reported none.
**Not established:** nothing here is a long outage; a window with a multi-hour outage in it was not observed, so chaining is neither seen nor ruled out. One installation, one week.
**Means:** no evidence of chaining on this installation. The case F-28 fears did not occur.

### Is an HA restart visible in the logbook? (F-31)

**Kind:** world-discoverable
**Method:** `logbook/get_events`, 7d window, with `entity_ids` omitted and with `entity_ids: []`.
**Found:** both forms answered identically: **45 672 rows, 10 844 564 B, 3.7–4.8 s**. Four rows had `domain` = `homeassistant`, messages `started` ×2 and `stopped` ×2. Row fields: `when`, `name`, `message`, `domain`, `entity_id`, `state`, `source`, `icon`, `context_*`.
**Not established:** whether a narrower window cuts the size (it should; unmeasured), and whether a restart row can be requested without pulling the rest. The shipped `logbookGetEventsCommand` always sends `entity_ids` and so cannot select these rows today.
**Means:** restart evidence **exists and is readable**. Reading it unfiltered is ~10× the composite byte cap, so it needs a bounded window and a command change, both for `P5-09` to decide.
