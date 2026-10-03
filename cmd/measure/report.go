package main

import (
	"fmt"
	"strings"
	"time"
)

// callOutcome is what one tool call produced: the server's audit row, the
// wall time the client saw, and the error text if the call was refused.
type callOutcome struct {
	audit  auditRow
	wall   time.Duration
	errMsg string
}

// report accumulates markdown. Nothing here takes an id: labels are ordinals
// and sizes, so the pasted report cannot leak an entity or config entry id.
type report struct{ b strings.Builder }

func (r *report) writef(format string, args ...any) { _, _ = fmt.Fprintf(&r.b, format, args...) }
func (r *report) String() string                    { return r.b.String() }

func (r *report) header() {
	r.writef("| tool | target | HA requests / limit | bytes / limit | server ms | wall | status |\n")
	r.writef("|---|---|--:|--:|--:|--:|---|\n")
}

func (r *report) callRow(tool, label string, o callOutcome, maxRequests int, maxBytes int64) {
	status := o.audit.Status
	if status == "" {
		status = "no audit record"
	}
	if o.audit.Reason != "" {
		status += " (" + o.audit.Reason + ")"
	}
	r.writef("| `%s` | %s | %d / %d | %d / %d | %d | %s | %s |\n",
		tool, label, o.audit.HARequests, maxRequests, o.audit.ResultBytes, maxBytes,
		o.audit.DurationMS, o.wall.Round(time.Millisecond), status)
}
