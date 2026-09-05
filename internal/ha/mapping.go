package ha

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// This file is the explicit mapping boundary CLAUDE.md's API & DTO Design
// section requires: raw HA JSON is decoded permissively into map[string]any
// here and only here, so a field HA renamed, dropped or mistyped on an
// upgrade degrades one value to partial instead of panicking the process
// (CLAUDE.md, Error Handling — "fail fast on programmer errors, be tolerant
// of external data"). internal/model never sees a HA JSON shape.

// MapEntityRegistryList maps a config/entity_registry/list (or /get_single
// element) result — a JSON array of entity registry entries — into Entities.
// A malformed element (not a JSON object) is skipped from the slice and does
// not abort the rest; a malformed *field* inside an otherwise-decodable
// element maps to a Partial entity instead.
func MapEntityRegistryList(raw json.RawMessage) ([]model.Entity, error) {
	var elements []map[string]any
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("ha: decoding entity registry list: %w", err)
	}
	entities := make([]model.Entity, 0, len(elements))
	for _, e := range elements {
		entities = append(entities, MapEntity(e))
	}
	return entities, nil
}

// MapEntity maps one entity registry entry. raw is the entry already decoded
// to a generic JSON object (a map/slice/string/float64/bool/nil tree), so a
// wrong-typed field is a missed type assertion here rather than a decode
// error the caller has to route around.
func MapEntity(raw map[string]any) model.Entity {
	var reasons []string

	entityID, ok := stringField(raw, "entity_id")
	if !ok || entityID == "" {
		reasons = append(reasons, "entity_id missing or not a string")
	}
	platform, ok := stringField(raw, "platform")
	if !ok {
		reasons = append(reasons, "platform missing or not a string")
	}

	e := model.Entity{
		ID:             model.EntityID(entityID),
		Domain:         entityDomain(entityID),
		UniqueID:       optString(raw, "unique_id"),
		Platform:       platform,
		DeviceID:       model.DeviceID(optString(raw, "device_id")),
		AreaID:         model.AreaID(optString(raw, "area_id")),
		ConfigEntryID:  model.ConfigEntryID(optString(raw, "config_entry_id")),
		Name:           optString(raw, "name"),
		OriginalName:   optString(raw, "original_name"),
		Icon:           optString(raw, "icon"),
		OriginalIcon:   optString(raw, "original_icon"),
		EntityCategory: optString(raw, "entity_category"),
		DeviceClass:    firstNonEmpty(optString(raw, "device_class"), optString(raw, "original_device_class")),
		DisabledBy:     optString(raw, "disabled_by"),
		HiddenBy:       optString(raw, "hidden_by"),
		HasEntityName:  optBool(raw, "has_entity_name"),
		TranslationKey: optString(raw, "translation_key"),
		Labels:         optStringSlice(raw, "labels"),
	}
	if t, ok := optTime(raw, "created_at"); ok {
		e.CreatedAt = t
	}
	if t, ok := optTime(raw, "modified_at"); ok {
		e.ModifiedAt = t
	}
	if len(reasons) > 0 {
		e.Partial = true
		e.PartialReason = strings.Join(reasons, "; ")
	}
	return e
}

// entityDomain extracts the "domain" half of a "domain.object_id" entity id.
// An entity id that does not contain the separator (already reported partial
// by the caller) yields an empty domain rather than panicking on a missing
// index.
func entityDomain(entityID string) string {
	if i := strings.IndexByte(entityID, '.'); i > 0 {
		return entityID[:i]
	}
	return ""
}

// MapDeviceRegistryList maps a config/device_registry/list result into
// DeviceRefs, following the same per-element tolerance as
// MapEntityRegistryList.
func MapDeviceRegistryList(raw json.RawMessage) ([]model.DeviceRef, error) {
	var elements []map[string]any
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("ha: decoding device registry list: %w", err)
	}
	devices := make([]model.DeviceRef, 0, len(elements))
	for _, e := range elements {
		devices = append(devices, MapDevice(e))
	}
	return devices, nil
}

