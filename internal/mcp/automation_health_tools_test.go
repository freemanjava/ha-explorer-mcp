package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

const testAutomation = "automation.evening_lights"

// dependencyOutage is a dependency's history with one closed outage, three to
// two hours before now, inside the default 7d period.
func dependencyOutage() []model.HistoryPoint {
	now := time.Now().UTC()
	return []model.HistoryPoint{
		{Timestamp: now.Add(-24 * time.Hour), State: "on"},
		{Timestamp: now.Add(-3 * time.Hour), State: "unavailable"},
		{Timestamp: now.Add(-2 * time.Hour), State: "on"},
	}
}

// failedRunInsideOutage is a run that stopped in error 150 minutes ago, inside
// dependencyOutage's window.
func failedRunInsideOutage() []model.AutomationTraceSummary {
	at := time.Now().UTC().Add(-150 * time.Minute)
	return []model.AutomationTraceSummary{
		{RunID: "r1", ScriptExecution: "error", TimestampStart: at, TimestampFinish: at.Add(time.Second)},
	}
}

func automationHealthFixture() (*fakeAutomationDetailReader, *fakeInventoryReader) {
	detail := &fakeAutomationDetailReader{
		automation: model.Automation{
			EntityID:  testAutomation,
			DependsOn: model.AutomationDependencies{Entities: []model.EntityID{"light.porch"}},
		},
		traces: failedRunInsideOutage(),
	}
	inv := &fakeInventoryReader{entities: []model.Entity{{ID: "light.porch", Platform: "hue"}}}
	return detail, inv
}

func automationHealthOptions(history historyReader, inv systemInventoryReader, detail automationDetailReader,
	automations automationReader, logbook logbookReader, repairs repairReader, profile policy.Profile) Options {
	opts := entityHealthOptions(history, inv, repairs, profile)
	opts.AutomationDetail = detail
	opts.Automations = automations
	opts.Logbook = logbook
	return opts
}

func callAnalyzeAutomationHealth(t *testing.T, opts Options, args map[string]any) (*sdkmcp.CallToolResult, AutomationHealthResponse) {
	t.Helper()
	client := connect(t, newServer(opts, Catalog()))
	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "analyze_automation_health", Arguments: args})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		return res, AutomationHealthResponse{}
	}
	var out AutomationHealthResponse
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return res, out
}

func missingSources(out AutomationHealthResponse) []string {
	var sources []string
	for _, m := range out.MissingEvidence {
		sources = append(sources, m.Source)
	}
	return sources
}

func TestAnalyzeAutomationHealth_FailedRunInsideDependencyOutage_HypothesisCitesBoth(t *testing.T) {
	detail, inv := automationHealthFixture()
	opts := automationHealthOptions(&fakeHistoryReader{points: dependencyOutage()}, inv, detail, nil, nil, &fakeRepairReader{}, policy.Profile{})

	res, out := callAnalyzeAutomationHealth(t, opts, map[string]any{"entity_id": testAutomation})
	if res.IsError {
		t.Fatalf("error result: %s", resultText(res))
	}
	if len(out.DependencyEvidence) != 1 || out.DependencyEvidence[0].EntityID != "light.porch" {
		t.Fatalf("dependency evidence = %+v, want light.porch measured", out.DependencyEvidence)
	}
	depID := out.DependencyEvidence[0].Evidence
	if !slices.ContainsFunc(out.Hypotheses, func(h HypothesisView) bool {
		return slices.Contains(h.Cites, depID) && slices.Contains(h.Cites, "automation_runs")
	}) {
		t.Errorf("no hypothesis cites the run and the dependency window: %+v", out.Hypotheses)
	}
	if out.SubjectID != testAutomation || out.Source == "" || out.ObservedAt.IsZero() {
		t.Errorf("provenance incomplete: %+v", out)
	}
}

