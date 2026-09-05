package analysis

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// regularSeries builds n points spaced exactly `every` apart, starting at
// windowStart+offset, alternating state so every point is also a change.
func regularSeries(offset, every time.Duration, n int) []model.HistoryPoint {
	pts := make([]model.HistoryPoint, 0, n)
	for i := range n {
		state := "20.0"
		if i%2 == 1 {
			state = "20.1"
		}
		pts = append(pts, point(offset+time.Duration(i)*every, state))
	}
	return pts
}

func TestComputeCadence_RegularUpdates_MedianAndP95MatchInterval(t *testing.T) {
	// 120 points exactly 30s apart across the first hour of a 1h window: a
	// perfectly regular entity, still updating at the end.
	points := regularSeries(0, 30*time.Second, 121)

	rep, err := ComputeCadence(windowStart, at(time.Hour), points)
	if err != nil {
		t.Fatalf("ComputeCadence: %v", err)
	}
	if !rep.Computable {
		t.Fatal("not computable, want computable")
	}
	if rep.Updates != 120 || rep.Intervals != 120 {
		t.Errorf("updates = %d, intervals = %d, want 120/120", rep.Updates, rep.Intervals)
	}
	if rep.MedianUpdateInterval != 30*time.Second || rep.P95UpdateInterval != 30*time.Second {
		t.Errorf("median = %s p95 = %s, want 30s/30s", rep.MedianUpdateInterval, rep.P95UpdateInterval)
	}
	if rep.MinUpdateInterval != 30*time.Second || rep.MaxUpdateInterval != 30*time.Second {
		t.Errorf("min = %s max = %s, want 30s/30s", rep.MinUpdateInterval, rep.MaxUpdateInterval)
	}
	if rep.StateChanges != 120 {
		t.Errorf("state changes = %d, want 120", rep.StateChanges)
	}
	if got, want := rep.StateChangeRatePerHour, 120.0; math.Abs(got-want) > 1e-9 {
		t.Errorf("state-change rate = %v/h, want %v/h", got, want)
	}
	if rep.SilentFor != 0 || rep.Stale {
		t.Errorf("silent for %s, stale = %v; want 0s and not stale", rep.SilentFor, rep.Stale)
	}
	if !rep.StaleJudgeable {
		t.Error("staleness not judgeable, want judgeable: 120 observed intervals")
	}
}

func TestComputeCadence_FastEntitySilentForAnHour_Stale(t *testing.T) {
	// Normally every 30s, last seen an hour before the window's end: far
	// beyond anything this entity has ever done.
	points := regularSeries(0, 30*time.Second, 121)

	rep, err := ComputeCadence(windowStart, at(2*time.Hour), points)
	if err != nil {
		t.Fatalf("ComputeCadence: %v", err)
	}
	if !rep.Stale {
		t.Errorf("not stale; silent %s against p95 %s", rep.SilentFor, rep.P95UpdateInterval)
	}
	if rep.SilentFor != time.Hour {
		t.Errorf("silent for = %s, want 1h", rep.SilentFor)
	}
	if rep.StaleThreshold != staleIntervalFactor*30*time.Second {
		t.Errorf("threshold = %s, want %s", rep.StaleThreshold, staleIntervalFactor*30*time.Second)
	}
	if want := float64(time.Hour) / float64(30*time.Second); math.Abs(rep.StalenessRatio-want) > 1e-9 {
		t.Errorf("staleness ratio = %v, want %v", rep.StalenessRatio, want)
	}
	if !rep.LastUpdate.Equal(at(time.Hour)) {
		t.Errorf("last update = %v, want %v", rep.LastUpdate, at(time.Hour))
	}
}

func TestComputeCadence_HourlyEntitySilentForHalfAnHour_NotStale(t *testing.T) {
	// The DoD's counter-case: an entity that legitimately updates hourly must
	// not be called stale for a gap smaller than its own cadence. A global
	// constant threshold — "nothing for 15 minutes is stale" — would flag it.
	points := regularSeries(0, time.Hour, 25) // 24h of hourly updates

	rep, err := ComputeCadence(windowStart, at(24*time.Hour+30*time.Minute), points)
	if err != nil {
		t.Fatalf("ComputeCadence: %v", err)
	}
	if rep.MedianUpdateInterval != time.Hour || rep.P95UpdateInterval != time.Hour {
		t.Errorf("median = %s p95 = %s, want 1h/1h", rep.MedianUpdateInterval, rep.P95UpdateInterval)
	}
	if rep.SilentFor != 30*time.Minute {
		t.Errorf("silent for = %s, want 30m", rep.SilentFor)
	}
	if rep.Stale {
		t.Errorf("stale, want not: %s of silence is inside this entity's own cadence", rep.SilentFor)
	}
}

