package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/audit"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
	"github.com/freemanjava/ha-explorer-mcp/internal/redact"
)

const (
	testHTTPSecret = "0123456789abcdef0123456789abcdef-http-secret"
	toolsCallBody  = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_areas","arguments":{}}}`
	toolsListBody  = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
)

// httpFixture serves a server over the real handler stack on httptest.
func httpFixture(t *testing.T, opts Options, tools []Tool, logger *slog.Logger) *httptest.Server {
	t.Helper()
	srv := newServer(opts, tools)
	h, err := newHTTPHandler(srv, testHTTPSecret, logger)
	if err != nil {
		t.Fatalf("newHTTPHandler: %v", err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}

func post(t *testing.T, url, body string, header map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func bearer(secret string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + secret}
}

// TestHTTP_Authentication_RefusedBeforeTheServerSeesIt asserts every way a
// credential can be wrong is a 401 and that the MCP server never runs a tool
// for it (D-08-4).
func TestHTTP_Authentication_RefusedBeforeTheServerSeesIt(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	tools := probeTable(func(context.Context, string) error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		return nil
	})
	ts := httpFixture(t, testOptions(), tools, slog.New(slog.DiscardHandler))

	cases := map[string]map[string]string{
		"missing":      nil,
		"wrong secret": bearer("wrong-" + testHTTPSecret),
		"prefix":       bearer(testHTTPSecret[:len(testHTTPSecret)-1]),
		"longer":       bearer(testHTTPSecret + "x"),
		"basic scheme": {"Authorization": "Basic " + testHTTPSecret},
		"no scheme":    {"Authorization": testHTTPSecret},
		"empty token":  {"Authorization": "Bearer "},
	}
	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			res := post(t, ts.URL+httpPath, toolsCallBody, header)
			if res.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", res.StatusCode)
			}
			if got := res.Header.Get("WWW-Authenticate"); got != "Bearer" {
				t.Errorf("WWW-Authenticate = %q, want Bearer", got)
			}
		})
	}

	t.Run("duplicated header", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL+httpPath, strings.NewReader(toolsCallBody))
		req.Header.Add("Authorization", "Bearer "+testHTTPSecret)
		req.Header.Add("Authorization", "Bearer "+testHTTPSecret)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", res.StatusCode)
		}
	})

	t.Run("scheme is case-insensitive", func(t *testing.T) {
		res := post(t, ts.URL+httpPath, toolsListBody, map[string]string{"Authorization": "bEaReR " + testHTTPSecret})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.StatusCode)
		}
	})

	mu.Lock()
	defer mu.Unlock()
	if calls != 0 {
		t.Errorf("a tool ran %d times for an unauthenticated request", calls)
	}
}

// TestHTTP_AnyOrigin_Forbidden asserts D-08-5: an Origin header is refused
// even when it equals Host and even with the right secret.
func TestHTTP_AnyOrigin_Forbidden(t *testing.T) {
	ts := httpFixture(t, testOptions(), Catalog(), slog.New(slog.DiscardHandler))
	host := strings.TrimPrefix(ts.URL, "http://")

	for _, origin := range []string{"http://" + host, "http://evil.example", "null", ""} {
		header := bearer(testHTTPSecret)
		header["Origin"] = origin
		res := post(t, ts.URL+httpPath, toolsListBody, header)
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("Origin %q: status = %d, want 403", origin, res.StatusCode)
		}
	}
}

func TestHTTP_RoutesAndMethods(t *testing.T) {
	ts := httpFixture(t, testOptions(), Catalog(), slog.New(slog.DiscardHandler))

	res := post(t, ts.URL+"/other", toolsListBody, bearer(testHTTPSecret))
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("other path: status = %d, want 404", res.StatusCode)
	}

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+httpPath, nil)
	req.Header.Set("Authorization", "Bearer "+testHTTPSecret)
	got, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = got.Body.Close() }()
	if got.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET: status = %d, want 405", got.StatusCode)
	}
}

