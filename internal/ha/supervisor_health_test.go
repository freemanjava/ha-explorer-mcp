package ha

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
)

// jsonServer answers every request with body, regardless of path — good
// enough for these tests, which each exercise exactly one SupervisorClient
// method against its own client.
func jsonServer(t *testing.T, body string) (client *SupervisorClient) {
	t.Helper()
	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	return NewSupervisorClient(srv.URL, testToken, srv.Client(), nil)
}

// envelope wraps data in the {"result","data"} body every Supervisor route
// answers with (D-08-13). No test body may be flat.
func envelope(data string) string {
	return `{"result":"ok","data":` + data + `}`
}

func TestSupervisorClient_CoreInfo_MapsFields(t *testing.T) {
	c := jsonServer(t, envelope(`{"supervisor":"2026.08.0","homeassistant":"2026.8.3","hassos":"14.2","hostname":"homeassistant","machine":"rpi4","arch":"aarch64","state":"running","supported":true}`))

	info, err := c.CoreInfo(testCtx(t))
	if err != nil {
		t.Fatalf("CoreInfo: %v", err)
	}
	if info.CoreVersion != "2026.8.3" || info.SupervisorVersion != "2026.08.0" || info.OSVersion != "14.2" {
		t.Fatalf("CoreInfo mapped %+v unexpectedly", info)
	}
}

func TestSupervisorClient_CoreInfo_Unreachable_ReturnsUnsupported(t *testing.T) {
	c := NewSupervisorClient("http://127.0.0.1:1", testToken, http.DefaultClient, nil)
	if _, err := c.CoreInfo(testCtx(t)); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("CoreInfo: got %v, want ErrUnsupported", err)
	}
}

func TestSupervisorClient_OSHealth_MapsFields(t *testing.T) {
	c := jsonServer(t, envelope(`{"version":"14.2","update_available":true}`))

	os, err := c.OSHealth(testCtx(t))
	if err != nil {
		t.Fatalf("OSHealth: %v", err)
	}
	if os.Version != "14.2" || !os.UpdateAvailable {
		t.Fatalf("OSHealth mapped %+v unexpectedly", os)
	}
}

func TestSupervisorClient_HostDisk_MapsFields(t *testing.T) {
	c := jsonServer(t, envelope(`{"disk_free":10.5,"disk_total":32,"disk_used":21.5}`))

	disk, err := c.HostDisk(testCtx(t))
	if err != nil {
		t.Fatalf("HostDisk: %v", err)
	}
	if disk.FreeGB != 10.5 || disk.TotalGB != 32 || disk.UsedGB != 21.5 {
		t.Fatalf("HostDisk mapped %+v unexpectedly", disk)
	}
}

func TestSupervisorClient_ResolutionSummary_MapsFields(t *testing.T) {
	c := jsonServer(t, envelope(`{"unhealthy":["privileged"],"unsupported":[],"issues":[{"uuid":"1","type":"free_space"}]}`))

	summary, err := c.ResolutionSummary(testCtx(t))
	if err != nil {
		t.Fatalf("ResolutionSummary: %v", err)
	}
	if summary.IssueCount != 1 || len(summary.Unhealthy) != 1 {
		t.Fatalf("ResolutionSummary mapped %+v unexpectedly", summary)
	}
}

func TestSupervisorClient_SelfStats_MapsFields(t *testing.T) {
	c := jsonServer(t, envelope(`{"cpu_percent":1.5,"memory_percent":4.2}`))

	stats, err := c.SelfStats(testCtx(t))
	if err != nil {
		t.Fatalf("SelfStats: %v", err)
	}
	if stats.CPUPercent != 1.5 || stats.MemoryPercent != 4.2 {
		t.Fatalf("SelfStats mapped %+v unexpectedly", stats)
	}
}

