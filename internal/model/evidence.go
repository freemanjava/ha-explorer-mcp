package model

import (
	"errors"
	"fmt"
	"time"
)

// Fact, inference and recommendation are separate types, not fields on one
// struct (D-05-1, ADR-010): Evidence is what was measured, Hypothesis is what
// might explain it, MissingEvidence is what could not be measured, NextAction
// is what to look at next. Doc §12.2's one-object example is the serialized
// rendering of an Evidence plus the Hypothesis citing it, produced at the MCP
// boundary — never a shape the analysis code can fill in one assignment. No
// type here carries a cause: a correlation is evidence, never an established
// cause.

// ErrUncitedHypothesis is returned by NewHypothesis when no evidence is
// cited. An inference resting on nothing is not a weak hypothesis; it is not
// one at all (D-05-1).
var ErrUncitedHypothesis = errors.New("hypothesis cites no evidence")

// ErrInvalidHypothesis is returned by NewHypothesis for an empty statement or
// a confidence outside the defined levels.
var ErrInvalidHypothesis = errors.New("invalid hypothesis")

// EvidenceID identifies one Evidence value within a HealthAnalysis, so a
// Hypothesis can cite it by reference rather than by restating it.
type EvidenceID string

// Evidence is one measured observation: what was seen, by which source, over
// which period. It has no inference, confidence or recommendation field —
// those belong to Hypothesis and NextAction.
type Evidence struct {
	ID EvidenceID

	// Observation describes what was measured, e.g. "unavailable periods".
	// It names the measurement, not what it implies.
	Observation string

	// Source names the HA subsystem or recorder endpoint the measurement came
	// from (e.g. "recorder_history"), as Health.Source does.
	Source string

	From time.Time
	To   time.Time

	// Measurements carries the numbers behind Observation, keyed by name
	// (doc §12.2's "evidence": {"outages": 7}). Keys are set by analysis
	// code, never copied from HA data (CLAUDE.md rule 6).
	Measurements map[string]float64

	// SampleSize and Coverage are the inputs D-05-2's ConfidenceFor reads:
	// how many observations back the measurement, and the fraction (0–1) of
	// [From, To] the source actually covered. Degraded is set when the
	// source answered only partially.
	SampleSize int
	Coverage   float64
	Degraded   bool
}

// Confidence is how strongly a Hypothesis's cited evidence supports it — not
// how certain the evidence itself is, which is a deterministic measurement.
// It is a level, not a number: arithmetic on it has no meaning (D-05-2). The
// only producer of a level is analysis.ConfidenceFor (P5-03); the ladder that
// maps evidence onto these levels is documented there.
type Confidence string

const (
	ConfidenceLow    Confidence = "low"
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

func (c Confidence) valid() bool {
	switch c {
	case ConfidenceLow, ConfidenceMedium, ConfidenceHigh:
		return true
	}
	return false
}

// Hypothesis is an inference that might explain cited evidence. It carries no
// measurement of its own, so nothing on it can be read as an observed fact.
// Its fields are unexported so NewHypothesis is the only way to build one
// that says anything: a hypothesis citing no evidence cannot be constructed.
type Hypothesis struct {
	statement  string
	confidence Confidence
	cites      []EvidenceID
}

// NewHypothesis builds a Hypothesis citing at least one evidence id.
// Duplicate citations are dropped, keeping first-seen order; a blank id is
// not a citation.
func NewHypothesis(statement string, confidence Confidence, cites ...EvidenceID) (Hypothesis, error) {
	if statement == "" {
		return Hypothesis{}, fmt.Errorf("%w: empty statement", ErrInvalidHypothesis)
	}
	if !confidence.valid() {
		return Hypothesis{}, fmt.Errorf("%w: unknown confidence %q", ErrInvalidHypothesis, confidence)
	}
	unique := dedupeCitations(cites)
	if len(unique) == 0 {
		return Hypothesis{}, ErrUncitedHypothesis
	}
	return Hypothesis{statement: statement, confidence: confidence, cites: unique}, nil
}

func dedupeCitations(cites []EvidenceID) []EvidenceID {
	seen := make(map[EvidenceID]bool, len(cites))
	unique := make([]EvidenceID, 0, len(cites))
	for _, id := range cites {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	return unique
}

// Statement is the inference, phrased as a possibility.
func (h Hypothesis) Statement() string { return h.statement }

// Confidence is the derived level of support from the cited evidence.
func (h Hypothesis) Confidence() Confidence { return h.confidence }

// Cites returns a copy of the cited evidence ids, so a caller cannot rewrite
// what a Hypothesis rests on.
func (h Hypothesis) Cites() []EvidenceID {
	return append([]EvidenceID(nil), h.cites...)
}

// MissingReason says why a source could not be observed. The values stay
// distinct the way ErrNotFound/ErrUnsupported/ErrPolicyDenied do (CLAUDE.md,
// Error Handling): each points the agent at a different next step.
type MissingReason string

const (
	// MissingUnsupported: this installation or HA version does not offer the
	// source (e.g. traces unavailable to the principal, F-11).
	MissingUnsupported MissingReason = "unsupported"
	// MissingPolicyDenied: the privacy profile refused the read.
	MissingPolicyDenied MissingReason = "policy_denied"
	// MissingUpstreamUnavailable: the source exists but did not answer (e.g.
	// Supervisor absent while Core is up, doc §3.2).
	MissingUpstreamUnavailable MissingReason = "upstream_unavailable"
	// MissingDeadline: the source did not answer within the call's deadline.
	MissingDeadline MissingReason = "deadline"
	// MissingBudgetExceeded: the query budget stopped the read before it ran.
	MissingBudgetExceeded MissingReason = "budget_exceeded"
	// MissingOutOfRetention: the recorder does not reach back over the
	// requested period.
	MissingOutOfRetention MissingReason = "out_of_retention"
	// MissingEntityDisabled: the entity carrying the metric exists but is
	// disabled, so it records nothing — ZHA's LQI/RSSI diagnostics ship this
	// way (D-05-5). Enabling it is the fix.
	MissingEntityDisabled MissingReason = "entity_disabled"
	// MissingNotExposed: the integration has no equivalent at all —
	// Zigbee2MQTT exposes no RSSI (D-05-5). No action on HA's side helps.
	MissingNotExposed MissingReason = "not_exposed"
	// MissingPrivileged: the evidence needs host privileges this binary never
	// has (USB resets, dmesg — ADR-012's separate Host Probe).
	MissingPrivileged MissingReason = "privileged"
)

// MissingEvidence names what could not be observed and why. It is a
// first-class output, not an error: it is what keeps an agent from
// over-concluding, and an unreadable source lowers confidence rather than
// failing the call (doc §3.2).
type MissingEvidence struct {
	// What names the evidence that is absent, e.g. "signal strength".
	What   string
	Source string
	Reason MissingReason
	// Detail is analysis-authored context for Reason; never HA-supplied text.
	Detail string
}

// NextAction is a recommended diagnostic step — a recommendation, never an
// observation.
type NextAction struct {
	Step string
	// Tool names the catalog tool that performs the step, empty when no tool
	// of this server can (e.g. a privileged Host Probe read).
	Tool string
}

// HealthAnalysis is the envelope analyze_entity_health and
// analyze_integration_health return (Appendix A.3). Hypotheses are ranked,
// most supported first. There is no score field (D-05-4).
type HealthAnalysis struct {
	SubjectID  string
	ObservedAt time.Time
	From       time.Time
	To         time.Time

	Evidence        []Evidence
	Hypotheses      []Hypothesis
	MissingEvidence []MissingEvidence
	NextActions     []NextAction

	Provenance
}
