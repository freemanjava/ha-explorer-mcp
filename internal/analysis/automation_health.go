package analysis

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// Evidence ids AnalyzeAutomationHealth assigns. Per-dependency evidence is
// numbered ("dependency_1", …) in entity-id order rather than named after the
// entity: an evidence id is analysis-authored, never derived from HA data
// (CLAUDE.md rule 6). AutomationHealth.DependencyEvidence says which id
// measured which entity.
const (
	EvidenceAutomationRuns model.EvidenceID = "automation_runs"
	EvidenceDependencies   model.EvidenceID = "dependencies"
)

const (
	// The sources an automation analysis reads, named by the HA command that
	// answers them so missing_evidence points at exactly what was not asked.
	configSource  = "automation/config"
	tracesSource  = "trace/list"
	logbookSource = "logbook"

	automationDomain = "automation"
)

// Run outcomes, from trace/list's script_execution. Any other value is
// HA-supplied text: it is counted as unrecognized, never echoed or branched
// on (rule 6).
const (
	execFinished         = "finished"
	execFailedConditions = "failed_conditions"
	execError            = "error"
	execAborted          = "aborted"
)

// knownExecutions are the remaining script_execution values HA documents:
// mode-related refusals (single/max-runs), a cancelled run and one still in
// flight. Recognized, but none says anything about a dependency.
var knownExecutions = []string{"cancelled", "failed_single", "failed_max_runs", "running"}

// DependencyHistory is one dependency entity's input: its recorder history,
// or why it was not read. Device and area dependencies arrive already
// resolved to their entities — that resolution is where the registry is read
// (P5-13), not here.
type DependencyHistory struct {
	EntityID     model.EntityID
	Read         bool
	UnreadReason model.MissingReason
	Points       []model.HistoryPoint
	// Degraded marks a history read that answered only partly.
	Degraded bool
}

// AutomationHealthInput is everything AnalyzeAutomationHealth composes,
// already read. As with the entity and integration analyses, analysis never
// fetches, and each source carries a flag saying whether it was read — with
// the reason when it was not — so an unread source is never mistaken for an
// empty one (rule 7).
type AutomationHealthInput struct {
	// Automation is get_automation's mapped view: DependsOn and the
	// extraction gaps beside it (P5-11).
	Automation model.Automation
	ObservedAt time.Time
	From, To   time.Time

	ConfigRead   bool
	ConfigUnread model.MissingReason

	// Traces are the automation's run summaries, unfiltered by period: a
	// trace older than From is how the analysis knows the stored traces reach
	// back over the whole period.
	TracesRead   bool
	TracesUnread model.MissingReason
	Traces       []model.AutomationTraceSummary

	// The F-11 fallback, read when traces were not: logbook events for the
	// automation's own entity since LogbookSince, and last_triggered. Runs
	// are counted from them but their outcome is unknowable.
	FallbackRead   bool
	FallbackUnread model.MissingReason
	LastTriggered  *time.Time
	LogbookEvents  []model.LogbookEvent
	LogbookSince   time.Time

	Dependencies []DependencyHistory

	RepairsRead bool
	Repairs     []model.Repair

	// Missing carries any other source the caller could not read, with why.
	Missing []model.MissingEvidence
}

// DependencyEvidenceRef names the dependency entity a per-dependency
// evidence id measured, so the tool layer can render it.
type DependencyEvidenceRef struct {
	EntityID model.EntityID
	Evidence model.EvidenceID
}

// AutomationHealth is AnalyzeAutomationHealth's result: Appendix A.3's
// envelope, plus which dependency each per-dependency evidence id is about.
type AutomationHealth struct {
	model.HealthAnalysis
	DependencyEvidence []DependencyEvidenceRef
}

