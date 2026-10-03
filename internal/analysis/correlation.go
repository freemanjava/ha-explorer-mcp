package analysis

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// outageClusterTolerance is how far apart two unavailable windows may sit and
// still count as overlapping (D-05-3). It absorbs clock and polling skew: an
// integration polling every 30–60 s marks its entities unavailable up to one
// interval apart for the same upstream event, and the recorder commits on its
// own interval on top of that. Two minutes covers both twice over without
// joining outages a human would call separate. A starting default, not a
// measurement (doc §26); P5-10 is where it gets revisited.
const outageClusterTolerance = 2 * time.Minute

// clusterSource is the recorder API every input window was measured from.
const clusterSource = "recorder_history"

// EntityOutages is one entity's input to ClusterOutages: its registry entry,
// for what it shares with others, and its availability over the clustering
// window, for when it was down. Degraded marks a history read that answered
// only partly.
type EntityOutages struct {
	Entity       model.Entity
	Availability AvailabilityReport
	Degraded     bool
}

// TraitKind names a registry property cluster members can share.
type TraitKind string

const (
	TraitDevice      TraitKind = "device"
	TraitViaDevice   TraitKind = "via_device"
	TraitConfigEntry TraitKind = "config_entry"
	TraitArea        TraitKind = "area"
)

// traitOrder fixes the order traits are listed in, so output is deterministic.
var traitOrder = [...]TraitKind{TraitDevice, TraitViaDevice, TraitConfigEntry, TraitArea}

// SharedTrait is a registry value every member of a cluster has in common. It
// is evidence about the cluster, never a claim that the shared thing made the
// members go down (D-05-3).
type SharedTrait struct {
	Kind  TraitKind
	Value string
}

// OutageCluster is a set of entities whose unavailable windows overlapped
// within outageClusterTolerance. Shared lists what every member has in
// common; a trait not listed is either not shared or not known, never a claim
// that members differ. Withheld lists traits that are shared but true of so
// much of the installation that naming them would distinguish nothing (F-27);
// they are named so their absence from Shared cannot be misread.
type OutageCluster struct {
	From    time.Time
	To      time.Time
	Members []model.EntityID

	Shared   []SharedTrait
	Withheld []SharedTrait

	// Evidence is the cluster as a citable measurement. Its SampleSize is the
	// number of outage windows clustered; Coverage and Degraded are the
	// weakest member's, since the cluster is only as observed as its least
	// observed member.
	Evidence model.Evidence
}

// OutageClustering is ClusterOutages' result. UnavailableThroughout lists
// entities down for the whole of their observed period: such a window
// overlaps every other by construction, so it carries no timing information
// and would chain unrelated outages into one cluster. It is reported here
// rather than clustered.
type OutageClustering struct {
	Clusters              []OutageCluster
	UnavailableThroughout []model.EntityID
}

// window is one outage of one entity, the unit the sweep orders and joins.
type window struct {
	entity model.EntityID
	from   time.Time
	to     time.Time
}

// ClusterOutages groups entities whose unavailable windows overlap, then
// annotates each group with what its members share (D-05-3). Grouping is by
// time alone; topology is consulted only afterwards, so a cross-integration
// outage is found even when nothing explains it. devices is the device
// registry, used to resolve each member's via_device parent, inherited area,
// and whether a parent is a star over its whole config entry (F-27).
//
// Cost is a sort plus one linear sweep over all windows — never a comparison
// of every entity against every other. Output is deterministic for a given
// input regardless of the order inputs or devices arrive in.
func ClusterOutages(from, to time.Time, inputs []EntityOutages, devices []model.DeviceRef) (OutageClustering, error) {
	if !to.After(from) {
		return OutageClustering{}, fmt.Errorf("%w: from=%s to=%s",
			ErrInvalidWindow, from.Format(time.RFC3339), to.Format(time.RFC3339))
	}

	byEntity := make(map[model.EntityID]EntityOutages, len(inputs))
	var windows []window
	var throughout []model.EntityID
	for _, in := range inputs {
		byEntity[in.Entity.ID] = in
		for _, o := range in.Availability.Outages {
			if o.TruncatedStart && o.OpenEnded {
				throughout = append(throughout, in.Entity.ID)
				continue
			}
			windows = append(windows, window{entity: in.Entity.ID, from: o.From, to: o.To})
		}
	}
	slices.SortFunc(windows, compareWindows)

	topo := newTopology(devices)
	var res OutageClustering
	for _, group := range sweepClusters(windows, joins) {
		c, ok := buildCluster(group, byEntity, topo)
		if !ok {
			continue
		}
		c.Evidence.ID = model.EvidenceID(fmt.Sprintf("outage_cluster_%d", len(res.Clusters)+1))
		c.Evidence.From, c.Evidence.To = from, to
		res.Clusters = append(res.Clusters, c)
	}
	slices.Sort(throughout)
	res.UnavailableThroughout = slices.Compact(throughout)
	return res, nil
}

func compareWindows(a, b window) int {
	if c := a.from.Compare(b.from); c != 0 {
		return c
	}
	if c := a.to.Compare(b.to); c != 0 {
		return c
	}
	return strings.Compare(string(a.entity), string(b.entity))
}

// joins reports whether w, the next window in start order, overlaps a group
// reaching up to end, within the tolerance.
func joins(end time.Time, w window) bool {
	return !w.from.After(end.Add(outageClusterTolerance))
}

