package ha

import (
	"context"
	"errors"
	"fmt"
)

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

	// ErrNotFound indicates the upstream answered that the thing asked for
	// does not exist — an entity id with no state, for example. It is a
	// different answer from "could not check" (ErrUnsupported) and stays
	// distinguishable all the way to the MCP response.
	ErrNotFound = errors.New("ha: not found")

	// ErrResponseTooLarge indicates an upstream response exceeded the
	// process safety cap and was refused rather than buffered whole. It is a
	// truncation report, not a budget decision — budgets are Phase 02.
	ErrResponseTooLarge = errors.New("ha: response exceeds size limit")

	// ErrUnsupported indicates the thing asked for cannot be answered by this
	// connection — not "absent" (ErrNotFound) and not "refused by our own
	// policy" (ErrPolicyDenied), but "HA itself would not serve it here": an
	// admin-gated command answered `unauthorized` to a non-admin principal, a
	// Supervisor-only feature unreachable while Core is up. A caller degrades
	// on it — lowers confidence, populates missing_evidence — rather than
	// aborting the whole diagnostic (CLAUDE.md, Reliability).
	ErrUnsupported = errors.New("ha: not supported by this connection")

	// ErrDeadline indicates the caller's own context deadline was reached
	// while a request was outstanding — distinct from ErrUpstreamUnavailable,
	// which means the connection itself failed. A caller that retries on
	// ErrUpstreamUnavailable but not on ErrDeadline needs the two kept apart.
	ErrDeadline = errors.New("ha: caller deadline exceeded")
)

// wrapDeadline reports a caller's context error as ErrDeadline when it is a
// deadline, and leaves anything else (context.Canceled, nil) untouched. It
// exists so every site that surfaces ctx.Err() does so through the same
// sentinel, rather than callers above having to know context.DeadlineExceeded
// is the stdlib spelling of it. The wrapped text is the context package's own
// fixed string, never a URL or payload, so it is safe to include whole
// (CLAUDE.md rule 4).
func wrapDeadline(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrDeadline, err)
	}
	return err
}
