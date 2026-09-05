package model

import "time"

// Automation is the normalized view of one Home Assistant automation, read
// through the admin-gated automation/config command (CLAUDE.md rule 6: an
// automation's trigger/condition/action bodies are HA data and are counted
// here, never parsed for their content).
type Automation struct {
	EntityID EntityID
	ID       string
	Alias    string

	Mode           string
	TriggerCount   int
	ConditionCount int
	ActionCount    int

	Provenance
}

// AutomationSummary is one list_automations row, derived from get_states —
// the confirmed non-admin fallback source
// (docs/research/2026-08-23-ha-automation-traces.md): enabled state and
// last_triggered, not automation/config's full detail, which get_automation
// (P3-07) reaches through the admin-gated API instead.
type AutomationSummary struct {
	EntityID EntityID
	Alias    string

	Enabled       bool
	LastTriggered *time.Time
	Mode          string
	CurrentRuns   int

	Provenance
}

// AutomationList is list_automations' page: automation summaries plus the
// cursor-pagination envelope every list_* tool shares (doc §9.1).
type AutomationList struct {
	Source     string
	ObservedAt time.Time

	Items        []AutomationSummary
	NextCursor   string
	Truncated    bool
	LimitClamped bool

	Provenance
}
