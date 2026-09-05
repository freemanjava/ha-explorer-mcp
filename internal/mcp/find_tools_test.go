package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// fakeMultiHistoryReader is a historyReader test double keyed by entity id,
// for find_stale_entities' scan over several entities at once — unlike
// fakeHistoryReader, which answers one fixed series regardless of which
// entity was asked for. It records every id it was called for, in order, so
// a test can assert both which entities were examined and in what sequence.
type fakeMultiHistoryReader struct {
	points map[model.EntityID][]model.HistoryPoint
	calls  []model.EntityID
}

func (f *fakeMultiHistoryReader) History(_ context.Context, id model.EntityID, _, _ time.Time, _ bool) ([]model.HistoryPoint, error) {
	f.calls = append(f.calls, id)
	return f.points[id], nil
}

func findOptions(inventory *fakeInventoryReader, avail entityAvailabilityReader, reader historyReader, profile policy.Profile) Options {
	opts := testOptions()
	opts.Inventory = inventory
	opts.Availability = avail
	opts.History = reader
	opts.Profile = profile
	return opts
}

func callFindUnavailableEntities(t *testing.T, opts Options, args map[string]any) (*sdkmcp.CallToolResult, model.UnavailableEntityList) {
	t.Helper()
	client := connect(t, newServer(opts, Catalog()))
	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "find_unavailable_entities", Arguments: args})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		return res, model.UnavailableEntityList{}
	}
	var out model.UnavailableEntityList
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return res, out
}

func callFindStaleEntities(t *testing.T, opts Options, args map[string]any) (*sdkmcp.CallToolResult, model.StaleEntityList) {
	t.Helper()
	client := connect(t, newServer(opts, Catalog()))
	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "find_stale_entities", Arguments: args})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		return res, model.StaleEntityList{}
	}
	var out model.StaleEntityList
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return res, out
}

// hourlyPoints builds n points an hour apart starting at base — a regular
// cadence a real p95 can be computed from.
func hourlyPoints(base time.Time, n int) []model.HistoryPoint {
	points := make([]model.HistoryPoint, n)
	for i := range points {
		points[i] = model.HistoryPoint{Timestamp: base.Add(time.Duration(i) * time.Hour), State: "on"}
	}
	return points
}

// TestFindUnavailableEntities_FiltersAndRanksServerSide is the DoD's "entity
// counts and ranking are computed server-side": only unavailable entities
// matching the domain filter come back, sorted deterministically.
func TestFindUnavailableEntities_FiltersAndRanksServerSide(t *testing.T) {
	inventory := &fakeInventoryReader{entities: []model.Entity{
		{ID: "light.kitchen", Domain: "light"},
		{ID: "light.attic", Domain: "light"},
		{ID: "sensor.hallway", Domain: "sensor"},
	}}
	avail := &fakeAvailabilityReader{unavailable: unavailableSet("light.kitchen", "light.attic", "sensor.hallway")}
	opts := findOptions(inventory, avail, &fakeMultiHistoryReader{}, policy.Profile{})

	_, out := callFindUnavailableEntities(t, opts, map[string]any{"domain": "light"})

	if len(out.Items) != 2 {
		t.Fatalf("Items = %+v, want 2 light entities", out.Items)
	}
	if out.Items[0].ID != "light.attic" || out.Items[1].ID != "light.kitchen" {
		t.Errorf("Items not sorted by id: %+v", out.Items)
	}
}

// TestFindUnavailableEntities_AvailableEntity_Excluded pins that an entity
// merely matching the filter, but not unavailable, never appears.
func TestFindUnavailableEntities_AvailableEntity_Excluded(t *testing.T) {
	inventory := &fakeInventoryReader{entities: []model.Entity{
		{ID: "light.kitchen", Domain: "light"},
	}}
	avail := &fakeAvailabilityReader{unavailable: unavailableSet()}
	opts := findOptions(inventory, avail, &fakeMultiHistoryReader{}, policy.Profile{})

	_, out := callFindUnavailableEntities(t, opts, nil)
	if len(out.Items) != 0 {
		t.Fatalf("Items = %+v, want none (nothing unavailable)", out.Items)
	}
}

// TestFindUnavailableEntities_PrivateEntity_DenyProfile_ExcludedAndCounted is
// the DoD's "a PRIVATE entity is included or excluded per the Phase 02
// profile, and the response says which happened": under deny, the private
// entity is dropped from Items and counted, not silently absent.
func TestFindUnavailableEntities_PrivateEntity_DenyProfile_ExcludedAndCounted(t *testing.T) {
	inventory := &fakeInventoryReader{entities: []model.Entity{
		{ID: "person.dmitry", Domain: "person"},
		{ID: "sensor.hallway", Domain: "sensor"},
	}}
	avail := &fakeAvailabilityReader{unavailable: unavailableSet("person.dmitry", "sensor.hallway")}
	opts := findOptions(inventory, avail, &fakeMultiHistoryReader{}, policy.Profile{Private: policy.HandlingDeny})

	_, out := callFindUnavailableEntities(t, opts, nil)

	if len(out.Items) != 1 || out.Items[0].ID != "sensor.hallway" {
		t.Fatalf("Items = %+v, want only sensor.hallway (person.dmitry denied)", out.Items)
	}
	if out.PrivateExcluded != 1 {
		t.Errorf("PrivateExcluded = %d, want 1", out.PrivateExcluded)
	}
}

