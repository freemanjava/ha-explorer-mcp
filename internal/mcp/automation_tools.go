package mcp

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/page"
)

// automationReader is list_automations' read surface: get_states' automation-
// domain entries, mapped to enabled state and last_triggered — the confirmed
// non-admin fallback source
// (docs/research/2026-08-23-ha-automation-traces.md), not automation/config's
// admin-gated detail, which get_automation (P3-07) reaches instead.
type automationReader interface {
	Automations(ctx context.Context) ([]model.AutomationSummary, error)
}

// ListAutomationsInput is list_automations' typed, validated input: an
// optional enabled filter plus the Phase 02 cursor-pagination contract.
type ListAutomationsInput struct {
	// Enabled, given, filters to only-enabled (true) or only-disabled
	// (false) automations. Omitted means no filter.
	Enabled *bool `json:"enabled,omitempty" jsonschema:"filter by whether the automation is enabled"`

	Cursor string `json:"cursor,omitempty" jsonschema:"resume after this page's cursor"`
	Limit  int    `json:"limit,omitempty" jsonschema:"page size, default 50, max 200"`
}

// withAutomationTools returns tools with list_automations' handler bound,
// when opts supplies a reader. A row whose reader is absent keeps its
// bindNotImplemented default.
func withAutomationTools(tools []Tool, opts Options) []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	for i := range out {
		if out[i].Name == "list_automations" && opts.Automations != nil {
			out[i].bind = bindListAutomations(opts.Automations)
		}
	}
	return out
}

// bindListAutomations registers list_automations' typed handler.
func bindListAutomations(reader automationReader) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in ListAutomationsInput) (*sdkmcp.CallToolResult, model.AutomationList, error) {
			out, err := listAutomations(ctx, reader, in)
			return nil, out, err
		})
	}
}

// listAutomations filters, sorts by entity id, and pages the automation
// summaries get_states yields.
func listAutomations(ctx context.Context, reader automationReader, in ListAutomationsInput) (model.AutomationList, error) {
	automations, err := reader.Automations(ctx)
	if err != nil {
		return model.AutomationList{}, err
	}

	filtered := make([]model.AutomationSummary, 0, len(automations))
	for _, a := range automations {
		if in.Enabled != nil && a.Enabled != *in.Enabled {
			continue
		}
		filtered = append(filtered, a)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].EntityID < filtered[j].EntityID })

	pg, err := page.Paginate(filtered, in.Cursor, in.Limit, maxResponseBytes(ctx),
		func(a model.AutomationSummary) string { return string(a.EntityID) },
		automationByteSize,
	)
	if err != nil {
		return model.AutomationList{}, err
	}

	return model.AutomationList{
		Source:       "home_assistant_core",
		ObservedAt:   time.Now().UTC(),
		Items:        pg.Items,
		NextCursor:   pg.NextCursor,
		Truncated:    pg.Truncated,
		LimitClamped: pg.LimitClamped,
	}, nil
}

// automationByteSize approximates one automation summary's serialized size
// for the page package's byte cap — cheap enough to run per record without
// re-serializing the whole response afterward.
func automationByteSize(a model.AutomationSummary) int64 {
	b, err := json.Marshal(a)
	if err != nil {
		return 0
	}
	return int64(len(b))
}
