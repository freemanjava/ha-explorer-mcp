package redact

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// liveToken stands in for SUPERVISOR_TOKEN: the one value that must never
// cross the response boundary by any route (CLAUDE.md rule 4).
const liveToken = "eyJhbGciOiJIUzI1NiJ9.supervisor-live-token.PLANTED"

func TestSupervisorTokenNeverReturned(t *testing.T) {
	r := New(policy.Profile{}, liveToken)

	// Route 1 and 2: an entity attribute and a device name — the token as a
	// value under a key nothing classifies as secret.
	payload := map[string]any{
		"result": []any{
			map[string]any{
				"entity_id":  "sensor.hallway_temperature",
				"state":      "21.4",
				"attributes": map[string]any{"friendly_name": "Hallway " + liveToken},
			},
			map[string]any{
				"device_id":    "abc123",
				"name_by_user": "Pi running " + liveToken,
			},
		},
	}
	rendered := render(t, r.Payload(payload).Value)
	if strings.Contains(rendered, liveToken) {
		t.Errorf("token survived into the rendered payload: %s", rendered)
	}

	// Route 3: an error string, which reaches the agent as a tool error.
	wrapped := r.Error(errors.New("upstream refused Authorization=" + liveToken))
	if strings.Contains(wrapped.Error(), liveToken) {
		t.Errorf("token survived into the error string: %s", wrapped.Error())
	}

	// Route 4: a log line rendered into a response.
	line := r.Text("connecting with token " + liveToken + " to supervisor")
	if strings.Contains(line, liveToken) {
		t.Errorf("token survived into the log line: %s", line)
	}
}

func TestError_ScrubbedMessageKeepsSentinelComparable(t *testing.T) {
	r := New(policy.Profile{}, liveToken)
	sentinel := errors.New("upstream unavailable")
	got := r.Error(errors.New("dial failed with " + liveToken + ": " + sentinel.Error()))
	if strings.Contains(got.Error(), liveToken) {
		t.Fatalf("token in error: %s", got.Error())
	}

	wrappedSentinel := r.Error(&wrapErr{msg: "auth " + liveToken, err: sentinel})
	if !errors.Is(wrappedSentinel, sentinel) {
		t.Error("scrubbing broke errors.Is: the response must still distinguish error classes")
	}
}

type wrapErr struct {
	msg string
	err error
}

func (e *wrapErr) Error() string { return e.msg }
func (e *wrapErr) Unwrap() error { return e.err }

func TestPayload_SecretKeyFragments_RedactedCaseInsensitively(t *testing.T) {
	r := New(policy.Profile{})
	payload := map[string]any{
		"Token":         "aaa",
		"user_PASSWORD": "bbb",
		"client_Secret": "ccc",
		"API_Key":       "ddd",
		"credentialS":   "eee",
		"Authorization": "fff",
	}
	res := r.Payload(payload)
	rendered := render(t, res.Value)
	for _, v := range []string{"aaa", "bbb", "ccc", "ddd", "eee", "fff"} {
		if strings.Contains(rendered, v) {
			t.Errorf("secret value %q survived: %s", v, rendered)
		}
	}
	if got := len(res.Markers); got != len(payload) {
		t.Errorf("markers = %d, want %d — a redacted field must be marked, not silently dropped", got, len(payload))
	}
	for _, m := range res.Markers {
		if m.Action != policy.ActionRedact {
			t.Errorf("marker %s action = %v, want redact", m.Path, m.Action)
		}
	}
}

func TestPayload_RedactedFieldIsMarkedNotDropped(t *testing.T) {
	r := New(policy.Profile{})
	res := r.Payload(map[string]any{"api_key": "abc"})
	m, ok := res.Value.(map[string]any)
	if !ok {
		t.Fatalf("value = %T, want map", res.Value)
	}
	v, present := m["api_key"]
	if !present {
		t.Fatal("the key was dropped; the agent must be able to see something was withheld")
	}
	if v != RedactedPlaceholder {
		t.Errorf("api_key = %v, want %q", v, RedactedPlaceholder)
	}
}