// MapDevice maps one device registry entry.
func MapDevice(raw map[string]any) model.DeviceRef {
	var reasons []string

	id, ok := stringField(raw, "id")
	if !ok || id == "" {
		reasons = append(reasons, "id missing or not a string")
	}

	d := model.DeviceRef{
		ID:            model.DeviceID(id),
		ConfigEntryID: model.ConfigEntryID(firstConfigEntry(raw)),
		Name:          firstNonEmpty(optString(raw, "name_by_user"), optString(raw, "name")),
		AreaID:        model.AreaID(optString(raw, "area_id")),
		Manufacturer:  optString(raw, "manufacturer"),
		Model:         optString(raw, "model"),
		SWVersion:     optString(raw, "sw_version"),
		HWVersion:     optString(raw, "hw_version"),
		SerialNumber:  optString(raw, "serial_number"),
		Connections:   optPairSlice(raw, "connections"),
		Identifiers:   optPairSlice(raw, "identifiers"),
		ViaDeviceID:   model.DeviceID(optString(raw, "via_device_id")),
		DisabledBy:    optString(raw, "disabled_by"),
	}
	if len(reasons) > 0 {
		d.Partial = true
		d.PartialReason = strings.Join(reasons, "; ")
	}
	return d
}

// firstConfigEntry returns a device's primary config entry id. Core 2026.8
// carries it as a "config_entries" array (a device may belong to more than
// one), replacing the older singular "config_entry_id" field on some
// releases; both are checked so an adapter degrades on either shape rather
// than silently returning empty (architecture doc §8, CLAUDE.md Reliability:
// "never assume a field survives an HA upgrade").
func firstConfigEntry(raw map[string]any) string {
	if s := optString(raw, "config_entry_id"); s != "" {
		return s
	}
	if arr, ok := raw["config_entries"].([]any); ok && len(arr) > 0 {
		if s, ok := arr[0].(string); ok {
			return s
		}
	}
	return ""
}

// MapAreaRegistryList maps a config/area_registry/list result into Areas.
func MapAreaRegistryList(raw json.RawMessage) ([]model.Area, error) {
	var elements []map[string]any
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("ha: decoding area registry list: %w", err)
	}
	areas := make([]model.Area, 0, len(elements))
	for _, e := range elements {
		areas = append(areas, MapArea(e))
	}
	return areas, nil
}

// MapArea maps one area registry entry.
func MapArea(raw map[string]any) model.Area {
	var reasons []string

	id, ok := stringField(raw, "area_id")
	if !ok || id == "" {
		reasons = append(reasons, "area_id missing or not a string")
	}
	name, ok := stringField(raw, "name")
	if !ok {
		reasons = append(reasons, "name missing or not a string")
	}

	a := model.Area{
		ID:      model.AreaID(id),
		Name:    name,
		FloorID: optString(raw, "floor_id"),
		Icon:    optString(raw, "icon"),
		Labels:  optStringSlice(raw, "labels"),
	}
	if len(reasons) > 0 {
		a.Partial = true
		a.PartialReason = strings.Join(reasons, "; ")
	}
	return a
}

// MapConfigEntriesGet maps a config_entries/get result — a bare JSON array,
// distinct from get_single's {"config_entry": {...}} envelope (research doc
// finding 2; get_single is not allow-listed, see gateway.go) — into
// Integrations.
func MapConfigEntriesGet(raw json.RawMessage) ([]model.Integration, error) {
	var elements []map[string]any
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("ha: decoding config entries: %w", err)
	}
	integrations := make([]model.Integration, 0, len(elements))
	for _, e := range elements {
		integrations = append(integrations, MapIntegration(e))
	}
	return integrations, nil
}

// MapIntegration maps one config entry.
func MapIntegration(raw map[string]any) model.Integration {
	var reasons []string

	id, ok := stringField(raw, "entry_id")
	if !ok || id == "" {
		reasons = append(reasons, "entry_id missing or not a string")
	}
	domain, ok := stringField(raw, "domain")
	if !ok {
		reasons = append(reasons, "domain missing or not a string")
	}

	state := optString(raw, "state")
	i := model.Integration{
		ID:         model.ConfigEntryID(id),
		Domain:     domain,
		Title:      optString(raw, "title"),
		State:      state,
		Source:     optString(raw, "source"),
		Disabled:   optString(raw, "disabled_by") != "",
		DisabledBy: optString(raw, "disabled_by"),
		Reason:     optString(raw, "reason"),
	}
	if len(reasons) > 0 {
		i.Partial = true
		i.PartialReason = strings.Join(reasons, "; ")
	}
	return i
}