func TestAnalyzeAutomationHealth_NonAdmin_FallbackAnswersAndNamesWhatIsMissing(t *testing.T) {
	detail, inv := automationHealthFixture()
	detail.automationErr = fmt.Errorf("config: %w", ha.ErrUnsupported)
	detail.tracesErr = fmt.Errorf("traces: %w", ha.ErrUnsupported)
	ranAt := time.Now().UTC().Add(-150 * time.Minute)
	automations := &fakeAutomationReader{automations: []model.AutomationSummary{{EntityID: testAutomation, Enabled: true, LastTriggered: &ranAt}}}
	logbook := &fakeLogbookReader{events: []model.LogbookEvent{{When: ranAt, EntityID: testAutomation, ContextID: "c1"}}}
	opts := automationHealthOptions(&fakeHistoryReader{points: dependencyOutage()}, inv, detail, automations, logbook, &fakeRepairReader{}, policy.Profile{})

	res, out := callAnalyzeAutomationHealth(t, opts, map[string]any{"entity_id": testAutomation})
	if res.IsError {
		t.Fatalf("a non-admin principal must still get an answer: %s", resultText(res))
	}
	sources := missingSources(out)
	for _, want := range []string{"automation/config", "trace/list"} {
		if !slices.Contains(sources, want) {
			t.Errorf("missing_evidence does not name %s: %+v", want, out.MissingEvidence)
		}
	}
	if !out.Partial {
		t.Error("a response with missing evidence must be marked partial")
	}
	runs := evidenceByID(out.Evidence, "automation_runs")
	if runs == nil || !runs.Degraded || runs.SampleSize != 1 {
		t.Fatalf("run evidence = %+v, want one degraded logbook-counted run", runs)
	}
}

// TestAnalyzeAutomationHealth_Fallback_ConfidenceStrictlyBelowTraces runs the
// same scenario with traces and through the logbook fallback. The samples are
// large enough (and the period short enough for the 24h logbook window to cover
// it) that the traced hypothesis ranks above "low", so the comparison measures
// the fallback's Degraded demotion and nothing else.
func TestAnalyzeAutomationHealth_Fallback_ConfidenceStrictlyBelowTraces(t *testing.T) {
	rank := map[string]int{"low": 0, "medium": 1, "high": 2}
	best := func(out AutomationHealthResponse) int {
		top := -1
		for _, h := range out.Hypotheses {
			top = max(top, rank[h.Confidence])
		}
		return top
	}
	now := time.Now().UTC()
	points := append([]model.HistoryPoint{{Timestamp: now.Add(-30 * time.Hour), State: "on"}}, dependencyOutage()...)
	for h := 10; h >= 6; h-- {
		points = append(points, model.HistoryPoint{Timestamp: now.Add(-time.Duration(h) * time.Hour), State: "on"})
	}
	slices.SortFunc(points, func(a, b model.HistoryPoint) int { return a.Timestamp.Compare(b.Timestamp) })

	var runs []model.AutomationTraceSummary
	var events []model.LogbookEvent
	for i, h := range []int{10, 9, 8, 7, 6} {
		at := now.Add(-time.Duration(h) * time.Hour)
		runs = append(runs, model.AutomationTraceSummary{RunID: fmt.Sprint(i), ScriptExecution: "finished", TimestampStart: at, TimestampFinish: at})
		events = append(events, model.LogbookEvent{When: at, EntityID: testAutomation, ContextID: fmt.Sprint(i)})
	}
	// A trace older than the period shows the stored traces reach back over all
	// of it, so the traced run evidence has full coverage.
	runs = append(runs, model.AutomationTraceSummary{RunID: "old", ScriptExecution: "finished", TimestampStart: now.Add(-30 * time.Hour)})
	runs = append(runs, failedRunInsideOutage()...)
	events = append(events, model.LogbookEvent{When: now.Add(-150 * time.Minute), EntityID: testAutomation, ContextID: "failed"})
	args := map[string]any{"entity_id": testAutomation, "period": "24h"}

	detail, inv := automationHealthFixture()
	detail.traces = runs
	_, traced := callAnalyzeAutomationHealth(t, automationHealthOptions(&fakeHistoryReader{points: points}, inv, detail, nil, nil, nil, policy.Profile{}), args)

	detail2, inv2 := automationHealthFixture()
	detail2.tracesErr = ha.ErrUnsupported
	_, fallback := callAnalyzeAutomationHealth(t, automationHealthOptions(&fakeHistoryReader{points: points}, inv2, detail2, nil, &fakeLogbookReader{events: events}, nil, policy.Profile{}), args)

	if best(traced) < 0 || best(fallback) < 0 {
		t.Fatalf("both scenarios must yield a hypothesis: traced=%+v fallback=%+v", traced.Hypotheses, fallback.Hypotheses)
	}
	if best(fallback) >= best(traced) {
		t.Errorf("fallback confidence rank %d is not strictly below traced %d", best(fallback), best(traced))
	}
}

