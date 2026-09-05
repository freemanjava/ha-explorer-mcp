package mcp

import (
	"context"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// EmptyInput is the input type for a tool that takes no arguments. Its
// inferred schema has no properties, so a client cannot smuggle a route,
// command or query parameter into a tool that was never meant to take one
// (CLAUDE.md rule 2); binders that use it also pin def.InputSchema to
// emptyObjectSchema explicitly, so the guarantee does not depend on
// inference alone.
type EmptyInput struct{}

// systemCoreReader is get_system_overview's read surface into Core: identity
// (get_config) and a live-state summary (get_states), already aggregated —
// the tool never sees, and so cannot return, the underlying per-entity list
// (P3-02 DoD).
type systemCoreReader interface {
	CoreConfig(ctx context.Context) (model.CoreConfig, error)
	StateCounts(ctx context.Context) (model.StateCounts, error)
}

// systemInventoryReader is get_system_overview's read surface into the slow-
// moving registries: only their counts leave the boundary.
type systemInventoryReader interface {
	Entities(ctx context.Context) ([]model.Entity, time.Time, error)
	Devices(ctx context.Context) ([]model.DeviceRef, time.Time, error)
	Areas(ctx context.Context) ([]model.Area, time.Time, error)
	ConfigEntries(ctx context.Context) ([]model.Integration, time.Time, error)
}

// systemHealthReader is get_system_health's read surface into Supervisor.
// Every method degrades the same way: an error means Supervisor could not be
// reached at the granted role, and the tool answers unsupported rather than
// failing (Reliability — Supervisor absent must not break Core-based
// diagnostics, which this tool does not touch).
type systemHealthReader interface {
	CoreInfo(ctx context.Context) (model.CoreInfo, error)
	SupervisorInfo(ctx context.Context) (model.SupervisorInfo, error)
	OSHealth(ctx context.Context) (model.OSInfo, error)
	HostDisk(ctx context.Context) (model.HostDisk, error)
	ResolutionSummary(ctx context.Context) (model.ResolutionSummary, error)
	SelfStats(ctx context.Context) (model.AddonStats, error)
}

// withSystemTools returns tools with get_system_overview and
// get_system_health's handlers bound, for whichever of the two opts supplies
// readers for. A table that does not carry those rows at all (a test's
// probeTable) is returned unchanged; a row whose readers are absent from opts
// keeps its bindNotImplemented default, so a build wired without Core access
// still answers honestly rather than serving a tool that would panic on a nil
// reader.
func withSystemTools(tools []Tool, opts Options) []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	for i := range out {
		switch out[i].Name {
		case "get_system_overview":
			if opts.Core != nil && opts.Inventory != nil {
				out[i].bind = bindSystemOverview(opts.Core, opts.Inventory)
			}
		case "get_system_health":
			if opts.Supervisor != nil {
				out[i].bind = bindSystemHealth(opts.Supervisor)
			}
		}
	}
	return out
}

// bindSystemOverview registers get_system_overview's typed handler.
func bindSystemOverview(core systemCoreReader, inventory systemInventoryReader) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		def.InputSchema = emptyObjectSchema
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, _ EmptyInput) (*sdkmcp.CallToolResult, model.SystemOverview, error) {
			out, err := systemOverview(ctx, core, inventory)
			return nil, out, err
		})
	}
}