// TestFindUnavailableEntities_PrivateEntity_MaskProfile_Included pins that
// the default (mask) profile keeps a PRIVATE entity's availability finding —
// HandlingMask's own purpose is that flapping/availability analysis keeps
// working while the entity's state values do not survive.
func TestFindUnavailableEntities_PrivateEntity_MaskProfile_Included(t *testing.T) {
	inventory := &fakeInventoryReader{entities: []model.Entity{
		{ID: "person.dmitry", Domain: "person"},
	}}
	avail := &fakeAvailabilityReader{unavailable: unavailableSet("person.dmitry")}
	opts := findOptions(inventory, avail, &fakeMultiHistoryReader{}, policy.Profile{})

	_, out := callFindUnavailableEntities(t, opts, nil)

	if len(out.Items) != 1 || out.Items[0].ID != "person.dmitry" {
		t.Fatalf("Items = %+v, want person.dmitry included under the mask profile", out.Items)
	}
	if out.PrivateExcluded != 0 {
		t.Errorf("PrivateExcluded = %d, want 0", out.PrivateExcluded)
	}
}

// staleFixture returns two entities' history over a 2-day window ending now:
// one silent since well past its own 3xP95 threshold, the other updating
// right up to the window's end.
func staleFixture(t *testing.T) (*fakeInventoryReader, *fakeMultiHistoryReader) {
	t.Helper()
	now := time.Now().UTC()
	windowStart := now.Add(-48 * time.Hour)

	stale := hourlyPoints(windowStart, 24) // last update at windowStart+23h: ~25h of silence after, p95 ~1h
	fresh := hourlyPoints(windowStart, 47) // last update ~1h before now

	inventory := &fakeInventoryReader{entities: []model.Entity{
		{ID: "sensor.stale", Domain: "sensor"},
		{ID: "sensor.fresh", Domain: "sensor"},
	}}
	reader := &fakeMultiHistoryReader{points: map[model.EntityID][]model.HistoryPoint{
		"sensor.stale": stale,
		"sensor.fresh": fresh,
	}}
	return inventory, reader
}

// TestFindStaleEntities_JudgesAgainstOwnCadence pins P4-03's per-entity
// relative judgement reaching the tool: the silent entity is reported stale
// with its evidence, the actively-updating one is not returned at all.
func TestFindStaleEntities_JudgesAgainstOwnCadence(t *testing.T) {
	inventory, reader := staleFixture(t)
	opts := findOptions(inventory, &fakeAvailabilityReader{}, reader, policy.Profile{})

	_, out := callFindStaleEntities(t, opts, map[string]any{"period": "2d"})

	if len(out.Items) != 1 || out.Items[0].ID != "sensor.stale" {
		t.Fatalf("Items = %+v, want only sensor.stale", out.Items)
	}
	if out.Items[0].StalenessRatio <= 1 {
		t.Errorf("StalenessRatio = %v, want > 1 (silent well past its threshold)", out.Items[0].StalenessRatio)
	}
	if out.Scanned != 2 {
		t.Errorf("Scanned = %d, want 2 (both candidates examined)", out.Scanned)
	}
}

// TestFindStaleEntities_RanksByStalenessRatioDescending is the DoD's
// "ranking are computed server-side": the entity silent for more of its own
// tail intervals sorts first.
func TestFindStaleEntities_RanksByStalenessRatioDescending(t *testing.T) {
	now := time.Now().UTC()
	windowStart := now.Add(-48 * time.Hour)
	// Both silent past their threshold, but "sensor.very_stale" for longer
	// relative to its own (identical) cadence.
	worseStale := hourlyPoints(windowStart, 10)  // silent since windowStart+9h
	mildlyStale := hourlyPoints(windowStart, 20) // silent since windowStart+19h

	inventory := &fakeInventoryReader{entities: []model.Entity{
		{ID: "sensor.mildly_stale", Domain: "sensor"},
		{ID: "sensor.very_stale", Domain: "sensor"},
	}}
	reader := &fakeMultiHistoryReader{points: map[model.EntityID][]model.HistoryPoint{
		"sensor.very_stale":   worseStale,
		"sensor.mildly_stale": mildlyStale,
	}}
	opts := findOptions(inventory, &fakeAvailabilityReader{}, reader, policy.Profile{})

	_, out := callFindStaleEntities(t, opts, map[string]any{"period": "2d"})

	if len(out.Items) != 2 {
		t.Fatalf("Items = %+v, want both entities stale", out.Items)
	}
	if out.Items[0].ID != "sensor.very_stale" {
		t.Errorf("Items[0] = %q, want sensor.very_stale ranked first (higher StalenessRatio)", out.Items[0].ID)
	}
	if out.Items[0].StalenessRatio <= out.Items[1].StalenessRatio {
		t.Errorf("ranking not descending: %v then %v", out.Items[0].StalenessRatio, out.Items[1].StalenessRatio)
	}
}

