package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/page"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
	"github.com/freemanjava/ha-explorer-mcp/internal/redact"
)

// automationReader is list_automations' read surface: get_states' automation-
// domain entries, mapped to enabled state and last_triggered — the confirmed
// non-admin fallback source
// (docs/research/2026-08-23-ha-automation-traces.md), not automation/config's
// admin-gated detail, which get_automation (P3-07) reaches instead.
type automationReader interface {
	Automations(ctx context.Context) ([]model.AutomationSummary, error)
}

// ListAutomationsInput is list_automations' typed, validated input: an
// optional enabled filter plus the Phase 02 cursor-pagination contract.
type ListAutomationsInput struct {
	// Enabled, given, filters to only-enabled (true) or only-disabled
	// (false) automations. Omitted means no filter.
	Enabled *bool `json:"enabled,omitempty" jsonschema:"filter by whether the automation is enabled"`

	Cursor string `json:"cursor,omitempty" jsonschema:"resume after this page's cursor"`
	Limit  int    `json:"limit,omitempty" jsonschema:"page size, default 50, max 200"`
}

// automationDetailReader is get_automation/get_automation_traces' admin-gated
// read surface — automation/config and trace/list, confirmed reachable only
// for an admin principal (P0-05/P0-06). Distinct from automationReader's
// get_states fallback, which any principal can read.
type automationDetailReader interface {
	AutomationDetail(ctx context.Context, entityID model.EntityID) (model.Automation, error)
	AutomationTraces(ctx context.Context, entityID model.EntityID) ([]model.AutomationTraceSummary, error)
}

// automationVersionReader supplies the detected HA version for the
// version-absent branch's reason (P3-07 DoD). Reuses whatever Options.Core
// already satisfies rather than a second field; nil — a valid deployment
// state — reports the version as "unknown" rather than failing the tool.
type automationVersionReader interface {
	CoreConfig(ctx context.Context) (model.CoreConfig, error)
}

// logbookReader is get_automation_traces' non-admin fallback evidence source
// (F-11): logbook/get_events, confirmed reachable at any principal.
type logbookReader interface {
	LogbookEvents(ctx context.Context, entityID model.EntityID, since time.Time) ([]model.LogbookEvent, error)
}

// logbookFallbackWindow is how far back get_automation_traces' degraded path
// looks for logbook events — the same 24h window P0-05's probe observed
// working for a non-admin principal
// (docs/research/2026-08-23-ha-automation-traces.md).
const logbookFallbackWindow = 24 * time.Hour

// withAutomationTools returns tools with list_automations, get_automation and
// get_automation_traces' handlers bound, for whichever opts supplies readers
// for. A row whose readers are absent keeps its bindNotImplemented default.
func withAutomationTools(tools []Tool, opts Options) []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	for i := range out {
		switch out[i].Name {
		case "list_automations":
			if opts.Automations != nil {
				out[i].bind = bindListAutomations(opts.Automations)
			}
		case "get_automation":
			if opts.AutomationDetail != nil {
				out[i].bind = bindGetAutomation(opts.AutomationDetail, opts.Core)
			}
		case "get_automation_traces":
			if opts.AutomationDetail != nil {
				out[i].bind = bindGetAutomationTraces(opts.AutomationDetail, opts.Core, opts.Automations, opts.Logbook, opts.Profile, opts.Secrets)
			}
		}
	}
	return out
}

// bindListAutomations registers list_automations' typed handler.
func bindListAutomations(reader automationReader) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in ListAutomationsInput) (*sdkmcp.CallToolResult, model.AutomationList, error) {
			out, err := listAutomations(ctx, reader, in)
			return nil, out, err
		})
	}
}

// listAutomations filters, sorts by entity id, and pages the automation
// summaries get_states yields.
func listAutomations(ctx context.Context, reader automationReader, in ListAutomationsInput) (model.AutomationList, error) {
	automations, err := reader.Automations(ctx)
	if err != nil {
		return model.AutomationList{}, err
	}

	filtered := make([]model.AutomationSummary, 0, len(automations))
	for _, a := range automations {
		if in.Enabled != nil && a.Enabled != *in.Enabled {
			continue
		}
		filtered = append(filtered, a)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].EntityID < filtered[j].EntityID })

	pg, err := page.Paginate(filtered, in.Cursor, in.Limit, maxResponseBytes(ctx),
		func(a model.AutomationSummary) string { return string(a.EntityID) },
		automationByteSize,
	)
	if err != nil {
		return model.AutomationList{}, err
	}

	return model.AutomationList{
		Source:       "home_assistant_core",
		ObservedAt:   time.Now().UTC(),
		Items:        pg.Items,
		NextCursor:   pg.NextCursor,
		Truncated:    pg.Truncated,
		LimitClamped: pg.LimitClamped,
	}, nil
}

// automationByteSize approximates one automation summary's serialized size
// for the page package's byte cap — cheap enough to run per record without
// re-serializing the whole response afterward.
func automationByteSize(a model.AutomationSummary) int64 {
	b, err := json.Marshal(a)
	if err != nil {
		return 0
	}
	return int64(len(b))
}

