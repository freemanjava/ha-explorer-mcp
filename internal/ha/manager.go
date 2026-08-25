package ha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// maxCommandFrame bounds a single command result frame. It is a process
// safety limit, not a budget: response-size budgeting per doc §10 is Phase 02
// policy work. 8 MiB is above the largest frame P0-07 measured against live HA
// (7 days of one entity's unfiltered history, ~3.5 MB) and far below what
// would threaten a Raspberry Pi running Core alongside this binary.
const maxCommandFrame = 8 << 20

// maxBackoffAttempt caps the exponent fed to backoffDelay. The delay is
// already clamped to backoffMax; capping the exponent keeps the shift itself
// meaningful on a connection that has been failing for days.
const maxBackoffAttempt = 16

// defaultCallTimeout bounds a call whose caller supplied no deadline. Every
// upstream call carries a deadline (CLAUDE.md, Error Handling) — this is the
// backstop, not a licence to omit one. A var, not a const, so tests can shorten
// it; nothing in production writes it.
var defaultCallTimeout = 30 * time.Second

// Command is one Home Assistant WebSocket command. Implementations are typed
// per command and marshal to a JSON object holding that command's arguments;
// the manager owns the envelope's "id" and "type". There is deliberately no
// way to send a free-form frame (CLAUDE.md rule 2), and CommandType is the
// single value the allow-list in gateway.go matches on.
type Command interface {
	// CommandType returns the HA command name.
	CommandType() string
}

// BareCommand is a command carrying no arguments beyond its type.
type BareCommand string

// CommandType implements Command.
func (c BareCommand) CommandType() string { return string(c) }

// MarshalJSON emits the empty argument object; the type name travels in the
// envelope, which the manager writes.
func (c BareCommand) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

// CommandError is an error result Home Assistant returned for a command that
// reached it intact. It means the request was understood and refused or could
// not be served — distinct from ErrUpstreamUnavailable, which means it never
// got an answer. It preserves what HA said verbatim; Unwrap maps its Code
// onto this project's taxonomy for callers that branch with errors.Is.
type CommandError struct {
	Code    string
	Message string
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("ha: command failed: %s: %s", e.Code, e.Message)
}

// Unwrap maps the HA error code CommandError carries onto this project's
// taxonomy, so a caller can branch with errors.Is(err, ErrUnsupported) etc.
// without knowing HA's wire vocabulary, while errors.As(err, &cmdErr) still
// recovers the original code and message. "unauthorized" is what HA answers
// an admin-gated command with under a non-admin principal (P0-05); a caller
// degrades on it rather than treating it as an empty result. An unrecognized
// code maps to nothing — the *CommandError itself is still a valid, reported
// failure, never silently swallowed.
func (e *CommandError) Unwrap() error {
	switch e.Code {
	case "unauthorized":
		return ErrUnsupported
	case "not_found":
		return ErrNotFound
	default:
		return nil
	}
}

// resultEnvelope is the reply shape shared by command results and pongs. HA
// correlates every reply to its request by id; nothing else is trusted for
// correlation (replies may arrive in any order).
type resultEnvelope struct {
	ID      uint64          `json:"id"`
	Type    string          `json:"type"`
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Manager is a long-lived, multiplexing WebSocket connection to Home
// Assistant Core through the Supervisor proxy. It keeps one connection alive,
// re-establishing it with bounded backoff when Core restarts, and serves any
// number of concurrent requests over it, correlated by message id.
//
// The zero value is not usable; construct with NewManager and Start.
type Manager struct {
	url    string
	token  string
	logger *slog.Logger

	// nextID is monotonic across the manager's whole life, which also
	// satisfies HA's per-connection "ids must increase" rule.
	nextID     atomic.Uint64
	reconnects atomic.Uint64

	mu      sync.Mutex
	current *session      // nil while disconnected
	changed chan struct{} // closed and replaced whenever current changes
	closed  bool

	cancel context.CancelFunc
	done   chan struct{} // closed when the connect loop has exited
}

// NewManager returns a manager for the Core WebSocket at url, authenticating
// with token. token is read once by the caller from SUPERVISOR_TOKEN and is
// never logged, never returned in an error, and never stored anywhere else.
func NewManager(url, token string, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		url:     url,
		token:   token,
		logger:  logger,
		changed: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// Start launches the connect/serve loop in the background. It returns
// immediately; the first Call blocks until a connection is up or its own
// context expires. The loop stops when ctx is cancelled or Close is called.
func (m *Manager) Start(ctx context.Context) {
	loopCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.cancel = cancel
	m.mu.Unlock()
	go m.loop(loopCtx)
}

// Close stops the connect loop, fails every in-flight request with
// ErrUpstreamUnavailable and waits for the loop to exit. It is idempotent.
func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		<-m.done
		return
	}
	m.closed = true
	cancel := m.cancel
	m.broadcastLocked()
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	<-m.done
}

