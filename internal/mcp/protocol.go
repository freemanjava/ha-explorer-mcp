// Package mcp holds the MCP server, its tool registry and tool definitions.
package mcp

// SupportedProtocolVersion is the MCP wire protocol version this project is
// built against (P0-01). TestSDKProtocolVersion asserts the pinned SDK still
// negotiates this version, so an SDK bump that changes the wire version fails
// the build instead of shipping silently.
const SupportedProtocolVersion = "2026-07-28"