// MapAutomation maps one automation/config result. That command answers for
// a single entity id passed by the caller and does not echo it back, so
// entityID comes from the request, not the payload (research doc — no
// element schema is shared across commands; this one is keyed by request,
// not response).
func MapAutomation(entityID model.EntityID, raw map[string]any) model.Automation {
	var reasons []string

	id, ok := stringField(raw, "id")
	if !ok {
		reasons = append(reasons, "id missing or not a string")
	}
	alias, ok := stringField(raw, "alias")
	if !ok {
		reasons = append(reasons, "alias missing or not a string")
	}

	a := model.Automation{
		EntityID:       entityID,
		ID:             id,
		Alias:          alias,
		Mode:           optString(raw, "mode"),
		TriggerCount:   sequenceLen(raw, "trigger", "triggers"),
		ConditionCount: sequenceLen(raw, "condition", "conditions"),
		ActionCount:    sequenceLen(raw, "action", "actions"),
	}
	if len(reasons) > 0 {
		a.Partial = true
		a.PartialReason = strings.Join(reasons, "; ")
	}
	return a
}

// sequenceLen counts a HA automation config sequence. HA accepts both the
// plural and, for a single step, the singular key holding a bare object
// instead of a one-element array — both forms are counted as 1 in that case,
// never parsed for content (CLAUDE.md rule 6).
func sequenceLen(raw map[string]any, keys ...string) int {
	for _, k := range keys {
		v, ok := raw[k]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return len(arr)
		}
		if v != nil {
			return 1
		}
	}
	return 0
}

// supervisorInfoWire is the strictly-typed shape of a /supervisor/info
// response. Unlike the registry mappers above, a field HA/Supervisor renamed
// or retyped is not degraded to Partial here: json.Unmarshal fails the whole
// call, so a mutated shape is reported rather than mapped into garbage
// (P1-08 DoD). /supervisor/info is Supervisor's own status endpoint, not a
// Core registry HA upgrades independently, so this project chooses to trust
// its shape more strictly than Core's.
type supervisorInfoWire struct {
	Version       string `json:"version"`
	VersionLatest string `json:"version_latest"`
	Channel       string `json:"channel"`
	Supported     bool   `json:"supported"`
	Healthy       bool   `json:"healthy"`
	Addons        []struct {
		Slug            string `json:"slug"`
		Name            string `json:"name"`
		Version         string `json:"version"`
		VersionLatest   string `json:"version_latest"`
		UpdateAvailable bool   `json:"update_available"`
		State           string `json:"state"`
		Repository      string `json:"repository"`
	} `json:"addons"`
}

// MapSupervisorInfo maps a /supervisor/info response into model.SupervisorInfo.
func MapSupervisorInfo(raw json.RawMessage) (model.SupervisorInfo, error) {
	var wire supervisorInfoWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return model.SupervisorInfo{}, fmt.Errorf("%w: decoding /supervisor/info: %v", ErrUnexpectedMessage, err)
	}

	info := model.SupervisorInfo{
		Version:       wire.Version,
		VersionLatest: wire.VersionLatest,
		Channel:       wire.Channel,
		Supported:     wire.Supported,
		Healthy:       wire.Healthy,
	}
	for _, a := range wire.Addons {
		info.Apps = append(info.Apps, model.App{
			Slug:            a.Slug,
			Name:            a.Name,
			Version:         a.Version,
			VersionLatest:   a.VersionLatest,
			UpdateAvailable: a.UpdateAvailable,
			State:           a.State,
			Repository:      a.Repository,
		})
	}
	return info, nil
}

