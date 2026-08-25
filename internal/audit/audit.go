// Package audit emits one structured record per MCP invocation, in the shape
// of doc §17: what tool ran, with what parameters, at what upstream cost, and
// how it ended. It answers "what did the agent request and how expensive was
// it?" without becoming a copy of private smart-home history — the result
// body is never persisted unless a caller opts in explicitly.
//
// It classifies and redacts nothing itself: parameters cross the same
// internal/redact boundary a response does, so a secret planted in a tool
// argument cannot reach this trail either (CLAUDE.md rule 4).
package audit

import (
	"context"
	"log/slog"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/redact"
)

// Status is how an invocation ended. It appears in the audit record and must
// stay distinguishable the way CLAUDE.md's Error Handling section requires:
// a refusal, a budget cutoff and a success are three different answers.
type Status string

const (
	StatusSuccess        Status = "success"
	StatusDenied         Status = "denied"
	StatusBudgetExceeded Status = "budget_exceeded"
	StatusError          Status = "error"
)

// Record is one invocation's audit entry. Time is not a field here — the
// slog handler stamps it, the same way every other log line gets its
// timestamp.
type Record struct {
	Tool        string
	Parameters  map[string]any
	HARequests  int
	Duration    time.Duration
	ResultBytes int64
	Status      Status
	// Reason explains a non-success status — which dimension a budget
	// exhausted, or what the policy refused.
	Reason string
	// Body is the full result payload. Emit persists it only when the
	// Logger was built with WithBody: the default keeps the trail to cost
	// metadata, not a copy of whatever the tool returned.
	Body any
}

// Logger emits audit records through an injected *slog.Logger, never a
// package global (CLAUDE.md, Logging).
type Logger struct {
	log         *slog.Logger
	includeBody bool
}

// New returns a Logger that never persists result bodies.
func New(log *slog.Logger) *Logger {
	return &Logger{log: log}
}

// WithBody returns a Logger that also persists the result body passed in
// Record.Body. Callers opt in per instance, not by a config flag flipped
// once and forgotten.
func (l *Logger) WithBody() *Logger {
	return &Logger{log: l.log, includeBody: true}
}

// Emit writes one audit record. It never fails the call it is recording: a
// broken sink or a payload this package cannot format is caught here rather
// than propagated, because audit is an observer of the invocation, not a
// dependency of it (P2-05 DoD).
func (l *Logger) Emit(ctx context.Context, redactor *redact.Redactor, rec Record) {
	defer func() {
		if r := recover(); r != nil {
			defer func() { recover() }() // the recovery log call must not itself escape
			l.log.ErrorContext(ctx, "audit: emit failed", "recovered", true)
		}
	}()

	params := redactParameters(redactor, rec.Parameters)

	attrs := []any{
		"tool", rec.Tool,
		"parameters", params,
		"ha_requests", rec.HARequests,
		"duration_ms", rec.Duration.Milliseconds(),
		"result_bytes", rec.ResultBytes,
		"status", string(rec.Status),
	}
	if rec.Reason != "" {
		attrs = append(attrs, "reason", rec.Reason)
	}
	if l.includeBody && rec.Body != nil {
		attrs = append(attrs, "body", redactBody(redactor, rec.Body))
	}

	l.log.InfoContext(ctx, "audit", attrs...)
}

// redactParameters applies the invocation's redactor to the parameter map so
// a secret literal or a private value never reaches the trail, matching what
// the response itself would do with the same data (CLAUDE.md rule 4).
func redactParameters(redactor *redact.Redactor, params map[string]any) map[string]any {
	if redactor == nil || params == nil {
		return params
	}
	clean, ok := redactor.Payload(params).Value.(map[string]any)
	if !ok {
		return params
	}
	return clean
}

func redactBody(redactor *redact.Redactor, body any) any {
	if redactor == nil {
		return body
	}
	return redactor.Payload(body).Value
}
