package mcp

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// TestNewLogger_WritesNothingToStdout is the DoD's stdout clause. Under stdio
// transport, stdout carries the MCP framing: a log line there corrupts the
// protocol stream for the rest of the session, and no test of a tool would
// catch it. The assertion is over the constructor, so every future log call
// inherits it.
func TestNewLogger_WritesNothingToStdout(t *testing.T) {
	stdout, stderr := captureStdio(t)

	log := NewLogger(slog.LevelDebug, "s3cret")
	log.Debug("debug", "k", "v")
	log.Info("info", "k", "v")
	log.Warn("warn", "k", "v")
	log.Error("error", "k", "v")

	if got := stdout(); got != "" {
		t.Fatalf("the logger wrote %q to stdout — this corrupts the MCP framing", got)
	}
	if got := stderr(); got == "" {
		t.Fatal("the logger wrote nothing to stderr either — the test would pass vacuously")
	}
}

// TestNewLogger_ScrubsSecrets asserts the constructor puts the redacting
// handler in front of the sink, so rule 4 holds for log lines and not only for
// responses.
func TestNewLogger_ScrubsSecrets(t *testing.T) {
	_, stderr := captureStdio(t)

	NewLogger(slog.LevelInfo, "super-secret-token").Info("connected", "token", "super-secret-token")

	if got := stderr(); got == "" {
		t.Fatal("nothing was logged")
	} else if strings.Contains(got, "super-secret-token") {
		t.Fatalf("the secret reached the log line: %q", got)
	}
}

// captureStdio replaces the process's stdout and stderr with pipes for the
// duration of the test and returns readers for what was written to each. The
// logger captures os.Stderr at construction, so the swap must precede it.
func captureStdio(t *testing.T) (stdout, stderr func() string) {
	t.Helper()
	return capture(t, &os.Stdout), capture(t, &os.Stderr)
}

func capture(t *testing.T, target **os.File) func() string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	original := *target
	*target = w
	t.Cleanup(func() { *target = original })

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	var read string
	var closed bool
	return func() string {
		if !closed {
			closed = true
			_ = w.Close()
			read = <-done
		}
		return read
	}
}
