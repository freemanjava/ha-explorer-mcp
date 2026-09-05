package model

import "time"

// SystemOverview is get_system_overview's root discovery snapshot: what the
// installation is and how big it is — never a per-entity list (P3-02 DoD).
// Inventory counts are computed server-side from the registries; the tool
// never returns the lists themselves (CLAUDE.md, Performance: aggregate
// before serializing).
type SystemOverview struct {
	Source     string
	ObservedAt time.Time

	CoreVersion  string
	LocationName string
	TimeZone     string
	CoreState    string

	Entities     int
	Devices      int
	Areas        int
	Integrations int

	// UnavailableEntities and UnknownEntities are get_states aggregated
	// in-process — the headline health this tool promises, without the
	// per-entity list find_unavailable_entities exists to return instead.
	UnavailableEntities int
	UnknownEntities     int

	Provenance
}

// CoreConfig is Core's own get_config, mapped: version, installation
// identity and run state. A malformed or partially missing payload degrades
// to Partial rather than aborting get_system_overview (CLAUDE.md, Error
// Handling).
type CoreConfig struct {
	Version      string
	LocationName string
	TimeZone     string
	State        string

	Provenance
}

// StateCounts is get_states aggregated in-process into the three numbers
// get_system_overview reports — never the underlying per-entity list.
type StateCounts struct {
	Total       int
	Unavailable int
	Unknown     int
}

// SystemHealth is get_system_health's Supervisor-backed resource and service
// health. It carries no Core CPU/RAM field by construction: the 2026-08-25
// Supervisor role decision grants no `*/stats` route for Core, so that figure
// is absent, never a fabricated zero (P3-02 DoD, CLAUDE.md rule 7).
//
// Unsupported is distinct from Provenance.Partial: Partial means one field
// among many could not be mapped, Unsupported means the whole tool has
// nothing to report because Supervisor itself could not be reached at the
// granted role (Reliability — Supervisor absent must not break Core-based
// diagnostics, which this tool does not touch).
type SystemHealth struct {
	Source     string
	ObservedAt time.Time

	Unsupported       bool
	UnsupportedReason string

	CoreState         string
	CoreVersion       string
	SupervisorVersion string
	OSVersion         string
	OSUpdateAvailable bool
	Hostname          string
	Machine           string
	Arch              string
	Supported         bool
	Healthy           bool

	// AddonCPUPercent and AddonMemoryPercent are this App's own container
	// resource use — never another App's or Core's (CLAUDE.md rule 5; the
	// manager role that would grant either is deliberately not requested).
	AddonCPUPercent    float64
	AddonMemoryPercent float64

	// Disk figures are Supervisor's own host disk view, in GB — Supervisor's
	// own unit (supervisor/api/host.py rounds to one decimal).
	DiskFreeGB  float64
	DiskTotalGB float64
	DiskUsedGB  float64

	ResolutionIssueCount  int
	ResolutionUnhealthy   []string
	ResolutionUnsupported []string

	Provenance
}

// CoreInfo is Supervisor's own /info, mapped: Supervisor's view of Core/OS/
// Docker versions, hostname, machine, arch and Core's run state. Granted
// whether or not hassio_api is set (api_bypass).
type CoreInfo struct {
	CoreVersion       string
	SupervisorVersion string
	OSVersion         string
	Hostname          string
	Machine           string
	Arch              string
	State             string
	Supported         bool
}

// OSInfo is Supervisor's own /os/info, mapped.
type OSInfo struct {
	Version         string
	UpdateAvailable bool
}

// HostDisk is the disk fields of Supervisor's /host/info, mapped — the rest
// of that payload is out of scope for get_system_health (P3-02).
type HostDisk struct {
	FreeGB  float64
	TotalGB float64
	UsedGB  float64
}

// ResolutionSummary is Supervisor's /resolution/info, mapped to counts and
// reason strings — never the full issue objects, which is more than a
// headline health figure needs.
type ResolutionSummary struct {
	IssueCount  int
	Unhealthy   []string
	Unsupported []string
}

// AddonStats is this App's own container resource use, from Supervisor's
// /addons/self/stats.
type AddonStats struct {
	CPUPercent    float64
	MemoryPercent float64
}
