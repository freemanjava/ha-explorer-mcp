package analysis

import (
	"encoding/json"
	"errors"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

var (
	clusterFrom = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	clusterTo   = clusterFrom.Add(24 * time.Hour)
)

// minute returns an instant the given minutes into the clustering window.
func minute(minutes int) time.Time { return clusterFrom.Add(time.Duration(minutes) * time.Minute) }

// outageInput builds one entity's input with outages given as minute pairs
// inside the window, attached to a device in a config entry and area.
func outageInput(id string, device model.DeviceID, entry model.ConfigEntryID, area model.AreaID, spans ...[2]int) EntityOutages {
	var outages []Outage
	for _, s := range spans {
		outages = append(outages, Outage{From: minute(s[0]), To: minute(s[1]), Duration: minute(s[1]).Sub(minute(s[0]))})
	}
	return EntityOutages{
		Entity: model.Entity{ID: model.EntityID(id), DeviceID: device, ConfigEntryID: entry, AreaID: area},
		Availability: AvailabilityReport{
			From: clusterFrom, To: clusterTo, Window: clusterTo.Sub(clusterFrom),
			Covered: clusterTo.Sub(clusterFrom), CoveredFrom: clusterFrom, CoverageComplete: true,
			Computable: true, Outages: outages, UnavailablePeriods: len(outages),
		},
	}
}

// zigbeeStar is the F-27 topology: every device of one config entry names the
// coordinator as its via_device, the coordinator itself names none.
func zigbeeStar() []model.DeviceRef {
	return []model.DeviceRef{
		{ID: "coordinator", ConfigEntryID: "zigbee"},
		{ID: "plug", ConfigEntryID: "zigbee", ViaDeviceID: "coordinator"},
		{ID: "bulb", ConfigEntryID: "zigbee", ViaDeviceID: "coordinator"},
		{ID: "sensor", ConfigEntryID: "zigbee", ViaDeviceID: "coordinator"},
	}
}

// hubTopology has two hubs in one config entry, so naming one of them is a
// claim that distinguishes some devices from others.
func hubTopology() []model.DeviceRef {
	return []model.DeviceRef{
		{ID: "hub_a", ConfigEntryID: "bridge"},
		{ID: "hub_b", ConfigEntryID: "bridge"},
		{ID: "a1", ConfigEntryID: "bridge", ViaDeviceID: "hub_a"},
		{ID: "a2", ConfigEntryID: "bridge", ViaDeviceID: "hub_a"},
		{ID: "b1", ConfigEntryID: "bridge", ViaDeviceID: "hub_b"},
	}
}

func mustCluster(t *testing.T, inputs []EntityOutages, devices []model.DeviceRef) OutageClustering {
	t.Helper()
	got, err := ClusterOutages(clusterFrom, clusterTo, inputs, devices)
	if err != nil {
		t.Fatalf("ClusterOutages: %v", err)
	}
	return got
}

func members(c OutageCluster) []string {
	out := make([]string, len(c.Members))
	for i, m := range c.Members {
		out[i] = string(m)
	}
	return out
}

func hasTrait(traits []model.ClusterTrait, kind model.TraitKind) (model.ClusterTrait, bool) {
	for _, tr := range traits {
		if tr.Kind == kind {
			return tr, true
		}
	}
	return model.ClusterTrait{}, false
}

func TestClusterOutages_InvalidWindow_Error(t *testing.T) {
	_, err := ClusterOutages(clusterTo, clusterFrom, nil, nil)
	if !errors.Is(err, ErrInvalidWindow) {
		t.Fatalf("err = %v, want ErrInvalidWindow", err)
	}
}

func TestClusterOutages_NoOverlap_NoCluster(t *testing.T) {
	got := mustCluster(t, []EntityOutages{
		outageInput("sensor.a", "a1", "bridge", "", [2]int{10, 20}),
		outageInput("sensor.b", "a2", "bridge", "", [2]int{60, 70}),
	}, hubTopology())
	if len(got.Clusters) != 0 {
		t.Fatalf("clusters = %+v, want none", got.Clusters)
	}
}

func TestClusterOutages_OneEntityRepeating_NotACrossEntityCluster(t *testing.T) {
	got := mustCluster(t, []EntityOutages{
		outageInput("sensor.a", "a1", "bridge", "", [2]int{10, 20}, [2]int{20, 30}),
	}, hubTopology())
	if len(got.Clusters) != 0 {
		t.Fatalf("clusters = %+v, want none — one entity is not a cross-entity cluster", got.Clusters)
	}
}

func TestClusterOutages_OneCleanCluster_AnnotatedWithSharedHub(t *testing.T) {
	got := mustCluster(t, []EntityOutages{
		outageInput("sensor.a1", "a1", "bridge", "kitchen", [2]int{100, 130}),
		outageInput("sensor.a2", "a2", "bridge", "kitchen", [2]int{105, 140}),
		outageInput("sensor.b1", "b1", "bridge", "hall", [2]int{600, 610}),
	}, hubTopology())
	if len(got.Clusters) != 1 {
		t.Fatalf("clusters = %d, want 1", len(got.Clusters))
	}
	c := got.Clusters[0]
	if want := []string{"sensor.a1", "sensor.a2"}; !reflect.DeepEqual(members(c), want) {
		t.Errorf("members = %v, want %v", members(c), want)
	}
	if !c.From.Equal(minute(100)) || !c.To.Equal(minute(140)) {
		t.Errorf("span = %s..%s, want 100..140 min", c.From, c.To)
	}
	for kind, want := range map[model.TraitKind]string{model.TraitViaDevice: "hub_a", model.TraitConfigEntry: "bridge", model.TraitArea: "kitchen"} {
		tr, ok := hasTrait(c.Shared, kind)
		if !ok || tr.Value != want {
			t.Errorf("shared %s = %+v (present %v), want %q", kind, tr, ok, want)
		}
	}
	if _, ok := hasTrait(c.Shared, model.TraitDevice); ok {
		t.Error("members on two devices must not be annotated as sharing a device")
	}
	if len(c.Withheld) != 0 {
		t.Errorf("withheld = %+v, want none — hub_a does not parent the whole entry", c.Withheld)
	}
	ev := c.Evidence
	if ev.Source != "recorder_history" || !ev.From.Equal(clusterFrom) || !ev.To.Equal(clusterTo) {
		t.Errorf("evidence provenance = %q %s..%s", ev.Source, ev.From, ev.To)
	}
	if ev.Measurements["entities"] != 2 || ev.Measurements["outage_periods"] != 2 || ev.Measurements["span_seconds"] != 40*60 {
		t.Errorf("measurements = %v", ev.Measurements)
	}
	if ev.SampleSize != 2 || ev.Coverage != 1 || ev.Degraded {
		t.Errorf("confidence inputs = %d/%v/%v", ev.SampleSize, ev.Coverage, ev.Degraded)
	}
}

func TestClusterOutages_WithinTolerance_Joins_BeyondTolerance_Splits(t *testing.T) {
	gap := int(outageClusterTolerance / time.Minute)
	got := mustCluster(t, []EntityOutages{
		outageInput("sensor.a", "a1", "bridge", "", [2]int{100, 110}),
		outageInput("sensor.b", "a2", "bridge", "", [2]int{110 + gap, 120 + gap}),
		outageInput("sensor.c", "b1", "bridge", "", [2]int{300, 310}),
		outageInput("sensor.d", "hub_b", "bridge", "", [2]int{310 + gap + 1, 320 + gap}),
	}, hubTopology())
	if len(got.Clusters) != 1 {
		t.Fatalf("clusters = %+v, want exactly the a/b one", got.Clusters)
	}
	if want := []string{"sensor.a", "sensor.b"}; !reflect.DeepEqual(members(got.Clusters[0]), want) {
		t.Errorf("members = %v, want %v", members(got.Clusters[0]), want)
	}
}

func TestClusterOutages_TwoClusters_OrderedByTime(t *testing.T) {
	got := mustCluster(t, []EntityOutages{
		outageInput("sensor.late1", "b1", "bridge", "", [2]int{900, 910}),
		outageInput("sensor.early1", "a1", "bridge", "", [2]int{10, 30}),
		outageInput("sensor.late2", "b1", "bridge", "", [2]int{905, 920}),
		outageInput("sensor.early2", "a2", "bridge", "", [2]int{20, 40}),
	}, hubTopology())
	if len(got.Clusters) != 2 {
		t.Fatalf("clusters = %d, want 2", len(got.Clusters))
	}
	if want := []string{"sensor.early1", "sensor.early2"}; !reflect.DeepEqual(members(got.Clusters[0]), want) {
		t.Errorf("first = %v, want %v", members(got.Clusters[0]), want)
	}
	if want := []string{"sensor.late1", "sensor.late2"}; !reflect.DeepEqual(members(got.Clusters[1]), want) {
		t.Errorf("second = %v, want %v", members(got.Clusters[1]), want)
	}
	if tr, ok := hasTrait(got.Clusters[1].Shared, model.TraitDevice); !ok || tr.Value != "b1" {
		t.Errorf("second cluster shared device = %+v, want b1", tr)
	}
	if got.Clusters[0].Evidence.ID == got.Clusters[1].Evidence.ID {
		t.Error("evidence ids must be distinct so a hypothesis can cite one cluster")
	}
}

// TestClusterOutages_CoincidentalOverlap_NoSharedParentClaim: two entities on
// different hubs, different areas, go down together. The time overlap is
// evidence; a topology claim would be invented.
func TestClusterOutages_CoincidentalOverlap_NoSharedParentClaim(t *testing.T) {
	got := mustCluster(t, []EntityOutages{
		outageInput("sensor.a1", "a1", "bridge", "kitchen", [2]int{100, 130}),
		outageInput("sensor.b1", "b1", "bridge", "hall", [2]int{110, 120}),
	}, hubTopology())
	if len(got.Clusters) != 1 {
		t.Fatalf("clusters = %d, want 1 time cluster", len(got.Clusters))
	}
	c := got.Clusters[0]
	for _, kind := range []model.TraitKind{model.TraitDevice, model.TraitViaDevice, model.TraitArea} {
		if tr, ok := hasTrait(c.Shared, kind); ok {
			t.Errorf("coincidental overlap annotated with shared %s %q", kind, tr.Value)
		}
	}
	if _, ok := hasTrait(c.Withheld, model.TraitViaDevice); ok {
		t.Error("different parents are not shared at all, so nothing is withheld")
	}
}

// TestClusterOutages_CoordinatorStar_ViaDeviceWithheld settles F-27: in a
// star every device of the entry names the same parent, so "they share a
// via_device" is true of the whole network and distinguishes nothing. It is
// withheld — named as withheld, so its absence is not read as "different
// parents" — and the config-entry annotation, which says the same thing
// honestly, stays.
func TestClusterOutages_CoordinatorStar_ViaDeviceWithheld(t *testing.T) {
	got := mustCluster(t, []EntityOutages{
		outageInput("switch.plug", "plug", "zigbee", "", [2]int{100, 130}),
		outageInput("light.bulb", "bulb", "zigbee", "", [2]int{101, 129}),
	}, zigbeeStar())
	if len(got.Clusters) != 1 {
		t.Fatalf("clusters = %d, want 1", len(got.Clusters))
	}
	c := got.Clusters[0]
	if tr, ok := hasTrait(c.Shared, model.TraitViaDevice); ok {
		t.Fatalf("star parent %q annotated as shared — vacuous for a coordinator star (F-27)", tr.Value)
	}
	tr, ok := hasTrait(c.Withheld, model.TraitViaDevice)
	if !ok || tr.Value != "coordinator" {
		t.Errorf("withheld = %+v, want via_device coordinator", c.Withheld)
	}
	if tr, ok := hasTrait(c.Shared, model.TraitConfigEntry); !ok || tr.Value != "zigbee" {
		t.Errorf("shared config entry = %+v, want zigbee", tr)
	}
}

// TestClusterOutages_ParentOfPartOfEntry_Shared: a device of the entry that
// names no parent (the parent itself excepted) breaks the star, so naming the
// parent distinguishes some devices from others again.
func TestClusterOutages_ParentOfPartOfEntry_Shared(t *testing.T) {
	devices := append(zigbeeStar(), model.DeviceRef{ID: "direct", ConfigEntryID: "zigbee"})
	got := mustCluster(t, []EntityOutages{
		outageInput("switch.plug", "plug", "zigbee", "", [2]int{100, 130}),
		outageInput("light.bulb", "bulb", "zigbee", "", [2]int{101, 129}),
	}, devices)
	c := got.Clusters[0]
	if tr, ok := hasTrait(c.Shared, model.TraitViaDevice); !ok || tr.Value != "coordinator" {
		t.Errorf("shared via = %+v, want coordinator — it no longer parents the whole entry", tr)
	}
	if len(c.Withheld) != 0 {
		t.Errorf("withheld = %+v, want none", c.Withheld)
	}
}

func TestClusterOutages_EntityAreaOverridesDeviceArea(t *testing.T) {
	devices := []model.DeviceRef{
		{ID: "d1", ConfigEntryID: "e", AreaID: "garage"},
		{ID: "d2", ConfigEntryID: "e", AreaID: "garage"},
	}
	got := mustCluster(t, []EntityOutages{
		outageInput("sensor.a", "d1", "e", "", [2]int{10, 20}),
		outageInput("sensor.b", "d2", "e", "attic", [2]int{12, 22}),
	}, devices)
	if tr, ok := hasTrait(got.Clusters[0].Shared, model.TraitArea); ok {
		t.Errorf("shared area %q, want none — sensor.b's own area is attic", tr.Value)
	}
	got = mustCluster(t, []EntityOutages{
		outageInput("sensor.a", "d1", "e", "", [2]int{10, 20}),
		outageInput("sensor.b", "d2", "e", "", [2]int{12, 22}),
	}, devices)
	if tr, ok := hasTrait(got.Clusters[0].Shared, model.TraitArea); !ok || tr.Value != "garage" {
		t.Errorf("shared area = %+v, want garage inherited from both devices", tr)
	}
}

// TestClusterOutages_UnavailableThroughout_ReportedNotChained: an entity down
// for the whole observed period overlaps everything by construction. Letting
// it chain would merge unrelated outages into one cluster and strip every
// shared annotation; its timing carries no information, so it is reported on
// its own.
func TestClusterOutages_UnavailableThroughout_ReportedNotChained(t *testing.T) {
	dead := outageInput("sensor.dead", "b1", "bridge", "", [2]int{0, 24 * 60})
	dead.Availability.Outages[0].TruncatedStart = true
	dead.Availability.Outages[0].OpenEnded = true
	got := mustCluster(t, []EntityOutages{
		dead,
		outageInput("sensor.a1", "a1", "bridge", "", [2]int{100, 110}),
		outageInput("sensor.a2", "a2", "bridge", "", [2]int{105, 115}),
		outageInput("sensor.x", "b1", "bridge", "", [2]int{900, 910}),
	}, hubTopology())
	if want := []model.EntityID{"sensor.dead"}; !reflect.DeepEqual(got.UnavailableThroughout, want) {
		t.Errorf("throughout = %v, want %v", got.UnavailableThroughout, want)
	}
	if len(got.Clusters) != 1 || !reflect.DeepEqual(members(got.Clusters[0]), []string{"sensor.a1", "sensor.a2"}) {
		t.Errorf("clusters = %+v, want only a1/a2", got.Clusters)
	}
}

func TestClusterOutages_ConfidenceInputs_TakeWeakestMember(t *testing.T) {
	a := outageInput("sensor.a", "a1", "bridge", "", [2]int{10, 20})
	b := outageInput("sensor.b", "a2", "bridge", "", [2]int{12, 22})
	b.Availability.Covered = b.Availability.Window / 4
	b.Degraded = true
	got := mustCluster(t, []EntityOutages{a, b}, hubTopology())
	ev := got.Clusters[0].Evidence
	if ev.Coverage != 0.25 || !ev.Degraded {
		t.Errorf("coverage/degraded = %v/%v, want 0.25/true from the weakest member", ev.Coverage, ev.Degraded)
	}
}

// TestClusterOutages_ShuffledInput_Deterministic runs the same inputs in many
// orders, with devices shuffled too, and requires identical output.
func TestClusterOutages_ShuffledInput_Deterministic(t *testing.T) {
	inputs := []EntityOutages{
		outageInput("sensor.a1", "a1", "bridge", "kitchen", [2]int{100, 130}, [2]int{500, 510}),
		outageInput("sensor.a2", "a2", "bridge", "kitchen", [2]int{105, 140}),
		outageInput("sensor.b1", "b1", "bridge", "hall", [2]int{120, 125}, [2]int{505, 520}),
		outageInput("sensor.hb", "hub_b", "bridge", "hall", [2]int{800, 810}),
	}
	devices := hubTopology()
	want := mustCluster(t, inputs, devices)
	if len(want.Clusters) != 2 {
		t.Fatalf("baseline clusters = %d, want 2", len(want.Clusters))
	}
	rng := rand.New(rand.NewSource(1))
	for i := range 50 {
		in := append([]EntityOutages(nil), inputs...)
		dev := append([]model.DeviceRef(nil), devices...)
		rng.Shuffle(len(in), func(i, j int) { in[i], in[j] = in[j], in[i] })
		rng.Shuffle(len(dev), func(i, j int) { dev[i], dev[j] = dev[j], dev[i] })
		if got := mustCluster(t, in, dev); !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d differs:\n got %+v\nwant %+v", i, got, want)
		}
	}
}

