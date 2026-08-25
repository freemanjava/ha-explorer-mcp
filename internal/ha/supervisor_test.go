package ha

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
)

// TestSupervisorRoutes_AreGetShapedExactMatch guards the allow-list itself:
// every entry is a concrete path (no template placeholder), so there is no
// parameter that could widen it at request time.
func TestSupervisorRoutes_AreGetShapedExactMatch(t *testing.T) {
	for route := range allowedSupervisorRoutes {
		if !strings.HasPrefix(route, "/") {
			t.Errorf("route %q must be an absolute path", route)
		}
		if strings.Contains(route, "{") {
			t.Errorf("route %q carries a template placeholder; Supervisor routes must be exact-match concrete paths", route)
		}
	}
}

// TestSupervisorRoute_OutsideAllowList_Denied covers exactly the DoD's named
// examples: /core/stats, /addons and anything under /host/ beyond /host/info.
func TestSupervisorRoute_OutsideAllowList_Denied(t *testing.T) {
	outside := []string{
		"/core/stats",
		"/core/info",
		"/addons",
		"/addons/self/stats/extra",
		"/host/logs",
		"/host/logs/follow",
		"/supervisor/stats",
		"/supervisor/restart",
		"/store",
		"/available_updates",
	}
	for _, route := range outside {
		t.Run(route, func(t *testing.T) {
			if err := checkSupervisorRoute(http.MethodGet, route); !errors.Is(err, ErrPolicyDenied) {
				t.Fatalf("checkSupervisorRoute(GET, %q): got %v, want ErrPolicyDenied", route, err)
			}
		})
	}
}

// TestSupervisorRoute_OutsideAllowList_NoBytesReachServer proves the denial
// happens before any request is issued, not merely that checkSupervisorRoute
// alone returns a denial.
func TestSupervisorRoute_OutsideAllowList_NoBytesReachServer(t *testing.T) {
	srv, requests := countingServer(t, nil)
	c := NewSupervisorClient(srv.URL, testToken, srv.Client(), nil)

	_, err := c.get(testCtx(t), "/addons")
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("get(/addons): got %v, want ErrPolicyDenied", err)
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("denied Supervisor route still reached the server: %d requests", n)
	}
}

// TestSupervisorRoute_NonGetMethod_Denied — the method check happens before
// the table lookup, exactly as Core's checkRoute does.
func TestSupervisorRoute_NonGetMethod_Denied(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			err := checkSupervisorRoute(method, SupervisorRouteInfo)
			if !errors.Is(err, ErrPolicyDenied) {
				t.Fatalf("checkSupervisorRoute(%s, %s): got %v, want ErrPolicyDenied", method, SupervisorRouteInfo, err)
			}
			if !strings.Contains(err.Error(), "method") {
				t.Fatalf("denial reason %q does not name the method check", err)
			}
		})
	}
}

// TestNoNonGetSupervisorRequestPathExists mirrors TestNoNonGetRequestPathExists
// for the Supervisor client file: read-only-ness must hold because no code
// path here can build a mutating request, not only because the gateway check
// catches it (CLAUDE.md rule 1).
func TestNoNonGetSupervisorRequestPathExists(t *testing.T) {
	b, err := os.ReadFile("supervisor.go")
	if err != nil {
		t.Fatalf("reading supervisor.go: %v", err)
	}
	src := string(b)
	for _, method := range []string{"MethodPost", "MethodPut", "MethodPatch", "MethodDelete", "MethodHead", "MethodOptions"} {
		if strings.Contains(src, "http."+method) {
			t.Fatalf("supervisor.go references http.%s: the Supervisor client must be able to issue GET only", method)
		}
	}
}

// TestSupervisorClient_Unreachable_ReturnsUnsupported — Supervisor being
// absent while Core is up must degrade, not fail the same way as a Core
// outage (CLAUDE.md, Reliability; phase 01 DoD).
func TestSupervisorClient_Unreachable_ReturnsUnsupported(t *testing.T) {
	c := NewSupervisorClient("http://127.0.0.1:1", testToken, http.DefaultClient, nil)

	_, err := c.Info(testCtx(t))
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Info: got %v, want ErrUnsupported", err)
	}
}

