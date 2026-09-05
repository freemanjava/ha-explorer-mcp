package ha

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// defaultSupervisorBaseURL is where the Supervisor API is reachable from
// inside an App container (docs/research/2026-08-23-supervisor-permissions.md).
const defaultSupervisorBaseURL = "http://supervisor"

// SupervisorClient reads Supervisor's own REST API — distinct from RESTClient,
// which reads Core through the Supervisor proxy. It issues GET only, exactly
// like RESTClient, for the same reason (CLAUDE.md rule 1, ADR-008): there is
// no method parameter anywhere in this file.
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

// Info returns Supervisor's /info: supervisor/core/OS/docker versions,
// hostname, arch and Core's state as Supervisor sees it. Granted whether or
// not hassio_api is set (api_bypass).
func (c *SupervisorClient) Info(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, SupervisorRouteInfo)
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

// OSInfo returns Supervisor's /os/info.
func (c *SupervisorClient) OSInfo(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, SupervisorRouteOSInfo)
}

// HostInfo returns Supervisor's /host/info.
func (c *SupervisorClient) HostInfo(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, SupervisorRouteHostInfo)
}

// ResolutionInfo returns Supervisor's /resolution/info — issues, checks and
// suggestions.
func (c *SupervisorClient) ResolutionInfo(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, SupervisorRouteResolutionInfo)
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

// NetworkInfo returns Supervisor's /network/info.
func (c *SupervisorClient) NetworkInfo(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, SupervisorRouteNetworkInfo)
}

// HardwareInfo returns Supervisor's /hardware/info.
func (c *SupervisorClient) HardwareInfo(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, SupervisorRouteHardwareInfo)
}

// JobsInfo returns Supervisor's /jobs/info — currently running Supervisor
// jobs.
func (c *SupervisorClient) JobsInfo(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, SupervisorRouteJobsInfo)
}

// AddonSelfInfo returns this App's own manifest, version and state.
func (c *SupervisorClient) AddonSelfInfo(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, SupervisorRouteAddonSelfInfo)
}

// AddonSelfStats returns this App's own container resource use — never
// another App's (that needs the manager role, deliberately not requested).
func (c *SupervisorClient) AddonSelfStats(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, SupervisorRouteAddonSelfStats)
}

// Ping reports whether Supervisor answers its liveness endpoint. It carries
// no security check on Supervisor's side, so a failure here means Supervisor
// itself is unreachable, not a permission problem.
func (c *SupervisorClient) Ping(ctx context.Context) error {
	_, err := c.get(ctx, SupervisorRoutePing)
	return err
}

// get is the single place this package issues a Supervisor HTTP request from,
// so checkSupervisorRoute is the single place such a request can be refused.
func (c *SupervisorClient) get(ctx context.Context, route string) (json.RawMessage, error) {
	if err := checkSupervisorRoute(http.MethodGet, route); err != nil {
		return nil, err
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultRESTTimeout)
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRESTResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: GET %s: reading response body", ErrUpstreamUnavailable, route)
	}
	if int64(len(body)) > maxRESTResponseBytes {
		return nil, fmt.Errorf("%w: GET %s: response exceeds %d bytes", ErrResponseTooLarge, route, maxRESTResponseBytes)
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("%w: GET %s: response is not valid JSON", ErrUnexpectedMessage, route)
	}
	return json.RawMessage(body), nil
}

// supervisorStatusError maps Supervisor's HTTP status onto this project's
// sentinels. Unlike statusError (Core), a non-2xx status here is reported as
// ErrUnsupported rather than ErrUpstreamUnavailable: Supervisor refusing or
// erroring on a role-permitted route is "this cannot be answered by this
// connection" for a diagnostic tool, not "Core reads are broken too"
// (CLAUDE.md, Reliability). Missing or wrong credentials still surface as
// ErrAuthFailed, since that is a configuration problem, not a Supervisor
// outage.
func supervisorStatusError(status int, route string) error {
	switch {
	case status == http.StatusOK:
		return nil
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return fmt.Errorf("%w: GET %s", ErrAuthFailed, route)
	case status == http.StatusNotFound:
		return fmt.Errorf("%w: GET %s", ErrNotFound, route)
	default:
		return fmt.Errorf("%w: Supervisor GET %s: status %d", ErrUnsupported, route, status)
	}
}
