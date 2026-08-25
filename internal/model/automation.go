package model

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
