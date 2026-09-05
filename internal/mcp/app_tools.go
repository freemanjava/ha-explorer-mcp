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

// supervisorAppReader is list_apps' read surface: Supervisor's own status
// endpoint, whose payload embeds the installed-App inventory — its only
// enumeration path at the granted role (P3-06). systemHealthReader already
// carries this method, so list_apps reuses Options.Supervisor rather than a
// second field.
type supervisorAppReader interface {
	SupervisorInfo(ctx context.Context) (model.SupervisorInfo, error)
}

// ListAppsInput is list_apps' typed, validated input: the Phase 02
// cursor-pagination contract, with no filter beyond it.
type ListAppsInput struct {
	Cursor string `json:"cursor,omitempty" jsonschema:"resume after this page's cursor"`
	Limit  int    `json:"limit,omitempty" jsonschema:"page size, default 50, max 200"`
}

// withAppTools returns tools with list_apps' handler bound, when opts
// supplies a Supervisor reader. A row whose reader is absent keeps its
// bindNotImplemented default.
func withAppTools(tools []Tool, opts Options) []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	for i := range out {
		if out[i].Name == "list_apps" && opts.Supervisor != nil {
			out[i].bind = bindListApps(opts.Supervisor)
		}
	}
	return out
}

// bindListApps registers list_apps' typed handler.
func bindListApps(supervisor supervisorAppReader) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in ListAppsInput) (*sdkmcp.CallToolResult, model.AppList, error) {
			out, err := listApps(ctx, supervisor, in)
			return nil, out, err
		})
	}
}

// listApps enumerates from /supervisor/info and pages the App inventory.
// Supervisor being unreachable at the granted role degrades the whole
// response to Unsupported with a reason, never an empty Items that would
// look like "no Apps installed" (P3-06 DoD, CLAUDE.md rule 7) — distinct
// from a pagination error, which is a real tool error, not a fact about the
// installation.
func listApps(ctx context.Context, supervisor supervisorAppReader, in ListAppsInput) (model.AppList, error) {
	out := model.AppList{Source: "home_assistant_supervisor", ObservedAt: time.Now().UTC()}

	info, err := supervisor.SupervisorInfo(ctx)
	if err != nil {
		out.Unsupported = true
		out.UnsupportedReason = err.Error()
		return out, nil
	}

	apps := make([]model.App, len(info.Apps))
	copy(apps, info.Apps)
	sort.Slice(apps, func(i, j int) bool { return apps[i].Slug < apps[j].Slug })

	pg, err := page.Paginate(apps, in.Cursor, in.Limit, maxResponseBytes(ctx),
		func(a model.App) string { return a.Slug },
		appByteSize,
	)
	if err != nil {
		return model.AppList{}, err
	}
	out.Items = pg.Items
	out.NextCursor = pg.NextCursor
	out.Truncated = pg.Truncated
	out.LimitClamped = pg.LimitClamped
	return out, nil
}

// appByteSize approximates one App's serialized size for the page package's
// byte cap — cheap enough to run per record without re-serializing the whole
// response afterward.
func appByteSize(a model.App) int64 {
	b, err := json.Marshal(a)
	if err != nil {
		return 0
	}
	return int64(len(b))
}
