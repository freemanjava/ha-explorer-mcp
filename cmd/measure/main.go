// Command measure answers P5-10 (F-26, F-28): what the composite tools cost on
// a real installation, driven through the real server code.
//
// Unlike cmd/spike, which probes raw HA commands with its own dialer, this runs
// the shipped tools in-process — ha.Manager, the real readers, the real
// invocation middleware — over an in-memory MCP session, and reads each call's
// cost from the server's own audit record. A harness that stubbed the readers
// would measure the stub. It issues only what the tools themselves issue, and
// there is no write path in the binary it links.
//
// Output is markdown containing counts, sizes and timings only — ordinal
// labels instead of ids, never a value from the installation, never the token.
// The owner runs it and pastes the report; no HA token reaches the agent.
//
// Usage:
//
//	HA_URL=http://homeassistant.local:8123 HA_TOKEN=<long-lived token> \
//	  go run ./cmd/measure > report.md
//
// Supervisor is not wired (it is unreachable outside an App), so
// analyze_integration_health runs with that evidence reported as missing, which
// is the degraded path production takes when Supervisor is down.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/mcp"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

const (
	// callGap keeps clear of the server's own invocation limiter
	// (policy.invocationInterval = 500ms): a refused call would be measuring
	// the limiter, not the tool.
	callGap = 700 * time.Millisecond

	// runBudget bounds the whole run; each call is itself bounded by the tool's
	// composite deadline.
	runBudget = 20 * time.Minute

	// pageLimit is find_stale_entities' maximum page (doc §9.1). The widest page
	// is the one that exposes the 20-request ceiling.
	pageLimit = 200
	// maxStalePages bounds how far one run follows the cursor.
	maxStalePages = 10

	// perToolTargets is how many targets each analyze_* tool is measured on.
	perToolTargets = 3

	period7d  = "7d"
	period24h = "24h"
)

// periodWidth maps the period strings this command sends to their width, for
// the span-over-period share. time.ParseDuration has no "d" unit.
var periodWidth = map[string]time.Duration{period7d: 7 * 24 * time.Hour, period24h: 24 * time.Hour}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "measure: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	baseURL := strings.TrimSuffix(os.Getenv("HA_URL"), "/")
	token := os.Getenv("HA_TOKEN")
	if baseURL == "" || token == "" {
		return fmt.Errorf("set HA_URL (e.g. http://homeassistant.local:8123) and HA_TOKEN")
	}

	ctx, cancel := context.WithTimeout(context.Background(), runBudget)
	defer cancel()

	capture := &auditCapture{}
	log := slog.New(capture)

	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/api/websocket"
	manager := ha.NewManager(wsURL, token, log)
	manager.Start(ctx)
	defer manager.Close()

	registry := ha.NewRegistryCache(manager)
	core := ha.NewCoreReader(manager)
	srv := mcp.NewServer(mcp.Options{
		Version: "measure", Logger: log, Secrets: []string{token},
		Core: core, Inventory: registry, Availability: core, States: core,
		Areas: registry, Automations: core, Repairs: core, History: core,
		AutomationDetail: core, Logbook: core,
	})

	client, closeSession, err := connect(ctx, srv)
	if err != nil {
		return err
	}
	defer closeSession()

	m := &measurer{ctx: ctx, client: client, capture: capture, out: &report{}, classes: classesByTool()}
	m.out.writef("# composite budget measurement (P5-10)\n\nRun at %s (UTC)\n\n", time.Now().UTC().Format(time.RFC3339))
	m.out.writef("Costs are the server's own audit records: HA requests, result bytes and " +
		"server milliseconds per invocation, against the limit of the tool's budget class. " +
		"`wall` is what the client waited. Supervisor is not wired.\n\n")

	targets, err := m.discover()
	if err != nil {
		return err
	}
	m.measureStale()
	m.measureIntegrationHealth(targets.integrations)
	m.measureEntityHealth(targets.entities)
	m.measureAutomationHealth(targets.automations)

	fmt.Print(m.out.String())
	return nil
}

