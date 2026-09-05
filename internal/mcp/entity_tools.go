package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/page"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
	"github.com/freemanjava/ha-explorer-mcp/internal/redact"
)

// entityRegistryReader is list_entities/get_entity's read surface into the
// slow-moving registries: the entity registry itself, plus the device, area
// and config-entry registries get_entity's metadata enrichment and the
// integration filter need.
type entityRegistryReader interface {
	Entities(ctx context.Context) ([]model.Entity, time.Time, error)
	Devices(ctx context.Context) ([]model.DeviceRef, time.Time, error)
	Areas(ctx context.Context) ([]model.Area, time.Time, error)
	ConfigEntries(ctx context.Context) ([]model.Integration, time.Time, error)
}

// entityStateReader reports every entity's current state string, keyed by
// id — the one place in this package a per-entity value is read directly,
// because reporting it is list_entities/get_entity's job (P3-05), unlike the
// aggregate-only entityAvailabilityReader every other tool uses.
type entityStateReader interface {
	States(ctx context.Context) (map[model.EntityID]string, error)
}

// availabilityAvailable and availabilityUnavailable are the only two values
// the availability filter accepts. Unlike doc §9's richer state vocabulary,
// this mirrors entityAvailabilityReader's own two-way split (unavailable
// lumps in "unknown"), so the filter answers the same question the rest of
// the catalog already does.
const (
	availabilityAvailable   = "available"
	availabilityUnavailable = "unavailable"
)

// ListEntitiesInput is list_entities' typed, validated input: the Appendix
// A.1 filter set plus the Phase 02 cursor-pagination contract. No field
// accepts a route, command, path or query (rule 2).
type ListEntitiesInput struct {
	// Domain filters to one entity domain (e.g. "light"). Empty means no
	// filter.
	Domain string `json:"domain,omitempty" jsonschema:"filter to one entity domain, e.g. light"`
	// Integration filters to entities owned by one integration domain (e.g.
	// "hue"), resolved through the entity's config entry. Empty means no
	// filter.
	Integration string `json:"integration,omitempty" jsonschema:"filter to one integration domain, e.g. hue"`
	// DeviceID filters to entities attached to one device. Empty means no
	// filter.
	DeviceID string `json:"device_id,omitempty" jsonschema:"filter to one device id"`
	// AreaID filters to entities assigned to one area. Empty means no
	// filter.
	AreaID string `json:"area_id,omitempty" jsonschema:"filter to one area id"`
	// State filters to entities whose current state equals this value
	// exactly (e.g. "on"). Empty means no filter.
	State string `json:"state,omitempty" jsonschema:"filter to entities whose current state equals this value exactly"`
	// Availability filters to "available" or "unavailable" (which, like the
	// rest of the catalog, includes "unknown"). Empty means no filter.
	Availability string `json:"availability,omitempty" jsonschema:"filter by availability: available or unavailable"`
	// Category filters to one entity_category (e.g. "diagnostic"). Empty
	// means no filter.
	Category string `json:"category,omitempty" jsonschema:"filter to one entity_category, e.g. diagnostic"`
	// Disabled, given, filters to only-disabled (true) or only-enabled
	// (false) entities. Omitted means no filter.
	Disabled *bool `json:"disabled,omitempty" jsonschema:"filter by whether the entity is disabled"`
	// Search matches a case-insensitive substring against the entity's id or
	// name. It is treated as inert literal text, never interpreted, however
	// it is spelled (threat T2).
	Search string `json:"search,omitempty" jsonschema:"case-insensitive substring match against entity id or name"`

	Cursor string `json:"cursor,omitempty" jsonschema:"resume after this page's cursor"`
	Limit  int    `json:"limit,omitempty" jsonschema:"page size, default 50, max 200"`
}

// GetEntityInput is get_entity's typed input: exactly the entity id, nothing
// an agent could use as a free-form route or query.
type GetEntityInput struct {
	ID string `json:"id" jsonschema:"the entity id to drill into"`
}

// withEntityTools returns tools with list_entities and get_entity's handlers
// bound, when opts supplies readers for all three. A row whose readers are
// absent keeps its bindNotImplemented default.
func withEntityTools(tools []Tool, opts Options) []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	for i := range out {
		if opts.Inventory == nil || opts.Availability == nil || opts.States == nil {
			continue
		}
		switch out[i].Name {
		case "list_entities":
			out[i].bind = bindListEntities(opts.Inventory, opts.Availability, opts.States, opts.Profile, opts.Secrets)
		case "get_entity":
			out[i].bind = bindGetEntity(opts.Inventory, opts.Availability, opts.States, opts.Profile, opts.Secrets)
		}
	}
	return out
}

