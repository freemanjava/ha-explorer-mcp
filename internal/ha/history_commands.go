package ha

import "time"

// historyDuringPeriodCommand asks history/history_during_period for one
// entity's recorded states over an explicit window — get_entity_history's
// source (P4-01), preferred over the REST fallback for both size and latency
// (P0-07; docs/research/2026-08-23-ha-history-statistics.md).
//
// MinimalResponse and NoAttributes always travel together: P0-07 found the
// combination is what matters (4.9x smaller, consistently faster), and
// Appendix A.2 exposes a single `minimal` toggle to the tool caller, not the
// two flags independently. SignificantChangesOnly is left false: Appendix
// A.2 does not expose it, and doc §9.1 wants raw, unfiltered points for a
// focused window.
type historyDuringPeriodCommand struct {
	StartTime              time.Time `json:"start_time"`
	EndTime                time.Time `json:"end_time"`
	EntityIDs              []string  `json:"entity_ids"`
	MinimalResponse        bool      `json:"minimal_response"`
	NoAttributes           bool      `json:"no_attributes"`
	SignificantChangesOnly bool      `json:"significant_changes_only"`
}

// CommandType implements Command.
func (historyDuringPeriodCommand) CommandType() string { return CommandHistoryDuringPeriod }
