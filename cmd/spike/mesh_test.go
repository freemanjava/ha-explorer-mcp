package main

import (
	"strings"
	"testing"
)

func device(id string, viaDeviceID any, domains ...string) map[string]any {
	var idents []any
	for _, d := range domains {
		idents = append(idents, []any{d, "irrelevant"})
	}
	return map[string]any{
		"id":            id,
		"identifiers":   idents,
		"via_device_id": viaDeviceID,
	}
}

func TestDeviceIdentifierDomains_CountsEachDomainOnce(t *testing.T) {
	list := []any{
		device("d1", nil, "zha"),
		device("d2", "d1", "zha"),
		device("d3", nil, "mqtt"),
	}

	got := deviceIdentifierDomains(list)

	if got["zha"] != 2 || got["mqtt"] != 1 {
		t.Fatalf("domains = %v, want zha:2 mqtt:1", got)
	}
}

func TestDeviceIdentifierDomains_Empty_ReturnsNone(t *testing.T) {
	if got := deviceIdentifierDomains(nil); len(got) != 0 {
		t.Fatalf("domains = %v, want empty", got)
	}
}

func TestEntityPlatformByID_MapsEntityToItsPlatform(t *testing.T) {
	list := []any{
		map[string]any{"entity_id": "sensor.a", "platform": "zha"},
		map[string]any{"entity_id": "sensor.b", "platform": "mqtt"},
		map[string]any{"entity_id": "sensor.c"},
	}

	got := entityPlatformByID(list)

	if got["sensor.a"] != "zha" || got["sensor.b"] != "mqtt" || got["sensor.c"] != "" {
		t.Fatalf("platform map = %v", got)
	}
}

func TestScanMeshEntities_ZHAViaAttributeKey(t *testing.T) {
	states := []any{
		state("sensor.hallway_motion", "on", map[string]any{"lqi": float64(200)}),
	}
	entityPlatform := map[string]string{"sensor.hallway_motion": "zha"}

	lqi, rssi := scanMeshEntities(states, entityPlatform)

	if len(lqi) != 1 || lqi[0].platform != "zha" || lqi[0].via != "attribute_key" {
		t.Fatalf("lqi = %+v, want one zha/attribute_key hit", lqi)
	}
	if len(rssi) != 0 {
		t.Fatalf("rssi = %+v, want none", rssi)
	}
}

func TestScanMeshEntities_Zigbee2MQTTViaEntityID(t *testing.T) {
	states := []any{
		state("sensor.hallway_motion_linkquality", "180", map[string]any{}),
	}
	entityPlatform := map[string]string{"sensor.hallway_motion_linkquality": "mqtt"}

	lqi, _ := scanMeshEntities(states, entityPlatform)

	if len(lqi) != 1 || lqi[0].platform != "mqtt" || lqi[0].via != "entity_id" {
		t.Fatalf("lqi = %+v, want one mqtt/entity_id hit", lqi)
	}
}

func TestScanMeshEntities_SignalStrengthViaDeviceClass(t *testing.T) {
	states := []any{
		state("sensor.hallway_motion_signal", "-60", map[string]any{"device_class": "signal_strength"}),
	}
	entityPlatform := map[string]string{"sensor.hallway_motion_signal": "zha"}

	_, rssi := scanMeshEntities(states, entityPlatform)

	if len(rssi) != 1 || rssi[0].via != "device_class" {
		t.Fatalf("rssi = %+v, want one device_class hit", rssi)
	}
}

func TestScanMeshEntities_NoHint_NoHit(t *testing.T) {
	states := []any{
		state("sensor.hallway_temperature", "21.5", map[string]any{"unit_of_measurement": "°C"}),
	}

	lqi, rssi := scanMeshEntities(states, nil)

	if len(lqi) != 0 || len(rssi) != 0 {
		t.Fatalf("lqi = %+v, rssi = %+v, want none", lqi, rssi)
	}
}

func TestStarVerdict_SingleParentOverNMinusOne_NamedAStar(t *testing.T) {
	// The ZHA shape confirmed from source, and the shape the owner's 27-of-28
	// Zigbee2MQTT run is consistent with.
	got := starVerdict(27, 28, 1)
	if !strings.Contains(got, "star") || !strings.Contains(got, "distinguishes nothing") {
		t.Fatalf("starVerdict = %q, want a star verdict naming that it distinguishes nothing", got)
	}
}

func TestStarVerdict_SeveralParents_NotAStar(t *testing.T) {
	got := starVerdict(27, 28, 4)
	if !strings.Contains(got, "real hierarchy") {
		t.Fatalf("starVerdict = %q, want a hierarchy verdict", got)
	}
}

func TestStarVerdict_NoParentAtAll_NoClaim(t *testing.T) {
	// Nothing populated is neither a star nor a hierarchy; claiming either
	// would be a verdict the numbers do not carry.
	if got := starVerdict(0, 28, 0); got != "" {
		t.Fatalf("starVerdict = %q, want no verdict", got)
	}
}

func TestFormatDomainCounts_Empty_ReturnsNone(t *testing.T) {
	if got := formatDomainCounts(nil); got != "none" {
		t.Fatalf("formatDomainCounts = %q, want none", got)
	}
}

func TestFormatDomainCounts_SortedByDomain(t *testing.T) {
	got := formatDomainCounts(map[string]int{"mqtt": 3, "zha": 5})
	want := "mqtt: 3, zha: 5"
	if got != want {
		t.Fatalf("formatDomainCounts = %q, want %q", got, want)
	}
}
