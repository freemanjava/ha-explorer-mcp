package ha

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// maxSupervisorResponseBytes bounds a single Supervisor response. It is a process safety
// limit, not a budget — response-size budgeting per doc §10 is Phase 02 policy
// work. 8 MiB matches maxCommandFrame and sits above the largest Supervisor
// body observed while staying far below what would threaten a Raspberry Pi running
// Core alongside this binary. A var, not a const, so tests can shorten it;
// nothing in production writes it.
var maxSupervisorResponseBytes int64 = 8 << 20

// defaultSupervisorTimeout bounds a request whose caller supplied no deadline,
// mirroring defaultCallTimeout on the WebSocket side. Every upstream call
// carries a deadline (CLAUDE.md, Error Handling) — this is the backstop, not a
// licence to omit one.
var defaultSupervisorTimeout = 30 * time.Second

// defaultSupervisorBaseURL is where the Supervisor API is reachable from
// inside an App container (docs/research/2026-08-23-supervisor-permissions.md).
const defaultSupervisorBaseURL = "http://supervisor"

// SupervisorClient reads Supervisor's own REST API. It issues GET only
// (CLAUDE.md rule 1, ADR-008): there is no method parameter anywhere in this
// file. Core is reached over the WebSocket only; no Core REST client exists.
//
// Every request is matched against allowedSupervisorRoutes before it is
// built. Supervisor being unreachable is reported as ErrUnsupported, not
// ErrUpstreamUnavailable: a Core-based diagnostic must keep working with
// Supervisor absent (CLAUDE.md, Reliability), so its failure mode is
// "degrade", not "the same outage as losing Core".
type SupervisorClient struct {
	baseURL string
	token   string
	http    *http.Client
	logger  *slog.Logger
}

// NewSupervisorClient returns a client for Supervisor's API rooted at
// baseURL — defaultSupervisorBaseURL in production — authenticating with
// token. token is read once by the caller from SUPERVISOR_TOKEN and is never
// logged, never returned in an error, and never stored anywhere else.
func NewSupervisorClient(baseURL, token string, httpClient *http.Client, logger *slog.Logger) *SupervisorClient {
	if baseURL == "" {
		baseURL = defaultSupervisorBaseURL
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &SupervisorClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		token:   token,
		http:    httpClient,
		logger:  logger,
	}
}

// SupervisorInfo returns Supervisor's own status and installed-App inventory,
// mapped to internal/model. A mutated response shape fails loudly rather than
// being coerced into garbage (P1-08 DoD) — see MapSupervisorInfo.
func (c *SupervisorClient) SupervisorInfo(ctx context.Context) (model.SupervisorInfo, error) {
	raw, err := c.get(ctx, SupervisorRouteSupervisorInfo)
	if err != nil {
		return model.SupervisorInfo{}, err
	}
	return MapSupervisorInfo(raw)
}

// CoreInfo returns Supervisor's /info mapped to model.CoreInfo — get_system_
// health's component versions, hostname, machine, arch and Core's run state,
// granted whether or not hassio_api is set (api_bypass).
func (c *SupervisorClient) CoreInfo(ctx context.Context) (model.CoreInfo, error) {
	raw, err := c.get(ctx, SupervisorRouteInfo)
	if err != nil {
		return model.CoreInfo{}, err
	}
	return MapCoreInfo(raw)
}

// OSHealth returns Supervisor's /os/info mapped to model.OSInfo.
func (c *SupervisorClient) OSHealth(ctx context.Context) (model.OSInfo, error) {
	raw, err := c.get(ctx, SupervisorRouteOSInfo)
	if err != nil {
		return model.OSInfo{}, err
	}
	return MapOSInfo(raw)
}

// HostDisk returns the disk fields of Supervisor's /host/info mapped to
// model.HostDisk.
func (c *SupervisorClient) HostDisk(ctx context.Context) (model.HostDisk, error) {
	raw, err := c.get(ctx, SupervisorRouteHostInfo)
	if err != nil {
		return model.HostDisk{}, err
	}
	return MapHostDisk(raw)
}

