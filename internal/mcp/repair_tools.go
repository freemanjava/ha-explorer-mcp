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

// repairReader is list_repairs' read surface: repairs/list_issues, reachable
// at any principal (docs/research/2026-09-05-ha-repairs-api.md), unlike the
// automation/trace surface.
type repairReader interface {
	Repairs(ctx context.Context) ([]model.Repair, error)
}

// ListRepairsInput is list_repairs' typed, validated input: the Phase 02
// cursor-pagination contract, with no filter beyond it — the DoD asks for
// severity and issue id on each item, not a filter dimension.
type ListRepairsInput struct {
	Cursor string `json:"cursor,omitempty" jsonschema:"resume after this page's cursor"`
	Limit  int    `json:"limit,omitempty" jsonschema:"page size, default 50, max 200"`
}

// withRepairTools returns tools with list_repairs' handler bound, when opts
// supplies a reader. A row whose reader is absent keeps its
// bindNotImplemented default.
func withRepairTools(tools []Tool, opts Options) []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	for i := range out {
		if out[i].Name == "list_repairs" && opts.Repairs != nil {
			out[i].bind = bindListRepairs(opts.Repairs)
		}
	}
	return out
}

// bindListRepairs registers list_repairs' typed handler.
func bindListRepairs(reader repairReader) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in ListRepairsInput) (*sdkmcp.CallToolResult, model.RepairList, error) {
			out, err := listRepairs(ctx, reader, in)
			return nil, out, err
		})
	}
}

// listRepairs sorts by issue id and pages the repair/issue inventory.
func listRepairs(ctx context.Context, reader repairReader, in ListRepairsInput) (model.RepairList, error) {
	repairs, err := reader.Repairs(ctx)
	if err != nil {
		return model.RepairList{}, err
	}
	sort.Slice(repairs, func(i, j int) bool { return repairs[i].IssueID < repairs[j].IssueID })

	pg, err := page.Paginate(repairs, in.Cursor, in.Limit, maxResponseBytes(ctx),
		func(r model.Repair) string { return r.IssueID },
		repairByteSize,
	)
	if err != nil {
		return model.RepairList{}, err
	}

	return model.RepairList{
		Source:       "home_assistant_core",
		ObservedAt:   time.Now().UTC(),
		Items:        pg.Items,
		NextCursor:   pg.NextCursor,
		Truncated:    pg.Truncated,
		LimitClamped: pg.LimitClamped,
	}, nil
}

// repairByteSize approximates one repair's serialized size for the page
// package's byte cap — cheap enough to run per record without re-serializing
// the whole response afterward.
func repairByteSize(r model.Repair) int64 {
	b, err := json.Marshal(r)
	if err != nil {
		return 0
	}
	return int64(len(b))
}
