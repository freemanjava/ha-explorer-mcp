package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/audit"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
	"github.com/freemanjava/ha-explorer-mcp/internal/redact"
)

// methodCallTool is the JSON-RPC method one tool invocation arrives as. The
// SDK does not export its method-name constants, so it is named here once.
const methodCallTool = "tools/call"

// invocationMiddleware is the envelope every tool invocation runs inside:
// arrival rate limiting, a per-invocation query budget with its deadline,
// panic recovery, and one audit record whichever way the call ends.
//
// It sits in the transport layer on purpose. A tool cannot opt out of it, and
// a tool added later cannot forget it — which is the same reason the budget
// class lives on the catalog row rather than in the tool's own code.
type invocationMiddleware struct {
	tools   []Tool
	audit   *audit.Logger
	limiter *policy.RateLimiter
	profile policy.Profile
	secrets []string
}

func newInvocationMiddleware(opts Options, tools []Tool) *invocationMiddleware {
	return &invocationMiddleware{
		tools:   tools,
		audit:   opts.Audit,
		limiter: opts.Limiter,
		profile: opts.Profile,
		secrets: opts.Secrets,
	}
}

func (m *invocationMiddleware) wrap(next sdkmcp.MethodHandler) sdkmcp.MethodHandler {
	return func(ctx context.Context, method string, req sdkmcp.Request) (sdkmcp.Result, error) {
		if method != methodCallTool {
			return next(ctx, method, req)
		}
		call, ok := req.(*sdkmcp.CallToolRequest)
		if !ok {
			// A tools/call that does not carry tool-call params cannot be
			// budgeted or audited, so it is refused rather than passed on.
			return nil, fmt.Errorf("%w: malformed tools/call request", ErrUnknownTool)
		}
		return m.invoke(ctx, next, call)
	}
}

func (m *invocationMiddleware) invoke(ctx context.Context, next sdkmcp.MethodHandler, call *sdkmcp.CallToolRequest) (sdkmcp.Result, error) {
	name := call.Params.Name
	redactor := redact.New(m.profile, m.secrets...)
	params := decodeArguments(call.Params.Arguments)

	// Fail closed before anything runs: a name outside the static catalog has
	// no budget class, and there is no default that would be safe to invent.
	tool, ok := lookup(m.tools, name)
	if !ok {
		err := fmt.Errorf("%w: %s", ErrUnknownTool, name)
		m.emit(ctx, redactor, audit.Record{
			Tool: name, Parameters: params,
			Status: audit.StatusDenied, Reason: ErrUnknownTool.Error(),
		})
		return nil, err
	}

	if err := m.limiter.Allow(); err != nil {
		m.emit(ctx, redactor, audit.Record{
			Tool: name, Parameters: params,
			Status: audit.StatusDenied, Reason: err.Error(),
		})
		return nil, err
	}

	budget := policy.NewQueryBudget(tool.Class)
	ctx, cancel := policy.WithBudget(ctx, budget)
	defer cancel()

	started := time.Now()
	res, err := callWithRecovery(ctx, next, call)
	used := budget.Usage()

	rec := audit.Record{
		Tool:       name,
		Parameters: params,
		HARequests: used.HARequests,
		Duration:   time.Since(started),
		// The bytes charged to the budget are what the invocation actually
		// cost; re-serializing the result to measure it would double the work
		// on the machine this server is trying not to load.
		ResultBytes: used.Bytes,
		Status:      audit.StatusSuccess,
	}
	if err != nil {
		rec.Status, rec.Reason = classify(err), redactor.Error(err).Error()
	}
	m.emit(ctx, redactor, rec)

	if err != nil {
		return nil, redactor.Error(err)
	}
	return res, nil
}

// callWithRecovery turns a panicking tool into an error result. A long-lived
// process must not die because one handler hit a nil map (CLAUDE.md, Error
// Handling: fail fast on programmer errors, but not by taking the server with
// it while an agent is mid-investigation).
func callWithRecovery(ctx context.Context, next sdkmcp.MethodHandler, call *sdkmcp.CallToolRequest) (res sdkmcp.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			// The panic value may embed an upstream payload, so it is
			// summarized by type, never interpolated into the error.
			err = fmt.Errorf("%w: %s panicked (%T)", ErrToolPanicked, call.Params.Name, r)
		}
	}()
	return next(ctx, methodCallTool, call)
}

// classify maps an invocation's error to the audit status that keeps a
// refusal, a budget cutoff and a failure three different answers.
func classify(err error) audit.Status {
	switch {
	case errors.Is(err, policy.ErrBudgetExceeded):
		return audit.StatusBudgetExceeded
	case errors.Is(err, policy.ErrPolicyDenied),
		errors.Is(err, policy.ErrRateLimited),
		errors.Is(err, ErrUnknownTool):
		return audit.StatusDenied
	default:
		return audit.StatusError
	}
}

// decodeArguments renders the raw tool arguments as a map for the audit
// record. Arguments that are not a JSON object are recorded as their raw text
// rather than dropped — the trail says what was asked for, and the redactor
// scrubs it before it is written.
func decodeArguments(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var params map[string]any
	if err := json.Unmarshal(raw, &params); err != nil {
		return map[string]any{"raw": string(raw)}
	}
	return params
}

func (m *invocationMiddleware) emit(ctx context.Context, redactor *redact.Redactor, rec audit.Record) {
	if m.audit == nil {
		return
	}
	m.audit.Emit(ctx, redactor, rec)
}
