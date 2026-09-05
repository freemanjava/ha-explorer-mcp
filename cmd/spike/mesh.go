package main

import (
	"context"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/coder/websocket"
)

// linkQualityHints and signalStrengthHints are substrings this probe treats as
// evidence an entity id or attribute key names a mesh-diagnostic value. They
// are deliberately broad (both ZHA's and Zigbee2MQTT's own naming, and the
// generic HA `device_class`/`unit_of_measurement` conventions) because P5-01's
// question is exactly whether the two integrations agree on a name — a narrow
// pattern chosen from one of them would bias the answer before it is measured.
var (
	linkQualityHints    = []string{"lqi", "linkquality", "link_quality"}
	signalStrengthHints = []string{"rssi", "signal_strength", "signal_dbm"}
)

// meshEntityHit records why one entity matched, never its state or attribute
// values — only the platform that created it (from the entity registry) and
// which naming convention matched.
type meshEntityHit struct {
	platform string
	via      string // "entity_id" | "device_class" | "unit_of_measurement" | "attribute_key"
}

// probeMesh answers P5-01 (F-6, Q9): whether Zigbee/mesh link-quality,
// signal-strength and parent-topology evidence is readable in a comparable
// shape regardless of which Zigbee integration is in use. It never assumes
// which integration is present, nor which of the two conventions (ZHA's or
// Zigbee2MQTT's) either one follows — it looks for both and reports what it
// finds, split by the entity registry's own `platform` field, or states
// plainly that a signal was absent. Only field names, counts and shapes leave
// this function; no entity id, device id or attribute value does (CLAUDE.md
// rule 6, doc §4 T2).
func probeMesh(ctx context.Context, out *report, conn *websocket.Conn, ids *idSeq, red *redactor) {
	out.writef("## Mesh/Zigbee metric normalization (P5-01, F-6, Q9)\n\n")

	devices, err := wsCall(ctx, conn, ids, map[string]any{"type": "config/device_registry/list"})
	if err != nil {
		out.writef("`config/device_registry/list` TRANSPORT FAILURE: %v\n\n", err)
		return
	}
	if devices.status != "OK" {
		out.writef("`config/device_registry/list` — %s\n\n", red.apply(devices.status))
		return
	}
	deviceList, _ := devices.decoded.([]any)

	entities, err := wsCall(ctx, conn, ids, map[string]any{"type": "config/entity_registry/list"})
	if err != nil {
		out.writef("`config/entity_registry/list` TRANSPORT FAILURE: %v\n\n", err)
		return
	}
	if entities.status != "OK" {
		out.writef("`config/entity_registry/list` — %s\n\n", red.apply(entities.status))
		return
	}
	entityList, _ := entities.decoded.([]any)

	states, err := wsCall(ctx, conn, ids, map[string]any{"type": "get_states"})
	if err != nil {
		out.writef("`get_states` (mesh scan) TRANSPORT FAILURE: %v\n\n", err)
		return
	}
	stateList, _ := states.decoded.([]any)

	domains := deviceIdentifierDomains(deviceList)
	out.writef("Device identifier domains present in the registry: %s.\n\n", formatDomainCounts(domains))
	reportParentTopology(out, deviceList, domains)

	entityPlatform := entityPlatformByID(entityList)
	platforms := presentPlatforms(entityPlatform)
	if len(platforms) == 0 {
		out.writef("No entity registry entries carried a `platform`; the entity-level scan below is unattributed.\n\n")
	}

	lqi, rssi := scanMeshEntities(stateList, entityPlatform)
	reportMeshEntities(out, "link quality", lqi)
	reportMeshEntities(out, "signal strength", rssi)
}

