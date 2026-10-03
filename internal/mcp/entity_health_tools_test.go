package mcp

import (
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

func entityHealthOptions(history historyReader, inv systemInventoryReader, repairs repairReader, profile policy.Profile) Options {
	opts := testOptions()
	opts.History = history
	opts.Inventory = inv
	opts.Repairs = repairs
	opts.Profile = profile
	return opts
}

func healthInventory(state string) *fakeInventoryReader {
	return &fakeInventoryReader{
		entities:     []model.Entity{{ID: "sensor.kitchen", ConfigEntryID: "entry-hue", Platform: "hue"}},
		integrations: []model.Integration{{ID: "entry-hue", Domain: "hue", State: state}},
	}
}

// flapping returns n hourly alternating points ending an hour ago.
func flapping(n int) []model.HistoryPoint {
	start := time.Now().Add(-time.Duration(n+1) * time.Hour)
	pts := make([]model.HistoryPoint, 0, n)
	for i := range n {
		state := "on"
		if i%2 == 1 {
			state = "unavailable"
		}
		pts = append(pts, model.HistoryPoint{Timestamp: start.Add(time.Duration(i) * time.Hour), State: state})
	}
	return pts
}

func callAnalyzeEntityHealth(t *testing.T, opts Options, args map[string]any) (*sdkmcp.CallToolResult, HealthResponse) {
	t.Helper()
	client := connect(t, newServer(opts, Catalog()))
	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "analyze_entity_health", Arguments: args})
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

func TestAnalyzeEntityHealth_ComposesSources_RankedHypothesesCiteEvidence(t *testing.T) {
	opts := entityHealthOptions(
		&fakeHistoryReader{points: flapping(40)},
		healthInventory("setup_retry"),
		&fakeRepairReader{repairs: []model.Repair{{IssueID: "r1", Domain: "hue"}}},
		policy.Profile{},
	)

	res, out := callAnalyzeEntityHealth(t, opts, map[string]any{"entity_id": "sensor.kitchen"})
	if res.IsError {
		t.Fatalf("error result: %s", resultText(res))
	}
	if len(out.Hypotheses) == 0 {
		t.Fatal("no hypotheses")
	}
	ids := map[model.EvidenceID]bool{}
	for _, ev := range out.Evidence {
		ids[ev.ID] = true
	}
	for _, id := range []model.EvidenceID{"availability", "integration_state", "repairs"} {
		if !ids[id] {
			t.Errorf("evidence %q missing", id)
		}
	}
	for _, h := range out.Hypotheses {
		if len(h.Cites) == 0 {
			t.Errorf("hypothesis %q cites nothing", h.Statement)
		}
		for _, c := range h.Cites {
			if !ids[c] {
				t.Errorf("hypothesis %q cites unknown evidence %q", h.Statement, c)
			}
		}
		if h.Confidence == "" {
			t.Errorf("hypothesis %q has no confidence", h.Statement)
		}
	}
}

func TestAnalyzeEntityHealth_Provenance_SourceAndObservationTime(t *testing.T) {
	opts := entityHealthOptions(&fakeHistoryReader{points: flapping(4)}, healthInventory("loaded"), &fakeRepairReader{}, policy.Profile{})
	_, out := callAnalyzeEntityHealth(t, opts, map[string]any{"entity_id": "sensor.kitchen"})
	if out.Source == "" || out.ObservedAt.IsZero() || out.SubjectID != "sensor.kitchen" {
		t.Errorf("provenance incomplete: %+v", out)
	}
	for _, ev := range out.Evidence {
		if ev.Source == "" {
			t.Errorf("evidence %q has no source", ev.ID)
		}
	}
}

func TestAnalyzeEntityHealth_DegradedSources_LowerConfidenceNotFail(t *testing.T) {
	cases := []struct {
		name    string
		history error
		repairs error
		want    model.MissingReason
	}{
		{"history deadline", fmt.Errorf("%w: x", ha.ErrDeadline), nil, model.MissingDeadline},
		{"history unsupported", fmt.Errorf("%w: x", ha.ErrUnsupported), nil, model.MissingUnsupported},
		{"history upstream", fmt.Errorf("%w: x", ha.ErrUpstreamUnavailable), nil, model.MissingUpstreamUnavailable},
		{"repairs unsupported", nil, fmt.Errorf("%w: x", ha.ErrUnsupported), model.MissingUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := entityHealthOptions(
				&fakeHistoryReader{points: flapping(10), err: tc.history},
				healthInventory("loaded"),
				&fakeRepairReader{err: tc.repairs},
				policy.Profile{},
			)
			res, out := callAnalyzeEntityHealth(t, opts, map[string]any{"entity_id": "sensor.kitchen"})
			if res.IsError {
				t.Fatalf("a degraded source failed the call: %s", resultText(res))
			}
			if !slices.ContainsFunc(out.MissingEvidence, func(m model.MissingEvidence) bool { return m.Reason == tc.want }) {
				t.Errorf("missing_evidence = %+v, want reason %q", out.MissingEvidence, tc.want)
			}
			if !out.Partial {
				t.Error("Partial = false")
			}
		})
	}
}

