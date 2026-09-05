package mcp

import (
	"context"
	"log/slog"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/audit"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// ServerName is the implementation name this server reports at initialize.
const ServerName = "ha-inspector-mcp"

// Options are the collaborators the server is built from. Everything is
// injected: no package here reaches for a global client or a global logger
// (CLAUDE.md, Dependency inversion and Logging).
type Options struct {
	// Version is the build version reported at initialize.
	Version string
	// Logger receives lifecycle and audit output. It must never write to
	// stdout — stdout carries the MCP framing (phase 01's stdio decision).
	// Use NewLogger.
	Logger *slog.Logger
	// Audit emits one record per invocation. Defaults to a body-free logger
	// over Logger.
	Audit *audit.Logger
	// Profile is the privacy profile applied at the response and audit
	// boundary. Its zero value is the default (mask).
	Profile policy.Profile
	// Secrets are literals that must never appear in a response, a log line
	// or an audit record — SUPERVISOR_TOKEN above all (rule 4).
	Secrets []string
	// Limiter bounds how fast invocations may arrive. Defaults to
	// policy.NewInvocationLimiter.
	Limiter *policy.RateLimiter
}

// NewServer builds the MCP server over the static catalog: every tool doc §9
// names, each annotated read-only, each charged against the budget class its
// catalog row carries, and every invocation wrapped in rate limiting, budget,
// panic recovery and audit.
func NewServer(opts Options) *sdkmcp.Server {
	return newServer(opts, Catalog())
}

// newServer is NewServer over an explicit table, so tests can exercise the
// middleware with a tool that misbehaves on purpose without that tool ever
// existing in a shipped build.
func newServer(opts Options, tools []Tool) *sdkmcp.Server {
	opts = opts.withDefaults()

	srv := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    ServerName,
		Version: opts.Version,
	}, &sdkmcp.ServerOptions{
		Instructions: instructions,
		Logger:       opts.Logger,
	})

	for _, t := range tools {
		register(srv, t)
	}

	srv.AddReceivingMiddleware(newInvocationMiddleware(opts, tools).wrap)
	return srv
}

// Run serves the MCP protocol over stdio until ctx is cancelled or the client
// disconnects. There is no listening socket and no auth subsystem: the
// transport decision of 2026-08-25 is stdio only, and the Supervisor starts
// this binary as a child process.
func Run(ctx context.Context, opts Options) error {
	return NewServer(opts).Run(ctx, &sdkmcp.StdioTransport{})
}

// instructions tell the client what this server is for. It is the one place
// the read-only, diagnostic contract is stated to the model — the guarantee
// itself is enforced by what is linked in, not by this text (ADR-008).
const instructions = "Read-only diagnostic access to a Home Assistant installation: " +
	"inventory, availability, history, statistics and automation traces. " +
	"No tool changes anything, and no tool accepts a route, command, query, path or code. " +
	"Everyday device control belongs to the official Home Assistant MCP server, not this one. " +
	"Responses separate observed facts from inference, and report explicitly when a source " +
	"was unavailable rather than answering with an empty result."

func (o Options) withDefaults() Options {
	if o.Logger == nil {
		o.Logger = NewLogger(slog.LevelInfo, o.Secrets...)
	}
	if o.Audit == nil {
		o.Audit = audit.New(o.Logger)
	}
	if o.Limiter == nil {
		o.Limiter = policy.NewInvocationLimiter()
	}
	return o
}
