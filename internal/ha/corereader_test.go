package ha

import (
	"encoding/json"
	"testing"
)

func TestCoreReader_CoreConfig_MapsFields(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandGetConfig, json.RawMessage(`{"version":"2026.8.3","location_name":"Home","time_zone":"UTC","state":"RUNNING"}`))

	cfg, err := NewCoreReader(fc).CoreConfig(testCtx(t))
	if err != nil {
		t.Fatalf("CoreConfig: %v", err)
	}
	if cfg.Version != "2026.8.3" || cfg.LocationName != "Home" || cfg.TimeZone != "UTC" || cfg.State != "RUNNING" {
		t.Fatalf("CoreConfig mapped %+v unexpectedly", cfg)
	}
	if cfg.Partial {
		t.Errorf("well-formed get_config marked Partial: %s", cfg.PartialReason)
	}
}

func TestCoreReader_CoreConfig_MissingVersion_MarksPartial(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandGetConfig, json.RawMessage(`{"location_name":"Home"}`))

	cfg, err := NewCoreReader(fc).CoreConfig(testCtx(t))
	if err != nil {
		t.Fatalf("CoreConfig: %v", err)
	}
	if !cfg.Partial {
		t.Fatal("get_config missing version was not marked Partial")
	}
}

func TestCoreReader_StateCounts_AggregatesWithoutExposingEntities(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandGetStates, json.RawMessage(`[
		{"entity_id":"light.kitchen","state":"on"},
		{"entity_id":"light.hallway","state":"unavailable"},
		{"entity_id":"sensor.attic","state":"unknown"},
		{"entity_id":"sensor.basement","state":"unknown"}
	]`))

	counts, err := NewCoreReader(fc).StateCounts(testCtx(t))
	if err != nil {
		t.Fatalf("StateCounts: %v", err)
	}
	if counts.Total != 4 || counts.Unavailable != 1 || counts.Unknown != 2 {
		t.Fatalf("StateCounts = %+v, want Total 4, Unavailable 1, Unknown 2", counts)
	}
}

func TestCoreReader_UpstreamError_Propagates(t *testing.T) {
	fc := newFakeCaller()
	fc.err = ErrUpstreamUnavailable

	if _, err := NewCoreReader(fc).CoreConfig(testCtx(t)); err == nil {
		t.Fatal("CoreConfig swallowed the upstream error")
	}
	if _, err := NewCoreReader(fc).StateCounts(testCtx(t)); err == nil {
		t.Fatal("StateCounts swallowed the upstream error")
	}
}
