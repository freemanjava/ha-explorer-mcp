package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/page"
)

// deviceRegistryReader is list_devices/get_device's read surface into the
// slow-moving registries: the device registry itself plus the entity
// registry needed for get_device's related-entities list.
type deviceRegistryReader interface {
	Devices(ctx context.Context) ([]model.DeviceRef, time.Time, error)
	Entities(ctx context.Context) ([]model.Entity, time.Time, error)
}

// ListDevicesInput is list_devices' typed, validated input: an optional
// area/config-entry/disabled filter plus the Phase 02 cursor-pagination
// contract. No field accepts a route, command, path or query (rule 2).
type ListDevicesInput struct {
	// AreaID filters to devices assigned to one area. Empty means no filter.
	AreaID string `json:"area_id,omitempty" jsonschema:"filter to one area id"`
	// ConfigEntryID filters to devices owned by one integration instance.
	// Empty means no filter.
	ConfigEntryID string `json:"config_entry_id,omitempty" jsonschema:"filter to one config entry (integration instance) id"`
	// Disabled, given, filters to only-disabled (true) or only-enabled
	// (false) devices. Omitted means no filter.
	Disabled *bool `json:"disabled,omitempty" jsonschema:"filter by whether the device is disabled"`

	Cursor string `json:"cursor,omitempty" jsonschema:"resume after this page's cursor"`
	Limit  int    `json:"limit,omitempty" jsonschema:"page size, default 50, max 200"`
}

// GetDeviceInput is get_device's typed input: exactly the device id, nothing
// an agent could use as a free-form route or query.
type GetDeviceInput struct {
	ID string `json:"id" jsonschema:"the device registry id to drill into"`
}

// withDeviceTools returns tools with list_devices and get_device's handlers
// bound, when opts supplies readers for both. A row whose readers are absent
// keeps its bindNotImplemented default.
func withDeviceTools(tools []Tool, opts Options) []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	for i := range out {
		if opts.Inventory == nil || opts.Availability == nil {
			continue
		}
		switch out[i].Name {
		case "list_devices":
			out[i].bind = bindListDevices(opts.Inventory)
		case "get_device":
			out[i].bind = bindGetDevice(opts.Inventory, opts.Availability)
		}
	}
	return out
}

// bindListDevices registers list_devices' typed handler. The input schema is
// left to inference from ListDevicesInput, which the SDK closes with
// additionalProperties:false.
func bindListDevices(registry deviceRegistryReader) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in ListDevicesInput) (*sdkmcp.CallToolResult, model.DeviceList, error) {
			out, err := listDevices(ctx, registry, in)
			return nil, out, err
		})
	}
}

// bindGetDevice registers get_device's typed handler.
func bindGetDevice(registry deviceRegistryReader, avail entityAvailabilityReader) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in GetDeviceInput) (*sdkmcp.CallToolResult, model.DeviceDetail, error) {
			out, err := getDevice(ctx, registry, avail, in)
			return nil, out, err
		})
	}
}

// matchesDeviceFilter reports whether d passes in's optional filters. A
// zero-value filter field matches everything, so an unset AreaID,
// ConfigEntryID or Disabled never excludes a device.
func matchesDeviceFilter(d model.DeviceRef, in ListDevicesInput) bool {
	if in.AreaID != "" && string(d.AreaID) != in.AreaID {
		return false
	}
	if in.ConfigEntryID != "" && string(d.ConfigEntryID) != in.ConfigEntryID {
		return false
	}
	if in.Disabled != nil && (d.DisabledBy != "") != *in.Disabled {
		return false
	}
	return true
}

// listDevices filters, sorts by id, and pages the device registry. DeviceRef
// is what leaves the boundary (P3-04 DoD) — no derived counts, no related
// lists, just the registry entries themselves.
func listDevices(ctx context.Context, registry deviceRegistryReader, in ListDevicesInput) (model.DeviceList, error) {
	devices, observedAt, err := registry.Devices(ctx)
	if err != nil {
		return model.DeviceList{}, err
	}

	filtered := make([]model.DeviceRef, 0, len(devices))
	for _, d := range devices {
		if matchesDeviceFilter(d, in) {
			filtered = append(filtered, d)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID < filtered[j].ID })

	pg, err := page.Paginate(filtered, in.Cursor, in.Limit, maxResponseBytes(ctx),
		func(d model.DeviceRef) string { return string(d.ID) },
		deviceByteSize,
	)
	if err != nil {
		return model.DeviceList{}, err
	}

	return model.DeviceList{
		Source:       "home_assistant_core",
		ObservedAt:   observedAt,
		Items:        pg.Items,
		NextCursor:   pg.NextCursor,
		Truncated:    pg.Truncated,
		LimitClamped: pg.LimitClamped,
	}, nil
}

// getDevice drills into one device by id: its metadata, its related
// entities (with availability computed the same way list_integrations'
// counts are, never by returning the underlying state list), and its
// via/parent topology. A missing id is ErrNotFound, not a
// partially-populated object (Appendix B: "gone between list and get").
func getDevice(ctx context.Context, registry deviceRegistryReader, avail entityAvailabilityReader, in GetDeviceInput) (model.DeviceDetail, error) {
	if in.ID == "" {
		return model.DeviceDetail{}, fmt.Errorf("get_device: id is required")
	}

	devices, observedAt, err := registry.Devices(ctx)
	if err != nil {
		return model.DeviceDetail{}, err
	}
	var device model.DeviceRef
	found := false
	for _, d := range devices {
		if string(d.ID) == in.ID {
			device = d
			found = true
			break
		}
	}
	if !found {
		return model.DeviceDetail{}, fmt.Errorf("%w: device %q", ha.ErrNotFound, in.ID)
	}

	entities, _, err := registry.Entities(ctx)
	if err != nil {
		return model.DeviceDetail{}, err
	}
	unavailable, err := avail.UnavailableEntityIDs(ctx)
	if err != nil {
		return model.DeviceDetail{}, err
	}

	var related []model.DeviceEntityRef
	for _, e := range entities {
		if e.DeviceID != device.ID {
			continue
		}
		_, isUnavailable := unavailable[e.ID]
		related = append(related, model.DeviceEntityRef{
			ID:        e.ID,
			Domain:    e.Domain,
			Name:      e.Name,
			Available: !isUnavailable,
		})
	}
	sort.Slice(related, func(i, j int) bool { return related[i].ID < related[j].ID })

	var viaDevice *model.DeviceRef
	var children []model.DeviceRef
	for _, d := range devices {
		if device.ViaDeviceID != "" && d.ID == device.ViaDeviceID {
			v := d
			viaDevice = &v
		}
		if d.ViaDeviceID == device.ID {
			children = append(children, d)
		}
	}
	sort.Slice(children, func(i, j int) bool { return children[i].ID < children[j].ID })

	return model.DeviceDetail{
		Source:          "home_assistant_core",
		ObservedAt:      observedAt,
		Device:          device,
		RelatedEntities: related,
		ViaDevice:       viaDevice,
		ChildDevices:    children,
	}, nil
}

// deviceByteSize approximates one device's serialized size for the page
// package's byte cap — cheap enough to run per record without
// re-serializing the whole response afterward.
func deviceByteSize(d model.DeviceRef) int64 {
	b, err := json.Marshal(d)
	if err != nil {
		return 0
	}
	return int64(len(b))
}