// Reconnects reports how many times the manager re-established the connection
// after losing one. Every recovery increments it, so degradation cannot be
// silent (CLAUDE.md, Logging).
func (m *Manager) Reconnects() uint64 { return m.reconnects.Load() }

// Call sends cmd and waits for its correlated reply. It returns
// ErrPolicyDenied if the command is not allow-listed — before any bytes leave
// the process — ErrUpstreamUnavailable if the connection dies with the request
// in flight, a *CommandError if HA answered with a failure, and ctx's error if
// the caller gave up first — four answers that must stay distinguishable.
//
// An HA restart that lands between acquiring the connection and sending is a
// non-event: nothing reached the wire, so Call waits for the next connection
// and sends there. Once the command has been transmitted its outcome is final
// and never re-sent.
func (m *Manager) Call(ctx context.Context, cmd Command) (json.RawMessage, error) {
	// The gateway check sits here, ahead of everything: ahead of acquiring a
	// connection, ahead of encoding, ahead of any wait. A denial must not
	// depend on whether HA happens to be reachable, and no denied command may
	// ever be encoded into a frame. Call is the only route to callOn, so this
	// is the single place a command can be sent from.
	if err := checkCommand(cmd.CommandType()); err != nil {
		return nil, err
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultCallTimeout)
		defer cancel()
	}

	var failed *session
	for {
		s, err := m.session(ctx, failed)
		if err != nil {
			return nil, err
		}

		result, transmitted, err := m.callOn(ctx, s, cmd)
		if transmitted || err == nil {
			return result, err
		}
		// The connection ended before the command left the process. Wait for
		// a different one rather than reporting a failure the manager is
		// already recovering from; m.session blocks until one exists, so this
		// cannot spin.
		failed = s
	}
}

// callOn runs one attempt on s. transmitted reports whether the command
// reached the wire: if it did not, the caller may safely try the next
// connection; if it did, the outcome is final.
func (m *Manager) callOn(ctx context.Context, s *session, cmd Command) (result json.RawMessage, transmitted bool, err error) {
	// A fresh id per attempt: HA requires ids to increase within a connection,
	// and a retried attempt is a new request on a new connection.
	id := m.nextID.Add(1)
	frame, err := encodeCommand(id, cmd)
	if err != nil {
		// A command that cannot be encoded is a programmer error; no
		// connection will make it encodable.
		return nil, true, err
	}

	replies := make(chan commandResult, 1)
	if err := s.register(id, replies); err != nil {
		return nil, false, err
	}
	defer s.unregister(id)

	if err := s.write(ctx, cmd, frame); err != nil {
		// A policy denial from write is final, not a connection problem: the
		// Call-level check ahead of everything else already catches this in
		// practice, so reaching it here means a future second send site is
		// missing its own early check. Either way retrying on another
		// connection would not help, so this is reported as done, not as a
		// transient failure to recover from.
		if errors.Is(err, ErrPolicyDenied) {
			return nil, true, err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, true, wrapDeadline(ctxErr)
		}
		return nil, false, fmt.Errorf("%w: sending %s", ErrUpstreamUnavailable, cmd.CommandType())
	}

	select {
	case reply := <-replies:
		return reply.result, true, reply.err
	case <-s.done:
		return nil, true, fmt.Errorf("%w: connection closed with %s in flight", ErrUpstreamUnavailable, cmd.CommandType())
	case <-ctx.Done():
		return nil, true, wrapDeadline(ctx.Err())
	}
}

