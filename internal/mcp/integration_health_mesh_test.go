package mcp

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// meshHistoryReader serves a numeric link-quality series for mesh entities and
// a flapping series for everything else, recording what was read.
type meshHistoryReader struct {
	lqi  string
	read []string
}

func (m *meshHistoryReader) History(_ context.Context, id model.EntityID, _, _ time.Time, _ bool) ([]model.HistoryPoint, error) {
	m.read = append(m.read, string(id))
	if strings.HasSuffix(string(id), "_linkquality") {
		return []model.HistoryPoint{{Timestamp: time.Now().Add(-6 * 24 * time.Hour), State: m.lqi}}, nil
	}
	return flapping(40), nil
}

func zigbeeInventory(devices int) *fakeInventoryReader {
	inv := &fakeInventoryReader{integrations: []model.Integration{{ID: "entry-z", Domain: "mqtt", State: "loaded"}}}
	for i := range devices {
		dev := model.DeviceID(fmt.Sprintf("dev%02d", i))
		inv.devices = append(inv.devices, model.DeviceRef{ID: dev, ConfigEntryID: "entry-z"})
		inv.entities = append(inv.entities,
			model.Entity{ID: model.EntityID(fmt.Sprintf("light.bulb_%02d", i)), Domain: "light", ConfigEntryID: "entry-z", DeviceID: dev},
			model.Entity{ID: model.EntityID(fmt.Sprintf("sensor.bulb_%02d_linkquality", i)), Domain: "sensor", ConfigEntryID: "entry-z", DeviceID: dev},
		)
	}
	return inv
}

func TestAnalyzeIntegrationHealth_Mesh_AffectedDevice_EvidenceAndNotExposedRow(t *testing.T) {
	history := &meshHistoryReader{lqi: "87"}
	opts := integrationHealthOptions(history, zigbeeInventory(2), downReader("light.bulb_00"), &fakeRepairReader{}, nil)
	res, out := callAnalyzeIntegrationHealth(t, opts, map[string]any{"config_entry_id": "entry-z"})
	if res.IsError {
		t.Fatalf("error: %s", resultText(res))
	}
	if !slices.ContainsFunc(out.Evidence, func(e model.Evidence) bool { return strings.HasPrefix(string(e.ID), "mesh_link_quality_") }) {
		t.Errorf("no mesh evidence in the response: %+v", out.Evidence)
	}
	if !slices.ContainsFunc(out.MissingEvidence, func(m model.MissingEvidence) bool { return m.Reason == model.MissingNotExposed }) {
		t.Errorf("the absent RSSI is not named: %+v", out.MissingEvidence)
	}
	for _, h := range out.Hypotheses {
		for _, c := range h.Cites {
			if strings.HasPrefix(string(c), "mesh_") {
				t.Errorf("hypothesis %q cites mesh evidence", h.Statement)
			}
		}
	}
}

func TestAnalyzeIntegrationHealth_Mesh_NoProblem_MetricsNotRead(t *testing.T) {
	history := &meshHistoryReader{lqi: "87"}
	inv := zigbeeInventory(2)
	opts := integrationHealthOptions(&stableHistory{inner: history}, inv, downReader(), &fakeRepairReader{}, nil)
	callAnalyzeIntegrationHealth(t, opts, map[string]any{"config_entry_id": "entry-z"})
	// readOutages may read a metric entity as an ordinary entity once; the
	// mesh pass would be a second read of the same id.
	seen := map[string]bool{}
	for _, id := range history.read {
		if seen[id] {
			t.Errorf("%s read twice: the mesh pass ran for an integration with no outage", id)
		}
		seen[id] = true
	}
}

// stableHistory answers every entity with one steady "on" state, so no outage
// exists anywhere.
type stableHistory struct{ inner *meshHistoryReader }

func (s *stableHistory) History(_ context.Context, id model.EntityID, _, _ time.Time, _ bool) ([]model.HistoryPoint, error) {
	s.inner.read = append(s.inner.read, string(id))
	return []model.HistoryPoint{{Timestamp: time.Now().Add(-6 * 24 * time.Hour), State: "on"}}, nil
}

func TestAnalyzeIntegrationHealth_Mesh_ManyDevices_ReadBoundedAndNamed(t *testing.T) {
	history := &meshHistoryReader{lqi: "50"}
	opts := integrationHealthOptions(history, zigbeeInventory(30), downReader("light.bulb_00"), &fakeRepairReader{}, nil)
	_, out := callAnalyzeIntegrationHealth(t, opts, map[string]any{"config_entry_id": "entry-z"})
	if !slices.ContainsFunc(out.MissingEvidence, func(m model.MissingEvidence) bool {
		return m.Reason == model.MissingBudgetExceeded && strings.Contains(m.What, "mesh")
	}) {
		t.Errorf("the unread mesh metrics are not named: %+v", out.MissingEvidence)
	}
	meshReads := 0
	for _, id := range history.read {
		if strings.HasSuffix(id, "_linkquality") {
			meshReads++
		}
	}
	if meshReads != maxMeshMetricEntities {
		t.Errorf("mesh reads = %d, want exactly the cap %d", meshReads, maxMeshMetricEntities)
	}
}