// MapCoreConfig maps a get_config result into model.CoreConfig, following the
// same permissive-field convention as the registry mappers: a missing or
// wrong-typed field degrades the value to Partial rather than aborting
// get_system_overview.
func MapCoreConfig(raw json.RawMessage) (model.CoreConfig, error) {
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return model.CoreConfig{}, fmt.Errorf("ha: decoding get_config: %w", err)
	}

	var reasons []string
	version, ok := stringField(fields, "version")
	if !ok || version == "" {
		reasons = append(reasons, "version missing or not a string")
	}

	c := model.CoreConfig{
		Version:      version,
		LocationName: optString(fields, "location_name"),
		TimeZone:     optString(fields, "time_zone"),
		State:        optString(fields, "state"),
	}
	if len(reasons) > 0 {
		c.Partial = true
		c.PartialReason = strings.Join(reasons, "; ")
	}
	return c, nil
}

// MapStateCounts aggregates a get_states result into model.StateCounts
// in-process, so get_system_overview never returns the underlying per-entity
// list (P3-02 DoD; CLAUDE.md, Performance: aggregate before serializing). An
// element that is not a JSON object is skipped rather than aborting the
// count.
func MapStateCounts(raw json.RawMessage) (model.StateCounts, error) {
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return model.StateCounts{}, fmt.Errorf("ha: decoding get_states: %w", err)
	}

	var counts model.StateCounts
	for _, raw := range elements {
		var e map[string]any
		if err := json.Unmarshal(raw, &e); err != nil {
			continue
		}
		counts.Total++
		switch optString(e, "state") {
		case "unavailable":
			counts.Unavailable++
		case "unknown":
			counts.Unknown++
		}
	}
	return counts, nil
}

// coreInfoWire is the strictly-typed shape of Supervisor's /info response —
// Supervisor's own status endpoint, not a Core registry HA upgrades
// independently, so (like supervisorInfoWire below) a renamed or retyped
// field fails the call loudly rather than degrading to Partial (P1-08 DoD
// rationale, docs/research/2026-08-23-supervisor-permissions.md).
type coreInfoWire struct {
	Supervisor    string `json:"supervisor"`
	HomeAssistant string `json:"homeassistant"`
	Hassos        string `json:"hassos"`
	Hostname      string `json:"hostname"`
	Machine       string `json:"machine"`
	Arch          string `json:"arch"`
	State         string `json:"state"`
	Supported     bool   `json:"supported"`
}

// MapCoreInfo maps Supervisor's /info response into model.CoreInfo.
func MapCoreInfo(raw json.RawMessage) (model.CoreInfo, error) {
	var wire coreInfoWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return model.CoreInfo{}, fmt.Errorf("%w: decoding Supervisor /info: %v", ErrUnexpectedMessage, err)
	}
	return model.CoreInfo{
		CoreVersion:       wire.HomeAssistant,
		SupervisorVersion: wire.Supervisor,
		OSVersion:         wire.Hassos,
		Hostname:          wire.Hostname,
		Machine:           wire.Machine,
		Arch:              wire.Arch,
		State:             wire.State,
		Supported:         wire.Supported,
	}, nil
}

// osInfoWire is the strictly-typed shape of Supervisor's /os/info response.
type osInfoWire struct {
	Version         string `json:"version"`
	UpdateAvailable bool   `json:"update_available"`
}

// MapOSInfo maps Supervisor's /os/info response into model.OSInfo.
func MapOSInfo(raw json.RawMessage) (model.OSInfo, error) {
	var wire osInfoWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return model.OSInfo{}, fmt.Errorf("%w: decoding Supervisor /os/info: %v", ErrUnexpectedMessage, err)
	}
	return model.OSInfo{Version: wire.Version, UpdateAvailable: wire.UpdateAvailable}, nil
}

// hostInfoWire is the strictly-typed shape of the disk fields in Supervisor's
// /host/info response — the rest of that payload is out of scope for
// get_system_health (P3-02).
type hostInfoWire struct {
	DiskFree  float64 `json:"disk_free"`
	DiskTotal float64 `json:"disk_total"`
	DiskUsed  float64 `json:"disk_used"`
}

