package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// errRepairsLookup is a fixed test error for a repairs reader that fails to
// answer.
var errRepairsLookup = errors.New("repairs unreachable")

// fakeRepairReader is a repairReader test double.
type fakeRepairReader struct {
	repairs []model.Repair
	err     error
}

func (f *fakeRepairReader) Repairs(context.Context) ([]model.Repair, error) {
	return f.repairs, f.err
}

func repairOptions(reader repairReader) Options {
	opts := testOptions()
	opts.Repairs = reader
	return opts
}

// TestListRepairs_ReportsSeverityAndIssueID pins the P3-06 DoD: repairs are
// returned with the severity/issue id an agent can cite as evidence.
func TestListRepairs_ReportsSeverityAndIssueID(t *testing.T) {
	reader := &fakeRepairReader{repairs: []model.Repair{
		{IssueID: "deprecated_setting", Domain: "sun", Severity: "warning", IsFixable: true, TranslationPlaceholders: map[string]any{}},
	}}
	client := connect(t, newServer(repairOptions(reader), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "list_repairs", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var list model.RepairList
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("Items = %+v, want 1", list.Items)
	}
	item := list.Items[0]
	if item.IssueID != "deprecated_setting" || item.Severity != "warning" {
		t.Errorf("item = %+v, want issue id and severity preserved", item)
	}
}

// TestListRepairs_UpstreamFailure_ReturnsError pins that a failed
// repairs/list_issues call is a real tool error, not a degraded-but-served
// response — unlike list_apps, this surface has no principal-refused case to
// distinguish (docs/research/2026-09-05-ha-repairs-api.md).
func TestListRepairs_UpstreamFailure_ReturnsError(t *testing.T) {
	reader := &fakeRepairReader{err: errRepairsLookup}
	client := connect(t, newServer(repairOptions(reader), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "list_repairs", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool transport error: %v", err)
	}
	if !res.IsError {
		t.Fatal("list_repairs with a failing reader did not return an error result")
	}
}

// TestListRepairs_CursorPagination_HonorsPhase02Contract pins list_repairs
// onto the same cursor-pagination envelope every list_* tool shares.
func TestListRepairs_CursorPagination_HonorsPhase02Contract(t *testing.T) {
	var repairs []model.Repair
	for i := range 3 {
		repairs = append(repairs, model.Repair{IssueID: "issue-" + string(rune('a'+i)), TranslationPlaceholders: map[string]any{}})
	}
	client := connect(t, newServer(repairOptions(&fakeRepairReader{repairs: repairs}), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "list_repairs", Arguments: map[string]any{"limit": 2}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var list model.RepairList
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list.Items) != 2 || !list.Truncated || list.NextCursor == "" {
		t.Fatalf("first page = %+v, want 2 items, Truncated true, a NextCursor", list)
	}
}

// TestRepairTools_SchemaRejectsFreeFormParameter pins the parity rule's
// fourth clause: an implemented list_* tool's schema still closes off any
// parameter beyond its own typed fields.
func TestRepairTools_SchemaRejectsFreeFormParameter(t *testing.T) {
	client := connect(t, newServer(repairOptions(&fakeRepairReader{}), Catalog()))
	res, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name != "list_repairs" {
			continue
		}
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("%s: marshal schema: %v", tool.Name, err)
		}
		var schema struct {
			Type                 string `json:"type"`
			AdditionalProperties any    `json:"additionalProperties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("%s: unmarshal schema: %v", tool.Name, err)
		}
		if schema.Type != "object" || schema.AdditionalProperties != false {
			t.Errorf("%s: schema = %s, want a closed object with additionalProperties false", tool.Name, raw)
		}
	}
}
