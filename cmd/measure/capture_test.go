package main

import (
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestAuditCapture_AuditRecord_Captured(t *testing.T) {
	c := &auditCapture{}
	log := slog.New(c)

	log.InfoContext(context.Background(), "audit",
		"tool", "find_stale_entities", "ha_requests", 7, "duration_ms", int64(420),
		"result_bytes", int64(9000), "status", "success")
	log.InfoContext(context.Background(), "starting", "version", "x")

	got := c.take()
	if len(got) != 1 {
		t.Fatalf("captured %d records, want 1 (only msg=audit)", len(got))
	}
	want := auditRow{Tool: "find_stale_entities", HARequests: 7, DurationMS: 420, ResultBytes: 9000, Status: "success"}
	if got[0] != want {
		t.Fatalf("got %+v, want %+v", got[0], want)
	}
	if len(c.take()) != 0 {
		t.Fatal("take must drain")
	}
}

func TestAuditCapture_Reason_Kept(t *testing.T) {
	c := &auditCapture{}
	slog.New(c).Info("audit", "tool", "t", "status", "budget_exceeded", "reason", "ha_requests")
	if got := c.take(); len(got) != 1 || got[0].Reason != "ha_requests" {
		t.Fatalf("reason lost: %+v", got)
	}
}

func TestPickWidest_OrdersByCountDescending_CapsAtN(t *testing.T) {
	in := []candidate{{label: "a", weight: 3}, {label: "b", weight: 90}, {label: "c", weight: 40}, {label: "d", weight: 0}}
	got := pickWidest(in, 2)
	if len(got) != 2 || got[0].label != "b" || got[1].label != "c" {
		t.Fatalf("got %+v", got)
	}
}

func TestPickWidest_SkipsZeroWeight(t *testing.T) {
	if got := pickWidest([]candidate{{label: "x"}}, 3); len(got) != 0 {
		t.Fatalf("zero-weight candidate picked: %+v", got)
	}
}

func TestSummarizeClusters_JoinsEvidenceToCluster(t *testing.T) {
	h := healthShape{
		Evidence: []evidenceShape{
			{ID: "ev1", Measurements: map[string]float64{"entities": 4, "outage_periods": 6, "span_seconds": 3600}},
			{ID: "ev2", Measurements: map[string]float64{"unrelated": 1}},
		},
		Clusters: []clusterShape{{Evidence: "ev1", Members: []string{"a", "b", "c", "d"}}},
	}
	rows := summarizeClusters(h, 7*24*3600)
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	r := rows[0]
	if r.Entities != 4 || r.OutagePeriods != 6 || r.SpanSeconds != 3600 {
		t.Fatalf("row = %+v", r)
	}
	if r.SpanShare < 0.0059 || r.SpanShare > 0.0060 {
		t.Fatalf("span share = %v", r.SpanShare)
	}
}

func TestSummarizeClusters_MissingEvidence_NotInvented(t *testing.T) {
	h := healthShape{Clusters: []clusterShape{{Evidence: "gone", Members: []string{"a", "b"}}}}
	rows := summarizeClusters(h, 3600)
	if len(rows) != 1 || rows[0].Entities != 2 || rows[0].SpanSeconds != 0 || rows[0].EvidenceFound {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestReport_NeverPrintsIdentifiers(t *testing.T) {
	r := &report{}
	r.callRow("analyze_integration_health", "integration #1 (120 entities)", callOutcome{
		audit: auditRow{Tool: "analyze_integration_health", HARequests: 12, DurationMS: 800, ResultBytes: 5000, Status: "success"},
	}, 50, 1<<20)
	if strings.Contains(r.String(), "config_entry") {
		t.Fatal("report must carry labels, not ids")
	}
	if !strings.Contains(r.String(), "12 / 50") {
		t.Fatalf("headroom missing: %s", r.String())
	}
}
