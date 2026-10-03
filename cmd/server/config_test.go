package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
