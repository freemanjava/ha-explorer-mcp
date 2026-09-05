package ha

import (
	"context"
	"strings"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// CoreReader reads Home Assistant Core's own identity and live-state summary
// — get_config and get_states — the two commands get_system_overview needs
// beyond the slow-moving registries RegistryCache already serves. Neither is
// cached: get_config changes only across a Core restart and get_states
// changes continuously, so a TTL would either be pointless or stale by
// construction; both are a few KB over an already-open connection, cheap
// enough to fetch per call.
//
// The zero value is not usable; construct with NewCoreReader.
type CoreReader struct {
	call caller
}

// NewCoreReader returns a reader that fetches through call.
func NewCoreReader(call caller) *CoreReader {
	return &CoreReader{call: call}
}

// CoreConfig returns Core's own get_config, mapped.
func (r *CoreReader) CoreConfig(ctx context.Context) (model.CoreConfig, error) {
	raw, err := r.call.Call(ctx, BareCommand(CommandGetConfig))
	if err != nil {
		return model.CoreConfig{}, err
	}
	return MapCoreConfig(raw)
}

// StateCounts returns get_states aggregated in-process into
// model.StateCounts — the caller never sees the underlying per-entity list.
func (r *CoreReader) StateCounts(ctx context.Context) (model.StateCounts, error) {
	raw, err := r.call.Call(ctx, BareCommand(CommandGetStates))
	if err != nil {
		return model.StateCounts{}, err
	}
	return MapStateCounts(raw)
}

// UnavailableEntityIDs returns get_states aggregated in-process into the set
// of entity ids currently unavailable or unknown — for list_integrations and
// get_integration's per-integration counts (P3-03), never the underlying
// per-entity state list.
func (r *CoreReader) UnavailableEntityIDs(ctx context.Context) (map[model.EntityID]struct{}, error) {
	raw, err := r.call.Call(ctx, BareCommand(CommandGetStates))
	if err != nil {
		return nil, err
	}
	return MapUnavailableEntityIDs(raw)
}

// Automations returns get_states filtered and mapped to one summary per
// automation-domain entity, for list_automations (P3-06) — the confirmed
// non-admin fallback (enabled state and last_triggered), not
// automation/config's admin-gated detail get_automation (P3-07) reaches
// instead.
func (r *CoreReader) Automations(ctx context.Context) ([]model.AutomationSummary, error) {
	raw, err := r.call.Call(ctx, BareCommand(CommandGetStates))
	if err != nil {
		return nil, err
	}
	return MapAutomationStates(raw)
}

// Repairs returns repairs/list_issues mapped to one Repair per issue, for
// list_repairs (P3-06) — reachable at any principal, admin or not
// (docs/research/2026-09-05-ha-repairs-api.md).
func (r *CoreReader) Repairs(ctx context.Context) ([]model.Repair, error) {
	raw, err := r.call.Call(ctx, BareCommand(CommandRepairsListIssues))
	if err != nil {
		return nil, err
	}
	return MapRepairs(raw)
}

// AutomationDetail returns automation/config for one entity, mapped —
// get_automation's admin-gated detail source (P3-07), distinct from
// Automations' get_states fallback every principal can read. A non-admin
// principal or an HA version without the command answers with a
// *CommandError the caller classifies (P3-07 DoD); AutomationDetail itself
// makes no such judgment.
func (r *CoreReader) AutomationDetail(ctx context.Context, entityID model.EntityID) (model.Automation, error) {
	raw, err := r.call.Call(ctx, automationConfigCommand{EntityID: string(entityID)})
	if err != nil {
		return model.Automation{}, err
	}
	return MapAutomationConfigResult(entityID, raw)
}

// AutomationTraces returns trace/list for one automation, mapped —
// get_automation_traces' admin-gated evidence source (P3-07). ItemID is
// derived from entityID's own object id, not a caller-supplied value, so it
// can never diverge from the entity the caller asked about.
func (r *CoreReader) AutomationTraces(ctx context.Context, entityID model.EntityID) ([]model.AutomationTraceSummary, error) {
	domain, itemID := splitEntityID(string(entityID))
	raw, err := r.call.Call(ctx, traceListCommand{Domain: domain, ItemID: itemID})
	if err != nil {
		return nil, err
	}
	return MapAutomationTraces(raw)
}

// LogbookEvents returns logbook/get_events for one entity since start,
// mapped — get_automation_traces' non-admin fallback evidence (F-11),
// reachable at any principal.
func (r *CoreReader) LogbookEvents(ctx context.Context, entityID model.EntityID, since time.Time) ([]model.LogbookEvent, error) {
	raw, err := r.call.Call(ctx, logbookGetEventsCommand{StartTime: since, EntityIDs: []string{string(entityID)}})
	if err != nil {
		return nil, err
	}
	return MapLogbookEvents(raw)
}

// splitEntityID separates an entity id into its domain and object id, the
// shape trace/list's domain/item_id arguments need. A malformed id (no dot)
// returns the whole string as both, which fails the same way an invalid
// entity id fails anywhere else in this package: HA rejects the request
// rather than this code fabricating a plausible-looking split.
func splitEntityID(entityID string) (domain, objectID string) {
	if i := strings.IndexByte(entityID, '.'); i > 0 {
		return entityID[:i], entityID[i+1:]
	}
	return entityID, entityID
}

// States returns get_states mapped to each entity's current state string,
// keyed by id — for list_entities and get_entity (P3-05), whose job is
// reporting individual current state rather than an aggregate.
func (r *CoreReader) States(ctx context.Context) (map[model.EntityID]string, error) {
	raw, err := r.call.Call(ctx, BareCommand(CommandGetStates))
	if err != nil {
		return nil, err
	}
	return MapEntityStateValues(raw)
}
