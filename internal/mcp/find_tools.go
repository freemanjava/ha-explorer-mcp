package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/analysis"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/page"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// findFilterInput is the Appendix A.1 filter subset that scopes an
// installation-wide find_* scan to part of the inventory. Both
// FindUnavailableEntitiesInput and FindStaleEntitiesInput carry these same
// five fields so matchesEntityFilter (P3-05) can be reused unchanged — state
// and availability do not appear here because each find_* tool is itself an
// entity-state filter.
type findFilterInput struct {
	Domain      string
	Integration string
	DeviceID    string
	AreaID      string
	Search      string
}

// asListEntitiesFilter adapts the shared filter fields to
// matchesEntityFilter's ListEntitiesInput parameter, leaving State and
// Availability at their zero value so neither filter fires.
func (f findFilterInput) asListEntitiesFilter() ListEntitiesInput {
	return ListEntitiesInput{Domain: f.Domain, Integration: f.Integration, DeviceID: f.DeviceID, AreaID: f.AreaID, Search: f.Search}
}

// FindUnavailableEntitiesInput is find_unavailable_entities' typed input: the
// shared filter set plus cursor pagination (doc §9.1). No field accepts a
// route, command or query (rule 2).
type FindUnavailableEntitiesInput struct {
	Domain      string `json:"domain,omitempty" jsonschema:"filter to one entity domain, e.g. light"`
	Integration string `json:"integration,omitempty" jsonschema:"filter to entities owned by one integration domain, e.g. hue"`
	DeviceID    string `json:"device_id,omitempty" jsonschema:"filter to entities attached to one device"`
	AreaID      string `json:"area_id,omitempty" jsonschema:"filter to entities assigned to one area"`
	Search      string `json:"search,omitempty" jsonschema:"case-insensitive substring match against entity id or name"`

	Cursor string `json:"cursor,omitempty" jsonschema:"resume after this page's cursor"`
	Limit  int    `json:"limit,omitempty" jsonschema:"page size, default 50, max 200"`
}

func (in FindUnavailableEntitiesInput) filter() findFilterInput {
	return findFilterInput{Domain: in.Domain, Integration: in.Integration, DeviceID: in.DeviceID, AreaID: in.AreaID, Search: in.Search}
}

// FindStaleEntitiesInput is find_stale_entities' typed input: the shared
// filter set, a bounded lookback period like get_entity_statistics', and a
// cursor. Limit bounds candidate entities examined this call, not results
// returned — judging cadence costs one recorder read per entity (P4-03), so
// scanning the whole installation is inherently a multi-call operation.
type FindStaleEntitiesInput struct {
	Domain      string `json:"domain,omitempty" jsonschema:"filter to one entity domain, e.g. light"`
	Integration string `json:"integration,omitempty" jsonschema:"filter to entities owned by one integration domain, e.g. hue"`
	DeviceID    string `json:"device_id,omitempty" jsonschema:"filter to entities attached to one device"`
	AreaID      string `json:"area_id,omitempty" jsonschema:"filter to entities assigned to one area"`
	Search      string `json:"search,omitempty" jsonschema:"case-insensitive substring match against entity id or name"`
	// Period bounds the lookback window cadence is judged over, ending now.
	// Defaults to 7d like get_entity_statistics and is refused above
	// maxHistoryWindow.
	Period *string `json:"period,omitempty" jsonschema:"bounded lookback window ending now, e.g. \"7d\" or \"24h\" (default 7d, max 7d)"`

	Cursor string `json:"cursor,omitempty" jsonschema:"resume scanning after this cursor"`
	Limit  int    `json:"limit,omitempty" jsonschema:"how many candidate entities to examine this call, default 50, max 200 (not a result count)"`
}

func (in FindStaleEntitiesInput) filter() findFilterInput {
	return findFilterInput{Domain: in.Domain, Integration: in.Integration, DeviceID: in.DeviceID, AreaID: in.AreaID, Search: in.Search}
}

func (in FindStaleEntitiesInput) period() string {
	if in.Period == nil || *in.Period == "" {
		return defaultStatisticsPeriod
	}
	return *in.Period
}

// withFindTools returns tools with find_unavailable_entities and
// find_stale_entities' handlers bound, when opts supplies the readers each
// needs. A row whose readers are absent keeps its bindNotImplemented
// default.
func withFindTools(tools []Tool, opts Options) []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	for i := range out {
		switch out[i].Name {
		case "find_unavailable_entities":
			if opts.Inventory == nil || opts.Availability == nil {
				continue
			}
			out[i].bind = bindFindUnavailableEntities(opts.Inventory, opts.Availability, opts.Profile)
		case "find_stale_entities":
			if opts.Inventory == nil || opts.History == nil {
				continue
			}
			out[i].bind = bindFindStaleEntities(opts.Inventory, opts.History, opts.Profile)
		}
	}
	return out
}

