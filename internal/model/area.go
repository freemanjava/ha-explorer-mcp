package model

// Area is the normalized view of one Home Assistant area.
type Area struct {
	ID      AreaID
	Name    string
	FloorID string
	Icon    string
	Labels  []string

	Provenance
}
