package analysis

import (
	"slices"
	"sort"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// Evidence ids AnalyzeEntityHealth assigns. They are stable so a hypothesis
// and a test can cite one by name; they are analysis-authored, never derived
// from HA data (CLAUDE.md rule 6).
const (
	EvidenceAvailability     model.EvidenceID = "availability"
	EvidenceCadence          model.EvidenceID = "cadence"
	EvidenceIntegrationState model.EvidenceID = "integration_state"
	EvidenceRepairs          model.EvidenceID = "repairs"
)

const (
	// recorderSource and coreSource name the HA subsystems the evidence was
	// read from, as Health.Source does.
	recorderSource = "recorder_history"
	coreSource     = "home_assistant_core"

	// repeatedOutageThreshold is how many separate unavailable periods make
	// "dropped out repeatedly" a claim worth ranking: one outage is an event,
	// two could still be coincidence, three is a pattern. A starting default
	// (doc §26), revisited by P5-10.
	repeatedOutageThreshold = 3

	setupStateLoaded = "loaded"
)

// knownSetupStates is the config-entry state enum HA documents. A state
// outside it is HA-supplied text and is never echoed into a response or
// branched on (rule 6): it is reported as unrecognized.
var knownSetupStates = []string{
	setupStateLoaded, "setup_error", "setup_retry", "not_loaded",
	"failed_unload", "migration_error", "setup_in_progress",
}

// EntityHealthInput is everything AnalyzeEntityHealth composes, already read.
// Analysis never fetches (CLAUDE.md, Module Layout), so each source arrives
// with a flag saying whether it was read: a source that was not read is
// reported in Missing by the caller, and an unread source is never mistaken
// for an empty one (rule 7).
type EntityHealthInput struct {
	EntityID   model.EntityID
	ObservedAt time.Time
	From, To   time.Time

	HistoryRead bool
	Points      []model.HistoryPoint

	// RegistryRead says the entity/device/config-entry registries answered.
	// Entity, Device and ConfigEntry are nil when read but not found.
	RegistryRead bool
	Entity       *model.Entity
	Device       *model.DeviceRef
	ConfigEntry  *model.Integration

	RepairsRead bool
	Repairs     []model.Repair

	// Missing carries the sources the caller could not read, with why.
	Missing []model.MissingEvidence
}

// AnalyzeEntityHealth composes availability, cadence, registry context, the
// owning integration's setup state and related repairs into Appendix A.3's
// shape: Evidence, ranked Hypotheses whose confidence comes only from
// ConfidenceFor, and MissingEvidence. There is no score (D-05-4). A source
// that is missing lowers what can be concluded; it never fails the call
// (doc §3.2).
func AnalyzeEntityHealth(in EntityHealthInput) (model.HealthAnalysis, error) {
	a := analysisBuilder{in: in, missing: slices.Clone(in.Missing)}
	if err := a.addHistoryEvidence(); err != nil {
		return model.HealthAnalysis{}, err
	}
	a.noteRegistryGaps()
	a.addIntegrationEvidence()
	a.addRepairEvidence()
	a.rankHypotheses()

	out := model.HealthAnalysis{
		SubjectID:       string(in.EntityID),
		ObservedAt:      in.ObservedAt,
		From:            in.From,
		To:              in.To,
		Evidence:        a.evidence,
		Hypotheses:      a.hypotheses,
		MissingEvidence: a.missing,
		NextActions:     a.nextActions(),
	}
	if len(a.missing) > 0 {
		out.Partial = true
		out.PartialReason = "one or more evidence sources could not be observed; see missing_evidence"
	}
	return out, nil
}

// ledger holds the evidence an analysis has produced and the hypotheses citing
// it, shared by the entity and integration analyses.
type ledger struct {
	evidence   []model.Evidence
	hypotheses []model.Hypothesis
}

type analysisBuilder struct {
	ledger
	in      EntityHealthInput
	missing []model.MissingEvidence

	avail   AvailabilityReport
	cadence CadenceReport
}

func (a *analysisBuilder) addHistoryEvidence() error {
	if !a.in.HistoryRead {
		return nil
	}
	avail, err := ComputeAvailability(a.in.From, a.in.To, a.in.Points)
	if err != nil {
		return err
	}
	cadence, err := ComputeCadence(a.in.From, a.in.To, a.in.Points)
	if err != nil {
		return err
	}
	a.avail, a.cadence = avail, cadence

	if !avail.Computable {
		a.noteEmptyHistory()
		return nil
	}
	coverage := coverageOf(avail)
	a.evidence = append(a.evidence, model.Evidence{
		ID:          EvidenceAvailability,
		Observation: "availability over the period",
		Source:      recorderSource,
		From:        a.in.From,
		To:          a.in.To,
		Measurements: map[string]float64{
			"availability_ratio":        avail.AvailabilityRatio,
			"unavailable_periods":       float64(avail.UnavailablePeriods),
			"total_unavailable_seconds": avail.TotalUnavailable.Seconds(),
			"longest_unavailable_secs":  avail.LongestUnavailable.Seconds(),
			"state_changes":             float64(avail.StateChanges),
		},
		SampleSize: len(a.in.Points),
		Coverage:   coverage,
	})
	if cadence.Computable {
		a.evidence = append(a.evidence, cadenceEvidence(a.in, cadence, coverage))
	}
	a.addOutageHypotheses()
	a.addStaleHypothesis()
	return nil
}

func cadenceEvidence(in EntityHealthInput, c CadenceReport, coverage float64) model.Evidence {
	stale := 0.0
	if c.StaleJudgeable && c.Stale {
		stale = 1
	}
	return model.Evidence{
		ID:          EvidenceCadence,
		Observation: "update cadence over the period",
		Source:      recorderSource,
		From:        in.From,
		To:          in.To,
		Measurements: map[string]float64{
			"median_interval_seconds": c.MedianUpdateInterval.Seconds(),
			"p95_interval_seconds":    c.P95UpdateInterval.Seconds(),
			"silent_for_seconds":      c.SilentFor.Seconds(),
			"stale":                   stale,
		},
		SampleSize: c.Intervals,
		Coverage:   coverage,
	}
}

// noteEmptyHistory explains an empty recorder answer. A disabled entity
// records nothing by definition, which is a different next step from a
// recorder that does not reach back far enough.
func (a *analysisBuilder) noteEmptyHistory() {
	if a.in.Entity != nil && a.in.Entity.DisabledBy != "" {
		a.missing = append(a.missing, model.MissingEvidence{
			What:   "entity history",
			Source: recorderSource,
			Reason: model.MissingEntityDisabled,
			Detail: "the entity is disabled in the registry, so it records no states",
		})
		return
	}
	a.missing = append(a.missing, model.MissingEvidence{
		What:   "entity history",
		Source: recorderSource,
		Reason: model.MissingOutOfRetention,
		Detail: "the recorder holds no state for this entity at or before the end of the period",
	})
}

func (a *analysisBuilder) noteRegistryGaps() {
	if !a.in.RegistryRead || a.in.Entity != nil && a.in.ConfigEntry != nil {
		return
	}
	detail := "the entity has no entity-registry entry, so device and integration context is unavailable"
	if a.in.Entity != nil {
		detail = "the entity's registry entry names no readable config entry"
	}
	a.missing = append(a.missing, model.MissingEvidence{
		What:   "device and integration context",
		Source: coreSource,
		Reason: model.MissingNotExposed,
		Detail: detail,
	})
}

func (a *analysisBuilder) addIntegrationEvidence() {
	entry := a.in.ConfigEntry
	if entry == nil {
		return
	}
	ev := setupStateEvidence(entry, a.in.ObservedAt)
	a.evidence = append(a.evidence, ev)
	loaded := ev.Measurements["loaded"]
	if loaded == 1 || !a.showsProblem() {
		return
	}
	a.cite("the integration's setup is not in a loaded state, which may explain the entity not reporting",
		EvidenceIntegrationState, EvidenceAvailability)
}

func (a *analysisBuilder) addRepairEvidence() {
	domain := a.integrationDomain()
	if !a.in.RepairsRead || domain == "" {
		return
	}
	open := 0
	for _, r := range a.in.Repairs {
		if r.Domain == domain && !r.Ignored {
			open++
		}
	}
	if open == 0 {
		return
	}
	a.evidence = append(a.evidence, model.Evidence{
		ID:           EvidenceRepairs,
		Observation:  "open repairs for the entity's integration",
		Source:       coreSource,
		From:         a.in.ObservedAt,
		To:           a.in.ObservedAt,
		Measurements: map[string]float64{"open_repairs": float64(open)},
		SampleSize:   open,
		Coverage:     1,
	})
	if !a.showsProblem() {
		return
	}
	a.cite("an open repair for the entity's integration may relate to the entity's behaviour",
		EvidenceRepairs, EvidenceAvailability)
}

// showsProblem reports whether the history evidence observed anything a
// setup state or repair could be offered as an explanation for. Without it
// those are context, not hypotheses: a repair beside a healthy entity
// explains nothing.
func (a *analysisBuilder) showsProblem() bool {
	return a.avail.UnavailablePeriods > 0 || a.cadence.StaleJudgeable && a.cadence.Stale
}

func (a *analysisBuilder) integrationDomain() string {
	switch {
	case a.in.ConfigEntry != nil:
		return a.in.ConfigEntry.Domain
	case a.in.Entity != nil:
		return a.in.Entity.Platform
	}
	return ""
}

func (a *analysisBuilder) addOutageHypotheses() {
	if a.avail.UnavailablePeriods >= repeatedOutageThreshold {
		a.cite("the entity repeatedly stopped reporting, which may point to an unstable connection or source",
			EvidenceAvailability)
	}
	if n := len(a.avail.Outages); n > 0 && a.avail.Outages[n-1].OpenEnded {
		a.cite("the entity was still not reporting at the end of the period",
			EvidenceAvailability)
	}
}

func (a *analysisBuilder) addStaleHypothesis() {
	if a.cadence.StaleJudgeable && a.cadence.Stale {
		a.cite("the entity has been silent for much longer than its own update cadence, which may mean its source stopped sending",
			EvidenceCadence)
	}
}

// cite records a hypothesis resting on the named evidence, dropping any
// that was not produced: a hypothesis with no surviving evidence is absent,
// not present with low confidence (D-05-1).
func (a *ledger) cite(statement string, ids ...model.EvidenceID) {
	var cited []model.Evidence
	var citedIDs []model.EvidenceID
	for _, id := range ids {
		if ev, ok := a.evidenceByID(id); ok {
			cited = append(cited, ev)
			citedIDs = append(citedIDs, id)
		}
	}
	h, err := model.NewHypothesis(statement, ConfidenceFor(cited...), citedIDs...)
	if err != nil {
		return
	}
	a.hypotheses = append(a.hypotheses, h)
}

func (a *ledger) evidenceByID(id model.EvidenceID) (model.Evidence, bool) {
	for _, ev := range a.evidence {
		if ev.ID == id {
			return ev, true
		}
	}
	return model.Evidence{}, false
}

// rankHypotheses orders most supported first: higher confidence, then more
// independent citations, then statement text so the order is deterministic.
func (a *ledger) rankHypotheses() {
	sort.SliceStable(a.hypotheses, func(i, j int) bool {
		hi, hj := a.hypotheses[i], a.hypotheses[j]
		if ri, rj := confidenceRank(hi.Confidence()), confidenceRank(hj.Confidence()); ri != rj {
			return ri > rj
		}
		if ci, cj := len(hi.Cites()), len(hj.Cites()); ci != cj {
			return ci > cj
		}
		return hi.Statement() < hj.Statement()
	})
}

// nextActions recommends the catalog tool that would deepen each finding.
// These are recommendations, never observations.
func (a *analysisBuilder) nextActions() []model.NextAction {
	var actions []model.NextAction
	if _, ok := a.evidenceByID(EvidenceAvailability); ok && a.avail.UnavailablePeriods > 0 {
		actions = append(actions, model.NextAction{
			Step: "inspect the raw state history around the unavailable periods",
			Tool: "get_entity_history",
		})
	}
	if ev, ok := a.evidenceByID(EvidenceIntegrationState); ok && ev.Measurements["loaded"] == 0 {
		actions = append(actions, model.NextAction{
			Step: "check the integration's setup state and reason",
			Tool: "get_integration",
		})
	}
	if _, ok := a.evidenceByID(EvidenceRepairs); ok {
		actions = append(actions, model.NextAction{
			Step: "read the open repairs for the integration",
			Tool: "list_repairs",
		})
	}
	return actions
}

// setupStateEvidence is one reading of a config entry's setup state, shared by
// the entity and integration analyses so both phrase and weigh it identically.
func setupStateEvidence(entry *model.Integration, at time.Time) model.Evidence {
	loaded := 0.0
	observation := "config entry setup state is unrecognized"
	if slices.Contains(knownSetupStates, entry.State) {
		observation = "config entry setup state is " + entry.State
		if entry.State == setupStateLoaded {
			loaded = 1
		}
	}
	return model.Evidence{
		ID:           EvidenceIntegrationState,
		Observation:  observation,
		Source:       coreSource,
		From:         at,
		To:           at,
		Measurements: map[string]float64{"loaded": loaded},
		// One reading of the current state; the ladder reads that as thin
		// support for a claim about a whole period, which is accurate.
		SampleSize: 1,
		Coverage:   1,
	}
}
