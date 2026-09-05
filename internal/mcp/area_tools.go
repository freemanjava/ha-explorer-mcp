package mcp

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/page"
)

// areaRegistryReader is list_areas' read surface into the slow-moving
// registries: the area registry itself plus the floor/label registries
// joined in-process to resolve names (P3-06 DoD: "optional floor and label
// mapping").
type areaRegistryReader interface {
	Areas(ctx context.Context) ([]model.Area, time.Time, error)
	Floors(ctx context.Context) ([]model.Floor, time.Time, error)
	Labels(ctx context.Context) ([]model.Label, time.Time, error)
}

// ListAreasInput is list_areas' typed, validated input: the Phase 02
// cursor-pagination contract, with no filter beyond it — area topology has
// no field doc §9 asks a filter for.
type ListAreasInput struct {
	Cursor string `json:"cursor,omitempty" jsonschema:"resume after this page's cursor"`
	Limit  int    `json:"limit,omitempty" jsonschema:"page size, default 50, max 200"`
}

// withAreaTools returns tools with list_areas' handler bound, when opts
// supplies a reader. A row whose reader is absent keeps its
// bindNotImplemented default.
func withAreaTools(tools []Tool, opts Options) []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	for i := range out {
		if out[i].Name == "list_areas" && opts.Areas != nil {
			out[i].bind = bindListAreas(opts.Areas)
		}
	}
	return out
}

// bindListAreas registers list_areas' typed handler.
func bindListAreas(registry areaRegistryReader) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in ListAreasInput) (*sdkmcp.CallToolResult, model.AreaList, error) {
			out, err := listAreas(ctx, registry, in)
			return nil, out, err
		})
	}
}

// listAreas joins the area registry with floor/label names, sorts by id, and
// pages. A floor or label registry that fails to answer degrades only the
// name-resolution fields: the areas themselves, with their raw FloorID and
// label ids, are still returned (Reliability — graceful degradation).
func listAreas(ctx context.Context, registry areaRegistryReader, in ListAreasInput) (model.AreaList, error) {
	areas, observedAt, err := registry.Areas(ctx)
	if err != nil {
		return model.AreaList{}, err
	}

	floorNames := map[string]string{}
	if floors, _, err := registry.Floors(ctx); err == nil {
		for _, f := range floors {
			floorNames[f.ID] = f.Name
		}
	}
	labelNames := map[string]string{}
	if labels, _, err := registry.Labels(ctx); err == nil {
		for _, l := range labels {
			labelNames[l.ID] = l.Name
		}
	}

	summaries := make([]model.AreaSummary, len(areas))
	for i, a := range areas {
		s := model.AreaSummary{Area: a, FloorName: floorNames[a.FloorID]}
		for _, id := range a.Labels {
			if name, ok := labelNames[id]; ok {
				s.LabelNames = append(s.LabelNames, name)
			}
		}
		summaries[i] = s
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ID < summaries[j].ID })

	pg, err := page.Paginate(summaries, in.Cursor, in.Limit, maxResponseBytes(ctx),
		func(s model.AreaSummary) string { return string(s.ID) },
		areaByteSize,
	)
	if err != nil {
		return model.AreaList{}, err
	}

	return model.AreaList{
		Source:       "home_assistant_core",
		ObservedAt:   observedAt,
		Items:        pg.Items,
		NextCursor:   pg.NextCursor,
		Truncated:    pg.Truncated,
		LimitClamped: pg.LimitClamped,
	}, nil
}

// areaByteSize approximates one area summary's serialized size for the page
// package's byte cap — cheap enough to run per record without re-serializing
// the whole response afterward.
func areaByteSize(s model.AreaSummary) int64 {
	b, err := json.Marshal(s)
	if err != nil {
		return 0
	}
	return int64(len(b))
}
