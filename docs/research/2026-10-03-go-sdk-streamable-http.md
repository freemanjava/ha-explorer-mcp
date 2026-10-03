# What go-sdk v1.7.0's Streamable HTTP handler already does (P8-08)

Dated snapshot. Read from the module cache at `github.com/modelcontextprotocol/go-sdk@v1.7.0` (the version pinned in
`go.mod`): `mcp/streamable.go`, `mcp/shared.go`, `mcp/logging.go`, `auth/auth.go`. Re-check by reading the same files
at a newer pin; a bump past v1.8.0 removes the `MCPGODEBUG` compatibility switches named below.
Companion to `2026-10-03-mcp-client-paths.md` (why HTTP at all) and phase 08's D-08-4…D-08-11 (what we decided).

### 2026-10-03 · Stateless mode

**Kind:** world-discoverable
**Method:** read `StreamableHTTPOptions` and `serveStateless` (`mcp/streamable.go:128-221`, `:366-440`).
**Found:** `Stateless: true` never reads or sets `Mcp-Session-Id`; each POST gets a temporary session with default
initialization parameters, closed when the request ends. GET and DELETE return **405** with `Allow: POST`. A
server→client *request* (sampling, elicitation, roots) is rejected immediately; notifications made inside a request's
context may still reach the client. Before the body is read it requires `Content-Type: application/json` (else
**415**) and an `Accept` carrying both `application/json` and `text/event-stream` (else **400**). `JSONResponse: true`
answers with `application/json` instead of an SSE stream. The old behavior (honouring session ids in stateless mode)
is behind `MCPGODEBUG=allowsessionsinstateless=1`, removed in v1.9.0.
**Means:** stateless needs no session table, so there is no per-client state to bound or to hijack. Nothing in
`internal/mcp` issues a server→client request (grep for `CreateMessage|Elicit|ListRoots` finds none), so nothing is
lost. Clients must send the spec's dual `Accept`; `curl` probes in `P8-09` have to as well.

### 2026-10-03 · Origin and cross-origin checks

**Kind:** world-discoverable
**Method:** read `ServeHTTP` (`mcp/streamable.go:324-364`) and the `CrossOriginProtection` option doc (`:184-196`).
**Found:** **No Origin check by default.** `CrossOriginProtection` is nil unless set, and is *deprecated* in favour of
wrapping the handler in `http.CrossOriginProtection` middleware; `MCPGODEBUG=enableoriginverification=1` restores
the v1.4.1–v1.5.0 default until v1.8.0. `http.CrossOriginProtection` itself (Go 1.25) admits requests with no
`Origin`/`Sec-Fetch-Site` (non-browser) and **same-origin** browser requests — which under DNS rebinding is exactly
what the attacker's page is, since the rebound host name is its own origin.
**Means:** Origin validation is ours to write; the SDK's option would not satisfy the MCP spec's DNS-rebinding
requirement for a LAN listener anyway. D-08-5.

### 2026-10-03 · Localhost (DNS-rebinding) protection

**Kind:** world-discoverable
**Method:** read `ServeHTTP` (`mcp/streamable.go:325-334`) and `DisableLocalhostProtection` (`:174-182`).
**Found:** on by default: a request whose *local* address is loopback but whose `Host` is not is answered **403**.
It looks only at the accepting socket's address; on a non-loopback socket it does nothing.
**Means:** inert inside the App, where the socket is on the `hassio` bridge interface. Leave it enabled (no reason to
turn off a guard), but do not count it as a control. D-08-5.

### 2026-10-03 · Body limits

**Kind:** world-discoverable
**Method:** read `MaxRequestBodyBytes` (`mcp/streamable.go:198-209`), `DefaultMaxRequestBodyBytes` (`:225`), the
`http.MaxBytesReader` wrap in `ServeHTTP` and the `MaxBytesError` branch in `serveStateless`.
**Found:** default **4 MiB**, enforced during the read (so `Content-Length`, chunked and HTTP/2 alike), answered
**413**. Zero means the default; negative disables it. The SDK sets **no** header size, read, write or idle timeout —
those belong to the `http.Server` the caller constructs — and no concurrency limit.
**Means:** the body cap is one option; every other request limit is `transport_http.go`'s. D-08-7.

### 2026-10-03 · Headers reach the MCP layer

**Kind:** world-discoverable
**Method:** read `RequestExtra` (`mcp/shared.go:603-618`).
**Found:** every request handed to server code carries `Extra.Header http.Header` — **the whole HTTP header set**,
`Authorization` included — and `Extra.TokenInfo` when the SDK's auth middleware ran. There is no remote address.
**Means:** unless the auth middleware removes `Authorization` before the SDK handler, the secret is reachable from
`internal/mcp`'s middleware and from every tool handler — one careless `slog.Any("req", req)` from a log line.
Stripping it is a D-08-4 requirement, asserted in `P8-02`. Recording the caller's IP in the audit record would need
smuggling it through a header or context the SDK does not define; D-08-10 declines that.

### 2026-10-03 · The SDK's bearer middleware

**Kind:** world-discoverable
**Method:** read `auth.RequireBearerToken` and `verify` (`auth/auth.go:97-170`).
**Found:** OAuth-shaped: it calls a `TokenVerifier`, enforces scopes and an expiration (a token without one is
rejected unless `AllowMissingExpiration`), and writes the verifier's error text into the 401 body. It does not strip
the header afterwards.
**Means:** a static shared secret fits it only by faking expiry and scopes, and it would still leave the header in
`Extra.Header`. A ~20-line middleware of our own is smaller than the adaptation. D-08-4.

### 2026-10-03 · Logging

**Kind:** world-discoverable
**Method:** read `ensureLogger` (`mcp/logging.go:105`) and the handler's log calls.
**Found:** a nil `Logger` discards. The handler logs connection failures as `fmt.Sprintf("failed to connect: %v",
err)` — the error, not the request.
**Means:** pass our redacting logger (`mcp.NewLogger` with the secret registered) so SDK lines go through the same
redaction as ours.
