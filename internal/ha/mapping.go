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

// MapFloorRegistryList maps a config/floor_registry/list result into Floors.
// The element schema was unobserved by the 2026-08-23 probe — every
// installation sampled had an empty floor registry — so MapFloor assumes the
// field names HA's floor_registry component documents and marks a Floor
// Partial the moment one of them is missing, rather than trusting the
// assumption silently (docs/research/2026-08-23-ha-registry-apis.md finding 8).
func MapFloorRegistryList(raw json.RawMessage) ([]model.Floor, error) {
	var elements []map[string]any
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("ha: decoding floor registry list: %w", err)
	}
	floors := make([]model.Floor, 0, len(elements))
	for _, e := range elements {
		floors = append(floors, MapFloor(e))
	}
	return floors, nil
}

// MapFloor maps one floor registry entry. See MapFloorRegistryList for the
// unverified-schema caveat.
func MapFloor(raw map[string]any) model.Floor {
	var reasons []string

	id, ok := stringField(raw, "floor_id")
	if !ok || id == "" {
		reasons = append(reasons, "floor_id missing or not a string")
	}
	name, ok := stringField(raw, "name")
	if !ok {
		reasons = append(reasons, "name missing or not a string")
	}

	f := model.Floor{ID: id, Name: name, Icon: optString(raw, "icon")}
	if len(reasons) > 0 {
		f.Partial = true
		f.PartialReason = strings.Join(reasons, "; ")
	}
	return f
}

// MapLabelRegistryList maps a config/label_registry/list result into Labels.
// Same unverified-schema caveat as MapFloorRegistryList.
func MapLabelRegistryList(raw json.RawMessage) ([]model.Label, error) {
	var elements []map[string]any
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("ha: decoding label registry list: %w", err)
	}
	labels := make([]model.Label, 0, len(elements))
	for _, e := range elements {
		labels = append(labels, MapLabel(e))
	}
	return labels, nil
}

// MapLabel maps one label registry entry. See MapLabelRegistryList for the
// unverified-schema caveat.
func MapLabel(raw map[string]any) model.Label {
	var reasons []string

	id, ok := stringField(raw, "label_id")
	if !ok || id == "" {
		reasons = append(reasons, "label_id missing or not a string")
	}
	name, ok := stringField(raw, "name")
	if !ok {
		reasons = append(reasons, "name missing or not a string")
	}

	l := model.Label{ID: id, Name: name, Icon: optString(raw, "icon"), Color: optString(raw, "color")}
	if len(reasons) > 0 {
		l.Partial = true
		l.PartialReason = strings.Join(reasons, "; ")
	}
	return l
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

// MapUnavailableEntityIDs aggregates a get_states result into the set of
// entity ids currently unavailable or unknown, in-process — so a caller
// computing per-integration or per-device counts never sees the full
// per-entity state list either (P3-03 DoD: "counts are computed
// server-side, not by returning the underlying lists"). An element missing
// entity_id, or not a JSON object, is skipped rather than aborting the scan.
func MapUnavailableEntityIDs(raw json.RawMessage) (map[model.EntityID]struct{}, error) {
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("ha: decoding get_states: %w", err)
	}

	out := make(map[model.EntityID]struct{})
	for _, raw := range elements {
		var e map[string]any
		if err := json.Unmarshal(raw, &e); err != nil {
			continue
		}
		state := optString(e, "state")
		if state != "unavailable" && state != "unknown" {
			continue
		}
		id, ok := stringField(e, "entity_id")
		if !ok || id == "" {
			continue
		}
		out[model.EntityID(id)] = struct{}{}
	}
	return out, nil
}

// MapEntityStateValues maps a get_states result into the current state string
// of every entity, keyed by id. Unlike MapStateCounts and
// MapUnavailableEntityIDs, this deliberately does hand back a per-entity
// value — list_entities/get_entity's whole job is reporting one entity's
// state (P3-05), where get_system_overview and list_integrations exist
// precisely to avoid it. An element missing entity_id, or not a JSON object,
// is skipped rather than aborting the scan.
func MapEntityStateValues(raw json.RawMessage) (map[model.EntityID]string, error) {
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("ha: decoding get_states: %w", err)
	}

	out := make(map[model.EntityID]string, len(elements))
	for _, raw := range elements {
		var e map[string]any
		if err := json.Unmarshal(raw, &e); err != nil {
			continue
		}
		id, ok := stringField(e, "entity_id")
		if !ok || id == "" {
			continue
		}
		out[model.EntityID(id)] = optString(e, "state")
	}
	return out, nil
}

