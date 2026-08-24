package main

import (
	"testing"
	"time"
)

func TestLadder_EnoughEntities_AllRungs(t *testing.T) {
	assertRungs(t, ladder(200), []int{1, 10, 50, 200})
	assertRungs(t, ladder(640), []int{1, 10, 50, 200})
}

func TestLadder_BetweenRungs_TopsOutAtWhatExists(t *testing.T) {
	// 73 entities: the widest honest query is 73, not a skipped rung.
	assertRungs(t, ladder(73), []int{1, 10, 50, 73})
}

func TestLadder_TooFewForAnyRung_Empty(t *testing.T) {
	if got := ladder(0); got != nil {
		t.Fatalf("ladder(0) = %v, want nil", got)
	}
}

func TestSumSamples_PrefixOnly(t *testing.T) {
	s := []sample{
		{bytes: 10, elapsed: time.Second, points: 1},
		{bytes: 20, elapsed: 2 * time.Second, points: 2},
		{bytes: 40, elapsed: 4 * time.Second, points: 4},
	}

	got := sumSamples(s, 2)

	if got.bytes != 30 || got.elapsed != 3*time.Second || got.points != 3 {
		t.Fatalf("sumSamples(_, 2) = %+v, want {30 3s 3}", got)
	}
}

func TestSumSamples_MoreThanMeasured_ClampsToWhatExists(t *testing.T) {
	// A rung wider than the baseline run must not silently report a short sum
	// as if it covered every id.
	got := sumSamples([]sample{{bytes: 10}}, 50)

	if got.bytes != 10 {
		t.Fatalf("bytes = %d, want 10", got.bytes)
	}
}

func TestCountPoints_KeyedArrays_Summed(t *testing.T) {
	decoded := map[string]any{
		"sensor.power":  []any{1, 2, 3},
		"sensor.energy": []any{1, 2},
	}

	if got := countPoints(decoded); got != 5 {
		t.Fatalf("countPoints() = %d, want 5", got)
	}
}

func TestCountPoints_UnexpectedShape_ReportsZero(t *testing.T) {
	// An HA upgrade changing this shape must degrade to "no points counted",
	// never panic a probe mid-run.
	if got := countPoints([]any{1, 2, 3}); got != 0 {
		t.Fatalf("countPoints() = %d, want 0", got)
	}
	if got := countPoints(map[string]any{"sensor.power": "not an array"}); got != 0 {
		t.Fatalf("countPoints() = %d, want 0", got)
	}
}

func assertRungs(t *testing.T, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ladder = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ladder = %v, want %v", got, want)
		}
	}
}