// httpClient connects an SDK client over HTTP with the right secret.
func httpClient(t *testing.T, ts *httptest.Server) *sdkmcp.ClientSession {
	t.Helper()
	transport := &sdkmcp.StreamableClientTransport{
		Endpoint:   ts.URL + httpPath,
		HTTPClient: &http.Client{Transport: bearerTransport{secret: testHTTPSecret}},
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	cs, err := client.Connect(t.Context(), transport, nil)
	if err != nil {
		t.Fatalf("client.Connect over HTTP: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

type bearerTransport struct{ secret string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.secret)
	return http.DefaultTransport.RoundTrip(r)
}

func TestHTTP_ToolsList_MatchesStdio(t *testing.T) {
	ts := httpFixture(t, testOptions(), Catalog(), slog.New(slog.DiscardHandler))
	res, err := httpClient(t, ts).ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools over HTTP: %v", err)
	}
	if len(res.Tools) != len(expectedTools) {
		t.Fatalf("HTTP tools/list returned %d tools, want %d", len(res.Tools), len(expectedTools))
	}
	seen := map[string]bool{}
	for _, tool := range res.Tools {
		seen[tool.Name] = true
	}
	for _, want := range expectedTools {
		if !seen[want] {
			t.Errorf("HTTP tools/list is missing %s", want)
		}
	}
}

// TestHTTP_ToolHandler_SeesNoAuthorizationHeader asserts the secret never
// enters internal/mcp: the SDK would hand every header to server code, so the
// gate must delete it (D-08-4).
func TestHTTP_ToolHandler_SeesNoAuthorizationHeader(t *testing.T) {
	var mu sync.Mutex
	var seen http.Header
	tools := probeTable(func(context.Context, string) error { return nil })
	for i := range tools {
		if tools[i].Name != "list_areas" {
			continue
		}
		tools[i].bind = func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
			def.InputSchema = emptyObjectSchema
			srv.AddTool(def, func(_ context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
				mu.Lock()
				defer mu.Unlock()
				if req.Extra != nil {
					seen = req.Extra.Header.Clone()
				}
				return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "ok"}}}, nil
			})
		}
	}
	ts := httpFixture(t, testOptions(), tools, slog.New(slog.DiscardHandler))

	res := post(t, ts.URL+httpPath, toolsCallBody, bearer(testHTTPSecret))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	mu.Lock()
	defer mu.Unlock()
	if seen == nil {
		t.Fatal("the handler saw no request headers; the assertion would be vacuous")
	}
	if v := seen.Values("Authorization"); len(v) != 0 {
		t.Errorf("tool handler saw an Authorization header: %v", v)
	}
}

// TestHTTP_BodyLimit asserts the 128 KiB cap, and that the largest legal tool
// input — 200 ids at HA's 255-character ceiling — fits inside it (D-08-7).
func TestHTTP_BodyLimit(t *testing.T) {
	ts := httpFixture(t, testOptions(), probeTable(func(context.Context, string) error { return nil }), slog.New(slog.DiscardHandler))

	ids := make([]string, 200)
	for i := range ids {
		ids[i] = strings.Repeat("a", 255)
	}
	args, _ := json.Marshal(map[string]any{"entity_ids": ids})
	legal := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_areas","arguments":` + string(args) + `}}`
	if len(legal) >= maxRequestBodyBytes {
		t.Fatalf("largest legal input is %d bytes, over the %d cap", len(legal), maxRequestBodyBytes)
	}
	res := post(t, ts.URL+httpPath, legal, bearer(testHTTPSecret))
	if res.StatusCode != http.StatusOK {
		t.Errorf("largest legal input: status = %d, want 200", res.StatusCode)
	}

	oversized := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_areas","arguments":{"x":"` +
		strings.Repeat("a", maxRequestBodyBytes) + `"}}}`
	res = post(t, ts.URL+httpPath, oversized, bearer(testHTTPSecret))
	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: status = %d, want 413", res.StatusCode)
	}
}

// TestHTTP_FifthConcurrentRequest_Unavailable asserts the in-flight cap, and
// that unauthenticated traffic cannot occupy the slots (D-08-7).
func TestHTTP_FifthConcurrentRequest_Unavailable(t *testing.T) {
	entered := make(chan struct{}, maxInFlight)
	release := make(chan struct{})
	tools := probeTable(func(ctx context.Context, _ string) error {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	})
	ts := httpFixture(t, testOptions(), tools, slog.New(slog.DiscardHandler))

	var wg sync.WaitGroup
	for range maxInFlight {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := post(t, ts.URL+httpPath, toolsCallBody, bearer(testHTTPSecret))
			_, _ = io.Copy(io.Discard, res.Body)
		}()
	}
	for range maxInFlight {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("the first four requests did not all reach the handler")
		}
	}

	if res := post(t, ts.URL+httpPath, toolsCallBody, nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated while full: status = %d, want 401 (it must not reach the slot check)", res.StatusCode)
	}
	res := post(t, ts.URL+httpPath, toolsCallBody, bearer(testHTTPSecret))
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("fifth request: status = %d, want 503", res.StatusCode)
	}
	if got := res.Header.Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After = %q, want 1", got)
	}

	close(release)
	wg.Wait()
}

// TestHTTP_Requests_ShareOneInvocationLimiter asserts every POST draws on the
// one process-wide limiter, so a request storm cannot get a fresh allowance
// per connection (D-08-7).
func TestHTTP_Requests_ShareOneInvocationLimiter(t *testing.T) {
	opts := testOptions()
	opts.Limiter = policy.NewRateLimiter(1, time.Hour)
	ts := httpFixture(t, opts, probeTable(func(context.Context, string) error { return nil }), slog.New(slog.DiscardHandler))

	first := post(t, ts.URL+httpPath, toolsCallBody, bearer(testHTTPSecret))
	firstBody, _ := io.ReadAll(first.Body)
	second := post(t, ts.URL+httpPath, toolsCallBody, bearer(testHTTPSecret))
	secondBody, _ := io.ReadAll(second.Body)

	if bytes.Contains(firstBody, []byte(`"error"`)) {
		t.Fatalf("first call was refused: %s", firstBody)
	}
	if !bytes.Contains(secondBody, []byte(`"error"`)) {
		t.Errorf("second call was served despite an exhausted limiter: %s", secondBody)
	}
}

