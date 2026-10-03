package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/analysis"
	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// maxClusterEntities bounds how many of an integration's entities have their
// recorder history read for outage clustering. Each read is one HA request
// against a composite budget of 50 (policy.compositeMaxHARequests), and one
// integration can own hundreds of entities, so the read is bounded and the
// rest is named in missing_evidence rather than silently dropped. Entities
// that are down now go first. A starting default (doc §26), revisited by
// P5-10.
const maxClusterEntities = 25

// configEntryIDPattern accepts the identifiers HA gives config entries (ULID
// style, but older entries use other word-character ids). It exists so the
// id is never anything a route or query could be built from (rule 2).
var configEntryIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// AnalyzeIntegrationHealthInput is analyze_integration_health's typed input:
// one config entry id and a bounded lookback period — no field accepts a
// route, command or query (rule 2).
type AnalyzeIntegrationHealthInput struct {
	ConfigEntryID string `json:"config_entry_id" jsonschema:"the config entry id of the integration to analyze, as returned by list_integrations"`
	// Period bounds the lookback window ending now, like analyze_entity_health.
	Period *string `json:"period,omitempty" jsonschema:"bounded lookback window ending now, e.g. \"7d\" or \"24h\" (default 7d, max 7d)"`
}

// integrationHealthDeps are the read surfaces analyze_integration_health
// composes. Availability, Repairs and Supervisor are optional: a build without
// one reports it as missing evidence rather than dropping the tool.
type integrationHealthDeps struct {
	history    historyReader
	registry   entityRegistryReader
	avail      entityAvailabilityReader
	repairs    repairReader
	supervisor systemHealthReader
	profile    policy.Profile
}

// withIntegrationHealthTools binds analyze_integration_health when history and
// the registries are available; the row otherwise keeps bindNotImplemented.
func withIntegrationHealthTools(tools []Tool, opts Options) []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	if opts.History == nil || opts.Inventory == nil {
		return out
	}
	deps := integrationHealthDeps{
		history: opts.History, registry: opts.Inventory, avail: opts.Availability,
		repairs: opts.Repairs, supervisor: opts.Supervisor, profile: opts.Profile,
	}
	for i := range out {
		if out[i].Name == "analyze_integration_health" {
			out[i].bind = bindAnalyzeIntegrationHealth(deps)
		}
	}
	return out
}

func bindAnalyzeIntegrationHealth(deps integrationHealthDeps) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in AnalyzeIntegrationHealthInput) (*sdkmcp.CallToolResult, HealthResponse, error) {
			out, err := analyzeIntegrationHealth(ctx, deps, in)
			return nil, out, err
		})
	}
}

// analyzeIntegrationHealth validates the request, resolves the config entry,
// then reads each remaining source independently. A source that cannot be
// read becomes MissingEvidence and lowers what is concluded; it never fails
// the call (doc §3.2). Only the registries are load-bearing: without them
// there is no integration to analyze.
func analyzeIntegrationHealth(ctx context.Context, deps integrationHealthDeps, in AnalyzeIntegrationHealthInput) (HealthResponse, error) {
	if !configEntryIDPattern.MatchString(in.ConfigEntryID) {
		return HealthResponse{}, fmt.Errorf("analyze_integration_health: %q is not a valid config entry id", in.ConfigEntryID)
	}
	window, err := healthWindow("analyze_integration_health", in.Period)
	if err != nil {
		return HealthResponse{}, err
	}

	to := time.Now().UTC()
	input := analysis.IntegrationHealthInput{ObservedAt: to, From: to.Add(-window), To: to}
	entities, err := resolveIntegration(ctx, deps.registry, in.ConfigEntryID, &input)
	if err != nil {
		return HealthResponse{}, err
	}

	down, err := readUnavailable(ctx, deps.avail, entities, &input)
	if err != nil {
		return HealthResponse{}, err
	}
	if err := readOutages(ctx, deps, entities, down, &input); err != nil {
		return HealthResponse{}, err
	}
	if err := readIntegrationRepairs(ctx, deps.repairs, &input); err != nil {
		return HealthResponse{}, err
	}
	if err := readSupervisor(ctx, deps.supervisor, &input); err != nil {
		return HealthResponse{}, err
	}

	result, err := analysis.AnalyzeIntegrationHealth(input)
	if err != nil {
		return HealthResponse{}, err
	}
	out := renderHealth(result)
	if budget, ok := policy.BudgetFrom(ctx); ok {
		if b, mErr := json.Marshal(out); mErr == nil {
			if err := budget.ChargeBytes(int64(len(b))); err != nil {
				return HealthResponse{}, err
			}
		}
	}
	return out, nil
}

