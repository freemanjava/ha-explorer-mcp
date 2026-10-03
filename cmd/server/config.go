package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/freemanjava/ha-explorer-mcp/internal/mcp"
)

const (
	// envTransport selects stdio (default) or http. The App image fixes http
	// in run.sh; there is no App option for it, because the only other value
	// would make the App exit at once (D-08-9).
	envTransport = "HA_INSPECTOR_TRANSPORT"
	// envHTTPSecret supplies the client secret in development, when no App
	// options file exists.
	envHTTPSecret = "HA_INSPECTOR_HTTP_SECRET"

	// optionsPath is where Supervisor writes the App's options. A fixed
	// constant, not configurable — and not /config (ADR-004).
	optionsPath = "/data/options.json"
	// optionHTTPSecret is the options-file key, declared a password in the
	// App schema so the UI masks it.
	optionHTTPSecret = "http_secret"

	// Secret rules (D-08-4): 32 is the floor of `openssl rand -hex 16`; the
	// ceiling and printable-ASCII-without-spaces keep it header-safe.
	minSecretLen = 32
	maxSecretLen = 256
)

// transportConfig is what start-up reads to decide how to serve.
type transportConfig struct {
	transport  string
	httpSecret string
}

// loadTransport reads the transport and, for http, its secret. The secret
// has one source per run, never merged: the options file when it exists,
// else the environment (D-08-9). Errors name the option and the rule, never a
// value or its length (CLAUDE.md rule 4).
func loadTransport(getenv func(string) string, optionsFile string) (transportConfig, error) {
	name := strings.ToLower(strings.TrimSpace(getenv(envTransport)))
	switch name {
	case "", mcp.TransportStdio:
		return transportConfig{transport: mcp.TransportStdio}, nil
	case mcp.TransportHTTP:
	default:
		return transportConfig{}, fmt.Errorf("unknown %s: want stdio or http", envTransport)
	}

	secret, err := readSecret(getenv, optionsFile)
	if err != nil {
		return transportConfig{}, err
	}
	if err := validateSecret(secret); err != nil {
		return transportConfig{}, err
	}
	return transportConfig{transport: mcp.TransportHTTP, httpSecret: secret}, nil
}

func readSecret(getenv func(string) string, optionsFile string) (string, error) {
	raw, err := os.ReadFile(optionsFile)
	if errors.Is(err, fs.ErrNotExist) {
		return getenv(envHTTPSecret), nil
	}
	if err != nil {
		return "", errors.New("cannot read the App options file")
	}
	var opts map[string]json.RawMessage
	if err := json.Unmarshal(raw, &opts); err != nil {
		// Not wrapped: a decode error can quote the offending bytes.
		return "", errors.New("the App options file is not valid JSON")
	}
	var secret string
	if v, ok := opts[optionHTTPSecret]; ok {
		if err := json.Unmarshal(v, &secret); err != nil {
			return "", fmt.Errorf("option %s must be a string", optionHTTPSecret)
		}
	}
	return secret, nil
}

func validateSecret(secret string) error {
	if secret == "" {
		return fmt.Errorf("transport http requires the %s option", optionHTTPSecret)
	}
	if len(secret) < minSecretLen || len(secret) > maxSecretLen || !printableASCII(secret) {
		return fmt.Errorf("option %s must be %d–%d printable ASCII characters without spaces",
			optionHTTPSecret, minSecretLen, maxSecretLen)
	}
	return nil
}

func printableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] <= ' ' || s[i] > '~' {
			return false
		}
	}
	return true
}
