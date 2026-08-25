package model

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
