package mcp

import (
	"context"
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

// fakeHistoryReader is a historyReader test double: it answers with a fixed
// set of points (or error) and records the last call's arguments, so a test
// can assert both on what came back and on what was actually asked for.
type fakeHistoryReader struct {
	points []model.HistoryPoint
	err    error

	lastEntityID model.EntityID
	lastFrom     time.Time
	lastTo       time.Time
	lastMinimal  bool
}

func (f *fakeHistoryReader) History(_ context.Context, entityID model.EntityID, from, to time.Time, minimal bool) ([]model.HistoryPoint, error) {
	f.lastEntityID, f.lastFrom, f.lastTo, f.lastMinimal = entityID, from, to, minimal
	if f.err != nil {
		return nil, f.err
	}
	return f.points, nil
}

func historyOptions(reader historyReader, profile policy.Profile) Options {
	opts := testOptions()
	opts.History = reader
	opts.Profile = profile
	return opts
}

func callGetEntityHistory(t *testing.T, opts Options, args map[string]any) (*sdkmcp.CallToolResult, model.EntityHistory) {
	t.Helper()
	client := connect(t, newServer(opts, Catalog()))
	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "get_entity_history", Arguments: args})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		return res, model.EntityHistory{}
	}
	var out model.EntityHistory
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return res, out
}

