package analysis

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// windowStart is the fixture's window start; entity_history_7d.json records
// exactly seven days from here.
var windowStart = time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)

func at(d time.Duration) time.Time { return windowStart.Add(d) }

func point(d time.Duration, state string) model.HistoryPoint {
	return model.HistoryPoint{Timestamp: at(d), State: state}
}

// readFixtureHistory loads a captured history/history_during_period payload
// through the real mapper, so these metrics are asserted over the same values
// get_entity_history would produce, not over hand-built structs.
func readFixtureHistory(t *testing.T, name string, entityID model.EntityID) []model.HistoryPoint {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	points, err := ha.MapHistoryDuringPeriod(entityID, json.RawMessage(data))
	if err != nil {
		t.Fatalf("MapHistoryDuringPeriod: %v", err)
	}
	return points
}

// TestComputeAvailability_Fixture7d_MatchesDocExample asserts the doc §12.1
// availability numbers over a captured seven-day history: 412 state changes,
// 7 unavailable periods, 3h12m total and 54m longest — read through the real
// mapper from the fixture's epoch-seconds ("lu") minimal shape.
func TestComputeAvailability_Fixture7d_MatchesDocExample(t *testing.T) {
	points := readFixtureHistory(t, "entity_history_7d.json", "sensor.example")
	if len(points) != 413 {
		t.Fatalf("fixture has %d points, want 413", len(points))
	}

	rep, err := ComputeAvailability(windowStart, at(7*24*time.Hour), points)
	if err != nil {
		t.Fatalf("ComputeAvailability: %v", err)
	}

	if !rep.Computable || !rep.CoverageComplete {
		t.Fatalf("computable=%v coverageComplete=%v, want both true", rep.Computable, rep.CoverageComplete)
	}
	if rep.StateChanges != 412 {
		t.Errorf("state changes = %d, want 412", rep.StateChanges)
	}
	if rep.UnavailablePeriods != 7 {
		t.Errorf("unavailable periods = %d, want 7", rep.UnavailablePeriods)
	}
	if got := rep.TotalUnavailable; got != 3*time.Hour+12*time.Minute {
		t.Errorf("total unavailable = %s, want 3h12m", got)
	}
	if got := rep.LongestUnavailable; got != 54*time.Minute {
		t.Errorf("longest unavailable = %s, want 54m", got)
	}
	if rep.UnknownDuration != 0 {
		t.Errorf("unknown duration = %s, want 0", rep.UnknownDuration)
	}
	// 1 - 11520s/604800s, the value the fixture's own outages imply. The
	// doc's 0.982 is illustrative and not consistent with its 3h12m/7d.
	if want := 0.9809523809523809; math.Abs(rep.AvailabilityRatio-want) > 1e-9 {
		t.Errorf("availability ratio = %v, want %v", rep.AvailabilityRatio, want)
	}
	for _, o := range rep.Outages {
		if o.TruncatedStart || o.OpenEnded {
			t.Errorf("outage %v: truncated=%v openEnded=%v, want both false", o.From, o.TruncatedStart, o.OpenEnded)
		}
	}
}

func TestComputeAvailability_UnavailableAtWindowStart_LeadingOutageTruncated(t *testing.T) {
	// The recorder holds "unavailable" from before the window opened: the
	// outage is clamped to the window and marked as having begun earlier.
	points := []model.HistoryPoint{
		point(-2*time.Hour, "unavailable"),
		point(30*time.Minute, "20.0"),
	}

	rep, err := ComputeAvailability(windowStart, at(time.Hour), points)
	if err != nil {
		t.Fatalf("ComputeAvailability: %v", err)
	}
	if len(rep.Outages) != 1 {
		t.Fatalf("got %d outages, want 1", len(rep.Outages))
	}
	o := rep.Outages[0]
	if !o.From.Equal(windowStart) || o.Duration != 30*time.Minute {
		t.Errorf("outage = %v..%v (%s), want window start +30m", o.From, o.To, o.Duration)
	}
	if !o.TruncatedStart || o.OpenEnded {
		t.Errorf("truncated=%v openEnded=%v, want true/false", o.TruncatedStart, o.OpenEnded)
	}
	if !rep.CoverageComplete || rep.CoverageGap != 0 {
		t.Errorf("coverage gap = %s, want none: a state before the window covers it", rep.CoverageGap)
	}
	if rep.AvailabilityRatio != 0.5 {
		t.Errorf("availability ratio = %v, want 0.5", rep.AvailabilityRatio)
	}
	if rep.StateChanges != 1 {
		t.Errorf("state changes = %d, want 1", rep.StateChanges)
	}
}

