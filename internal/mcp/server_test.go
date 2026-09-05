package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/audit"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// TestServer_Initialize_ListsTheReadOnlyCatalog is the P3-01 DoD's client
// round trip: a client completes initialize and tools/list over the SDK
// transport and sees exactly doc §9's twenty tools, every one annotated
// read-only (doc §11's first enforcement point).
func TestServer_Initialize_ListsTheReadOnlyCatalog(t *testing.T) {
	client := connect(t, NewServer(testOptions()))

	res, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	seen := map[string]bool{}
	for _, tool := range res.Tools {
		seen[tool.Name] = true
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s is not annotated read-only", tool.Name)
		}
	}
	if len(seen) != len(expectedTools) {
		t.Fatalf("tools/list returned %d tools, want %d", len(seen), len(expectedTools))
	}
	for _, want := range expectedTools {
		if !seen[want] {
			t.Errorf("tools/list is missing %s", want)
		}
	}
}

// TestServer_ToolSchemas_AcceptNoFreeFormParameter is the parity rule's fourth
// clause (CLAUDE.md) and phase 03's DoD, asserted over the registered schemas
// rather than by review — so a tool added later cannot introduce an escape
// hatch without failing this test.
func TestServer_ToolSchemas_AcceptNoFreeFormParameter(t *testing.T) {
	forbidden := []string{"route", "url", "endpoint", "command", "cmd", "path", "file", "query", "sql", "shell", "script", "code", "eval", "exec", "raw"}

	client := connect(t, NewServer(testOptions()))
	res, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	for _, tool := range res.Tools {
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("%s: marshal input schema: %v", tool.Name, err)
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("%s: unmarshal input schema: %v", tool.Name, err)
		}
		for prop := range schema.Properties {
			for _, bad := range forbidden {
				if strings.EqualFold(prop, bad) {
					t.Errorf("%s accepts a free-form %q parameter (CLAUDE.md rule 2)", tool.Name, prop)
				}
			}
		}
	}
}

// TestServer_EveryTool_InvokedWithItsBudget asserts the DoD's "no tool can be
// registered without a budget": every catalog row, invoked, finds a query
// budget in its context carrying that row's class limits and a deadline.
func TestServer_EveryTool_InvokedWithItsBudget(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]policy.Limits{}
	deadlines := map[string]bool{}

	tools := probeTable(func(ctx context.Context, name string) error {
		budget, ok := policy.BudgetFrom(ctx)
		if !ok {
			return errors.New("no budget in context")
		}
		_, hasDeadline := ctx.Deadline()
		mu.Lock()
		defer mu.Unlock()
		seen[name] = budget.Limits()
		deadlines[name] = hasDeadline
		return nil
	})

	client := connect(t, newServer(testOptions(), tools))
	for _, tool := range tools {
		if _, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: tool.Name}); err != nil {
			t.Fatalf("CallTool(%s): %v", tool.Name, err)
		}
		want := policy.LimitsFor(tool.Class)
		if got := seen[tool.Name]; got != want {
			t.Errorf("%s ran with limits %+v, want its class's %+v", tool.Name, got, want)
		}
		if !deadlines[tool.Name] {
			t.Errorf("%s ran without a deadline — every upstream call must carry one", tool.Name)
		}
	}
}

// TestServer_ToolPanic_ReturnsErrorAuditsAndSurvives covers the DoD's panic
// clause: the client gets an error, the audit trail records it, and the server
// keeps serving the next call.
func TestServer_ToolPanic_ReturnsErrorAuditsAndSurvives(t *testing.T) {
	sink := &recordSink{}
	tools := probeTable(func(_ context.Context, name string) error {
		if name == "get_entity" {
			panic("boom: " + name)
		}
		return nil
	})

	client := connect(t, newServer(auditOptions(sink), tools))

	if _, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "get_entity"}); err == nil {
		t.Fatal("panicking tool returned no error to the client")
	}
	rec, ok := sink.last()
	if !ok {
		t.Fatal("panicking tool produced no audit record")
	}
	if rec["status"] != string(audit.StatusError) {
		t.Errorf("audited status = %v, want %v", rec["status"], audit.StatusError)
	}
	if reason, _ := rec["reason"].(string); !strings.Contains(reason, ErrToolPanicked.Error()) {
		t.Errorf("audited reason = %q, want it to name the panic", reason)
	}

	// The process is still serving: the next invocation succeeds.
	if _, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "list_areas"}); err != nil {
		t.Fatalf("server did not survive the panic: %v", err)
	}
}

