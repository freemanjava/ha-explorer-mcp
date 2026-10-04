package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// A Supervisor answering a flat body — the shape F-45 mistook for the real
// one — must surface as Unsupported with a reason, never as an empty,
// unmarked answer (CLAUDE.md rule 7). The real SupervisorClient is used, so
// the check covers the envelope handling and the mappers together.
func TestSupervisorFlatBody_HealthAndApps_ReportUnsupported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"homeassistant":"2099.1.0","version":"2099.01.0","addons":[]}`))
	}))
	t.Cleanup(srv.Close)
	supervisor := ha.NewSupervisorClient(srv.URL, "test-token", srv.Client(), nil)

	client := connect(t, newServer(systemOptions(&fakeCoreReader{}, &fakeInventoryReader{}, supervisor), Catalog()))

	var health model.SystemHealth
	callStructured(t, client, "get_system_health", &health)
	if !health.Unsupported || health.UnsupportedReason == "" {
		t.Errorf("get_system_health = %+v, want Unsupported with a reason", health)
	}

	var apps model.AppList
	callStructured(t, client, "list_apps", &apps)
	if !apps.Unsupported || apps.UnsupportedReason == "" {
		t.Errorf("list_apps = %+v, want Unsupported with a reason", apps)
	}
}
