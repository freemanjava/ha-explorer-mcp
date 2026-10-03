// Command server runs the HA Inspector MCP server: a read-only diagnostic
// bridge between an AI agent and Home Assistant.
//
// It speaks MCP over stdio for development and over Streamable HTTP as the
// App (D-08-1, selected by HA_INSPECTOR_TRANSPORT; the image fixes http).
// Every diagnostic line goes to stderr, because stdout carries the stdio
// protocol framing.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/mcp"
)

// version is overridden at build time via -ldflags.
var version = "0.0.0-dev"

// Configuration comes from the environment and the App options file — never
// from /config (ADR-004).
const (
	// envSupervisorToken is the Supervisor-issued token. It is read once, held
	// in memory, and registered as a secret so it cannot appear in a response,
	// a log line or an audit record (rule 4).
	envSupervisorToken = "SUPERVISOR_TOKEN"
	// envPrivacyProfile selects the privacy profile: mask (default), allow or
	// deny. An unrecognized value is a startup failure, not a fallback.
	envPrivacyProfile = "HA_INSPECTOR_PRIVACY_PROFILE"
	// envLogLevel selects the log level: debug, info (default), warn or error.
	envLogLevel = "HA_INSPECTOR_LOG_LEVEL"
)

// coreWebSocketURL is where Core's WebSocket API is reachable from inside an
// App container, through the Supervisor proxy (architecture doc §2;
// docs/research/2026-08-23-supervisor-permissions.md: `/core/websocket` is
// `no_security_check` at Supervisor's own layer — Core still requires its own
// auth handshake, which is what SUPERVISOR_TOKEN is for). There is no
// override: this binary only ever runs as the App the Supervisor started.
const coreWebSocketURL = "ws://supervisor/core/websocket"

func main() {
	if err := run(); err != nil {
		// The discard is explicit, not an oversight: this is the last-resort
		// reporting path, and a failed write to stderr has nowhere left to be
		// reported. The exit code still carries the failure to the Supervisor.
		_, _ = fmt.Fprintf(os.Stderr, "ha-inspector-mcp: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadSettings(os.Getenv, optionsPath)
	if err != nil {
		return err
	}

	transport, err := loadTransport(os.Getenv, optionsPath)
	if err != nil {
		return err
	}

	token := os.Getenv(envSupervisorToken)
	var secrets []string
	if token != "" {
		secrets = append(secrets, token)
	}
	if transport.httpSecret != "" {
		secrets = append(secrets, transport.httpSecret)
	}

	log := mcp.NewLogger(cfg.level, secrets...)

	// An interrupt cancels the context, which closes the stdio session or the
	// HTTP listener; the Supervisor stopping the App is a normal shutdown, not
	// a crash.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.InfoContext(ctx, "starting", "version", version, "transport", transport.transport, "privacy_profile", cfg.profileName)

	manager := ha.NewManager(coreWebSocketURL, token, log)
	manager.Start(ctx)
	defer manager.Close()

	registry := ha.NewRegistryCache(manager)
	core := ha.NewCoreReader(manager)
	supervisor := ha.NewSupervisorClient("", token, nil, log)

	err = mcp.Run(ctx, mcp.Options{
		Version:      version,
		Transport:    transport.transport,
		HTTPSecret:   transport.httpSecret,
		Logger:       log,
		Profile:      cfg.profile,
		Secrets:      secrets,
		Core:         core,
		Inventory:    registry,
		Supervisor:   supervisor,
		Availability: core,
		States:       core,
		Areas:        registry,
		Automations:  core,
		Repairs:      core,
		History:      core,

		AutomationDetail: core,
		Logbook:          core,
		Lifecycle:        core,
	})
	// mcp.Run already treats any way an established session ends — cancelled
	// context, clean disconnect, or a client dying mid-request — as a normal
	// shutdown (P3-08, F-21); a non-nil error here means the session never
	// started, a real failure to propagate.
	return err
}
