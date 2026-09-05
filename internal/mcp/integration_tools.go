package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/page"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// integrationRegistryReader is list_integrations/get_integration's read
// surface into the slow-moving registries: the config entries themselves,
// plus the entity and device registries needed to compute their counts —
// the underlying lists never leave this package (P3-03 DoD).
type integrationRegistryReader interface {
	ConfigEntries(ctx context.Context) ([]model.Integration, time.Time, error)
	Entities(ctx context.Context) ([]model.Entity, time.Time, error)
	Devices(ctx context.Context) ([]model.DeviceRef, time.Time, error)
}

// entityAvailabilityReader reports which entities are currently unavailable
// or unknown, aggregated in-process — never the full per-entity state list.
type entityAvailabilityReader interface {
	UnavailableEntityIDs(ctx context.Context) (map[model.EntityID]struct{}, error)
}

// ListIntegrationsInput is list_integrations' typed, validated input: an
// optional domain/disabled filter plus the Phase 02 cursor-pagination
// contract. No field accepts a route, command, path or query (rule 2).
type ListIntegrationsInput struct {
	// Domain filters to config entries of one HA integration domain (e.g.
	// "hue"). Empty means no filter.
	Domain string `json:"domain,omitempty" jsonschema:"filter to one integration domain, e.g. hue"`
	// Disabled, given, filters to only-disabled (true) or only-enabled
	// (false) config entries. Omitted means no filter.
	Disabled *bool `json:"disabled,omitempty" jsonschema:"filter by whether the config entry is disabled"`

	Cursor string `json:"cursor,omitempty" jsonschema:"resume after this page's cursor"`
	Limit  int    `json:"limit,omitempty" jsonschema:"page size, default 50, max 200"`
}

// GetIntegrationInput is get_integration's typed input: exactly the config
// entry id, nothing an agent could use as a free-form route or query.
type GetIntegrationInput struct {
	ID string `json:"id" jsonschema:"the config entry id to drill into"`
}

// withIntegrationTools returns tools with list_integrations and
// get_integration's handlers bound, when opts supplies readers for both. A
// row whose readers are absent keeps its bindNotImplemented default.
func withIntegrationTools(tools []Tool, opts Options) []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	for i := range out {
		if opts.Inventory == nil || opts.Availability == nil {
			continue
		}
		switch out[i].Name {
		case "list_integrations":
			out[i].bind = bindListIntegrations(opts.Inventory, opts.Availability)
		case "get_integration":
			out[i].bind = bindGetIntegration(opts.Inventory, opts.Availability)
		}
	}
	return out
}

// bindListIntegrations registers list_integrations' typed handler. The input
// schema is left to inference from ListIntegrationsInput, which the SDK
// closes with additionalProperties:false — a free-form parameter is refused
// by the schema itself, not by handler-side discipline.
func bindListIntegrations(registry integrationRegistryReader, avail entityAvailabilityReader) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in ListIntegrationsInput) (*sdkmcp.CallToolResult, model.IntegrationList, error) {
			out, err := listIntegrations(ctx, registry, avail, in)
			return nil, out, err
		})
	}
}

// bindGetIntegration registers get_integration's typed handler.
func bindGetIntegration(registry integrationRegistryReader, avail entityAvailabilityReader) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in GetIntegrationInput) (*sdkmcp.CallToolResult, model.IntegrationDetail, error) {
			out, err := getIntegration(ctx, registry, avail, in)
			return nil, out, err
		})
	}
}

