package ha

import (
	"context"
	"encoding/json"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// Registry TTLs, doc §16 "Caching, Resilience and Performance". Entity
// registry churns fastest — renames, entities appearing as integrations
// reload — so it sits at the bottom of its documented 30-60s range; device
// and area registries change on hardware pairing or manual reorganization,
// rarely inside one session, so they sit at the top of "~5 min"; config
// entries move only on integration setup/removal and get the middle of their
// 1-5min range.
const (
	entityRegistryTTL = 30 * time.Second
	deviceRegistryTTL = 5 * time.Minute
	areaRegistryTTL   = 5 * time.Minute
	configEntriesTTL  = 3 * time.Minute
)

// caller is the subset of *Manager a RegistryCache needs. Kept narrow
// (CLAUDE.md, Design Principles — interface segregation) so a test can supply
// a fetch function without standing up a real WebSocket connection.
type caller interface {
	Call(ctx context.Context, cmd Command) (json.RawMessage, error)
}

// RegistryCache serves Home Assistant's slow-moving registries — entity,
// device, area, config entries — each with its own TTL and single-flight
// refill, so N concurrent readers past expiry cause one upstream fetch, not N
// (CLAUDE.md, Concurrency). Every served value carries the time it was
// observed: a cache is a load-control mechanism, not a source of truth (doc
// §16), and a caller judges freshness for itself rather than trusting it
// silently.
//
// The zero value is not usable; construct with NewRegistryCache.
type RegistryCache struct {
	call caller

	entities      *cachedValue[[]model.Entity]
	devices       *cachedValue[[]model.DeviceRef]
	areas         *cachedValue[[]model.Area]
	configEntries *cachedValue[[]model.Integration]
}

// NewRegistryCache returns a cache that fetches through call.
func NewRegistryCache(call caller) *RegistryCache {
	return &RegistryCache{
		call:          call,
		entities:      newCachedValue[[]model.Entity](entityRegistryTTL),
		devices:       newCachedValue[[]model.DeviceRef](deviceRegistryTTL),
		areas:         newCachedValue[[]model.Area](areaRegistryTTL),
		configEntries: newCachedValue[[]model.Integration](configEntriesTTL),
	}
}

// Entities returns the entity registry, refetching if the cached copy has
// expired. observedAt is when the returned slice was fetched from HA, not
// when this call returned it.
func (c *RegistryCache) Entities(ctx context.Context) ([]model.Entity, time.Time, error) {
	return c.entities.Get(ctx, func(ctx context.Context) ([]model.Entity, error) {
		raw, err := c.call.Call(ctx, BareCommand(CommandEntityRegistryList))
		if err != nil {
			return nil, err
		}
		return MapEntityRegistryList(raw)
	})
}

// Devices returns the device registry, refetching if the cached copy has
// expired.
func (c *RegistryCache) Devices(ctx context.Context) ([]model.DeviceRef, time.Time, error) {
	return c.devices.Get(ctx, func(ctx context.Context) ([]model.DeviceRef, error) {
		raw, err := c.call.Call(ctx, BareCommand(CommandDeviceRegistryList))
		if err != nil {
			return nil, err
		}
		return MapDeviceRegistryList(raw)
	})
}

// Areas returns the area registry, refetching if the cached copy has expired.
func (c *RegistryCache) Areas(ctx context.Context) ([]model.Area, time.Time, error) {
	return c.areas.Get(ctx, func(ctx context.Context) ([]model.Area, error) {
		raw, err := c.call.Call(ctx, BareCommand(CommandAreaRegistryList))
		if err != nil {
			return nil, err
		}
		return MapAreaRegistryList(raw)
	})
}

// ConfigEntries returns the integration config entries, refetching if the
// cached copy has expired.
func (c *RegistryCache) ConfigEntries(ctx context.Context) ([]model.Integration, time.Time, error) {
	return c.configEntries.Get(ctx, func(ctx context.Context) ([]model.Integration, error) {
		raw, err := c.call.Call(ctx, BareCommand(CommandConfigEntriesGet))
		if err != nil {
			return nil, err
		}
		return MapConfigEntriesGet(raw)
	})
}
