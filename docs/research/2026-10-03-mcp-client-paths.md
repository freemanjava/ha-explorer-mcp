# MCP client paths to an HA-hosted server (F-41, D-08-1)

Dated snapshot. Sources were read at the commits named below; re-check by reading the same files at a newer commit.
Companion to `2026-10-03-app-under-supervisor.md` (what the App does under Supervisor).

### 2026-10-03 · How does Home Assistant's own MCP server reach clients?

**Kind:** world-discoverable
**Method:** read `homeassistant/components/mcp_server/{http,server,config_flow}.py` in `home-assistant/core` at
`3d38905ff741` (sparse clone); read https://www.home-assistant.io/integrations/mcp_server/.
**Found:** it is a Core **integration**, not an App. It registers HTTP views on Core's own web server: Streamable HTTP
`POST /api/mcp` and `/api/mcp/<api_id>` in **stateless** mode (a fresh MCP server per request, 60 s timeout), plus the
legacy SSE pair `/mcp_server/sse` + `/mcp_server/messages/{session_id}`. Authentication is Core's own: OAuth via
IndieAuth (client id = the client's base URL, no registration, no secret) or a long-lived access token. Authorization is
Core's own: the config entry defaults to `require_admin: True`; any API other than Assist requires an admin user
(`_validate_admin`, `ModelContextProtocolStreamableApiView.post`). Tools are the LLM API's tools (Assist by default),
scoped by the "exposed entities" page, with `readOnlyHint`/`destructiveHint` annotations passed through. Unknown JSON-RPC
methods are answered `METHOD_NOT_FOUND` before a server is built. Stdio-only clients are told to run `mcp-proxy`
locally (`--transport=streamablehttp --stateless`).
**Means:** HA's answer to "how does a client reach it" is *put the server inside the process that already owns the
port, TLS, users and tokens*. It adds no listener and no credential of its own. That option does not exist for a
separate Go App without reopening the architecture (an integration runs Python inside Core).

### 2026-10-03 · Can a client reach an App's stdio through Supervisor (`stdin: true`)? — D-08-1 option (d)

**Kind:** world-discoverable
**Method:** read `home-assistant/supervisor` at `bdcba61fc7c1`: `supervisor/api/apps.py:517` (`stdin` handler),
`supervisor/docker/app.py:985` (`write_stdin`), `supervisor/api/__init__.py:768,812` (routes),
`supervisor/api/middleware/security.py` (role regexes).
**Found:** `POST /apps/{app}/stdin` (alias `/addons/{app}/stdin`) is **write-only**: it attaches with `stdin=True`, writes
the body plus `\n`, closes the socket, and returns an empty 200. Nothing reads the container's stdout back; stdout goes
only to the container log. By the role regexes, `/addons/<slug>/stdin` is reachable for `manager` (`/addons/<slug>/(?!security).+`)
and `admin` (`.*`), not for `default` — so only a manager/admin Supervisor token, or Core via its `hassio` stdin action,
can write. `stdin: true` does make Docker keep stdin open
(`stdin_open=self.app.with_stdin`, line 711), so the process would no longer exit on EOF.
**Means:** option (d) is **void**. It would keep the App running but give no client a response channel: replies would
land in the App log, interleaved with nothing a client can consume. A JSON-RPC client cannot be built on it.

### 2026-10-03 · Can an external MCP client use Supervisor Ingress?

**Kind:** world-discoverable
**Method:** read `supervisor/api/ingress.py` at the same commit.
**Found:** the Ingress handler requires an `ingress_session` cookie validated by `sys_ingress.validate_session`
(line 146); sessions are created through `POST /ingress/session`, which the HA frontend calls over Core's WebSocket for
a logged-in user. Requests are proxied with `X-Remote-User-*` headers.
**Not established:** whether any MCP client can be made to obtain and refresh an ingress cookie — none documents it,
and the session is a browser-UI construct with a short lifetime.
**Means:** Ingress is not a usable MCP client path without a custom shim on the client side. Not an option as-is.

### 2026-10-03 · Is `docker` absent from the SSH App only because of protection mode? — F-41 (1)

**Kind:** world-discoverable
**Method:** `curl` of `home-assistant/addons/master/ssh/config.yaml` (official "Terminal & SSH", 10.5.0) and
`hassio-addons/addon-ssh/main/ssh/config.yaml` (community "Advanced SSH & Web Terminal").
**Found:** the official App declares `hassio_role: manager` and **no** `docker_api` — protection mode is irrelevant; it
has no Docker access to lower. The community App declares `docker_api: true` and `privileged:` capabilities; its Docker
CLI works only with protection mode off.
**Means:** option (a) means installing a *second*, community SSH App with Docker socket access and protection mode off —
root-equivalent on the host, a far weaker posture than ADR-004/005 allow this App. Not a missing setting.

### 2026-10-03 · How do community HA MCP Apps expose themselves?

**Kind:** world-discoverable
**Method:** read `homeassistant-ai/ha-mcp` at `3a66ec5055fd`: `homeassistant-addon/config.yaml`,
`homeassistant-addon-webhook-proxy/{config.yaml,DOCS.md}`.
**Found:** the main App is Streamable HTTP on a published port (`9583/tcp`) with `host_network: true`,
`hassio_role: manager`, and the credential is a **secret URL path** (`secret_path`). Its companion "webhook proxy" App
adds `map: config:rw`, installs a custom integration into `/config`, and forwards an HA webhook
(`/api/webhook/mcp_<random>`) to the App, with OAuth marked beta and **off** by default.
**Means:** the de-facto community pattern is "listener + bearer-in-URL", reached remotely by piggy-backing on Core. Each
piece breaks a rule here (host network, `/config`, manager role, URL-as-secret) — a counter-example, not a template.

### 2026-10-03 · What does the MCP spec require of the transports?

**Kind:** world-discoverable
**Method:** read https://modelcontextprotocol.io/specification/2025-11-25/basic/transports and
`…/security_best_practices`.
**Found:** "Clients SHOULD support stdio whenever possible." Streamable HTTP: servers "MUST validate the `Origin` header",
"SHOULD bind only to localhost" when local, "SHOULD implement proper authentication for all connections". Session ids
MUST NOT be used for authentication. Token passthrough is forbidden: servers "MUST NOT accept any tokens that were not
explicitly issued for the MCP server". Local servers SHOULD "use the stdio transport to limit access to just the MCP
client", or for HTTP "require an authorization token" / use unix sockets.
**Means:** if this App ever listens, it must validate `Origin`, require its own credential (not an HA token passed
through to Core), and run stateless or with non-authenticating session ids.

### Summary for D-08-1

| option | status after this verify | credential on the client | what a leaked credential can do |
|---|---|---|---|
| (a) `docker exec -i` via SSH | real, but needs the community SSH App, `docker_api`, protection off | SSH key | root on the HAOS host |
| (b) binary off-box, stdio, Core WS | real (stdio works off-box, P8-01) | HA long-lived token | everything that HA user can do — writes included |
| (c) HTTP in the App | real; new listener, needs auth + Origin check | App-issued secret | call the read-only tools under the App's privacy profile |
| (d) `stdin: true` | **void** — Supervisor stdin is write-only | — | — |
| (e) Core integration (HA's own model) | real only by reopening the architecture | HA OAuth / token | that user's HA rights |

**Not established:** whether every reader the tools use works for a non-admin HA user (relevant to (b) with a narrower
token; F-34 already shows one gap). Whether the 17:03 start in the earlier file was manual (F-41 (3)) is moot once (d)
is void.
