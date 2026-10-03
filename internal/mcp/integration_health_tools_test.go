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

func integrationHealthInventory(state string, n int) *fakeInventoryReader {
	inv := &fakeInventoryReader{integrations: []model.Integration{
		{ID: "entry-hue", Domain: "hue", State: state},
		{ID: "entry-other", Domain: "zha", State: "loaded"},
	}}
	for i := range n {
		inv.entities = append(inv.entities, model.Entity{
			ID: model.EntityID(fmt.Sprintf("light.hue_%02d", i)), ConfigEntryID: "entry-hue", Platform: "hue", DeviceID: "dev1",
		})
	}
	inv.entities = append(inv.entities, model.Entity{ID: "light.other", ConfigEntryID: "entry-other", Platform: "zha"})
	inv.devices = []model.DeviceRef{{ID: "dev1", ConfigEntryID: "entry-hue"}}
	return inv
}

func integrationHealthOptions(history historyReader, inv systemInventoryReader, avail entityAvailabilityReader, repairs repairReader, sup systemHealthReader) Options {
	opts := entityHealthOptions(history, inv, repairs, policy.Profile{})
	opts.Availability = avail
	opts.Supervisor = sup
	return opts
}

func callAnalyzeIntegrationHealth(t *testing.T, opts Options, args map[string]any) (*sdkmcp.CallToolResult, HealthResponse) {
	t.Helper()
	client := connect(t, newServer(opts, Catalog()))
	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "analyze_integration_health", Arguments: args})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		return res, HealthResponse{}
	}
	var out HealthResponse
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return res, out
}

func downReader(ids ...string) *fakeAvailabilityReader {
	set := map[model.EntityID]struct{}{}
	for _, id := range ids {
		set[model.EntityID(id)] = struct{}{}
	}
	return &fakeAvailabilityReader{unavailable: set}
}

func TestAnalyzeIntegrationHealth_ClusterOfItsEntities_CitedByHypothesis(t *testing.T) {
	opts := integrationHealthOptions(&fakeHistoryReader{points: flapping(40)}, integrationHealthInventory("loaded", 3),
		downReader(), &fakeRepairReader{}, &fakeSupervisorReader{})

	res, out := callAnalyzeIntegrationHealth(t, opts, map[string]any{"config_entry_id": "entry-hue"})
	if res.IsError {
		t.Fatalf("error result: %s", resultText(res))
	}
	var cluster model.EvidenceID
	for _, ev := range out.Evidence {
		if strings.HasPrefix(string(ev.ID), "outage_cluster_") {
			cluster = ev.ID
		}
	}
	if cluster == "" {
		t.Fatalf("no cluster evidence: %+v", out.Evidence)
	}
	if !slices.ContainsFunc(out.Hypotheses, func(h HypothesisView) bool { return slices.Contains(h.Cites, cluster) }) {
		t.Errorf("no hypothesis cites %q: %+v", cluster, out.Hypotheses)
	}
	if out.SubjectID != "entry-hue" || out.Source == "" || out.ObservedAt.IsZero() {
		t.Errorf("provenance incomplete: %+v", out)
	}
}

func TestAnalyzeIntegrationHealth_OtherIntegrationsEntities_NeverRead(t *testing.T) {
	history := &fakeHistoryReader{points: flapping(4)}
	opts := integrationHealthOptions(history, integrationHealthInventory("loaded", 1), downReader(), &fakeRepairReader{}, nil)
	callAnalyzeIntegrationHealth(t, opts, map[string]any{"config_entry_id": "entry-hue"})
	if history.lastEntityID == "light.other" {
		t.Error("history was read for an entity of another config entry")
	}
}

func TestAnalyzeIntegrationHealth_SupervisorAbsent_NamedMissingAndStillAnswers(t *testing.T) {
	cases := map[string]systemHealthReader{
		"not configured": nil,
		"refused":        &fakeSupervisorReader{resolutionErr: fmt.Errorf("%w: x", ha.ErrUnsupported)},
		"unreachable":    &fakeSupervisorReader{resolutionErr: fmt.Errorf("%w: x", ha.ErrUpstreamUnavailable)},
	}
	for name, sup := range cases {
		t.Run(name, func(t *testing.T) {
			opts := integrationHealthOptions(&fakeHistoryReader{points: flapping(4)}, integrationHealthInventory("setup_retry", 2),
				downReader(), &fakeRepairReader{}, sup)
			res, out := callAnalyzeIntegrationHealth(t, opts, map[string]any{"config_entry_id": "entry-hue"})
			if res.IsError {
				t.Fatalf("a missing Supervisor failed the call: %s", resultText(res))
			}
			if !slices.ContainsFunc(out.MissingEvidence, func(m model.MissingEvidence) bool { return m.What == "supervisor host health" }) {
				t.Errorf("Supervisor gap not named: %+v", out.MissingEvidence)
			}
			if !out.Partial || len(out.Hypotheses) == 0 {
				t.Errorf("partial=%v hypotheses=%d, want a partial answer that still ranks", out.Partial, len(out.Hypotheses))
			}
		})
	}
}