func connect(ctx context.Context, srv *sdkmcp.Server) (*sdkmcp.ClientSession, func(), error) {
	serverT, clientT := sdkmcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("server connect: %w", err)
	}
	cs, err := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "measure", Version: "0"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		_ = ss.Close()
		return nil, nil, fmt.Errorf("client connect: %w", err)
	}
	return cs, func() { _ = cs.Close(); _ = ss.Close() }, nil
}

func classesByTool() map[string]policy.Class {
	out := map[string]policy.Class{}
	for _, t := range mcp.Catalog() {
		out[t.Name] = t.Class
	}
	return out
}

type measurer struct {
	ctx     context.Context
	client  *sdkmcp.ClientSession
	capture *auditCapture
	out     *report
	classes map[string]policy.Class
}

// call invokes one tool and returns its structured result with the audit row.
// A refused call (budget exceeded, denied) is a result, not a failure: it is
// reported with its reason, and the run goes on.
func (m *measurer) call(name string, args map[string]any) (any, callOutcome) {
	time.Sleep(callGap)
	m.capture.take()
	start := time.Now()
	res, err := m.client.CallTool(m.ctx, &sdkmcp.CallToolParams{Name: name, Arguments: args})
	o := callOutcome{wall: time.Since(start)}
	if rows := m.capture.take(); len(rows) > 0 {
		o.audit = rows[len(rows)-1]
	}
	switch {
	case err != nil:
		o.errMsg = err.Error()
		return nil, o
	case res.IsError:
		o.errMsg = "tool answered with an error result"
		return nil, o
	}
	return res.StructuredContent, o
}

// decode round-trips structured content into a typed shape.
func decode(from any, into any) bool {
	raw, err := json.Marshal(from)
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, into) == nil
}

func (m *measurer) row(tool, label string, o callOutcome) {
	l := policy.LimitsFor(m.classes[tool])
	m.out.callRow(tool, label, o, l.MaxHARequests, l.MaxBytes)
}

type targetSet struct {
	integrations []candidate
	entities     []candidate
	automations  []candidate
}

// discover finds what to measure against, through the same tools an agent
// would use. Its own calls are not part of the report.
func (m *measurer) discover() (targetSet, error) {
	var t targetSet

	var integrations model.IntegrationList
	v, o := m.call("list_integrations", map[string]any{"limit": pageLimit})
	if v == nil || !decode(v, &integrations) {
		return t, fmt.Errorf("list_integrations failed: %s", o.errMsg)
	}
	for i, it := range integrations.Items {
		t.integrations = append(t.integrations, candidate{
			label: fmt.Sprintf("integration #%d (%d entities, %d unavailable)", i+1, it.EntityCount, it.UnavailableEntities),
			id:    string(it.ID), weight: it.EntityCount,
		})
	}

	var unavailable model.UnavailableEntityList
	if v, _ := m.call("find_unavailable_entities", map[string]any{"limit": pageLimit}); v != nil && decode(v, &unavailable) {
		for i, e := range unavailable.Items {
			t.entities = append(t.entities, candidate{label: fmt.Sprintf("unavailable entity #%d", i+1), id: string(e.ID), weight: 1})
		}
	}

	var automations model.AutomationList
	if v, _ := m.call("list_automations", map[string]any{"limit": pageLimit}); v != nil && decode(v, &automations) {
		for i, a := range automations.Items {
			if a.Enabled {
				t.automations = append(t.automations, candidate{label: fmt.Sprintf("automation #%d", i+1), id: string(a.EntityID), weight: 1})
			}
		}
	}

	m.out.writef("Installation: %d config entries, %d unavailable entities (first page), %d enabled automations.\n\n",
		len(integrations.Items), len(unavailable.Items), len(t.automations))
	return t, nil
}

