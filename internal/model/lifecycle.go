package model

import "time"

// LifecycleEvent is one Home Assistant core lifecycle row of the logbook (it
// started or stopped). Only the instant is kept: the row's message is log
// text, and no behaviour may depend on it (CLAUDE.md rule 6), so a start is
// not told from a stop. Any such row near an outage's onset is evidence the
// core went through a restart cycle there.
type LifecycleEvent struct {
	When time.Time

	Provenance
}
