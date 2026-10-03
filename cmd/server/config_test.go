package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// Built at runtime, low-entropy, so the secret scanner has no literal to flag.
var goodSecret = strings.Repeat("test-secret-", 3)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func writeOptions(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "options.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadTransport_DefaultsToStdio(t *testing.T) {
	cfg, err := loadTransport(env(nil), filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("loadTransport: %v", err)
	}
	if cfg.transport != "stdio" || cfg.httpSecret != "" {
		t.Errorf("cfg = %+v, want stdio with no secret", cfg)
	}
}

func TestLoadTransport_UnknownTransport_Refused(t *testing.T) {
	_, err := loadTransport(env(map[string]string{envTransport: "websocket"}), "")
	if err == nil || !strings.Contains(err.Error(), envTransport) {
		t.Fatalf("err = %v, want a refusal naming %s", err, envTransport)
	}
}

func TestLoadTransport_HTTPSecretRules_Refused(t *testing.T) {
	cases := map[string]string{
		"absent":    "",
		"too short": strings.Repeat("a", minSecretLen-1),
		"too long":  strings.Repeat("a", maxSecretLen+1),
		"space":     strings.Repeat("a", 20) + " " + strings.Repeat("b", 20),
		"control":   strings.Repeat("a", 40) + "\n",
		"non-ascii": strings.Repeat("é", 40),
	}
	for name, secret := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := loadTransport(env(map[string]string{envTransport: "http", envHTTPSecret: secret}), filepath.Join(t.TempDir(), "absent.json"))
			if err == nil {
				t.Fatal("start was not refused")
			}
			if !strings.Contains(err.Error(), "http_secret") {
				t.Errorf("error %q does not name http_secret", err)
			}
			if secret != "" && strings.Contains(err.Error(), secret) {
				t.Errorf("error echoes the secret value")
			}
		})
	}
}

func TestLoadTransport_SecretFromOptionsFile_EnvIgnored(t *testing.T) {
	path := writeOptions(t, `{"http_secret":"`+goodSecret+`"}`)
	cfg, err := loadTransport(env(map[string]string{envTransport: "http", envHTTPSecret: strings.Repeat("env-ignored-", 4)}), path)
	if err != nil {
		t.Fatalf("loadTransport: %v", err)
	}
	if cfg.httpSecret != goodSecret {
		t.Error("the secret did not come from the options file")
	}
}

// One source per run: a present options file without a valid secret is
// refused even when the environment carries a good one (D-08-9).
func TestLoadTransport_OptionsFileWithoutSecret_RefusedDespiteEnv(t *testing.T) {
	path := writeOptions(t, `{}`)
	_, err := loadTransport(env(map[string]string{envTransport: "http", envHTTPSecret: goodSecret}), path)
	if err == nil {
		t.Fatal("the environment secret was used although the options file exists")
	}
}

func TestLoadTransport_MalformedOptionsFile_Refused(t *testing.T) {
	path := writeOptions(t, `{"http_secret": "`+goodSecret)
	_, err := loadTransport(env(map[string]string{envTransport: "http"}), path)
	if err == nil {
		t.Fatal("a malformed options file did not refuse start")
	}
	if strings.Contains(err.Error(), goodSecret) {
		t.Error("error echoes the secret value")
	}
}

func TestLoadTransport_SecretFromEnv_WhenNoOptionsFile(t *testing.T) {
	cfg, err := loadTransport(env(map[string]string{envTransport: "http", envHTTPSecret: goodSecret}), filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("loadTransport: %v", err)
	}
	if cfg.transport != "http" || cfg.httpSecret != goodSecret {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestLoadSettings_OptionsFile_WinsOverEnvironment(t *testing.T) {
	p := writeOptions(t, `{"privacy_profile":"deny","log_level":"debug"}`)
	got, err := loadSettings(env(map[string]string{
		envPrivacyProfile: "allow", envLogLevel: "error",
	}), p)
	if err != nil {
		t.Fatalf("loadSettings: %v", err)
	}
	if got.profileName != "deny" || got.level != slog.LevelDebug {
		t.Errorf("got %q / %v, want deny / debug", got.profileName, got.level)
	}
	if got.profile.Private != policy.HandlingDeny {
		t.Errorf("profile handling = %v, want deny", got.profile.Private)
	}
}

func TestLoadSettings_OptionsFileWithoutKeys_Defaults(t *testing.T) {
	p := writeOptions(t, `{}`)
	got, err := loadSettings(env(map[string]string{envPrivacyProfile: "deny", envLogLevel: "debug"}), p)
	if err != nil {
		t.Fatalf("loadSettings: %v", err)
	}
	if got.profileName != "mask" || got.level != slog.LevelInfo {
		t.Errorf("got %q / %v, want mask / info (environment must be ignored)", got.profileName, got.level)
	}
}

func TestLoadSettings_UnknownValue_RefusedNamingKey(t *testing.T) {
	cases := map[string]string{
		`{"privacy_profile":"open"}`: optionPrivacyProfile,
		`{"log_level":"trace"}`:      optionLogLevel,
		`{"log_level":3}`:            optionLogLevel,
	}
	for body, key := range cases {
		_, err := loadSettings(env(nil), writeOptions(t, body))
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s: err = %v, want a refusal naming %s", body, err, key)
		}
	}
}

func TestLoadSettings_NoOptionsFile_EnvironmentBehaviorUnchanged(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "absent.json")
	got, err := loadSettings(env(map[string]string{envPrivacyProfile: "ALLOW", envLogLevel: "warn"}), absent)
	if err != nil {
		t.Fatalf("loadSettings: %v", err)
	}
	if got.profileName != "allow" || got.level != slog.LevelWarn {
		t.Errorf("got %q / %v, want allow / warn", got.profileName, got.level)
	}

	def, err := loadSettings(env(nil), absent)
	if err != nil || def.profileName != "mask" || def.level != slog.LevelInfo {
		t.Errorf("defaults = %+v, %v, want mask / info", def, err)
	}

	_, err = loadSettings(env(map[string]string{envPrivacyProfile: "open"}), absent)
	if err == nil || !strings.Contains(err.Error(), envPrivacyProfile) {
		t.Errorf("err = %v, want a refusal naming %s", err, envPrivacyProfile)
	}
}