// deviceIdentifierDomains counts, across every device registry entry, how many
// carry at least one identifier from each integration domain. `identifiers` is
// `[][2]string` of `[domain, id]`; only the domain — schema, not owner data —
// is ever read out of it.
func deviceIdentifierDomains(deviceList []any) map[string]int {
	domains := map[string]int{}
	for _, d := range deviceList {
		m, ok := d.(map[string]any)
		if !ok {
			continue
		}
		idents, _ := m["identifiers"].([]any)
		seen := map[string]bool{}
		for _, raw := range idents {
			pair, ok := raw.([]any)
			if !ok || len(pair) == 0 {
				continue
			}
			domain, _ := pair[0].(string)
			if domain != "" && !seen[domain] {
				seen[domain] = true
				domains[domain]++
			}
		}
	}
	return domains
}

func formatDomainCounts(domains map[string]int) string {
	if len(domains) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(domains))
	for k := range domains {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, formatCount(k, domains[k]))
	}
	return strings.Join(parts, ", ")
}

func formatCount(name string, n int) string {
	return name + ": " + strconv.Itoa(n)
}

// reportParentTopology answers the "how is via_device expressed" half of the
// DoD, split by which Zigbee-relevant domains are present. `via_device_id` is
// the device registry's only parent-topology field regardless of integration,
// so the interesting question is not its name but whether either integration
// populates it — and, decisively, with how many *distinct* values.
//
// The cardinality is what separates a real parent relation from a star: ZHA
// points every non-coordinator device at the coordinator
// (core 2026.8.3 `zha/entity.py` `device_info`), giving N−1 populated and
// exactly 1 distinct value. A count alone cannot tell that apart from a real
// routing hierarchy, which is why P5-01 could not close F-27 on counts.
func reportParentTopology(out *report, deviceList []any, domains map[string]int) {
	zigbeeDomains := []string{"zha", "mqtt", "zigbee2mqtt"}
	present := make([]string, 0, len(zigbeeDomains))
	for _, d := range zigbeeDomains {
		if domains[d] > 0 {
			present = append(present, d)
		}
	}
	if len(present) == 0 {
		out.writef("No ZHA, Zigbee2MQTT-via-MQTT-discovery or standalone `zigbee2mqtt` " +
			"domain devices were found; the parent-topology and entity scans below have " +
			"nothing Zigbee-specific to attribute, and are reported unattributed.\n\n")
		return
	}

	withParent := map[string]int{}
	total := map[string]int{}
	parents := map[string]map[string]bool{}
	for _, d := range deviceList {
		m, ok := d.(map[string]any)
		if !ok {
			continue
		}
		idents, _ := m["identifiers"].([]any)
		domain := ""
		for _, raw := range idents {
			pair, ok := raw.([]any)
			if !ok || len(pair) == 0 {
				continue
			}
			candidate, _ := pair[0].(string)
			if slices.Contains(present, candidate) {
				domain = candidate
				break
			}
		}
		if domain == "" {
			continue
		}
		total[domain]++
		// The parent id is a device-registry id — owner data, never printed.
		// Only how many distinct ones exist leaves this function.
		if parent, ok := m["via_device_id"].(string); ok && parent != "" {
			withParent[domain]++
			if parents[domain] == nil {
				parents[domain] = map[string]bool{}
			}
			parents[domain][parent] = true
		}
	}

	out.writef("`via_device_id` (the device registry's only parent-topology field) populated, by domain:\n\n")
	for _, d := range present {
		out.writef("- `%s`: %d of %d devices, pointing at %d distinct parent(s)%s\n",
			d, withParent[d], total[d], len(parents[d]), starVerdict(withParent[d], total[d], len(parents[d])))
	}
	out.writef("\n")
}

// starVerdict names the shape the three numbers describe, so the report says
// what they mean rather than leaving the reader to spot it. N−1 devices over a
// single parent is a coordinator star — uniform across integrations, and
// carrying no routing information, since every device shares that one parent
// (F-27).
func starVerdict(withParent, total, distinct int) string {
	if distinct == 1 && withParent == total-1 {
		return " — a coordinator/bridge star: the shared parent is every device's, so it distinguishes nothing"
	}
	if distinct > 1 {
		return " — more than one parent: a real hierarchy, not a bare star"
	}
	return ""
}

