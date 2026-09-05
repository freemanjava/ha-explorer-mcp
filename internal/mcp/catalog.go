package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// Tool is one entry in the static tool catalog. The catalog is the whole
// definition of what this server exposes: a tool that is not a row here does
// not exist, and there is no dynamic registration path an input could reach
// (phase 03 design notes, doc §11).
//
// Every row carries a budget class, so "registered without a budget" is not a
// state the type can express (P3-01 DoD).
type Tool struct {
	// Name is the MCP tool name, snake_case, matching doc §9's catalog exactly.
	Name string
	// Description is what the client shows the model. It says what the tool
	// answers, not which upstream endpoint it calls (ADR-007).
	Description string
	// Class is the query budget this tool's invocations are charged against.
	Class policy.Class
	// bind registers the tool's typed handler on an SDK server. It is nil
	// until the phase task that implements the tool lands; Register then
	// wires bindNotImplemented in its place, so the catalog stays the single
	// list of what exists and a half-built tool answers honestly rather than
	// being silently absent.
	bind binder
}

// binder registers one catalog row's handler on the SDK server. The row's
// metadata is already applied to def; a binder adds the input schema and the
// handler, which is where a later task's typed sdkmcp.AddTool[In, Out] goes.
type binder func(srv *sdkmcp.Server, def *sdkmcp.Tool)

// catalog is doc §9's twenty tools, in doc order. The 2026-08-25 decision
// (phase 03) is that all twenty ship: a tool the evidence rules out answers
// with a reason, it is never dropped from this table. The registry test
// asserting these exact names is what keeps "twenty" a fact rather than a
// claim.
//
// Budget class: only the two tools doc §9 itself calls composite carry
// ClassComposite. Everything else takes the tighter normal-read budget —
// failing closed on spend, the way rule 3 fails closed on access. A tool that
// turns out to genuinely fan out (the find_* pair is the likely candidate)
// gets re-classed by the task that implements it, with the measurement that
// justifies it.
var catalog = []Tool{
	{Name: "get_system_overview", Class: policy.ClassNormalRead,
		Description: "Root discovery snapshot of the Home Assistant installation: version, installation metadata, inventory counts and headline health."},
	{Name: "get_system_health", Class: policy.ClassNormalRead,
		Description: "Core, OS and Supervisor resource and service health, where the granted Supervisor role exposes it."},
	{Name: "list_integrations", Class: policy.ClassNormalRead,
		Description: "Integration and config-entry summary with per-integration entity, device and unavailable counts."},
	{Name: "get_integration", Class: policy.ClassNormalRead,
		Description: "Drill-down for one integration or config entry, including its setup state."},
	{Name: "list_devices", Class: policy.ClassNormalRead,
		Description: "Filtered, paginated device inventory."},
	{Name: "get_device", Class: policy.ClassNormalRead,
		Description: "Device metadata with its related entities and via/parent topology."},
	{Name: "list_entities", Class: policy.ClassNormalRead,
		Description: "Filtered, paginated entity inventory."},
	{Name: "get_entity", Class: policy.ClassNormalRead,
		Description: "Current state of one entity, enriched with registry, device and area metadata."},
	{Name: "get_entity_history", Class: policy.ClassNormalRead,
		Description: "Bounded raw history for named entities over an explicit time range."},
	{Name: "get_entity_statistics", Class: policy.ClassNormalRead,
		Description: "Server-side availability, update-cadence and outage statistics for one entity over a bounded period."},
	{Name: "find_unavailable_entities", Class: policy.ClassNormalRead,
		Description: "Entities currently unavailable or unknown, or with recent availability problems."},
	{Name: "find_stale_entities", Class: policy.ClassNormalRead,
		Description: "Entities whose updates are unexpectedly old or irregular relative to their own observed cadence."},
	{Name: "list_areas", Class: policy.ClassNormalRead,
		Description: "Area topology, with optional floor and label mapping."},
	{Name: "list_automations", Class: policy.ClassNormalRead,
		Description: "Automation inventory with enabled state and last triggered time."},
	{Name: "get_automation", Class: policy.ClassNormalRead,
		Description: "Automation details through the supported-and-safe adapter."},
	{Name: "get_automation_traces", Class: policy.ClassNormalRead,
		Description: "Automation execution evidence, through a compatibility-sensitive adapter that reports when traces are unavailable and why."},
	{Name: "list_repairs", Class: policy.ClassNormalRead,
		Description: "Native Home Assistant Repairs and issues, with severity and issue id."},
	{Name: "list_apps", Class: policy.ClassNormalRead,
		Description: "Supervisor App inventory and state, where the granted Supervisor role exposes it."},
	{Name: "analyze_entity_health", Class: policy.ClassComposite,
		Description: "Composite deterministic health analysis for one entity: observed facts, inferences and recommendations kept separate."},
	{Name: "analyze_integration_health", Class: policy.ClassComposite,
		Description: "Composite health and outage-correlation analysis for one integration."},
}

// Catalog returns the static tool table. The slice is a copy so a caller
// cannot mutate the registry it was built from.
func Catalog() []Tool {
	out := make([]Tool, len(catalog))
	copy(out, catalog)
	return out
}

// lookup returns the catalog row for a tool name. The second result is false
// for a name that is not in the table — the caller must then refuse, never
// invent a budget for it (rule 3).
func lookup(tools []Tool, name string) (Tool, bool) {
	for _, t := range tools {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

// emptyObjectSchema is the placeholder input schema for a catalog row whose
// implementing task has not landed. It accepts an object with no properties,
// so a not-yet-implemented tool cannot accept a free-form parameter either.
var emptyObjectSchema = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)

// register adds one catalog row to the SDK server, applying the metadata every
// tool shares: the read-only annotation (doc §11's first enforcement point —
// the registry registers only read tools) and, for a row without a handler
// yet, the not-implemented binder.
func register(srv *sdkmcp.Server, t Tool) {
	def := &sdkmcp.Tool{
		Name:        t.Name,
		Description: t.Description,
		Annotations: &sdkmcp.ToolAnnotations{ReadOnlyHint: true},
	}
	bind := t.bind
	if bind == nil {
		bind = bindNotImplemented
	}
	bind(srv, def)
}

// bindNotImplemented registers a row whose implementing task has not landed.
//
// It answers with ErrNotImplemented, deliberately not with "unsupported":
// unsupported is a fact about this installation (an API this HA version or
// this principal does not offer), and letting an unfinished build borrow that
// word would make the two indistinguishable to an agent that must separate
// what it observed from what it could not check (CLAUDE.md rule 7).
func bindNotImplemented(srv *sdkmcp.Server, def *sdkmcp.Tool) {
	def.InputSchema = emptyObjectSchema
	name := def.Name
	srv.AddTool(def, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		return nil, fmt.Errorf("%w: %s", ErrNotImplemented, name)
	})
}