// AnalyzeAutomationHealth composes an automation's run outcomes with its
// dependencies' unavailable and stale windows into Appendix A.3's shape. A
// failed run inside a dependency's window is an overlap (within
// outageClusterTolerance, D-05-3) offered as a possibility, never as a
// cause. Run evidence from the F-11 fallback is Degraded, so ConfidenceFor
// alone demotes what rests on it (D-05-2). There is no score (D-05-4); a
// missing source lowers what can be concluded and never fails the call.
func AnalyzeAutomationHealth(in AutomationHealthInput) (AutomationHealth, error) {
	if !in.To.After(in.From) {
		return AutomationHealth{}, fmt.Errorf("%w: from=%s to=%s",
			ErrInvalidWindow, in.From.Format(time.RFC3339), in.To.Format(time.RFC3339))
	}
	b := automationBuilder{in: in, missing: slices.Clone(in.Missing)}
	b.addRunEvidence()
	b.noteConfigGaps()
	if err := b.addDependencyEvidence(); err != nil {
		return AutomationHealth{}, err
	}
	b.addRepairEvidence()
	b.addHypotheses()
	b.rankHypotheses()

	out := AutomationHealth{
		HealthAnalysis: model.HealthAnalysis{
			SubjectID:       string(in.Automation.EntityID),
			ObservedAt:      in.ObservedAt,
			From:            in.From,
			To:              in.To,
			Evidence:        b.evidence,
			Hypotheses:      b.hypotheses,
			MissingEvidence: b.missing,
			NextActions:     b.nextActions(),
		},
		DependencyEvidence: b.depRefs,
	}
	if len(b.missing) > 0 {
		out.Partial = true
		out.PartialReason = "one or more evidence sources could not be observed; see missing_evidence"
	}
	return out, nil
}

// run is one automation run in the period. outcomeKnown is false for a run
// counted from the logbook, which says the automation fired but not how its
// run ended.
type run struct {
	from, to     time.Time
	outcomeKnown bool
	failed       bool
	errored      bool
}

// span is one window during which a dependency was not reporting usefully.
type span struct{ from, to time.Time }

type automationBuilder struct {
	ledger
	in      AutomationHealthInput
	missing []model.MissingEvidence

	runs    []run
	depRefs []DependencyEvidenceRef
	// windows are every measured dependency's spans together.
	windows []span
	// overlapping are the dependency evidence ids at least one relevant run
	// overlapped; stillDown the ones not reporting at the end of the period.
	overlapping []model.EvidenceID
	stillDown   []model.EvidenceID
}

func (b *automationBuilder) addRunEvidence() {
	switch {
	case b.in.TracesRead:
		b.addTraceEvidence()
	case b.in.FallbackRead:
		b.noteTracesMissing("run outcomes are not observable; runs are counted from the logbook instead, so their evidence is degraded")
		b.addFallbackEvidence()
	default:
		b.noteTracesMissing("neither traces nor the logbook fallback answered, so no run of the automation is observed")
		b.missing = append(b.missing, model.MissingEvidence{
			What:   "automation runs from the logbook",
			Source: logbookSource,
			Reason: reasonOr(b.in.FallbackUnread),
			Detail: "the logbook fallback did not answer",
		})
	}
}

func (b *automationBuilder) noteTracesMissing(detail string) {
	b.missing = append(b.missing, model.MissingEvidence{
		What:   "automation run traces",
		Source: tracesSource,
		Reason: reasonOr(b.in.TracesUnread),
		Detail: detail,
	})
}

// reasonOr defaults an unstated reason to unsupported: a source that was not
// read and gave no reason could not be asked.
func reasonOr(r model.MissingReason) model.MissingReason {
	if r == "" {
		return model.MissingUnsupported
	}
	return r
}

func (b *automationBuilder) addTraceEvidence() {
	counts := map[string]float64{
		"runs": 0, execFinished: 0, "stopped_at_condition": 0, execError: 0,
		execAborted: 0, "other": 0, "unrecognized": 0,
	}
	reachesBack := false
	for _, tr := range b.in.Traces {
		if tr.TimestampStart.Before(b.in.From) {
			reachesBack = true
			continue
		}
		if tr.TimestampStart.After(b.in.To) {
			continue
		}
		counts["runs"]++
		counts[outcomeKey(tr.ScriptExecution)]++
		r := run{from: tr.TimestampStart, to: tr.TimestampFinish, outcomeKnown: true}
		if r.to.Before(r.from) {
			r.to = r.from
		}
		switch tr.ScriptExecution {
		case execFailedConditions:
			r.failed = true
		case execError, execAborted:
			r.failed, r.errored = true, true
		}
		b.runs = append(b.runs, r)
	}
	b.evidence = append(b.evidence, model.Evidence{
		ID:           EvidenceAutomationRuns,
		Observation:  "automation run outcomes over the period",
		Source:       tracesSource,
		From:         b.in.From,
		To:           b.in.To,
		Measurements: counts,
		SampleSize:   len(b.runs),
		Coverage:     b.traceCoverage(reachesBack),
	})
}

