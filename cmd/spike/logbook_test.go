package main

import (
	"reflect"
	"testing"
)

func TestSummarizeLogbook_CountsHomeAssistantEntries(t *testing.T) {
	decoded := []any{
		map[string]any{"when": 1.0, "name": "Home Assistant", "message": "started", "domain": "homeassistant"},
		map[string]any{"when": 2.0, "name": "Lamp", "state": "on", "entity_id": "light.x"},
		map[string]any{"when": 3.0, "name": "Home Assistant", "message": "stopped", "domain": "homeassistant"},
		map[string]any{"when": 4.0, "name": "Home Assistant", "message": "started", "domain": "homeassistant"},
	}
	got := summarizeLogbook(decoded)
	if got.Entries != 4 || got.HomeAssistant != 3 {
		t.Fatalf("counts = %+v", got)
	}
	if got.Messages["started"] != 2 || got.Messages["stopped"] != 1 {
		t.Fatalf("messages = %v", got.Messages)
	}
	wantKeys := []string{"domain", "entity_id", "message", "name", "state", "when"}
	if !reflect.DeepEqual(got.Keys, wantKeys) {
		t.Fatalf("keys = %v, want %v", got.Keys, wantKeys)
	}
}

func TestSummarizeLogbook_NotAList_ZeroSummary(t *testing.T) {
	if got := summarizeLogbook(map[string]any{"x": 1}); got.Entries != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestSummarizeLogbook_OnlyHomeAssistantMessagesAreKept(t *testing.T) {
	// A message on any other row is the installation's own text; only the
	// system's start/stop vocabulary may reach the report.
	got := summarizeLogbook([]any{map[string]any{"message": "door left open", "domain": "binary_sensor"}})
	if len(got.Messages) != 0 {
		t.Fatalf("leaked message text: %v", got.Messages)
	}
}
