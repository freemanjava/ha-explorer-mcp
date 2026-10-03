package mcp

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// investigationScenario is doc §13.1's installation: an automation that stopped
// in error while its one dependency was unavailable. The history is long enough
// (five clean runs before the failure, a trace older than the period) that the
// traced hypothesis ranks above "low", so a strictly lower fallback measures the
// Degraded demotion and nothing else.
type investigationScenario struct {
	points []model.HistoryPoint
	runs   []model.AutomationTraceSummary
	events []model.LogbookEvent
}

func newInvestigationScenario() investigationScenario {
	now := time.Now().UTC()
	points := append([]model.HistoryPoint{{Timestamp: now.Add(-30 * time.Hour), State: "on"}}, dependencyOutage()...)
	var runs []model.AutomationTraceSummary
	var events []model.LogbookEvent
	for i, h := range []int{10, 9, 8, 7, 6} {
		at := now.Add(-time.Duration(h) * time.Hour)
		points = append(points, model.HistoryPoint{Timestamp: at, State: "on"})
		runs = append(runs, model.AutomationTraceSummary{RunID: fmt.Sprint(i), ScriptExecution: "finished", TimestampStart: at, TimestampFinish: at})
		events = append(events, model.LogbookEvent{When: at, EntityID: testAutomation, ContextID: fmt.Sprint(i)})
	}
	slices.SortFunc(points, func(a, b model.HistoryPoint) int { return a.Timestamp.Compare(b.Timestamp) })
	runs = append(runs, model.AutomationTraceSummary{RunID: "old", ScriptExecution: "finished", TimestampStart: now.Add(-30 * time.Hour)})
	runs = append(runs, failedRunInsideOutage()...)
	events = append(events, model.LogbookEvent{When: now.Add(-150 * time.Minute), EntityID: testAutomation, ContextID: "failed"})
	return investigationScenario{points: points, runs: runs, events: events}
}

// walkInvestigation1 follows doc §13.1 over one server session, one tool per
// step, and returns the final analysis. Each step must answer: an investigation
// that dies at step two is not an investigation.
func walkInvestigation1(t *testing.T, opts Options) AutomationHealthResponse {
	t.Helper()
	client := connect(t, newServer(opts, Catalog()))
	now := time.Now().UTC()
	call := func(name string, args map[string]any) *sdkmcp.CallToolResult {
		t.Helper()
		res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.IsError {
			t.Fatalf("%s answered with an error: %s", name, resultText(res))
		}
		return res
	}
	call("get_automation", map[string]any{"entity_id": testAutomation})
	call("get_automation_traces", map[string]any{"entity_id": testAutomation})
	call("get_entity_history", map[string]any{
		"entity_id": "light.porch",
		"from":      now.Add(-24 * time.Hour).Format(time.RFC3339),
		"to":        now.Format(time.RFC3339),
	})
	call("list_repairs", map[string]any{})

	res := call("analyze_automation_health", map[string]any{"entity_id": testAutomation, "period": "24h"})
	var out AutomationHealthResponse
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal analysis: %v", err)
	}
	return out
}

// principal is what the caller may read of the automation surface. HA gates
// automation/config and trace/* together (P0-05), so the two refusals below are
// distinct only in tests: tracesRefused isolates the F-11 fallback's own
// demotion, nonAdmin is what a non-admin principal actually gets.
type principal int

const (
	admin principal = iota
	tracesRefused
	nonAdmin
)

func investigation1Options(s investigationScenario, who principal) Options {
	detail, inv := automationHealthFixture()
	detail.traces = s.runs
	automations := &fakeAutomationReader{automations: []model.AutomationSummary{{EntityID: testAutomation, Enabled: true}}}
	var logbook logbookReader
	if who != admin {
		if who == nonAdmin {
			detail.automationErr = fmt.Errorf("config: %w", ha.ErrUnsupported)
		}
		detail.tracesErr = fmt.Errorf("traces: %w", ha.ErrUnsupported)
		ranAt := time.Now().UTC().Add(-150 * time.Minute)
		automations.automations[0].LastTriggered = &ranAt
		logbook = &fakeLogbookReader{events: s.events}
	}
	repairs := &fakeRepairReader{repairs: []model.Repair{{IssueID: "i1", Domain: "hue", Severity: "warning", TranslationPlaceholders: map[string]any{}}}}
	return automationHealthOptions(&fakeHistoryReader{points: s.points}, inv, detail, automations, logbook, repairs, policy.Profile{})
}