func outcomeKey(execution string) string {
	switch {
	case execution == execFailedConditions:
		return "stopped_at_condition"
	case execution == execFinished || execution == execError || execution == execAborted:
		return execution
	case slices.Contains(knownExecutions, execution):
		return "other"
	}
	return "unrecognized"
}

// traceCoverage is how much of the period the stored traces observe. HA keeps
// only the last few traces per automation, so runs older than the oldest one
// are unseen: the traces cover the period only from their oldest run, unless
// a trace predates the period or there are none to have been dropped.
func (b *automationBuilder) traceCoverage(reachesBack bool) float64 {
	if reachesBack || len(b.runs) == 0 {
		return 1
	}
	oldest := b.runs[0].from
	for _, r := range b.runs[1:] {
		if r.from.Before(oldest) {
			oldest = r.from
		}
	}
	return b.periodFraction(oldest)
}

// periodFraction is the share of the period from since to its end.
func (b *automationBuilder) periodFraction(since time.Time) float64 {
	if !since.After(b.in.From) {
		return 1
	}
	return float64(b.in.To.Sub(since)) / float64(b.in.To.Sub(b.in.From))
}

// addFallbackEvidence counts runs from the logbook: one run per distinct
// context id, since one run can log more than one entry.
func (b *automationBuilder) addFallbackEvidence() {
	seen := map[string]bool{}
	for _, ev := range b.in.LogbookEvents {
		if ev.When.Before(b.in.From) || ev.When.After(b.in.To) {
			continue
		}
		if ev.ContextID != "" {
			if seen[ev.ContextID] {
				continue
			}
			seen[ev.ContextID] = true
		}
		b.runs = append(b.runs, run{from: ev.When, to: ev.When})
	}
	m := map[string]float64{"runs": float64(len(b.runs))}
	if lt := b.in.LastTriggered; lt != nil && !lt.After(b.in.ObservedAt) {
		m["seconds_since_last_triggered"] = b.in.ObservedAt.Sub(*lt).Seconds()
	}
	b.evidence = append(b.evidence, model.Evidence{
		ID:           EvidenceAutomationRuns,
		Observation:  "automation runs over the period; their outcome is not observable from the logbook",
		Source:       logbookSource,
		From:         b.in.From,
		To:           b.in.To,
		Measurements: m,
		SampleSize:   len(b.runs),
		Coverage:     b.periodFraction(b.in.LogbookSince),
		Degraded:     true,
	})
}

// noteConfigGaps names each way the dependency list is incomplete. Each gap
// is a different next step, so each is its own entry.
func (b *automationBuilder) noteConfigGaps() {
	if !b.in.ConfigRead {
		b.missing = append(b.missing, model.MissingEvidence{
			What:   "automation dependencies",
			Source: configSource,
			Reason: reasonOr(b.in.ConfigUnread),
			Detail: "the automation's config could not be read, so the entities it depends on are unknown",
		})
		return
	}
	a := b.in.Automation
	if a.UnextractedRefs > 0 {
		b.missing = append(b.missing, model.MissingEvidence{
			What:   "template or unrecognized dependency references",
			Source: configSource,
			Reason: model.MissingUnsupported,
			Detail: fmt.Sprintf("%d references in the config are templates or values that are not valid ids; they were not followed", a.UnextractedRefs),
		})
	}
	if a.DependsTruncated {
		b.missing = append(b.missing, model.MissingEvidence{
			What:   "dependencies past the extraction cap",
			Source: configSource,
			Reason: model.MissingBudgetExceeded,
			Detail: "the dependency list hit its cap; dependencies past it were not checked",
		})
	}
	if a.DependsWithheld > 0 {
		b.missing = append(b.missing, model.MissingEvidence{
			What:   "dependencies withheld by the privacy profile",
			Source: configSource,
			Reason: model.MissingPolicyDenied,
			Detail: fmt.Sprintf("%d dependencies are private under the active profile and were not checked", a.DependsWithheld),
		})
	}
}

