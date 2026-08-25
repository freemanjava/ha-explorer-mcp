package model

// Integration is the normalized view of one Home Assistant config entry (one
// integration instance).
type Integration struct {
	ID     ConfigEntryID
	Domain string
	Title  string

	State      string
	Source     string
	Disabled   bool
	DisabledBy string
	Reason     string // set when State reports an error

	Provenance
}
