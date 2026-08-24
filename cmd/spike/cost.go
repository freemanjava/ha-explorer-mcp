package main

import "time"

// rungs are the entity counts P0-09's DoD names. 1 is the P0-07 baseline, 200
// is doc §10's per-tool entity cap; 10 and 50 sit where a per-room and a
// per-integration detector land.
var rungs = []int{1, 10, 50, 200}

// ladder clamps the rungs to what the installation actually has, so a run on a
// small installation still measures something instead of asking for ids that
// do not exist. The available count is always the top rung when it falls
// between two of them — the widest query the run can honestly make.
func ladder(available int) []int {
	var out []int
	for _, n := range rungs {
		if n > available {
			break
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil
	}
	if last := out[len(out)-1]; last < available && available < maxEntities {
		out = append(out, available)
	}
	return out
}

// sample is one measured answer: what it cost on the wire, in time, and in
// data points. Points are what doc §10's MaxHistoryPoints bounds, and they are
// the only one of the three that does not fall out of the transport.
type sample struct {
	bytes   int
	elapsed time.Duration
	points  int
}

// sumSamples adds up the first n single-entity calls — the baseline a batched
// call of the same width is compared against. Measuring every id once and
// summing prefixes keeps the run bounded: one pass over the ids answers every
// rung, instead of re-measuring the same entities at each one.
func sumSamples(s []sample, n int) sample {
	if n > len(s) {
		n = len(s)
	}
	var total sample
	for _, one := range s[:n] {
		total.bytes += one.bytes
		total.elapsed += one.elapsed
		total.points += one.points
	}
	return total
}

// countPoints sums the length of every per-entity array in a keyed answer.
//
// Both history/history_during_period and recorder/statistics_during_period
// answer with an object keyed by entity or statistic id whose values are
// arrays of points, so one counter serves both.
func countPoints(decoded any) int {
	m, ok := decoded.(map[string]any)
	if !ok {
		return 0
	}
	n := 0
	for _, v := range m {
		if arr, ok := v.([]any); ok {
			n += len(arr)
		}
	}
	return n
}
