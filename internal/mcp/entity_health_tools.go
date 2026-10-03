package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/analysis"
	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// healthSource names the response as a composition: each Evidence
// carries the subsystem it actually came from.
const healthSource = "composite"

// AnalyzeEntityHealthInput is analyze_entity_health's typed input (Appendix
// A.3): one entity id and a bounded lookback period — no field accepts a
// route, command or query (rule 2).
type AnalyzeEntityHealthInput struct {
	EntityID string `json:"entity_id" jsonschema:"the entity id to analyze"`
	// Period bounds the lookback window ending now. Defaults to 7d and is
	// refused above maxHistoryWindow, like get_entity_statistics.
	Period *string `json:"period,omitempty" jsonschema:"bounded lookback window ending now, e.g. \"7d\" or \"24h\" (default 7d, max 7d)"`
}

// HypothesisView is the serialized rendering of a model.Hypothesis, whose
// fields are unexported so that only NewHypothesis can build one (D-05-1).
type HypothesisView struct {
	Statement  string
	Confidence string
	Cites      []model.EvidenceID
}

// HealthResponse is analyze_entity_health's response: fact (Evidence),
// inference (Hypotheses) and recommendation (NextActions) in separate
// fields, with what could not be observed named in MissingEvidence. It has no
// score (D-05-4).
type HealthResponse struct {
	Source     string
	ObservedAt time.Time
	SubjectID  string
	From       time.Time
	To         time.Time

	Evidence        []model.Evidence
	Hypotheses      []HypothesisView
	MissingEvidence []model.MissingEvidence
	NextActions     []model.NextAction

	model.Provenance
}

// entityHealthDeps are the read surfaces analyze_entity_health composes.
// Repairs is optional: a build without it reports repairs as missing
// evidence rather than dropping the tool.
type entityHealthDeps struct {
	history  historyReader
	registry entityRegistryReader
	repairs  repairReader
	profile  policy.Profile
}

// withEntityHealthTools binds analyze_entity_health when history and the
// registries are available; the row otherwise keeps bindNotImplemented.
func withEntityHealthTools(tools []Tool, opts Options) []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	if opts.History == nil || opts.Inventory == nil {
		return out
	}
	deps := entityHealthDeps{history: opts.History, registry: opts.Inventory, repairs: opts.Repairs, profile: opts.Profile}
	for i := range out {
		if out[i].Name == "analyze_entity_health" {
			out[i].bind = bindAnalyzeEntityHealth(deps)
		}
	}
	return out
}

func bindAnalyzeEntityHealth(deps entityHealthDeps) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in AnalyzeEntityHealthInput) (*sdkmcp.CallToolResult, HealthResponse, error) {
			out, err := analyzeEntityHealth(ctx, deps, in)
			return nil, out, err
		})
	}
}

// analyzeEntityHealth validates the request, refuses what the range cap and
// the privacy profile will not serve, then reads each source independently.
// A source that cannot be read becomes MissingEvidence and lowers what is
// concluded; it never fails the call (doc §3.2). Refusal order mirrors
// get_entity_statistics: shape and range, then the profile, then any read.
func analyzeEntityHealth(ctx context.Context, deps entityHealthDeps, in AnalyzeEntityHealthInput) (HealthResponse, error) {
	if !historyEntityIDPattern.MatchString(in.EntityID) {
		return HealthResponse{}, fmt.Errorf("analyze_entity_health: %q is not a valid entity id", in.EntityID)
	}
	entityID := model.EntityID(in.EntityID)

	window, err := healthWindow("analyze_entity_health", in.Period)
	if err != nil {
		return HealthResponse{}, err
	}
	if err := deps.profile.CheckHistoryScope(policy.HistoryScope{Entities: []model.EntityID{entityID}}); err != nil {
		return HealthResponse{}, err
	}

	to := time.Now().UTC()
	input := analysis.EntityHealthInput{EntityID: entityID, ObservedAt: to, From: to.Add(-window), To: to}

	if err := readHistory(ctx, deps.history, &input); err != nil {
		return HealthResponse{}, err
	}
	if err := readRegistry(ctx, deps.registry, &input); err != nil {
		return HealthResponse{}, err
	}
	if input.RegistryRead && input.Entity == nil && input.HistoryRead && len(input.Points) == 0 {
		return HealthResponse{}, fmt.Errorf("%w: entity %q", ha.ErrNotFound, in.EntityID)
	}
	if err := readRepairs(ctx, deps.repairs, &input); err != nil {
		return HealthResponse{}, err
	}

	result, err := analysis.AnalyzeEntityHealth(input)
	if err != nil {
		return HealthResponse{}, err
	}
	out := renderHealth(result)

	if budget, ok := policy.BudgetFrom(ctx); ok {
		b, mErr := json.Marshal(out)
		if mErr == nil {
			if err := budget.ChargeBytes(int64(len(b))); err != nil {
				return HealthResponse{}, err
			}
		}
	}
	return out, nil
}

// healthWindow parses and bounds a composite health tool's lookback period.
func healthWindow(tool string, period *string) (time.Duration, error) {
	periodStr := defaultStatisticsPeriod
	if period != nil && *period != "" {
		periodStr = *period
	}
	window, err := parseStatisticsPeriod(periodStr)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a valid period", tool, periodStr)
	}
	if window <= 0 {
		return 0, fmt.Errorf("%s: period %q must be positive", tool, periodStr)
	}
	if window > maxHistoryWindow {
		return 0, fmt.Errorf("%w: %s: requested period %s exceeds the maximum %s",
			policy.ErrPolicyDenied, tool, window, maxHistoryWindow)
	}
	return window, nil
}

