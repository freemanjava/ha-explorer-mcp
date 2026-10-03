package analysis

import (
	"math"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// The confidence ladder (D-05-2). A level means exactly this, per cited
// Evidence:
//
//	high   — at least highSampleSize observations, covering at least
//	         highCoverage of the requested period, from a source that
//	         answered fully.
//	medium — at least mediumSampleSize observations covering at least
//	         mediumCoverage of the period.
//	low    — anything thinner.
//
// A degraded source then demotes the level by one step: it answered, but
// only partly, so what it left out is unknown in a way Coverage cannot
// measure. A hypothesis citing several Evidence values gets the weakest of
// their levels — an inference is no better supported than its thinnest
// leg.
//
// The thresholds are starting defaults chosen to be explainable, not
// measurements (doc §26); P5-10 measures against a real recorder and is
// where they get revisited.
const (
	// mediumSampleSize: under five observations, one flap or one missed
	// poll is a large fraction of the sample, so no pattern is claimable.
	mediumSampleSize = 5
	// highSampleSize: twenty observations let a pattern repeat several
	// times inside the period rather than happen once or twice.
	highSampleSize = 20
	// mediumCoverage: with under half the period observed, the unseen part
	// could hold most of what happened.
	mediumCoverage = 0.5
	// highCoverage: not 1.0, because the recorder's purge boundary and an
	// HA restart routinely shave the edges of an otherwise complete period.
	highCoverage = 0.9
)

// confidenceRank orders the levels so the ladder can demote and take a
// minimum. Unexported on purpose: a level has no arithmetic meaning outside
// this file (D-05-2).
func confidenceRank(c model.Confidence) int {
	switch c {
	case model.ConfidenceHigh:
		return 2
	case model.ConfidenceMedium:
		return 1
	}
	return 0
}

var confidenceByRank = [...]model.Confidence{model.ConfidenceLow, model.ConfidenceMedium, model.ConfidenceHigh}

// ConfidenceFor is the single producer of a confidence level (D-05-2). It
// reads only SampleSize, Coverage and Degraded from each cited Evidence, and
// is monotone by construction: fewer samples, less coverage, a degraded
// source or one more weak citation can only lower the result. No evidence at
// all is low — a Hypothesis cannot cite none (D-05-1), so this is defensive.
func ConfidenceFor(cited ...model.Evidence) model.Confidence {
	if len(cited) == 0 {
		return model.ConfidenceLow
	}
	weakest := confidenceRank(model.ConfidenceHigh)
	for _, ev := range cited {
		weakest = min(weakest, evidenceRank(ev))
	}
	return confidenceByRank[weakest]
}

func evidenceRank(ev model.Evidence) int {
	coverage := clampCoverage(ev.Coverage)
	rank := 0
	switch {
	case ev.SampleSize >= highSampleSize && coverage >= highCoverage:
		rank = 2
	case ev.SampleSize >= mediumSampleSize && coverage >= mediumCoverage:
		rank = 1
	}
	if ev.Degraded {
		rank = max(rank-1, 0)
	}
	return rank
}

// clampCoverage bounds Coverage to [0, 1]. Over-coverage is a caller's
// rounding, not extra support; NaN means nothing was measured.
func clampCoverage(c float64) float64 {
	if math.IsNaN(c) {
		return 0
	}
	return min(max(c, 0), 1)
}
