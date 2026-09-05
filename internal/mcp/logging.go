package mcp

import (
	"log/slog"
	"os"

	"github.com/freemanjava/ha-explorer-mcp/internal/redact"
)

// NewLogger returns the server's logger: structured, levelled, scrubbed of the
// given secrets, and writing to **stderr at every level**.
//
// The stream choice is not a preference. Under the 2026-08-25 stdio decision,
// stdout carries the MCP framing: one stray log line there corrupts the
// protocol stream for the rest of the session. Routing every level to stderr
// in one constructor is what keeps that a property of the program rather than
// a rule each future log call has to remember (P3-01 DoD).
func NewLogger(level slog.Level, secrets ...string) *slog.Logger {
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	return slog.New(redact.NewLogHandler(h, secrets...))
}