func TestComputeAvailability_UnavailableAtWindowEnd_OutageOpenEnded(t *testing.T) {
	points := []model.HistoryPoint{
		point(0, "20.0"),
		point(45*time.Minute, "unavailable"),
	}

	rep, err := ComputeAvailability(windowStart, at(time.Hour), points)
	if err != nil {
		t.Fatalf("ComputeAvailability: %v", err)
	}
	if len(rep.Outages) != 1 {
		t.Fatalf("got %d outages, want 1", len(rep.Outages))
	}
	o := rep.Outages[0]
	if !o.OpenEnded || o.TruncatedStart {
		t.Errorf("openEnded=%v truncated=%v, want true/false", o.OpenEnded, o.TruncatedStart)
	}
	if o.Duration != 15*time.Minute {
		t.Errorf("outage duration = %s, want 15m", o.Duration)
	}
	if rep.AvailabilityRatio != 0.75 {
		t.Errorf("availability ratio = %v, want 0.75", rep.AvailabilityRatio)
	}
}

func TestComputeAvailability_UnavailableThenUnknown_CountsAsOnePeriod(t *testing.T) {
	// "unknown" is not "unavailable", but a run of both is one continuous
	// stretch of not reporting — one period, with the unknown share kept
	// separately visible.
	points := []model.HistoryPoint{
		point(0, "20.0"),
		point(20*time.Minute, "unavailable"),
		point(30*time.Minute, "unknown"),
		point(50*time.Minute, "20.1"),
	}

	rep, err := ComputeAvailability(windowStart, at(time.Hour), points)
	if err != nil {
		t.Fatalf("ComputeAvailability: %v", err)
	}
	if rep.UnavailablePeriods != 1 {
		t.Fatalf("unavailable periods = %d, want 1", rep.UnavailablePeriods)
	}
	if rep.TotalUnavailable != 30*time.Minute {
		t.Errorf("total unavailable = %s, want 30m", rep.TotalUnavailable)
	}
	if rep.UnknownDuration != 20*time.Minute {
		t.Errorf("unknown duration = %s, want 20m", rep.UnknownDuration)
	}
	if rep.StateChanges != 3 {
		t.Errorf("state changes = %d, want 3", rep.StateChanges)
	}
}

func TestComputeAvailability_RecorderGap_NotAnOutage(t *testing.T) {
	// Nothing was recorded for the window's first two hours — the recorder
	// purged it, or the entity did not exist yet. That is missing evidence:
	// the ratio is computed over what was observed, and the gap is reported
	// rather than charged as downtime.
	points := []model.HistoryPoint{
		point(2*time.Hour, "20.0"),
		point(3*time.Hour, "unavailable"),
		point(4*time.Hour, "20.1"),
	}

	rep, err := ComputeAvailability(windowStart, at(6*time.Hour), points)
	if err != nil {
		t.Fatalf("ComputeAvailability: %v", err)
	}
	if rep.CoverageComplete {
		t.Error("coverage reported complete, want incomplete")
	}
	if rep.CoverageGap != 2*time.Hour || !rep.CoveredFrom.Equal(at(2*time.Hour)) {
		t.Errorf("gap = %s from %v, want 2h from +2h", rep.CoverageGap, rep.CoveredFrom)
	}
	if rep.Covered != 4*time.Hour {
		t.Errorf("covered = %s, want 4h", rep.Covered)
	}
	if rep.UnavailablePeriods != 1 || rep.TotalUnavailable != time.Hour {
		t.Errorf("outages = %d totalling %s, want 1 totalling 1h", rep.UnavailablePeriods, rep.TotalUnavailable)
	}
	if rep.Outages[0].TruncatedStart {
		t.Error("outage marked truncated, want false: its start was observed")
	}
	// 1h of 4h observed, not of the 6h window.
	if rep.AvailabilityRatio != 0.75 {
		t.Errorf("availability ratio = %v, want 0.75", rep.AvailabilityRatio)
	}
}

func TestComputeAvailability_NoRecordedHistory_NotComputable(t *testing.T) {
	rep, err := ComputeAvailability(windowStart, at(time.Hour), nil)
	if err != nil {
		t.Fatalf("ComputeAvailability: %v", err)
	}
	if rep.Computable {
		t.Error("computable, want not: nothing was recorded")
	}
	if rep.AvailabilityRatio != 0 || rep.UnavailablePeriods != 0 {
		t.Errorf("ratio = %v, periods = %d, want zero values with Computable false",
			rep.AvailabilityRatio, rep.UnavailablePeriods)
	}
	if rep.CoverageGap != time.Hour || rep.CoverageComplete {
		t.Errorf("gap = %s complete = %v, want the whole window uncovered", rep.CoverageGap, rep.CoverageComplete)
	}
}