// TestServer_UnknownTool_DeniedBeforeDispatch asserts rule 3: a name outside
// the static catalog is refused, and refused as a denial rather than an
// internal error, before any handler runs.
func TestServer_UnknownTool_DeniedBeforeDispatch(t *testing.T) {
	sink := &recordSink{}
	client := connect(t, NewServer(auditOptions(sink)))

	_, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "call_service"})
	if err == nil {
		t.Fatal("an uncatalogued tool name was accepted")
	}
	rec, ok := sink.last()
	if !ok {
		t.Fatal("the refusal was not audited")
	}
	if rec["status"] != string(audit.StatusDenied) {
		t.Errorf("audited status = %v, want %v", rec["status"], audit.StatusDenied)
	}
}

// TestServer_RateLimited_DeniedAndAudited asserts the arrival limiter runs in
// front of every invocation, and that its refusal is audited as a denial —
// distinguishable from a budget cutoff and from a failure.
func TestServer_RateLimited_DeniedAndAudited(t *testing.T) {
	sink := &recordSink{}
	opts := auditOptions(sink)
	// No tokens: the next arrival is refused outright, not queued.
	opts.Limiter = policy.NewRateLimiter(0, time.Hour)
	client := connect(t, NewServer(opts))

	_, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "list_areas"})
	if err == nil {
		t.Fatal("a rate-limited invocation was served")
	}
	rec, ok := sink.last()
	if !ok {
		t.Fatal("the refusal was not audited")
	}
	if rec["status"] != string(audit.StatusDenied) {
		t.Errorf("audited status = %v, want %v", rec["status"], audit.StatusDenied)
	}
}

// TestServer_UnimplementedTool_ErrorsWithoutClaimingUnsupported asserts a
// catalog row whose task has not landed answers with "not implemented in this
// build" — never with an empty result, and never borrowing the word
// "unsupported", which is a claim about the installation (rule 7).
func TestServer_UnimplementedTool_ErrorsWithoutClaimingUnsupported(t *testing.T) {
	client := connect(t, NewServer(testOptions()))

	_, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "get_system_overview"})
	if err == nil {
		t.Fatal("an unimplemented tool returned a result")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("error = %q, want it to say the tool is not implemented in this build", err)
	}
	if strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error = %q, must not claim the installation is unsupported", err)
	}
}

// testOptions are the server options every test starts from: SDK chatter
// discarded, and an arrival limiter wide enough that a test walking the whole
// catalog is not refused by the storm guard it is not testing.
func testOptions() Options {
	return Options{
		Version: "test",
		Logger:  slog.New(slog.DiscardHandler),
		Limiter: policy.NewRateLimiter(len(catalog)+10, time.Millisecond),
	}
}

// auditOptions are testOptions with the audit trail captured by sink.
func auditOptions(sink *recordSink) Options {
	opts := testOptions()
	opts.Audit = audit.New(slog.New(sink))
	return opts
}

// probeTable is the catalog with every handler replaced by fn, so middleware
// behavior can be exercised per tool without any of these handlers existing in
// a shipped build.
func probeTable(fn func(ctx context.Context, name string) error) []Tool {
	tools := Catalog()
	for i := range tools {
		name := tools[i].Name
		tools[i].bind = func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
			def.InputSchema = emptyObjectSchema
			srv.AddTool(def, func(ctx context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
				if err := fn(ctx, name); err != nil {
					return nil, err
				}
				return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "ok"}}}, nil
			})
		}
	}
	return tools
}

// connect wires an in-memory client to srv and returns the client session.
func connect(t *testing.T, srv *sdkmcp.Server) *sdkmcp.ClientSession {
	t.Helper()
	ctx := t.Context()

	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	return clientSession
}

// recordSink captures audit records as attribute maps.
type recordSink struct {
	mu      sync.Mutex
	records []map[string]any
}

func (s *recordSink) Enabled(context.Context, slog.Level) bool { return true }

func (s *recordSink) Handle(_ context.Context, rec slog.Record) error {
	attrs := map[string]any{}
	rec.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, attrs)
	return nil
}

func (s *recordSink) WithAttrs([]slog.Attr) slog.Handler { return s }
func (s *recordSink) WithGroup(string) slog.Handler      { return s }

func (s *recordSink) last() (map[string]any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.records) == 0 {
		return nil, false
	}
	return s.records[len(s.records)-1], true
}
