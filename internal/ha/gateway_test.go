package ha

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// recordingServer answers every command it receives and records the command
// names that actually reached the wire. Denial tests assert on this, not on
// the returned error: a gateway that returns ErrPolicyDenied *after*
// transmitting has already failed (doc §11, ADR-008).
type recordingServer struct {
	mu   sync.Mutex
	seen []string
}

func (r *recordingServer) record(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, name)
}

func (r *recordingServer) transmitted() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.seen...)
}

// startGatewayFixture brings up a fake HA that echoes every command back and
// records what it saw, plus a manager talking to it.
func startGatewayFixture(t *testing.T) (*Manager, *recordingServer) {
	t.Helper()
	rec := &recordingServer{}
	srv := newFakeHAServer(t, func(ctx context.Context, conn *websocket.Conn) {
		if !serveAuthHandshake(ctx, conn, testToken) {
			return
		}
		serveCommands(ctx, conn, func(cmd commandFrame) {
			rec.record(cmd.Type)
			_ = writeResult(ctx, conn, cmd.ID, map[string]any{"echoed": cmd.Type})
		})
	})
	return startManager(t, wsURL(srv), nil), rec
}

// waitConnected forces the manager to establish its connection, so a later
// denial cannot be mistaken for "nothing was sent because nothing could be".
func waitConnected(t *testing.T, m *Manager) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := m.Call(ctx, BareCommand(CommandGetConfig)); err != nil {
		t.Fatalf("priming call: unexpected error: %v", err)
	}
}

func TestUnknownCommandDenied(t *testing.T) {
	m, rec := startGatewayFixture(t)
	waitConnected(t, m)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := m.Call(ctx, BareCommand("definitely_not_a_command"))
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("Call returned %v, want ErrPolicyDenied", err)
	}
	assertNotTransmitted(t, rec, "definitely_not_a_command")
}

func TestMutatingCommandDenied(t *testing.T) {
	mutating := []string{
		"call_service",
		"fire_event",
		"config/entity_registry/update",
		"supervisor/restart",
		"supervisor/api",
	}

	m, rec := startGatewayFixture(t)
	waitConnected(t, m)

	for _, name := range mutating {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, err := m.Call(ctx, BareCommand(name))
			if !errors.Is(err, ErrPolicyDenied) {
				t.Fatalf("Call(%q) returned %v, want ErrPolicyDenied", name, err)
			}
			assertNotTransmitted(t, rec, name)
		})
	}
}

// assertNotTransmitted fails unless the denied command left no trace on the
// wire. The echo of the priming call proves the connection was live, so an
// empty recording is a denial, not an outage.
func assertNotTransmitted(t *testing.T, rec *recordingServer, name string) {
	t.Helper()
	seen := rec.transmitted()
	if len(seen) == 0 {
		t.Fatalf("the fake server saw no commands at all; the fixture is not proving anything")
	}
	for _, got := range seen {
		if got == name {
			t.Fatalf("%q reached the socket; denial must happen before transmission", name)
		}
	}
}

// TestAllowList_NoMutationVerb is the guard on future edits: the allow-list is
// the security boundary, and a mutating command added to it would be a silent
// removal of the product's read-only guarantee (CLAUDE.md rule 1).
func TestAllowList_NoMutationVerb(t *testing.T) {
	verbs := []string{"update", "create", "delete", "remove", "set", "call"}
	for name := range allowedCommands {
		for _, verb := range verbs {
			if strings.Contains(name, verb) {
				t.Errorf("allow-list entry %q contains the mutation verb %q", name, verb)
			}
		}
	}
}

// TestAllowList_DenialReasonNamesTheTable keeps the two denial reasons
// distinguishable for the audit record P1-07 builds on.
func TestAllowList_DenialReasonNamesTheTable(t *testing.T) {
	err := checkCommand("definitely_not_a_command")
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("checkCommand returned %v, want ErrPolicyDenied", err)
	}
	if !strings.Contains(err.Error(), "not allow-listed") {
		t.Fatalf("denial reason %q does not say the command is not allow-listed", err)
	}
	if !strings.Contains(err.Error(), "definitely_not_a_command") {
		t.Fatalf("denial reason %q does not name the refused command", err)
	}
}

// TestGateway_SupervisorAPICommand_Denied is P1-07's DoD: supervisor/api is
// refused by name, with no bytes reaching the fake server.
func TestGateway_SupervisorAPICommand_Denied(t *testing.T) {
	m, rec := startGatewayFixture(t)
	waitConnected(t, m)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := m.Call(ctx, BareCommand("supervisor/api"))
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("Call returned %v, want ErrPolicyDenied", err)
	}
	assertNotTransmitted(t, rec, "supervisor/api")
}

