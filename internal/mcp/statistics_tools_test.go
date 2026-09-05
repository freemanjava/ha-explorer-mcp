package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

func statisticsOptions(reader historyReader, profile policy.Profile) Options {
	opts := testOptions()
	opts.History = reader
	opts.Profile = profile
	return opts
}

func callGetEntityStatistics(t *testing.T, opts Options, args map[string]any) (*sdkmcp.CallToolResult, model.Health) {
	t.Helper()
	client := connect(t, newServer(opts, Catalog()))
	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "get_entity_statistics", Arguments: args})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		return res, model.Health{}
	}
	var out model.Health
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return res, out
}

// fixturePoints builds a small, deterministic series a test can reason about
// by hand: 4 points a day apart, with one outage in the middle.
func fixturePoints(base time.Time) []model.HistoryPoint {
	return []model.HistoryPoint{
		{Timestamp: base, State: "on"},
		{Timestamp: base.Add(24 * time.Hour), State: "unavailable"},
		{Timestamp: base.Add(25 * time.Hour), State: "on"},
		{Timestamp: base.Add(48 * time.Hour), State: "off"},
	}
}

// TestGetEntityStatistics_DefaultPeriod_SevenDays pins Appendix A.3's "period
// defaults to 7d" and that the reader is asked for a window ending now.
func TestGetEntityStatistics_DefaultPeriod_SevenDays(t *testing.T) {
	base := time.Now().Add(-6 * 24 * time.Hour)
	reader := &fakeHistoryReader{points: fixturePoints(base)}
	opts := statisticsOptions(reader, policy.Profile{})

	_, out := callGetEntityStatistics(t, opts, map[string]any{"entity_id": "sensor.kitchen"})

	if out.Period != 7*24*time.Hour {
		t.Errorf("Period = %s, want 7d (Appendix A.3 default)", out.Period)
	}
	if reader.lastEntityID != "sensor.kitchen" {
		t.Errorf("reader.lastEntityID = %q", reader.lastEntityID)
	}
	if got := reader.lastTo.Sub(reader.lastFrom); got != 7*24*time.Hour {
		t.Errorf("reader window = %s, want 7d", got)
	}
}

// TestGetEntityStatistics_JoinsAvailabilityAndCadence pins the DoD's "response
// matches the doc §12.1 shape": both halves — availability and cadence — are
// present and computed from the same series.
func TestGetEntityStatistics_JoinsAvailabilityAndCadence(t *testing.T) {
	base := time.Now().Add(-48 * time.Hour)
	reader := &fakeHistoryReader{points: fixturePoints(base)}
	opts := statisticsOptions(reader, policy.Profile{})

	_, out := callGetEntityStatistics(t, opts, map[string]any{
		"entity_id": "sensor.kitchen",
		"period":    "3d",
	})

	if !out.AvailabilityComputable {
		t.Fatal("AvailabilityComputable = false, want true")
	}
	if out.UnavailablePeriods != 1 {
		t.Errorf("UnavailablePeriods = %d, want 1", out.UnavailablePeriods)
	}
	if out.TotalUnavailable != time.Hour {
		t.Errorf("TotalUnavailable = %s, want 1h", out.TotalUnavailable)
	}
	if !out.CadenceComputable {
		t.Fatal("CadenceComputable = false, want true")
	}
	if out.MedianUpdateInterval <= 0 {
		t.Errorf("MedianUpdateInterval = %s, want > 0", out.MedianUpdateInterval)
	}
	if out.StateChanges == 0 {
		t.Error("StateChanges = 0, want > 0")
	}
	if out.Source != "recorder_history" {
		t.Errorf("Source = %q, want %q (DoD: source named in the response)", out.Source, "recorder_history")
	}
}

// TestGetEntityStatistics_PeriodDaysShorthand pins Appendix A.3's "Nd" unit,
// distinct from Go's own duration syntax.
func TestGetEntityStatistics_PeriodDaysShorthand(t *testing.T) {
	base := time.Now().Add(-2 * 24 * time.Hour)
	reader := &fakeHistoryReader{points: fixturePoints(base)}
	opts := statisticsOptions(reader, policy.Profile{})

	_, out := callGetEntityStatistics(t, opts, map[string]any{
		"entity_id": "sensor.kitchen",
		"period":    "2d",
	})

	if out.Period != 2*24*time.Hour {
		t.Errorf("Period = %s, want 2d", out.Period)
	}
}

