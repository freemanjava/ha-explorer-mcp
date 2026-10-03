package main

import "slices"

// candidate is something worth measuring against: a label safe to print
// (an ordinal and a size, never an id) next to the id the call needs.
type candidate struct {
	label  string
	id     string
	weight int
}

// pickWidest returns up to n candidates, widest first. "Realistic installation
// width" is the question being measured, so the widest integrations are the
// ones that expose a budget that is too small; a zero weight is skipped
// because measuring an empty target says nothing about width.
func pickWidest(in []candidate, n int) []candidate {
	var kept []candidate
	for _, c := range in {
		if c.weight > 0 {
			kept = append(kept, c)
		}
	}
	slices.SortStableFunc(kept, func(a, b candidate) int { return b.weight - a.weight })
	if len(kept) > n {
		kept = kept[:n]
	}
	return kept
}

// The shapes below decode only the fields the F-28 observation needs from an
// analyze_* response. They are local on purpose: the response types carry no
// JSON tags, and decoding into a minimal shape keeps this command from
// depending on how the envelope happens to be laid out.
type evidenceShape struct {
	ID           string
	Measurements map[string]float64
}

type clusterShape struct {
	Evidence string
	Members  []string
}

type healthShape struct {
	Evidence []evidenceShape
	Clusters []clusterShape
}

// clusterRow is one cluster's size and span — what F-28 asks about: does a long
// outage chain unrelated ones into a cluster far wider than its members?
type clusterRow struct {
	Entities      int
	OutagePeriods int
	SpanSeconds   float64
	// SpanShare is the cluster's span over the analysed period. A cluster
	// spanning most of the period with more periods than entities is the
	// chaining signature.
	SpanShare     float64
	EvidenceFound bool
}

func summarizeClusters(h healthShape, periodSeconds float64) []clusterRow {
	byID := make(map[string]evidenceShape, len(h.Evidence))
	for _, e := range h.Evidence {
		byID[e.ID] = e
	}
	rows := make([]clusterRow, 0, len(h.Clusters))
	for _, c := range h.Clusters {
		row := clusterRow{Entities: len(c.Members)}
		if e, ok := byID[c.Evidence]; ok {
			row.EvidenceFound = true
			row.OutagePeriods = int(e.Measurements["outage_periods"])
			row.SpanSeconds = e.Measurements["span_seconds"]
			if periodSeconds > 0 {
				row.SpanShare = row.SpanSeconds / periodSeconds
			}
		}
		rows = append(rows, row)
	}
	return rows
}