func TestAnalyzeAutomationHealth_DeviceAndAreaDependencies_ResolvedToEntities(t *testing.T) {
	detail, inv := automationHealthFixture()
	detail.automation.DependsOn = model.AutomationDependencies{
		Devices: []model.DeviceID{"dev1"},
		Areas:   []model.AreaID{"hall"},
	}
	inv.entities = []model.Entity{
		{ID: "light.via_device", DeviceID: "dev1"},
		{ID: "light.in_area", AreaID: "hall"},
		{ID: "light.via_device_area", DeviceID: "dev2"},
		{ID: "light.elsewhere", AreaID: "garage"},
	}
	inv.devices = []model.DeviceRef{{ID: "dev1"}, {ID: "dev2", AreaID: "hall"}}
	history := &countingHistoryReader{points: dependencyOutage()}
	opts := automationHealthOptions(history, inv, detail, nil, nil, nil, policy.Profile{})

	if res, _ := callAnalyzeAutomationHealth(t, opts, map[string]any{"entity_id": testAutomation}); res.IsError {
		t.Fatalf("error result: %s", resultText(res))
	}
	slices.Sort(history.read)
	want := []string{"light.in_area", "light.via_device", "light.via_device_area"}
	if !slices.Equal(history.read, want) {
		t.Errorf("history read for %v, want %v", history.read, want)
	}
}

func TestAnalyzeAutomationHealth_ManyDependencies_ReadBoundedAndRestNamed(t *testing.T) {
	detail, inv := automationHealthFixture()
	detail.automation.DependsOn.Entities = nil
	for i := range maxClusterEntities + 5 {
		detail.automation.DependsOn.Entities = append(detail.automation.DependsOn.Entities, model.EntityID(fmt.Sprintf("sensor.dep_%02d", i)))
	}
	history := &countingHistoryReader{points: dependencyOutage()}
	opts := automationHealthOptions(history, inv, detail, nil, nil, nil, policy.Profile{})

	_, out := callAnalyzeAutomationHealth(t, opts, map[string]any{"entity_id": testAutomation})
	if history.calls != maxClusterEntities {
		t.Errorf("history reads = %d, want the cap %d", history.calls, maxClusterEntities)
	}
	if !slices.ContainsFunc(out.MissingEvidence, func(m model.MissingEvidence) bool {
		return m.What == "dependency history" && m.Reason == model.MissingBudgetExceeded
	}) {
		t.Errorf("the unread remainder is not named: %+v", out.MissingEvidence)
	}
}

func TestAnalyzeAutomationHealth_PrivateDependency_DenyProfile_ExcludedNotRead(t *testing.T) {
	detail, inv := automationHealthFixture()
	detail.automation.DependsOn.Entities = []model.EntityID{"light.porch", "lock.front_door"}
	history := &countingHistoryReader{points: dependencyOutage()}
	opts := automationHealthOptions(history, inv, detail, nil, nil, nil, policy.Profile{Private: policy.HandlingDeny})

	_, out := callAnalyzeAutomationHealth(t, opts, map[string]any{"entity_id": testAutomation})
	if slices.Contains(history.read, "lock.front_door") {
		t.Fatalf("history was read for a private dependency: %v", history.read)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "lock.front_door") {
		t.Errorf("a private dependency's id leaked into the response: %s", raw)
	}
	if !slices.ContainsFunc(out.MissingEvidence, func(m model.MissingEvidence) bool { return m.Reason == model.MissingPolicyDenied }) {
		t.Errorf("the exclusion is not named: %+v", out.MissingEvidence)
	}
}

func TestAnalyzeAutomationHealth_HistoryFails_NamedMissingAndStillAnswers(t *testing.T) {
	detail, inv := automationHealthFixture()
	opts := automationHealthOptions(&fakeHistoryReader{err: errors.New("db password=hunter2")}, inv, detail, nil, nil, nil, policy.Profile{})

	res, out := callAnalyzeAutomationHealth(t, opts, map[string]any{"entity_id": testAutomation})
	if res.IsError {
		t.Fatalf("a failed dependency read must not fail the call: %s", resultText(res))
	}
	if !slices.Contains(missingSources(out), "recorder_history") {
		t.Errorf("unread history is not named: %+v", out.MissingEvidence)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "hunter2") {
		t.Errorf("an upstream error string reached the response: %s", raw)
	}
}

