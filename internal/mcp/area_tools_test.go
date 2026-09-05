package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// errAreaLookup is a fixed test error for a floor/label registry that fails
// to answer.
var errAreaLookup = errors.New("registry unreachable")

// fakeAreaReader is an areaRegistryReader test double.
type fakeAreaReader struct {
	areas  []model.Area
	floors []model.Floor
	labels []model.Label

	floorsErr error
	labelsErr error
}

func (f *fakeAreaReader) Areas(context.Context) ([]model.Area, time.Time, error) {
	return f.areas, time.Time{}, nil
}
func (f *fakeAreaReader) Floors(context.Context) ([]model.Floor, time.Time, error) {
	return f.floors, time.Time{}, f.floorsErr
}
func (f *fakeAreaReader) Labels(context.Context) ([]model.Label, time.Time, error) {
	return f.labels, time.Time{}, f.labelsErr
}

func areaOptions(reader areaRegistryReader) Options {
	opts := testOptions()
	opts.Areas = reader
	return opts
}

// TestListAreas_ResolvesFloorAndLabelNames is the P3-06 DoD's "optional floor
// and label mapping": an area's raw floor_id/label ids are joined against the
// floor/label registries to attach names.
func TestListAreas_ResolvesFloorAndLabelNames(t *testing.T) {
	reader := &fakeAreaReader{
		areas: []model.Area{
			{ID: "kitchen", Name: "Kitchen", FloorID: "floor-1", Labels: []string{"label-a"}},
		},
		floors: []model.Floor{{ID: "floor-1", Name: "Ground Floor"}},
		labels: []model.Label{{ID: "label-a", Name: "Important"}},
	}
	client := connect(t, newServer(areaOptions(reader), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "list_areas", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var list model.AreaList
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("Items = %+v, want 1", list.Items)
	}
	item := list.Items[0]
	if item.FloorName != "Ground Floor" {
		t.Errorf("FloorName = %q, want %q", item.FloorName, "Ground Floor")
	}
	if len(item.LabelNames) != 1 || item.LabelNames[0] != "Important" {
		t.Errorf("LabelNames = %+v, want [Important]", item.LabelNames)
	}
}

// TestListAreas_FloorAndLabelRegistryFailure_StillReturnsAreas is
// Reliability's graceful-degradation rule: a floor/label registry that
// cannot be reached degrades only the name-resolution fields, never the
// area's own registry data.
func TestListAreas_FloorAndLabelRegistryFailure_StillReturnsAreas(t *testing.T) {
	reader := &fakeAreaReader{
		areas:     []model.Area{{ID: "kitchen", Name: "Kitchen", FloorID: "floor-1", Labels: []string{"label-a"}}},
		floorsErr: errAreaLookup,
		labelsErr: errAreaLookup,
	}
	client := connect(t, newServer(areaOptions(reader), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "list_areas", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var list model.AreaList
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].ID != "kitchen" {
		t.Fatalf("Items = %+v, want the area to survive a failed floor/label lookup", list.Items)
	}
	if list.Items[0].FloorName != "" || len(list.Items[0].LabelNames) != 0 {
		t.Errorf("Items[0] = %+v, want no resolved names when the registries fail", list.Items[0])
	}
}

// TestListAreas_CursorPagination_HonorsPhase02Contract pins list_areas onto
// the same cursor-pagination envelope every list_* tool shares.
func TestListAreas_CursorPagination_HonorsPhase02Contract(t *testing.T) {
	var areas []model.Area
	for i := range 3 {
		areas = append(areas, model.Area{ID: model.AreaID("area-" + string(rune('a'+i)))})
	}
	client := connect(t, newServer(areaOptions(&fakeAreaReader{areas: areas}), Catalog()))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "list_areas", Arguments: map[string]any{"limit": 2}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var list model.AreaList
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list.Items) != 2 || !list.Truncated || list.NextCursor == "" {
		t.Fatalf("first page = %+v, want 2 items, Truncated true, a NextCursor", list)
	}
}

// TestAreaTools_SchemaRejectsFreeFormParameter pins the parity rule's fourth
// clause: an implemented list_* tool's schema still closes off any parameter
// beyond its own typed fields.
func TestAreaTools_SchemaRejectsFreeFormParameter(t *testing.T) {
	client := connect(t, newServer(areaOptions(&fakeAreaReader{}), Catalog()))
	res, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name != "list_areas" {
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