// TestFindStaleEntities_PrivateEntity_DenyProfile_ExcludedWithoutReading pins
// that a denied PRIVATE candidate is skipped before spending a recorder
// read, and counted rather than silently dropped.
func TestFindStaleEntities_PrivateEntity_DenyProfile_ExcludedWithoutReading(t *testing.T) {
	inventory := &fakeInventoryReader{entities: []model.Entity{
		{ID: "lock.front_door", Domain: "lock"},
		{ID: "sensor.hallway", Domain: "sensor"},
	}}
	reader := &fakeMultiHistoryReader{points: map[model.EntityID][]model.HistoryPoint{}}
	opts := findOptions(inventory, &fakeAvailabilityReader{}, reader, policy.Profile{Private: policy.HandlingDeny})

	_, out := callFindStaleEntities(t, opts, map[string]any{"period": "2d"})

	if out.PrivateExcluded != 1 {
		t.Errorf("PrivateExcluded = %d, want 1", out.PrivateExcluded)
	}
	for _, id := range reader.calls {
		if id == "lock.front_door" {
			t.Fatalf("reader was called for a denied private entity: %v", reader.calls)
		}
	}
}

// TestFindStaleEntities_EntityBudgetTruncates_NotArbitrarySubset is the DoD's
// "both respect the entity budget and return an explicit truncated marker
// rather than an arbitrary subset presented as complete": judging cadence
// costs one recorder read per entity, and the normal-read class allows only
// policy.LimitsFor(ClassNormalRead).MaxHARequests of them per call.
func TestFindStaleEntities_EntityBudgetTruncates_NotArbitrarySubset(t *testing.T) {
	maxRequests := policy.LimitsFor(policy.ClassNormalRead).MaxHARequests
	total := maxRequests + 5

	entities := make([]model.Entity, total)
	points := make(map[model.EntityID][]model.HistoryPoint, total)
	for i := range entities {
		id := model.EntityID("sensor.e" + string(rune('a'+i)))
		entities[i] = model.Entity{ID: id, Domain: "sensor"}
		points[id] = nil // not computable either way; only the scan boundary is under test
	}
	inventory := &fakeInventoryReader{entities: entities}
	reader := &fakeMultiHistoryReader{points: points}
	opts := findOptions(inventory, &fakeAvailabilityReader{}, reader, policy.Profile{})

	_, out := callFindStaleEntities(t, opts, map[string]any{"limit": 200})

	if out.Scanned != maxRequests {
		t.Fatalf("Scanned = %d, want %d (the HA-request budget, not the candidate count)", out.Scanned, maxRequests)
	}
	if !out.Truncated {
		t.Fatal("Truncated = false, want true: candidates remain unexamined")
	}
	if out.NextCursor == "" {
		t.Fatal("NextCursor is empty despite Truncated = true")
	}
	if len(reader.calls) != maxRequests {
		t.Errorf("reader was called %d times, want %d (no wasted reads past the budget)", len(reader.calls), maxRequests)
	}

	// Resuming from the cursor picks up exactly where the first call left
	// off: no entity skipped, none re-examined.
	_, second := callFindStaleEntities(t, opts, map[string]any{"limit": 200, "cursor": out.NextCursor})
	if second.Scanned != total-maxRequests {
		t.Errorf("second Scanned = %d, want %d (the remainder)", second.Scanned, total-maxRequests)
	}
	if second.Truncated {
		t.Error("second Truncated = true, want false: nothing left to scan")
	}
}

// TestFindStaleEntities_PeriodExceedsMaximum_RefusedNamingMaximum mirrors
// get_entity_statistics' own case: the same maxHistoryWindow cap applies.
func TestFindStaleEntities_PeriodExceedsMaximum_RefusedNamingMaximum(t *testing.T) {
	inventory := &fakeInventoryReader{}
	opts := findOptions(inventory, &fakeAvailabilityReader{}, &fakeMultiHistoryReader{}, policy.Profile{})

	res, _ := callFindStaleEntities(t, opts, map[string]any{"period": "8d"})
	if !res.IsError {
		t.Fatal("expected an error result for a period over the maximum")
	}
}