// systemOverview builds get_system_overview's response: version and
// installation identity from get_config, inventory counts from the
// registries, and a headline availability count from get_states — counts
// only, never the lists themselves (DoD: "overview returns counts without
// dumping entities").
func systemOverview(ctx context.Context, core systemCoreReader, inventory systemInventoryReader) (model.SystemOverview, error) {
	cfg, err := core.CoreConfig(ctx)
	if err != nil {
		return model.SystemOverview{}, err
	}
	counts, err := core.StateCounts(ctx)
	if err != nil {
		return model.SystemOverview{}, err
	}
	entities, _, err := inventory.Entities(ctx)
	if err != nil {
		return model.SystemOverview{}, err
	}
	devices, _, err := inventory.Devices(ctx)
	if err != nil {
		return model.SystemOverview{}, err
	}
	areas, _, err := inventory.Areas(ctx)
	if err != nil {
		return model.SystemOverview{}, err
	}
	integrations, _, err := inventory.ConfigEntries(ctx)
	if err != nil {
		return model.SystemOverview{}, err
	}

	out := model.SystemOverview{
		Source:              "home_assistant_core",
		ObservedAt:          time.Now().UTC(),
		CoreVersion:         cfg.Version,
		LocationName:        cfg.LocationName,
		TimeZone:            cfg.TimeZone,
		CoreState:           cfg.State,
		Entities:            len(entities),
		Devices:             len(devices),
		Areas:               len(areas),
		Integrations:        len(integrations),
		UnavailableEntities: counts.Unavailable,
		UnknownEntities:     counts.Unknown,
	}
	out.Partial = cfg.Partial
	out.PartialReason = cfg.PartialReason
	return out, nil
}

// bindSystemHealth registers get_system_health's typed handler.
func bindSystemHealth(supervisor systemHealthReader) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		def.InputSchema = emptyObjectSchema
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, _ EmptyInput) (*sdkmcp.CallToolResult, model.SystemHealth, error) {
			return nil, systemHealth(ctx, supervisor), nil
		})
	}
}

// systemHealth builds get_system_health's response. It never returns a Go
// error for a Supervisor read failing: that is an expected, nameable state
// (rule 7), reported as Unsupported with a reason rather than an MCP tool
// error the client would treat as this build having broken.
//
// Any one Supervisor endpoint failing degrades the whole response to
// unsupported, rather than serving a partially-filled health report an agent
// could mistake for the complete picture: Supervisor at the granted role
// answers all of these routes or none of them in every case this project has
// observed, so partial success here is itself the more surprising outcome
// and the one worth surfacing plainly rather than papering over.
func systemHealth(ctx context.Context, supervisor systemHealthReader) model.SystemHealth {
	out := model.SystemHealth{Source: "home_assistant_supervisor", ObservedAt: time.Now().UTC()}

	info, err := supervisor.CoreInfo(ctx)
	if markUnsupported(&out, err) {
		return out
	}
	out.CoreVersion = info.CoreVersion
	out.SupervisorVersion = info.SupervisorVersion
	out.OSVersion = info.OSVersion
	out.Hostname = info.Hostname
	out.Machine = info.Machine
	out.Arch = info.Arch
	out.CoreState = info.State
	out.Supported = info.Supported

	sup, err := supervisor.SupervisorInfo(ctx)
	if markUnsupported(&out, err) {
		return out
	}
	out.Healthy = sup.Healthy
	out.Supported = out.Supported && sup.Supported

	os, err := supervisor.OSHealth(ctx)
	if markUnsupported(&out, err) {
		return out
	}
	if os.Version != "" {
		out.OSVersion = os.Version
	}
	out.OSUpdateAvailable = os.UpdateAvailable

	disk, err := supervisor.HostDisk(ctx)
	if markUnsupported(&out, err) {
		return out
	}
	out.DiskFreeGB, out.DiskTotalGB, out.DiskUsedGB = disk.FreeGB, disk.TotalGB, disk.UsedGB

	res, err := supervisor.ResolutionSummary(ctx)
	if markUnsupported(&out, err) {
		return out
	}
	out.ResolutionIssueCount = res.IssueCount
	out.ResolutionUnhealthy = res.Unhealthy
	out.ResolutionUnsupported = res.Unsupported

	stats, err := supervisor.SelfStats(ctx)
	if markUnsupported(&out, err) {
		return out
	}
	out.AddonCPUPercent = stats.CPUPercent
	out.AddonMemoryPercent = stats.MemoryPercent

	return out
}

// markUnsupported records err on out and reports whether the caller should
// stop. Home Assistant's own errors here never carry SUPERVISOR_TOKEN (rule
// 4) or a raw payload, so err.Error() is safe to surface as the reason
// directly.
func markUnsupported(out *model.SystemHealth, err error) bool {
	if err == nil {
		return false
	}
	out.Unsupported = true
	out.UnsupportedReason = err.Error()
	return true
}