// GetAutomationInput is get_automation's typed input: exactly the entity id,
// nothing an agent could use as a free-form route or query.
type GetAutomationInput struct {
	EntityID string `json:"entity_id" jsonschema:"the automation entity id, e.g. automation.evening_lights"`
}

// GetAutomationTracesInput is get_automation_traces' typed input: the entity
// id plus the Phase 02 cursor-pagination contract.
type GetAutomationTracesInput struct {
	EntityID string `json:"entity_id" jsonschema:"the automation entity id, e.g. automation.evening_lights"`

	Cursor string `json:"cursor,omitempty" jsonschema:"resume after this page's cursor"`
	Limit  int    `json:"limit,omitempty" jsonschema:"page size, default 50, max 200"`
}

// automationFallbackReason names get_automation's non-admin fallback: a whole
// other tool, since list_automations already reports everything a non-admin
// principal can see about an automation (P3-07 DoD: "points at the degraded
// evidence path").
const automationFallbackReason = "list_automations, which reports enabled state and last_triggered from get_states and is reachable at any principal"

// traceFallbackReason names get_automation_traces' non-admin fallback:
// last_triggered plus logbook/get_events, both fetched into this response's
// Fallback* fields rather than merely mentioned (P3-07 DoD, F-11).
const traceFallbackReason = "last_triggered (list_automations) plus logbook/get_events correlated by context_id, both attached to this response's fallback_* fields"

// validateAutomationEntityID rejects anything that is not shaped like an
// automation entity id before a command is ever built. HA data is untrusted
// (CLAUDE.md rule 6), and a plainly wrong domain is a validation error, not a
// fact about the installation worth spending a round trip on.
func validateAutomationEntityID(entityID string) error {
	if !strings.HasPrefix(entityID, "automation.") || len(entityID) == len("automation.") {
		return fmt.Errorf("entity_id %q is not an automation entity id", entityID)
	}
	return nil
}

// classifyAutomationError turns an automation/config or trace/list failure
// into get_automation/get_automation_traces' three-way outcome (P3-07 DoD):
// permission-refused and version-absent are each "unsupported" with their own
// reason, kept distinct from one another and from a genuine failure, which is
// returned unchanged (ok false) for the caller to report as a real tool error
// rather than a fact about the installation (CLAUDE.md rule 7).
func classifyAutomationError(ctx context.Context, versions automationVersionReader, err error, fallback string) (reason string, ok bool) {
	var cmdErr *ha.CommandError
	switch {
	case errors.Is(err, ha.ErrUnsupported):
		return fmt.Sprintf("Home Assistant refused this request to the current principal (permission denied); fall back to %s.", fallback), true
	case errors.As(err, &cmdErr) && cmdErr.Code == "unknown_command":
		return fmt.Sprintf("not available on Home Assistant %s: %s", detectedVersion(ctx, versions), cmdErr.Message), true
	default:
		return "", false
	}
}

// detectedVersion reports the installed HA version for a version-unsupported
// reason, or "unknown" when it cannot be determined — never failing the tool
// over a detail that only makes the reason more specific.
func detectedVersion(ctx context.Context, versions automationVersionReader) string {
	if versions == nil {
		return "unknown"
	}
	cfg, err := versions.CoreConfig(ctx)
	if err != nil || cfg.Version == "" {
		return "unknown"
	}
	return cfg.Version
}

// bindGetAutomation registers get_automation's typed handler.
func bindGetAutomation(detail automationDetailReader, versions automationVersionReader) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in GetAutomationInput) (*sdkmcp.CallToolResult, model.Automation, error) {
			out, err := getAutomation(ctx, detail, versions, in)
			return nil, out, err
		})
	}
}

// getAutomation reads automation/config through the admin-gated adapter.
// Where HA refuses it by permission or does not offer it on the detected
// version, the response degrades to Unsupported with a reason naming which
// and pointing at list_automations (P3-07 DoD) rather than returning a Go
// tool error for an expected, nameable state (CLAUDE.md rule 7).
func getAutomation(ctx context.Context, detail automationDetailReader, versions automationVersionReader, in GetAutomationInput) (model.Automation, error) {
	if err := validateAutomationEntityID(in.EntityID); err != nil {
		return model.Automation{}, fmt.Errorf("get_automation: %w", err)
	}
	entityID := model.EntityID(in.EntityID)

	a, err := detail.AutomationDetail(ctx, entityID)
	if err != nil {
		if reason, ok := classifyAutomationError(ctx, versions, err, automationFallbackReason); ok {
			return model.Automation{
				Source:            "home_assistant_core",
				ObservedAt:        time.Now().UTC(),
				EntityID:          entityID,
				Unsupported:       true,
				UnsupportedReason: reason,
			}, nil
		}
		return model.Automation{}, err
	}

	a.Source = "home_assistant_core"
	a.ObservedAt = time.Now().UTC()
	return a, nil
}

