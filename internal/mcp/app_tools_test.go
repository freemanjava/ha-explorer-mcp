package mcp

import (
	"encoding/json"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

func appOptions(supervisor systemHealthReader) Options {
	opts := testOptions()
	opts.Supervisor = supervisor
	return opts
}

// TestListApps_EnumeratesFromSupervisorInfo is the P3-06 DoD's happy path:
// list_apps enumerates from /supervisor/info's embedded App inventory.
func TestListApps_EnumeratesFromSupervisorInfo(t *testing.T) {
	supervisor := &fakeSupervisorReader{supervisorInfo: model.SupervisorInfo{
		Apps: []model.App{
			{Slug: "core_mosquitto", Name: "Mosquitto broker", Version: "6.4.0"},
			{Slug: "core_ssh", Name: "Terminal & SSH", Version: "9.14.0"},
		},
	}}
	client := connect(t, newServer(appOptions(supervisor), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "list_apps", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var list model.AppList
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if list.Unsupported {
		t.Fatalf("Unsupported = true, want false: %+v", list)
	}
	if len(list.Items) != 2 {
		t.Fatalf("Items = %+v, want 2", list.Items)
	}
}

// TestListApps_SupervisorUnreachable_ReportsUnsupportedNotEmpty is the P3-06
// DoD's distinguishing assertion: Supervisor unreachable must not look like
// "no Apps installed".
func TestListApps_SupervisorUnreachable_ReportsUnsupportedNotEmpty(t *testing.T) {
	supervisor := &fakeSupervisorReader{supervisorInfoErr: errAreaLookup}
	client := connect(t, newServer(appOptions(supervisor), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "list_apps", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var list model.AppList
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !list.Unsupported || list.UnsupportedReason == "" {
		t.Fatalf("list = %+v, want Unsupported true with a reason", list)
	}
	if len(list.Items) != 0 {
		t.Fatalf("Items = %+v, want none alongside Unsupported", list.Items)
	}
}

// TestListApps_EmptyInventory_IsDistinctFromUnsupported pins the other half
// of the same DoD line: a Supervisor that answers with zero installed Apps
// is a real, supported empty list, not Unsupported.
func TestListApps_EmptyInventory_IsDistinctFromUnsupported(t *testing.T) {
	supervisor := &fakeSupervisorReader{supervisorInfo: model.SupervisorInfo{}}
	client := connect(t, newServer(appOptions(supervisor), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "list_apps", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var list model.AppList
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if list.Unsupported {
		t.Fatalf("Unsupported = true, want false for a real empty inventory: %+v", list)
	}
	if len(list.Items) != 0 {
		t.Fatalf("Items = %+v, want none", list.Items)
	}
}

// TestAppTools_SchemaRejectsFreeFormParameter pins the parity rule's fourth
// clause: an implemented list_* tool's schema still closes off any parameter
// beyond its own typed fields.
func TestAppTools_SchemaRejectsFreeFormParameter(t *testing.T) {
	client := connect(t, newServer(appOptions(&fakeSupervisorReader{}), Catalog()))
	res, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name != "list_apps" {
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
