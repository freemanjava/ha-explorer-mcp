package ha

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// countingServer returns a test server that records how many requests reached
// it, so a denial can be asserted on the wire rather than on the return value
// alone (phase 01 DoD).
func countingServer(t *testing.T, h http.HandlerFunc) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var got atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Add(1)
		if h != nil {
			h(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func testClient(t *testing.T, srv *httptest.Server) *RESTClient {
	t.Helper()
	return NewRESTClient(srv.URL, testToken, srv.Client(), nil)
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// TestNonGetMethodDenied — every mutating method is refused by the route
// gateway, for a route that is otherwise allow-listed. The refusal is a
// policy decision made before any request object exists.
func TestNonGetMethodDenied(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			err := checkRoute(method, RouteConfig)
			if !errors.Is(err, ErrPolicyDenied) {
				t.Fatalf("checkRoute(%s, %s): got %v, want ErrPolicyDenied", method, RouteConfig, err)
			}
			if !strings.Contains(err.Error(), "method") {
				t.Fatalf("denial reason %q does not name the method check", err)
			}
		})
	}
}

// TestNoNonGetRequestPathExists — the guarantee above holds only because no
// code in this package can build a non-GET request in the first place. A
// method parameter added to the REST client would make checkRoute the only
// thing standing between a caller and a write; this asserts it stays absent
// (CLAUDE.md rule 1: read-only by what is linked in).
func TestNoNonGetRequestPathExists(t *testing.T) {
	for _, method := range []string{"MethodPost", "MethodPut", "MethodPatch", "MethodDelete", "MethodHead", "MethodOptions"} {
		if strings.Contains(restSource(t), "http."+method) {
			t.Fatalf("rest.go references http.%s: the REST client must be able to issue GET only", method)
		}
	}
}

// TestUnlistedRouteDenied — a route that is not in the table is refused, and
// nothing reaches the server.
func TestUnlistedRouteDenied(t *testing.T) {
	srv, requests := countingServer(t, nil)
	c := testClient(t, srv)

	_, err := c.get(testCtx(t), "/api/services", "/api/services", nil)
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("get(/api/services): got %v, want ErrPolicyDenied", err)
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("denied route still reached the server: %d requests", n)
	}
}

// TestState_TraversalEntityID_RejectedAtValidation — a path-traversal-shaped
// entity id is refused at parameter validation, not escaped and sent.
func TestState_TraversalEntityID_RejectedAtValidation(t *testing.T) {
	bad := []string{
		"../../config",
		"light.kitchen/../../config",
		"light.kitchen%2f..%2fconfig",
		"light kitchen",
		"LIGHT.kitchen",
		"light",
		"",
	}
	srv, requests := countingServer(t, nil)
	c := testClient(t, srv)

	for _, entityID := range bad {
		t.Run(entityID, func(t *testing.T) {
			_, err := c.State(testCtx(t), entityID)
			if !errors.Is(err, ErrPolicyDenied) {
				t.Fatalf("State(%q): got %v, want ErrPolicyDenied", entityID, err)
			}
		})
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("rejected entity id still produced %d request(s)", n)
	}
}