// addDependencyEvidence measures each read dependency's windows and the runs
// overlapping them. Only a dependency with a window gets its own evidence;
// the rest are counted in one summary, so a long healthy list stays one
// entry.
func (b *automationBuilder) addDependencyEvidence() error {
	if !b.in.ConfigRead {
		return nil
	}
	deps := slices.Clone(b.in.Dependencies)
	slices.SortFunc(deps, func(x, y DependencyHistory) int { return cmp.Compare(x.EntityID, y.EntityID) })
	unread := map[model.MissingReason]int{}
	checked, empty := 0, 0
	for _, d := range deps {
		if !d.Read {
			unread[reasonOr(d.UnreadReason)]++
			continue
		}
		checked++
		ok, err := b.measureDependency(d)
		if err != nil {
			return err
		}
		if !ok {
			empty++
		}
	}
	b.noteUnreadDependencies(unread, empty)
	b.evidence = append(b.evidence, model.Evidence{
		ID:          EvidenceDependencies,
		Observation: "dependency entities checked for unavailable or stale windows",
		Source:      recorderSource,
		From:        b.in.From,
		To:          b.in.To,
		Measurements: map[string]float64{
			"checked":      float64(checked),
			"with_windows": float64(len(b.depRefs)),
			"unread":       float64(len(deps) - checked),
		},
		SampleSize: checked,
		Coverage:   1,
	})
	return nil
}

// measureDependency adds evidence for one dependency that had a window, and
// reports false when its history held nothing to measure.
func (b *automationBuilder) measureDependency(d DependencyHistory) (bool, error) {
	avail, err := ComputeAvailability(b.in.From, b.in.To, d.Points)
	if err != nil {
		return false, err
	}
	cadence, err := ComputeCadence(b.in.From, b.in.To, d.Points)
	if err != nil {
		return false, err
	}
	if !avail.Computable {
		return false, nil
	}
	spans := make([]span, 0, len(avail.Outages)+1)
	for _, o := range avail.Outages {
		spans = append(spans, span{from: o.From, to: o.To})
	}
	stale := cadence.StaleJudgeable && cadence.Stale
	if stale {
		spans = append(spans, span{from: cadence.LastUpdate.Add(cadence.StaleThreshold), to: b.in.To})
	}
	if len(spans) == 0 {
		return true, nil
	}

	b.windows = append(b.windows, spans...)
	id := model.EvidenceID(fmt.Sprintf("dependency_%d", len(b.depRefs)+1))
	b.depRefs = append(b.depRefs, DependencyEvidenceRef{EntityID: d.EntityID, Evidence: id})
	overlapping, overlappingFailed := b.overlaps(spans)
	stillDown := stale || len(avail.Outages) > 0 && avail.Outages[len(avail.Outages)-1].OpenEnded
	b.evidence = append(b.evidence, model.Evidence{
		ID:          id,
		Observation: "unavailable or stale windows of one dependency, and the runs overlapping them",
		Source:      recorderSource,
		From:        b.in.From,
		To:          b.in.To,
		Measurements: map[string]float64{
			"unavailable_periods":       float64(avail.UnavailablePeriods),
			"total_unavailable_seconds": avail.TotalUnavailable.Seconds(),
			"stale":                     boolMeasure(stale),
			"still_unavailable":         boolMeasure(stillDown),
			"overlapping_runs":          float64(overlapping),
			"overlapping_failed_runs":   float64(overlappingFailed),
		},
		SampleSize: len(d.Points),
		Coverage:   coverageOf(avail),
		Degraded:   d.Degraded,
	})
	if b.overlapMatters(overlapping, overlappingFailed) {
		b.overlapping = append(b.overlapping, id)
	}
	if stillDown {
		b.stillDown = append(b.stillDown, id)
	}
	return true, nil
}

// overlapMatters: with traces, only a failed run inside a window is worth a
// hypothesis; from the logbook any run is, since its outcome is unknown and
// may have been a failure.
func (b *automationBuilder) overlapMatters(overlapping, failed int) bool {
	if b.in.TracesRead {
		return failed > 0
	}
	return overlapping > 0
}

// overlaps counts runs touching any span, allowing outageClusterTolerance on
// either side for the same clock and polling skew clustering allows (D-05-3).
// Each run counts once however many spans it touches.
func (b *automationBuilder) overlaps(spans []span) (all, failed int) {
	for _, r := range b.runs {
		if !touches(r, spans) {
			continue
		}
		all++
		if r.failed {
			failed++
		}
	}
	return all, failed
}

