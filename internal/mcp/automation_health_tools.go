package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/analysis"
	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// AnalyzeAutomationHealthInput is analyze_automation_health's typed input: one
// automation entity id and a bounded lookback period — no field accepts a
// route, command or query (rule 2).
type AnalyzeAutomationHealthInput struct {
	EntityID string `json:"entity_id" jsonschema:"the automation entity id, e.g. automation.evening_lights"`
	// Period bounds the lookback window ending now, like the other analyze_* tools.
	Period *string `json:"period,omitempty" jsonschema:"bounded lookback window ending now, e.g. \"7d\" or \"24h\" (default 7d, max 7d)"`
}

// DependencyEvidenceView says which dependency entity a per-dependency
// evidence id measured. The id itself is analysis-authored (rule 6).
type DependencyEvidenceView struct {
	Evidence model.EvidenceID
	EntityID model.EntityID
}

// AutomationHealthResponse is analyze_automation_health's response: the shared
// health envelope plus the evidence-id → dependency-entity map.
type AutomationHealthResponse struct {
	HealthResponse
	DependencyEvidence []DependencyEvidenceView
}

// automationHealthDeps are the read surfaces analyze_automation_health
// composes. Every one but history and the registries is optional: a build
// without one reports that source as missing evidence.
type automationHealthDeps struct {
	detail      automationDetailReader
	automations automationReader
	logbook     logbookReader
	history     historyReader
	registry    entityRegistryReader
	repairs     repairReader
	profile     policy.Profile
}

// withAutomationHealthTools binds analyze_automation_health when history and
// the registries are available; the row otherwise keeps bindNotImplemented.
func withAutomationHealthTools(tools []Tool, opts Options) []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	if opts.History == nil || opts.Inventory == nil {
		return out
	}
	deps := automationHealthDeps{
		detail: opts.AutomationDetail, automations: opts.Automations, logbook: opts.Logbook,
		history: opts.History, registry: opts.Inventory, repairs: opts.Repairs, profile: opts.Profile,
	}
	for i := range out {
		if out[i].Name == "analyze_automation_health" {
			out[i].bind = bindAnalyzeAutomationHealth(deps)
		}
	}
	return out
}

func bindAnalyzeAutomationHealth(deps automationHealthDeps) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in AnalyzeAutomationHealthInput) (*sdkmcp.CallToolResult, AutomationHealthResponse, error) {
			out, err := analyzeAutomationHealth(ctx, deps, in)
			return nil, out, err
		})
	}
}

// analyzeAutomationHealth validates the request, then reads each source
// independently. A source that cannot be read becomes MissingEvidence and
// lowers what is concluded; it never fails the call (doc §3.2) — a non-admin
// principal, for whom automation/config and trace/list are refused, still gets
// the logbook-based answer.
func analyzeAutomationHealth(ctx context.Context, deps automationHealthDeps, in AnalyzeAutomationHealthInput) (AutomationHealthResponse, error) {
	if err := validateAutomationEntityID(in.EntityID); err != nil || !historyEntityIDPattern.MatchString(in.EntityID) {
		return AutomationHealthResponse{}, fmt.Errorf("analyze_automation_health: %q is not a valid automation entity id", in.EntityID)
	}
	entityID := model.EntityID(in.EntityID)
	window, err := healthWindow("analyze_automation_health", in.Period)
	if err != nil {
		return AutomationHealthResponse{}, err
	}

	to := time.Now().UTC()
	input := analysis.AutomationHealthInput{
		Automation: model.Automation{EntityID: entityID}, ObservedAt: to, From: to.Add(-window), To: to,
	}
	summary, err := readAutomationSummary(ctx, deps.automations, entityID, &input)
	if err != nil {
		return AutomationHealthResponse{}, err
	}
	if err := readAutomationConfig(ctx, deps.detail, entityID, &input); err != nil {
		return AutomationHealthResponse{}, err
	}
	if err := readAutomationTraces(ctx, deps.detail, entityID, &input); err != nil {
		return AutomationHealthResponse{}, err
	}
	if !input.TracesRead {
		if err := readAutomationFallback(ctx, deps.logbook, entityID, summary, &input); err != nil {
			return AutomationHealthResponse{}, err
		}
	}
	if err := readDependencies(ctx, deps, &input); err != nil {
		return AutomationHealthResponse{}, err
	}
	if err := readAutomationRepairs(ctx, deps.repairs, &input); err != nil {
		return AutomationHealthResponse{}, err
	}

	result, err := analysis.AnalyzeAutomationHealth(input)
	if err != nil {
		return AutomationHealthResponse{}, err
	}
	out := renderAutomationHealth(result)
	if budget, ok := policy.BudgetFrom(ctx); ok {
		if b, mErr := json.Marshal(out); mErr == nil {
			if err := budget.ChargeBytes(int64(len(b))); err != nil {
				return AutomationHealthResponse{}, err
			}
		}
	}
	return out, nil
}