// resolveIntegration finds the config entry and the registry rows that belong
// to it, in a stable order. It returns the entry's entities for the later
// reads; an unknown entry is ErrNotFound, which stays distinct from an
// integration with no entities.
func resolveIntegration(ctx context.Context, reg entityRegistryReader, id string, in *analysis.IntegrationHealthInput) ([]model.Entity, error) {
	entries, _, err := reg.ConfigEntries(ctx)
	if err != nil {
		return nil, err
	}
	idx := slices.IndexFunc(entries, func(e model.Integration) bool { return string(e.ID) == id })
	if idx < 0 {
		return nil, fmt.Errorf("%w: config entry %q", ha.ErrNotFound, id)
	}
	all, _, err := reg.Entities(ctx)
	if err != nil {
		return nil, err
	}
	devices, _, err := reg.Devices(ctx)
	if err != nil {
		return nil, err
	}

	in.Entry = entries[idx]
	var entities []model.Entity
	for _, e := range all {
		if e.ConfigEntryID == in.Entry.ID {
			entities = append(entities, e)
		}
	}
	slices.SortFunc(entities, func(a, b model.Entity) int { return compareEntityIDs(a.ID, b.ID) })
	for _, d := range devices {
		if d.ConfigEntryID == in.Entry.ID {
			in.Devices = append(in.Devices, d)
		}
	}
	in.EntityCount, in.DeviceCount = len(entities), len(in.Devices)
	return entities, nil
}

func compareEntityIDs(a, b model.EntityID) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// readUnavailable counts how many of the entry's entities are down now and
// returns the set so history can be read for those first. The set is nil when
// the live state was not read.
func readUnavailable(ctx context.Context, avail entityAvailabilityReader, entities []model.Entity, in *analysis.IntegrationHealthInput) (map[model.EntityID]struct{}, error) {
	if avail == nil {
		in.Missing = append(in.Missing, notConfigured("current entity availability", coreHealthSource))
		return nil, nil
	}
	down, err := avail.UnavailableEntityIDs(ctx)
	if err != nil {
		return nil, noteMissing(&in.Missing, "current entity availability", coreHealthSource, err)
	}
	for _, e := range entities {
		if _, ok := down[e.ID]; ok {
			in.UnavailableNow++
		}
	}
	in.UnavailableRead = true
	return down, nil
}

// readOutages reads recorder history for a bounded, privacy-filtered subset of
// the entry's entities and turns each into an availability report. Entities
// that are down now are read first: they are the ones most likely to carry an
// outage worth clustering. A private entity under a deny profile is excluded
// and counted, never named (T2, rule 6 spirit: no id leaks through the gap).
func readOutages(ctx context.Context, deps integrationHealthDeps, entities []model.Entity, down map[model.EntityID]struct{}, in *analysis.IntegrationHealthInput) error {
	candidates, denied := permittedEntities(deps.profile, entities)
	if denied > 0 {
		in.Missing = append(in.Missing, model.MissingEvidence{
			What: "outage history for some entities", Source: recorderSourceName, Reason: model.MissingPolicyDenied,
			Detail: fmt.Sprintf("%d entities were excluded by the privacy profile", denied),
		})
	}
	if len(candidates) == 0 {
		return nil
	}
	chosen := downFirst(down, candidates)
	if len(chosen) > maxClusterEntities {
		chosen = chosen[:maxClusterEntities]
		in.Missing = append(in.Missing, model.MissingEvidence{
			What: "outage history for some entities", Source: recorderSourceName, Reason: model.MissingBudgetExceeded,
			Detail: fmt.Sprintf("history was read for %d of the integration's %d permitted entities", len(chosen), len(candidates)),
		})
	}

	window := in.To.Sub(in.From)
	budget, hasBudget := policy.BudgetFrom(ctx)
	for i, e := range chosen {
		points, err := readEntityPoints(ctx, deps.history, budget, hasBudget, e.ID, in.From, in.To, window)
		if err != nil {
			// One failure usually repeats for the rest (deadline, budget,
			// recorder down); stop rather than spend the request budget on it.
			return noteMissing(&in.Missing, "outage history", recorderSourceName, err, withPartial(i > 0))
		}
		report, err := analysis.ComputeAvailability(in.From, in.To, points)
		if err != nil {
			return err
		}
		in.Outages = append(in.Outages, analysis.EntityOutages{Entity: e, Availability: report})
		in.OutagesRead = true
	}
	return nil
}

