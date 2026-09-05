package mcp

import "errors"

var (
	// ErrNotImplemented indicates a catalog row whose implementing task has
	// not landed yet. It is distinct from ErrUnsupported (internal/ha): that
	// one is an observation about the installation, this one is a fact about
	// this build, and collapsing them would let an unfinished server look
	// like an unfinished Home Assistant.
	ErrNotImplemented = errors.New("mcp: tool not implemented in this build")

	// ErrUnknownTool indicates an invocation named a tool that is not in the
	// static catalog. It is refused before anything runs: an unknown name has
	// no budget class, and inventing one is the failure rule 3 forbids.
	ErrUnknownTool = errors.New("mcp: unknown tool")

	// ErrToolPanicked indicates a tool handler panicked. The panic value is
	// deliberately not part of the error: it can carry an upstream payload,
	// and an error string must never do that (CLAUDE.md, Error Handling).
	ErrToolPanicked = errors.New("mcp: tool failed unexpectedly")
)
