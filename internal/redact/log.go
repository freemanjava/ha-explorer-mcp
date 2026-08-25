package redact

import (
	"context"
	"log/slog"

	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// NewLogHandler wraps a slog.Handler so no secret literal can reach a log
// line at any level (CLAUDE.md rule 4 and Logging). It is the second half of
// the same guarantee Redactor.Text gives a response: a token withheld from
// the agent but written to stdout is still a token that left the process.
//
// It scrubs known literals only. Deciding not to log a payload in the first
// place stays the caller's job — this is a backstop, not a licence to log
// bodies.
func NewLogHandler(next slog.Handler, secrets ...string) slog.Handler {
	return &logHandler{next: next, scrub: New(policy.Profile{}, secrets...)}
}

type logHandler struct {
	next  slog.Handler
	scrub *Redactor
}

func (h *logHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *logHandler) Handle(ctx context.Context, rec slog.Record) error {
	clean := slog.NewRecord(rec.Time, rec.Level, h.scrub.Text(rec.Message), rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		clean.AddAttrs(h.attr(a))
		return true
	})
	return h.next.Handle(ctx, clean)
}

func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		clean[i] = h.attr(a)
	}
	return &logHandler{next: h.next.WithAttrs(clean), scrub: h.scrub}
}

func (h *logHandler) WithGroup(name string) slog.Handler {
	return &logHandler{next: h.next.WithGroup(h.scrub.Text(name)), scrub: h.scrub}
}

// attr scrubs one attribute, descending into groups. A non-string value is
// scrubbed through its rendered form and only replaced when that changes
// something, so an int stays an int and a struct carrying a token does not
// slip through as one.
func (h *logHandler) attr(a slog.Attr) slog.Attr {
	a.Value = a.Value.Resolve()
	key := h.scrub.Text(a.Key)
	switch a.Value.Kind() {
	case slog.KindGroup:
		group := a.Value.Group()
		clean := make([]slog.Attr, len(group))
		for i, g := range group {
			clean[i] = h.attr(g)
		}
		return slog.Attr{Key: key, Value: slog.GroupValue(clean...)}
	case slog.KindString:
		return slog.String(key, h.scrub.Text(a.Value.String()))
	default:
		rendered := a.Value.String()
		if clean := h.scrub.Text(rendered); clean != rendered {
			return slog.String(key, clean)
		}
		return slog.Attr{Key: key, Value: a.Value}
	}
}
