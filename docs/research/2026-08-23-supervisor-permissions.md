# Supervisor endpoints under minimal App permissions — derived

**Task:** `P0-06` · resolves **F-4** (and closes the residue F-2 left open)
**Established:** 2026-08-23
**Method:** source reading of the two authorities that actually decide the
answer, at pinned versions — **not** a live call. See *Not established* below.

| authority | version | file |
|---|---|---|
| Supervisor | `2026.08.0` (latest release, published 2026-08-19) | `supervisor/api/middleware/security.py`, `supervisor/api/__init__.py`, `supervisor/api/{root,supervisor,homeassistant,host,os,apps,proxy}.py`, `supervisor/const.py`, `supervisor/apps/validate.py` |
| Core | `2026.8.3` (the owner's installation, per `P0-04`) | `homeassistant/components/hassio/{__init__,const,websocket_api}.py` |

Fetched from `raw.githubusercontent.com` at those exact tags. Re-check command:

```sh
curl -s https://raw.githubusercontent.com/home-assistant/supervisor/2026.08.0/supervisor/api/middleware/security.py
```

> **Why source and not a probe.** Supervisor's API is reachable only from inside
> an App container (`http://supervisor`), and this App is not deployed yet —
> deployment is `P0-08`'s subject. The permission decision is made by one regex
> table in one middleware, which is a stronger and more complete answer than
> probing a handful of paths would give; but it is a *derivation*, and the live
> confirmation is named at the end of this file.

## How Supervisor actually decides

`SecurityMiddleware.token_validation` (`api/middleware/security.py`) applies, in
this order, per API version (`/…` = v1, `/v2/…` = v2 — same structure, same
roles):

1. **`BLACKLIST`** — `/core/api/hassio…` and `/homeassistant/api/hassio…` are
   `403` for everyone, always. The App cannot reach Core's own hassio API
   through the proxy.
2. **`no_security_check`** — matched paths skip token validation entirely:
   `/core/api/*`, `/core/websocket`, `/homeassistant/api/*`,
   `/homeassistant/websocket`, `/supervisor/ping`, `/ingress/*`, frontend assets.
3. **`api_bypass`** — matched paths are allowed **for any installed App, whatever
   its `hassio_api` setting**: `/addons/self/<x>` (any single segment except
   `security` and `update`), `/addons/self/options/config`, **`/info`**,
   `/services*`, `/discovery*`, `/auth`.
4. **role check** — only if `hassio_api: true`; the App's `hassio_role`
   (`supervisor/apps/validate.py`, default `default`, one of
   `default|homeassistant|backup|manager|admin`) selects a regex:
   - `default` → `^/.+/info$` — *any* endpoint whose path ends in `/info`
   - `homeassistant` → the above **+** `/core/.+`, `/homeassistant/.+`
   - `backup` → the above `/…/info` **+** `/backups.*`
   - `manager` → a long list including `/addons…`, `/core/.+`, `/host/.+`,
     `/os/…`, `/store.*`, `/supervisor/.+`, `/docker/.+`, `/network/.+`
   - `admin` → `.*`
5. otherwise **`403 Forbidden`** (Supervisor logs `missing API permission`).

Two consequences that are easy to get wrong:

- `default` is not "the info endpoints of the system components" — it is
  literally *any path ending in `/info`*, including `/addons/<slug>/info`.
- Conversely `default` grants **no** `*/stats` and **no** collection endpoint:
  `/addons` (the App list) does not end in `/info`, so it needs `manager`.

## The matrix — what each tool needs

`get_system_health` (doc §9 #2) and `list_apps` (doc §9 #18) are the only tools
that want Supervisor. Every row is `GET`.

| endpoint | `hassio_api: false` | `+ role default` | first role that grants it | carries |
|---|:--:|:--:|---|---|
| `/supervisor/ping` | ✅ no token needed | ✅ | — (`no_security_check`) | liveness |
| `/info` | ✅ (`api_bypass`) | ✅ | — (`api_bypass`) | supervisor / core / OS / docker versions, hostname, operating_system, machine, machine_id, arch, supported_arch, core `state`, `supported`, channel, logging, timezone, host features |
| `/addons/self/info` | ✅ (`api_bypass`) | ✅ | — (`api_bypass`) | this App's own manifest, version, state |
| `/addons/self/stats` | ✅ (`api_bypass`) | ✅ | — (`api_bypass`) | **own container only**: cpu_percent, memory_usage/limit/percent, network_rx/tx, blk_read/write |
| `/supervisor/info` | ⛔ | ✅ | `default` | supervisor version + latest + update_available, channel, arch, `supported`, `healthy`, timezone, logging, diagnostics, auto_update, feature flags, **`addons[]`** (name, slug, version, version_latest, update_available, state, repository, icon) and `addons_repositories[]` |
| `/os/info` | ⛔ | ✅ | `default` | OS version/latest/pending, update_available, board, boot slot, data disk, boot_slots |
| `/host/info` | ⛔ | ✅ | `default` | kernel, operating_system, chassis, virtualization, cpe, deployment, **disk_free / disk_total / disk_used / disk_life_time**, hostname, timezone, dt_utc, dt_synchronized, use_ntp, boot_timestamp, startup_time, features, apparmor_version, agent_version |
| `/resolution/info` | ⛔ | ✅ | `default` | `issues[]`, `checks[]`, `suggestions[]`, unsupported/unhealthy reasons |
| `/network/info` | ⛔ | ✅ | `default` | interfaces, connectivity |
| `/hardware/info` | ⛔ | ✅ | `default` | devices, drives |
| `/jobs/info` | ⛔ | ✅ | `default` | running Supervisor jobs |
| `/addons/<slug>/info` | ⛔ | ✅ | `default` | one App's full record — **but the slug must already be known** |
| `/core/info` | ⛔ | ⛔ | `homeassistant` | core version/latest, machine, arch, image, boot, port, ssl, watchdog |
| `/core/stats` | ⛔ | ⛔ | `homeassistant` | **Core CPU / RAM** |
| `/supervisor/stats` | ⛔ | ⛔ | `manager` | Supervisor CPU / RAM |
| `/addons` | ⛔ | ⛔ | `manager` | the App list (superset of `/supervisor/info`'s: adds description, stage, available, detached, homeassistant_version, build, url, system_managed) |
| `/addons/<slug>/stats` | ⛔ | ⛔ | `manager` | another App's CPU / RAM |
| `/available_updates`, `/store*`, `/host/logs*`, `/supervisor/logs*` | ⛔ | ⛔ | `manager` | — |

**The load-bearing row is `/supervisor/info`.** It ends in `/info`, so the
`default` role reaches it, and it already embeds the installed-App inventory
with state and version. `list_apps` therefore does **not** need `manager` — the
`/addons` collection endpoint is a richer duplicate, not a requirement.

### Against the current manifest (`addon/config.yaml`, `P0-02`)

`homeassistant_api: true`, `hassio_api: false`, no `hassio_role`.

- **`get_system_health` is partially implementable with no change at all.**
  `/info` alone yields every version (Core, Supervisor, OS, Docker), the
  hostname, machine, arch, the Supervisor's view of Core `state`, and the
  `supported` flag; `/addons/self/stats` yields this process's own resource use.
  What is *not* reachable without a change: Core CPU/RAM, host disk, and the
  repair/unsupported list.
- **`list_apps` is not implementable at all** under the current manifest. There
  is no enumeration path: `api_bypass` reaches only `self`, and every collection
  endpoint is role-gated. The tool would have to answer `unsupported`.

The delta needed is `hassio_api: true` with the **default** role — which is what
`hassio_role` already is when unset, so the manifest change is exactly one line.
That is written up as a `needs-decision` entry in
`docs/development/phases/00-spike-foundations.md`; it is **not** applied here.

## Two results outside the question that change other tasks

### 1. Everything the App sends to Core runs as an **admin** user

`api/proxy.py` `_check_access` only verifies the caller is an App with
`homeassistant_api: true`; it then forwards through
`sys_homeassistant.api.make_request` / `connect_websocket`, i.e. with
**Supervisor's own Core token**, for both REST and the WebSocket pipe. In Core
`2026.8.3`, `homeassistant/components/hassio/__init__.py` creates that principal
with `async_create_system_user(HASSIO_USER_NAME, group_ids=[GROUP_ID_ADMIN])`
and re-asserts `GROUP_ID_ADMIN` on every setup.

**Means:** the admin-gated commands catalogued by `P0-04`
(`config_entries/get_single`) and `P0-05` (every automation and trace command)
are reachable from this App. `get_automation` and `get_automation_traces` exist
in v1 rather than being fallback-only — the branch F-11 describes remains
required, because an operator can still deploy against a non-Supervisor Core,
but it is the degraded branch, not the expected one. This is the residue F-2
explicitly left open, and it closes here.

It also means the App's Core principal is *more* privileged than the App's
Supervisor principal, and that read-only-ness on the Core side is guaranteed by
this project's own gateway (CLAUDE.md rule 1) and by nothing else.

### 2. `hassio_api: false` is not a boundary against an App that has Core access

Core registers the WebSocket command **`supervisor/api`**
(`hassio/websocket_api.py`, `WS_TYPE_API = "supervisor/api"`), which takes a free
`endpoint` **and a free `method`** and calls Supervisor with Core's own token.
It is gated on `connection.user.is_admin` — which, by result 1, the App is.

**Means:** an App with `homeassistant_api: true` can reach *any* Supervisor
endpoint, with any HTTP method, regardless of its own `hassio_api` and
`hassio_role`. The manifest's Supervisor permissions are a statement of intent,
not an enforced ceiling. Two consequences for this project, filed as **F-13**:
Phase 01's gateway must deny `supervisor/api` explicitly (it is a write path and
a universal escape hatch — CLAUDE.md rules 1 and 2, in one command name), and no
document may claim `hassio_api: false` *prevents* Supervisor access. Note that
Home Assistant blocks the HTTP-shaped version of exactly this hole
(`BLACKLIST`, with a comment about the loopback proxy), which is what makes the
WebSocket route worth treating as an oversight to route around rather than a
feature to use.

## Not established

- **No live call was made.** The matrix is derived from the middleware's regexes
  and the route table, not observed against the owner's Supervisor. The
  derivation is mechanical — one regex per role, matched against a literal path —
  so the risk is not misreading it but that the deployed Supervisor is a
  different version than `2026.08.0`.
- **The confirming probe, for the first live deployment** (`P0-08` or later),
  from inside the App container, is three curls and needs no new code:

  ```sh
  curl -sS -o /dev/null -w '%{http_code} /info\n'            http://supervisor/info            -H "Authorization: Bearer $SUPERVISOR_TOKEN"
  curl -sS -o /dev/null -w '%{http_code} /supervisor/info\n' http://supervisor/supervisor/info -H "Authorization: Bearer $SUPERVISOR_TOKEN"
  curl -sS -o /dev/null -w '%{http_code} /addons\n'          http://supervisor/addons          -H "Authorization: Bearer $SUPERVISOR_TOKEN"
  ```

  Expected under the current manifest: `200`, `403`, `403`. Expected after the
  proposed one-line delta: `200`, `200`, `403`.
- **Result 1 is likewise derived.** The one-line live confirmation is
  `auth/current_user` over the proxy WebSocket from inside the container; it
  should report `is_admin: true` and the Supervisor system user.
- **`/supervisor/info`'s `addons[]` field set was read from the response builder,
  not from a response.** Element schemas of empty collections (e.g.
  `addons_repositories` on a store-less installation) are unobserved, as in
  `P0-04`.
- Supervisor's own version on the owner's installation is unknown — `P0-04`
  recorded Core `2026.8.3` only. `/info` reports it, and the probe above gets it
  for free.