// ResolutionSummary returns Supervisor's /resolution/info mapped to
// model.ResolutionSummary.
func (c *SupervisorClient) ResolutionSummary(ctx context.Context) (model.ResolutionSummary, error) {
	raw, err := c.get(ctx, SupervisorRouteResolutionInfo)
	if err != nil {
		return model.ResolutionSummary{}, err
	}
	return MapResolutionInfo(raw)
}

// SelfStats returns this App's own container resource use, mapped to
// model.AddonStats — never another App's (that needs the manager role,
// deliberately not requested).
func (c *SupervisorClient) SelfStats(ctx context.Context) (model.AddonStats, error) {
	raw, err := c.get(ctx, SupervisorRouteAddonSelfStats)
	if err != nil {
		return model.AddonStats{}, err
	}
	return MapAddonStats(raw)
}

// get is the single place this package issues a Supervisor HTTP request from,
// so checkSupervisorRoute is the single place such a request can be refused.
func (c *SupervisorClient) get(ctx context.Context, route string) (json.RawMessage, error) {
	if err := checkSupervisorRoute(http.MethodGet, route); err != nil {
		return nil, err
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultSupervisorTimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+route, nil)
	if err != nil {
		return nil, fmt.Errorf("ha: building Supervisor request for %s: %w", route, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// Not wrapped: an http.Client transport error can quote the request
		// URL, and a URL is one refactor away from carrying a credential
		// (CLAUDE.md rule 4). ctx.Err() is checked separately: it is a fixed
		// stdlib string, safe to wrap, and tells apart "our own deadline"
		// from "Supervisor unreachable".
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("%w: GET %s", wrapDeadline(ctxErr), route)
		}
		return nil, fmt.Errorf("%w: Supervisor unreachable: GET %s", ErrUnsupported, route)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := supervisorStatusError(resp.StatusCode, route); err != nil {
		return nil, err
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSupervisorResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: GET %s: reading response body", ErrUpstreamUnavailable, route)
	}
	if int64(len(body)) > maxSupervisorResponseBytes {
		return nil, fmt.Errorf("%w: GET %s: response exceeds %d bytes", ErrResponseTooLarge, route, maxSupervisorResponseBytes)
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("%w: GET %s: response is not valid JSON", ErrUnexpectedMessage, route)
	}
	return unwrapSupervisorEnvelope(body, route)
}

// supervisorEnvelope is the {"result","data"} wrapper every Supervisor route
// answers with (D-08-13; observed on /info, docs/research/2026-10-04-supervisor-response-shape.md).
type supervisorEnvelope struct {
	Result string          `json:"result"`
	Data   json.RawMessage `json:"data"`
}

// unwrapSupervisorEnvelope returns the envelope's data object. Supervisor's
// own "message" field is deliberately not decoded: it is upstream text
// (CLAUDE.md rule 6) and must not reach an error string.
func unwrapSupervisorEnvelope(body []byte, route string) (json.RawMessage, error) {
	var env supervisorEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("%w: GET %s: response is not an envelope object", ErrUnexpectedMessage, route)
	}
	if env.Result != "ok" {
		return nil, fmt.Errorf("%w: Supervisor GET %s: result is not ok", ErrUnsupported, route)
	}
	if len(env.Data) == 0 || env.Data[0] != '{' {
		return nil, fmt.Errorf("%w: GET %s: envelope has no data object", ErrUnexpectedMessage, route)
	}
	return env.Data, nil
}

// supervisorStatusError maps Supervisor's HTTP status onto this project's
// sentinels. A non-2xx status is reported as
// ErrUnsupported rather than ErrUpstreamUnavailable: Supervisor refusing or
// erroring on a role-permitted route is "this cannot be answered by this
// connection" for a diagnostic tool, not "Core reads are broken too"
// (CLAUDE.md, Reliability). Missing or wrong credentials still surface as
// ErrAuthFailed, since that is a configuration problem, not a Supervisor
// outage.
func supervisorStatusError(status int, route string) error {
	switch status {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: GET %s", ErrAuthFailed, route)
	case http.StatusNotFound:
		return fmt.Errorf("%w: GET %s", ErrNotFound, route)
	default:
		return fmt.Errorf("%w: Supervisor GET %s: status %d", ErrUnsupported, route, status)
	}
}
