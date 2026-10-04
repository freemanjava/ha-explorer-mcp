package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

const wireTestToken = "wire-test-token"

// wireStack is a real ha.Manager and ha.SupervisorClient talking to fake
// upstreams that count what actually reaches them — the ground truth the
// audited ha_requests is compared with (F-46).
type wireStack struct {
	wsFrames  atomic.Int64
	restCalls atomic.Int64
	mu        sync.Mutex
	commands  []string
	opts      Options
	sink      *recordSink
}

func (w *wireStack) wireRequests() int { return int(w.wsFrames.Load() + w.restCalls.Load()) }

func (w *wireStack) fixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return raw
}

func newWireStack(t *testing.T) *wireStack {
	t.Helper()
	w := &wireStack{}

	results := map[string]json.RawMessage{
		ha.CommandConfigEntriesGet:    w.fixture(t, "config_entries_get.json"),
		ha.CommandEntityRegistryList:  w.fixture(t, "entity_registry_list.json"),
		ha.CommandDeviceRegistryList:  w.fixture(t, "device_registry_list.json"),
		ha.CommandAreaRegistryList:    w.fixture(t, "area_registry_list.json"),
		ha.CommandGetConfig:           json.RawMessage(`{"version":"2026.8.0","location_name":"Home","time_zone":"UTC","unit_system":{},"components":[]}`),
		ha.CommandAutomationConfig:    w.fixture(t, "automation_config.json"),
		ha.CommandHistoryDuringPeriod: json.RawMessage(`{}`),
	}

	hasrv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(rw, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ctx := r.Context()
		if wsjson.Write(ctx, conn, map[string]any{"type": "auth_required", "ha_version": "2026.8.0"}) != nil {
			return
		}
		var auth map[string]any
		if wsjson.Read(ctx, conn, &auth) != nil {
			return
		}
		if wsjson.Write(ctx, conn, map[string]any{"type": "auth_ok", "ha_version": "2026.8.0"}) != nil {
			return
		}
		for {
			var cmd struct {
				ID   uint64 `json:"id"`
				Type string `json:"type"`
			}
			if wsjson.Read(ctx, conn, &cmd) != nil {
				return
			}
			if cmd.Type == "ping" {
				_ = wsjson.Write(ctx, conn, map[string]any{"type": "pong", "id": cmd.ID})
				continue
			}
			w.wsFrames.Add(1)
			w.mu.Lock()
			w.commands = append(w.commands, cmd.Type)
			w.mu.Unlock()
			result, ok := results[cmd.Type]
			if !ok {
				result = json.RawMessage(`[]`)
			}
			_ = wsjson.Write(ctx, conn, map[string]any{"id": cmd.ID, "type": "result", "success": true, "result": result})
		}
	}))
	t.Cleanup(hasrv.Close)

	supsrv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		w.restCalls.Add(1)
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"result":"ok","data":{}}`))
	}))
	t.Cleanup(supsrv.Close)

	manager := ha.NewManager("ws"+strings.TrimPrefix(hasrv.URL, "http"), wireTestToken, nil)
	manager.Start(context.Background())
	t.Cleanup(manager.Close)
	registry := ha.NewRegistryCache(manager)
	core := ha.NewCoreReader(manager)
	supervisor := ha.NewSupervisorClient(supsrv.URL, wireTestToken, supsrv.Client(), nil)

	w.sink = &recordSink{}
	w.opts = auditOptions(w.sink)
	w.opts.Core = core
	w.opts.Inventory = registry
	w.opts.Supervisor = supervisor
	w.opts.Availability = core
	w.opts.States = core
	w.opts.Areas = registry
	w.opts.Automations = core
	w.opts.Repairs = core
	w.opts.History = core
	w.opts.AutomationDetail = core
	w.opts.Logbook = core
	w.opts.Lifecycle = core
	return w
}

func (w *wireStack) call(t *testing.T, client *sdkmcp.ClientSession, name string, args map[string]any) (*sdkmcp.CallToolResult, map[string]any) {
	t.Helper()
	before := len(w.sink.records)
	res, _ := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: name, Arguments: args})
	w.sink.mu.Lock()
	defer w.sink.mu.Unlock()
	if len(w.sink.records) != before+1 {
		t.Fatalf("%s: %d audit records emitted, want 1", name, len(w.sink.records)-before)
	}
	return res, w.sink.records[len(w.sink.records)-1]
}

// TestListIntegrations_ColdThenWarmRegistry_AuditsWireRequests: the audited
// count is what reached the fake HA — cold refill counted against the
// invocation that triggered it, warm cache counted as nothing.
func TestListIntegrations_ColdThenWarmRegistry_AuditsWireRequests(t *testing.T) {
	w := newWireStack(t)
	client := connect(t, NewServer(w.opts))

	_, cold := w.call(t, client, "list_integrations", nil)
	coldWire := w.wireRequests()
	if coldWire == 0 {
		t.Fatal("cold list_integrations sent nothing upstream")
	}
	if got := auditedRequests(cold); got != coldWire {
		t.Errorf("cold audit ha_requests = %v, want %d (the requests the fake HA received)", got, coldWire)
	}

	// Only the registry is cached; availability reads states every time. A
	// warm call must audit exactly what reached the wire this time, and
	// strictly less than the cold one.
	_, warm := w.call(t, client, "list_integrations", nil)
	warmWire := w.wireRequests() - coldWire
	if got := auditedRequests(warm); got != warmWire {
		t.Errorf("warm audit ha_requests = %v, want %d", got, warmWire)
	}
	if warmWire >= coldWire {
		t.Errorf("warm call sent %d requests, cold sent %d: the cache did not spare any", warmWire, coldWire)
	}
}

// TestCatalog_EveryTool_CountedRequestsFitItsClassAndMatchTheWire drives all
// twenty-one tools and holds the audit to the wire: nothing a tool sends goes
// uncounted (F-46), and nothing exceeds the class's request bound.
func TestCatalog_EveryTool_CountedRequestsFitItsClassAndMatchTheWire(t *testing.T) {
	args := map[string]map[string]any{
		"get_integration":            {"id": "entry-zha-1"},
		"get_device":                 {"id": "device-1"},
		"get_entity":                 {"id": "sensor.x"},
		"get_entity_history":         {"entity_id": "sensor.x", "from": time.Now().Add(-time.Hour).Format(time.RFC3339), "to": time.Now().Format(time.RFC3339)},
		"get_entity_statistics":      {"entity_id": "sensor.x"},
		"get_automation":             {"entity_id": "automation.x"},
		"get_automation_traces":      {"entity_id": "automation.x"},
		"analyze_entity_health":      {"entity_id": "sensor.x"},
		"analyze_integration_health": {"config_entry_id": "entry-zha-1"},
		"analyze_automation_health":  {"entity_id": "automation.x"},
	}

	for _, tool := range Catalog() {
		t.Run(tool.Name, func(t *testing.T) {
			w := newWireStack(t)
			client := connect(t, NewServer(w.opts))

			_, rec := w.call(t, client, tool.Name, args[tool.Name])

			got := auditedRequests(rec)
			if want := w.wireRequests(); got != want {
				t.Errorf("audited ha_requests = %d, wire received %d (status %v, reason %v)", got, want, rec["status"], rec["reason"])
			}
			if limit := policy.LimitsFor(tool.Class).MaxHARequests; got > limit {
				t.Errorf("counted %d requests, class %s allows %d", got, tool.Class, limit)
			}
		})
	}
}

// TestInternalMCP_NoToolChargesHARequests: the wire seams in internal/ha count
// requests; a tool that also charged would double-count (D-08-16).
func TestInternalMCP_NoToolChargesHARequests(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "ChargeHARequests") {
			t.Errorf("%s calls ChargeHARequests; requests are counted at the wire seams in internal/ha", f)
		}
	}
}

// auditedRequests reads ha_requests off a captured audit record; slog hands
// integers back as int64.
func auditedRequests(rec map[string]any) int {
	n, _ := rec["ha_requests"].(int64)
	return int(n)
}

// resultSize sums what the agent received: structured content plus text blocks.
func resultSize(t *testing.T, res *sdkmcp.CallToolResult) int {
	t.Helper()
	n := 0
	if res.StructuredContent != nil {
		raw, err := json.Marshal(res.StructuredContent)
		if err != nil {
			t.Fatalf("marshal structured content: %v", err)
		}
		n += len(raw)
	}
	for _, c := range res.Content {
		if text, ok := c.(*sdkmcp.TextContent); ok {
			n += len(text.Text)
		}
	}
	return n
}

func auditedBytes(rec map[string]any) int {
	n, _ := rec["result_bytes"].(int64)
	return int(n)
}

// TestInvocation_ResultBytes_MeasuredFromReturnedResult: the audited size is
// the result's own, whether or not the tool charged bytes to the budget (F-46).
func TestInvocation_ResultBytes_MeasuredFromReturnedResult(t *testing.T) {
	now := time.Now()
	cases := map[string]map[string]any{
		"list_integrations":   nil,
		"get_system_overview": nil,
		// get_entity_history charges bytes; the audit must still report the size.
		"get_entity_history": {"entity_id": "sensor.x", "from": now.Add(-time.Hour).Format(time.RFC3339), "to": now.Format(time.RFC3339)},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			w := newWireStack(t)
			client := connect(t, NewServer(w.opts))

			res, rec := w.call(t, client, name, args)
			if res == nil || res.IsError {
				t.Fatalf("call did not succeed: %+v", rec)
			}
			got, want := auditedBytes(rec), resultSize(t, res)
			if got <= 0 {
				t.Fatalf("audited result_bytes = %d, want > 0", got)
			}
			if got != want {
				t.Errorf("audited result_bytes = %d, result carries %d", got, want)
			}
		})
	}
}

// TestInvocation_ErrorResult_AuditsZeroBytes: a call that returns no result
// has no size.
func TestInvocation_ErrorResult_AuditsZeroBytes(t *testing.T) {
	w := newWireStack(t)
	client := connect(t, NewServer(w.opts))

	_, rec := w.call(t, client, "get_entity", map[string]any{"id": ""})
	if got := auditedBytes(rec); got != 0 {
		t.Errorf("audited result_bytes = %d for a failed call, want 0 (status %v)", got, rec["status"])
	}
}