// TestGateway_DenySet_IndependentOfAllowList proves the deny does not depend
// on allow-list contents: supervisor/api is refused identically whether the
// allow-list holds its usual entries or is empty (F-13).
func TestGateway_DenySet_IndependentOfAllowList(t *testing.T) {
	populated := checkCommand("supervisor/api")
	if !errors.Is(populated, ErrPolicyDenied) {
		t.Fatalf("checkCommand with a populated allow-list returned %v, want ErrPolicyDenied", populated)
	}

	original := allowedCommands
	allowedCommands = map[string]struct{}{}
	t.Cleanup(func() { allowedCommands = original })

	empty := checkCommand("supervisor/api")
	if !errors.Is(empty, ErrPolicyDenied) {
		t.Fatalf("checkCommand with an empty allow-list returned %v, want ErrPolicyDenied", empty)
	}
	if populated.Error() != empty.Error() {
		t.Fatalf("denial reason changed with the allow-list emptied: %q vs %q", populated, empty)
	}
}

// TestGateway_DenySet_NotInAllowList keeps the two tables from contradicting
// each other: nothing refused by name may also be permitted by the
// allow-list.
func TestGateway_DenySet_NotInAllowList(t *testing.T) {
	for name := range deniedCommands {
		if _, ok := allowedCommands[name]; ok {
			t.Errorf("denied command %q also appears in the allow-list", name)
		}
	}
}

// TestGateway_DenialReason_DistinguishesDenyFromAllowList lets an audit
// record say which table refused a command (P1-07).
func TestGateway_DenialReason_DistinguishesDenyFromAllowList(t *testing.T) {
	denied := checkCommand("supervisor/api")
	if !strings.Contains(denied.Error(), "denied by name") {
		t.Fatalf("denial reason %q does not say the command is denied by name", denied)
	}
	notAllowed := checkCommand("definitely_not_a_command")
	if strings.Contains(notAllowed.Error(), "denied by name") {
		t.Fatalf("denial reason %q for an unlisted command should not read as a named deny", notAllowed)
	}
}

// TestSession_Write_DeniedCommand_NeverReachesSocket is F-18's chokepoint
// property: a denied command cannot be turned into a sendable frame even
// when the caller talks to the session directly, bypassing Manager.Call
// entirely. This is what proves the guarantee survives a future second
// send site inside internal/ha.
func TestSession_Write_DeniedCommand_NeverReachesSocket(t *testing.T) {
	rec := &recordingServer{}
	srv := newFakeHAServer(t, func(ctx context.Context, conn *websocket.Conn) {
		if !serveAuthHandshake(ctx, conn, testToken) {
			return
		}
		serveCommands(ctx, conn, func(cmd commandFrame) {
			rec.record(cmd.Type)
			_ = writeResult(ctx, conn, cmd.ID, map[string]any{"echoed": cmd.Type})
		})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(srv), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow()
	if err := authenticate(ctx, conn, testToken); err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	s := newSession(conn, nil)
	cmd := BareCommand("supervisor/api")
	frame, err := encodeCommand(1, cmd)
	if err != nil {
		t.Fatalf("encodeCommand: %v", err)
	}

	if err := s.write(ctx, cmd, frame); !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("session.write returned %v, want ErrPolicyDenied", err)
	}

	// Prove the connection was live by sending a permitted command on it,
	// then assert the denied one never reached the server at any point.
	allowedFrame, err := encodeCommand(2, BareCommand(CommandGetConfig))
	if err != nil {
		t.Fatalf("encodeCommand: %v", err)
	}
	if err := s.write(ctx, BareCommand(CommandGetConfig), allowedFrame); err != nil {
		t.Fatalf("session.write of a permitted command: unexpected error: %v", err)
	}
	// Wait for the reply so the server's record() call for get_config has
	// definitely happened before the assertion below — the fixture serves
	// commands from one goroutine, in order, so a reply proves it.
	if _, _, err := conn.Read(ctx); err != nil {
		t.Fatalf("reading reply to the permitted command: %v", err)
	}
	assertNotTransmitted(t, rec, "supervisor/api")
}

func TestAllowList_PermittedCommandsAreSent(t *testing.T) {
	m, rec := startGatewayFixture(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for name := range allowedCommands {
		if _, err := m.Call(ctx, BareCommand(name)); err != nil {
			t.Fatalf("Call(%q): unexpected error: %v", name, err)
		}
	}

	if got, want := len(rec.transmitted()), len(allowedCommands); got != want {
		t.Fatalf("%d commands reached the socket, want %d", got, want)
	}
}
