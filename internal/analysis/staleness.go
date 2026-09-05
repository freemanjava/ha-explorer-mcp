package analysis

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// staleIntervalFactor scales an entity's own observed tail interval into the
// silence that counts as stale: an entity is stale when it has been quiet for
// more than staleIntervalFactor × its p95 update interval. The judgement is
// relative on purpose — doc §11's "staleness" is a property of an entity
// against its own cadence, and a global "nothing for N minutes" constant
// would flag every hourly sensor and miss every one-second one.
//
// Three is a starting default, not a measurement (doc §26): it is the
// smallest multiple that clears ordinary jitter around the tail interval
// without needing a second, absolute threshold. Revisit against a real
// installation's false-positive rate at P4-05, which is the first task to
// rank entities by it.
const staleIntervalFactor = 3

// CadenceReport is the deterministic update-cadence and staleness picture for
// one entity over one bounded window (doc §12.1's cadence half; availability
// is P4-02's). Percentiles are always observed intervals, never interpolated
// values, and the flags that say whether a judgement could be made sit next
// to the judgement, so "not stale" and "could not tell" stay distinguishable.
type CadenceReport struct {
	From   time.Time
	To     time.Time
	Window time.Duration

	// Observed is false when the recorder held nothing at or before the
	// window's end — no update, no state carried in from earlier. Nothing in
	// this report may then be read as a fact about the entity.
	Observed bool
	// Updates counts recorded points inside the window. A state carried in
	// from before the window is not an update: it happened earlier.
	Updates int
	// Intervals is the number of gaps between consecutive recorded points
	// that the percentiles are computed over — the sample size, kept visible
	// because a p95 over two samples is not a p95 over two hundred.
	Intervals int

	// Computable is false when no interval could be observed (fewer than two
	// recorded points), in which case the interval fields are meaningless and
	// must not be reported as zero — "could not check" is not "instant".
	Computable           bool
	MedianUpdateInterval time.Duration
	P95UpdateInterval    time.Duration
	MinUpdateInterval    time.Duration
	MaxUpdateInterval    time.Duration

	// StateChanges counts observed transitions to a different state, on the
	// same collapsed-segment basis as AvailabilityReport.StateChanges, so the
	// two halves of doc §12.1 cannot disagree about the same window.
	StateChanges           int
	StateChangeRatePerHour float64

	// LastUpdate is the most recent recorded point at or before the window's
	// end, including one from before the window began: an entity that has not
	// updated for a week is exactly the case staleness exists to catch, and
	// clamping the measurement to the window would hide it.
	LastUpdate time.Time
	SilentFor  time.Duration

	// StaleJudgeable is false when cadence is not computable: without an
	// observed interval there is nothing to call the silence long against.
	StaleJudgeable bool
	Stale          bool
	// StaleThreshold is the silence beyond which this entity counts as stale,
	// staleIntervalFactor × P95UpdateInterval.
	StaleThreshold time.Duration
	// StalenessRatio is SilentFor / P95UpdateInterval — how many tail
	// intervals of silence, for ranking entities against each other (P4-05).
	StalenessRatio float64
}