// encodeCommand splices the envelope's id and type onto the command's own
// argument object. A Command that does not marshal to a JSON object is a
// programmer error and fails loudly rather than travelling malformed.
func encodeCommand(id uint64, cmd Command) ([]byte, error) {
	args, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("ha: marshalling command %s: %w", cmd.CommandType(), err)
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(args, &fields); err != nil {
		return nil, fmt.Errorf("ha: command %s must marshal to a JSON object: %w", cmd.CommandType(), err)
	}
	fields["id"] = json.RawMessage(fmt.Sprintf("%d", id))
	typeName, err := json.Marshal(cmd.CommandType())
	if err != nil {
		return nil, fmt.Errorf("ha: marshalling command type: %w", err)
	}
	fields["type"] = typeName
	return json.Marshal(fields)
}

// session returns a live connection, waiting for one if the manager is
// reconnecting. Waiting rather than failing fast is deliberate: an HA restart
// is normal, and the caller's deadline already bounds the wait. A session
// already known to be finished — avoid, or one that ended since — is never
// handed out.
func (m *Manager) session(ctx context.Context, avoid *session) (*session, error) {
	for {
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return nil, fmt.Errorf("%w: connection manager closed", ErrUpstreamUnavailable)
		}
		s, changed := m.current, m.changed
		m.mu.Unlock()

		if s != nil && s != avoid && !s.hasEnded() {
			return s, nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, wrapDeadline(ctx.Err())
		}
	}
}

func (m *Manager) setSession(s *session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current = s
	m.broadcastLocked()
}

// broadcastLocked wakes every goroutine waiting on a state change. The caller
// holds m.mu.
func (m *Manager) broadcastLocked() {
	close(m.changed)
	m.changed = make(chan struct{})
}

// loop keeps exactly one connection alive: connect, serve until it dies,
// back off, connect again.
func (m *Manager) loop(ctx context.Context) {
	defer close(m.done)
	defer m.setSession(nil)

	attempt := 0
	connected := false

	for {
		if ctx.Err() != nil {
			return
		}

		s, err := m.dial(ctx)
		if err != nil {
			// A rejected token is not transient: retrying it is a
			// self-inflicted denial of service against Supervisor.
			if errors.Is(err, ErrAuthFailed) {
				m.logger.ErrorContext(ctx, "ha: websocket auth rejected, connection manager stopping")
				return
			}
			delay := backoffDelay(attempt)
			if attempt < maxBackoffAttempt {
				attempt++
			}
			m.logger.WarnContext(ctx, "ha: websocket connect failed, backing off",
				"attempt", attempt, "delay_ms", delay.Milliseconds())
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			continue
		}

		attempt = 0
		if connected {
			n := m.reconnects.Add(1)
			m.logger.WarnContext(ctx, "ha: websocket reconnected", "reconnects", n)
		} else {
			connected = true
			m.logger.InfoContext(ctx, "ha: websocket connection manager ready")
		}

		m.setSession(s)
		s.serve(ctx)
		m.setSession(nil)
	}
}

// dial establishes one authenticated connection and wraps it in a session.
func (m *Manager) dial(ctx context.Context) (*session, error) {
	conn, _, err := websocket.Dial(ctx, m.url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: dial %s: %w", ErrUpstreamUnavailable, m.url, err)
	}

	conn.SetReadLimit(maxHandshakeFrame)
	if err := authenticate(ctx, conn, m.token); err != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "auth failed")
		return nil, err
	}
	conn.SetReadLimit(maxCommandFrame)

	return newSession(conn, m.logger), nil
}

// commandResult is one reply handed back to the caller that is waiting on it.
type commandResult struct {
	result json.RawMessage
	err    error
}

