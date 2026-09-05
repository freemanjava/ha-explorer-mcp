package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/analysis"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// defaultStatisticsPeriod is get_entity_statistics' period when the caller
// omits it (Appendix A.3: "period?: duration = \"7d\"").
const defaultStatisticsPeriod = "7d"

// statisticsPeriodDaysPattern accepts Appendix A.3's "Nd" day shorthand — the
// only unit doc §12.1's example and default use. Anything else is tried
// against Go's own time.ParseDuration (h/m/s), so "24h" works too.
var statisticsPeriodDaysPattern = regexp.MustCompile(`^([0-9]+)d$`)

// GetEntityStatisticsInput is get_entity_statistics's typed input: one entity
// id and a bounded lookback period ending now — no field accepts a route,
// command or query (rule 2).
type GetEntityStatisticsInput struct {
	EntityID string `json:"entity_id" jsonschema:"the entity id to compute statistics for"`
	// Period bounds the lookback window ending now, e.g. "7d" or "24h".
	// Defaults to 7d (Appendix A.3) and is refused above maxHistoryWindow,
	// the same cap get_entity_history enforces (P4-01's decision record: one
	// range cap for both tools, not two to keep in sync).
	Period *string `json:"period,omitempty" jsonschema:"bounded lookback window ending now, e.g. \"7d\" or \"24h\" (default 7d, max 7d)"`
}

// period returns the input's effective period string, "7d" by default
// (Appendix A.3).
func (in GetEntityStatisticsInput) period() string {
	if in.Period == nil || *in.Period == "" {
		return defaultStatisticsPeriod
	}
	return *in.Period
}

// parseStatisticsPeriod turns a period string into a duration, accepting
// Appendix A.3's "Nd" day shorthand alongside Go's standard duration units.
func parseStatisticsPeriod(s string) (time.Duration, error) {
	if m := statisticsPeriodDaysPattern.FindStringSubmatch(s); m != nil {
		days, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, fmt.Errorf("get_entity_statistics: %q is not a valid period", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("get_entity_statistics: %q is not a valid period", s)
	}
	return d, nil
}

// withStatisticsTools returns tools with get_entity_statistics's handler
// bound, when opts supplies a reader for it. It reuses get_entity_history's
// historyReader (P0-07's chosen source) — the two tools differ only in what
// they do with the same recorder read, not in where it comes from.
func withStatisticsTools(tools []Tool, opts Options) []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	if opts.History == nil {
		return out
	}
	for i := range out {
		if out[i].Name == "get_entity_statistics" {
			out[i].bind = bindGetEntityStatistics(opts.History, opts.Profile)
		}
	}
	return out
}

// bindGetEntityStatistics registers get_entity_statistics's typed handler.
func bindGetEntityStatistics(reader historyReader, profile policy.Profile) binder {
	return func(srv *sdkmcp.Server, def *sdkmcp.Tool) {
		sdkmcp.AddTool(srv, def, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in GetEntityStatisticsInput) (*sdkmcp.CallToolResult, model.Health, error) {
			out, err := getEntityStatistics(ctx, reader, profile, in)
			return nil, out, err
		})
	}
}

// getEntityStatistics validates the request, refuses what the range cap and
// the privacy profile will not serve, reads the recorder-backed source once,
// and joins P4-02's availability analysis with P4-03's cadence analysis into
// the one response doc §12.1 describes.
//
// Refusal order mirrors get_entity_history: shape and range before the
// profile, the profile before the query, so a malformed request or a denied
// one never spends a recorder read (phase 02 design notes).
func getEntityStatistics(ctx context.Context, reader historyReader, profile policy.Profile, in GetEntityStatisticsInput) (model.Health, error) {
	if !historyEntityIDPattern.MatchString(in.EntityID) {
		return model.Health{}, fmt.Errorf("get_entity_statistics: %q is not a valid entity id", in.EntityID)
	}
	entityID := model.EntityID(in.EntityID)

	periodStr := in.period()
	window, err := parseStatisticsPeriod(periodStr)
	if err != nil {
		return model.Health{}, err
	}
	if window <= 0 {
		return model.Health{}, fmt.Errorf("get_entity_statistics: period %q must be positive", periodStr)
	}
	if window > maxHistoryWindow {
		return model.Health{}, fmt.Errorf("%w: get_entity_statistics: requested period %s exceeds the maximum %s",
			policy.ErrPolicyDenied, window, maxHistoryWindow)
	}

	if err := profile.CheckHistoryScope(policy.HistoryScope{Entities: []model.EntityID{entityID}}); err != nil {
		return model.Health{}, err
	}

	if budget, ok := policy.BudgetFrom(ctx); ok {
		if err := budget.Preflight(policy.SourceHistory, 1, window); err != nil {
			return model.Health{}, err
		}
	}

	to := time.Now().UTC()
	from := to.Add(-window)
	// Statistics are computed aggregates, never raw values: attributes are
	// never needed, so the read always asks for the minimal shape regardless
	// of what get_entity_history's own caller might choose.
	points, err := reader.History(ctx, entityID, from, to, true)
	if err != nil {
		return model.Health{}, err
	}

	if budget, ok := policy.BudgetFrom(ctx); ok {
		if err := budget.ChargeHARequests(1); err != nil {
			return model.Health{}, err
		}
		if err := budget.ChargeHistoryPoints(len(points)); err != nil {
			return model.Health{}, err
		}
	}

	avail, err := analysis.ComputeAvailability(from, to, points)
	if err != nil {
		return model.Health{}, err
	}
	cadence, err := analysis.ComputeCadence(from, to, points)
	if err != nil {
		return model.Health{}, err
	}

	out := model.Health{
		Source:     "recorder_history",
		ObservedAt: to,
		SubjectID:  string(entityID),
		Period:     window,
		From:       from,
		To:         to,

		AvailabilityComputable: avail.Computable,
		AvailabilityRatio:      avail.AvailabilityRatio,
		StateChanges:           avail.StateChanges,
		UnavailablePeriods:     avail.UnavailablePeriods,
		TotalUnavailable:       avail.TotalUnavailable,
		LongestUnavailable:     avail.LongestUnavailable,

		CadenceComputable:    cadence.Computable,
		MedianUpdateInterval: cadence.MedianUpdateInterval,
		P95UpdateInterval:    cadence.P95UpdateInterval,
		StaleJudgeable:       cadence.StaleJudgeable,
		Stale:                cadence.Stale,
	}

	if budget, ok := policy.BudgetFrom(ctx); ok {
		if err := budget.ChargeBytes(healthByteSize(out)); err != nil {
			return model.Health{}, err
		}
	}

	return out, nil
}

// healthByteSize approximates a Health response's serialized size for the
// byte dimension of the query budget.
func healthByteSize(h model.Health) int64 {
	b, err := json.Marshal(h)
	if err != nil {
		return 0
	}
	return int64(len(b))
}
