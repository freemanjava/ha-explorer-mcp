// Package model holds the normalized domain types every analysis and MCP
// layer builds on: Entity, DeviceRef, Integration, Area, Automation, Health
// and Evidence. It imports nothing from internal/ha — raw HA JSON shapes stay
// on the other side of the mapping boundary (CLAUDE.md, API & DTO Design).
package model

// EntityID is Home Assistant's "domain.object_id" entity identifier.
type EntityID string

// DeviceID is HA's device registry id. It is not a permanent physical-device
// identity: Core 2026.8 restricts an ordinary device to a single config entry
// (and at most one subentry), so a device can be split across an HA upgrade
// (architecture doc §8).
type DeviceID string

// ConfigEntryID is a Home Assistant config entry id (one integration
// instance).
type ConfigEntryID string

// AreaID is a Home Assistant area registry id.
type AreaID string
