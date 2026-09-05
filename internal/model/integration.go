package model

import "time"

// Integration is the normalized view of one Home Assistant config entry (one
// integration instance).
type Integration struct {
	ID     ConfigEntryID
	Domain string
	Title  string

	State      string
	Source     string
	Disabled   bool
	DisabledBy string
	Reason     string // set when State reports an error

	Provenance
}

// IntegrationSummary is one list_integrations/get_integration row: the
// config entry itself plus its entity, device and unavailable counts,
// computed server-side from the registries — the tool never returns the
// underlying entity or device lists themselves (P3-03 DoD).
type IntegrationSummary struct {
	Integration

	EntityCount         int
	DeviceCount         int
	UnavailableEntities int
}

// IntegrationList is list_integrations' page: config-entry summaries plus
// the cursor-pagination envelope every list_* tool shares (doc §9.1).
type IntegrationList struct {
	Source     string
	ObservedAt time.Time

	Items        []IntegrationSummary
	NextCursor   string
	Truncated    bool
	LimitClamped bool

	Provenance
}

// IntegrationDetail is get_integration's drill-down: one config entry's
// summary plus the response-level provenance every tool response carries
// (doc §9.1). An integration in a failed setup state is represented with
// its State and Reason, never omitted (P3-03 DoD).
type IntegrationDetail struct {
	Source     string
	ObservedAt time.Time

	IntegrationSummary
}