// TestSweepClusters_LinearInWindows counts every overlap test the sweep
// makes. A pairwise design (D-05-3, rejected) makes n(n-1)/2; the sweep over
// sorted windows makes at most one per window after the first.
func TestSweepClusters_LinearInWindows(t *testing.T) {
	for _, n := range []int{1, 10, 1000} {
		windows := make([]window, n)
		for i := range windows {
			// Windows chain in threes, then a gap starts the next group.
			start := minute(i*5 + (i/3)*60)
			windows[i] = window{entity: model.EntityID("sensor.x"), from: start, to: start.Add(3 * time.Minute)}
		}
		calls := 0
		counting := func(end time.Time, w window) bool {
			calls++
			return joins(end, w)
		}
		sweepClusters(windows, counting)
		if calls > n-1 && n > 0 {
			t.Errorf("n=%d: %d overlap tests, want at most %d", n, calls, n-1)
		}
	}
}

// TestOutageCluster_SerializedForm_HasNoCausalField marshals a fully
// annotated cluster and walks every key: an outage cluster is evidence, and
// nothing in its serialized form may read as a cause (D-05-1, ADR-010).
func TestOutageCluster_SerializedForm_HasNoCausalField(t *testing.T) {
	got := mustCluster(t, []EntityOutages{
		outageInput("switch.plug", "plug", "zigbee", "den", [2]int{100, 130}),
		outageInput("light.bulb", "plug", "zigbee", "den", [2]int{101, 129}),
	}, zigbeeStar())
	if len(got.Clusters) != 1 || len(got.Clusters[0].Shared) == 0 || len(got.Clusters[0].Withheld) == 0 {
		t.Fatalf("fixture must exercise shared and withheld traits: %+v", got.Clusters)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range jsonKeys(decoded) {
		lower := strings.ToLower(key)
		for _, banned := range []string{"cause", "culprit", "blame", "because", "root"} {
			if strings.Contains(lower, banned) {
				t.Errorf("serialized cluster carries key %q — a cluster is evidence, not a cause", key)
			}
		}
	}
}

func jsonKeys(v any) []string {
	var keys []string
	switch v := v.(type) {
	case map[string]any:
		for k, child := range v {
			keys = append(keys, k)
			keys = append(keys, jsonKeys(child)...)
		}
	case []any:
		for _, child := range v {
			keys = append(keys, jsonKeys(child)...)
		}
	}
	return keys
}
