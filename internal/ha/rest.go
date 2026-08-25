package ha

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxRESTResponseBytes bounds a single REST response. It is a process safety
// limit, not a budget — response-size budgeting per doc §10 is Phase 02 policy
// work. 8 MiB matches maxCommandFrame and sits above the largest REST body
// P0-07 measured against live HA (7 days of one entity's unfiltered history,
// ~3.5 MB) while staying far below what would threaten a Raspberry Pi running
// Core alongside this binary. A var, not a const, so tests can shorten it;
// nothing in production writes it.
var maxRESTResponseBytes int64 = 8 << 20

// defaultRESTTimeout bounds a request whose caller supplied no deadline,
// mirroring defaultCallTimeout on the WebSocket side. Every upstream call
// carries a deadline (CLAUDE.md, Error Handling) — this is the backstop, not a
// licence to omit one.
var defaultRESTTimeout = 30 * time.Second

// RESTClient reads Core's REST API through the Supervisor proxy. It issues
// GET only: there is no method parameter anywhere in this file, so a mutating
// request is not something the gateway has to catch — it is something no code
// path here can construct (CLAUDE.md rule 1, ADR-008).
//
// Every request is matched against the exact route templates in gateway.go
// before it is built, and every path parameter is validated before it is
// expanded, so no caller-supplied value can select a route.
type RESTClient struct {
	baseURL string
	token   string
	http    *http.Client
	logger  *slog.Logger
}

// NewRESTClient returns a client for Core's REST API rooted at baseURL — the
// Supervisor proxy's http://supervisor/core in production — authenticating
// with token. token is read once by the caller from SUPERVISOR_TOKEN and is
// never logged, never returned in an error, and never stored anywhere else.
func NewRESTClient(baseURL, token string, httpClient *http.Client, logger *slog.Logger) *RESTClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &RESTClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		token:   token,
		http:    httpClient,
		logger:  logger,
	}
}

// HistoryOptions are the recorder-side filters GET /api/history/period accepts.
// They are typed fields, never a free-form query map: a caller cannot add a
// parameter this struct does not name (CLAUDE.md rule 2).
//
// MinimalResponse is the parameter that matters — P0-07 measured it 4.9×
// smaller than an unfiltered window, where NoAttributes alone still emits an
// empty attributes object on every element. Both are opt-in because
// minimal_response collapses consecutive same-state rows: it is a summary, not
// a lossless subset, and the caller must choose that trade knowingly.
type HistoryOptions struct {
	// End bounds the window. Zero means Core's default (one day from start).
	End time.Time
	// EntityIDs restricts the query to these entities. Each is validated;
	// an empty slice asks Core for every recorded entity, which P0-07 shows
	// is the expensive shape.
	EntityIDs              []string
	MinimalResponse        bool
	NoAttributes           bool
	SignificantChangesOnly bool
}

// LogbookOptions are the filters GET /api/logbook accepts.
type LogbookOptions struct {
	// End bounds the window. Zero means Core's default.
	End time.Time
	// EntityID restricts the query to one entity; empty means all.
	EntityID string
}

// Config returns Core's configuration and version.
func (c *RESTClient) Config(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, RouteConfig, RouteConfig, nil)
}

// States returns the current state of every entity.
func (c *RESTClient) States(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, RouteStates, RouteStates, nil)
}

// State returns one entity's current state. entityID is validated against
// Core's entity-id shape before it is expanded into the path, so a traversal
// or escape sequence is refused rather than escaped and sent.
func (c *RESTClient) State(ctx context.Context, entityID string) (json.RawMessage, error) {
	if err := validateEntityID(entityID); err != nil {
		return nil, err
	}
	return c.get(ctx, RouteStateByID, "/api/states/"+entityID, nil)
}