// TestGetEntityStatistics_PeriodExceedsMaximum_RefusedNamingMaximum is the
// DoD's "period is validated against the maximum".
func TestGetEntityStatistics_PeriodExceedsMaximum_RefusedNamingMaximum(t *testing.T) {
	reader := &fakeHistoryReader{}
	opts := statisticsOptions(reader, policy.Profile{})

	res, _ := callGetEntityStatistics(t, opts, map[string]any{
		"entity_id": "sensor.kitchen",
		"period":    "8d",
	})
	if !res.IsError {
		t.Fatal("expected an error result for a period over the maximum")
	}
	text := resultText(res)
	if !strings.Contains(text, maxHistoryWindow.String()) {
		t.Errorf("error %q does not name the maximum %s", text, maxHistoryWindow)
	}
	if reader.lastEntityID != "" {
		t.Errorf("reader was called despite the oversized period: %+v", reader)
	}
}

// TestGetEntityStatistics_InvalidPeriod_Rejected pins that a malformed period
// string is refused before any reader call, rather than silently defaulting.
func TestGetEntityStatistics_InvalidPeriod_Rejected(t *testing.T) {
	reader := &fakeHistoryReader{}
	opts := statisticsOptions(reader, policy.Profile{})

	res, _ := callGetEntityStatistics(t, opts, map[string]any{
		"entity_id": "sensor.kitchen",
		"period":    "not-a-period",
	})
	if !res.IsError {
		t.Fatal("expected an error result for a malformed period")
	}
	if reader.lastEntityID != "" {
		t.Errorf("reader was called with a malformed period: %+v", reader)
	}
}

// TestGetEntityStatistics_InvalidEntityID_Rejected mirrors
// get_entity_history's own check: HA data and caller ids are untrusted
// (CLAUDE.md rule 6).
func TestGetEntityStatistics_InvalidEntityID_Rejected(t *testing.T) {
	reader := &fakeHistoryReader{}
	opts := statisticsOptions(reader, policy.Profile{})

	res, _ := callGetEntityStatistics(t, opts, map[string]any{"entity_id": "not-an-entity-id"})
	if !res.IsError {
		t.Fatal("expected an error result for a malformed entity id")
	}
	if reader.lastEntityID != "" {
		t.Errorf("reader was called with a malformed entity id: %q", reader.lastEntityID)
	}
}

// TestGetEntityStatistics_PrivateEntity_DenyProfile_Refused mirrors
// get_entity_history's Appendix B case: the deny profile must refuse before
// a recorder read is issued.
func TestGetEntityStatistics_PrivateEntity_DenyProfile_Refused(t *testing.T) {
	reader := &fakeHistoryReader{points: []model.HistoryPoint{{Timestamp: time.Now(), State: "locked"}}}
	opts := statisticsOptions(reader, policy.Profile{Private: policy.HandlingDeny})

	res, _ := callGetEntityStatistics(t, opts, map[string]any{"entity_id": "lock.front_door"})
	if !res.IsError {
		t.Fatal("expected an error result for a private entity under the deny profile")
	}
	if reader.lastEntityID != "" {
		t.Errorf("reader was called despite the deny profile: %+v", reader)
	}
}

// TestGetEntityStatistics_NoPointsObserved_NotComputable is CLAUDE.md rule
// 7's "never fabricate": with nothing recorded, both halves must report
// "could not check" rather than a zero-valued ratio or an instant cadence.
func TestGetEntityStatistics_NoPointsObserved_NotComputable(t *testing.T) {
	reader := &fakeHistoryReader{points: nil}
	opts := statisticsOptions(reader, policy.Profile{})

	_, out := callGetEntityStatistics(t, opts, map[string]any{"entity_id": "sensor.quiet"})

	if out.AvailabilityComputable {
		t.Error("AvailabilityComputable = true, want false with nothing recorded")
	}
	if out.CadenceComputable {
		t.Error("CadenceComputable = true, want false with nothing recorded")
	}
	if out.StaleJudgeable {
		t.Error("StaleJudgeable = true, want false with nothing recorded")
	}
}

// TestGetEntityStatistics_UpstreamDeadline_Propagated mirrors
// get_entity_history's own case: an upstream deadline must surface unchanged.
func TestGetEntityStatistics_UpstreamDeadline_Propagated(t *testing.T) {
	reader := &fakeHistoryReader{err: fmt.Errorf("%w: history/history_during_period", ha.ErrDeadline)}
	opts := statisticsOptions(reader, policy.Profile{})

	res, _ := callGetEntityStatistics(t, opts, map[string]any{"entity_id": "sensor.slow"})
	if !res.IsError {
		t.Fatal("expected an error result for an upstream deadline")
	}
	text := resultText(res)
	if !strings.Contains(text, "deadline") {
		t.Errorf("error %q does not surface the deadline", text)
	}
}
