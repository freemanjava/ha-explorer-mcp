package model

import "time"

// DeviceRef is the normalized registry view of one Home Assistant device.
// HADeviceID is not treated as a permanent physical-device identity — see
// DeviceID's doc comment (architecture doc §8).
type DeviceRef struct {
	ID            DeviceID
	ConfigEntryID ConfigEntryID
	Platform      string

	Name         string
	AreaID       AreaID
	Manufacturer string
	Model        string
	SWVersion    string
	HWVersion    string

	// SerialNumber, Connections and Identifiers carry privacy-relevant
	// external identity metadata (MAC addresses, integration-scoped ids) —
	// they pass through unclassified here; classification and masking is
	// internal/policy and internal/redact's job (Phase 02), not this
	// mapping's (CLAUDE.md rule 6, research doc finding 7).
	SerialNumber string
	Connections  [][2]string
	Identifiers  [][2]string

	ViaDeviceID DeviceID
	DisabledBy  string

	Provenance
}

// DeviceList is list_devices' page: device registry entries plus the
// cursor-pagination envelope every list_* tool shares (doc §9.1).
type DeviceList struct {
	Source     string
	ObservedAt time.Time

	Items        []DeviceRef
	NextCursor   string
	Truncated    bool
	LimitClamped bool

	Provenance
}

// DeviceEntityRef is one entity attached to a device, as seen from
// get_device: enough to identify it and gauge its availability, not the full
// Entity/current-state surface list_entities/get_entity own (P3-05).
type DeviceEntityRef struct {
	ID        EntityID
	Domain    string
	Name      string
	Available bool
}

// DeviceDetail is get_device's drill-down: the device itself, its related
// entities, and its via/parent topology. ViaDevice is nil when the device
// carries no ViaDeviceID or that device is no longer present in the
// registry — a dangling reference degrades the topology, it never fails the
// whole response.
type DeviceDetail struct {
	Source     string
	ObservedAt time.Time

	Device          DeviceRef
	RelatedEntities []DeviceEntityRef
	ViaDevice       *DeviceRef
	ChildDevices    []DeviceRef
}
