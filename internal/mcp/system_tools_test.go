package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// fakeCoreReader is a systemCoreReader test double: no WebSocket, no HA.
type fakeCoreReader struct {
	cfg       model.CoreConfig
	cfgErr    error
	counts    model.StateCounts
	countsErr error
}

func (f *fakeCoreReader) CoreConfig(context.Context) (model.CoreConfig, error) {
	return f.cfg, f.cfgErr
}
func (f *fakeCoreReader) StateCounts(context.Context) (model.StateCounts, error) {
	return f.counts, f.countsErr
}

// fakeInventoryReader is a systemInventoryReader test double.
type fakeInventoryReader struct {
	entities     []model.Entity
	devices      []model.DeviceRef
	areas        []model.Area
	integrations []model.Integration
}

func (f *fakeInventoryReader) Entities(context.Context) ([]model.Entity, time.Time, error) {
	return f.entities, time.Time{}, nil
}
func (f *fakeInventoryReader) Devices(context.Context) ([]model.DeviceRef, time.Time, error) {
	return f.devices, time.Time{}, nil
}
func (f *fakeInventoryReader) Areas(context.Context) ([]model.Area, time.Time, error) {
	return f.areas, time.Time{}, nil
}
func (f *fakeInventoryReader) ConfigEntries(context.Context) ([]model.Integration, time.Time, error) {
	return f.integrations, time.Time{}, nil
}

// fakeSupervisorReader is a systemHealthReader test double. Each field's
// error, when set, is what that one endpoint "fails" with; a zero error means
// it "answers" with the paired value.
type fakeSupervisorReader struct {
	coreInfo    model.CoreInfo
	coreInfoErr error

	supervisorInfo    model.SupervisorInfo
	supervisorInfoErr error

	osInfo    model.OSInfo
	osInfoErr error

	hostDisk    model.HostDisk
	hostDiskErr error

	resolution    model.ResolutionSummary
	resolutionErr error

	addonStats    model.AddonStats
	addonStatsErr error
}

func (f *fakeSupervisorReader) CoreInfo(context.Context) (model.CoreInfo, error) {
	return f.coreInfo, f.coreInfoErr
}
func (f *fakeSupervisorReader) SupervisorInfo(context.Context) (model.SupervisorInfo, error) {
	return f.supervisorInfo, f.supervisorInfoErr
}
func (f *fakeSupervisorReader) OSHealth(context.Context) (model.OSInfo, error) {
	return f.osInfo, f.osInfoErr
}
func (f *fakeSupervisorReader) HostDisk(context.Context) (model.HostDisk, error) {
	return f.hostDisk, f.hostDiskErr
}
func (f *fakeSupervisorReader) ResolutionSummary(context.Context) (model.ResolutionSummary, error) {
	return f.resolution, f.resolutionErr
}
func (f *fakeSupervisorReader) SelfStats(context.Context) (model.AddonStats, error) {
	return f.addonStats, f.addonStatsErr
}

// systemOptions is testOptions wired with the given readers.
func systemOptions(core systemCoreReader, inventory systemInventoryReader, supervisor systemHealthReader) Options {
	opts := testOptions()
	opts.Core = core
	opts.Inventory = inventory
	opts.Supervisor = supervisor
	return opts
}

