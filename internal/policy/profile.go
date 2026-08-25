package policy

import (
	"fmt"
	"strings"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// Handling is what a profile does with PRIVATE data. Secrecy is not on this
// axis: SECRET is always redacted, under every profile (CLAUDE.md rule 4 and
// Configuration — limits and the privacy profile are configurable,
// read-only-ness and secrecy are not).
type Handling int

const (
	// HandlingMask is the default, and is the zero value deliberately: the
	// PRIVATE-handling decision of 2026-08-25 requires that an empty
	// configuration resolve to mask rather than to a setup step somebody can
	// forget. States become opaque tokens while timestamps, ordering and
	// transition counts survive verbatim, so availability, flapping and
	// correlation analysis keep working while occupancy reconstruction does
	// not (threat T3).
	HandlingMask Handling = iota
	// HandlingAllow returns PRIVATE values unchanged. For an owner running
	// this server entirely against a local model.
	HandlingAllow
	// HandlingDeny refuses any PRIVATE value with an explicit policy error.
	// Rejected as the default: a flaky presence sensor or an unreliable lock
	// is exactly what this server exists to diagnose, and a default every
	// owner immediately loosens is not a default.
	HandlingDeny
)

// String returns the configuration name, which is also what appears in an
// audit record.
func (h Handling) String() string {
	switch h {
	case HandlingAllow:
		return "allow"
	case HandlingDeny:
		return "deny"
	default:
		return "mask"
	}
}

// Action is what the response boundary must do with one value. internal/policy
// decides it; internal/redact applies it (PRIVATE-handling decision,
// Consequences).
type Action int

const (
	// ActionAllow passes the value through unchanged.
	ActionAllow Action = iota
	// ActionMask replaces the value's meaning with a stable opaque token
	// while preserving its shape in time. Masking is not redaction: a masked
	// field is marked masked, and the agent must be able to tell the two
	// apart or it will reason about "state_A" as though it were a state name.
	ActionMask
	// ActionRedact removes the value because it must never be seen.
	ActionRedact
	// ActionDeny refuses the whole request rather than returning a shaped
	// answer.
	ActionDeny
)

// String returns the marker name a response carries for this action.
func (a Action) String() string {
	switch a {
	case ActionMask:
		return "masked"
	case ActionRedact:
		return "redacted"
	case ActionDeny:
		return "denied"
	default:
		return "allowed"
	}
}

// CoordinateDecimals is the precision the installation's own coordinates are
// coarsened to before they cross the response boundary — one decimal place,
// roughly 11 km (PRIVATE-handling decision, 2026-08-25). It keeps sun
// elevation, weather and timezone correlation working, which are real
// diagnostic inputs, while removing address-level identification. Withholding
// the coordinates outright would degrade those correlations to buy nothing
// masking does not already buy.
//
// policy decides the precision; internal/redact applies it (P2-03).
const CoordinateDecimals = 1

// Profile is the configured privacy policy for one server instance. Its zero
// value is the default profile — mask — so a Profile that nobody configured
// is safe rather than absent.
type Profile struct {
	Private Handling
}

// NewProfile builds a profile from its configuration name. An empty name is
// the default (mask); an unrecognized one is a configuration error, never a
// silent fallback — failing closed is the rule (CLAUDE.md rule 3).
func NewProfile(name string) (Profile, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "mask":
		return Profile{Private: HandlingMask}, nil
	case "allow":
		return Profile{Private: HandlingAllow}, nil
	case "deny":
		return Profile{Private: HandlingDeny}, nil
	default:
		return Profile{}, fmt.Errorf("policy: unknown privacy profile %q: want mask, allow or deny", name)
	}
}

// Decide returns what the response boundary must do with a value of this
// class under this profile.
func (p Profile) Decide(s Sensitivity) Action {
	switch s {
	case SensitivitySecret:
		return ActionRedact
	case SensitivityPrivate:
		switch p.Private {
		case HandlingAllow:
			return ActionAllow
		case HandlingDeny:
			return ActionDeny
		default:
			return ActionMask
		}
	default:
		return ActionAllow
	}
}

// HistoryScope describes what a history query will cover, before it is
// issued. Entities are explicitly named; Domains are swept whole — every
// entity the domain contains.
//
// The distinction is the one the profile acts on: naming a lock or a presence
// sensor is a targeted diagnostic, and masking keeps its shape in time while
// destroying its meaning. Sweeping person.* is the occupancy timeline itself,
// in volume, and masking does not take that back — the correlation between N
// masked trackers still reconstructs the household's day. Doc §5.1's PRIVATE
// row says "avoid bulk history" for exactly this reason.
type HistoryScope struct {
	Entities []model.EntityID
	Domains  []string
}

// PolicyError says what was refused and why, so the agent can narrow its
// query rather than guess. It never carries the value it refused to return.
type PolicyError struct {
	Sensitivity Sensitivity
	// Subject is the entity id or domain the refusal is about.
	Subject string
	Reason  string
}

func (e *PolicyError) Error() string {
	return fmt.Sprintf("policy: refused %s data for %q: %s", e.Sensitivity, e.Subject, e.Reason)
}

func (e *PolicyError) Unwrap() error { return ErrPolicyDenied }

// CheckHistoryScope refuses a history query the profile will not serve,
// before it is issued. Refusing after the recorder has answered would spend
// the Raspberry Pi's budget to produce nothing (phase 02 design notes).
//
// A denial and an empty result are three different answers away from each
// other (CLAUDE.md, Error Handling): this returns ErrPolicyDenied, never an
// empty list.
func (p Profile) CheckHistoryScope(sc HistoryScope) error {
	if p.Private == HandlingAllow {
		return nil
	}

	for _, domain := range sc.Domains {
		if ClassifyEntity(model.EntityID(domain+".")) != SensitivityPrivate {
			continue
		}
		return &PolicyError{
			Sensitivity: SensitivityPrivate,
			Subject:     domain,
			Reason: "bulk history over a private domain reconstructs occupancy even when " +
				"individual states are masked; name specific entities instead",
		}
	}

	if p.Private != HandlingDeny {
		return nil
	}
	for _, id := range sc.Entities {
		if ClassifyEntity(id) != SensitivityPrivate {
			continue
		}
		return &PolicyError{
			Sensitivity: SensitivityPrivate,
			Subject:     string(id),
			Reason:      "the deny profile withholds private entities entirely",
		}
	}
	return nil
}