func TestPayload_NestedTraceSecretsAndUserID_RedactedAtDepth(t *testing.T) {
	// F-12: sensitivity travels with what a payload embeds. The planted
	// secrets and the context.user_id sit inside
	// trace["trigger/1"][0].changed_variables, six levels down.
	payload := readFixture(t, "automation_trace_secrets.json")
	res := New(policy.Profile{}).Payload(payload)
	rendered := render(t, res.Value)

	for _, planted := range []string{
		"planted-api-key-0000",
		"planted-access-token-0000",
		"5f2a91c04b7e4d3f8a6c1e0d9b3a7c25", // context.user_id
	} {
		if strings.Contains(rendered, planted) {
			t.Errorf("planted value %q survived the nested walk", planted)
		}
	}
	// The diagnostic shape survives: paths, timestamps and results are what
	// the trace is read for.
	for _, kept := range []string{"trigger/1", "2026-08-22T19:04:11.512345+00:00", "light.turn_on"} {
		if !strings.Contains(rendered, kept) {
			t.Errorf("diagnostic value %q was lost", kept)
		}
	}
	if !hasMarkerUnder(res.Markers, "trace.trigger/1[0].changed_variables") {
		t.Errorf("no marker recorded at trace depth; markers = %+v", res.Markers)
	}
}

func TestPayload_PrivateHistory_MaskedStatesKeepTimestampsAndTransitions(t *testing.T) {
	history := personHistory()
	res := New(policy.Profile{}).Payload(history)

	states := res.Value.(map[string]any)["person.owner"].([]any)
	input := history["person.owner"].([]any)
	if len(states) != len(input) {
		t.Fatalf("history length = %d, want %d", len(states), len(input))
	}

	var tokens []string
	for i, s := range states {
		got := s.(map[string]any)
		want := input[i].(map[string]any)
		if got["last_changed"] != want["last_changed"] {
			t.Errorf("state %d last_changed = %v, want %v", i, got["last_changed"], want["last_changed"])
		}
		state, ok := got["state"].(string)
		if !ok {
			t.Fatalf("state %d is %T, want a masked token string", i, got["state"])
		}
		if state == want["state"] {
			t.Errorf("state %d came back unmasked: %q", i, state)
		}
		if !isMasked(state) {
			t.Errorf("state %d = %q, which is not visibly marked masked", i, state)
		}
		tokens = append(tokens, state)
	}

	// Stable within one response: the same underlying state is the same
	// token, or transition counting becomes unreadable. home,not_home,home,home
	if tokens[0] != tokens[2] || tokens[0] != tokens[3] {
		t.Errorf("equal states got different tokens: %v", tokens)
	}
	if tokens[0] == tokens[1] {
		t.Errorf("different states collapsed to one token: %v", tokens)
	}
	if got := transitions(tokens); got != transitions([]string{"home", "not_home", "home", "home"}) {
		t.Errorf("transition count = %d, want 2", got)
	}
}

func TestPayload_MaskTokens_ScopedPerEntity(t *testing.T) {
	// Two private entities sharing a token would say their states agree —
	// the meaning withheld, the correlation handed over anyway. Stability is
	// a property of one entity's timeline, not of the response.
	res := New(policy.Profile{}).Payload(map[string]any{
		"person.owner": []any{map[string]any{
			"entity_id": "person.owner", "state": "home", "last_changed": "t1",
		}},
		"person.partner": []any{map[string]any{
			"entity_id": "person.partner", "state": "home", "last_changed": "t1",
		}},
	})
	m := res.Value.(map[string]any)
	a := m["person.owner"].([]any)[0].(map[string]any)["state"]
	b := m["person.partner"].([]any)[0].(map[string]any)["state"]
	if a == b {
		t.Errorf("two entities in the same state share token %v", a)
	}
}

