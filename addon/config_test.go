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
	defer func() { _ = f.Close() }()

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

// TestAddonManifestDeclaresSettingsAsClosedLists guards D-08-12: the privacy
// profile and log level are App options whose schema is a closed list, so the
// UI cannot offer, and Supervisor cannot accept, a value start-up would refuse.
func TestAddonManifestDeclaresSettingsAsClosedLists(t *testing.T) {
	raw, err := os.ReadFile("config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		"  privacy_profile: mask\n",
		"  log_level: info\n",
		"  privacy_profile: list(mask|allow|deny)\n",
		"  log_level: list(debug|info|warn|error)\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("config.yaml lacks %q", strings.TrimSpace(want))
		}
	}
}

// TestDockerfile_SetsBinaryVersionFromBuildArg guards F-39: the image's binary
// must report the version config.yaml names, so the Dockerfile has to pass a
// build arg into -X main.version rather than leave the 0.0.0-dev default.
func TestDockerfile_SetsBinaryVersionFromBuildArg(t *testing.T) {
	raw, err := os.ReadFile("../Dockerfile")
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	dockerfile := string(raw)
	if !strings.Contains(dockerfile, "ARG VERSION") {
		t.Error("Dockerfile declares no ARG VERSION")
	}
	if !strings.Contains(dockerfile, "-X main.version=${VERSION}") {
		t.Error("Dockerfile does not set -X main.version from the VERSION build arg")
	}
}

// TestReleaseWorkflow_PassesManifestVersionAsBuildArg keeps the single source
// of truth: release.yml feeds config.yaml's version into the image build.
func TestReleaseWorkflow_PassesManifestVersionAsBuildArg(t *testing.T) {
	raw, err := os.ReadFile("../.github/workflows/release.yml")
	if err != nil {
		t.Fatalf("read release.yml: %v", err)
	}
	if got := strings.Count(string(raw), "VERSION=${{ steps.version.outputs.version }}"); got != 2 {
		t.Errorf("release.yml passes the VERSION build arg %d times, want 2 (one per image)", got)
	}
}

// TestAddonManifest_HTTPSecret_IsRequiredPassword guards D-08-9: the client
// secret is a masked, required App option with no default, so a fresh install
// cannot start with a secret nobody chose (start-up refuses, D-08-4).
func TestAddonManifest_HTTPSecret_IsRequiredPassword(t *testing.T) {
	raw, err := os.ReadFile("config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "\n  http_secret: password\n") {
		t.Error("config.yaml schema must declare http_secret as a required password")
	}
	if strings.Contains(optionsBlock(text), "http_secret") {
		t.Error("config.yaml options: must not ship a default http_secret")
	}
}

// optionsBlock returns the text of the top-level options: block.
func optionsBlock(text string) string {
	_, after, ok := strings.Cut(text, "\noptions:\n")
	if !ok {
		return ""
	}
	block, _, _ := strings.Cut(after, "\nschema:")
	return block
}

// TestAddonManifest_Port_ClosedByDefault guards D-08-6: 8790/tcp is declared
// but mapped to null, so nothing is published on the host until the owner
// chooses a port on the Network tab; and the App has no Ingress and no host
// network (D-08-1).
func TestAddonManifest_Port_ClosedByDefault(t *testing.T) {
	raw, err := os.ReadFile("config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "\nports:\n  8790/tcp: null\n") {
		t.Error("config.yaml must map 8790/tcp to null (closed by default)")
	}
	m := parseManifest(t, "config.yaml")
	if _, present := m.scalars["ingress"]; present {
		t.Error("config.yaml must not declare ingress")
	}
	if v := m.scalars["host_network"]; v != "false" {
		t.Errorf("host_network = %q, want false", v)
	}
}

// TestRunScript_SelectsHTTPTransport guards D-08-9: the image fixes the
// transport, because the only other value would make the App exit at once.
func TestRunScript_SelectsHTTPTransport(t *testing.T) {
	raw, err := os.ReadFile("rootfs/run.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "HA_INSPECTOR_TRANSPORT=http") {
		t.Error("run.sh must set HA_INSPECTOR_TRANSPORT=http")
	}
}

// TestAppArmor_NetworkIsStreamOnly guards "accepting on that socket and
// nothing more": TCP over IPv4/IPv6 covers the listener and the Supervisor
// proxy; no blanket, datagram or raw network rule.
func TestAppArmor_NetworkIsStreamOnly(t *testing.T) {
	raw, err := os.ReadFile("apparmor.txt")
	if err != nil {
		t.Fatal(err)
	}
	var rules []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "network") {
			rules = append(rules, line)
		}
	}
	want := []string{"network inet stream,", "network inet6 stream,"}
	if strings.Join(rules, "|") != strings.Join(want, "|") {
		t.Errorf("network rules = %q, want exactly %q", rules, want)
	}
}

// TestAppArmor_AllowsReadingOnlyTheOptionsFile guards the P8-09 live finding:
// the profile denies everything unlisted, so without this rule the binary
// cannot read /data/options.json and refuses to start. Exactly that one file,
// read-only — not /data/**, and never /config (ADR-004).
func TestAppArmor_AllowsReadingOnlyTheOptionsFile(t *testing.T) {
	raw, err := os.ReadFile("apparmor.txt")
	if err != nil {
		t.Fatal(err)
	}
	var dataRules []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "/data") {
			dataRules = append(dataRules, line)
		}
	}
	want := "/data/options.json r,"
	if len(dataRules) != 1 || dataRules[0] != want {
		t.Errorf("/data rules = %q, want exactly [%q]", dataRules, want)
	}
}