// integrationSummaries builds every config entry's summary: its entity,
// device and unavailable-entity counts, computed server-side from the
// registries in one pass rather than by returning the entity or device
// lists themselves (P3-03 DoD).
func integrationSummaries(ctx context.Context, registry integrationRegistryReader, avail entityAvailabilityReader) ([]model.IntegrationSummary, time.Time, error) {
	entries, observedAt, err := registry.ConfigEntries(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	entities, _, err := registry.Entities(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	devices, _, err := registry.Devices(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	unavailable, err := avail.UnavailableEntityIDs(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}

	entityCount := make(map[model.ConfigEntryID]int, len(entries))
	unavailableCount := make(map[model.ConfigEntryID]int, len(entries))
	for _, e := range entities {
		entityCount[e.ConfigEntryID]++
		if _, ok := unavailable[e.ID]; ok {
			unavailableCount[e.ConfigEntryID]++
		}
	}
	deviceCount := make(map[model.ConfigEntryID]int, len(entries))
	for _, d := range devices {
		deviceCount[d.ConfigEntryID]++
	}

	out := make([]model.IntegrationSummary, len(entries))
	for i, entry := range entries {
		out[i] = model.IntegrationSummary{
			Integration:         entry,
			EntityCount:         entityCount[entry.ID],
			DeviceCount:         deviceCount[entry.ID],
			UnavailableEntities: unavailableCount[entry.ID],
		}
	}
	return out, observedAt, nil
}

// matchesIntegrationFilter reports whether s passes in's optional filters. A
// zero-value filter field matches everything, so an unset Domain or Disabled
// never excludes an entry.
func matchesIntegrationFilter(s model.IntegrationSummary, in ListIntegrationsInput) bool {
	if in.Domain != "" && s.Domain != in.Domain {
		return false
	}
	if in.Disabled != nil && s.Disabled != *in.Disabled {
		return false
	}
	return true
}

// listIntegrations filters, sorts by id, and pages the config-entry
// summaries — an entry in a failed setup state is filtered and sorted like
// any other, never dropped for carrying an error state (P3-03 DoD).
func listIntegrations(ctx context.Context, registry integrationRegistryReader, avail entityAvailabilityReader, in ListIntegrationsInput) (model.IntegrationList, error) {
	summaries, observedAt, err := integrationSummaries(ctx, registry, avail)
	if err != nil {
		return model.IntegrationList{}, err
	}

	filtered := make([]model.IntegrationSummary, 0, len(summaries))
	for _, s := range summaries {
		if matchesIntegrationFilter(s, in) {
			filtered = append(filtered, s)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID < filtered[j].ID })

	pg, err := page.Paginate(filtered, in.Cursor, in.Limit, maxResponseBytes(ctx),
		func(s model.IntegrationSummary) string { return string(s.ID) },
		integrationByteSize,
	)
	if err != nil {
		return model.IntegrationList{}, err
	}

	return model.IntegrationList{
		Source:       "home_assistant_core",
		ObservedAt:   observedAt,
		Items:        pg.Items,
		NextCursor:   pg.NextCursor,
		Truncated:    pg.Truncated,
		LimitClamped: pg.LimitClamped,
	}, nil
}

// getIntegration drills into one config entry by id. A missing id is
// ErrNotFound, not a partially-populated object (Appendix B: "gone between
// list and get").
func getIntegration(ctx context.Context, registry integrationRegistryReader, avail entityAvailabilityReader, in GetIntegrationInput) (model.IntegrationDetail, error) {
	if in.ID == "" {
		return model.IntegrationDetail{}, fmt.Errorf("get_integration: id is required")
	}

	summaries, observedAt, err := integrationSummaries(ctx, registry, avail)
	if err != nil {
		return model.IntegrationDetail{}, err
	}
	for _, s := range summaries {
		if string(s.ID) == in.ID {
			return model.IntegrationDetail{
				Source:             "home_assistant_core",
				ObservedAt:         observedAt,
				IntegrationSummary: s,
			}, nil
		}
	}
	return model.IntegrationDetail{}, fmt.Errorf("%w: config entry %q", ha.ErrNotFound, in.ID)
}

// integrationByteSize approximates one summary's serialized size for the
// page package's byte cap — cheap enough to run per record without
// re-serializing the whole response afterward.
func integrationByteSize(s model.IntegrationSummary) int64 {
	b, err := json.Marshal(s)
	if err != nil {
		return 0
	}
	return int64(len(b))
}

// maxResponseBytes reads the invocation's charged byte cap from ctx, falling
// back to the normal-read default if no budget was attached (a handler
// invoked directly in a test, outside the middleware).
func maxResponseBytes(ctx context.Context) int64 {
	if b, ok := policy.BudgetFrom(ctx); ok {
		return b.Limits().MaxBytes
	}
	return policy.LimitsFor(policy.ClassNormalRead).MaxBytes
}