func TestPayload_MaskTokens_DifferAcrossResponses(t *testing.T) {
	// Stable across responses would make the token a de-facto identifier
	// that leaks the value by correlation (PRIVATE-handling decision).
	first := New(policy.Profile{}).Payload(personHistory())
	second := New(policy.Profile{}).Payload(personHistory())

	a := first.Value.(map[string]any)["person.owner"].([]any)[0].(map[string]any)["state"]
	b := second.Value.(map[string]any)["person.owner"].([]any)[0].(map[string]any)["state"]
	if a == b {
		t.Errorf("token %v is stable across responses", a)
	}
}

func TestPayload_OccupancyDeviceClass_StateMasked(t *testing.T) {
	// A door contact and a power meter share the binary_sensor domain; the
	// device class is what separates them.
	res := New(policy.Profile{}).Payload(map[string]any{
		"binary_sensor.hall": []any{
			map[string]any{
				"entity_id":    "binary_sensor.hall",
				"state":        "on",
				"attributes":   map[string]any{"device_class": "occupancy"},
				"last_changed": "2026-08-22T19:00:00+00:00",
			},
		},
	})
	got := res.Value.(map[string]any)["binary_sensor.hall"].([]any)[0].(map[string]any)["state"].(string)
	if !isMasked(got) {
		t.Errorf("occupancy state = %q, want a masked token", got)
	}
}

func TestPayload_AllowProfile_PrivateUntouched(t *testing.T) {
	p, err := policy.NewProfile("allow")
	if err != nil {
		t.Fatal(err)
	}
	res := New(p).Payload(personHistory())
	got := res.Value.(map[string]any)["person.owner"].([]any)[0].(map[string]any)["state"]
	if got != "home" {
		t.Errorf("state = %v, want home under the allow profile", got)
	}
}

func TestPayload_DenyProfile_PrivateWithheldAndMarked(t *testing.T) {
	p, err := policy.NewProfile("deny")
	if err != nil {
		t.Fatal(err)
	}
	res := New(p).Payload(personHistory())
	got := res.Value.(map[string]any)["person.owner"].([]any)[0].(map[string]any)["state"]
	if got != DeniedPlaceholder {
		t.Errorf("state = %v, want %q under the deny profile", got, DeniedPlaceholder)
	}
}

func TestPayload_MaskedAndRedactedAreDistinguishable(t *testing.T) {
	res := New(policy.Profile{}).Payload(map[string]any{
		"entity_id": "person.owner",
		"state":     "home",
		"api_key":   "abc",
	})
	m := res.Value.(map[string]any)
	masked, _ := m["state"].(string)
	redacted, _ := m["api_key"].(string)
	if masked == redacted {
		t.Fatalf("masked and redacted render identically as %q", masked)
	}
	if !isMasked(masked) || redacted != RedactedPlaceholder {
		t.Errorf("state = %q, api_key = %q — each must carry its own marker", masked, redacted)
	}

	byPath := map[string]policy.Action{}
	for _, mk := range res.Markers {
		byPath[mk.Path] = mk.Action
	}
	if byPath["state"] != policy.ActionMask || byPath["api_key"] != policy.ActionRedact {
		t.Errorf("markers = %+v, want state masked and api_key redacted", res.Markers)
	}
}

func TestConfig_CoordinatesCoarsened_LocationNameUntouched(t *testing.T) {
	cfg := map[string]any{
		"latitude":      52.370216,
		"longitude":     4.895168,
		"elevation":     float64(3),
		"location_name": "Home",
		"time_zone":     "Europe/Amsterdam",
	}
	res := New(policy.Profile{}).Config(cfg)
	m := res.Value.(map[string]any)

	if got := m["latitude"]; got != 52.4 {
		t.Errorf("latitude = %v, want 52.4 (one decimal)", got)
	}
	if got := m["longitude"]; got != 4.9 {
		t.Errorf("longitude = %v, want 4.9 (one decimal)", got)
	}
	if got := m["location_name"]; got != "Home" {
		t.Errorf("location_name = %v, want Home untouched", got)
	}
	if got := m["time_zone"]; got != "Europe/Amsterdam" {
		t.Errorf("time_zone = %v, want untouched", got)
	}
	var coarsened int
	for _, mk := range res.Markers {
		if mk.Action == policy.ActionMask {
			coarsened++
		}
	}
	if coarsened != 2 {
		t.Errorf("mask markers = %d, want 2 (latitude, longitude)", coarsened)
	}
}