// measureStale follows find_stale_entities' cursor at the widest page, so the
// report says how many calls full coverage costs, not just what one costs.
func (m *measurer) measureStale() {
	m.out.writef("## `find_stale_entities` (F-26), currently `%s`\n\n", m.classes["find_stale_entities"])
	m.out.header()
	cursor, scanned := "", 0
	for page := 1; page <= maxStalePages; page++ {
		args := map[string]any{"limit": pageLimit, "period": period7d}
		if cursor != "" {
			args["cursor"] = cursor
		}
		v, o := m.call("find_stale_entities", args)
		var list model.StaleEntityList
		label := fmt.Sprintf("page %d, limit %d, %s", page, pageLimit, period7d)
		if v != nil && decode(v, &list) {
			scanned += list.Scanned
			label += fmt.Sprintf(": scanned %d, %d stale, truncated=%t", list.Scanned, len(list.Items), list.Truncated)
		}
		m.row("find_stale_entities", label, o)
		if list.NextCursor == "" {
			break
		}
		cursor = list.NextCursor
	}
	m.out.writef("\nCandidates examined across the pages above: %d (cursor %s after %d pages max).\n\n",
		scanned, map[bool]string{true: "exhausted", false: "still open"}[cursor == ""], maxStalePages)
}

func (m *measurer) measureIntegrationHealth(integrations []candidate) {
	m.out.writef("## `analyze_integration_health`, `%s`\n\n", m.classes["analyze_integration_health"])
	m.out.header()
	var clusters []string
	for i, c := range pickWidest(integrations, perToolTargets) {
		periods := []string{period7d}
		if i == 0 {
			periods = append(periods, period24h)
		}
		for _, p := range periods {
			v, o := m.call("analyze_integration_health", map[string]any{"config_entry_id": c.id, "period": p})
			m.row("analyze_integration_health", c.label+", "+p, o)
			if h, ok := healthOf(v); ok {
				clusters = append(clusters, m.clusterLines(c.label+", "+p, h, p)...)
			}
		}
	}
	m.out.writef("\n")
	m.writeClusters(clusters)
}

func (m *measurer) measureEntityHealth(entities []candidate) {
	m.out.writef("## `analyze_entity_health`, `%s`\n\n", m.classes["analyze_entity_health"])
	m.out.header()
	for _, c := range entities[:min(perToolTargets, len(entities))] {
		_, o := m.call("analyze_entity_health", map[string]any{"entity_id": c.id, "period": period7d})
		m.row("analyze_entity_health", c.label+", "+period7d, o)
	}
	m.out.writef("\n")
}

func (m *measurer) measureAutomationHealth(automations []candidate) {
	m.out.writef("## `analyze_automation_health`, `%s`\n\n", m.classes["analyze_automation_health"])
	m.out.header()
	for _, c := range automations[:min(perToolTargets, len(automations))] {
		_, o := m.call("analyze_automation_health", map[string]any{"entity_id": c.id, "period": period7d})
		m.row("analyze_automation_health", c.label+", "+period7d, o)
	}
	m.out.writef("\n")
}

func healthOf(v any) (healthShape, bool) {
	var h healthShape
	if v == nil || !decode(v, &h) {
		return h, false
	}
	return h, true
}

func (m *measurer) clusterLines(label string, h healthShape, period string) []string {
	d := periodWidth[period]
	var lines []string
	for i, c := range summarizeClusters(h, d.Seconds()) {
		lines = append(lines, fmt.Sprintf("| %s | #%d | %d | %d | %.0f | %.0f%% | %t |",
			label, i+1, c.Entities, c.OutagePeriods, c.SpanSeconds, c.SpanShare*100, c.EvidenceFound))
	}
	return lines
}

// writeClusters is the F-28 observation: a cluster whose span is a large share
// of the period and which holds more outage periods than entities is a long
// outage that chained others in.
func (m *measurer) writeClusters(lines []string) {
	m.out.writef("### Outage clusters in the integration analyses (F-28)\n\n")
	if len(lines) == 0 {
		m.out.writef("No clusters were reported by any analysis above.\n\n")
		return
	}
	m.out.writef("| analysis | cluster | entities | outage periods | span (s) | span / period | evidence found |\n|---|--:|--:|--:|--:|--:|---|\n")
	for _, l := range lines {
		m.out.writef("%s\n", l)
	}
	m.out.writef("\n")
}
