package ha

import "time"

// automationConfigCommand asks automation/config for one automation's stored
// config, by entity_id — the admin-gated source get_automation reads (P3-07;
// docs/research/2026-08-23-ha-automation-traces.md).
type automationConfigCommand struct {
	EntityID string `json:"entity_id"`
}

// CommandType implements Command.
func (automationConfigCommand) CommandType() string { return CommandAutomationConfig }

// traceListCommand asks trace/list for one domain's stored traces, optionally
// filtered to one item — get_automation_traces' admin-gated evidence source
// (P3-07). ItemID is the automation's object id (its entity id with the
// domain and dot stripped), which is how HA's trace store keys automation
// (and script) traces; empty asks for the whole domain.
type traceListCommand struct {
	Domain string `json:"domain"`
	ItemID string `json:"item_id,omitempty"`
}

// CommandType implements Command.
func (traceListCommand) CommandType() string { return CommandTraceList }

// logbookGetEventsCommand asks logbook/get_events for one or more entities
// since a start time — get_automation_traces' non-admin fallback evidence
// (F-11), confirmed reachable at any principal
// (docs/research/2026-08-23-ha-automation-traces.md).
type logbookGetEventsCommand struct {
	StartTime time.Time `json:"start_time"`
	EntityIDs []string  `json:"entity_ids"`
}

// CommandType implements Command.
func (logbookGetEventsCommand) CommandType() string { return CommandLogbookGetEvents }