// bindListEntities registers list_entities' typed handler. The input schema
// is left to inference from ListEntitiesInput, which the SDK closes with
// additionalProperties:false.
func bindListEntities(registry entityRegistryReader, avail entityAvailabilityReader, states entityStateReader, profile policy.Profile, secrets []string) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in ListEntitiesInput) (*sdkmcp.CallToolResult, model.EntityList, error) {
			out, err := listEntities(ctx, registry, avail, states, profile, secrets, in)
			return nil, out, err
		})
	}
}

// bindGetEntity registers get_entity's typed handler.
func bindGetEntity(registry entityRegistryReader, avail entityAvailabilityReader, states entityStateReader, profile policy.Profile, secrets []string) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in GetEntityInput) (*sdkmcp.CallToolResult, model.EntityDetail, error) {
			out, err := getEntity(ctx, registry, avail, states, profile, secrets, in)
			return nil, out, err
		})
	}
}

// matchesEntityFilter reports whether e passes in's optional filters,
// evaluated against raw (unmasked) data: the filter's own inputs are never
// more revealing than what the caller already supplied, and the response
// field they help select is redacted independently before it leaves the
// boundary.
func matchesEntityFilter(e model.Entity, state string, unavailable map[model.EntityID]struct{}, integrationDomains map[model.ConfigEntryID]string, in ListEntitiesInput) bool {
	if in.Domain != "" && e.Domain != in.Domain {
		return false
	}
	if in.Integration != "" && integrationDomains[e.ConfigEntryID] != in.Integration {
		return false
	}
	if in.DeviceID != "" && string(e.DeviceID) != in.DeviceID {
		return false
	}
	if in.AreaID != "" && string(e.AreaID) != in.AreaID {
		return false
	}
	if in.State != "" && state != in.State {
		return false
	}
	if in.Availability != "" {
		_, isUnavailable := unavailable[e.ID]
		if isUnavailable != (in.Availability == availabilityUnavailable) {
			return false
		}
	}
	if in.Category != "" && e.EntityCategory != in.Category {
		return false
	}
	if in.Disabled != nil && (e.DisabledBy != "") != *in.Disabled {
		return false
	}
	if in.Search != "" && !matchesSearch(e, in.Search) {
		return false
	}
	return true
}

// matchesSearch reports whether search appears, case-insensitively, in e's id
// or name. It is a plain substring test: HA-supplied text is untrusted data
// (CLAUDE.md rule 6), and a substring match never interprets it as anything
// but literal characters (threat T2).
func matchesSearch(e model.Entity, search string) bool {
	needle := strings.ToLower(search)
	if strings.Contains(strings.ToLower(string(e.ID)), needle) {
		return true
	}
	return strings.Contains(strings.ToLower(e.Name), needle)
}

