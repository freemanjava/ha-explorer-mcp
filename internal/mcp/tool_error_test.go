package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

const toolErrorSecret = "s3cret-token-value"

type noInput struct{}
type noOutput struct{}

// typedProbeTable is the catalog with every handler replaced by one registered
// through the SDK's typed AddTool — the path every shipped tool takes, and the
// one that carries a handler error as an IsError result with a nil Go error
// (D-08-19). The raw AddTool of probeTable does not.
func typedProbeTable(fn func(name string) error) []Tool {
	tools := Catalog()
	for i := range tools {
		name := tools[i].Name
		tools[i].bind = func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
			sdkmcp.AddTool(srv, def, func(context.Context, *sdkmcp.CallToolRequest, noInput) (*sdkmcp.CallToolResult, noOutput, error) {
				return nil, noOutput{}, fn(name)
			})
		}
	}
	return tools
}

// TestInvocation_ToolFailure_AuditedAsTheErrorItIs: whichever way the SDK
// carries a tool's failure, the audit separates not-found, refused and
// budget cutoff, and the agent still gets a tool error, not a protocol one.
func TestInvocation_ToolFailure_AuditedAsTheErrorItIs(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"not found", fmt.Errorf("entity: %w", ha.ErrNotFound), "error"},
		{"policy denied", fmt.Errorf("blocked: %w", policy.ErrPolicyDenied), "denied"},
		{"budget exceeded", fmt.Errorf("cut off: %w", policy.ErrBudgetExceeded), "budget_exceeded"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink := &recordSink{}
			client := connect(t, newServer(auditOptions(sink), typedProbeTable(func(string) error { return tc.err })))

			res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "list_areas"})
			if err != nil {
				t.Fatalf("client error = %v, want a tool error result", err)
			}
			if res == nil || !res.IsError {
				t.Fatalf("result = %+v, want IsError", res)
			}
			if got := sink.records[len(sink.records)-1]["status"]; got != tc.want {
				t.Errorf("audit status = %v, want %s", got, tc.want)
			}
		})
	}
}

func TestInvocation_ValidationFailure_AuditsError(t *testing.T) {
	w := newWireStack(t)
	client := connect(t, NewServer(w.opts))

	res, rec := w.call(t, client, "get_entity", map[string]any{"id": ""})
	if res == nil || !res.IsError {
		t.Fatalf("result = %+v, want IsError", res)
	}
	if rec["status"] != "error" {
		t.Errorf("audit status = %v, want error", rec["status"])
	}
}

// TestInvocation_ToolFailureCarryingSecret_ReachesNeitherAuditNorResult: the
// scrub applies to an IsError result exactly as to a returned error.
func TestInvocation_ToolFailureCarryingSecret_ReachesNeitherAuditNorResult(t *testing.T) {
	sink := &recordSink{}
	opts := auditOptions(sink)
	opts.Secrets = []string{toolErrorSecret}
	boom := errors.New("upstream said " + toolErrorSecret)
	client := connect(t, newServer(opts, typedProbeTable(func(string) error { return boom })))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "list_areas"})
	if err != nil || res == nil || !res.IsError {
		t.Fatalf("got (%+v, %v), want an IsError result and nil error", res, err)
	}
	for _, c := range res.Content {
		if text, ok := c.(*sdkmcp.TextContent); ok && strings.Contains(text.Text, toolErrorSecret) {
			t.Errorf("result text leaks the secret: %q", text.Text)
		}
	}
	rec := sink.records[len(sink.records)-1]
	if reason, _ := rec["reason"].(string); strings.Contains(reason, toolErrorSecret) {
		t.Errorf("audit reason leaks the secret: %q", reason)
	}
}

func TestInvocation_ToolSuccess_StillAuditsSuccess(t *testing.T) {
	sink := &recordSink{}
	client := connect(t, newServer(auditOptions(sink), typedProbeTable(func(string) error { return nil })))

	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "list_areas"})
	if err != nil || res == nil || res.IsError {
		t.Fatalf("got (%+v, %v), want success", res, err)
	}
	if got := sink.records[len(sink.records)-1]["status"]; got != "success" {
		t.Errorf("audit status = %v, want success", got)
	}
}
