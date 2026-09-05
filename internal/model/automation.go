package model

import "time"

// Automation is get_automation's response: the normalized view of one Home
// Assistant automation, read through the admin-gated automation/config
// command (CLAUDE.md rule 6: an automation's trigger/condition/action bodies
// are HA data and are counted here, never parsed for their content).
//
// Unsupported is distinct from Provenance.Partial, the same way
// model.SystemHealth's is: Partial means automation/config answered but one
// field could not be mapped, Unsupported means automation/config could not be
// asked at all — refused by permission, or absent on the detected HA version
// (P3-07 DoD; F-11) — and UnsupportedReason names which and, for the
// permission case, the non-admin fallback (list_automations).
type Automation struct {
	Source     string
	ObservedAt time.Time

	Unsupported       bool
	UnsupportedReason string

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

// AutomationTraceSummary is one run's execution outcome, from trace/list —
// the admin-gated index get_automation_traces reads (P3-07). trace/get's full
// per-step detail (trigger/condition/action bodies, and the whole entity
// states F-12 found embedded in it) is out of this tool's scope: every field
// the detection layer needs to say "did this run fail, and how" is already in
// the index (docs/research/2026-08-23-ha-automation-traces.md).
type AutomationTraceSummary struct {
	RunID           string
	State           string
	ScriptExecution string
	LastStep        string
	Trigger         string
	TimestampStart  time.Time
	TimestampFinish time.Time

	Provenance
}

// LogbookEvent is one logbook/get_events entry — get_automation_traces'
// non-admin fallback evidence (F-11), reachable at any principal
// (docs/research/2026-08-23-ha-automation-traces.md). ContextID is the same
// id trace/contexts indexes, so a run can still be correlated to its logbook
// entries even where the trace behind it cannot be read.
type LogbookEvent struct {
	When      time.Time
	Name      string
	Message   string
	EntityID  EntityID
	ContextID string

	Provenance
}

// AutomationTraceList is get_automation_traces' response: a page of
// AutomationTraceSummary plus the cursor-pagination envelope every list-
// shaped tool shares.
//
// Unsupported carries the same three-way meaning as model.Automation's: not
// asked because Home Assistant refused it to this principal (permission), or
// because the detected HA version does not offer trace/list at all — versus a
// well-formed answer that happens to hold zero runs, which is Items being
// empty with Unsupported false (CLAUDE.md rule 7: an empty list means "none",
// never "could not check"). FallbackLastTriggered/FallbackEvents are
// populated only in the permission-refused case (F-11's degraded mode): the
// version-absent case has no fallback worth fetching, since an HA release
// that dropped trace/list did not thereby change last_triggered/logbook.
type AutomationTraceList struct {
	Source     string
	ObservedAt time.Time

	Unsupported       bool
	UnsupportedReason string

	FallbackLastTriggered *time.Time
	FallbackEvents        []LogbookEvent

	Items        []AutomationTraceSummary
	NextCursor   string
	Truncated    bool
	LimitClamped bool

	Provenance
}