func TestComputeCadence_IrregularUpdates_PercentilesSeparate(t *testing.T) {
	// Mostly a minute apart with two long gaps: the median stays at the
	// typical interval while p95 reports the tail, which is the whole reason
	// both are computed.
	var points []model.HistoryPoint
	offset := time.Duration(0)
	for i := range 20 {
		points = append(points, point(offset, "20."+string(rune('0'+i%10))))
		if i == 7 || i == 15 {
			offset += 20 * time.Minute
			continue
		}
		offset += time.Minute
	}

	rep, err := ComputeCadence(windowStart, at(2*time.Hour), points)
	if err != nil {
		t.Fatalf("ComputeCadence: %v", err)
	}
	if rep.Intervals != 19 {
		t.Fatalf("intervals = %d, want 19", rep.Intervals)
	}
	if rep.MedianUpdateInterval != time.Minute {
		t.Errorf("median = %s, want 1m", rep.MedianUpdateInterval)
	}
	if rep.P95UpdateInterval != 20*time.Minute {
		t.Errorf("p95 = %s, want 20m", rep.P95UpdateInterval)
	}
	if rep.MaxUpdateInterval != 20*time.Minute || rep.MinUpdateInterval != time.Minute {
		t.Errorf("min = %s max = %s, want 1m/20m", rep.MinUpdateInterval, rep.MaxUpdateInterval)
	}
}

func TestComputeCadence_BurstyUpdates_NotStaleBetweenBursts(t *testing.T) {
	// A motion sensor: bursts of one-second updates separated by quiet hours.
	// p95 sits inside a burst, but the quiet stretches are this entity's
	// normal too — the tail interval, not the median, is what staleness is
	// judged against, so a gap the entity has shown before is not stale.
	var points []model.HistoryPoint
	for burst := range 3 {
		base := time.Duration(burst) * 2 * time.Hour
		for i := range 10 {
			state := "on"
			if i%2 == 1 {
				state = "off"
			}
			points = append(points, point(base+time.Duration(i)*time.Second, state))
		}
	}

	rep, err := ComputeCadence(windowStart, at(5*time.Hour), points)
	if err != nil {
		t.Fatalf("ComputeCadence: %v", err)
	}
	if rep.MedianUpdateInterval != time.Second {
		t.Errorf("median = %s, want 1s: most intervals are inside a burst", rep.MedianUpdateInterval)
	}
	if rep.P95UpdateInterval != 2*time.Hour-9*time.Second {
		t.Errorf("p95 = %s, want the between-burst gap", rep.P95UpdateInterval)
	}
	// Silent for an hour after the last burst — long against the median, but
	// half of what this entity does between bursts.
	if rep.SilentFor != time.Hour-9*time.Second {
		t.Errorf("silent for = %s, want ~1h", rep.SilentFor)
	}
	if rep.Stale {
		t.Error("stale, want not: the gap is shorter than the entity's own between-burst interval")
	}
}

func TestComputeCadence_SmallSamples_PercentilesWellDefined(t *testing.T) {
	// Percentiles at n=1..4, where naive implementations index out of range,
	// truncate to -1, or average two samples into a value that never
	// occurred. Nearest-rank reports an observed interval, always.
	for _, tc := range []struct {
		name        string
		gaps        []time.Duration
		median, p95 time.Duration
		intervals   int
	}{
		{"one interval", []time.Duration{time.Minute}, time.Minute, time.Minute, 1},
		{"two intervals", []time.Duration{time.Minute, 3 * time.Minute}, time.Minute, 3 * time.Minute, 2},
		{"three intervals", []time.Duration{time.Minute, 5 * time.Minute, 3 * time.Minute}, 3 * time.Minute, 5 * time.Minute, 3},
		{"four intervals", []time.Duration{4 * time.Minute, time.Minute, 3 * time.Minute, 2 * time.Minute}, 2 * time.Minute, 4 * time.Minute, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			points := []model.HistoryPoint{point(0, "s0")}
			offset := time.Duration(0)
			for i, g := range tc.gaps {
				offset += g
				points = append(points, point(offset, "s"+string(rune('1'+i))))
			}

			rep, err := ComputeCadence(windowStart, at(offset), points)
			if err != nil {
				t.Fatalf("ComputeCadence: %v", err)
			}
			if rep.Intervals != tc.intervals {
				t.Fatalf("intervals = %d, want %d", rep.Intervals, tc.intervals)
			}
			if rep.MedianUpdateInterval != tc.median {
				t.Errorf("median = %s, want %s", rep.MedianUpdateInterval, tc.median)
			}
			if rep.P95UpdateInterval != tc.p95 {
				t.Errorf("p95 = %s, want %s", rep.P95UpdateInterval, tc.p95)
			}
		})
	}
}

