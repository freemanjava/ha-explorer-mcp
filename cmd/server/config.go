package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"

	"github.com/freemanjava/ha-explorer-mcp/internal/mcp"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
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
	// optionPrivacyProfile and optionLogLevel are closed lists in the App
	// schema (D-08-12), so the UI offers only valid values.
	optionPrivacyProfile = "privacy_profile"
	optionLogLevel       = "log_level"

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

// readOptions returns the App options file's keys, or nil when the file does
// not exist (the development case: environment only). Errors never quote the
// file, which can hold the secret.
func readOptions(optionsFile string) (map[string]json.RawMessage, error) {
	raw, err := os.ReadFile(optionsFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("cannot read the App options file")
	}
	var opts map[string]json.RawMessage
	if err := json.Unmarshal(raw, &opts); err != nil {
		// Not wrapped: a decode error can quote the offending bytes.
		return nil, errors.New("the App options file is not valid JSON")
	}
	if opts == nil {
		opts = map[string]json.RawMessage{}
	}
	return opts, nil
}

// stringOption reads one string key; absent is "", a non-string is an error
// naming the key.
func stringOption(opts map[string]json.RawMessage, key string) (string, error) {
	v, ok := opts[key]
	if !ok {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return "", fmt.Errorf("option %s must be a string", key)
	}
	return s, nil
}

func readSecret(getenv func(string) string, optionsFile string) (string, error) {
	opts, err := readOptions(optionsFile)
	if err != nil {
		return "", err
	}
	if opts == nil {
		return getenv(envHTTPSecret), nil
	}
	return stringOption(opts, optionHTTPSecret)
}

// settings are the operator's two App options (D-08-12). Budget limits are
// deliberately not here: they are the measured constants in internal/policy.
type settings struct {
	profile     policy.Profile
	profileName string // effective, normalized — what the startup log reports
	level       slog.Level
}

// loadSettings reads the privacy profile and log level under D-08-9's
// one-source rule: the options file when it exists (its absent keys take the
// defaults, the environment is ignored), else the environment. An unknown
// value refuses start-up, naming the key.
func loadSettings(getenv func(string) string, optionsFile string) (settings, error) {
	opts, err := readOptions(optionsFile)
	if err != nil {
		return settings{}, err
	}
	profileKey, levelKey := envPrivacyProfile, envLogLevel
	profileRaw, levelRaw := getenv(envPrivacyProfile), getenv(envLogLevel)
	if opts != nil {
		profileKey, levelKey = optionPrivacyProfile, optionLogLevel
		if profileRaw, err = stringOption(opts, profileKey); err != nil {
			return settings{}, err
		}
		if levelRaw, err = stringOption(opts, levelKey); err != nil {
			return settings{}, err
		}
	}

	profile, err := policy.NewProfile(profileRaw)
	if err != nil {
		return settings{}, fmt.Errorf("unknown %s: want mask, allow or deny", profileKey)
	}
	level, err := logLevel(levelRaw)
	if err != nil {
		return settings{}, fmt.Errorf("unknown %s: want debug, info, warn or error", levelKey)
	}
	name := strings.ToLower(strings.TrimSpace(profileRaw))
	if name == "" {
		name = "mask"
	}
	return settings{profile: profile, profileName: name, level: level}, nil
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
		return 0, fmt.Errorf("unknown log level %q", name)
	}
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
