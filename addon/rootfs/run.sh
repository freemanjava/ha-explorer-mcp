#!/bin/sh
# init: false in config.yaml — Supervisor runs this directly, no s6/tini
# layer between it and the Go binary, so signals reach the process unmodified.
set -e
exec /usr/bin/ha-inspector-mcp
