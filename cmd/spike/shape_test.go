package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustUnmarshal(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("fixture is not valid JSON: %v", err)
	}
	return v
}

// The probe output is pasted into a chat and committed to docs/research/, so
// the one property that must hold unconditionally is that no value from the
// user's Home Assistant ever reaches it — only field names and their types.
func TestShape_PayloadValues_NeverEmitted(t *testing.T) {
	payload := mustUnmarshal(t, `[
	  {
	    "entity_id": "light.example_room",
	    "name": "Example Room Lamp",
	    "access_token": "eyJhbGciOiJIUzI1NiJ9.SECRETVALUE",
	    "latitude": 12.3456,
	    "disabled_by": null,
	    "options": {"conversation": {"should_expose": true}}
	  }
	]`)

	out := renderShape(shapeOf(payload))

	for _, leak := range []string{
		"light.example_room", "Example Room Lamp",
		"eyJhbGciOiJIUzI1NiJ9", "SECRETVALUE", "12.3456",
	} {
		if strings.Contains(out, leak) {
			t.Errorf("probe output leaked a payload value %q:\n%s", leak, out)
		}
	}

	// Field names are the point of the exercise and must survive.
	for _, want := range []string{"entity_id", "access_token", "disabled_by", "should_expose"} {
		if !strings.Contains(out, want) {
			t.Errorf("probe output dropped field name %q:\n%s", want, out)
		}
	}
}

// Phase 01 maps these payloads into internal/model, so which fields are
// optional and which are nullable is load-bearing evidence, not a nicety.
func TestShape_NullableField_ReportsBothTypes(t *testing.T) {
	payload := mustUnmarshal(t, `[{"area_id": "kitchen"}, {"area_id": null}]`)

	out := renderShape(shapeOf(payload))

	if !strings.Contains(out, "null") || !strings.Contains(out, "string") {
		t.Errorf("nullable field must report both observed types, got:\n%s", out)
	}
}

func TestShape_MissingKey_ReportedAsPartialPresence(t *testing.T) {
	payload := mustUnmarshal(t, `[{"a": 1, "b": 2}, {"a": 1}, {"a": 1}]`)

	out := renderShape(shapeOf(payload))

	if !strings.Contains(out, "1/3") {
		t.Errorf("a key present in only some elements must report its presence count, got:\n%s", out)
	}
	if !strings.Contains(out, "3/3") {
		t.Errorf("a key present in every element must report its presence count, got:\n%s", out)
	}
}

// A registry list is thousands of elements; the report must describe the
// element shape once, not repeat it per element.
func TestShape_Array_MergesElementsIntoOneShape(t *testing.T) {
	payload := mustUnmarshal(t, `[{"a": "x"}, {"a": "y"}, {"a": "z"}]`)

	out := renderShape(shapeOf(payload))

	if n := strings.Count(out, "a:"); n != 1 {
		t.Errorf("expected the element shape once, saw field %d times:\n%s", n, out)
	}
	if !strings.Contains(out, "array[3]") {
		t.Errorf("expected the element count to be reported, got:\n%s", out)
	}
}

// F-9: an object used as a map keyed by id carries values in its keys. HA does
// this in config_entries_subentries, where the keys are config entry ids from
// the owner's installation.
func TestShape_MapKeyedByID_KeysNotEmitted(t *testing.T) {
	payload := mustUnmarshal(t, `{
	  "config_entries_subentries": {
	    "01JATEFWVVF1XWYT5DSEGW97RP": [null],
	    "846552b7b20267edf49f183ba999188d": [null]
	  },
	  "options": {"conversation": {"should_expose": true}}
	}`)

	out := renderShape(shapeOf(payload))

	for _, leak := range []string{"01JATEFWVVF1XWYT5DSEGW97RP", "846552b7b20267edf49f183ba999188d"} {
		if strings.Contains(out, leak) {
			t.Errorf("probe output leaked an id used as an object key %q:\n%s", leak, out)
		}
	}
	if !strings.Contains(out, "<id>") {
		t.Errorf("a map keyed by id must be reported as such, got:\n%s", out)
	}
	// A genuinely schema-shaped object must keep its real key names.
	if !strings.Contains(out, "conversation") || !strings.Contains(out, "should_expose") {
		t.Errorf("schema-shaped object keys must survive, got:\n%s", out)
	}
}

func TestShape_EntityIDKeyedMap_KeysWithheld(t *testing.T) {
	// The shape of history/history_during_period: one entry per entity id.
	payload := map[string]any{
		"sensor.ha_panel1_app_memory": []any{
			map[string]any{"lu": 1.0, "s": "812.4"},
		},
	}

	got := renderShape(shapeOf(payload))

	if strings.Contains(got, "sensor.ha_panel1_app_memory") {
		t.Fatalf("entity id reached the report:\n%s", got)
	}
	if !strings.Contains(got, "map keyed by id") {
		t.Fatalf("entity-keyed object not recognised as a map:\n%s", got)
	}
}