// entityPlatformByID maps entity id to the integration that created it, from
// the entity registry's own `platform` field — the authoritative attribution,
// independent of any naming convention this probe might guess wrong.
func entityPlatformByID(entityList []any) map[string]string {
	out := make(map[string]string, len(entityList))
	for _, e := range entityList {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		entityID, _ := m["entity_id"].(string)
		platform, _ := m["platform"].(string)
		if entityID != "" {
			out[entityID] = platform
		}
	}
	return out
}

func presentPlatforms(entityPlatform map[string]string) []string {
	seen := map[string]bool{}
	for _, p := range entityPlatform {
		if p != "" {
			seen[p] = true
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// scanMeshEntities looks for both ZHA's and Zigbee2MQTT's naming conventions
// on every live state: the entity id itself, its `device_class`, its
// `unit_of_measurement`, and — for the case neither integration exposes a
// dedicated entity and instead puts it on an existing sensor as an attribute —
// the attribute key set. Only which convention matched and which platform
// created the entity are kept; every value observed along the way is
// discarded.
func scanMeshEntities(stateList []any, entityPlatform map[string]string) (lqi, rssi []meshEntityHit) {
	for _, s := range stateList {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		entityID, _ := m["entity_id"].(string)
		attrs, _ := m["attributes"].(map[string]any)
		platform := entityPlatform[entityID]

		if via, ok := matchesHints(entityID, attrs, linkQualityHints); ok {
			lqi = append(lqi, meshEntityHit{platform: platform, via: via})
		}
		if via, ok := matchesHints(entityID, attrs, signalStrengthHints); ok {
			rssi = append(rssi, meshEntityHit{platform: platform, via: via})
		}
	}
	return lqi, rssi
}

// matchesHints checks the entity id, `device_class`, `unit_of_measurement` and
// attribute key set against hints, in that order, and reports which one hit.
func matchesHints(entityID string, attrs map[string]any, hints []string) (via string, ok bool) {
	lowerID := strings.ToLower(entityID)
	if containsAny(lowerID, hints) {
		return "entity_id", true
	}
	if dc, _ := attrs["device_class"].(string); containsAny(strings.ToLower(dc), hints) {
		return "device_class", true
	}
	if unit, _ := attrs["unit_of_measurement"].(string); containsAny(strings.ToLower(unit), hints) {
		return "unit_of_measurement", true
	}
	for k := range attrs {
		if containsAny(strings.ToLower(k), hints) {
			return "attribute_key", true
		}
	}
	return "", false
}

func containsAny(s string, hints []string) bool {
	for _, h := range hints {
		if strings.Contains(s, h) {
			return true
		}
	}
	return false
}

func reportMeshEntities(out *report, label string, hits []meshEntityHit) {
	if len(hits) == 0 {
		out.writef("No entity read as carrying **%s** (checked entity id, "+
			"`device_class`, `unit_of_measurement` and attribute keys against both "+
			"ZHA's and Zigbee2MQTT's naming). Either integration may still expose it "+
			"in a shape this heuristic does not recognise — absence here is not proof "+
			"of absence.\n\n", label)
		return
	}

	byPlatform := map[string]map[string]int{}
	for _, h := range hits {
		platform := h.platform
		if platform == "" {
			platform = "(no entity-registry entry)"
		}
		if byPlatform[platform] == nil {
			byPlatform[platform] = map[string]int{}
		}
		byPlatform[platform][h.via]++
	}

	out.writef("Entities read as carrying **%s**: %d total.\n\n", label, len(hits))
	platforms := make([]string, 0, len(byPlatform))
	for p := range byPlatform {
		platforms = append(platforms, p)
	}
	sort.Strings(platforms)
	for _, p := range platforms {
		vias := byPlatform[p]
		viaKeys := make([]string, 0, len(vias))
		for v := range vias {
			viaKeys = append(viaKeys, v)
		}
		sort.Strings(viaKeys)
		parts := make([]string, 0, len(viaKeys))
		for _, v := range viaKeys {
			parts = append(parts, v+": "+strconv.Itoa(vias[v]))
		}
		out.writef("- `%s` — %s\n", p, strings.Join(parts, ", "))
	}
	out.writef("\n")
}