var confidenceRank = map[string]int{"low": 0, "medium": 1, "high": 2}

func topConfidence(out AutomationHealthResponse) int {
	top := -1
	for _, h := range out.Hypotheses {
		top = max(top, confidenceRank[h.Confidence])
	}
	return top
}

// TestInvestigation1_HappyPath_RankedHypothesesCiteEvidence pins P5-07's first
// DoD clause: every hypothesis names evidence that exists in the response, and
// the one that overlaps the dependency outage outranks any without.
func TestInvestigation1_HappyPath_RankedHypothesesCiteEvidence(t *testing.T) {
	out := walkInvestigation1(t, investigation1Options(newInvestigationScenario(), admin))

	if len(out.Hypotheses) == 0 {
		t.Fatal("no hypotheses from a failed run inside a dependency outage")
	}
	for _, h := range out.Hypotheses {
		if len(h.Cites) == 0 {
			t.Errorf("hypothesis %q cites no evidence", h.Statement)
		}
		for _, id := range h.Cites {
			if evidenceByID(out.Evidence, id) == nil && !slices.ContainsFunc(out.DependencyEvidence, func(d DependencyEvidenceView) bool { return d.Evidence == id }) {
				t.Errorf("hypothesis %q cites %q, which is not in the response", h.Statement, id)
			}
		}
	}
	runs := evidenceByID(out.Evidence, "automation_runs")
	if runs == nil || runs.Degraded {
		t.Fatalf("run evidence = %+v, want traced (not degraded) runs", runs)
	}
	if slices.Contains(missingSources(out), "trace/list") {
		t.Errorf("traces were readable but missing_evidence names them: %+v", out.MissingEvidence)
	}
}

// TestInvestigation1_DegradedBranch_LowerConfidenceAndNamesTraces pins the F-11
// clause: with traces refused to the principal the walk still answers, from
// last_triggered + logbook, names what is absent, and ranks strictly below the
// same scenario with traces (a comparison, never a fixed level).
func TestInvestigation1_DegradedBranch_LowerConfidenceAndNamesTraces(t *testing.T) {
	s := newInvestigationScenario()
	traced := walkInvestigation1(t, investigation1Options(s, admin))
	degraded := walkInvestigation1(t, investigation1Options(s, tracesRefused))

	if topConfidence(traced) < 0 || topConfidence(degraded) < 0 {
		t.Fatalf("both branches must yield a hypothesis: traced=%+v degraded=%+v", traced.Hypotheses, degraded.Hypotheses)
	}
	if topConfidence(degraded) >= topConfidence(traced) {
		t.Errorf("degraded confidence rank %d is not strictly below traced %d", topConfidence(degraded), topConfidence(traced))
	}
	if !slices.Contains(missingSources(degraded), "trace/list") {
		t.Errorf("degraded missing_evidence does not name trace/list: %+v", degraded.MissingEvidence)
	}
	if runs := evidenceByID(degraded.Evidence, "automation_runs"); runs == nil || !runs.Degraded {
		t.Errorf("degraded run evidence = %+v, want Degraded", runs)
	}
	if !degraded.Partial {
		t.Error("a degraded answer must be marked partial")
	}
}

// TestInvestigation1_NonAdmin_NamesBothGatedSourcesAndInventsNoHypothesis pins
// what the real non-admin principal gets: with automation/config refused there
// is no dependency list to overlay runs on, so the walk answers with the absent
// sources named and no hypothesis (rule 7) — see F-34.
func TestInvestigation1_NonAdmin_NamesBothGatedSourcesAndInventsNoHypothesis(t *testing.T) {
	out := walkInvestigation1(t, investigation1Options(newInvestigationScenario(), nonAdmin))

	for _, want := range []string{"automation/config", "trace/list"} {
		if !slices.Contains(missingSources(out), want) {
			t.Errorf("missing_evidence does not name %s: %+v", want, out.MissingEvidence)
		}
	}
	if len(out.Hypotheses) != 0 {
		t.Errorf("hypotheses without a dependency list: %+v", out.Hypotheses)
	}
	if !out.Partial {
		t.Error("a non-admin answer must be marked partial")
	}
}