func TestAnalyzeIntegrationHealth_SupervisorPresent_EvidenceWithoutNames(t *testing.T) {
	sup := &fakeSupervisorReader{resolution: model.ResolutionSummary{IssueCount: 1, Unhealthy: []string{"leak-me"}}}
	opts := integrationHealthOptions(&fakeHistoryReader{points: flapping(4)}, integrationHealthInventory("setup_retry", 2),
		downReader(), &fakeRepairReader{}, sup)
	_, out := callAnalyzeIntegrationHealth(t, opts, map[string]any{"config_entry_id": "entry-hue"})

	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "leak-me") {
		t.Error("a Supervisor-supplied condition name reached the response")
	}
	if !slices.ContainsFunc(out.Evidence, func(e model.Evidence) bool { return e.Source == "supervisor" }) {
		t.Error("no Supervisor-sourced evidence")
	}
}

func TestAnalyzeIntegrationHealth_DegradedSources_NeverFailTheCall(t *testing.T) {
	cases := []struct {
		name    string
		history error
		repairs error
		want    model.MissingReason
	}{
		{"history deadline", fmt.Errorf("%w: x", ha.ErrDeadline), nil, model.MissingDeadline},
		{"history unsupported", fmt.Errorf("%w: x", ha.ErrUnsupported), nil, model.MissingUnsupported},
		{"repairs upstream", nil, fmt.Errorf("%w: x", ha.ErrUpstreamUnavailable), model.MissingUpstreamUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := integrationHealthOptions(&fakeHistoryReader{points: flapping(10), err: tc.history}, integrationHealthInventory("loaded", 2),
				downReader(), &fakeRepairReader{err: tc.repairs}, &fakeSupervisorReader{})
			res, out := callAnalyzeIntegrationHealth(t, opts, map[string]any{"config_entry_id": "entry-hue"})
			if res.IsError {
				t.Fatalf("degraded source failed the call: %s", resultText(res))
			}
			if !slices.ContainsFunc(out.MissingEvidence, func(m model.MissingEvidence) bool { return m.Reason == tc.want }) {
				t.Errorf("missing_evidence = %+v, want reason %q", out.MissingEvidence, tc.want)
			}
		})
	}
}

func TestAnalyzeIntegrationHealth_ManyEntities_ReadBoundedAndTruncationNamed(t *testing.T) {
	history := &countingHistoryReader{points: flapping(4)}
	opts := integrationHealthOptions(history, integrationHealthInventory("loaded", 80), downReader(), &fakeRepairReader{}, nil)
	res, out := callAnalyzeIntegrationHealth(t, opts, map[string]any{"config_entry_id": "entry-hue"})
	if res.IsError {
		t.Fatalf("error: %s", resultText(res))
	}
	if history.calls > maxClusterEntities {
		t.Errorf("history reads = %d, want at most %d", history.calls, maxClusterEntities)
	}
	if !slices.ContainsFunc(out.MissingEvidence, func(m model.MissingEvidence) bool { return m.Reason == model.MissingBudgetExceeded }) {
		t.Errorf("the unread entities are not named: %+v", out.MissingEvidence)
	}
}

func TestAnalyzeIntegrationHealth_CurrentlyUnavailableEntities_ReadFirst(t *testing.T) {
	history := &countingHistoryReader{points: flapping(4)}
	opts := integrationHealthOptions(history, integrationHealthInventory("loaded", 80), downReader("light.hue_79"), &fakeRepairReader{}, nil)
	callAnalyzeIntegrationHealth(t, opts, map[string]any{"config_entry_id": "entry-hue"})
	if !slices.Contains(history.read, "light.hue_79") {
		t.Error("an entity that is down now was not among the bounded reads")
	}
}

func TestAnalyzeIntegrationHealth_PrivateEntity_DenyProfile_ExcludedNotRead(t *testing.T) {
	inv := integrationHealthInventory("loaded", 1)
	inv.entities = append(inv.entities, model.Entity{ID: "lock.front_door", ConfigEntryID: "entry-hue", Platform: "hue"})
	history := &countingHistoryReader{points: flapping(4)}
	opts := integrationHealthOptions(history, inv, downReader(), &fakeRepairReader{}, nil)
	opts.Profile = policy.Profile{Private: policy.HandlingDeny}

	res, out := callAnalyzeIntegrationHealth(t, opts, map[string]any{"config_entry_id": "entry-hue"})
	if res.IsError {
		t.Fatalf("a private member failed the whole call: %s", resultText(res))
	}
	if slices.Contains(history.read, "lock.front_door") {
		t.Error("a private entity's history was read under deny")
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "lock.front_door") {
		t.Error("a private entity id appears in the response")
	}
	if !slices.ContainsFunc(out.MissingEvidence, func(m model.MissingEvidence) bool { return m.Reason == model.MissingPolicyDenied }) {
		t.Errorf("the exclusion is not named: %+v", out.MissingEvidence)
	}
}