// bindFindUnavailableEntities registers find_unavailable_entities' typed
// handler.
func bindFindUnavailableEntities(registry entityRegistryReader, avail entityAvailabilityReader, profile policy.Profile) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in FindUnavailableEntitiesInput) (*sdkmcp.CallToolResult, model.UnavailableEntityList, error) {
			out, err := findUnavailableEntities(ctx, registry, avail, profile, in)
			return nil, out, err
		})
	}
}

// bindFindStaleEntities registers find_stale_entities' typed handler.
func bindFindStaleEntities(registry entityRegistryReader, reader historyReader, profile policy.Profile) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in FindStaleEntitiesInput) (*sdkmcp.CallToolResult, model.StaleEntityList, error) {
			out, err := findStaleEntities(ctx, registry, reader, profile, in)
			return nil, out, err
		})
	}
}

// integrationDomainsOf resolves the config-entry-id -> integration-domain
// lookup matchesEntityFilter's Integration filter needs, shared by both
// find_* tools the same way entitySummaries/listIntegrations already build
// it.
func integrationDomainsOf(entries []model.Integration) map[model.ConfigEntryID]string {
	out := make(map[model.ConfigEntryID]string, len(entries))
	for _, entry := range entries {
		out[entry.ID] = entry.Domain
	}
	return out
}

// excludedByPrivacy reports whether e must be withheld outright under
// profile — the deny profile's decision for a PRIVATE entity — rather than
// merely masked. A find_* scan has no per-entity state value to mask in the
// first place (membership in "unavailable" or "stale" is the finding), so
// the only meaningful profile action here is inclusion or exclusion.
func excludedByPrivacy(e model.Entity, profile policy.Profile) bool {
	sensitivity := policy.ClassifyEntityWithClass(e.ID, e.DeviceClass)
	return profile.Decide(sensitivity) == policy.ActionDeny
}

// findUnavailableEntities scans the registry for entities currently
// unavailable or unknown, applies the shared filters, excludes what the
// privacy profile denies, and pages the (cheap, aggregate-only) result.
func findUnavailableEntities(ctx context.Context, registry entityRegistryReader, avail entityAvailabilityReader, profile policy.Profile, in FindUnavailableEntitiesInput) (model.UnavailableEntityList, error) {
	entities, observedAt, err := registry.Entities(ctx)
	if err != nil {
		return model.UnavailableEntityList{}, err
	}
	unavailable, err := avail.UnavailableEntityIDs(ctx)
	if err != nil {
		return model.UnavailableEntityList{}, err
	}
	entries, _, err := registry.ConfigEntries(ctx)
	if err != nil {
		return model.UnavailableEntityList{}, err
	}
	integrationDomains := integrationDomainsOf(entries)
	filter := in.filter().asListEntitiesFilter()

	items := make([]model.Entity, 0)
	privateExcluded := 0
	for _, e := range entities {
		if _, isUnavailable := unavailable[e.ID]; !isUnavailable {
			continue
		}
		if !matchesEntityFilter(e, "", unavailable, integrationDomains, filter) {
			continue
		}
		if excludedByPrivacy(e, profile) {
			privateExcluded++
			continue
		}
		items = append(items, e)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })

	pg, err := page.Paginate(items, in.Cursor, in.Limit, maxResponseBytes(ctx),
		func(e model.Entity) string { return string(e.ID) },
		entityByteSize,
	)
	if err != nil {
		return model.UnavailableEntityList{}, err
	}

	if budget, ok := policy.BudgetFrom(ctx); ok {
		// Two aggregate reads regardless of installation size: the entity
		// registry and the availability snapshot, neither charged per entity
		// (P4-02's UnavailableEntityIDs is already a single aggregate call).
		if err := budget.ChargeHARequests(2); err != nil {
			return model.UnavailableEntityList{}, err
		}
		if err := budget.ChargeEntities(len(pg.Items)); err != nil {
			return model.UnavailableEntityList{}, err
		}
	}

	out := model.UnavailableEntityList{
		Source:          "home_assistant_core",
		ObservedAt:      observedAt,
		Items:           pg.Items,
		NextCursor:      pg.NextCursor,
		Truncated:       pg.Truncated,
		LimitClamped:    pg.LimitClamped,
		PrivateExcluded: privateExcluded,
	}
	if budget, ok := policy.BudgetFrom(ctx); ok {
		if err := budget.ChargeBytes(unavailableEntityListByteSize(out)); err != nil {
			return model.UnavailableEntityList{}, err
		}
	}
	return out, nil
}