// renderAutomationHealth is the serialized rendering of the analysis (D-05-1).
func renderAutomationHealth(a analysis.AutomationHealth) AutomationHealthResponse {
	refs := make([]DependencyEvidenceView, 0, len(a.DependencyEvidence))
	for _, r := range a.DependencyEvidence {
		refs = append(refs, DependencyEvidenceView{Evidence: r.Evidence, EntityID: r.EntityID})
	}
	return AutomationHealthResponse{HealthResponse: renderHealth(a.HealthAnalysis), DependencyEvidence: refs}
}

// readAutomationSummary finds the automation in get_states, which any
// principal can read: it settles "does this automation exist" independently of
// the admin-gated detail, and supplies last_triggered for the fallback. A nil
// reader or a failed read leaves existence unchecked and the summary nil.
func readAutomationSummary(ctx context.Context, reader automationReader, id model.EntityID, in *analysis.AutomationHealthInput) (*model.AutomationSummary, error) {
	if reader == nil {
		return nil, nil
	}
	all, err := reader.Automations(ctx)
	if err != nil {
		return nil, noteMissing(&in.Missing, "automation inventory", coreHealthSource, err)
	}
	idx := slices.IndexFunc(all, func(a model.AutomationSummary) bool { return a.EntityID == id })
	if idx < 0 {
		return nil, fmt.Errorf("%w: automation %q", ha.ErrNotFound, id)
	}
	return &all[idx], nil
}

// unreadReason maps a failed read onto the reason the analysis names it by,
// aborting only when the caller itself went away.
func unreadReason(err error) (model.MissingReason, error) {
	m, abort := missingFor("", "", err)
	return m.Reason, abort
}

func readAutomationConfig(ctx context.Context, reader automationDetailReader, id model.EntityID, in *analysis.AutomationHealthInput) error {
	if reader == nil {
		in.ConfigUnread = model.MissingUpstreamUnavailable
		return nil
	}
	a, err := fetchAutomationConfig(ctx, reader, id)
	if err != nil {
		reason, abort := unreadReason(err)
		in.ConfigUnread = reason
		return abort
	}
	a.EntityID = id
	in.ConfigRead, in.Automation = true, a
	return nil
}

func fetchAutomationConfig(ctx context.Context, reader automationDetailReader, id model.EntityID) (model.Automation, error) {
	return reader.AutomationDetail(ctx, id)
}

func readAutomationTraces(ctx context.Context, reader automationDetailReader, id model.EntityID, in *analysis.AutomationHealthInput) error {
	if reader == nil {
		in.TracesUnread = model.MissingUpstreamUnavailable
		return nil
	}
	traces, err := reader.AutomationTraces(ctx, id)
	if err != nil {
		reason, abort := unreadReason(err)
		in.TracesUnread = reason
		return abort
	}
	in.TracesRead, in.Traces = true, traces
	return nil
}

// readAutomationFallback reads the F-11 non-admin evidence — last_triggered
// and the automation's own logbook entries — when traces were not readable.
// The window is the shorter of the period and logbookFallbackWindow, the span
// the logbook was observed to serve any principal.
func readAutomationFallback(ctx context.Context, reader logbookReader, id model.EntityID, summary *model.AutomationSummary, in *analysis.AutomationHealthInput) error {
	if reader == nil {
		in.FallbackUnread = model.MissingUpstreamUnavailable
		return nil
	}
	since := in.To.Add(-min(in.To.Sub(in.From), logbookFallbackWindow))
	events, err := reader.LogbookEvents(ctx, id, since)
	if err != nil {
		reason, abort := unreadReason(err)
		in.FallbackUnread = reason
		return abort
	}
	in.FallbackRead, in.LogbookEvents, in.LogbookSince = true, events, since
	if summary != nil {
		in.LastTriggered = summary.LastTriggered
	}
	return nil
}