func TestState_ValidEntityID_RequestsExactPath(t *testing.T) {
	var path atomic.Value
	path.Store("")
	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		path.Store(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"entity_id":"light.kitchen","state":"on"}`))
	})
	c := testClient(t, srv)

	body, err := c.State(testCtx(t), "light.kitchen")
	if err != nil {
		t.Fatalf("State: unexpected error: %v", err)
	}
	if got := path.Load().(string); got != "/api/states/light.kitchen" {
		t.Fatalf("State requested %q, want /api/states/light.kitchen", got)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("State: body is not JSON: %v", err)
	}
	if decoded["state"] != "on" {
		t.Fatalf("State: got %v, want state on", decoded)
	}
}

// TestOversizedResponse_TruncatedWithExplicitError — the body offered here is
// unbounded: a client that buffered the whole response would never return.
func TestOversizedResponse_TruncatedWithExplicitError(t *testing.T) {
	prev := maxRESTResponseBytes
	maxRESTResponseBytes = 4096
	t.Cleanup(func() { maxRESTResponseBytes = prev })

	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		chunk := strings.Repeat("a", 1024)
		for {
			select {
			case <-r.Context().Done():
				return
			default:
			}
			if _, err := w.Write([]byte(chunk)); err != nil {
				return
			}
		}
	})
	c := testClient(t, srv)

	body, err := c.Config(testCtx(t))
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("Config: got %v, want ErrResponseTooLarge", err)
	}
	if body != nil {
		t.Fatalf("Config: returned %d bytes alongside a size error, want nil", len(body))
	}
}

func TestConfig_ValidToken_ReturnsBody(t *testing.T) {
	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/config" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet {
			t.Errorf("server saw method %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"2026.8.0","location_name":"Home"}`))
	})

	body, err := testClient(t, srv).Config(testCtx(t))
	if err != nil {
		t.Fatalf("Config: unexpected error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("Config: body is not JSON: %v", err)
	}
	if decoded["version"] != "2026.8.0" {
		t.Fatalf("Config: got %v, want version 2026.8.0", decoded)
	}
}

func TestConfig_WrongToken_ReturnsTypedAuthError(t *testing.T) {
	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := NewRESTClient(srv.URL, "wrong-token", srv.Client(), nil).Config(testCtx(t))
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("Config: got %v, want ErrAuthFailed", err)
	}
}

func TestState_MissingEntity_ReturnsNotFound(t *testing.T) {
	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := testClient(t, srv).State(testCtx(t), "light.gone")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("State: got %v, want ErrNotFound", err)
	}
}

func TestConfig_ServerUnreachable_ReturnsUpstreamUnavailable(t *testing.T) {
	c := NewRESTClient("http://127.0.0.1:1", testToken, http.DefaultClient, nil)

	_, err := c.Config(testCtx(t))
	if !errors.Is(err, ErrUpstreamUnavailable) {
		t.Fatalf("Config: got %v, want ErrUpstreamUnavailable", err)
	}
}

// No error the REST client returns may carry the token (CLAUDE.md rule 4).
func TestRESTClient_Errors_NeverCarryTheToken(t *testing.T) {
	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	c := testClient(t, srv)
	ctx := testCtx(t)

	_, statusErr := c.Config(ctx)
	_, denyErr := c.State(ctx, "../../config")
	_, unreachErr := NewRESTClient("http://127.0.0.1:1", testToken, http.DefaultClient, nil).Config(ctx)

	deadlineSrv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	deadlineCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, deadlineErr := testClient(t, deadlineSrv).Config(deadlineCtx)

	for _, err := range []error{statusErr, denyErr, unreachErr, deadlineErr} {
		if err == nil {
			t.Fatal("expected an error to inspect")
		}
		if strings.Contains(err.Error(), testToken) {
			t.Fatalf("error string carries the token: %q", err)
		}
	}
}

func TestHistoryPeriod_Options_BuildTypedQueryOnly(t *testing.T) {
	var seen atomic.Value
	seen.Store(url.Values{})
	var path atomic.Value
	path.Store("")
	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		path.Store(r.URL.Path)
		seen.Store(r.URL.Query())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})
	c := testClient(t, srv)

	start := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	_, err := c.HistoryPeriod(testCtx(t), start, HistoryOptions{
		End:                    end,
		EntityIDs:              []string{"light.kitchen", "sensor.temperature"},
		MinimalResponse:        true,
		NoAttributes:           true,
		SignificantChangesOnly: true,
	})
	if err != nil {
		t.Fatalf("HistoryPeriod: unexpected error: %v", err)
	}

	if got := path.Load().(string); got != "/api/history/period/2026-08-25T10:00:00Z" {
		t.Fatalf("HistoryPeriod requested %q", got)
	}
	q := seen.Load().(url.Values)
	if got := q.Get("filter_entity_id"); got != "light.kitchen,sensor.temperature" {
		t.Fatalf("filter_entity_id = %q", got)
	}
	if got := q.Get("end_time"); got != "2026-08-25T11:00:00Z" && got != end.UTC().Format(time.RFC3339) {
		t.Fatalf("end_time = %q, want %q", got, end.UTC().Format(time.RFC3339))
	}
	for _, flag := range []string{"minimal_response", "no_attributes", "significant_changes_only"} {
		if _, ok := q[flag]; !ok {
			t.Fatalf("%s missing from query %v", flag, q)
		}
	}
}