func TestSupervisorClient_Info_ValidToken_ReturnsBody(t *testing.T) {
	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != SupervisorRouteInfo {
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
		_, _ = w.Write([]byte(`{"supervisor":"2026.08.0","hostname":"homeassistant"}`))
	})

	c := NewSupervisorClient(srv.URL, testToken, srv.Client(), nil)
	body, err := c.Info(testCtx(t))
	if err != nil {
		t.Fatalf("Info: unexpected error: %v", err)
	}
	if !strings.Contains(string(body), "2026.08.0") {
		t.Fatalf("Info body = %s, want it to carry the supervisor version", body)
	}
}

// TestSupervisorClient_TokenNeverReturned is the phase 01 DoD line verbatim
// (CLAUDE.md rule 4): SUPERVISOR_TOKEN appears in no response, no error
// string and no log line, across every failure mode this client produces.
func TestSupervisorClient_TokenNeverReturned(t *testing.T) {
	var logBuf strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	c := NewSupervisorClient(srv.URL, testToken, srv.Client(), logger)
	ctx := testCtx(t)

	_, statusErr := c.Info(ctx)
	_, denyErr := c.get(ctx, "/addons")
	_, unreachErr := NewSupervisorClient("http://127.0.0.1:1", testToken, http.DefaultClient, logger).Info(ctx)
	_, mutatedErr := NewSupervisorClient(srv.URL, testToken, srv.Client(), logger).SupervisorInfo(ctx)
	body, respErr := c.Info(ctx)

	for name, err := range map[string]error{
		"status":  statusErr,
		"deny":    denyErr,
		"unreach": unreachErr,
		"mutated": mutatedErr,
		"resp":    respErr,
	} {
		if err == nil {
			t.Fatalf("%s: expected an error to inspect", name)
		}
		if strings.Contains(err.Error(), testToken) {
			t.Fatalf("%s: error string carries the token: %q", name, err)
		}
	}
	if strings.Contains(string(body), testToken) {
		t.Fatalf("response body carries the token: %q", body)
	}
	if strings.Contains(logBuf.String(), testToken) {
		t.Fatalf("log output carries the token: %q", logBuf.String())
	}
}

func TestSupervisorInfo_ValidShape_Maps(t *testing.T) {
	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"version": "2026.08.0",
			"version_latest": "2026.09.0",
			"channel": "stable",
			"supported": true,
			"healthy": true,
			"addons": [
				{"slug": "core_ssh", "name": "Terminal & SSH", "version": "9.14.0", "version_latest": "9.14.0", "update_available": false, "state": "started", "repository": "core"}
			]
		}`))
	})
	c := NewSupervisorClient(srv.URL, testToken, srv.Client(), nil)

	info, err := c.SupervisorInfo(testCtx(t))
	if err != nil {
		t.Fatalf("SupervisorInfo: unexpected error: %v", err)
	}
	if info.Version != "2026.08.0" || info.Channel != "stable" || !info.Supported || !info.Healthy {
		t.Fatalf("SupervisorInfo mapped %+v unexpectedly", info)
	}
	if len(info.Apps) != 1 || info.Apps[0].Slug != "core_ssh" || info.Apps[0].State != "started" {
		t.Fatalf("SupervisorInfo.Apps mapped %+v unexpectedly", info.Apps)
	}
}

// TestSupervisorInfo_MutatedShape_FailsLoudly is the phase 01 DoD line
// verbatim: a mutated /supervisor/info shape must fail with a reported error,
// never map silently into a zeroed or garbage value.
func TestSupervisorInfo_MutatedShape_FailsLoudly(t *testing.T) {
	cases := map[string]string{
		"supported is a string, not a bool":    `{"version":"2026.08.0","supported":"yes"}`,
		"addons element version is a number":   `{"version":"2026.08.0","addons":[{"slug":"core_ssh","version":9}]}`,
		"top-level is an array, not an object": `[1,2,3]`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(payload))
			})
			c := NewSupervisorClient(srv.URL, testToken, srv.Client(), nil)

			info, err := c.SupervisorInfo(testCtx(t))
			if err == nil {
				t.Fatalf("SupervisorInfo: got nil error and %+v, want a mapping failure", info)
			}
			if !errors.Is(err, ErrUnexpectedMessage) {
				t.Fatalf("SupervisorInfo: got %v, want ErrUnexpectedMessage", err)
			}
		})
	}
}