// readDependencies resolves the automation's dependencies to entities and
// reads recorder history for at most maxClusterEntities of them. A private
// entity under a deny profile is excluded and one past the cap is left unread;
// both reach the analysis as unread histories with the reason, so they are
// counted and never named (rule 6 spirit: no id leaks through the gap).
func readDependencies(ctx context.Context, deps automationHealthDeps, in *analysis.AutomationHealthInput) error {
	if !in.ConfigRead {
		return nil
	}
	ids, err := resolveDependencyEntities(ctx, deps.registry, in)
	if err != nil {
		return err
	}
	allowed, _ := permittedEntities(deps.profile, entitiesOf(ids))
	window := in.To.Sub(in.From)
	budget, hasBudget := policy.BudgetFrom(ctx)

	read := 0
	var readErr error
	for _, id := range ids {
		d := analysis.DependencyHistory{EntityID: id}
		switch {
		case !slices.ContainsFunc(allowed, func(e model.Entity) bool { return e.ID == id }):
			d.UnreadReason = model.MissingPolicyDenied
		case read >= maxClusterEntities:
			d.UnreadReason = model.MissingBudgetExceeded
		case readErr != nil:
			// One failure usually repeats for the rest (deadline, budget,
			// recorder down); stop rather than spend the request budget on it.
			d.UnreadReason, _ = unreadReason(readErr)
		default:
			read++
			points, err := readEntityPoints(ctx, deps.history, budget, hasBudget, id, in.From, in.To, window)
			if err != nil {
				reason, abort := unreadReason(err)
				if abort != nil {
					return abort
				}
				readErr, d.UnreadReason = err, reason
				break
			}
			d.Read, d.Points = true, points
		}
		in.Dependencies = append(in.Dependencies, d)
	}
	return nil
}

func entitiesOf(ids []model.EntityID) []model.Entity {
	out := make([]model.Entity, len(ids))
	for i, id := range ids {
		out[i] = model.Entity{ID: id}
	}
	return out
}

// resolveDependencyEntities expands the automation's entity, device and area
// dependencies into one sorted, de-duplicated entity list. Devices and areas
// resolve through the cached registries; if those cannot be read the direct
// entity dependencies still stand and the gap is named.
func resolveDependencyEntities(ctx context.Context, reg entityRegistryReader, in *analysis.AutomationHealthInput) ([]model.EntityID, error) {
	dep := in.Automation.DependsOn
	set := map[model.EntityID]struct{}{}
	for _, id := range dep.Entities {
		set[id] = struct{}{}
	}
	if len(dep.Devices) > 0 || len(dep.Areas) > 0 {
		resolved, err := entitiesOfDevicesAndAreas(ctx, reg, dep)
		if err != nil {
			if abort := noteMissing(&in.Missing, "entities of device and area dependencies", coreHealthSource, err); abort != nil {
				return nil, abort
			}
		}
		for _, id := range resolved {
			set[id] = struct{}{}
		}
	}
	ids := make([]model.EntityID, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, compareEntityIDs)
	return ids, nil
}

// entitiesOfDevicesAndAreas returns the registry entities that belong to a
// dependency device, or to a dependency area — directly, or through their
// device when the entity carries no area of its own.
func entitiesOfDevicesAndAreas(ctx context.Context, reg entityRegistryReader, dep model.AutomationDependencies) ([]model.EntityID, error) {
	entities, _, err := reg.Entities(ctx)
	if err != nil {
		return nil, err
	}
	devices, _, err := reg.Devices(ctx)
	if err != nil {
		return nil, err
	}
	deviceArea := make(map[model.DeviceID]model.AreaID, len(devices))
	for _, d := range devices {
		deviceArea[d.ID] = d.AreaID
	}
	var out []model.EntityID
	for _, e := range entities {
		area := e.AreaID
		if area == "" && e.DeviceID != "" {
			area = deviceArea[e.DeviceID]
		}
		onDevice := e.DeviceID != "" && slices.Contains(dep.Devices, e.DeviceID)
		inArea := area != "" && slices.Contains(dep.Areas, area)
		if onDevice || inArea {
			out = append(out, e.ID)
		}
	}
	return out, nil
}

func readAutomationRepairs(ctx context.Context, reader repairReader, in *analysis.AutomationHealthInput) error {
	if reader == nil {
		in.Missing = append(in.Missing, notConfigured("open repairs", coreHealthSource))
		return nil
	}
	repairs, err := reader.Repairs(ctx)
	if err != nil {
		return noteMissing(&in.Missing, "open repairs", coreHealthSource, err)
	}
	in.RepairsRead, in.Repairs = true, repairs
	return nil
}
