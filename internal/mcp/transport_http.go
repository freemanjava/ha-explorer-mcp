package mcp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// Transport names, as selected by HA_INSPECTOR_TRANSPORT and recorded in every
// audit record (D-08-9, D-08-10).
const (
	TransportStdio = "stdio"
	TransportHTTP  = "http"
)

// The HTTP transport's fixed surface and limits — ADR-013, D-08-4…D-08-11.
// None is configurable: the owner picks the host port on the App's Network
// tab, and the limits are what keeps a LAN client from loading the Pi.
const (
	// httpListenAddr binds every container interface; under host_network:
	// false that is only the hassio bridge (D-08-6).
	httpListenAddr = ":8790"
	// httpPath is the one route served; everything else is 404 (D-08-6).
	httpPath = "/mcp"

	// maxRequestBodyBytes is ~2.5x the largest legal tool input — 200 ids at
	// HA's 255-character ceiling — against the SDK's 4 MiB default (D-08-7).
	maxRequestBodyBytes = 128 << 10
	maxHeaderBytes      = 8 << 10
	readHeaderTimeout   = 5 * time.Second
	readTimeout         = 10 * time.Second // a <=128 KiB body on a LAN
	idleTimeout         = 60 * time.Second
	// writeTimeoutMargin is added to the composite deadline so a composite
	// tool's own deadline fires first and returns partial rather than a cut
	// connection (D-08-7).
	writeTimeoutMargin = 15 * time.Second
	// maxInFlight bounds authenticated requests in flight: composites fan out
	// to several HA reads each, and the recorder on the Pi is the measured
	// binding constraint (D-08-7).
	maxInFlight = 4

	// rejectionWarnEvery bounds the WARN a probing host can cause (D-08-10).
	rejectionWarnEvery = time.Minute
	shutdownGrace      = 5 * time.Second
)

// Why a request was refused before reaching the MCP server — a closed set, so
// the log says what class of attempt it was without echoing the credential.
const (
	reasonOrigin    = "origin"
	reasonMissing   = "missing"
	reasonMalformed = "malformed"
	reasonMismatch  = "mismatch"
)

// newHTTPHandler wraps srv in the gate: route, Origin, bearer secret, in-flight
// cap, in that order, then the SDK's stateless JSON handler. An empty secret is
// refused here as well as in cmd/server — a listener without one must not be
// constructible (D-08-4).
//
// srv is the one process-wide server, returned for every request, so its
// invocation rate limiter is shared by all POSTs rather than reset per request
// (D-08-7).
func newHTTPHandler(srv *sdkmcp.Server, secret string, log *slog.Logger) (http.Handler, error) {
	if secret == "" {
		return nil, errors.New("http transport requires a client secret")
	}
	sdk := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return srv }, &sdkmcp.StreamableHTTPOptions{
		Stateless:           true,
		JSONResponse:        true,
		MaxRequestBodyBytes: maxRequestBodyBytes,
		Logger:              log,
	})
	return &gate{
		digest: sha256.Sum256([]byte(secret)),
		slots:  make(chan struct{}, maxInFlight),
		rej:    &rejections{log: log, now: time.Now},
		next:   sdk,
	}, nil
}

type gate struct {
	// digest is SHA-256 of the secret: comparing equal-length digests in
	// constant time leaks neither content nor length through timing (D-08-4).
	digest [sha256.Size]byte
	slots  chan struct{}
	rej    *rejections
	next   http.Handler
}

func (g *gate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != httpPath {
		http.NotFound(w, r)
		return
	}
	// Any Origin, even one equal to Host: a browser always sends one on a
	// cross-site POST and no supported client sends any (D-08-5).
	if _, present := r.Header["Origin"]; present {
		g.rej.note(r.Context(), remoteHost(r), reasonOrigin)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if reason := g.check(r); reason != "" {
		g.rej.note(r.Context(), remoteHost(r), reason)
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// After auth, so unauthenticated traffic cannot occupy the slots.
	select {
	case g.slots <- struct{}{}:
		defer func() { <-g.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		http.Error(w, "busy", http.StatusServiceUnavailable)
		return
	}

	// The SDK passes every header to server code as RequestExtra.Header; a
	// secret that never enters internal/mcp cannot be logged from there.
	r.Header.Del("Authorization")
	g.next.ServeHTTP(w, r)
}

// check returns "" for a valid credential, else the reason class.
func (g *gate) check(r *http.Request) string {
	values := r.Header.Values("Authorization")
	switch len(values) {
	case 0:
		return reasonMissing
	case 1:
	default:
		return reasonMalformed
	}
	scheme, token, ok := strings.Cut(values[0], " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return reasonMalformed
	}
	presented := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(presented[:], g.digest[:]) != 1 {
		return reasonMismatch
	}
	return ""
}

func remoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rejections counts refused requests and logs them without letting one
// scanner flood the Supervisor log: DEBUG for each, WARN at most once per
// rejectionWarnEvery with the count since the last (D-08-10).
type rejections struct {
	log *slog.Logger
	now func() time.Time

	mu        sync.Mutex
	total     int64
	sinceWarn int64
	lastWarn  time.Time
}

func (r *rejections) note(ctx context.Context, ip, reason string) {
	r.log.DebugContext(ctx, "http request rejected", "remote_ip", ip, "reason", reason)

	r.mu.Lock()
	r.total++
	r.sinceWarn++
	warn := r.now().Sub(r.lastWarn) >= rejectionWarnEvery
	count, total := r.sinceWarn, r.total
	if warn {
		r.sinceWarn = 0
		r.lastWarn = r.now()
	}
	r.mu.Unlock()

	if warn {
		r.log.WarnContext(ctx, "http requests rejected", "count", count, "http_auth_rejections", total, "last_remote_ip", ip, "last_reason", reason)
	}
}

func newHTTPServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		MaxHeaderBytes:    maxHeaderBytes,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      policy.CompositeDeadline() + writeTimeoutMargin,
		IdleTimeout:       idleTimeout,
	}
}

// runHTTP serves srv on ln until ctx is cancelled. A cancelled context is the
// Supervisor stopping the App — a normal shutdown, not a failure.
func runHTTP(ctx context.Context, srv *sdkmcp.Server, secret string, ln net.Listener, log *slog.Logger) error {
	h, err := newHTTPHandler(srv, secret, log)
	if err != nil {
		return err
	}
	hs := newHTTPServer(h)

	served := make(chan error, 1)
	go func() { served <- hs.Serve(ln) }()
	log.InfoContext(ctx, "listening", "transport", TransportHTTP, "addr", ln.Addr().String())

	select {
	case err := <-served:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := hs.Shutdown(shutdownCtx); err != nil {
			_ = hs.Close()
		}
		log.InfoContext(ctx, "stopped", "reason", "context cancelled")
		return nil
	}
}