func TestComputeCadence_SingleUpdate_NotComputableButSilenceReported(t *testing.T) {
	// One point yields no interval: cadence is unknown, so staleness cannot
	// be judged against it. The silence is still a fact and is reported —
	// "could not check" is not "fine" (CLAUDE.md rule 7).
	points := []model.HistoryPoint{point(10*time.Minute, "20.0")}

	rep, err := ComputeCadence(windowStart, at(time.Hour), points)
	if err != nil {
		t.Fatalf("ComputeCadence: %v", err)
	}
	if rep.Computable || rep.StaleJudgeable || rep.Stale {
		t.Errorf("computable=%v judgeable=%v stale=%v, want all false",
			rep.Computable, rep.StaleJudgeable, rep.Stale)
	}
	if rep.MedianUpdateInterval != 0 || rep.P95UpdateInterval != 0 || rep.StalenessRatio != 0 {
		t.Error("percentiles or ratio non-zero with no observed interval")
	}
	if rep.Updates != 1 || !rep.LastUpdate.Equal(at(10*time.Minute)) {
		t.Errorf("updates = %d last = %v, want 1 at +10m", rep.Updates, rep.LastUpdate)
	}
	if rep.SilentFor != 50*time.Minute {
		t.Errorf("silent for = %s, want 50m", rep.SilentFor)
	}
}

func TestComputeCadence_NoUpdatesInWindow_SilenceFromLastKnownUpdate(t *testing.T) {
	// The recorder holds one state from before the window and nothing since:
	// the entity has not updated for the whole window plus what preceded it.
	points := []model.HistoryPoint{point(-3*time.Hour, "20.0")}

	rep, err := ComputeCadence(windowStart, at(time.Hour), points)
	if err != nil {
		t.Fatalf("ComputeCadence: %v", err)
	}
	if rep.Updates != 0 || rep.Computable || rep.StaleJudgeable {
		t.Errorf("updates = %d computable = %v judgeable = %v, want 0/false/false",
			rep.Updates, rep.Computable, rep.StaleJudgeable)
	}
	if rep.SilentFor != 4*time.Hour {
		t.Errorf("silent for = %s, want 4h measured from the pre-window state", rep.SilentFor)
	}
	if rep.StateChanges != 0 || rep.StateChangeRatePerHour != 0 {
		t.Errorf("changes = %d rate = %v, want 0/0", rep.StateChanges, rep.StateChangeRatePerHour)
	}
}

func TestComputeCadence_NoHistoryAtAll_NothingObserved(t *testing.T) {
	rep, err := ComputeCadence(windowStart, at(time.Hour), nil)
	if err != nil {
		t.Fatalf("ComputeCadence: %v", err)
	}
	if rep.Observed || rep.Computable || rep.StaleJudgeable || rep.Stale {
		t.Errorf("observed=%v computable=%v judgeable=%v stale=%v, want all false",
			rep.Observed, rep.Computable, rep.StaleJudgeable, rep.Stale)
	}
	if rep.SilentFor != 0 || !rep.LastUpdate.IsZero() {
		t.Errorf("silent for = %s last = %v, want zero: silence needs a last update to measure from",
			rep.SilentFor, rep.LastUpdate)
	}
}

