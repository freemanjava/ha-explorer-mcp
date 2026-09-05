package model

import "time"

// SupervisorInfo is the normalized view of Supervisor's own /supervisor/info
// response — Supervisor's status and the installed-App inventory it embeds,
// not Core's. Distinct from Health: this is mapped directly from one payload,
// not computed by internal/analysis.
type SupervisorInfo struct {
	Version       string
	VersionLatest string
	Channel       string
	Supported     bool
	Healthy       bool

	Apps []App

	Provenance
}

// App is one installed Home Assistant App (add-on) as Supervisor reports it
// in /supervisor/info's addons[] — the App list a Supervisor-role default
// grants, not the richer manager-only /addons collection (research doc
// 2026-08-23-supervisor-permissions.md).
type App struct {
	Slug            string
	Name            string
	Version         string
	VersionLatest   string
	UpdateAvailable bool
	State           string
	Repository      string
}

// AppList is list_apps' page: the installed-App inventory Supervisor's own
// /supervisor/info embeds, paginated like every list_* tool. Unsupported is
// distinct from an empty Items: Supervisor unreachable at the granted role
// reports Unsupported with a reason, never an empty list that would look
// like "no Apps installed" (P3-06 DoD, CLAUDE.md rule 7).
type AppList struct {
	Source     string
	ObservedAt time.Time

	Unsupported       bool
	UnsupportedReason string

	Items        []App
	NextCursor   string
	Truncated    bool
	LimitClamped bool

	Provenance
}