// MapAutomationStates aggregates a get_states result into one summary per
// automation-domain entity — the confirmed non-admin fallback source
// (docs/research/2026-08-23-ha-automation-traces.md): "did it fire, and
// when", not automation/config's full detail, which get_automation (P3-07)
// reaches through the admin-gated API instead. A non-automation entity is
// skipped; an element missing entity_id, or not a JSON object, is skipped
// rather than aborting the scan.
func MapAutomationStates(raw json.RawMessage) ([]model.AutomationSummary, error) {
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("ha: decoding get_states: %w", err)
	}

	out := make([]model.AutomationSummary, 0)
	for _, raw := range elements {
		var e map[string]any
		if err := json.Unmarshal(raw, &e); err != nil {
			continue
		}
		id, ok := stringField(e, "entity_id")
		if !ok || id == "" || entityDomain(id) != "automation" {
			continue
		}
		out = append(out, mapAutomationState(model.EntityID(id), e))
	}
	return out, nil
}

// mapAutomationState maps one get_states element already known to be an
// automation entity.
func mapAutomationState(id model.EntityID, e map[string]any) model.AutomationSummary {
	a := model.AutomationSummary{
		EntityID: id,
		Enabled:  optString(e, "state") == "on",
	}

	attrs, ok := e["attributes"].(map[string]any)
	if !ok {
		a.Partial = true
		a.PartialReason = "attributes missing or not an object"
		return a
	}

	a.Alias = optString(attrs, "friendly_name")
	a.Mode = optString(attrs, "mode")
	if t, ok := optTime(attrs, "last_triggered"); ok {
		a.LastTriggered = &t
	}
	if current, ok := attrs["current"].(float64); ok {
		a.CurrentRuns = int(current)
	}
	return a
}

// MapRepairs maps a repairs/list_issues result — {"issues": [...]}, an object
// wrapping the array, not the bare array get_states/registry commands return
// (docs/research/2026-09-05-ha-repairs-api.md) — into one Repair per element.
// A malformed element degrades to Partial rather than aborting the scan.
func MapRepairs(raw json.RawMessage) ([]model.Repair, error) {
	var wire struct {
		Issues []map[string]any `json:"issues"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("ha: decoding repairs/list_issues: %w", err)
	}

	out := make([]model.Repair, 0, len(wire.Issues))
	for _, e := range wire.Issues {
		out = append(out, MapRepair(e))
	}
	return out, nil
}

// MapRepair maps one repairs/list_issues element. See MapRepairs for the
// wrapper it is read through.
func MapRepair(raw map[string]any) model.Repair {
	var reasons []string

	id, ok := stringField(raw, "issue_id")
	if !ok || id == "" {
		reasons = append(reasons, "issue_id missing or not a string")
	}
	domain, ok := stringField(raw, "domain")
	if !ok {
		reasons = append(reasons, "domain missing or not a string")
	}
	severity, ok := stringField(raw, "severity")
	if !ok {
		reasons = append(reasons, "severity missing or not a string")
	}

	r := model.Repair{
		IssueID:                 id,
		Domain:                  domain,
		Severity:                severity,
		IsFixable:               optBool(raw, "is_fixable"),
		Ignored:                 optBool(raw, "ignored"),
		DismissedVersion:        optString(raw, "dismissed_version"),
		BreaksInHAVersion:       optString(raw, "breaks_in_ha_version"),
		IssueDomain:             optString(raw, "issue_domain"),
		LearnMoreURL:            optString(raw, "learn_more_url"),
		TranslationKey:          optString(raw, "translation_key"),
		TranslationPlaceholders: optObject(raw, "translation_placeholders"),
	}
	if created, ok := optTime(raw, "created"); ok {
		r.Created = created
	} else {
		reasons = append(reasons, "created missing or not a timestamp")
	}
	if len(reasons) > 0 {
		r.Partial = true
		r.PartialReason = strings.Join(reasons, "; ")
	}
	return r
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

// optObject reads a free-form JSON object field verbatim — for HA data that
// is opaque metadata rather than schema (CLAUDE.md rule 6), such as a
// repair's translation_placeholders. A missing or malformed field returns an
// empty, non-nil map: the MCP SDK's schema validation requires the declared
// "object" type even when HA sent nothing, and a nil map marshals to JSON
// null instead.
func optObject(raw map[string]any, key string) map[string]any {
	obj, ok := raw[key].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return obj
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

// MapAutomationConfigResult unwraps automation/config's {"config": {...}}
// envelope (docs/research/2026-08-23-ha-automation-traces.md) and maps the
// inner object with MapAutomation. entityID comes from the request, as
// MapAutomation documents: the command does not echo it back.
func MapAutomationConfigResult(entityID model.EntityID, raw json.RawMessage) (model.Automation, error) {
	var wire struct {
		Config map[string]any `json:"config"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return model.Automation{}, fmt.Errorf("ha: decoding automation/config: %w", err)
	}
	return MapAutomation(entityID, wire.Config), nil
}

