package mcp

import (
	"context"
	"encoding/json"
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

// layout is how the fixture installation's devices hang off a parent.
type layout int

const (
	// partialParent: a hub parents two of the entry's three leaf devices; the
	// third is attached directly, so sharing the hub distinguishes (F-27).
	partialParent layout = iota
	// noParent: leaf devices with no via_device at all.
	noParent
	// coordinatorStar: every other device of the entry names the coordinator,
	// so sharing it says no more than sharing the config entry (F-27).
	coordinatorStar
)

// meshShape is how the entry exposes its mesh metrics. It is a property of
// the registry, never of a platform name (D-05-5, rule 6).
type meshShape int

const (
	// metricExposed: an enabled link-quality sensor per device (Zigbee2MQTT-shaped).
	metricExposed meshShape = iota
	// metricDisabled: the link-quality sensor exists but is disabled (ZHA-shaped).
	metricDisabled
)

// outageHistory serves the shared outage window to the entities in outage,
// a numeric link quality to metric sensors, and a steady "on" to the rest.
type outageHistory struct{ outage map[string]bool }

func (h outageHistory) History(_ context.Context, id model.EntityID, _, _ time.Time, _ bool) ([]model.HistoryPoint, error) {
	switch {
	case h.outage[string(id)]:
		return dependencyOutage(), nil
	case strings.HasSuffix(string(id), "_linkquality") || strings.HasSuffix(string(id), "_lqi"):
		return []model.HistoryPoint{{Timestamp: time.Now().Add(-6 * 24 * time.Hour), State: "87"}}, nil
	}
	return []model.HistoryPoint{{Timestamp: time.Now().Add(-6 * 24 * time.Hour), State: "on"}}, nil
}

// meshInstallation builds one config entry "entry-1" of three leaf devices
// whose first two lights are unavailable together, wired per layout and shape.
func meshInstallation(l layout, shape meshShape) (*fakeInventoryReader, []string) {
	inv := &fakeInventoryReader{integrations: []model.Integration{{ID: "entry-1", Domain: "mesh", State: "loaded"}}}
	parent := model.DeviceID("")
	switch l {
	case partialParent, coordinatorStar:
		parent = "hub"
		inv.devices = append(inv.devices, model.DeviceRef{ID: parent, ConfigEntryID: "entry-1"})
	}
	var down []string
	for i := range 3 {
		dev := model.DeviceID(fmt.Sprintf("dev%d", i))
		via := parent
		if l == partialParent && i == 2 {
			via = ""
		}
		inv.devices = append(inv.devices, model.DeviceRef{ID: dev, ConfigEntryID: "entry-1", ViaDeviceID: via})
		light := fmt.Sprintf("light.leaf_%d", i)
		sensor := model.Entity{ID: model.EntityID(fmt.Sprintf("sensor.leaf_%d_linkquality", i)), Domain: "sensor", ConfigEntryID: "entry-1", DeviceID: dev}
		if shape == metricDisabled {
			sensor.DisabledBy = "integration"
		}
		inv.entities = append(inv.entities,
			model.Entity{ID: model.EntityID(light), Domain: "light", ConfigEntryID: "entry-1", DeviceID: dev}, sensor)
		if i < 2 {
			down = append(down, light)
		}
	}
	return inv, down
}

type investigation2 struct {
	unavailable model.UnavailableEntityList
	integration HealthResponse
	entity      HealthResponse
}

// walkInvestigation2 follows doc §13.2 over one server session: find the
// unavailable entities, analyze their integration, then a clustered member.
// Its "restart evidence" step is walkInvestigation3's.
func walkInvestigation2(t *testing.T, l layout, shape meshShape) investigation2 {
	t.Helper()
	inv, down := meshInstallation(l, shape)
	opts := integrationHealthOptions(outageHistory{outage: map[string]bool{down[0]: true, down[1]: true}}, inv, downReader(down...), &fakeRepairReader{}, nil)
	client := connect(t, newServer(opts, Catalog()))
	call := func(name string, args map[string]any, into any) {
		t.Helper()
		res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.IsError {
			t.Fatalf("%s answered with an error: %s", name, resultText(res))
		}
		raw, _ := json.Marshal(res.StructuredContent)
		if err := json.Unmarshal(raw, into); err != nil {
			t.Fatalf("%s: unmarshal: %v", name, err)
		}
	}
	var out investigation2
	call("find_unavailable_entities", map[string]any{}, &out.unavailable)
	call("analyze_integration_health", map[string]any{"config_entry_id": "entry-1"}, &out.integration)
	if len(out.integration.Clusters) == 0 || len(out.integration.Clusters[0].Members) == 0 {
		t.Fatalf("step 2 found no cluster to take a member from: %+v", out.integration.Clusters)
	}
	member := out.integration.Clusters[0].Members[0]
	call("analyze_entity_health", map[string]any{"entity_id": string(member)}, &out.entity)
	return out
}

func hasTrait(traits []model.ClusterTrait, kind model.TraitKind) bool {
	return slices.ContainsFunc(traits, func(tr model.ClusterTrait) bool { return tr.Kind == kind })
}

func assertHypothesesCiteEnvelope(t *testing.T, name string, out HealthResponse) {
	t.Helper()
	for _, h := range out.Hypotheses {
		if len(h.Cites) == 0 {
			t.Errorf("%s: hypothesis %q cites no evidence", name, h.Statement)
		}
		for _, id := range h.Cites {
			if evidenceByID(out.Evidence, id) == nil {
				t.Errorf("%s: hypothesis %q cites %q, which is not in its envelope", name, h.Statement, id)
			}
		}
	}
}

func TestInvestigation2_PartialParent_ClusterCarriesTopologyClaim(t *testing.T) {
	out := walkInvestigation2(t, partialParent, metricExposed)

	if len(out.unavailable.Items) != 2 {
		t.Errorf("step 1 listed %d unavailable entities, want the two in outage", len(out.unavailable.Items))
	}
	c := out.integration.Clusters[0]
	if i := slices.IndexFunc(c.Shared, func(tr model.ClusterTrait) bool { return tr.Kind == model.TraitViaDevice }); i < 0 || c.Shared[i].Value != "hub" {
		t.Errorf("shared = %+v, want via_device hub", c.Shared)
	}
	if hasTrait(c.Withheld, model.TraitViaDevice) {
		t.Errorf("a parent of part of its entry is withheld: %+v", c.Withheld)
	}
	if evidenceByID(out.integration.Evidence, c.Evidence) == nil {
		t.Errorf("cluster names %q, which is not in the evidence", c.Evidence)
	}
	assertHypothesesCiteEnvelope(t, "integration", out.integration)
	assertHypothesesCiteEnvelope(t, "entity", out.entity)
}

func TestInvestigation2_NoParent_SameTimeClusterWithoutTopologyClaim(t *testing.T) {
	out := walkInvestigation2(t, noParent, metricExposed)

	c := out.integration.Clusters[0]
	if len(c.Members) != 2 {
		t.Fatalf("members = %v, want the same two-entity time cluster", c.Members)
	}
	if hasTrait(c.Shared, model.TraitViaDevice) || hasTrait(c.Withheld, model.TraitViaDevice) {
		t.Errorf("topology claimed with no parent: shared %+v withheld %+v", c.Shared, c.Withheld)
	}
}

func TestInvestigation2_CoordinatorStar_ViaDeviceWithheldNotShared(t *testing.T) {
	out := walkInvestigation2(t, coordinatorStar, metricExposed)

	c := out.integration.Clusters[0]
	if hasTrait(c.Shared, model.TraitViaDevice) {
		t.Errorf("the coordinator star is presented as a finding: %+v", c.Shared)
	}
	if !hasTrait(c.Withheld, model.TraitViaDevice) {
		t.Errorf("withheld = %+v, want via_device named there", c.Withheld)
	}
}

func TestInvestigation2_Mesh_ExposedHasEvidenceDisabledNamesReason(t *testing.T) {
	exposed := walkInvestigation2(t, partialParent, metricExposed)
	if !slices.ContainsFunc(exposed.integration.Evidence, func(e model.Evidence) bool { return strings.HasPrefix(string(e.ID), "mesh_link_quality_") }) {
		t.Errorf("no mesh evidence for the exposed metric: %+v", exposed.integration.Evidence)
	}

	disabled := walkInvestigation2(t, partialParent, metricDisabled)
	if slices.ContainsFunc(disabled.integration.Evidence, func(e model.Evidence) bool { return strings.HasPrefix(string(e.ID), "mesh_") }) {
		t.Errorf("mesh evidence invented for a disabled metric: %+v", disabled.integration.Evidence)
	}
	if !slices.ContainsFunc(disabled.integration.MissingEvidence, func(m model.MissingEvidence) bool { return m.Reason == model.MissingEntityDisabled }) {
		t.Errorf("the disabled metric is not named with its reason: %+v", disabled.integration.MissingEvidence)
	}
}

func TestInvestigation2_Cluster_NamesHostEvidenceAsPrivileged(t *testing.T) {
	out := walkInvestigation2(t, partialParent, metricExposed)

	if !slices.ContainsFunc(out.integration.MissingEvidence, func(m model.MissingEvidence) bool {
		return m.Source == "host" && m.Reason == model.MissingPrivileged
	}) {
		t.Errorf("host evidence is not named as privileged: %+v", out.integration.MissingEvidence)
	}
	if !slices.ContainsFunc(out.integration.MissingEvidence, func(m model.MissingEvidence) bool { return m.Reason == model.MissingNotExposed }) {
		t.Errorf("the neighbour table is not named as not exposed: %+v", out.integration.MissingEvidence)
	}
}

// fakeLifecycleReader serves Home Assistant start/stop rows the way the real
// window read does: only those inside [from, to]. It records every window it
// was asked for, so a test can pin that no read was wider than a probe.
type fakeLifecycleReader struct {
	events  []model.LifecycleEvent
	err     error
	windows [][2]time.Time
}

func (f *fakeLifecycleReader) LifecycleEvents(_ context.Context, from, to time.Time) ([]model.LifecycleEvent, error) {
	f.windows = append(f.windows, [2]time.Time{from, to})
	if f.err != nil {
		return nil, f.err
	}
	var out []model.LifecycleEvent
	for _, e := range f.events {
		if !e.When.Before(from) && !e.When.After(to) {
			out = append(out, e)
		}
	}
	return out, nil
}

// walkInvestigation3 follows doc §21's third investigation over one server
// session: a batch of entities goes unavailable together; find them, analyze
// their integration (clustering, shared config entry, restart evidence,
// repairs). Both fixtures are the same installation and the same outage; only
// the logbook differs.
func walkInvestigation3(t *testing.T, lifecycle *fakeLifecycleReader) (HealthResponse, *fakeLifecycleReader) {
	t.Helper()
	inv, down := meshInstallation(noParent, metricExposed)
	opts := integrationHealthOptions(outageHistory{outage: map[string]bool{down[0]: true, down[1]: true}}, inv, downReader(down...),
		&fakeRepairReader{repairs: []model.Repair{{IssueID: "i1", Domain: "mesh", Severity: "warning", TranslationPlaceholders: map[string]any{}}}}, nil)
	if lifecycle != nil {
		opts.Lifecycle = lifecycle
	}
	client := connect(t, newServer(opts, Catalog()))
	call := func(name string, args map[string]any, into any) {
		t.Helper()
		res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.IsError {
			t.Fatalf("%s answered with an error: %s", name, resultText(res))
		}
		raw, _ := json.Marshal(res.StructuredContent)
		if err := json.Unmarshal(raw, into); err != nil {
			t.Fatalf("%s: unmarshal: %v", name, err)
		}
	}
	var unavailable model.UnavailableEntityList
	var out HealthResponse
	call("find_unavailable_entities", map[string]any{}, &unavailable)
	if len(unavailable.Items) != 2 {
		t.Fatalf("step 1 listed %d unavailable entities, want the two in outage", len(unavailable.Items))
	}
	call("analyze_integration_health", map[string]any{"config_entry_id": "entry-1"}, &out)
	if len(out.Clusters) == 0 {
		t.Fatalf("step 2 found no cluster: %+v", out.Evidence)
	}
	assertHypothesesCiteEnvelope(t, "integration", out)
	return out, lifecycle
}

// outageOnset is when dependencyOutage's entities went down.
func outageOnset() time.Time { return time.Now().UTC().Add(-3 * time.Hour) }

func hypothesisTexts(out HealthResponse) string {
	var b strings.Builder
	for _, h := range out.Hypotheses {
		b.WriteString(h.Statement + "\n")
	}
	return b.String()
}

func TestInvestigation3_RestartVersusIntegrationFailure_DistinguishedByEvidence(t *testing.T) {
	restarted, _ := walkInvestigation3(t, &fakeLifecycleReader{events: []model.LifecycleEvent{
		{When: outageOnset().Add(-40 * time.Second)}, {When: outageOnset().Add(90 * time.Second)},
	}})
	failed, _ := walkInvestigation3(t, &fakeLifecycleReader{events: []model.LifecycleEvent{
		{When: outageOnset().Add(-2 * time.Hour)},
	}})

	const leg = "restart_outage_cluster_1"
	if ev := evidenceByID(restarted.Evidence, leg); ev == nil || ev.Measurements["lifecycle_events"] != 2 {
		t.Fatalf("restart fixture evidence = %+v, want two lifecycle events", ev)
	}
	if ev := evidenceByID(failed.Evidence, leg); ev == nil || ev.Measurements["lifecycle_events"] != 0 {
		t.Fatalf("failure fixture evidence = %+v, want a measured zero (a row two hours away is not near)", ev)
	}
	if !strings.Contains(hypothesisTexts(restarted), "start or stop") || strings.Contains(hypothesisTexts(restarted), "shared upstream") {
		t.Errorf("restart fixture hypotheses:\n%s", hypothesisTexts(restarted))
	}
	if !strings.Contains(hypothesisTexts(failed), "shared upstream") || strings.Contains(hypothesisTexts(failed), "start or stop") {
		t.Errorf("failure fixture hypotheses:\n%s", hypothesisTexts(failed))
	}
	for name, out := range map[string]HealthResponse{"restarted": restarted, "failed": failed} {
		if !slices.ContainsFunc(out.Hypotheses, func(h HypothesisView) bool { return slices.Contains(h.Cites, leg) }) {
			t.Errorf("%s: no hypothesis cites the restart leg", name)
		}
	}
}

func TestInvestigation3_LogbookRead_BoundedToProbeWindowNeverThePeriod(t *testing.T) {
	_, reader := walkInvestigation3(t, &fakeLifecycleReader{})

	if len(reader.windows) != 1 {
		t.Fatalf("logbook reads = %d, want one per cluster", len(reader.windows))
	}
	if w := reader.windows[0]; w[1].Sub(w[0]) > 10*time.Minute {
		t.Errorf("window %v..%v is wider than a probe: unfiltered, it would pull the whole logbook", w[0], w[1])
	}
}

func TestInvestigation3_LogbookUnreadable_NamedMissingNeverReportedAsNoRestart(t *testing.T) {
	out, _ := walkInvestigation3(t, &fakeLifecycleReader{err: fmt.Errorf("logbook: %w", ha.ErrUpstreamUnavailable)})

	if evidenceByID(out.Evidence, "restart_outage_cluster_1") != nil {
		t.Error("restart evidence invented from a failed read")
	}
	if !slices.ContainsFunc(out.MissingEvidence, func(m model.MissingEvidence) bool { return m.Source == "logbook" }) {
		t.Errorf("the unread logbook is not named: %+v", out.MissingEvidence)
	}
	if !out.Partial {
		t.Error("an answer missing its restart evidence must be marked partial")
	}
}

func TestInvestigation3_NoLifecycleReader_NamedMissing(t *testing.T) {
	out, _ := walkInvestigation3(t, nil)

	if !slices.ContainsFunc(out.MissingEvidence, func(m model.MissingEvidence) bool { return m.Source == "logbook" }) {
		t.Errorf("a build without the logbook reader does not say so: %+v", out.MissingEvidence)
	}
}

// TestDocCriterion_ThreeInvestigationsProduceEvidenceBackedRankedHypotheses is
// doc §21's criterion — "at least three end-to-end investigations produce
// evidence-backed ranked hypotheses" — pinned by running all three, so it
// cannot regress by one of them quietly going silent.
func TestDocCriterion_ThreeInvestigationsProduceEvidenceBackedRankedHypotheses(t *testing.T) {
	automation := walkInvestigation1(t, investigation1Options(newInvestigationScenario(), admin))
	if len(automation.Hypotheses) == 0 {
		t.Error("investigation 1 (§13.1, automation failed during a dependency outage) produced no hypothesis")
	}
	for _, h := range automation.Hypotheses {
		if len(h.Cites) == 0 {
			t.Errorf("investigation 1: hypothesis %q cites nothing", h.Statement)
		}
	}

	mesh := walkInvestigation2(t, partialParent, metricExposed)
	if len(mesh.integration.Hypotheses) == 0 {
		t.Error("investigation 2 (§13.2, unreachable mesh devices) produced no hypothesis")
	}
	assertHypothesesCiteEnvelope(t, "investigation 2", mesh.integration)

	restart, _ := walkInvestigation3(t, &fakeLifecycleReader{events: []model.LifecycleEvent{{When: outageOnset()}}})
	if len(restart.Hypotheses) == 0 {
		t.Error("investigation 3 (§21, mass unavailability vs. restart) produced no hypothesis")
	}
	assertHypothesesCiteEnvelope(t, "investigation 3", restart)
}
