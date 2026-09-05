// Command server runs the HA Inspector MCP server: a read-only diagnostic
// bridge between an AI agent and Home Assistant.
//
// It speaks MCP over stdio (the 2026-08-25 transport decision): the Supervisor
// starts it as a child process, there is no listening socket and no client
// authentication subsystem. Every diagnostic line goes to stderr, because
// stdout carries the protocol framing.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/freemanjava/ha-explorer-mcp/internal/mcp"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
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
	level, err := logLevel(os.Getenv(envLogLevel))
	if err != nil {
		return err
	}
	profile, err := policy.NewProfile(os.Getenv(envPrivacyProfile))
	if err != nil {
		return err
	}

	var secrets []string
	if token := os.Getenv(envSupervisorToken); token != "" {
		secrets = append(secrets, token)
	}

	log := mcp.NewLogger(level, secrets...)

	// An interrupt cancels the context, which closes the stdio session; the
	// Supervisor stopping the App is a normal shutdown, not a crash.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.InfoContext(ctx, "starting", "version", version, "transport", "stdio", "privacy_profile", os.Getenv(envPrivacyProfile))

	err = mcp.Run(ctx, mcp.Options{
		Version: version,
		Logger:  log,
		Profile: profile,
		Secrets: secrets,
	})
	// A client that closes the pipe ends the session; that is a shutdown, not
	// a failure to report.
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		return err
	}
	log.InfoContext(ctx, "stopped")
	return nil
}

func logLevel(name string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown %s %q: want debug, info, warn or error", envLogLevel, name)
	}
}