func TestAnalyzeEntityHealth_MissingEvidence_NeverCarriesErrorText(t *testing.T) {
	const secret = "token-abc123"
	opts := entityHealthOptions(
		&fakeHistoryReader{err: fmt.Errorf("%w: payload %s", ha.ErrUpstreamUnavailable, secret)},
		healthInventory("loaded"), &fakeRepairReader{}, policy.Profile{})
	res, out := callAnalyzeEntityHealth(t, opts, map[string]any{"entity_id": "sensor.kitchen"})
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), secret) || strings.Contains(resultText(res), secret) {
		t.Error("an upstream error string reached the response")
	}
}

func TestAnalyzeEntityHealth_NoRepairsReader_ReportedMissing(t *testing.T) {
	opts := entityHealthOptions(&fakeHistoryReader{points: flapping(4)}, healthInventory("loaded"), nil, policy.Profile{})
	_, out := callAnalyzeEntityHealth(t, opts, map[string]any{"entity_id": "sensor.kitchen"})
	if !slices.ContainsFunc(out.MissingEvidence, func(m model.MissingEvidence) bool { return m.What == "open repairs" }) {
		t.Errorf("an unconfigured repairs source is not named: %+v", out.MissingEvidence)
	}
}

func TestAnalyzeEntityHealth_PrivateEntity_DenyProfile_RefusedBeforeAnyRead(t *testing.T) {
	history := &fakeHistoryReader{points: flapping(4)}
	opts := entityHealthOptions(history, healthInventory("loaded"), &fakeRepairReader{}, policy.Profile{Private: policy.HandlingDeny})
	res, _ := callAnalyzeEntityHealth(t, opts, map[string]any{"entity_id": "lock.front_door"})
	if !res.IsError {
		t.Fatal("a private entity under deny was analyzed")
	}
	if history.lastEntityID != "" {
		t.Error("the recorder was read despite the deny profile")
	}
}

func TestAnalyzeEntityHealth_UnknownEntity_NotFound(t *testing.T) {
	opts := entityHealthOptions(&fakeHistoryReader{}, healthInventory("loaded"), &fakeRepairReader{}, policy.Profile{})
	res, _ := callAnalyzeEntityHealth(t, opts, map[string]any{"entity_id": "sensor.ghost"})
	if !res.IsError || !strings.Contains(resultText(res), "not found") {
		t.Errorf("unknown entity: IsError=%v text=%q, want a not-found error", res.IsError, resultText(res))
	}
}

func TestAnalyzeEntityHealth_InvalidInput_Refused(t *testing.T) {
	opts := entityHealthOptions(&fakeHistoryReader{}, healthInventory("loaded"), &fakeRepairReader{}, policy.Profile{})
	for name, args := range map[string]map[string]any{
		"bad id":         {"entity_id": "../etc/passwd"},
		"bad period":     {"entity_id": "sensor.kitchen", "period": "soon"},
		"period too big": {"entity_id": "sensor.kitchen", "period": "30d"},
	} {
		if res, _ := callAnalyzeEntityHealth(t, opts, args); !res.IsError {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestAnalyzeEntityHealth_EmptyLists_SerializeAsArrays(t *testing.T) {
	opts := entityHealthOptions(&fakeHistoryReader{points: []model.HistoryPoint{{Timestamp: time.Now().Add(-time.Hour), State: "on"}}},
		healthInventory("loaded"), &fakeRepairReader{}, policy.Profile{})
	res, _ := callAnalyzeEntityHealth(t, opts, map[string]any{"entity_id": "sensor.kitchen"})
	raw, _ := json.Marshal(res.StructuredContent)
	if strings.Contains(string(raw), `"Hypotheses":null`) {
		t.Errorf("empty hypotheses serialized as null: %s", raw)
	}
}

// TestAnalyzeEntityHealth_ParityRule pins the four clauses: typed input,
// budget class, provenance (above) and no free-form parameter.
func TestAnalyzeEntityHealth_ParityRule(t *testing.T) {
	tool, ok := lookup(Catalog(), "analyze_entity_health")
	if !ok || tool.Class != policy.ClassComposite {
		t.Fatalf("catalog row = %+v, want composite budget class", tool)
	}
	opts := entityHealthOptions(&fakeHistoryReader{}, healthInventory("loaded"), &fakeRepairReader{}, policy.Profile{})
	client := connect(t, newServer(opts, Catalog()))
	res, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tl := range res.Tools {
		if tl.Name != "analyze_entity_health" {
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
	t.Fatal("analyze_entity_health not listed")
}