// callStructured invokes name and decodes its StructuredContent into out.
func callStructured(t *testing.T, client *sdkmcp.ClientSession, name string, out any) *sdkmcp.CallToolResult {
	t.Helper()
	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: name})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	if res.IsError {
		t.Fatalf("CallTool(%s) returned a tool error: %+v", name, res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
	return res
}

// TestSystemOverview_ReturnsCountsWithoutEntityList is the P3-02 DoD's first
// line: counts, computed server-side, never the per-entity list
// find_unavailable_entities exists to return instead.
func TestSystemOverview_ReturnsCountsWithoutEntityList(t *testing.T) {
	core := &fakeCoreReader{
		cfg:    model.CoreConfig{Version: "2026.8.3", LocationName: "Home", TimeZone: "UTC", State: "RUNNING"},
		counts: model.StateCounts{Total: 3, Unavailable: 1, Unknown: 0},
	}
	inventory := &fakeInventoryReader{
		entities:     []model.Entity{{ID: "light.kitchen"}, {ID: "light.hallway"}, {ID: "sensor.attic_temp"}},
		devices:      []model.DeviceRef{{ID: "dev-1"}, {ID: "dev-2"}},
		areas:        []model.Area{{ID: "area-1"}},
		integrations: []model.Integration{{ID: "entry-1"}},
	}

	client := connect(t, newServer(systemOptions(core, inventory, nil), Catalog()))

	var overview model.SystemOverview
	res := callStructured(t, client, "get_system_overview", &overview)

	if overview.Entities != 3 || overview.Devices != 2 || overview.Areas != 1 || overview.Integrations != 1 {
		t.Fatalf("overview counts = %+v, want 3/2/1/1", overview)
	}
	if overview.UnavailableEntities != 1 {
		t.Errorf("UnavailableEntities = %d, want 1", overview.UnavailableEntities)
	}
	if overview.CoreVersion != "2026.8.3" {
		t.Errorf("CoreVersion = %q, want 2026.8.3", overview.CoreVersion)
	}

	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, id := range []string{"light.kitchen", "light.hallway", "sensor.attic_temp", "dev-1", "dev-2"} {
		if strings.Contains(string(raw), id) {
			t.Errorf("overview response contains %q — it must report counts only, never the per-entity list", id)
		}
	}
}

// TestSystemHealth_SupervisorUnreachable_DegradesToUnsupported_OverviewStillSucceeds
// is the DoD's Appendix B line: Supervisor absent while Core is available
// degrades get_system_health without breaking get_system_overview, which does
// not touch Supervisor at all.
func TestSystemHealth_SupervisorUnreachable_DegradesToUnsupported_OverviewStillSucceeds(t *testing.T) {
	core := &fakeCoreReader{cfg: model.CoreConfig{Version: "2026.8.3"}}
	inventory := &fakeInventoryReader{}
	supervisor := &fakeSupervisorReader{
		coreInfoErr: fmt.Errorf("%w: Supervisor unreachable: GET /info", ha.ErrUnsupported),
	}

	client := connect(t, newServer(systemOptions(core, inventory, supervisor), Catalog()))

	var health model.SystemHealth
	callStructured(t, client, "get_system_health", &health)
	if !health.Unsupported {
		t.Fatal("get_system_health did not degrade to unsupported when Supervisor was unreachable")
	}
	if health.UnsupportedReason == "" {
		t.Error("Unsupported is true but UnsupportedReason is empty")
	}

	var overview model.SystemOverview
	callStructured(t, client, "get_system_overview", &overview)
	if overview.CoreVersion != "2026.8.3" {
		t.Errorf("get_system_overview did not succeed independently of Supervisor: %+v", overview)
	}
}

// TestSystemHealth_AllSupervisorEndpointsReachable_PopulatesHealth is the
// happy path: every field the tool promises is filled from its Supervisor
// source.
func TestSystemHealth_AllSupervisorEndpointsReachable_PopulatesHealth(t *testing.T) {
	supervisor := &fakeSupervisorReader{
		coreInfo:       model.CoreInfo{CoreVersion: "2026.8.3", SupervisorVersion: "2026.08.0", Hostname: "homeassistant", Machine: "rpi4", Arch: "aarch64", State: "running", Supported: true},
		supervisorInfo: model.SupervisorInfo{Healthy: true, Supported: true},
		osInfo:         model.OSInfo{Version: "14.2", UpdateAvailable: false},
		hostDisk:       model.HostDisk{FreeGB: 10, TotalGB: 32, UsedGB: 22},
		resolution:     model.ResolutionSummary{IssueCount: 1, Unhealthy: []string{"privileged"}},
		addonStats:     model.AddonStats{CPUPercent: 1.5, MemoryPercent: 4.2},
	}

	client := connect(t, newServer(systemOptions(&fakeCoreReader{}, &fakeInventoryReader{}, supervisor), Catalog()))

	var health model.SystemHealth
	callStructured(t, client, "get_system_health", &health)

	if health.Unsupported {
		t.Fatalf("health unexpectedly unsupported: %s", health.UnsupportedReason)
	}
	if health.CoreVersion != "2026.8.3" || health.SupervisorVersion != "2026.08.0" || health.Hostname != "homeassistant" {
		t.Errorf("component identity not populated: %+v", health)
	}
	if health.DiskFreeGB != 10 || health.DiskTotalGB != 32 || health.DiskUsedGB != 22 {
		t.Errorf("disk figures not populated: %+v", health)
	}
	if health.ResolutionIssueCount != 1 || len(health.ResolutionUnhealthy) != 1 {
		t.Errorf("resolution summary not populated: %+v", health)
	}
	if health.AddonCPUPercent != 1.5 || health.AddonMemoryPercent != 4.2 {
		t.Errorf("this App's own resource use not populated: %+v", health)
	}
}

// TestSystemHealth_NeverClaimsCoreCPUOrRAM is the DoD's third line as a
// structural guarantee: model.SystemHealth has no field that could report a
// Core CPU/RAM figure. The 2026-08-25 Supervisor role decision grants no
// */stats route for Core, so the field must not exist — never be present and
// zero.
func TestSystemHealth_NeverClaimsCoreCPUOrRAM(t *testing.T) {
	typ := reflect.TypeFor[model.SystemHealth]()
	for i := 0; i < typ.NumField(); i++ {
		name := strings.ToLower(typ.Field(i).Name)
		if !strings.Contains(name, "cpu") && !strings.Contains(name, "ram") && !strings.Contains(name, "memory") {
			continue
		}
		if strings.HasPrefix(name, "addon") {
			continue // this App's own resource use, not Core's — permitted.
		}
		t.Errorf("model.SystemHealth has field %q: Core CPU/RAM is not a granted figure (P3-02 DoD)", typ.Field(i).Name)
	}
}

// TestSystemTools_SchemaAcceptsNoArguments pins that, once wired with real
// readers, the two tools' schemas still accept nothing (the parity rule's
// fourth clause holds for an implemented tool, not just the placeholder).
func TestSystemTools_SchemaAcceptsNoArguments(t *testing.T) {
	client := connect(t, newServer(systemOptions(&fakeCoreReader{}, &fakeInventoryReader{}, &fakeSupervisorReader{}), Catalog()))
	res, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name != "get_system_overview" && tool.Name != "get_system_health" {
			continue
		}
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("%s: marshal schema: %v", tool.Name, err)
		}
		var schema struct {
			Type                 string `json:"type"`
			AdditionalProperties any    `json:"additionalProperties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("%s: unmarshal schema: %v", tool.Name, err)
		}
		if schema.Type != "object" || schema.AdditionalProperties != false {
			t.Errorf("%s: schema = %s, want a closed empty object", tool.Name, raw)
		}
	}
}
