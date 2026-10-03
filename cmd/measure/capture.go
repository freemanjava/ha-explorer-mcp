package main

import (
	"context"
	"log/slog"
	"sync"
)

// auditRow is one invocation's cost as the server's own audit trail records
// it. Measuring through the audit record, not a re-implementation, is the
// point: the numbers are the ones the budget enforces.
type auditRow struct {
	Tool        string
	HARequests  int
	DurationMS  int64
	ResultBytes int64
	Status      string
	Reason      string
}

// auditCapture is a slog.Handler that keeps only the audit records and drops
// everything else, so the run's stderr stays free of the server's lifecycle
// chatter and no log line can carry anything the report would not.
type auditCapture struct {
	mu   sync.Mutex
	rows []auditRow
}

func (*auditCapture) Enabled(context.Context, slog.Level) bool { return true }

func (c *auditCapture) Handle(_ context.Context, r slog.Record) error {
	if r.Message != "audit" {
		return nil
	}
	var row auditRow
	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "tool":
			row.Tool = a.Value.String()
		case "ha_requests":
			row.HARequests = int(a.Value.Int64())
		case "duration_ms":
			row.DurationMS = a.Value.Int64()
		case "result_bytes":
			row.ResultBytes = a.Value.Int64()
		case "status":
			row.Status = a.Value.String()
		case "reason":
			row.Reason = a.Value.String()
		}
		return true
	})
	c.mu.Lock()
	c.rows = append(c.rows, row)
	c.mu.Unlock()
	return nil
}

func (c *auditCapture) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *auditCapture) WithGroup(string) slog.Handler      { return c }

// take returns what was captured since the last call and clears it.
func (c *auditCapture) take() []auditRow {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.rows
	c.rows = nil
	return out
}
