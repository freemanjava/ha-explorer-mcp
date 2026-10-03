package analysis

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

func meshEntity(id, device, class, disabledBy string) model.Entity {
	// Platform is deliberately a string no code knows (D-05-5, rule 6).
	return model.Entity{ID: model.EntityID(id), Domain: "sensor", DeviceID: model.DeviceID(device),
		DeviceClass: class, DisabledBy: disabledBy, Platform: "unheard_of_stack_9f3"}
}

func missingReasons(r MeshResolution) []model.MissingReason {
	var out []model.MissingReason
	for _, m := range r.Missing {
		out = append(out, m.Reason)
	}
	return out
}

func TestResolveMeshMetrics_Zigbee2MQTTShape_LinkQualityPlusNotExposedRSSI(t *testing.T) {
	res := ResolveMeshMetrics([]model.Entity{
		meshEntity("sensor.bulb_linkquality", "d1", "", ""),
		{ID: "light.bulb", Domain: "light", DeviceID: "d1"},
	})
	if len(res.Metrics) != 1 || res.Metrics[0].Kind != MeshLinkQuality {
		t.Fatalf("metrics = %+v, want one link-quality metric", res.Metrics)
	}
	if got := missingReasons(res); !slices.Equal(got, []model.MissingReason{model.MissingNotExposed}) {
		t.Errorf("missing reasons = %v, want one not_exposed", got)
	}
}

func TestResolveMeshMetrics_ZHAShape_BothDisabled_TwoRowsNoEvidence(t *testing.T) {
	res := ResolveMeshMetrics([]model.Entity{
		meshEntity("sensor.bulb_lqi", "d1", "", "integration"),
		meshEntity("sensor.bulb_rssi", "d1", "signal_strength", "integration"),
	})
	if len(res.Metrics) != 0 {
		t.Errorf("metrics = %+v, want none: both are disabled", res.Metrics)
	}
	got := missingReasons(res)
	if !slices.Equal(got, []model.MissingReason{model.MissingEntityDisabled, model.MissingEntityDisabled}) {
		t.Errorf("missing reasons = %v, want two entity_disabled", got)
	}
}

func TestResolveMeshMetrics_DeviceClassWins_NameNotNeeded(t *testing.T) {
	res := ResolveMeshMetrics([]model.Entity{meshEntity("sensor.strange_name", "d1", "signal_strength", "")})
	if len(res.Metrics) != 1 || res.Metrics[0].Kind != MeshSignalStrength {
		t.Errorf("metrics = %+v, want a signal-strength metric from device_class alone", res.Metrics)
	}
}

func TestResolveMeshMetrics_NamesThatOnlyContainAHint_NotMatched(t *testing.T) {
	res := ResolveMeshMetrics([]model.Entity{
		meshEntity("sensor.mylqi", "d1", "", ""),
		meshEntity("sensor.temperature", "d1", "temperature", ""),
	})
	if len(res.Metrics) != 0 || len(res.Missing) != 0 {
		t.Errorf("resolution = %+v, want nothing", res)
	}
}

func TestResolveMeshMetrics_PlatformNeverRead(t *testing.T) {
	a := meshEntity("sensor.x_linkquality", "d1", "", "")
	b := a
	b.Platform = "zha"
	if !slices.Equal(missingReasons(ResolveMeshMetrics([]model.Entity{a})), missingReasons(ResolveMeshMetrics([]model.Entity{b}))) {
		t.Error("the platform string changed the outcome")
	}
	if len(ResolveMeshMetrics([]model.Entity{a}).Metrics) != 1 {
		t.Error("an unknown platform string blocked resolution")
	}
}

func TestResolveMeshMetrics_NoDevice_Ignored(t *testing.T) {
	if res := ResolveMeshMetrics([]model.Entity{meshEntity("sensor.x_lqi", "", "", "")}); len(res.Metrics)+len(res.Missing) != 0 {
		t.Errorf("a deviceless entity resolved: %+v", res)
	}
}

func TestMeshEvidence_NumericReadings_MinMeanSamples(t *testing.T) {
	to := clusterTo
	from := to.Add(-100 * time.Minute)
	pts := []model.HistoryPoint{
		{Timestamp: from, State: "unavailable"},
		{Timestamp: from.Add(50 * time.Minute), State: "40"},
		{Timestamp: from.Add(60 * time.Minute), State: "unknown"},
		{Timestamp: from.Add(70 * time.Minute), State: "80"},
	}
	ev, ok := MeshEvidence(MeshMetric{Kind: MeshLinkQuality, Entity: model.Entity{ID: "sensor.a_lqi"}}, from, to, pts)
	if !ok {
		t.Fatal("no evidence")
	}
	m := ev.Measurements
	if m["min"] != 40 || m["mean"] != 60 || m["samples"] != 2 || ev.SampleSize != 2 {
		t.Errorf("measurements = %v samples %d", m, ev.SampleSize)
	}
	if ev.Coverage != 0.5 {
		t.Errorf("coverage = %v, want 0.5 (first reading at the midpoint)", ev.Coverage)
	}
}

func TestMeshEvidence_NoNumericReading_NoEvidenceNeverZero(t *testing.T) {
	pts := []model.HistoryPoint{{Timestamp: clusterFrom, State: "unavailable"}}
	if ev, ok := MeshEvidence(MeshMetric{Kind: MeshLinkQuality, Entity: model.Entity{ID: "sensor.a_lqi"}}, clusterFrom, clusterTo, pts); ok {
		t.Errorf("evidence = %+v, want none", ev)
	}
}

func TestAnalyzeIntegrationHealth_MeshEvidence_NeverChangesHypotheses(t *testing.T) {
	run := func(lqi float64) model.HealthAnalysis {
		in := integrationInput()
		in.Outages = twoEntityCluster()
		ev, ok := MeshEvidence(MeshMetric{Kind: MeshLinkQuality, Entity: model.Entity{ID: "sensor.a_lqi"}},
			in.From, in.To, []model.HistoryPoint{{Timestamp: in.From, State: fmt.Sprint(lqi)}})
		if !ok {
			t.Fatal("no evidence")
		}
		in.MeshEvidence = []model.Evidence{ev}
		out, err := AnalyzeIntegrationHealth(in)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := evidenceByID(out, ev.ID); !ok {
			t.Fatalf("mesh evidence %q missing from the response", ev.ID)
		}
		return out
	}
	statements := func(a model.HealthAnalysis) (out []string) {
		for _, h := range a.Hypotheses {
			out = append(out, h.Statement()+string(h.Confidence()))
			for _, c := range h.Cites() {
				out = append(out, string(c))
			}
		}
		return out
	}
	weak, strong := statements(run(1)), statements(run(255))
	if !slices.Equal(weak, strong) {
		t.Errorf("LQI changed the hypotheses:\n1:   %v\n255: %v", weak, strong)
	}
	for _, s := range weak {
		if len(s) >= 5 && s[:5] == "mesh_" {
			t.Errorf("a hypothesis cites mesh evidence %q", s)
		}
	}
}