// readHistory reads the recorder once. Only a cancelled caller aborts;
// every other failure is recorded as missing evidence.
func readHistory(ctx context.Context, reader historyReader, in *analysis.EntityHealthInput) error {
	missing := func(err error) error {
		m, abort := missingFor("recorder history", "recorder_history", err)
		if abort != nil {
			return abort
		}
		in.Missing = append(in.Missing, m)
		return nil
	}
	window := in.To.Sub(in.From)
	if budget, ok := policy.BudgetFrom(ctx); ok {
		if err := budget.Preflight(policy.SourceHistory, 1, window); err != nil {
			return missing(err)
		}
	}
	// Aggregates only: attributes are never needed, so the minimal shape is
	// always requested.
	points, err := reader.History(ctx, in.EntityID, in.From, in.To, true)
	if err != nil {
		return missing(err)
	}
	if budget, ok := policy.BudgetFrom(ctx); ok {
		if err := budget.ChargeHARequests(1); err != nil {
			return missing(err)
		}
		if err := budget.ChargeHistoryPoints(len(points)); err != nil {
			return missing(err)
		}
	}
	in.HistoryRead, in.Points = true, points
	return nil
}

// readRegistry resolves the entity, its device and its config entry from the
// cached registries.
func readRegistry(ctx context.Context, reader entityRegistryReader, in *analysis.EntityHealthInput) error {
	entities, _, err := reader.Entities(ctx)
	if err != nil {
		return recordMissing(in, "entity registry", "home_assistant_core", err)
	}
	devices, _, err := reader.Devices(ctx)
	if err != nil {
		return recordMissing(in, "device registry", "home_assistant_core", err)
	}
	entries, _, err := reader.ConfigEntries(ctx)
	if err != nil {
		return recordMissing(in, "config entries", "home_assistant_core", err)
	}
	in.RegistryRead = true
	for i := range entities {
		if entities[i].ID != in.EntityID {
			continue
		}
		e := entities[i]
		in.Entity = &e
		break
	}
	if in.Entity == nil {
		return nil
	}
	for i := range devices {
		if devices[i].ID == in.Entity.DeviceID && in.Entity.DeviceID != "" {
			d := devices[i]
			in.Device = &d
			break
		}
	}
	for i := range entries {
		if entries[i].ID == in.Entity.ConfigEntryID && in.Entity.ConfigEntryID != "" {
			c := entries[i]
			in.ConfigEntry = &c
			break
		}
	}
	return nil
}

func readRepairs(ctx context.Context, reader repairReader, in *analysis.EntityHealthInput) error {
	if reader == nil {
		in.Missing = append(in.Missing, model.MissingEvidence{
			What: "open repairs", Source: "home_assistant_core", Reason: model.MissingUpstreamUnavailable,
			Detail: "no repairs source is configured in this build",
		})
		return nil
	}
	if budget, ok := policy.BudgetFrom(ctx); ok {
		if err := budget.ChargeHARequests(1); err != nil {
			return recordMissing(in, "open repairs", "home_assistant_core", err)
		}
	}
	repairs, err := reader.Repairs(ctx)
	if err != nil {
		return recordMissing(in, "open repairs", "home_assistant_core", err)
	}
	in.RepairsRead, in.Repairs = true, repairs
	return nil
}

func recordMissing(in *analysis.EntityHealthInput, what, source string, err error) error {
	m, abort := missingFor(what, source, err)
	if abort != nil {
		return abort
	}
	in.Missing = append(in.Missing, m)
	return nil
}

// missingFor maps a failed read onto the MissingReason that tells the agent
// what to do next. The detail is fixed analysis text, never the error's own
// string, which may carry upstream payload (CLAUDE.md, Error Handling). The
// second result is non-nil only when the caller itself went away, which no
// partial answer is worth returning for.
func missingFor(what, source string, err error) (model.MissingEvidence, error) {
	m := model.MissingEvidence{What: what, Source: source}
	switch {
	case errors.Is(err, context.Canceled):
		return m, err
	case errors.Is(err, ha.ErrUnsupported):
		m.Reason, m.Detail = model.MissingUnsupported, "this installation or principal does not offer the source"
	case errors.Is(err, policy.ErrBudgetExceeded):
		m.Reason, m.Detail = model.MissingBudgetExceeded, "the invocation's query budget stopped the read"
	case errors.Is(err, policy.ErrPolicyDenied), errors.Is(err, ha.ErrPolicyDenied):
		m.Reason, m.Detail = model.MissingPolicyDenied, "the privacy profile or gateway refused the read"
	case errors.Is(err, ha.ErrDeadline), errors.Is(err, context.DeadlineExceeded):
		m.Reason, m.Detail = model.MissingDeadline, "the source did not answer within the call's deadline"
	default:
		m.Reason, m.Detail = model.MissingUpstreamUnavailable, "the source could not be read"
	}
	return m, nil
}

// renderHealth is the serialized rendering of the analysis (D-05-1).
func renderHealth(a model.HealthAnalysis) HealthResponse {
	hypotheses := make([]HypothesisView, 0, len(a.Hypotheses))
	for _, h := range a.Hypotheses {
		hypotheses = append(hypotheses, HypothesisView{
			Statement:  h.Statement(),
			Confidence: string(h.Confidence()),
			Cites:      h.Cites(),
		})
	}
	return HealthResponse{
		Source:          healthSource,
		ObservedAt:      a.ObservedAt,
		SubjectID:       a.SubjectID,
		From:            a.From,
		To:              a.To,
		Evidence:        nonNil(a.Evidence),
		Hypotheses:      hypotheses,
		MissingEvidence: nonNil(a.MissingEvidence),
		NextActions:     nonNil(a.NextActions),
		Provenance:      a.Provenance,
	}
}

// nonNil keeps an empty list serializing as [] rather than null: "none" and
// "not checked" must stay distinguishable (rule 7).
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
