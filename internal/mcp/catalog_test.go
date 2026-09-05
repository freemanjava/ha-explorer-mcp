package mcp

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// expectedTools is doc §9's catalog, written out independently of the
// implementation. The 2026-08-25 decision is that all twenty ship; this list
// is what keeps that a fact — a tool quietly dropped or a twenty-first quietly
// added fails here.
var expectedTools = []string{
	"get_system_overview",
	"get_system_health",
	"list_integrations",
	"get_integration",
	"list_devices",
	"get_device",
	"list_entities",
	"get_entity",
	"get_entity_history",
	"get_entity_statistics",
	"find_unavailable_entities",
	"find_stale_entities",
	"list_areas",
	"list_automations",
	"get_automation",
	"get_automation_traces",
	"list_repairs",
	"list_apps",
	"analyze_entity_health",
	"analyze_integration_health",
}

func TestCatalog_Names_MatchDocCatalogExactly(t *testing.T) {
	var got []string
	for _, tool := range Catalog() {
		got = append(got, tool.Name)
	}
	if !slices.Equal(got, expectedTools) {
		t.Fatalf("catalog names = %v, want %v", got, expectedTools)
	}
	if len(got) != 20 {
		t.Fatalf("catalog holds %d tools, want the full twenty (phase 03 decision, 2026-08-25)", len(got))
	}
}

func TestCatalog_EveryTool_HasBudgetClassAndDescription(t *testing.T) {
	for _, tool := range Catalog() {
		limits := policy.LimitsFor(tool.Class)
		if limits.MaxBytes <= 0 || limits.Deadline <= 0 {
			t.Errorf("%s: budget class %v yields unusable limits %+v", tool.Name, tool.Class, limits)
		}
		if tool.Description == "" {
			t.Errorf("%s: no description — the client shows this to the model", tool.Name)
		}
	}
}

func TestCatalog_Names_AreSnakeCase(t *testing.T) {
	for _, tool := range Catalog() {
		if strings.ToLower(tool.Name) != tool.Name || strings.ContainsAny(tool.Name, " -.") {
			t.Errorf("%q is not snake_case (CLAUDE.md, Naming)", tool.Name)
		}
	}
}

func TestCatalog_Lookup_UnknownName_NotFound(t *testing.T) {
	if _, ok := lookup(Catalog(), "call_service"); ok {
		t.Fatal("lookup accepted a name outside the catalog — an unknown tool must fail closed")
	}
}

// TestEmptyObjectSchema_RejectsUnknownProperties pins the placeholder schema a
// not-yet-implemented tool is registered with: it accepts an empty object and
// nothing else, so an unimplemented row cannot become the free-form parameter
// rule 2 forbids.
func TestEmptyObjectSchema_RejectsUnknownProperties(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(emptyObjectSchema, &schema); err != nil {
		t.Fatalf("placeholder schema is not valid JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Fatalf("placeholder schema type = %v, want object", schema["type"])
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("placeholder schema allows additional properties: %v", schema["additionalProperties"])
	}
}