// findStaleEntities scans filtered candidate entities in deterministic id
// order, judging each one's cadence (P4-03) against a fresh recorder read,
// until the candidate window (Limit) or the invocation's HA-request budget
// is exhausted — whichever binds first. Truncated then means "more
// candidates remain unexamined", and NextCursor resumes exactly there: never
// an arbitrary subset presented as complete (P4-05 DoD).
func findStaleEntities(ctx context.Context, registry entityRegistryReader, reader historyReader, profile policy.Profile, in FindStaleEntitiesInput) (model.StaleEntityList, error) {
	periodStr := in.period()
	window, err := parseStatisticsPeriod(periodStr)
	if err != nil {
		return model.StaleEntityList{}, err
	}
	if window <= 0 {
		return model.StaleEntityList{}, fmt.Errorf("find_stale_entities: period %q must be positive", periodStr)
	}
	if window > maxHistoryWindow {
		return model.StaleEntityList{}, fmt.Errorf("%w: find_stale_entities: requested period %s exceeds the maximum %s",
			policy.ErrPolicyDenied, window, maxHistoryWindow)
	}

	entities, observedAt, err := registry.Entities(ctx)
	if err != nil {
		return model.StaleEntityList{}, err
	}
	entries, _, err := registry.ConfigEntries(ctx)
	if err != nil {
		return model.StaleEntityList{}, err
	}
	integrationDomains := integrationDomainsOf(entries)
	filter := in.filter().asListEntitiesFilter()

	candidates := make([]model.Entity, 0, len(entities))
	for _, e := range entities {
		if matchesEntityFilter(e, "", nil, integrationDomains, filter) {
			candidates = append(candidates, e)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })

	startKey, err := page.DecodeCursor(in.Cursor)
	if err != nil {
		return model.StaleEntityList{}, err
	}
	start := 0
	if startKey != "" {
		start = sort.Search(len(candidates), func(i int) bool { return string(candidates[i].ID) > startKey })
	}
	resolvedLimit, limitClamped := page.ResolveLimit(in.Limit)

	to := time.Now().UTC()
	from := to.Add(-window)
	budget, hasBudget := policy.BudgetFrom(ctx)

	var items []model.StaleEntity
	scanned := 0
	privateExcluded := 0
	lastKey := startKey
	i := start
	for ; i < len(candidates) && scanned < resolvedLimit; i++ {
		e := candidates[i]

		if excludedByPrivacy(e, profile) {
			privateExcluded++
			scanned++
			lastKey = string(e.ID)
			continue
		}

		if hasBudget {
			usage, limits := budget.Usage(), budget.Limits()
			if usage.HARequests >= limits.MaxHARequests {
				break // no upstream call issued: nothing to discard, only to resume from
			}
			if err := budget.Preflight(policy.SourceHistory, 1, window); err != nil {
				break
			}
		}

		points, err := reader.History(ctx, e.ID, from, to, true)
		if err != nil {
			return model.StaleEntityList{}, err
		}
		scanned++
		lastKey = string(e.ID)

		if hasBudget {
			if err := budget.ChargeHARequests(1); err != nil {
				return model.StaleEntityList{}, err
			}
			if err := budget.ChargeHistoryPoints(len(points)); err != nil {
				return model.StaleEntityList{}, err
			}
			if err := budget.ChargeEntities(1); err != nil {
				return model.StaleEntityList{}, err
			}
		}

		cadence, err := analysis.ComputeCadence(from, to, points)
		if err != nil {
			return model.StaleEntityList{}, err
		}
		if cadence.StaleJudgeable && cadence.Stale {
			items = append(items, model.StaleEntity{
				Entity:               e,
				LastUpdate:           cadence.LastUpdate,
				SilentFor:            cadence.SilentFor,
				MedianUpdateInterval: cadence.MedianUpdateInterval,
				P95UpdateInterval:    cadence.P95UpdateInterval,
				StaleThreshold:       cadence.StaleThreshold,
				StalenessRatio:       cadence.StalenessRatio,
			})
		}
	}

	truncated := i < len(candidates)
	var nextCursor string
	if truncated {
		nextCursor = page.EncodeCursor(lastKey)
	}

	// Rank by StalenessRatio descending (analysis.CadenceReport's own
	// purpose for the field): the worst offenders first, id as a
	// deterministic tiebreak.
	sort.Slice(items, func(a, b int) bool {
		if items[a].StalenessRatio != items[b].StalenessRatio {
			return items[a].StalenessRatio > items[b].StalenessRatio
		}
		return items[a].ID < items[b].ID
	})

	out := model.StaleEntityList{
		Source:          "recorder_history",
		ObservedAt:      observedAt,
		Period:          window,
		Items:           items,
		NextCursor:      nextCursor,
		Truncated:       truncated,
		LimitClamped:    limitClamped,
		Scanned:         scanned,
		PrivateExcluded: privateExcluded,
	}
	if hasBudget {
		if err := budget.ChargeBytes(staleEntityListByteSize(out)); err != nil {
			return model.StaleEntityList{}, err
		}
	}
	return out, nil
}

func entityByteSize(e model.Entity) int64 {
	b, err := json.Marshal(e)
	if err != nil {
		return 0
	}
	return int64(len(b))
}

func unavailableEntityListByteSize(l model.UnavailableEntityList) int64 {
	b, err := json.Marshal(l)
	if err != nil {
		return 0
	}
	return int64(len(b))
}

func staleEntityListByteSize(l model.StaleEntityList) int64 {
	b, err := json.Marshal(l)
	if err != nil {
		return 0
	}
	return int64(len(b))
}