func TestAnalyzeIntegrationHealth_UnknownConfigEntry_NotFound(t *testing.T) {
	opts := integrationHealthOptions(&fakeHistoryReader{}, integrationHealthInventory("loaded", 1), downReader(), &fakeRepairReader{}, nil)
	res, _ := callAnalyzeIntegrationHealth(t, opts, map[string]any{"config_entry_id": "ghost"})
	if !res.IsError || !strings.Contains(resultText(res), "not found") {
		t.Errorf("IsError=%v text=%q, want a not-found error", res.IsError, resultText(res))
	}
}

func TestAnalyzeIntegrationHealth_InvalidInput_Refused(t *testing.T) {
	opts := integrationHealthOptions(&fakeHistoryReader{}, integrationHealthInventory("loaded", 1), downReader(), &fakeRepairReader{}, nil)
	for name, args := range map[string]map[string]any{
		"empty id":       {"config_entry_id": ""},
		"path-like id":   {"config_entry_id": "../etc/passwd"},
		"bad period":     {"config_entry_id": "entry-hue", "period": "soon"},
		"period too big": {"config_entry_id": "entry-hue", "period": "30d"},
	} {
		if res, _ := callAnalyzeIntegrationHealth(t, opts, args); !res.IsError {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestAnalyzeIntegrationHealth_MissingEvidence_NeverCarriesErrorText(t *testing.T) {
	const secret = "token-abc123"
	opts := integrationHealthOptions(&fakeHistoryReader{err: fmt.Errorf("%w: payload %s", ha.ErrUpstreamUnavailable, secret)},
		integrationHealthInventory("loaded", 2), downReader(),
		&fakeRepairReader{err: fmt.Errorf("%w: %s", ha.ErrUpstreamUnavailable, secret)},
		&fakeSupervisorReader{resolutionErr: fmt.Errorf("%w: %s", ha.ErrUpstreamUnavailable, secret)})
	res, out := callAnalyzeIntegrationHealth(t, opts, map[string]any{"config_entry_id": "entry-hue"})
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), secret) || strings.Contains(resultText(res), secret) {
		t.Error("an upstream error string reached the response")
	}
}

func TestAnalyzeIntegrationHealth_NoAvailabilityReader_ReportedMissing(t *testing.T) {
	opts := integrationHealthOptions(&fakeHistoryReader{points: flapping(4)}, integrationHealthInventory("loaded", 2), nil, &fakeRepairReader{}, nil)
	_, out := callAnalyzeIntegrationHealth(t, opts, map[string]any{"config_entry_id": "entry-hue"})
	if !slices.ContainsFunc(out.MissingEvidence, func(m model.MissingEvidence) bool { return m.What == "current entity availability" }) {
		t.Errorf("the unread live state is not named: %+v", out.MissingEvidence)
	}
}

func TestAnalyzeIntegrationHealth_EmptyLists_SerializeAsArrays(t *testing.T) {
	opts := integrationHealthOptions(&fakeHistoryReader{}, integrationHealthInventory("loaded", 1), downReader(), &fakeRepairReader{}, &fakeSupervisorReader{})
	res, _ := callAnalyzeIntegrationHealth(t, opts, map[string]any{"config_entry_id": "entry-hue"})
	raw, _ := json.Marshal(res.StructuredContent)
	if strings.Contains(string(raw), "null") {
		t.Errorf("an empty list serialized as null: %s", raw)
	}
}

// TestAnalyzeIntegrationHealth_ParityRule pins the four clauses: typed input,
// budget class, provenance (above) and no free-form parameter.
func TestAnalyzeIntegrationHealth_ParityRule(t *testing.T) {
	tool, ok := lookup(Catalog(), "analyze_integration_health")
	if !ok || tool.Class != policy.ClassComposite {
		t.Fatalf("catalog row = %+v, want composite budget class", tool)
	}
	opts := integrationHealthOptions(&fakeHistoryReader{}, integrationHealthInventory("loaded", 1), downReader(), &fakeRepairReader{}, nil)
	client := connect(t, newServer(opts, Catalog()))
	res, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tl := range res.Tools {
		if tl.Name != "analyze_integration_health" {
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
			if name != "config_entry_id" && name != "period" {
				t.Errorf("unexpected input property %q", name)
			}
		}
		return
	}
	t.Fatal("analyze_integration_health not listed")
}

// countingHistoryReader answers every entity with the same points and records
// which entities were read.
type countingHistoryReader struct {
	points []model.HistoryPoint
	calls  int
	read   []string
}

func (c *countingHistoryReader) History(_ context.Context, id model.EntityID, _, _ time.Time, _ bool) ([]model.HistoryPoint, error) {
	c.calls++
	c.read = append(c.read, string(id))
	return c.points, nil
}
