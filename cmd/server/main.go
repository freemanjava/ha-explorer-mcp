// Command server runs the HA Inspector MCP server: a read-only diagnostic
// bridge between an AI agent and Home Assistant.
//
// The real wiring arrives in phase 01 (P1-01). This entry point exists so the
// build gate is green from the first commit.
package main

import (
	"fmt"
	"os"
)

// version is overridden at build time via -ldflags.
var version = "0.0.0-dev"

func main() {
	_, err := fmt.Fprintf(os.Stderr, "ha-inspector-mcp %s: not implemented yet (see docs/development/NEXT.md)\n", version)
	if err != nil {
		return
	}
}