// ComputeCadence reduces one entity's recorded history over [from, to] to
// update-interval percentiles, state-change rate and a staleness judgement
// made against the entity's own observed cadence.
//
// Points may arrive unsorted, may repeat a timestamp and may fall outside the
// window: HA data is untrusted input (CLAUDE.md rule 6). Intervals are
// measured between consecutive distinct instants; a second record at an
// instant already seen is dropped rather than admitted as a zero-length
// interval that would drag every percentile down.
func ComputeCadence(from, to time.Time, points []model.HistoryPoint) (CadenceReport, error) {
	if !to.After(from) {
		return CadenceReport{}, fmt.Errorf("%w: from=%s to=%s",
			ErrInvalidWindow, from.Format(time.RFC3339), to.Format(time.RFC3339))
	}

	rep := CadenceReport{From: from, To: to, Window: to.Sub(from)}

	// The leading point is the state carried into the window; it bounds the
	// first observed interval but is not itself an update inside the window.
	leading, inWindow := splitAtWindow(from, to, points)
	if leading == nil && len(inWindow) == 0 {
		return rep, nil
	}
	rep.Observed = true
	rep.Updates = len(inWindow)

	if n := len(inWindow); n > 0 {
		rep.LastUpdate = inWindow[n-1]
	} else {
		rep.LastUpdate = *leading
	}
	rep.SilentFor = to.Sub(rep.LastUpdate)

	intervals := intervalsBetween(leading, inWindow)
	rep.Intervals = len(intervals)
	if len(intervals) > 0 {
		slices.Sort(intervals)
		rep.Computable = true
		rep.MinUpdateInterval = intervals[0]
		rep.MaxUpdateInterval = intervals[len(intervals)-1]
		rep.MedianUpdateInterval = nearestRank(intervals, 0.50)
		rep.P95UpdateInterval = nearestRank(intervals, 0.95)

		rep.StaleJudgeable = true
		rep.StaleThreshold = staleIntervalFactor * rep.P95UpdateInterval
		rep.Stale = rep.SilentFor > rep.StaleThreshold
		rep.StalenessRatio = float64(rep.SilentFor) / float64(rep.P95UpdateInterval)
	}

	// The change rate shares P4-02's collapsed segments and covered span, so
	// one window cannot yield two different change counts.
	if segs := segmentsIn(from, to, points); len(segs) > 0 {
		rep.StateChanges = len(segs) - 1
		if covered := to.Sub(segs[0].start); covered > 0 {
			rep.StateChangeRatePerHour = float64(rep.StateChanges) / covered.Hours()
		}
	}
	return rep, nil
}

// splitAtWindow orders the points by time, drops everything after to and every
// repeat of an instant already seen, and returns the last instant at or before
// from — the state carried into the window, if any — separately from the
// instants recorded inside it.
func splitAtWindow(from, to time.Time, points []model.HistoryPoint) (leading *time.Time, inWindow []time.Time) {
	stamps := make([]time.Time, 0, len(points))
	for _, p := range points {
		if p.Timestamp.After(to) {
			continue
		}
		stamps = append(stamps, p.Timestamp)
	}
	sort.Slice(stamps, func(i, j int) bool { return stamps[i].Before(stamps[j]) })

	for _, ts := range stamps {
		if !ts.After(from) {
			carried := ts
			leading = &carried
			continue
		}
		if n := len(inWindow); n > 0 && inWindow[n-1].Equal(ts) {
			continue
		}
		inWindow = append(inWindow, ts)
	}
	return leading, inWindow
}

// intervalsBetween measures the gaps between consecutive recorded instants.
// The leading instant contributes the first gap: an update inside the window
// following a state carried in from before it is a genuinely observed
// interval, and it is often the only one a narrow window can show.
func intervalsBetween(leading *time.Time, inWindow []time.Time) []time.Duration {
	prev := leading
	intervals := make([]time.Duration, 0, len(inWindow))
	for i := range inWindow {
		if prev != nil {
			if d := inWindow[i].Sub(*prev); d > 0 {
				intervals = append(intervals, d)
			}
		}
		prev = &inWindow[i]
	}
	return intervals
}

// nearestRank returns the p-th percentile of an ascending sample by the
// nearest-rank method: the smallest observed value at or above rank
// ceil(p×n). It reports a value the entity actually exhibited rather than
// interpolating between two samples, and it is defined for every n ≥ 1 —
// including the small samples where index arithmetic on p×n underflows to -1
// or overruns the slice.
func nearestRank(ascending []time.Duration, p float64) time.Duration {
	n := len(ascending)
	if n == 0 {
		return 0
	}
	rank := min(max(int(math.Ceil(p*float64(n))), 1), n)
	return ascending[rank-1]
}
