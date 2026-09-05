package ha

import (
	"context"

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