// sweepClusters splits start-ordered windows into maximal overlapping groups,
// making one overlap test per window after the first. overlaps is a parameter
// so a test can count the tests made.
func sweepClusters(sorted []window, overlaps func(end time.Time, w window) bool) [][]window {
	if len(sorted) == 0 {
		return nil
	}
	var groups [][]window
	start, end := 0, sorted[0].to
	for i := 1; i < len(sorted); i++ {
		w := sorted[i]
		if overlaps(end, w) {
			if w.to.After(end) {
				end = w.to
			}
			continue
		}
		groups = append(groups, sorted[start:i])
		start, end = i, w.to
	}
	return append(groups, sorted[start:])
}

// buildCluster turns one time group into a cluster, or reports false when the
// group holds a single entity — one entity flapping is not a cross-entity
// outage.
func buildCluster(group []window, byEntity map[model.EntityID]EntityOutages, topo topology) (OutageCluster, bool) {
	c := OutageCluster{From: group[0].from, To: group[0].to}
	for _, w := range group {
		if w.to.After(c.To) {
			c.To = w.to
		}
		c.Members = append(c.Members, w.entity)
	}
	slices.Sort(c.Members)
	c.Members = slices.Compact(c.Members)
	if len(c.Members) < 2 {
		return OutageCluster{}, false
	}

	members := make([]EntityOutages, len(c.Members))
	for i, id := range c.Members {
		members[i] = byEntity[id]
	}
	c.Shared, c.Withheld = annotate(members, topo)
	c.Evidence = clusterEvidence(c, len(group), members)
	return c, true
}

func clusterEvidence(c OutageCluster, windows int, members []EntityOutages) model.Evidence {
	ev := model.Evidence{
		Observation: "unavailable periods overlapping across entities",
		Source:      clusterSource,
		Measurements: map[string]float64{
			"entities":       float64(len(c.Members)),
			"outage_periods": float64(windows),
			"span_seconds":   c.To.Sub(c.From).Seconds(),
		},
		SampleSize: windows,
		Coverage:   1,
	}
	for _, m := range members {
		ev.Coverage = min(ev.Coverage, coverageOf(m.Availability))
		ev.Degraded = ev.Degraded || m.Degraded
	}
	return ev
}

// coverageOf is the fraction of the report's window its history covered.
func coverageOf(r AvailabilityReport) float64 {
	if !r.Computable || r.Window <= 0 {
		return 0
	}
	return float64(r.Covered) / float64(r.Window)
}

// annotate returns the traits every member shares, split into those worth
// naming and those withheld as vacuous.
func annotate(members []EntityOutages, topo topology) (shared, withheld []SharedTrait) {
	for _, kind := range traitOrder {
		value, ok := commonValue(members, func(m EntityOutages) string { return topo.trait(kind, m.Entity) })
		if !ok {
			continue
		}
		tr := SharedTrait{Kind: kind, Value: value}
		if kind == TraitViaDevice && topo.vacuousParent(model.DeviceID(value), members) {
			withheld = append(withheld, tr)
			continue
		}
		shared = append(shared, tr)
	}
	return shared, withheld
}

// commonValue returns the value every member has, if it is non-empty and the
// same for all. An unknown value on any member means "not known to be
// shared", never "shared as empty".
func commonValue(members []EntityOutages, of func(EntityOutages) string) (string, bool) {
	first := of(members[0])
	if first == "" {
		return "", false
	}
	for _, m := range members[1:] {
		if of(m) != first {
			return "", false
		}
	}
	return first, true
}

// topology is the device registry indexed for annotation.
type topology struct {
	devices map[model.DeviceID]model.DeviceRef
	// entrySize counts devices per config entry; childCount counts, per
	// entry, the devices naming a given parent.
	entrySize  map[model.ConfigEntryID]int
	childCount map[entryParent]int
}

type entryParent struct {
	entry  model.ConfigEntryID
	parent model.DeviceID
}

func newTopology(devices []model.DeviceRef) topology {
	t := topology{
		devices:    make(map[model.DeviceID]model.DeviceRef, len(devices)),
		entrySize:  make(map[model.ConfigEntryID]int),
		childCount: make(map[entryParent]int),
	}
	for _, d := range devices {
		t.devices[d.ID] = d
		t.entrySize[d.ConfigEntryID]++
		if d.ViaDeviceID != "" {
			t.childCount[entryParent{d.ConfigEntryID, d.ViaDeviceID}]++
		}
	}
	return t
}

// trait resolves one entity's value for kind. An entity's own area overrides
// its device's, as it does in HA.
func (t topology) trait(kind TraitKind, e model.Entity) string {
	switch kind {
	case TraitDevice:
		return string(e.DeviceID)
	case TraitViaDevice:
		return string(t.devices[e.DeviceID].ViaDeviceID)
	case TraitConfigEntry:
		return string(e.ConfigEntryID)
	case TraitArea:
		if e.AreaID != "" {
			return string(e.AreaID)
		}
		return string(t.devices[e.DeviceID].AreaID)
	}
	return ""
}

// vacuousParent settles F-27. A parent is vacuous when, in the config entry
// of every member's device, every device other than the parent itself names
// it — the coordinator/bridge star ZHA and Zigbee2MQTT both build. Sharing
// such a parent says no more than sharing the config entry, which is already
// annotated, while sounding like a topology finding. A parent of only part of
// its entry (a hub among several, or beside directly attached devices) still
// distinguishes, so it is named. The rule reads only registry structure,
// never an integration name (CLAUDE.md rule 6).
func (t topology) vacuousParent(parent model.DeviceID, members []EntityOutages) bool {
	for _, m := range members {
		entry := t.devices[m.Entity.DeviceID].ConfigEntryID
		others := t.entrySize[entry]
		if p, known := t.devices[parent]; known && p.ConfigEntryID == entry {
			others--
		}
		if t.childCount[entryParent{entry, parent}] < others {
			return false
		}
	}
	return true
}
