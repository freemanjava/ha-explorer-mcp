package model

// Provenance marks whether a mapped value fully represents its source
// payload. A malformed or partially missing HA payload maps to a value with
// Partial set and PartialReason naming what was wrong, rather than panicking
// or silently zeroing fields (CLAUDE.md, Error Handling).
type Provenance struct {
	Partial       bool
	PartialReason string
}