// session is one authenticated connection and the requests in flight on it.
// It owns its state: when the connection ends, every pending caller is failed
// exactly once and the session is never reused.
type session struct {
	conn   *websocket.Conn
	logger *slog.Logger

	writeMu sync.Mutex // one writer goroutine at a time, per the wire protocol

	mu      sync.Mutex
	pending map[uint64]chan commandResult
	ended   bool

	done      chan struct{}
	closeOnce sync.Once
}

func newSession(conn *websocket.Conn, logger *slog.Logger) *session {
	return &session{
		conn:    conn,
		logger:  logger,
		pending: make(map[uint64]chan commandResult),
		done:    make(chan struct{}),
	}
}

// register claims the reply slot for id. It fails if the connection has
// already ended, so a caller never waits on a session that will never answer.
func (s *session) register(id uint64, replies chan commandResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return fmt.Errorf("%w: connection closed before the request was sent", ErrUpstreamUnavailable)
	}
	s.pending[id] = replies
	return nil
}

// unregister frees the slot for id. Every Call defers it, so a cancelled or
// failed request leaves nothing behind.
func (s *session) unregister(id uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, id)
}

// hasEnded reports whether the connection is finished and will answer nothing
// further.
func (s *session) hasEnded() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ended
}

// write is the chokepoint a frame must pass through to reach the wire (F-18):
// the gateway check sits here, not only at the top of Call, because Call
// being the only route to write was previously an unenforced comment — a
// second send site added later to internal/ha would have bypassed it
// silently. No caller of write, however it got here, can turn a denied
// command into bytes on the socket.
func (s *session) write(ctx context.Context, cmd Command, frame []byte) error {
	if err := checkCommand(cmd.CommandType()); err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.Write(ctx, websocket.MessageText, frame)
}

// serve reads replies until the connection ends, dispatching each to the
// caller that owns its id. It returns only when the connection is finished.
func (s *session) serve(ctx context.Context) {
	defer s.shutdown()

	for {
		_, data, err := s.conn.Read(ctx)
		if err != nil {
			return
		}

		var env resultEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			// HA data is untrusted (threat T2): a malformed frame is
			// reported and skipped, never fatal to a long-lived process.
			s.logger.WarnContext(ctx, "ha: discarding unparsable websocket frame", "bytes", len(data))
			continue
		}
		// Frames without an id are unsolicited (subscription events, and the
		// handshake echoes); nothing correlates to them here.
		if env.ID == 0 {
			continue
		}
		s.deliver(env)
	}
}

func (s *session) deliver(env resultEnvelope) {
	s.mu.Lock()
	replies, ok := s.pending[env.ID]
	if ok {
		delete(s.pending, env.ID)
	}
	s.mu.Unlock()

	if !ok {
		// A reply to a request whose caller already gave up. Expected after a
		// cancellation, and not worth more than a debug line.
		s.logger.Debug("ha: reply for an unknown request id", "id", env.ID)
		return
	}

	// The slot is buffered and used once, so this never blocks.
	replies <- resultFor(env)
}

func resultFor(env resultEnvelope) commandResult {
	if env.Error != nil {
		return commandResult{err: &CommandError{Code: env.Error.Code, Message: env.Error.Message}}
	}
	// "pong" carries no success flag; a "result" that is not successful and
	// carries no error object is a protocol violation, not an empty result.
	if env.Type == "result" && !env.Success {
		return commandResult{err: fmt.Errorf("%w: unsuccessful result without an error for id %d", ErrUnexpectedMessage, env.ID)}
	}
	return commandResult{result: env.Result}
}

// shutdown ends the session exactly once: no new slots, every waiting caller
// released, the socket closed.
func (s *session) shutdown() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.ended = true
		pending := s.pending
		s.pending = make(map[uint64]chan commandResult)
		s.mu.Unlock()

		for id, replies := range pending {
			replies <- commandResult{err: fmt.Errorf("%w: connection closed with request %d in flight", ErrUpstreamUnavailable, id)}
		}

		close(s.done)
		err := s.conn.CloseNow()
		if err != nil {
			return
		}
	})
}