// touches reports whether r overlaps any span within the tolerance.
func touches(r run, spans []span) bool {
	return slices.ContainsFunc(spans, func(s span) bool {
		return !r.from.After(s.to.Add(outageClusterTolerance)) && !r.to.Before(s.from.Add(-outageClusterTolerance))
	})
}

func boolMeasure(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

// noteUnreadDependencies names unread and empty dependency histories, one
// entry per reason so a long list stays bounded.
func (b *automationBuilder) noteUnreadDependencies(unread map[model.MissingReason]int, empty int) {
	reasons := make([]model.MissingReason, 0, len(unread))
	for r := range unread {
		reasons = append(reasons, r)
	}
	slices.Sort(reasons)
	for _, r := range reasons {
		b.missing = append(b.missing, model.MissingEvidence{
			What:   "dependency history",
			Source: recorderSource,
			Reason: r,
			Detail: fmt.Sprintf("%d dependencies' history was not read, so their windows are unknown", unread[r]),
		})
	}
	if empty > 0 {
		b.missing = append(b.missing, model.MissingEvidence{
			What:   "dependency history",
			Source: recorderSource,
			Reason: model.MissingOutOfRetention,
			Detail: fmt.Sprintf("%d dependencies have no recorded state at or before the end of the period", empty),
		})
	}
}

// addRepairEvidence counts open repairs raised by the automation integration.
// A repair names its automation only in HA-supplied text, which is never
// parsed (rule 6), so the count is of the domain's repairs, not this
// automation's.
func (b *automationBuilder) addRepairEvidence() {
	if !b.in.RepairsRead {
		return
	}
	open := 0
	for _, r := range b.in.Repairs {
		if r.Domain == automationDomain && !r.Ignored {
			open++
		}
	}
	if open == 0 {
		return
	}
	b.evidence = append(b.evidence, model.Evidence{
		ID:           EvidenceRepairs,
		Observation:  "open repairs raised by the automation integration",
		Source:       coreSource,
		From:         b.in.ObservedAt,
		To:           b.in.ObservedAt,
		Measurements: map[string]float64{"open_repairs": float64(open)},
		SampleSize:   open,
		Coverage:     1,
	})
}

func (b *automationBuilder) addHypotheses() {
	for _, id := range b.overlapping {
		if b.in.TracesRead {
			b.cite("runs of the automation failed while one of its dependencies was unavailable or stale, which may mean the dependency's state kept it from completing",
				EvidenceAutomationRuns, id)
			continue
		}
		b.cite("the automation ran while one of its dependencies was unavailable or stale; the run's outcome is not observable without traces, so it may not have completed",
			EvidenceAutomationRuns, id)
	}
	for _, id := range b.stillDown {
		b.cite("a dependency of the automation was not reporting at the end of the period, which may keep the automation from triggering or passing its conditions",
			id)
	}
	if !b.errorsOutsideWindows() {
		return
	}
	b.cite("runs of the automation ended in error or were aborted outside any dependency window, which may point to a fault in one of its actions",
		EvidenceAutomationRuns)
	if _, ok := b.evidenceByID(EvidenceRepairs); ok {
		b.cite("an open repair raised by the automation integration may relate to the failed runs",
			EvidenceRepairs, EvidenceAutomationRuns)
	}
}

// errorsOutsideWindows reports an errored or aborted run that no dependency
// window explains. A run stopped at a condition alone is not a problem —
// stopping runs is what conditions are for.
func (b *automationBuilder) errorsOutsideWindows() bool {
	for _, r := range b.runs {
		if r.errored && !touches(r, b.windows) {
			return true
		}
	}
	return false
}

// nextActions recommends the catalog tool that would deepen each finding.
// These are recommendations, never observations.
func (b *automationBuilder) nextActions() []model.NextAction {
	var actions []model.NextAction
	if b.in.TracesRead && slices.ContainsFunc(b.runs, func(r run) bool { return r.failed }) {
		actions = append(actions, model.NextAction{
			Step: "read the traces of the failed runs",
			Tool: "get_automation_traces",
		})
	}
	if len(b.depRefs) > 0 {
		actions = append(actions, model.NextAction{
			Step: "inspect the dependencies' state history around their unavailable or stale windows",
			Tool: "get_entity_history",
		})
	}
	if _, ok := b.evidenceByID(EvidenceRepairs); ok {
		actions = append(actions, model.NextAction{
			Step: "read the open repairs raised by the automation integration",
			Tool: "list_repairs",
		})
	}
	return actions
}