func TestComputeAvailability_ZeroStateChanges_FullyAvailable(t *testing.T) {
	// One recorded state held across the whole window: 100% available with
	// no changes, which is health, not missing data.
	points := []model.HistoryPoint{point(0, "20.0")}

	rep, err := ComputeAvailability(windowStart, at(24*time.Hour), points)
	if err != nil {
		t.Fatalf("ComputeAvailability: %v", err)
	}
	if rep.StateChanges != 0 || rep.UnavailablePeriods != 0 {
		t.Errorf("changes = %d, periods = %d, want 0/0", rep.StateChanges, rep.UnavailablePeriods)
	}
	if !rep.Computable || rep.AvailabilityRatio != 1 {
		t.Errorf("computable = %v ratio = %v, want true/1", rep.Computable, rep.AvailabilityRatio)
	}
}

func TestComputeAvailability_ZeroStateChangesUnavailable_FullOutage(t *testing.T) {
	points := []model.HistoryPoint{point(-time.Hour, "unavailable")}

	rep, err := ComputeAvailability(windowStart, at(24*time.Hour), points)
	if err != nil {
		t.Fatalf("ComputeAvailability: %v", err)
	}
	if rep.UnavailablePeriods != 1 || rep.TotalUnavailable != 24*time.Hour {
		t.Fatalf("outages = %d totalling %s, want 1 totalling 24h", rep.UnavailablePeriods, rep.TotalUnavailable)
	}
	o := rep.Outages[0]
	if !o.TruncatedStart || !o.OpenEnded {
		t.Errorf("truncated=%v openEnded=%v, want both true", o.TruncatedStart, o.OpenEnded)
	}
	if rep.AvailabilityRatio != 0 || !rep.Computable {
		t.Errorf("ratio = %v computable = %v, want 0/true", rep.AvailabilityRatio, rep.Computable)
	}
}

func TestComputeAvailability_WindowShorterThanUpdateInterval_UsesHeldState(t *testing.T) {
	// The entity updates hourly and the window is one minute wide: the only
	// evidence is the state held from before it, and that is enough.
	points := []model.HistoryPoint{point(-40*time.Minute, "20.0")}

	rep, err := ComputeAvailability(windowStart, at(time.Minute), points)
	if err != nil {
		t.Fatalf("ComputeAvailability: %v", err)
	}
	if !rep.Computable || rep.AvailabilityRatio != 1 || rep.StateChanges != 0 {
		t.Errorf("computable=%v ratio=%v changes=%d, want true/1/0",
			rep.Computable, rep.AvailabilityRatio, rep.StateChanges)
	}
	if !rep.CoverageComplete || rep.Covered != time.Minute {
		t.Errorf("covered = %s (complete=%v), want 1m complete", rep.Covered, rep.CoverageComplete)
	}
}

func TestComputeAvailability_UnorderedAndOutOfWindowPoints_Normalized(t *testing.T) {
	// HA data is untrusted (rule 6): unsorted points, a repeated state and a
	// point past the window must not distort the numbers.
	points := []model.HistoryPoint{
		point(90*time.Minute, "21.0"), // after `to` — ignored
		point(30*time.Minute, "unavailable"),
		point(0, "20.0"),
		point(45*time.Minute, "unavailable"), // repeat of the held state
	}

	rep, err := ComputeAvailability(windowStart, at(time.Hour), points)
	if err != nil {
		t.Fatalf("ComputeAvailability: %v", err)
	}
	if rep.StateChanges != 1 {
		t.Errorf("state changes = %d, want 1", rep.StateChanges)
	}
	if rep.UnavailablePeriods != 1 || rep.TotalUnavailable != 30*time.Minute {
		t.Errorf("outages = %d totalling %s, want 1 totalling 30m", rep.UnavailablePeriods, rep.TotalUnavailable)
	}
	if !rep.Outages[0].OpenEnded {
		t.Error("outage not open-ended: the window ends inside it")
	}
}

func TestComputeAvailability_EndNotAfterStart_Rejected(t *testing.T) {
	for _, tc := range []struct {
		name string
		to   time.Time
	}{
		{"equal", windowStart},
		{"reversed", at(-time.Hour)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ComputeAvailability(windowStart, tc.to, nil); !errors.Is(err, ErrInvalidWindow) {
				t.Fatalf("err = %v, want ErrInvalidWindow", err)
			}
		})
	}
}
