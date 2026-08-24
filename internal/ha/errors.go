package ha

import "errors"

var (
	// ErrAuthFailed indicates Core rejected the SUPERVISOR_TOKEN during the
	// WebSocket auth handshake (auth_invalid). Connect returns it once and
	// does not retry — a rejected token retried in a tight loop is a
	// self-inflicted denial of service against Supervisor.
	ErrAuthFailed = errors.New("ha: authentication rejected by websocket handshake")

	// ErrUpstreamUnavailable indicates Core/Supervisor could not be reached,
	// or the connection was lost before a request completed.
	ErrUpstreamUnavailable = errors.New("ha: upstream unavailable")

	// ErrPolicyDenied indicates the gateway refused a request before any
	// bytes left the process: the command is not on the allow-list, or is
	// denied by name. It is never the report of an upstream refusal — that is
	// a *CommandError.
	ErrPolicyDenied = errors.New("ha: denied by gateway policy")

	// ErrUnexpectedMessage indicates the server sent a message that violates
	// the documented handshake or response protocol.
	ErrUnexpectedMessage = errors.New("ha: unexpected message from server")
)
