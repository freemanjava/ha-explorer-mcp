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
