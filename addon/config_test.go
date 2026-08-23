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
		"hassio_api":         "false",
		"protection":         "true",
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

	for _, entry := range m.mapList {
		if strings.HasPrefix(entry, "config") {
			t.Errorf("map: must not contain a config entry, found %q", entry)
		}
	}
}
