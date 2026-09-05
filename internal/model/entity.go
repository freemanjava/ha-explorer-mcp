package model

import "time"

// Entity is the normalized registry view of one Home Assistant entity. It is
// built by explicit mapping in internal/ha from a config/entity_registry
// payload; no HA JSON shape crosses into this package (CLAUDE.md, API & DTO
// Design).
type Entity struct {
	ID       EntityID
	Domain   string // derived from ID's "domain.object_id" prefix
	UniqueID string
	Platform string

	DeviceID      DeviceID
	AreaID        AreaID
	ConfigEntryID ConfigEntryID

	Name           string
	OriginalName   string
	Icon           string
	OriginalIcon   string
	EntityCategory string
	DeviceClass    string

	DisabledBy     string
	HiddenBy       string
	HasEntityName  bool
	TranslationKey string
	Labels         []string

	CreatedAt  time.Time
	ModifiedAt time.Time

	Provenance
}

// EntitySummary is one list_entities/get_entity row: the registry entry plus
// its current state, already redacted per the Phase 02 profile (P3-05) — a
// PRIVATE entity's State is a masked token or "[denied]", never the raw
// value, whatever profile is configured.
type EntitySummary struct {
	Entity

	State     string
	Available bool
}

// EntityList is list_entities' page: entity summaries plus the
// cursor-pagination envelope every list_* tool shares (doc §9.1).
type EntityList struct {
	Source     string
	ObservedAt time.Time

	Items        []EntitySummary
	NextCursor   string
	Truncated    bool
	LimitClamped bool

	Provenance
}

// EntityDetail is get_entity's drill-down: the entity's summary plus the
// device/area/integration metadata doc §9's catalog names ("current state +
// entity registry + device/area metadata"). A dangling reference (device,
// area or config entry gone from its registry) degrades the corresponding
// name to empty rather than failing the response.
type EntityDetail struct {
	Source     string
	ObservedAt time.Time

	EntitySummary

	AreaName          string
	DeviceName        string
	IntegrationDomain string
}