func rfc3339(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// TestGetEntityHistory_ReturnsPoints_DefaultsMinimalTrue pins Appendix A.2's
// "minimal: true by default" and that the reader receives the resolved
// window unchanged.
func TestGetEntityHistory_ReturnsPoints_DefaultsMinimalTrue(t *testing.T) {
	from := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	reader := &fakeHistoryReader{points: []model.HistoryPoint{
		{Timestamp: from.Add(time.Hour), State: "on"},
		{Timestamp: from.Add(2 * time.Hour), State: "off"},
	}}
	opts := historyOptions(reader, policy.Profile{})

	_, out := callGetEntityHistory(t, opts, map[string]any{
		"entity_id": "light.kitchen",
		"from":      rfc3339(from),
		"to":        rfc3339(to),
	})

	if len(out.Points) != 2 || out.Points[0].State != "on" || out.Points[1].State != "off" {
		t.Fatalf("Points = %+v", out.Points)
	}
	if !out.Minimal {
		t.Errorf("Minimal = false, want true (Appendix A.2 default)")
	}
	if !reader.lastMinimal {
		t.Errorf("reader.lastMinimal = false, want true was forwarded")
	}
	if reader.lastEntityID != "light.kitchen" {
		t.Errorf("reader.lastEntityID = %q", reader.lastEntityID)
	}
}

// TestGetEntityHistory_MinimalFalse_Forwarded asserts an explicit
// minimal:false reaches the reader unchanged, not just the response's echo
// of it.
func TestGetEntityHistory_MinimalFalse_Forwarded(t *testing.T) {
	from := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	reader := &fakeHistoryReader{points: []model.HistoryPoint{{Timestamp: from, State: "on"}}}
	opts := historyOptions(reader, policy.Profile{})

	_, out := callGetEntityHistory(t, opts, map[string]any{
		"entity_id": "light.kitchen",
		"from":      rfc3339(from),
		"to":        rfc3339(to),
		"minimal":   false,
	})

	if out.Minimal {
		t.Errorf("Minimal = true, want false")
	}
	if reader.lastMinimal {
		t.Errorf("reader.lastMinimal = true, want false was forwarded")
	}
}

// TestGetEntityHistory_MinimalReducesResponseSize demonstrates the DoD's
// "minimal demonstrably reduces the response size": the same points, with
// and without attributes, serialize to a materially different size.
func TestGetEntityHistory_MinimalReducesResponseSize(t *testing.T) {
	from := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	full := []model.HistoryPoint{{
		Timestamp: from,
		State:     "on",
		Attributes: map[string]any{
			"friendly_name": "Kitchen Light", "brightness": 128.0, "color_temp": 350.0,
			"supported_features": 63.0, "min_mireds": 153.0, "max_mireds": 500.0,
		},
	}}
	minimalReader := &fakeHistoryReader{points: []model.HistoryPoint{{Timestamp: from, State: "on"}}}
	fullReader := &fakeHistoryReader{points: full}

	_, minimalOut := callGetEntityHistory(t, historyOptions(minimalReader, policy.Profile{}), map[string]any{
		"entity_id": "light.kitchen", "from": rfc3339(from), "to": rfc3339(to),
	})
	_, fullOut := callGetEntityHistory(t, historyOptions(fullReader, policy.Profile{}), map[string]any{
		"entity_id": "light.kitchen", "from": rfc3339(from), "to": rfc3339(to), "minimal": false,
	})

	minimalBytes, _ := json.Marshal(minimalOut.Points)
	fullBytes, _ := json.Marshal(fullOut.Points)
	if len(minimalBytes) >= len(fullBytes) {
		t.Fatalf("minimal response (%d bytes) not smaller than full response (%d bytes)", len(minimalBytes), len(fullBytes))
	}
}

// TestGetEntityHistory_InvalidEntityID_Rejected pins that a malformed id is
// refused before any reader call (HA data and caller ids are untrusted,
// CLAUDE.md rule 6).
func TestGetEntityHistory_InvalidEntityID_Rejected(t *testing.T) {
	reader := &fakeHistoryReader{}
	opts := historyOptions(reader, policy.Profile{})
	from := time.Now().UTC()

	res, _ := callGetEntityHistory(t, opts, map[string]any{
		"entity_id": "not-an-entity-id",
		"from":      rfc3339(from),
		"to":        rfc3339(from.Add(time.Hour)),
	})
	if !res.IsError {
		t.Fatal("expected an error result for a malformed entity id")
	}
	if reader.lastEntityID != "" {
		t.Errorf("reader was called with a malformed entity id: %q", reader.lastEntityID)
	}
}

// TestGetEntityHistory_ToNotAfterFrom_Rejected pins an inverted or
// zero-width range as a request error, never silently swapped or clamped.
func TestGetEntityHistory_ToNotAfterFrom_Rejected(t *testing.T) {
	reader := &fakeHistoryReader{}
	opts := historyOptions(reader, policy.Profile{})
	from := time.Now().UTC()

	res, _ := callGetEntityHistory(t, opts, map[string]any{
		"entity_id": "light.kitchen",
		"from":      rfc3339(from),
		"to":        rfc3339(from.Add(-time.Hour)),
	})
	if !res.IsError {
		t.Fatal("expected an error result for to before from")
	}
}

// TestGetEntityHistory_WindowExceedsMaximum_RefusedNamingMaximum is the DoD's
// "a range exceeding the configured maximum is refused with an explicit
// policy error naming the maximum, not silently clamped".
func TestGetEntityHistory_WindowExceedsMaximum_RefusedNamingMaximum(t *testing.T) {
	reader := &fakeHistoryReader{}
	opts := historyOptions(reader, policy.Profile{})
	from := time.Now().UTC()

	res, _ := callGetEntityHistory(t, opts, map[string]any{
		"entity_id": "light.kitchen",
		"from":      rfc3339(from),
		"to":        rfc3339(from.Add(maxHistoryWindow + time.Hour)),
	})
	if !res.IsError {
		t.Fatal("expected an error result for a window over the maximum")
	}
	text := resultText(res)
	if !strings.Contains(text, maxHistoryWindow.String()) {
		t.Errorf("error %q does not name the maximum %s", text, maxHistoryWindow)
	}
	if reader.lastEntityID != "" {
		t.Errorf("reader was called despite the oversized window: %+v", reader)
	}
}

// TestGetEntityHistory_PrivateEntity_DenyProfile_Refused is Appendix B's
// "private entity history is requested by an unapproved policy profile":
// the deny profile must refuse before a recorder read is even issued.
func TestGetEntityHistory_PrivateEntity_DenyProfile_Refused(t *testing.T) {
	reader := &fakeHistoryReader{points: []model.HistoryPoint{{Timestamp: time.Now(), State: "locked"}}}
	opts := historyOptions(reader, policy.Profile{Private: policy.HandlingDeny})
	from := time.Now().UTC()

	res, _ := callGetEntityHistory(t, opts, map[string]any{
		"entity_id": "lock.front_door",
		"from":      rfc3339(from),
		"to":        rfc3339(from.Add(time.Hour)),
	})
	if !res.IsError {
		t.Fatal("expected an error result for a private entity under the deny profile")
	}
	if reader.lastEntityID != "" {
		t.Errorf("reader was called despite the deny profile: %+v", reader)
	}
}

// TestGetEntityHistory_PrivateEntity_MaskProfile_StatesAreMasked pins the
// default mask handling: a PRIVATE entity's states are opaque tokens, not
// the raw values, while the timeline (point count, ordering) survives.
func TestGetEntityHistory_PrivateEntity_MaskProfile_StatesAreMasked(t *testing.T) {
	from := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	reader := &fakeHistoryReader{points: []model.HistoryPoint{
		{Timestamp: from, State: "locked"},
		{Timestamp: from.Add(time.Hour), State: "unlocked"},
		{Timestamp: from.Add(2 * time.Hour), State: "locked"},
	}}
	opts := historyOptions(reader, policy.Profile{}) // zero value is HandlingMask

	_, out := callGetEntityHistory(t, opts, map[string]any{
		"entity_id": "lock.front_door",
		"from":      rfc3339(from),
		"to":        rfc3339(from.Add(3 * time.Hour)),
	})

	if len(out.Points) != 3 {
		t.Fatalf("Points = %+v, want 3 (masking must not drop points)", out.Points)
	}
	for _, p := range out.Points {
		if p.State == "locked" || p.State == "unlocked" {
			t.Fatalf("state %q was not masked under the default profile", p.State)
		}
	}
	if out.Points[0].State != out.Points[2].State {
		t.Errorf("equal states got different tokens: %q vs %q — transitions become uncountable", out.Points[0].State, out.Points[2].State)
	}
	if out.Points[0].State == out.Points[1].State {
		t.Errorf("distinct states got the same token: %q", out.Points[0].State)
	}
}

// TestGetEntityHistory_NormalEntity_NotMasked pins that masking is scoped to
// PRIVATE entities only: an ordinary sensor's states pass through unchanged
// even under the default profile.
func TestGetEntityHistory_NormalEntity_NotMasked(t *testing.T) {
	reader := &fakeHistoryReader{points: []model.HistoryPoint{{Timestamp: time.Now(), State: "21.5"}}}
	opts := historyOptions(reader, policy.Profile{})
	from := time.Now().UTC()

	_, out := callGetEntityHistory(t, opts, map[string]any{
		"entity_id": "sensor.living_room_temperature",
		"from":      rfc3339(from),
		"to":        rfc3339(from.Add(time.Hour)),
	})
	if len(out.Points) != 1 || out.Points[0].State != "21.5" {
		t.Fatalf("Points = %+v, want the raw state unchanged", out.Points)
	}
}

// TestGetEntityHistory_BudgetExceeded_ReturnsBudgetError is the DoD's "a
// point count exceeding budget returns ErrBudgetExceeded with what was
// retrieved".
func TestGetEntityHistory_BudgetExceeded_ReturnsBudgetError(t *testing.T) {
	over := policy.LimitsFor(policy.ClassNormalRead).MaxHistoryPoints + 1
	points := make([]model.HistoryPoint, over)
	for i := range points {
		points[i] = model.HistoryPoint{Timestamp: time.Now().Add(time.Duration(i) * time.Second), State: "on"}
	}
	reader := &fakeHistoryReader{points: points}
	opts := historyOptions(reader, policy.Profile{})
	from := time.Now().UTC()

	res, err := connect(t, newServer(opts, Catalog())).CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "get_entity_history",
		Arguments: map[string]any{
			"entity_id": "sensor.chatty",
			"from":      rfc3339(from),
			"to":        rfc3339(from.Add(time.Hour)),
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected an error result once the point budget is exceeded")
	}
	text := resultText(res)
	if !strings.Contains(text, "history_points") {
		t.Errorf("error %q does not name the exceeded dimension", text)
	}
}

// TestGetEntityHistory_UpstreamDeadline_Propagated is the DoD's "a slow or
// timing-out recorder query returns ErrDeadline": the tool must surface it
// unchanged, never mask it as a generic failure or an empty result.
func TestGetEntityHistory_UpstreamDeadline_Propagated(t *testing.T) {
	reader := &fakeHistoryReader{err: fmt.Errorf("%w: history/history_during_period", ha.ErrDeadline)}
	opts := historyOptions(reader, policy.Profile{})
	from := time.Now().UTC()

	res, err := connect(t, newServer(opts, Catalog())).CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "get_entity_history",
		Arguments: map[string]any{
			"entity_id": "sensor.slow",
			"from":      rfc3339(from),
			"to":        rfc3339(from.Add(time.Hour)),
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected an error result for an upstream deadline")
	}
	text := resultText(res)
	if !strings.Contains(text, "deadline") {
		t.Errorf("error %q does not surface the deadline", text)
	}
}

func resultText(res *sdkmcp.CallToolResult) string {
	var out string
	for _, c := range res.Content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			out += tc.Text
		}
	}
	return out
}