// HistoryPeriod returns recorded state changes from start. The WebSocket
// history/history_during_period command is cheaper for the same data (P0-07);
// this route is the documented fallback for callers that cannot use it.
func (c *RESTClient) HistoryPeriod(ctx context.Context, start time.Time, opts HistoryOptions) (json.RawMessage, error) {
	query := url.Values{}
	if len(opts.EntityIDs) > 0 {
		for _, entityID := range opts.EntityIDs {
			if err := validateEntityID(entityID); err != nil {
				return nil, err
			}
		}
		query.Set("filter_entity_id", strings.Join(opts.EntityIDs, ","))
	}
	if !opts.End.IsZero() {
		query.Set("end_time", formatTimestamp(opts.End))
	}
	// Core treats these as valueless presence flags; the value is ignored.
	if opts.MinimalResponse {
		query.Set("minimal_response", "")
	}
	if opts.NoAttributes {
		query.Set("no_attributes", "")
	}
	if opts.SignificantChangesOnly {
		query.Set("significant_changes_only", "")
	}
	return c.get(ctx, RouteHistoryPeriod, "/api/history/period/"+formatTimestamp(start), query)
}

// LogbookPeriod returns logbook entries from start.
func (c *RESTClient) LogbookPeriod(ctx context.Context, start time.Time, opts LogbookOptions) (json.RawMessage, error) {
	query := url.Values{}
	if opts.EntityID != "" {
		if err := validateEntityID(opts.EntityID); err != nil {
			return nil, err
		}
		query.Set("entity", opts.EntityID)
	}
	if !opts.End.IsZero() {
		query.Set("end_time", formatTimestamp(opts.End))
	}
	return c.get(ctx, RouteLogbookPeriod, "/api/logbook/"+formatTimestamp(start), query)
}

// formatTimestamp renders a time the way Core's history and logbook routes
// parse it. UTC, so the rendering never depends on the App container's zone.
func formatTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// get is the single place this package issues an HTTP request from, so the
// gateway check below is the single place a REST request can be refused.
// template is the allow-listed route; path is that template with its
// parameters already validated and expanded. They are separate arguments
// precisely so the check matches the template — matching an expanded path
// would make the table a pattern rule in disguise.
func (c *RESTClient) get(ctx context.Context, template, path string, query url.Values) (json.RawMessage, error) {
	if err := checkRoute(http.MethodGet, template); err != nil {
		return nil, err
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultRESTTimeout)
		defer cancel()
	}

	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("ha: building request for %s: %w", template, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// The transport error is not wrapped: it can quote the request URL,
		// and a URL is one refactor away from carrying a credential
		// (CLAUDE.md rule 4). The route template says enough.
		return nil, fmt.Errorf("%w: GET %s", ErrUpstreamUnavailable, template)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := statusError(resp.StatusCode, template); err != nil {
		return nil, err
	}

	// Read one byte past the cap: an oversized body is reported, never
	// buffered whole, so a runaway response cannot exhaust the Pi's memory.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRESTResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: GET %s: reading response body", ErrUpstreamUnavailable, template)
	}
	if int64(len(body)) > maxRESTResponseBytes {
		return nil, fmt.Errorf("%w: GET %s: response exceeds %d bytes", ErrResponseTooLarge, template, maxRESTResponseBytes)
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("%w: GET %s: response is not valid JSON", ErrUnexpectedMessage, template)
	}
	return json.RawMessage(body), nil
}

// statusError maps an HTTP status onto this project's sentinels. "Absent"
// (ErrNotFound) and "could not reach" (ErrUpstreamUnavailable) are different
// answers and must stay distinguishable all the way to the MCP response
// (CLAUDE.md, Error Handling); P1-04 consolidates the full taxonomy.
func statusError(status int, template string) error {
	switch {
	case status == http.StatusOK:
		return nil
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return ErrAuthFailed
	case status == http.StatusNotFound:
		return fmt.Errorf("%w: GET %s", ErrNotFound, template)
	default:
		return fmt.Errorf("%w: GET %s: status %d", ErrUpstreamUnavailable, template, status)
	}
}