func TestHistoryPeriod_InvalidEntityID_Denied(t *testing.T) {
	srv, requests := countingServer(t, nil)
	c := testClient(t, srv)

	_, err := c.HistoryPeriod(testCtx(t), time.Now(), HistoryOptions{EntityIDs: []string{"light.kitchen", "../../config"}})
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("HistoryPeriod: got %v, want ErrPolicyDenied", err)
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("denied history request still reached the server: %d requests", n)
	}
}

func TestLogbookPeriod_RequestsAllowListedRoute(t *testing.T) {
	var path atomic.Value
	path.Store("")
	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		path.Store(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})
	c := testClient(t, srv)

	start := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	if _, err := c.LogbookPeriod(testCtx(t), start, LogbookOptions{EntityID: "light.kitchen"}); err != nil {
		t.Fatalf("LogbookPeriod: unexpected error: %v", err)
	}
	if got := path.Load().(string); got != "/api/logbook/2026-08-25T10:00:00Z" {
		t.Fatalf("LogbookPeriod requested %q", got)
	}
}

// A caller that supplies no deadline still gets one — no unbounded upstream
// wait exists (CLAUDE.md, Error Handling).
func TestRESTClient_NoCallerDeadline_AppliesBackstop(t *testing.T) {
	prev := defaultRESTTimeout
	defaultRESTTimeout = 50 * time.Millisecond
	t.Cleanup(func() { defaultRESTTimeout = prev })

	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	c := testClient(t, srv)

	done := make(chan error, 1)
	go func() {
		_, err := c.Config(context.Background())
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrDeadline) {
			t.Fatalf("Config: got %v, want ErrDeadline", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Config: no deadline applied — call did not return")
	}
}

// A caller's own deadline, not just the backstop, must surface as
// ErrDeadline — distinguishable from ErrUpstreamUnavailable, which means the
// connection itself failed rather than the caller giving up first.
func TestConfig_CallerDeadlineExceeded_ReturnsErrDeadline(t *testing.T) {
	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	c := testClient(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.Config(ctx)
	if !errors.Is(err, ErrDeadline) {
		t.Fatalf("Config: got %v, want ErrDeadline", err)
	}
	if errors.Is(err, ErrUpstreamUnavailable) {
		t.Fatalf("Config: %v also matches ErrUpstreamUnavailable, want it distinguishable from ErrDeadline", err)
	}
}

// The route table itself may not name a route outside Core's read surface.
func TestAllowedRoutes_AreGetShapedReads(t *testing.T) {
	for route := range allowedRoutes {
		if !strings.HasPrefix(route, "/api/") {
			t.Fatalf("route %q is not under /api/", route)
		}
		for _, verb := range []string{"service", "event", "config/", "template", "checkconfig"} {
			if strings.Contains(route, verb) {
				t.Fatalf("route %q looks like a write or execution surface (%q)", route, verb)
			}
		}
	}
}

// restSource returns the REST client's source, so a structural guarantee can
// be asserted instead of reviewed.
func restSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("rest.go")
	if err != nil {
		t.Fatalf("reading rest.go: %v", err)
	}
	return string(b)
}
