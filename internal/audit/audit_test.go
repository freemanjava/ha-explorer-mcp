package audit

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
	"github.com/freemanjava/ha-explorer-mcp/internal/redact"
)

const liveToken = "sup3r-secret-token-value"

func newLogger(buf *bytes.Buffer) *Logger {
	return New(slog.New(slog.NewTextHandler(buf, nil)))
}

func TestAuditNeverContainsSecrets(t *testing.T) {
	var buf bytes.Buffer
	log := newLogger(&buf)
	r := redact.New(policy.Profile{}, liveToken)

	log.Emit(context.Background(), r, Record{
		Tool: "get_entity",
		Parameters: map[string]any{
			"entity_id": "sensor.example",
			"token":     liveToken,
		},
		Status: StatusSuccess,
	})

	out := buf.String()
	if strings.Contains(out, liveToken) {
		t.Fatalf("audit record leaked the secret literal: %s", out)
	}
	if !strings.Contains(out, "redacted") {
		t.Fatalf("audit record does not show the parameter was redacted: %s", out)
	}
}

func TestEmit_StatusRecordedForEachOutcome(t *testing.T) {
	cases := []struct {
		name   string
		status Status
	}{
		{"success", StatusSuccess},
		{"denied", StatusDenied},
		{"budget_exceeded", StatusBudgetExceeded},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := newLogger(&buf)
			r := redact.New(policy.Profile{})

			log.Emit(context.Background(), r, Record{
				Tool:   "list_entities",
				Status: tc.status,
				Reason: "test",
			})

			out := buf.String()
			if !strings.Contains(out, string(tc.status)) {
				t.Fatalf("expected status %q in audit record, got: %s", tc.status, out)
			}
		})
	}
}

func TestEmit_RecordsCostFields(t *testing.T) {
	var buf bytes.Buffer
	log := newLogger(&buf)
	r := redact.New(policy.Profile{})

	log.Emit(context.Background(), r, Record{
		Tool:        "get_system_overview",
		HARequests:  3,
		Duration:    83 * time.Millisecond,
		ResultBytes: 18342,
		Status:      StatusSuccess,
	})

	out := buf.String()
	for _, want := range []string{"ha_requests=3", "duration_ms=83", "result_bytes=18342", "tool=get_system_overview"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in audit record, got: %s", want, out)
		}
	}
}

func TestEmit_NoBodyPersistedByDefault(t *testing.T) {
	var buf bytes.Buffer
	log := newLogger(&buf)
	r := redact.New(policy.Profile{})

	const secretBody = "very private household history that must not be persisted"
	log.Emit(context.Background(), r, Record{
		Tool:   "get_entity_history",
		Status: StatusSuccess,
		Body:   map[string]any{"note": secretBody},
	})

	out := buf.String()
	if strings.Contains(out, secretBody) {
		t.Fatalf("default logger persisted a result body: %s", out)
	}
	if strings.Contains(out, "body=") {
		t.Fatalf("default logger emitted a body field at all: %s", out)
	}
}

func TestEmit_BodyPersistedOnlyWhenOptedIn(t *testing.T) {
	var buf bytes.Buffer
	log := newLogger(&buf).WithBody()
	r := redact.New(policy.Profile{})

	log.Emit(context.Background(), r, Record{
		Tool:   "get_entity_history",
		Status: StatusSuccess,
		Body:   map[string]any{"note": "included on purpose"},
	})

	if !strings.Contains(buf.String(), "included on purpose") {
		t.Fatalf("expected opted-in body to be persisted, got: %s", buf.String())
	}
}

// panicHandler simulates an emission failure — a broken sink downstream of
// this package — so Emit can be proven not to let it escape.
type panicHandler struct{}

func (panicHandler) Enabled(context.Context, slog.Level) bool  { return true }
func (panicHandler) Handle(context.Context, slog.Record) error { panic("sink is down") }
func (h panicHandler) WithAttrs([]slog.Attr) slog.Handler      { return h }
func (h panicHandler) WithGroup(string) slog.Handler           { return h }

func TestEmit_EmissionFailureNeverFailsTheCall(t *testing.T) {
	log := New(slog.New(panicHandler{}))
	r := redact.New(policy.Profile{})

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("Emit let a panic escape: %v", rec)
		}
	}()

	log.Emit(context.Background(), r, Record{Tool: "get_entity", Status: StatusSuccess})
}