// traceSummaryWire is the strictly-typed shape of one trace/list element. A
// trace's execution outcome is the evidence get_automation_traces exists to
// serve — unlike a registry entry, a field HA retypes (state renamed,
// timestamp reshaped) must fail the whole call rather than let the detection
// layer reason about a value that was never really there (P3-07 DoD: "a
// mutated response shape ... fails loudly rather than mapping garbage into
// the domain model", Appendix B).
type traceSummaryWire struct {
	RunID           string `json:"run_id"`
	State           string `json:"state"`
	ScriptExecution string `json:"script_execution"`
	LastStep        string `json:"last_step"`
	Trigger         string `json:"trigger"`
	Timestamp       struct {
		Start  time.Time  `json:"start"`
		Finish *time.Time `json:"finish"`
	} `json:"timestamp"`
}

// MapAutomationTraces maps a trace/list result — a bare array, one element
// per stored run — into one AutomationTraceSummary per run, newest fields
// first as HA answers them. See traceSummaryWire for why a shape mismatch
// fails the whole call instead of degrading one element to Partial.
func MapAutomationTraces(raw json.RawMessage) ([]model.AutomationTraceSummary, error) {
	var wire []traceSummaryWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("%w: decoding trace/list: %v", ErrUnexpectedMessage, err)
	}

	out := make([]model.AutomationTraceSummary, 0, len(wire))
	for _, w := range wire {
		t := model.AutomationTraceSummary{
			RunID:           w.RunID,
			State:           w.State,
			ScriptExecution: w.ScriptExecution,
			LastStep:        w.LastStep,
			Trigger:         w.Trigger,
			TimestampStart:  w.Timestamp.Start,
		}
		if w.Timestamp.Finish != nil {
			t.TimestampFinish = *w.Timestamp.Finish
		}
		out = append(out, t)
	}
	return out, nil
}

// MapLogbookEvents maps a logbook/get_events result — a bare array — into one
// LogbookEvent per entry. An element missing a field degrades to Partial
// rather than aborting the whole fallback read: this is degraded evidence
// already, and the fallback losing one field must not also lose every other
// entry around it.
func MapLogbookEvents(raw json.RawMessage) ([]model.LogbookEvent, error) {
	var elements []map[string]any
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("ha: decoding logbook/get_events: %w", err)
	}

	out := make([]model.LogbookEvent, 0, len(elements))
	for _, e := range elements {
		ev := model.LogbookEvent{
			Name:      optString(e, "name"),
			Message:   optString(e, "message"),
			EntityID:  model.EntityID(optString(e, "entity_id")),
			ContextID: optString(e, "context_id"),
		}
		if t, ok := optTime(e, "when"); ok {
			ev.When = t
		} else {
			ev.Partial = true
			ev.PartialReason = "when missing or not a timestamp"
		}
		out = append(out, ev)
	}
	return out, nil
}