func TestAnalyzeAutomationHealth_NoDetailReader_ReportedMissing(t *testing.T) {
	_, inv := automationHealthFixture()
	opts := automationHealthOptions(&fakeHistoryReader{}, inv, nil, nil, nil, nil, policy.Profile{})
	opts.AutomationDetail = nil

	res, out := callAnalyzeAutomationHealth(t, opts, map[string]any{"entity_id": testAutomation})
	if res.IsError {
		t.Fatalf("error result: %s", resultText(res))
	}
	for _, want := range []string{"automation/config", "trace/list"} {
		if !slices.Contains(missingSources(out), want) {
			t.Errorf("missing_evidence does not name %s: %+v", want, out.MissingEvidence)
		}
	}
}

func TestAnalyzeAutomationHealth_UnknownAutomation_NotFound(t *testing.T) {
	detail, inv := automationHealthFixture()
	automations := &fakeAutomationReader{automations: []model.AutomationSummary{{EntityID: "automation.other"}}}
	opts := automationHealthOptions(&fakeHistoryReader{}, inv, detail, automations, nil, nil, policy.Profile{})

	res, _ := callAnalyzeAutomationHealth(t, opts, map[string]any{"entity_id": testAutomation})
	if !res.IsError || !strings.Contains(resultText(res), "not found") {
		t.Errorf("an automation the principal cannot see must be not-found, got %q", resultText(res))
	}
}

func TestAnalyzeAutomationHealth_InvalidInput_Refused(t *testing.T) {
	detail, inv := automationHealthFixture()
	opts := automationHealthOptions(&fakeHistoryReader{}, inv, detail, nil, nil, nil, policy.Profile{})
	for name, args := range map[string]map[string]any{
		"wrong domain":   {"entity_id": "light.porch"},
		"bare domain":    {"entity_id": "automation."},
		"period too big": {"entity_id": testAutomation, "period": "30d"},
		"bad period":     {"entity_id": testAutomation, "period": "soon"},
	} {
		if res, _ := callAnalyzeAutomationHealth(t, opts, args); !res.IsError {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestAnalyzeAutomationHealth_EmptyLists_SerializeAsArrays(t *testing.T) {
	detail, inv := automationHealthFixture()
	detail.automation.DependsOn = model.AutomationDependencies{}
	detail.traces = nil
	opts := automationHealthOptions(&fakeHistoryReader{}, inv, detail, nil, nil, &fakeRepairReader{}, policy.Profile{})

	res, _ := callAnalyzeAutomationHealth(t, opts, map[string]any{"entity_id": testAutomation})
	raw, _ := json.Marshal(res.StructuredContent)
	if strings.Contains(string(raw), "null") {
		t.Errorf("an empty list serialized as null: %s", raw)
	}
}

// TestAnalyzeAutomationHealth_ParityRule pins the four clauses: typed input,
// budget class, provenance (above) and no free-form parameter.
func TestAnalyzeAutomationHealth_ParityRule(t *testing.T) {
	tool, ok := lookup(Catalog(), "analyze_automation_health")
	if !ok || tool.Class != policy.ClassComposite {
		t.Fatalf("catalog row = %+v, want composite budget class", tool)
	}
	detail, inv := automationHealthFixture()
	opts := automationHealthOptions(&fakeHistoryReader{}, inv, detail, nil, nil, nil, policy.Profile{})
	client := connect(t, newServer(opts, Catalog()))
	res, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tl := range res.Tools {
		if tl.Name != "analyze_automation_health" {
			continue
		}
		raw, _ := json.Marshal(tl.InputSchema)
		var schema struct {
			AdditionalProperties any            `json:"additionalProperties"`
			Properties           map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		if schema.AdditionalProperties != false {
			t.Errorf("additionalProperties = %v, want false", schema.AdditionalProperties)
		}
		for name := range schema.Properties {
			if name != "entity_id" && name != "period" {
				t.Errorf("unexpected input property %q", name)
			}
		}
		return
	}
	t.Fatal("analyze_automation_health not listed")
}

func evidenceByID(evidence []model.Evidence, id model.EvidenceID) *model.Evidence {
	for i := range evidence {
		if evidence[i].ID == id {
			return &evidence[i]
		}
	}
	return nil
}