// permittedEntities drops entities the profile will not serve history for.
func permittedEntities(profile policy.Profile, entities []model.Entity) (kept []model.Entity, denied int) {
	for _, e := range entities {
		if err := profile.CheckHistoryScope(policy.HistoryScope{Entities: []model.EntityID{e.ID}}); err != nil {
			denied++
			continue
		}
		kept = append(kept, e)
	}
	return kept, denied
}

// downFirst orders entities with those unavailable now first, keeping the
// stable id order within each group.
func downFirst(down map[model.EntityID]struct{}, entities []model.Entity) []model.Entity {
	out := slices.Clone(entities)
	slices.SortStableFunc(out, func(a, b model.Entity) int {
		_, da := down[a.ID]
		_, db := down[b.ID]
		switch {
		case da && !db:
			return -1
		case db && !da:
			return 1
		}
		return 0
	})
	return out
}

// readEntityPoints reads one entity's history under the invocation budget.
func readEntityPoints(ctx context.Context, reader historyReader, budget *policy.QueryBudget, hasBudget bool,
	id model.EntityID, from, to time.Time, window time.Duration) ([]model.HistoryPoint, error) {
	if hasBudget {
		if err := budget.Preflight(policy.SourceHistory, 1, window); err != nil {
			return nil, err
		}
		if err := budget.ChargeEntities(1); err != nil {
			return nil, err
		}
	}
	points, err := reader.History(ctx, id, from, to, true)
	if err != nil {
		return nil, err
	}
	if hasBudget {
		if err := budget.ChargeHARequests(1); err != nil {
			return nil, err
		}
		if err := budget.ChargeHistoryPoints(len(points)); err != nil {
			return nil, err
		}
	}
	return points, nil
}

func readIntegrationRepairs(ctx context.Context, reader repairReader, in *analysis.IntegrationHealthInput) error {
	if reader == nil {
		in.Missing = append(in.Missing, notConfigured("open repairs", coreHealthSource))
		return nil
	}
	if budget, ok := policy.BudgetFrom(ctx); ok {
		if err := budget.ChargeHARequests(1); err != nil {
			return noteMissing(&in.Missing, "open repairs", coreHealthSource, err)
		}
	}
	repairs, err := reader.Repairs(ctx)
	if err != nil {
		return noteMissing(&in.Missing, "open repairs", coreHealthSource, err)
	}
	in.RepairsRead, in.Repairs = true, repairs
	return nil
}

// readSupervisor reads host-level health. Supervisor being absent, refused or
// down is a normal deployment state (doc §3.2): it lowers what is concluded
// and is named, but the Core-based analysis stands.
func readSupervisor(ctx context.Context, reader systemHealthReader, in *analysis.IntegrationHealthInput) error {
	const what = "supervisor host health"
	if reader == nil {
		in.Missing = append(in.Missing, notConfigured(what, supervisorHealthSource))
		return nil
	}
	if budget, ok := policy.BudgetFrom(ctx); ok {
		if err := budget.ChargeHARequests(1); err != nil {
			return noteMissing(&in.Missing, what, supervisorHealthSource, err)
		}
	}
	summary, err := reader.ResolutionSummary(ctx)
	if err != nil {
		return noteMissing(&in.Missing, what, supervisorHealthSource, err)
	}
	in.SupervisorRead, in.Resolution = true, summary
	return nil
}

const (
	coreHealthSource       = "home_assistant_core"
	recorderSourceName     = "recorder_history"
	supervisorHealthSource = "supervisor"
)

// notConfigured is the gap left by a source this build was not given.
func notConfigured(what, source string) model.MissingEvidence {
	return model.MissingEvidence{
		What: what, Source: source, Reason: model.MissingUpstreamUnavailable,
		Detail: "no source for this evidence is configured in this build",
	}
}

type missingOption func(*model.MissingEvidence)

// withPartial marks that earlier reads of the same source succeeded, so the
// agent knows the evidence is thinner rather than absent.
func withPartial(partial bool) missingOption {
	return func(m *model.MissingEvidence) {
		if partial {
			m.Detail += "; earlier reads of this source succeeded, so the evidence is partial"
		}
	}
}

// noteMissing records a failed read as MissingEvidence. It returns non-nil
// only when the caller itself went away.
func noteMissing(dst *[]model.MissingEvidence, what, source string, err error, opts ...missingOption) error {
	m, abort := missingFor(what, source, err)
	if abort != nil {
		return abort
	}
	for _, o := range opts {
		o(&m)
	}
	*dst = append(*dst, m)
	return nil
}