// MapHostDisk maps Supervisor's /host/info response into model.HostDisk.
func MapHostDisk(raw json.RawMessage) (model.HostDisk, error) {
	var wire hostInfoWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return model.HostDisk{}, fmt.Errorf("%w: decoding Supervisor /host/info: %v", ErrUnexpectedMessage, err)
	}
	return model.HostDisk{FreeGB: wire.DiskFree, TotalGB: wire.DiskTotal, UsedGB: wire.DiskUsed}, nil
}

// resolutionInfoWire is the strictly-typed shape of Supervisor's
// /resolution/info response that get_system_health reports: issue count and
// reason strings, never the full issue objects.
type resolutionInfoWire struct {
	Unhealthy   []string         `json:"unhealthy"`
	Unsupported []string         `json:"unsupported"`
	Issues      []map[string]any `json:"issues"`
}

// MapResolutionInfo maps Supervisor's /resolution/info response into
// model.ResolutionSummary.
func MapResolutionInfo(raw json.RawMessage) (model.ResolutionSummary, error) {
	var wire resolutionInfoWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return model.ResolutionSummary{}, fmt.Errorf("%w: decoding Supervisor /resolution/info: %v", ErrUnexpectedMessage, err)
	}
	return model.ResolutionSummary{
		IssueCount:  len(wire.Issues),
		Unhealthy:   wire.Unhealthy,
		Unsupported: wire.Unsupported,
	}, nil
}

// addonStatsWire is the strictly-typed shape of Supervisor's
// /addons/self/stats response fields get_system_health reports.
type addonStatsWire struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
}

// MapAddonStats maps Supervisor's /addons/self/stats response — this App's
// own container resource use — into model.AddonStats.
func MapAddonStats(raw json.RawMessage) (model.AddonStats, error) {
	var wire addonStatsWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return model.AddonStats{}, fmt.Errorf("%w: decoding Supervisor /addons/self/stats: %v", ErrUnexpectedMessage, err)
	}
	return model.AddonStats{CPUPercent: wire.CPUPercent, MemoryPercent: wire.MemoryPercent}, nil
}

// --- permissive field extraction -------------------------------------------
//
// Every accessor here reports absence or a type mismatch as "not present"
// rather than panicking a type assertion, so one malformed field degrades the
// value it belongs to instead of the whole mapping call.

func stringField(raw map[string]any, key string) (string, bool) {
	v, ok := raw[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func optString(raw map[string]any, key string) string {
	s, _ := stringField(raw, key)
	return s
}

func optBool(raw map[string]any, key string) bool {
	b, _ := raw[key].(bool)
	return b
}

func optStringSlice(raw map[string]any, key string) []string {
	arr, ok := raw[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// optPairSlice reads a HA `[[a, b], [a, b], ...]` field (device registry
// connections/identifiers). A malformed pair is skipped rather than aborting
// the whole field.
func optPairSlice(raw map[string]any, key string) [][2]string {
	arr, ok := raw[key].([]any)
	if !ok {
		return nil
	}
	out := make([][2]string, 0, len(arr))
	for _, v := range arr {
		pair, ok := v.([]any)
		if !ok || len(pair) != 2 {
			continue
		}
		a, aok := pair[0].(string)
		b, bok := pair[1].(string)
		if !aok || !bok {
			continue
		}
		out = append(out, [2]string{a, b})
	}
	return out
}

// optTime reads an RFC 3339 timestamp field. HA has also been observed
// encoding created_at/modified_at as a Unix epoch float on some releases; both
// are accepted so an upgrade that changes the encoding degrades to "field
// absent", not a decode error for the whole entry.
func optTime(raw map[string]any, key string) (time.Time, bool) {
	v, ok := raw[key]
	if !ok {
		return time.Time{}, false
	}
	switch t := v.(type) {
	case string:
		parsed, err := time.Parse(time.RFC3339, t)
		if err != nil {
			return time.Time{}, false
		}
		return parsed, true
	case float64:
		return time.Unix(0, int64(t*float64(time.Second))).UTC(), true
	default:
		return time.Time{}, false
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