func TestHTTPServer_WriteTimeout_DerivedFromCompositeDeadline(t *testing.T) {
	srv := newHTTPServer(http.NotFoundHandler())
	if srv.WriteTimeout != policy.CompositeDeadline()+writeTimeoutMargin {
		t.Errorf("WriteTimeout = %v, want composite deadline + %v", srv.WriteTimeout, writeTimeoutMargin)
	}
	if srv.WriteTimeout <= policy.CompositeDeadline() {
		t.Errorf("WriteTimeout %v does not outlast the composite deadline %v", srv.WriteTimeout, policy.CompositeDeadline())
	}
	if srv.ReadHeaderTimeout != readHeaderTimeout || srv.ReadTimeout != readTimeout ||
		srv.IdleTimeout != idleTimeout || srv.MaxHeaderBytes != maxHeaderBytes {
		t.Errorf("server limits do not match D-08-7: %+v", srv)
	}
}

func TestHTTP_AuditRecords_CarryTransport(t *testing.T) {
	sink := &recordSink{}
	opts := testOptions()
	opts.Transport = TransportHTTP
	opts.Audit = nil
	opts.Logger = slog.New(sink)
	ts := httpFixture(t, opts, probeTable(func(context.Context, string) error { return nil }), opts.Logger)

	res := post(t, ts.URL+httpPath, toolsCallBody, bearer(testHTTPSecret))
	_, _ = io.Copy(io.Discard, res.Body)

	rec, ok := sink.lastWithMessage("audit")
	if !ok {
		t.Fatal("no audit record was emitted")
	}
	if rec.attrs["transport"] != TransportHTTP {
		t.Errorf("audit transport = %v, want %q", rec.attrs["transport"], TransportHTTP)
	}
	if rec.attrs["status"] != string(audit.StatusSuccess) {
		t.Errorf("audit status = %v", rec.attrs["status"])
	}
}

// TestHTTP_Secret_NeverLoggedOrReturned is the token-never-returned assertion
// for the client secret: rejected and accepted traffic alike, at DEBUG, leaves
// neither the secret nor a Bearer header in any log line or response (rule 4).
func TestHTTP_Secret_NeverLoggedOrReturned(t *testing.T) {
	var buf lockedBuffer
	logger := slog.New(redact.NewLogHandler(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}), testHTTPSecret))
	opts := testOptions()
	opts.Logger = logger
	ts := httpFixture(t, opts, probeTable(func(context.Context, string) error { return nil }), logger)

	var responses bytes.Buffer
	for _, h := range []map[string]string{
		nil,
		bearer("wrong-" + testHTTPSecret),
		{"Origin": "http://evil.example", "Authorization": "Bearer " + testHTTPSecret},
		bearer(testHTTPSecret),
	} {
		res := post(t, ts.URL+httpPath, toolsCallBody, h)
		body, _ := io.ReadAll(res.Body)
		responses.Write(body)
		for k, v := range res.Header {
			responses.WriteString(k + ": " + strings.Join(v, ",") + "\n")
		}
	}

	for name, out := range map[string]string{"logs": buf.String(), "responses": responses.String()} {
		if strings.Contains(out, testHTTPSecret) {
			t.Errorf("the secret appears in the %s", name)
		}
	}
	if strings.Contains(buf.String(), "Bearer ") {
		t.Error(`a log line contains "Bearer "`)
	}
	if !strings.Contains(buf.String(), "rejected") {
		t.Error("rejections were not logged at all; the assertion would be vacuous")
	}
}

func TestHTTPRejections_WarnAtMostOncePerMinute(t *testing.T) {
	sink := &recordSink{}
	now := time.Unix(1_000_000, 0)
	rej := &rejections{log: slog.New(sink), now: func() time.Time { return now }}

	for range 5 {
		rej.note(context.Background(), "192.0.2.1", reasonMismatch)
	}
	if got := countLevel(sink, slog.LevelWarn); got != 1 {
		t.Fatalf("warnings within one minute = %d, want 1", got)
	}

	now = now.Add(2 * time.Minute)
	rej.note(context.Background(), "192.0.2.2", reasonMissing)
	if got := countLevel(sink, slog.LevelWarn); got != 2 {
		t.Errorf("warnings after the interval = %d, want 2", got)
	}
}

func TestNewHTTPHandler_EmptySecret_Refused(t *testing.T) {
	if _, err := newHTTPHandler(NewServer(testOptions()), "", slog.New(slog.DiscardHandler)); err == nil {
		t.Fatal("a handler was built without a secret; the transport would be open")
	}
}

func countLevel(s *recordSink, level slog.Level) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, l := range s.logs {
		if l.level == level {
			n++
		}
	}
	return n
}

// lockedBuffer is a bytes.Buffer safe for the concurrent log writes a server
// under test produces.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
