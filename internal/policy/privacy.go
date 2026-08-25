package policy

import (
	"strings"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// Sensitivity is doc §5.1's privacy class. It orders from least to most
// sensitive, so the highest class found in a payload is the one that governs
// it — SECRET outranks PRIVATE outranks NORMAL.
//
// Home Assistant filters nothing by principal (F-10): a non-admin connection
// receives the byte-identical registries an admin does, and get_config hands
// out the installation's coordinates. The classification here is therefore
// the only privacy decision made anywhere in the path; there is no upstream
// filter behind it to fall back on.
type Sensitivity int

const (
	// SensitivityNormal is ordinary telemetry: temperatures, CPU load,
	// generic sensor states. Allowed subject to the query budget.
	SensitivityNormal Sensitivity = iota
	// SensitivityPrivate is data that reconstructs a household's life:
	// presence, location, locks, alarm state. Handling follows the profile.
	SensitivityPrivate
	// SensitivitySecret is a credential. Never returned, under any profile.
	SensitivitySecret
)

// String returns the stable identifier used in responses and audit records.
func (s Sensitivity) String() string {
	switch s {
	case SensitivityPrivate:
		return "private"
	case SensitivitySecret:
		return "secret"
	default:
		return "normal"
	}
}

// max returns the stricter of two classes.
func (s Sensitivity) max(other Sensitivity) Sensitivity {
	if other > s {
		return other
	}
	return s
}

// privateDomains is the classification table for entity domains, doc §5.1's
// PRIVATE row made explicit. A reviewer must be able to read what is private
// off one list rather than reconstruct it from conditionals scattered across
// the tools (P2-02 DoD).
//
// A domain absent from this table classifies NORMAL. Failing closed on the
// unknown would be wrong here in a way it is right elsewhere: HA adds domains
// every release, and a default of PRIVATE would mask an entire installation
// the first time one appeared. The attribute table below is what catches an
// unlisted domain that nonetheless carries location.
var privateDomains = map[string]struct{}{
	// Doc §5.1, named directly.
	"person":              {}, // who is where
	"device_tracker":      {}, // where a device is, often with coordinates
	"lock":                {}, // whether the house is open
	"alarm_control_panel": {}, // whether the house is armed, and when it was
	"zone":                {}, // the coordinates of home, work, school

	// Doc §5.1's "presence/occupancy" has no domain of its own — an occupancy
	// sensor sits in binary_sensor next to a power meter — so the rest of
	// that row is caught by privateDeviceClasses, not here.
	"proximity": {}, // distance from a zone — location by another name
}

// privateDeviceClasses are the device classes that make an otherwise generic
// domain (binary_sensor, sensor) an occupancy signal. Doc §5.1 lists
// "presence/occupancy" without naming a domain, because HA does not give it
// one.
var privateDeviceClasses = map[string]struct{}{
	"occupancy":   {},
	"presence":    {},
	"motion":      {},
	"door":        {}, // a door sensor is an occupancy timeline with extra steps
	"garage_door": {},
	"window":      {},
}

// secretKeyFragments match an attribute key by class, not by exact spelling
// (phase 02 design notes): matching only "token" misses access_token, api_key
// and Authorization. Matching is case-insensitive on a substring, because the
// integrations that carry these keys do not agree on a convention.
var secretKeyFragments = []string{
	"token",
	"password",
	"secret",
	"api_key",
	"apikey",
	"credential",
	"authorization",
}

// privateAttributeKeys are the attribute keys that carry PRIVATE data
// whatever entity they hang off. A generic sensor that happens to expose
// latitude is as revealing as a device_tracker.
var privateAttributeKeys = map[string]struct{}{
	"latitude":      {},
	"longitude":     {},
	"gps_accuracy":  {},
	"user_id":       {}, // names a household member (F-12)
	"course":        {},
	"speed":         {},
	"altitude":      {},
	"address":       {},
	"street":        {},
	"postal_code":   {},
	"place_name":    {},
	"geocoded_area": {},
}

// privateConfigFields are the get_config fields that describe where the
// installation physically is. HA returns them to any principal (F-10) on a
// path unrelated to the history tools, so they need their own table entry or
// they are the one PRIVATE value nothing classifies.
//
// location_name is deliberately absent: it is the owner's own label for their
// home ("Home"), carries no address, and passing it through keeps a masked
// response readable (PRIVATE-handling decision, 2026-08-25).
var privateConfigFields = map[string]struct{}{
	"latitude":  {},
	"longitude": {},
}

// maxPayloadDepth bounds the classification walk. HA data is untrusted
// (CLAUDE.md rule 6): a deeply nested payload — malformed, or crafted — must
// not blow the stack of a long-lived process. The deepest structure observed
// in a real payload is a trace's changed_variables at 6 levels
// (docs/research/2026-08-23-ha-automation-traces.md), so this leaves an order
// of magnitude of headroom before the cut-off can fire on real data.
const maxPayloadDepth = 64

// ClassifyEntity returns the privacy class of an entity from its id alone.
// A malformed or empty id classifies NORMAL — it names nothing, and treating
// unparseable input as private would mask on the strength of a typo.
func ClassifyEntity(id model.EntityID) Sensitivity {
	domain, _, ok := strings.Cut(string(id), ".")
	if !ok || domain == "" {
		return SensitivityNormal
	}
	if _, private := privateDomains[domain]; private {
		return SensitivityPrivate
	}
	return SensitivityNormal
}

// ClassifyEntityWithClass classifies an entity whose device class is known.
// The device class is what separates a door contact from a power meter inside
// the same binary_sensor domain, and doc §5.1's "presence/occupancy" has no
// domain of its own.
func ClassifyEntityWithClass(id model.EntityID, deviceClass string) Sensitivity {
	s := ClassifyEntity(id)
	if _, private := privateDeviceClasses[strings.ToLower(deviceClass)]; private {
		return s.max(SensitivityPrivate)
	}
	return s
}

// ClassifyAttribute returns the privacy class of one attribute key. The key
// alone decides: a value is never inspected to classify it, because HA data
// is untrusted and branching on its content is threat T2.
func ClassifyAttribute(key string) Sensitivity {
	lower := strings.ToLower(key)
	for _, frag := range secretKeyFragments {
		if strings.Contains(lower, frag) {
			return SensitivitySecret
		}
	}
	if _, private := privateAttributeKeys[lower]; private {
		return SensitivityPrivate
	}
	return SensitivityNormal
}

// ClassifyConfigField returns the privacy class of a get_config field.
func ClassifyConfigField(name string) Sensitivity {
	if _, private := privateConfigFields[strings.ToLower(name)]; private {
		return SensitivityPrivate
	}
	return ClassifyAttribute(name)
}

// ClassifyPayload returns the highest class found anywhere in a decoded JSON
// payload: sensitivity travels with what a payload embeds, not with the
// endpoint it came from (F-12). An automation trace is diagnostic in shape
// and personal in content — its changed_variables carry whole state objects
// and a context.user_id — so classifying it by its endpoint would route
// personal data past the profile that exists to hold it.
//
// The walk stops early once SECRET is found; nothing outranks it.
func ClassifyPayload(v any) Sensitivity {
	return classifyValue(v, 0)
}

func classifyValue(v any, depth int) Sensitivity {
	if depth > maxPayloadDepth {
		// Beyond the cut-off nothing has been read, so nothing may be
		// reported as safe. Fail closed on the part of the payload the walk
		// declined to enter.
		return SensitivityPrivate
	}

	switch t := v.(type) {
	case map[string]any:
		worst := SensitivityNormal
		// A state object carries its own device_class, which is what
		// separates a door contact from a power meter inside binary_sensor.
		// This reads an HA-supplied value, which rule 6 otherwise forbids
		// branching on — it is safe only because the lookup can escalate the
		// class and never lower it, so no attacker-controlled string can buy
		// itself less protection (threat T2).
		if dc, ok := t["device_class"].(string); ok {
			if _, private := privateDeviceClasses[strings.ToLower(dc)]; private {
				worst = SensitivityPrivate
			}
		}
		for key, val := range t {
			worst = worst.max(ClassifyAttribute(key))
			if worst == SensitivitySecret {
				return worst
			}
			// A null under a PRIVATE key is not an exposure: HA sets
			// context.user_id to null for state changes it made itself.
			if val == nil {
				continue
			}
			worst = worst.max(classifyValue(val, depth+1))
			if worst == SensitivitySecret {
				return worst
			}
		}
		return worst
	case []any:
		worst := SensitivityNormal
		for _, val := range t {
			worst = worst.max(classifyValue(val, depth+1))
			if worst == SensitivitySecret {
				return worst
			}
		}
		return worst
	case string:
		// Entity ids appear as values in many places a trace touches:
		// trigger configs, service targets, condition results. Classifying
		// only under an "entity_id" key would miss them.
		return ClassifyEntity(model.EntityID(t))
	default:
		return SensitivityNormal
	}
}
