// Package analysis computes deterministic diagnostic metrics over Home
// Assistant history. Aggregation happens here, server-side, so the agent
// receives evidence rather than a dataset it has to reduce itself (ADR-009,
// doc §12). Nothing in this package talks to HA: it takes already-mapped
// model values and returns numbers, which is what makes the metrics
// reproducible from a captured fixture.
package analysis

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// stateUnavailable and stateUnknown are the two HA states that mean "this
// entity is not reporting a usable value". The rest of the catalog already
// lumps them together for current availability (internal/ha.MapEntityStates,
// internal/mcp's availability filter); availability over a window uses the
// same two-way split so one tool's answer cannot contradict another's.
// Their durations stay separately countable through
// AvailabilityReport.UnknownDuration.
const (
	stateUnavailable = "unavailable"
	stateUnknown     = "unknown"
)

// ErrInvalidWindow reports a window whose end is not strictly after its
// start — a caller bug, not malformed HA data, so it is returned rather than
// absorbed into a degraded result.
var ErrInvalidWindow = errors.New("analysis: window end is not after window start")

// Outage is one contiguous run of non-available states inside the analysed
// window. A run that mixes "unavailable" and "unknown" is one outage, not
// two: the entity was continuously not reporting, and splitting on the exact
// flavour would inflate UnavailablePeriods.
type Outage struct {
	From     time.Time
	To       time.Time
	Duration time.Duration

	// TruncatedStart marks an outage that was already in progress when
	// observation began — no transition into it was recorded, so its real
	// start is earlier than From and its real duration longer than Duration.
	TruncatedStart bool
	// OpenEnded marks an outage still in progress at the window's end. Its
	// real end is later than To.
	OpenEnded bool
}

// AvailabilityReport is the deterministic availability picture for one entity
// over one bounded window (doc §12.1's availability half; cadence is
// P4-03's). Every duration is measured, never estimated, and the fields that
// say how much of the window was actually observed sit next to the ratio, so
// a thin sample can never be read as a healthy one.
type AvailabilityReport struct {
	From   time.Time
	To     time.Time
	Window time.Duration

	// CoveredFrom is the first instant the entity's state is known. It is
	// From whenever the recorder held a state at or before the window's
	// start, and later when it did not — a recorder gap, which is an absence
	// of evidence and never counted as an outage (CLAUDE.md rule 7). Zero
	// when nothing at all was recorded.
	CoveredFrom time.Time
	// Covered is To - CoveredFrom: the span the ratio is computed over.
	Covered time.Duration
	// CoverageGap is the leading span with no recorded state, CoveredFrom -
	// From. A gap inside the window is not observable from state changes
	// alone and is deliberately not guessed at.
	CoverageGap      time.Duration
	CoverageComplete bool

	// Computable is false when nothing was recorded in the window, in which
	// case AvailabilityRatio is meaningless and must not be reported as 0.0
	// — "could not check" is not "0% available".
	Computable        bool
	AvailabilityRatio float64

	// StateChanges counts observed transitions to a different state within
	// the covered span. The state held at CoveredFrom is not a change.
	StateChanges int

	Outages            []Outage
	UnavailablePeriods int
	TotalUnavailable   time.Duration
	LongestUnavailable time.Duration
	// UnknownDuration is how much of TotalUnavailable was spent in "unknown"
	// rather than "unavailable", kept separate so an entity that is merely
	// value-less is distinguishable from one that is gone.
	UnknownDuration time.Duration
}

// segment is one held state and the instant it began, after adjacent
// duplicates have been collapsed. The segment ends where the next one starts,
// or at the window's end for the last.
type segment struct {
	start time.Time
	state string
}

// ComputeAvailability reduces one entity's recorded history over [from, to]
// to availability ratio, outage count, total and longest outage.
//
// Points may arrive unsorted, may repeat a state, and may fall outside the
// window: HA data is untrusted input (CLAUDE.md rule 6), so they are sorted,
// collapsed and clamped here rather than assumed well-formed. The last point
// at or before from establishes the state the window opens in; points after
// to are ignored, since the window is the contract.
func ComputeAvailability(from, to time.Time, points []model.HistoryPoint) (AvailabilityReport, error) {
	if !to.After(from) {
		return AvailabilityReport{}, fmt.Errorf("%w: from=%s to=%s",
			ErrInvalidWindow, from.Format(time.RFC3339), to.Format(time.RFC3339))
	}

	rep := AvailabilityReport{From: from, To: to, Window: to.Sub(from)}
	segs := segmentsIn(from, to, points)
	if len(segs) == 0 {
		// Nothing recorded: the whole window is a coverage gap, and the
		// ratio stays uncomputable rather than defaulting to zero.
		rep.CoverageGap = rep.Window
		return rep, nil
	}

	rep.CoveredFrom = segs[0].start
	rep.Covered = to.Sub(segs[0].start)
	rep.CoverageGap = segs[0].start.Sub(from)
	rep.CoverageComplete = rep.CoverageGap == 0
	rep.StateChanges = len(segs) - 1

	for i, seg := range segs {
		end := to
		if i+1 < len(segs) {
			end = segs[i+1].start
		}
		if available(seg.state) {
			continue
		}
		if seg.state == stateUnknown {
			rep.UnknownDuration += end.Sub(seg.start)
		}
		rep.extendOutage(seg.start, end, i == 0, end.Equal(to))
	}

	for _, o := range rep.Outages {
		rep.TotalUnavailable += o.Duration
		if o.Duration > rep.LongestUnavailable {
			rep.LongestUnavailable = o.Duration
		}
	}
	rep.UnavailablePeriods = len(rep.Outages)

	if rep.Covered > 0 {
		rep.Computable = true
		rep.AvailabilityRatio = 1 - float64(rep.TotalUnavailable)/float64(rep.Covered)
	}
	return rep, nil
}

// extendOutage appends a non-available segment as a new outage, or grows the
// previous one when the two are contiguous — which is how an
// "unavailable" → "unknown" run stays a single period.
func (r *AvailabilityReport) extendOutage(start, end time.Time, first, openEnded bool) {
	if n := len(r.Outages); n > 0 && r.Outages[n-1].To.Equal(start) {
		last := &r.Outages[n-1]
		last.To = end
		last.Duration = end.Sub(last.From)
		last.OpenEnded = openEnded
		return
	}
	r.Outages = append(r.Outages, Outage{
		From:           start,
		To:             end,
		Duration:       end.Sub(start),
		TruncatedStart: first,
		OpenEnded:      openEnded,
	})
}

// available reports whether a recorded state means the entity was reporting
// a usable value.
func available(state string) bool {
	return state != stateUnavailable && state != stateUnknown
}

// segmentsIn orders the points, keeps the last one at or before from as the
// state the window opens in, drops everything after to, and collapses
// adjacent repeats of the same state so a re-recorded identical state is not
// counted as a change.
func segmentsIn(from, to time.Time, points []model.HistoryPoint) []segment {
	sorted := make([]model.HistoryPoint, len(points))
	copy(sorted, points)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	segs := make([]segment, 0, len(sorted))
	for _, p := range sorted {
		if p.Timestamp.After(to) {
			break
		}
		start := p.Timestamp
		if !start.After(from) {
			// At or before the window: it only establishes the opening
			// state, so it collapses onto the window's start.
			start = from
			if len(segs) > 0 {
				segs = segs[:0]
			}
		}
		if n := len(segs); n > 0 && segs[n-1].state == p.State {
			continue
		}
		segs = append(segs, segment{start: start, state: p.State})
	}
	return segs
}