func TestConfig_DenyProfile_CoordinatesWithheld(t *testing.T) {
	p, err := policy.NewProfile("deny")
	if err != nil {
		t.Fatal(err)
	}
	res := New(p).Config(map[string]any{"latitude": 52.370216, "location_name": "Home"})
	if got := res.Value.(map[string]any)["latitude"]; got != DeniedPlaceholder {
		t.Errorf("latitude = %v, want %q", got, DeniedPlaceholder)
	}
}

func TestPayload_InputNotMutated(t *testing.T) {
	// Domain values crossing a layer are values, not mutable shared
	// structures (CLAUDE.md, Immutability at boundaries).
	in := map[string]any{"api_key": "abc", "nested": map[string]any{"password": "def"}}
	New(policy.Profile{}).Payload(in)
	if in["api_key"] != "abc" || in["nested"].(map[string]any)["password"] != "def" {
		t.Errorf("input was mutated: %+v", in)
	}
}

func TestPayload_OverDeepPayload_FailsClosed(t *testing.T) {
	// HA data is untrusted (rule 6): a pathological nesting must not recurse
	// without bound, and what the walk declined to enter must not be
	// reported as safe.
	deep := any("bottom")
	for range 400 {
		deep = map[string]any{"n": deep}
	}
	res := New(policy.Profile{}).Payload(deep)
	if !strings.Contains(render(t, res.Value), RedactedPlaceholder) {
		t.Error("over-deep payload passed through without being cut")
	}
}

func TestPayload_NoSecrets_NoMarkers(t *testing.T) {
	// An empty marker list means "nothing was withheld", never "could not
	// check" (rule 7).
	res := New(policy.Profile{}).Payload(map[string]any{
		"entity_id": "sensor.cpu",
		"state":     "12.5",
	})
	if len(res.Markers) != 0 {
		t.Errorf("markers = %+v, want none", res.Markers)
	}
}

func TestLogHandler_TokenScrubbedFromMessageAndAttrs(t *testing.T) {
	var buf strings.Builder
	log := slog.New(NewLogHandler(slog.NewTextHandler(&buf, nil), liveToken))
	log.Info("connecting to supervisor", "url", "http://supervisor/core/api", "auth", "Bearer "+liveToken)
	log.Error("auth failed for " + liveToken)
	if strings.Contains(buf.String(), liveToken) {
		t.Errorf("token survived into the log output: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "connecting to supervisor") {
		t.Errorf("scrubbing destroyed the log message: %s", buf.String())
	}
}

// --- helpers ---

var maskedPattern = regexp.MustCompile(`^\[masked:[a-z_]+_[0-9a-f]{4}[A-Z]+\]$`)

func isMasked(s string) bool { return maskedPattern.MatchString(s) }

func hasMarkerUnder(markers []Marker, prefix string) bool {
	for _, m := range markers {
		if strings.HasPrefix(m.Path, prefix) {
			return true
		}
	}
	return false
}

func transitions(states []string) int {
	n := 0
	for i := 1; i < len(states); i++ {
		if states[i] != states[i-1] {
			n++
		}
	}
	return n
}

func personHistory() map[string]any {
	state := func(s, ts string) map[string]any {
		return map[string]any{
			"entity_id":    "person.owner",
			"state":        s,
			"last_changed": ts,
			"attributes":   map[string]any{"friendly_name": "Owner"},
		}
	}
	return map[string]any{
		"person.owner": []any{
			state("home", "2026-08-22T08:00:00+00:00"),
			state("not_home", "2026-08-22T09:15:00+00:00"),
			state("home", "2026-08-22T17:40:00+00:00"),
			state("home", "2026-08-22T18:00:00+00:00"),
		},
	}
}

func render(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal redacted payload: %v", err)
	}
	return string(data)
}

func readFixture(t *testing.T, name string) any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("unmarshal fixture %s: %v", name, err)
	}
	return v
}
