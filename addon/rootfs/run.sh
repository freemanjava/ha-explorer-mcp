#!/bin/sh
# init: false in config.yaml — Supervisor runs this directly, no s6/tini
# layer between it and the Go binary, so signals reach the process unmodified.
set -e
# The App serves HTTP; stdio would read EOF at once and exit (D-08-9, F-37).
export HA_INSPECTOR_TRANSPORT=http
exec /usr/bin/ha-inspector-mcp
