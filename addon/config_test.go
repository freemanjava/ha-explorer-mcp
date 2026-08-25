// Package addon holds the Home Assistant App packaging skeleton:
// config.yaml, build.yaml, the Dockerfile, the run script and the AppArmor
// profile. This test guards the manifest's security posture so a future edit
// cannot quietly grant filesystem or Docker access — see CLAUDE.md's
// "Rules That Are Not Negotiable Here".
package addon

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// manifest is a minimal flat view of config.yaml sufficient for the security
// assertions below. The full App manifest schema is not this project's
// concern to parse — only that the forbidden keys stay absent-or-false.
type manifest struct {
	scalars map[string]string
	mapList []string
}

func parseManifest(t *testing.T, path string) manifest {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	m := manifest{scalars: map[string]string{}}
	inMapBlock := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if inMapBlock {
			if strings.HasPrefix(line, "  - ") {
				m.mapList = append(m.mapList, strings.TrimSpace(strings.TrimPrefix(line, "  -")))
				continue
			}
			inMapBlock = false
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if key == "map" {
			if value == "[]" || value == "" {
				if value == "" {
					inMapBlock = true
				}
				continue
			}
		}

		m.scalars[key] = strings.Trim(value, `"`)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return m
}

func TestAddonManifestSecurityPosture(t *testing.T) {
	m := parseManifest(t, "config.yaml")

	forbidden := []string{"docker_api", "host_network", "full_access"}
	for _, key := range forbidden {
		if v, present := m.scalars[key]; present && v != "false" {
			t.Errorf("forbidden key %q must be absent or false, got %q", key, v)
		}
	}

	required := map[string]string{
		"homeassistant_api": "true",
		"hassio_api":        "true",
		"protection":        "true",
	}
	for key, want := range required {
		got, present := m.scalars[key]
		if !present {
			t.Errorf("required key %q is missing", key)
			continue
		}
		if got != want {
			t.Errorf("key %q = %q, want %q", key, got, want)
		}
	}

	// hassio_role must stay unset (the default role) — a role above default
	// would grant broad /core/.+ or /host/.+ write access, which doc §15.2
	// rules out for observer v1 (phase 00 "Supervisor permission level"
	// decision, 2026-08-25).
	if role, present := m.scalars["hassio_role"]; present {
		t.Errorf("hassio_role must stay unset (the default role), got %q", role)
	}

	for _, entry := range m.mapList {
		if strings.HasPrefix(entry, "config") {
			t.Errorf("map: must not contain a config entry, found %q", entry)
		}
	}
}

// TestAddonManifestImageIsPinnedToVersion guards the App-distribution decision
// (phases/00-spike-foundations.md, "App distribution"): the App ships as a
// published image, and config.yaml's version: is the single source of truth
// for the tag Supervisor pulls — see CLAUDE.md's "API & DTO Design" on not
// writing the same fact twice.
func TestAddonManifestImageIsPinnedToVersion(t *testing.T) {
	m := parseManifest(t, "config.yaml")

	image, ok := m.scalars["image"]
	if !ok || image == "" {
		t.Fatal("config.yaml must set image:")
	}
	if !strings.Contains(image, "{arch}") {
		t.Errorf("image %q must contain the {arch} placeholder so Supervisor substitutes the App's architecture (aarch64/amd64)", image)
	}

	version, ok := m.scalars["version"]
	if !ok || version == "" {
		t.Fatal("config.yaml must set version:")
	}

	workflow, err := os.ReadFile("../.github/workflows/release.yml")
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}
	wf := string(workflow)

	if !strings.Contains(wf, "config.yaml") {
		t.Error("release workflow must derive the image tag from addon/config.yaml's version:, not a separately maintained value")
	}
	if strings.Contains(wf, ":"+version) {
		t.Errorf("release workflow must not hardcode the current version %q as a literal image tag — a version bump that forgets to update it must fail the build instead of leaving Supervisor pulling a stale image", version)
	}
}

// TestAddonLocalBuildPathRemoved guards the other half of the same decision:
// Supervisor never builds this App locally, so the local-build files must not
// come back.
func TestAddonLocalBuildPathRemoved(t *testing.T) {
	for _, p := range []string{"Dockerfile", "build.yaml"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("addon/%s must not exist — the App ships as a published image, Supervisor never builds it locally", p)
		}
	}
}
