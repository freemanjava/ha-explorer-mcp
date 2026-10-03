package main

import (
	"context"
	"slices"
	"time"

	"github.com/coder/websocket"
)

// logbookWindow is as wide as the analyze_* tools' maximum period: a restart
// the diagnostic could be asked about lies inside it.
const logbookWindow = weekWindow

// homeassistantDomain is the logbook domain HA gives its own lifecycle rows.
const homeassistantDomain = "homeassistant"

type logbookSummary struct {
	Entries       int
	HomeAssistant int
	// Messages counts the text of homeassistant-domain rows only. Rows from
	// any other domain carry the installation's own text, which this report
	// must not copy.
	Messages map[string]int
	// Keys is the union of field names across rows — shape, not content.
	Keys []string
}

// summarizeLogbook reduces a logbook/get_events answer to what F-31 asks:
// are Home Assistant start/stop rows present, and what do they look like.
func summarizeLogbook(decoded any) logbookSummary {
	out := logbookSummary{Messages: map[string]int{}}
	list, ok := decoded.([]any)
	if !ok {
		return out
	}
	keys := map[string]bool{}
	for _, row := range list {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		out.Entries++
		for k := range m {
			keys[k] = true
		}
		if domain, _ := m["domain"].(string); domain == homeassistantDomain {
			out.HomeAssistant++
			if msg, _ := m["message"].(string); msg != "" {
				out.Messages[msg]++
			}
		}
	}
	for k := range keys {
		out.Keys = append(out.Keys, k)
	}
	slices.Sort(out.Keys)
	return out
}

// probeLogbookRestart answers F-31: is "Home Assistant started/stopped" visible
// through logbook/get_events, and does the command answer without entity ids?
// The production command (internal/ha logbookGetEventsCommand) always sends
// entity_ids, so a restart row — which has none — could never be selected by it.
// Both forms are tried so the report says which one carries the rows.
func probeLogbookRestart(ctx context.Context, out *report, conn *websocket.Conn, idSeq *idSeq) {
	out.writef("## WebSocket `logbook/get_events` — HA restart rows (F-31)\n\n")
	end := time.Now().UTC()
	start := end.Add(-logbookWindow)

	for _, variant := range []struct {
		name    string
		payload map[string]any
	}{
		{"no entity_ids", map[string]any{
			"type": "logbook/get_events", "start_time": start.Format(time.RFC3339), "end_time": end.Format(time.RFC3339),
		}},
		{"empty entity_ids", map[string]any{
			"type": "logbook/get_events", "start_time": start.Format(time.RFC3339), "end_time": end.Format(time.RFC3339),
			"entity_ids": []string{},
		}},
	} {
		res, err := wsCall(ctx, conn, idSeq, variant.payload)
		if err != nil {
			out.writef("**%s** — TRANSPORT FAILURE: %v\n\n", variant.name, err)
			continue
		}
		if res.status != "OK" {
			out.writef("**%s** — %s\n\n", variant.name, res.status)
			continue
		}
		s := summarizeLogbook(res.decoded)
		out.writef("**%s** — %d rows in %d bytes (%s) over %s; %d are `%s`-domain rows.\n\n",
			variant.name, s.Entries, res.bytes, res.elapsed, logbookWindow, s.HomeAssistant, homeassistantDomain)
		out.writef("Row fields seen: %v. `%s`-domain messages: %v.\n\n", s.Keys, homeassistantDomain, s.Messages)
	}
}
