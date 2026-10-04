package ha

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/coder/websocket"
)

var errMeterRefused = errors.New("test: meter refused")

// countingMeter records charges and refuses once refuseAfter have been taken.
type countingMeter struct {
	charged     int
	refuseAfter int // -1 never refuses
}

func (m *countingMeter) ChargeHARequests(n int) error {
	if m.refuseAfter >= 0 && m.charged+n > m.refuseAfter {
		return errMeterRefused
	}
	m.charged += n
	return nil
}

func newMeter(refuseAfter int) *countingMeter { return &countingMeter{refuseAfter: refuseAfter} }

func frameCountingManager(t *testing.T, reply func(ctx context.Context, conn *websocket.Conn, cmd commandFrame)) (*Manager, *atomic.Int64) {
	t.Helper()
	var frames atomic.Int64
	srv := newFakeHAServer(t, func(ctx context.Context, conn *websocket.Conn) {
		if !serveAuthHandshake(ctx, conn, testToken) {
			return
		}
		serveCommands(ctx, conn, func(cmd commandFrame) {
			frames.Add(1)
			reply(ctx, conn, cmd)
		})
	})
	return startManager(t, wsURL(srv), nil), &frames
}

func TestManagerCall_MeterInContext_ChargedOncePerRequest(t *testing.T) {
	m, _ := frameCountingManager(t, func(ctx context.Context, conn *websocket.Conn, cmd commandFrame) {
		_ = writeResult(ctx, conn, cmd.ID, map[string]any{})
	})
	meter := newMeter(-1)
	ctx := WithRequestMeter(testCtx(t), meter)

	for range 3 {
		if _, err := m.Call(ctx, BareCommand("get_config")); err != nil {
			t.Fatalf("Call: %v", err)
		}
	}
	if meter.charged != 3 {
		t.Fatalf("charged = %d, want 3", meter.charged)
	}
}

func TestManagerCall_DeniedCommand_ChargesNothingSendsNothing(t *testing.T) {
	m, frames := frameCountingManager(t, func(ctx context.Context, conn *websocket.Conn, cmd commandFrame) {
		_ = writeResult(ctx, conn, cmd.ID, map[string]any{})
	})
	meter := newMeter(-1)
	ctx := WithRequestMeter(testCtx(t), meter)

	_, err := m.Call(ctx, BareCommand("call_service"))
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("Call(call_service): got %v, want ErrPolicyDenied", err)
	}
	if meter.charged != 0 {
		t.Errorf("a denied command charged %d requests, want 0", meter.charged)
	}
	if n := frames.Load(); n != 0 {
		t.Errorf("a denied command put %d frames on the wire, want 0", n)
	}
}

func TestManagerCall_MeterRefuses_ErrorKeepsSentinelAndNothingIsSent(t *testing.T) {
	m, frames := frameCountingManager(t, func(ctx context.Context, conn *websocket.Conn, cmd commandFrame) {
		_ = writeResult(ctx, conn, cmd.ID, map[string]any{})
	})
	ctx := WithRequestMeter(testCtx(t), newMeter(0))

	_, err := m.Call(ctx, BareCommand("get_config"))
	if !errors.Is(err, errMeterRefused) {
		t.Fatalf("Call: got %v, want the meter's refusal", err)
	}
	if n := frames.Load(); n != 0 {
		t.Errorf("a refused request put %d frames on the wire, want 0", n)
	}
	if n := m.pendingCount(); n != 0 {
		t.Errorf("%d pending slots after a refused request, want 0", n)
	}
}

func TestManagerCall_UpstreamFailure_StillCounted(t *testing.T) {
	m, _ := frameCountingManager(t, func(ctx context.Context, conn *websocket.Conn, cmd commandFrame) {
		_ = writeErrorResult(ctx, conn, cmd.ID, "not_found", "gone")
	})
	meter := newMeter(-1)

	if _, err := m.Call(WithRequestMeter(testCtx(t), meter), BareCommand("get_config")); err == nil {
		t.Fatal("Call: want the upstream error")
	}
	if meter.charged != 1 {
		t.Fatalf("charged = %d, want 1: a failed request was still asked for", meter.charged)
	}
}

func TestManagerCall_NoMeter_IsNoOp(t *testing.T) {
	m, _ := frameCountingManager(t, func(ctx context.Context, conn *websocket.Conn, cmd commandFrame) {
		_ = writeResult(ctx, conn, cmd.ID, map[string]any{})
	})
	if _, err := m.Call(testCtx(t), BareCommand("get_config")); err != nil {
		t.Fatalf("Call without a meter: %v", err)
	}
}

func TestSupervisorGet_MeterInContext_ChargedOncePerRequest(t *testing.T) {
	srv, requests := countingServer(t, nil)
	c := NewSupervisorClient(srv.URL, testToken, srv.Client(), nil)
	meter := newMeter(-1)

	if _, err := c.get(WithRequestMeter(testCtx(t), meter), SupervisorRouteInfo); err != nil {
		t.Fatalf("get: %v", err)
	}
	if meter.charged != 1 || requests.Load() != 1 {
		t.Fatalf("charged = %d, requests = %d, want 1 and 1", meter.charged, requests.Load())
	}
}

func TestSupervisorGet_DeniedRoute_ChargesNothing(t *testing.T) {
	srv, requests := countingServer(t, nil)
	c := NewSupervisorClient(srv.URL, testToken, srv.Client(), nil)
	meter := newMeter(-1)

	_, err := c.get(WithRequestMeter(testCtx(t), meter), "/addons")
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("get(/addons): got %v, want ErrPolicyDenied", err)
	}
	if meter.charged != 0 || requests.Load() != 0 {
		t.Fatalf("charged = %d, requests = %d, want 0 and 0", meter.charged, requests.Load())
	}
}

func TestSupervisorGet_MeterRefuses_NothingIsSent(t *testing.T) {
	srv, requests := countingServer(t, nil)
	c := NewSupervisorClient(srv.URL, testToken, srv.Client(), nil)

	_, err := c.get(WithRequestMeter(testCtx(t), newMeter(0)), SupervisorRouteInfo)
	if !errors.Is(err, errMeterRefused) {
		t.Fatalf("get: got %v, want the meter's refusal", err)
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("a refused request reached the server: %d", n)
	}
}

func TestSupervisorGet_UpstreamFailure_StillCounted(t *testing.T) {
	srv, _ := countingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	c := NewSupervisorClient(srv.URL, testToken, srv.Client(), nil)
	meter := newMeter(-1)

	if _, err := c.get(WithRequestMeter(testCtx(t), meter), SupervisorRouteInfo); err == nil {
		t.Fatal("get: want the upstream error")
	}
	if meter.charged != 1 {
		t.Fatalf("charged = %d, want 1", meter.charged)
	}
}
