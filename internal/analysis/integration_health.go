package analysis

import (
	"slices"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// Evidence ids AnalyzeIntegrationHealth adds to the entity analysis's set.
// Cluster evidence keeps ClusterOutages' own ids.
const (
	EvidenceInventory             model.EvidenceID = "inventory"
	EvidenceUnavailableThroughout model.EvidenceID = "unavailable_throughout"
	EvidenceSupervisor            model.EvidenceID = "supervisor_resolution"
)

const (
	supervisorSource = "supervisor"

	// restartTolerance is how far a core lifecycle row may sit from an outage
	// cluster's onset and still count as coinciding with it. A restart takes
	// seconds to stop and one to three minutes to bring integrations back, and
	// the recorder commits on its own interval on top, so five minutes covers a
	// slow Raspberry Pi start without reaching a separate event. A starting
	// default, not a measurement (doc §26).
	restartTolerance = 5 * time.Minute

	// majorityUnavailableRatio is the share of an integration's entities
	// currently unavailable at which "most of it is down" is worth ranking.
	// Half is the plain meaning of "most"; a starting default (doc §26),
	// revisited by P5-10.
	majorityUnavailableRatio = 0.5
)

// IntegrationHealthInput is everything AnalyzeIntegrationHealth composes,
// already read. As with EntityHealthInput, analysis never fetches, and each
// source carries a flag saying whether it was read so an unread source is
// never mistaken for an empty one (rule 7).
type IntegrationHealthInput struct {
	Entry      model.Integration
	ObservedAt time.Time
	From, To   time.Time

	// EntityCount and DeviceCount come from the registries.
	EntityCount, DeviceCount int

	// UnavailableRead says the live state answered; UnavailableNow is
	// meaningless otherwise.
	UnavailableRead bool
	UnavailableNow  int

	// OutagesRead says recorder history answered for the entities in
	// Outages. Devices are the entry's, for annotating clusters.
	OutagesRead bool
	Outages     []EntityOutages
	Devices     []model.DeviceRef

	RepairsRead bool
	Repairs     []model.Repair

	// MeshEvidence is the mesh-metric evidence already built from read
	// history (MeshEvidence). Evidence only: nothing here becomes a hypothesis
	// (D-05-9). Metrics that could not be read arrive through Missing.
	MeshEvidence []model.Evidence
	// MeshResolved says at least one link-quality or signal-strength entity
	// resolved for an affected device, whether or not its history was readable.
	MeshResolved bool

	// RestartProbes are the logbook windows read around cluster onsets. A
	// cluster with no probe was not checked, which is not the same as no
	// restart (rule 7): the caller names it in Missing.
	RestartProbes []RestartProbe

	// SupervisorRead says Supervisor's resolution summary answered.
	SupervisorRead bool
	Resolution     model.ResolutionSummary

	// Missing carries the sources the caller could not read, with why.
	Missing []model.MissingEvidence
}

// RestartProbe is one logbook window read around an outage cluster's onset:
// every core lifecycle row found in [Onset-restartTolerance,
// Onset+restartTolerance]. An empty Events is a read that found none. Onset is
// the cluster's own start, which is how analysis matches a probe to a cluster
// without sharing ids with the caller.
type RestartProbe struct {
	Onset  time.Time
	Events []model.LifecycleEvent
}

// RestartProbeWindow is the window a probe for a cluster starting at onset
// must cover, so the reader and the analysis agree on it.
func RestartProbeWindow(onset time.Time) (from, to time.Time) {
	return onset.Add(-restartTolerance), onset.Add(restartTolerance)
}

// OutageOnsets are the start times of the outage clusters ClusterOutages would
// report for the input, in cluster order. The caller probes the logbook around
// each; it is the same deterministic clustering, so the probes line up.
func OutageOnsets(from, to time.Time, outages []EntityOutages, devices []model.DeviceRef) ([]time.Time, error) {
	res, err := ClusterOutages(from, to, outages, devices)
	if err != nil {
		return nil, err
	}
	onsets := make([]time.Time, len(res.Clusters))
	for i, c := range res.Clusters {
		onsets[i] = c.From
	}
	return onsets, nil
}

// AnalyzeIntegrationHealth composes setup state, entity/device counts, the
// unavailable ratio, open repairs, outage clusters restricted to the
// integration's entities, and Supervisor's host health into Appendix A.3's
// shape. There is no score (D-05-4). A source that is missing lowers what can
// be concluded; it never fails the call (doc §3.2).
func AnalyzeIntegrationHealth(in IntegrationHealthInput) (model.HealthAnalysis, error) {
	b := integrationBuilder{in: in, missing: slices.Clone(in.Missing)}
	if err := b.addClusterEvidence(); err != nil {
		return model.HealthAnalysis{}, err
	}
	b.addInventoryEvidence()
	b.addStateEvidence()
	b.addRepairEvidence()
	b.addRestartEvidence()
	b.evidence = append(b.evidence, b.in.MeshEvidence...)
	b.addSupervisorEvidence()
	b.addUnreachableEvidence()
	b.addHypotheses()
	b.rankHypotheses()

	out := model.HealthAnalysis{
		SubjectID:       string(in.Entry.ID),
		ObservedAt:      in.ObservedAt,
		From:            in.From,
		To:              in.To,
		Evidence:        b.evidence,
		Clusters:        b.annotations,
		Hypotheses:      b.hypotheses,
		MissingEvidence: b.missing,
		NextActions:     b.nextActions(),
	}
	if len(b.missing) > 0 {
		out.Partial = true
		out.PartialReason = "one or more evidence sources could not be observed; see missing_evidence"
	}
	return out, nil
}

type integrationBuilder struct {
	ledger
	in      IntegrationHealthInput
	missing []model.MissingEvidence

	clusters    []model.EvidenceID
	onsets      map[model.EvidenceID]time.Time
	restartLegs map[model.EvidenceID]model.EvidenceID
	annotations []model.ClusterAnnotation
	loaded      bool
}

func (b *integrationBuilder) addClusterEvidence() error {
	if !b.in.OutagesRead {
		return nil
	}
	res, err := ClusterOutages(b.in.From, b.in.To, b.in.Outages, b.in.Devices)
	if err != nil {
		return err
	}
	for _, c := range res.Clusters {
		b.evidence = append(b.evidence, c.Evidence)
		b.clusters = append(b.clusters, c.Evidence.ID)
		if b.onsets == nil {
			b.onsets = map[model.EvidenceID]time.Time{}
		}
		b.onsets[c.Evidence.ID] = c.From
		b.annotations = append(b.annotations, model.ClusterAnnotation{
			Evidence: c.Evidence.ID,
			Members:  c.Members,
			Shared:   c.Shared,
			Withheld: c.Withheld,
		})
	}
	if n := len(res.UnavailableThroughout); n > 0 {
		b.evidence = append(b.evidence, model.Evidence{
			ID:           EvidenceUnavailableThroughout,
			Observation:  "entities unavailable for the whole observed period, which no outage window can time",
			Source:       recorderSource,
			From:         b.in.From,
			To:           b.in.To,
			Measurements: map[string]float64{"entities": float64(n)},
			SampleSize:   n,
			Coverage:     1,
		})
	}
	return nil
}

// addRestartEvidence turns each cluster's logbook probe into a measurement of
// how many core lifecycle rows sat near its onset. A probe that found none is
// still evidence — "no restart near this onset" — while a cluster with no probe
// gets nothing here, so an unread window is never reported as a clean one.
func (b *integrationBuilder) addRestartEvidence() {
	for _, id := range b.clusters {
		probe, ok := b.probeFor(b.onsets[id])
		if !ok {
			continue
		}
		from, to := RestartProbeWindow(probe.Onset)
		leg := model.EvidenceID("restart_" + string(id))
		if b.restartLegs == nil {
			b.restartLegs = map[model.EvidenceID]model.EvidenceID{}
		}
		b.restartLegs[id] = leg
		b.evidence = append(b.evidence, model.Evidence{
			ID:           leg,
			Observation:  "Home Assistant core start/stop log rows near the outage cluster's onset",
			Source:       logbookSource,
			From:         from,
			To:           to,
			Measurements: map[string]float64{"lifecycle_events": float64(len(probe.Events))},
			SampleSize:   max(len(probe.Events), 1),
			Coverage:     1,
		})
	}
}

func (b *integrationBuilder) probeFor(onset time.Time) (RestartProbe, bool) {
	for _, p := range b.in.RestartProbes {
		if p.Onset.Equal(onset) {
			return p, true
		}
	}
	return RestartProbe{}, false
}

// restartCoincides reports whether the cluster's probe found a lifecycle row.
func (b *integrationBuilder) restartCoincides(cluster model.EvidenceID) bool {
	leg, ok := b.restartLegs[cluster]
	if !ok {
		return false
	}
	ev, _ := b.evidenceByID(leg)
	return ev.Measurements["lifecycle_events"] > 0
}

// addUnreachableEvidence names what no source of this server can read, once
// the response has something that would use it. Rows only: missing evidence
// informs, it never refutes a hypothesis, so addHypotheses is unaffected.
func (b *integrationBuilder) addUnreachableEvidence() {
	if len(b.clusters) > 0 {
		b.missing = append(b.missing, model.MissingEvidence{
			What: "kernel and host logs around the clustered outage window (USB resets, dmesg)", Source: "host",
			Reason: model.MissingPrivileged,
			Detail: "needs host privileges this server never has; ADR-012's separate Host Probe would read it",
		})
	}
	if b.in.MeshResolved {
		b.missing = append(b.missing, model.MissingEvidence{
			What: "mesh neighbour/routing table", Source: "integration",
			Reason: model.MissingNotExposed,
			Detail: "the integration's topology sources lie outside this server's read-only gateway allow-list",
		})
	}
}

func (b *integrationBuilder) addInventoryEvidence() {
	m := map[string]float64{
		"entities": float64(b.in.EntityCount),
		"devices":  float64(b.in.DeviceCount),
	}
	if b.in.UnavailableRead {
		m["unavailable_entities"] = float64(b.in.UnavailableNow)
		if b.in.EntityCount > 0 {
			m["unavailable_ratio"] = float64(b.in.UnavailableNow) / float64(b.in.EntityCount)
		}
	}
	b.evidence = append(b.evidence, model.Evidence{
		ID:           EvidenceInventory,
		Observation:  "entity and device counts for the integration",
		Source:       coreSource,
		From:         b.in.ObservedAt,
		To:           b.in.ObservedAt,
		Measurements: m,
		SampleSize:   b.in.EntityCount,
		Coverage:     1,
	})
}

func (b *integrationBuilder) addStateEvidence() {
	ev := setupStateEvidence(&b.in.Entry, b.in.ObservedAt)
	b.evidence = append(b.evidence, ev)
	b.loaded = ev.Measurements["loaded"] == 1
}

func (b *integrationBuilder) addRepairEvidence() {
	if !b.in.RepairsRead {
		return
	}
	open := 0
	for _, r := range b.in.Repairs {
		if r.Domain == b.in.Entry.Domain && !r.Ignored {
			open++
		}
	}
	if open == 0 {
		return
	}
	b.evidence = append(b.evidence, model.Evidence{
		ID:           EvidenceRepairs,
		Observation:  "open repairs for the integration",
		Source:       coreSource,
		From:         b.in.ObservedAt,
		To:           b.in.ObservedAt,
		Measurements: map[string]float64{"open_repairs": float64(open)},
		SampleSize:   open,
		Coverage:     1,
	})
}

// addSupervisorEvidence records host-level health. Only counts are kept: the
// condition names are Supervisor-supplied text and stay out of the response.
func (b *integrationBuilder) addSupervisorEvidence() {
	if !b.in.SupervisorRead {
		return
	}
	r := b.in.Resolution
	b.evidence = append(b.evidence, model.Evidence{
		ID:          EvidenceSupervisor,
		Observation: "Supervisor's host health summary",
		Source:      supervisorSource,
		From:        b.in.ObservedAt,
		To:          b.in.ObservedAt,
		Measurements: map[string]float64{
			"issues":                 float64(r.IssueCount),
			"unhealthy_conditions":   float64(len(r.Unhealthy)),
			"unsupported_conditions": float64(len(r.Unsupported)),
		},
		SampleSize: 1,
		Coverage:   1,
	})
}

// showsProblem reports whether anything observed here a repair or a host
// condition could be offered as an explanation for. Without it those are
// context, not hypotheses: a repair beside a healthy integration explains
// nothing.
func (b *integrationBuilder) showsProblem() bool {
	return !b.loaded || len(b.clusters) > 0 || b.majorityUnavailable()
}

func (b *integrationBuilder) majorityUnavailable() bool {
	inv, ok := b.evidenceByID(EvidenceInventory)
	return ok && b.in.EntityCount >= 2 && inv.Measurements["unavailable_ratio"] >= majorityUnavailableRatio
}

func (b *integrationBuilder) addHypotheses() {
	if !b.loaded {
		b.cite("the integration's setup is not in a loaded state, which may explain its entities not reporting",
			EvidenceIntegrationState, EvidenceInventory)
	}
	if b.majorityUnavailable() {
		b.cite("most of the integration's entities are unavailable now, which may point to the integration or its source rather than to individual devices",
			EvidenceInventory)
	}
	for _, id := range b.clusters {
		b.citeCluster(id)
	}
	if _, ok := b.evidenceByID(EvidenceUnavailableThroughout); ok {
		b.cite("some of the integration's entities were unavailable for the whole period, which may point to a persistent fault rather than an intermittent one",
			EvidenceUnavailableThroughout)
	}
	if !b.showsProblem() {
		return
	}
	if _, ok := b.evidenceByID(EvidenceRepairs); ok {
		b.cite("an open repair for the integration may relate to its unavailable entities",
			append([]model.EvidenceID{EvidenceRepairs}, b.problemIDs()...)...)
	}
	if r := b.in.Resolution; b.in.SupervisorRead && len(r.Unhealthy)+len(r.Unsupported) > 0 {
		b.cite("Supervisor reports unhealthy or unsupported host conditions, which may affect the integration",
			append([]model.EvidenceID{EvidenceSupervisor}, b.problemIDs()...)...)
	}
}

// citeCluster ranks what a cluster may mean. A core start/stop row at its onset
// is enough on its own to account for entities going unavailable together —
// a restart takes every integration's entities down at once — so the
// shared-upstream hypothesis is not offered beside it; the integration may
// still have failed, and the statement says only that a restart may explain
// the timing. With the logbook read and no such row, the upstream hypothesis
// stands on stronger ground and cites that absence.
func (b *integrationBuilder) citeCluster(id model.EvidenceID) {
	leg := b.restartLegs[id]
	if b.restartCoincides(id) {
		b.cite("several of the integration's entities became unavailable together at a Home Assistant start or stop, which may mean the restart rather than the integration caused it",
			id, leg)
		return
	}
	b.cite("several of the integration's entities became unavailable together, which may point to a shared upstream connection",
		id, leg)
}

// problemIDs are the evidence legs that show something went wrong.
func (b *integrationBuilder) problemIDs() []model.EvidenceID {
	ids := slices.Clone(b.clusters)
	if !b.loaded {
		ids = append(ids, EvidenceIntegrationState)
	}
	if b.majorityUnavailable() {
		ids = append(ids, EvidenceInventory)
	}
	return ids
}

// nextActions recommends the catalog tool that would deepen each finding.
// These are recommendations, never observations.
func (b *integrationBuilder) nextActions() []model.NextAction {
	var actions []model.NextAction
	if !b.loaded {
		actions = append(actions, model.NextAction{
			Step: "check the integration's setup state and reason",
			Tool: "get_integration",
		})
	}
	if len(b.clusters) > 0 {
		actions = append(actions, model.NextAction{
			Step: "inspect the raw state history of the clustered entities around the shared window",
			Tool: "get_entity_history",
		})
	}
	if len(b.clusters) > 0 {
		actions = append(actions, model.NextAction{
			Step: "read the host's kernel log around the shared window for USB resets; no tool of this server can",
		})
	}
	if _, ok := b.evidenceByID(EvidenceRepairs); ok {
		actions = append(actions, model.NextAction{
			Step: "read the open repairs for the integration",
			Tool: "list_repairs",
		})
	}
	if ev, ok := b.evidenceByID(EvidenceSupervisor); ok &&
		ev.Measurements["unhealthy_conditions"]+ev.Measurements["unsupported_conditions"] > 0 {
		actions = append(actions, model.NextAction{
			Step: "read Supervisor's host health",
			Tool: "get_system_health",
		})
	}
	return actions
}