func TestComputeCadence_LeadingPointGivesFirstInterval(t *testing.T) {
	// The state held from before the window and the first update inside it
	// bound a genuinely observed interval — the only one available when the
	// window is narrower than the entity's cadence.
	points := []model.HistoryPoint{
		point(-20*time.Minute, "20.0"),
		point(10*time.Minute, "20.1"),
	}

	rep, err := ComputeCadence(windowStart, at(15*time.Minute), points)
	if err != nil {
		t.Fatalf("ComputeCadence: %v", err)
	}
	if rep.Intervals != 1 || rep.MedianUpdateInterval != 30*time.Minute {
		t.Fatalf("intervals = %d median = %s, want 1/30m", rep.Intervals, rep.MedianUpdateInterval)
	}
	if rep.Updates != 1 {
		t.Errorf("updates = %d, want 1: the pre-window point is evidence, not an in-window update", rep.Updates)
	}
	if rep.Stale {
		t.Error("stale, want not: 5m of silence against a 30m cadence")
	}
}

func TestComputeCadence_UnorderedDuplicateAndOutOfWindowPoints_Normalized(t *testing.T) {
	// Untrusted input (rule 6): unsorted points, two records at the same
	// instant, and a point past the window. A duplicate timestamp must not
	// enter the sample as a zero interval and drag the median to nothing.
	points := []model.HistoryPoint{
		point(90*time.Minute, "21.0"), // after `to` — not evidence for this window
		point(20*time.Minute, "20.1"),
		point(20*time.Minute, "20.2"), // same instant as the previous
		point(0, "20.0"),
		point(40*time.Minute, "20.3"),
	}

	rep, err := ComputeCadence(windowStart, at(time.Hour), points)
	if err != nil {
		t.Fatalf("ComputeCadence: %v", err)
	}
	if rep.Intervals != 2 || rep.MinUpdateInterval != 20*time.Minute {
		t.Errorf("intervals = %d min = %s, want 2/20m", rep.Intervals, rep.MinUpdateInterval)
	}
	if rep.MedianUpdateInterval != 20*time.Minute {
		t.Errorf("median = %s, want 20m", rep.MedianUpdateInterval)
	}
	if rep.SilentFor != 20*time.Minute {
		t.Errorf("silent for = %s, want 20m", rep.SilentFor)
	}
}

func TestComputeCadence_EndNotAfterStart_Rejected(t *testing.T) {
	if _, err := ComputeCadence(windowStart, windowStart, nil); !errors.Is(err, ErrInvalidWindow) {
		t.Fatalf("err = %v, want ErrInvalidWindow", err)
	}
}

// TestComputeCadence_Fixture7d_RealPayload runs the cadence metrics over the
// same captured seven-day history P4-02's availability numbers come from, read
// through the real mapper: 412 observed intervals, median 1376.5s and p95
// 2578.8s. Doc §12.1's own 31s/104s cannot coexist with its 412 changes over
// 7d (that is ~3.5h of samples, not a week) — see F-24; the computation is the
// contract.
func TestComputeCadence_Fixture7d_RealPayload(t *testing.T) {
	points := readFixtureHistory(t, "entity_history_7d.json", "sensor.example")

	rep, err := ComputeCadence(windowStart, at(7*24*time.Hour), points)
	if err != nil {
		t.Fatalf("ComputeCadence: %v", err)
	}
	if rep.Intervals != 412 || rep.Updates != 412 {
		t.Fatalf("intervals = %d updates = %d, want 412/412", rep.Intervals, rep.Updates)
	}
	if !rep.Computable || !rep.StaleJudgeable {
		t.Fatalf("computable = %v judgeable = %v, want both true", rep.Computable, rep.StaleJudgeable)
	}
	near := func(name string, got, want time.Duration) {
		t.Helper()
		if d := got - want; d > time.Second || d < -time.Second {
			t.Errorf("%s = %s, want ~%s", name, got, want)
		}
	}
	near("median", rep.MedianUpdateInterval, 1376500*time.Millisecond)
	near("p95", rep.P95UpdateInterval, 2578800*time.Millisecond)
	near("min", rep.MinUpdateInterval, 660*time.Second)
	near("max", rep.MaxUpdateInterval, 3240*time.Second)
	if rep.MedianUpdateInterval > rep.P95UpdateInterval || rep.P95UpdateInterval > rep.MaxUpdateInterval {
		t.Errorf("percentiles out of order: median %s p95 %s max %s",
			rep.MedianUpdateInterval, rep.P95UpdateInterval, rep.MaxUpdateInterval)
	}
	if rep.StateChanges != 412 {
		t.Errorf("state changes = %d, want 412 (P4-02 counts the same transitions)", rep.StateChanges)
	}
	if rep.Stale {
		t.Errorf("stale, want not: %s of silence against p95 %s", rep.SilentFor, rep.P95UpdateInterval)
	}
}