// bindGetAutomationTraces registers get_automation_traces' typed handler.
func bindGetAutomationTraces(detail automationDetailReader, versions automationVersionReader, automations automationReader, logbook logbookReader, profile policy.Profile, secrets []string) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in GetAutomationTracesInput) (*sdkmcp.CallToolResult, model.AutomationTraceList, error) {
			out, err := getAutomationTraces(ctx, detail, versions, automations, logbook, profile, secrets, in)
			return nil, out, err
		})
	}
}

// getAutomationTraces reads trace/list through the admin-gated adapter,
// newest run first. Where HA refuses it by permission, the response degrades
// to Unsupported and additionally attaches the confirmed non-admin fallback
// evidence — last_triggered and recent logbook events — fetched live rather
// than merely named (P3-07 DoD, F-11); where the version does not offer
// trace/list at all, no fallback fetch is attempted, since an HA release that
// dropped trace/list did not thereby change last_triggered or the logbook.
func getAutomationTraces(ctx context.Context, detail automationDetailReader, versions automationVersionReader, automations automationReader, logbook logbookReader, profile policy.Profile, secrets []string, in GetAutomationTracesInput) (model.AutomationTraceList, error) {
	if err := validateAutomationEntityID(in.EntityID); err != nil {
		return model.AutomationTraceList{}, fmt.Errorf("get_automation_traces: %w", err)
	}
	entityID := model.EntityID(in.EntityID)

	out := model.AutomationTraceList{Source: "home_assistant_core", ObservedAt: time.Now().UTC()}

	traces, err := detail.AutomationTraces(ctx, entityID)
	if err != nil {
		reason, ok := classifyAutomationError(ctx, versions, err, traceFallbackReason)
		if !ok {
			return model.AutomationTraceList{}, err
		}
		out.Unsupported = true
		out.UnsupportedReason = reason
		if errors.Is(err, ha.ErrUnsupported) {
			attachAutomationFallback(ctx, &out, automations, logbook, redact.New(profile, secrets...), entityID)
		}
		return out, nil
	}

	sort.Slice(traces, func(i, j int) bool { return traces[i].TimestampStart.After(traces[j].TimestampStart) })
	pg, err := page.Paginate(traces, in.Cursor, in.Limit, maxResponseBytes(ctx),
		func(t model.AutomationTraceSummary) string { return t.RunID },
		automationTraceByteSize,
	)
	if err != nil {
		return model.AutomationTraceList{}, err
	}
	out.Items = pg.Items
	out.NextCursor = pg.NextCursor
	out.Truncated = pg.Truncated
	out.LimitClamped = pg.LimitClamped
	return out, nil
}

// attachAutomationFallback fills out's Fallback* fields from the confirmed
// non-admin sources (F-11). Either source failing degrades that one field
// silently rather than the whole response: a fallback that cannot itself be
// fully served is still better than none, and this path is already the
// degraded case.
func attachAutomationFallback(ctx context.Context, out *model.AutomationTraceList, automations automationReader, logbook logbookReader, redactor *redact.Redactor, entityID model.EntityID) {
	if automations != nil {
		if all, err := automations.Automations(ctx); err == nil {
			for _, a := range all {
				if a.EntityID == entityID {
					out.FallbackLastTriggered = a.LastTriggered
					break
				}
			}
		}
	}
	if logbook != nil {
		if events, err := logbook.LogbookEvents(ctx, entityID, time.Now().Add(-logbookFallbackWindow)); err == nil {
			out.FallbackEvents = maskFallbackEvents(redactor, events)
		}
	}
}

// maskFallbackEvents applies the Phase 02 profile to the degraded path's
// logbook prose (F-23, P3-09 decision). Each event is classified by its own
// entity and masked whole: a logbook message is text HA composed from a
// friendly name and a state, with no field boundary inside it, and searching
// it for the entity's name or state would be branching on untrusted content
// (CLAUDE.md rule 6). When, ContextID and the event's presence survive at
// every classification, so the agent still learns that the automation ran and
// when; one Redactor serves the whole response, so equal values share a token
// as maskHistoryPoints already requires.
func maskFallbackEvents(redactor *redact.Redactor, events []model.LogbookEvent) []model.LogbookEvent {
	if len(events) == 0 {
		return events
	}
	out := make([]model.LogbookEvent, len(events))
	for i, ev := range events {
		out[i] = ev
		// The event's own entity decides, not the automation the tool was
		// called for: the message describes whatever entity the logbook
		// entry is about, which on the trigger's entry is the private one.
		class := policy.ClassifyEntity(ev.EntityID)
		out[i].Name = redactor.MaskedText(class, string(ev.EntityID), ev.Name)
		out[i].Message = redactor.MaskedText(class, string(ev.EntityID), ev.Message)
	}
	return out
}

// automationTraceByteSize approximates one trace summary's serialized size
// for the page package's byte cap — cheap enough to run per record without
// re-serializing the whole response afterward.
func automationTraceByteSize(t model.AutomationTraceSummary) int64 {
	b, err := json.Marshal(t)
	if err != nil {
		return 0
	}
	return int64(len(b))
}