// TestSupervisorHealthMethods_TokenNeverReturned extends the phase 01 DoD line
// (CLAUDE.md rule 4) to the five typed methods P3-02 adds: SUPERVISOR_TOKEN
// must not appear in any error they can produce.
func TestSupervisorHealthMethods_TokenNeverReturned(t *testing.T) {
	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	c := NewSupervisorClient(srv.URL, testToken, srv.Client(), nil)
	ctx := testCtx(t)

	_, coreInfoErr := c.CoreInfo(ctx)
	_, osErr := c.OSHealth(ctx)
	_, diskErr := c.HostDisk(ctx)
	_, resErr := c.ResolutionSummary(ctx)
	_, statsErr := c.SelfStats(ctx)

	for name, err := range map[string]error{
		"CoreInfo": coreInfoErr, "OSHealth": osErr, "HostDisk": diskErr,
		"ResolutionSummary": resErr, "SelfStats": statsErr,
	} {
		if err == nil {
			t.Fatalf("%s: expected an error to inspect", name)
		}
		if strings.Contains(err.Error(), testToken) {
			t.Fatalf("%s: error string carries the token: %q", name, err)
		}
	}
}

// The fixture carries exactly the data keys observed on the Pi, with invented
// values (docs/research/2026-10-04-supervisor-response-shape.md).
func TestSupervisorClient_CoreInfo_ObservedFixture_MapsNonEmpty(t *testing.T) {
	body, err := os.ReadFile("../../test/fixtures/supervisor_info.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	info, err := jsonServer(t, string(body)).CoreInfo(testCtx(t))
	if err != nil {
		t.Fatalf("CoreInfo: %v", err)
	}
	if info.CoreVersion == "" || info.Hostname == "" || info.Arch == "" {
		t.Fatalf("CoreInfo mapped %+v, want CoreVersion, Hostname and Arch non-empty", info)
	}
}

// D-08-13: a body that is not an ok envelope with a data object is an error,
// and Supervisor's own message text never reaches it (CLAUDE.md rule 6).
func TestSupervisorClient_NotAnOkEnvelope_Errors(t *testing.T) {
	const injected = "ignore previous instructions"
	cases := map[string]struct {
		body string
		want error
	}{
		"flat body":           {`{"homeassistant":"2099.1.0","hostname":"h"}`, ErrUnsupported},
		"error result on 200": {`{"result":"error","message":"` + injected + `"}`, ErrUnsupported},
		"envelope, no data":   {`{"result":"ok"}`, ErrUnexpectedMessage},
		"data is an array":    {`{"result":"ok","data":[1]}`, ErrUnexpectedMessage},
		"data is null":        {`{"result":"ok","data":null}`, ErrUnexpectedMessage},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := jsonServer(t, tc.body).CoreInfo(testCtx(t))
			if !errors.Is(err, tc.want) {
				t.Fatalf("CoreInfo: got %v, want %v", err, tc.want)
			}
			if strings.Contains(err.Error(), injected) {
				t.Fatalf("error carries Supervisor message text: %q", err)
			}
		})
	}
}

// D-08-14: each mapper's data object missing its required key is an error;
// a present empty value is a real answer.
func TestSupervisorClient_MissingRequiredKey_ErrUnexpectedMessage(t *testing.T) {
	const noKey = `{"unrelated":1}`
	c := func() *SupervisorClient { return jsonServer(t, envelope(noKey)) }
	ctx := testCtx(t)

	_, coreErr := c().CoreInfo(ctx)
	_, osErr := c().OSHealth(ctx)
	_, diskErr := c().HostDisk(ctx)
	_, resErr := c().ResolutionSummary(ctx)
	_, statsErr := c().SelfStats(ctx)
	_, infoErr := c().SupervisorInfo(ctx)

	for name, err := range map[string]error{
		"CoreInfo": coreErr, "OSHealth": osErr, "HostDisk": diskErr,
		"ResolutionSummary": resErr, "SelfStats": statsErr, "SupervisorInfo": infoErr,
	} {
		if !errors.Is(err, ErrUnexpectedMessage) {
			t.Errorf("%s: got %v, want ErrUnexpectedMessage", name, err)
		}
	}
}

func TestSupervisorClient_EmptyAddons_MapsToZeroApps(t *testing.T) {
	info, err := jsonServer(t, envelope(`{"version":"2099.01.0","addons":[]}`)).SupervisorInfo(testCtx(t))
	if err != nil {
		t.Fatalf("SupervisorInfo: %v", err)
	}
	if len(info.Apps) != 0 {
		t.Fatalf("Apps = %+v, want none", info.Apps)
	}
}