// entitySummaries builds every registry entity's summary, filters it, and
// resolves the integration-domain lookup the filter and this need in common.
func entitySummaries(ctx context.Context, registry entityRegistryReader, avail entityAvailabilityReader, states entityStateReader, redactor *redact.Redactor, in ListEntitiesInput) ([]model.EntitySummary, time.Time, error) {
	entities, observedAt, err := registry.Entities(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	stateValues, err := states.States(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	unavailable, err := avail.UnavailableEntityIDs(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	entries, _, err := registry.ConfigEntries(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	integrationDomains := make(map[model.ConfigEntryID]string, len(entries))
	for _, entry := range entries {
		integrationDomains[entry.ID] = entry.Domain
	}

	out := make([]model.EntitySummary, 0, len(entities))
	for _, e := range entities {
		state := stateValues[e.ID]
		if !matchesEntityFilter(e, state, unavailable, integrationDomains, in) {
			continue
		}
		_, isUnavailable := unavailable[e.ID]
		out = append(out, model.EntitySummary{
			Entity:    e,
			State:     maskEntityState(redactor, e, state),
			Available: !isUnavailable,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, observedAt, nil
}

// maskEntityState applies the Phase 02 profile to one entity's live state
// value, reusing internal/redact's own classification and masking rather
// than re-implementing it (P3-05 DoD: "a PRIVATE-classified entity is
// handled per the Phase 02 profile"). The synthetic payload mirrors the
// shape internal/redact already recognizes from a raw HA state object.
func maskEntityState(redactor *redact.Redactor, e model.Entity, state string) string {
	res := redactor.Payload(map[string]any{
		"entity_id":    string(e.ID),
		"device_class": e.DeviceClass,
		"state":        state,
	})
	masked, ok := res.Value.(map[string]any)["state"].(string)
	if !ok {
		return state
	}
	return masked
}

// listEntities validates input, filters, sorts by id, and pages the entity
// registry enriched with current state. limit defaults to 50 and clamps at
// 200 (P3-05 DoD).
func listEntities(ctx context.Context, registry entityRegistryReader, avail entityAvailabilityReader, states entityStateReader, profile policy.Profile, secrets []string, in ListEntitiesInput) (model.EntityList, error) {
	if in.Availability != "" && in.Availability != availabilityAvailable && in.Availability != availabilityUnavailable {
		return model.EntityList{}, fmt.Errorf("list_entities: availability must be %q or %q, got %q", availabilityAvailable, availabilityUnavailable, in.Availability)
	}

	redactor := redact.New(profile, secrets...)
	summaries, observedAt, err := entitySummaries(ctx, registry, avail, states, redactor, in)
	if err != nil {
		return model.EntityList{}, err
	}

	pg, err := page.Paginate(summaries, in.Cursor, in.Limit, maxResponseBytes(ctx),
		func(s model.EntitySummary) string { return string(s.ID) },
		entitySummaryByteSize,
	)
	if err != nil {
		return model.EntityList{}, err
	}

	return model.EntityList{
		Source:       "home_assistant_core",
		ObservedAt:   observedAt,
		Items:        pg.Items,
		NextCursor:   pg.NextCursor,
		Truncated:    pg.Truncated,
		LimitClamped: pg.LimitClamped,
	}, nil
}

// getEntity drills into one entity by id: its current state (masked per
// profile like list_entities') plus its device, area and integration
// metadata (doc §9). A missing id is ErrNotFound, not a
// partially-populated object (Appendix B: "gone between list and get").
func getEntity(ctx context.Context, registry entityRegistryReader, avail entityAvailabilityReader, states entityStateReader, profile policy.Profile, secrets []string, in GetEntityInput) (model.EntityDetail, error) {
	if in.ID == "" {
		return model.EntityDetail{}, fmt.Errorf("get_entity: id is required")
	}

	entities, observedAt, err := registry.Entities(ctx)
	if err != nil {
		return model.EntityDetail{}, err
	}
	var entity model.Entity
	found := false
	for _, e := range entities {
		if string(e.ID) == in.ID {
			entity = e
			found = true
			break
		}
	}
	if !found {
		return model.EntityDetail{}, fmt.Errorf("%w: entity %q", ha.ErrNotFound, in.ID)
	}

	stateValues, err := states.States(ctx)
	if err != nil {
		return model.EntityDetail{}, err
	}
	unavailable, err := avail.UnavailableEntityIDs(ctx)
	if err != nil {
		return model.EntityDetail{}, err
	}
	devices, _, err := registry.Devices(ctx)
	if err != nil {
		return model.EntityDetail{}, err
	}
	areas, _, err := registry.Areas(ctx)
	if err != nil {
		return model.EntityDetail{}, err
	}
	entries, _, err := registry.ConfigEntries(ctx)
	if err != nil {
		return model.EntityDetail{}, err
	}

	_, isUnavailable := unavailable[entity.ID]
	redactor := redact.New(profile, secrets...)
	summary := model.EntitySummary{
		Entity:    entity,
		State:     maskEntityState(redactor, entity, stateValues[entity.ID]),
		Available: !isUnavailable,
	}

	var deviceName string
	for _, d := range devices {
		if d.ID == entity.DeviceID {
			deviceName = d.Name
			break
		}
	}
	var areaName string
	for _, a := range areas {
		if a.ID == entity.AreaID {
			areaName = a.Name
			break
		}
	}
	var integrationDomain string
	for _, entry := range entries {
		if entry.ID == entity.ConfigEntryID {
			integrationDomain = entry.Domain
			break
		}
	}

	return model.EntityDetail{
		Source:            "home_assistant_core",
		ObservedAt:        observedAt,
		EntitySummary:     summary,
		AreaName:          areaName,
		DeviceName:        deviceName,
		IntegrationDomain: integrationDomain,
	}, nil
}

// entitySummaryByteSize approximates one entity summary's serialized size for
// the page package's byte cap — cheap enough to run per record without
// re-serializing the whole response afterward.
func entitySummaryByteSize(s model.EntitySummary) int64 {
	b, err := json.Marshal(s)
	if err != nil {
		return 0
	}
	return int64(len(b))
}
