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

	// ErrUnexpectedMessage indicates the server sent a message that violates
	// the documented handshake or response protocol.
	ErrUnexpectedMessage = errors.New("ha: unexpected message from server")
)
