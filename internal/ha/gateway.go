package ha

import "fmt"

// The WebSocket commands this binary is permitted to send. Every entry was
// observed answering successfully against live HA 2026.8.3 by a Phase 00
// probe; nothing is listed on the strength of the architecture doc's
// illustrative lists alone (P1-02, phase 01 "Depends On").
//
// Sources: docs/research/2026-08-23-ha-registry-apis.md (P0-04),
// docs/research/2026-08-23-ha-automation-traces.md (P0-05),
// docs/research/2026-08-23-ha-history-statistics.md (P0-07).
const (
	// Core state and identity.
	CommandGetConfig       = "get_config"
	CommandGetStates       = "get_states"
	CommandAuthCurrentUser = "auth/current_user"

	// Registries. list_for_display is a *display-filtered* population, not a
	// cheaper form of the full list (P0-04 finding 1): 469 entries where
	// list returned 952. It is allow-listed only so a tool that explicitly
	// means "what a user sees" can exist; inventory reads the full list.
	CommandEntityRegistryList           = "config/entity_registry/list"
	CommandEntityRegistryListForDisplay = "config/entity_registry/list_for_display"
	CommandEntityRegistryGet            = "config/entity_registry/get"
	CommandDeviceRegistryList           = "config/device_registry/list"
	CommandAreaRegistryList             = "config/area_registry/list"
	CommandFloorRegistryList            = "config/floor_registry/list"
	CommandLabelRegistryList            = "config/label_registry/list"
	CommandCategoryRegistryList         = "config/category_registry/list"

	// Config entries. config_entries/get_single is deliberately absent: it
	// returns the same data as the list form but is the one registry command
	// HA gates on admin, and P0-04 decided the detail path reads the open
	// list endpoint and selects in-process.
	CommandConfigEntriesGet = "config_entries/get"

	// Automations and traces. Every one of these is admin-gated by HA
	// (P0-05); a non-admin principal gets `unauthorized`, which the layers
	// above must surface as unsupported rather than as an empty answer.
	CommandAutomationConfig = "automation/config"
	CommandTraceList        = "trace/list"
	CommandTraceGet         = "trace/get"
	CommandTraceContexts    = "trace/contexts"

	// History, logbook and recorder statistics. Statistics are 1–3 orders of
	// magnitude cheaper than raw history on a real recorder (P0-07), which is
	// why all three statistics commands are listed and preferred.
	CommandLogbookGetEvents       = "logbook/get_events"
	CommandHistoryDuringPeriod    = "history/history_during_period"
	CommandListStatisticIDs       = "recorder/list_statistic_ids"
	CommandGetStatisticsMetadata  = "recorder/get_statistics_metadata"
	CommandStatisticsDuringPeriod = "recorder/statistics_during_period"
)

// allowedCommands is an exact-match set — never a prefix or pattern rule. A
// pattern that admits config/entity_registry/list also admits
// config/entity_registry/update, which is precisely how the read-only
// guarantee would be lost without anyone editing a security file (phase 01
// Design Notes, ADR-008).
var allowedCommands = map[string]struct{}{
	CommandGetConfig:                    {},
	CommandGetStates:                    {},
	CommandAuthCurrentUser:              {},
	CommandEntityRegistryList:           {},
	CommandEntityRegistryListForDisplay: {},
	CommandEntityRegistryGet:            {},
	CommandDeviceRegistryList:           {},
	CommandAreaRegistryList:             {},
	CommandFloorRegistryList:            {},
	CommandLabelRegistryList:            {},
	CommandCategoryRegistryList:         {},
	CommandConfigEntriesGet:             {},
	CommandAutomationConfig:             {},
	CommandTraceList:                    {},
	CommandTraceGet:                     {},
	CommandTraceContexts:                {},
	CommandLogbookGetEvents:             {},
	CommandHistoryDuringPeriod:          {},
	CommandListStatisticIDs:             {},
	CommandGetStatisticsMetadata:        {},
	CommandStatisticsDuringPeriod:       {},
}

// deniedCommands is a small, explicit deny set of known privileged escape
// hatches, consulted before the allow-list (F-13). An allow-list alone
// already refuses everything here — this table exists so the refusal is
// documented in the code that enforces it, not left to depend on nobody
// having typed a name into allowedCommands.
var deniedCommands = map[string]struct{}{
	// supervisor/api accepts a free-form endpoint and method and runs as
	// Core's Supervisor user, which Core 2026.8.3 puts in GROUP_ID_ADMIN: a
	// single command that is both a universal escape hatch and a write path,
	// the two shapes CLAUDE.md rules 1 and 2 forbid outright (F-13).
	"supervisor/api": {},
}

// checkCommand decides whether name may be sent at all. It fails closed: an
// unlisted command is refused, never passed through because it "looks
// harmless". The deny set is consulted first and does not depend on the
// allow-list's contents, so a name refused here stays refused even if it is
// (or later becomes) allow-listed by mistake — the guard test
// TestGateway_DenySet_NotInAllowList exists precisely to catch that mistake
// before it ships. The reason names the table that refused it, so an audit
// record can distinguish "denied by name" from "not allow-listed" (P1-07).
//
// There is deliberately no parameter, flag or profile that widens either set
// at runtime — read-only-ness is a property of what is linked in, not of
// configuration (CLAUDE.md rule 1).
func checkCommand(name string) error {
	if _, ok := deniedCommands[name]; ok {
		return fmt.Errorf("%w: websocket command %q is denied by name", ErrPolicyDenied, name)
	}
	if _, ok := allowedCommands[name]; !ok {
		return fmt.Errorf("%w: websocket command %q is not allow-listed", ErrPolicyDenied, name)
	}
	return nil
}
