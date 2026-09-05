// Package redact applies, at the response boundary, the decisions
// internal/policy makes about what may be seen. It strips SECRET values so
// they cannot cross (CLAUDE.md rule 4) and masks PRIVATE ones so their shape
// in time survives while their meaning does not (PRIVATE-handling decision,
// 2026-08-25).
//
// It classifies nothing. Every "is this sensitive?" and "what should happen
// to it?" question is answered by internal/policy; this package only carries
// out the answer and records that it did.
//
// Redaction runs here, on the decoded payload, rather than on the serialized
// response (doc §5): a redaction step bolted onto serialization is eventually
// bypassed by a code path that formats its own output.
package redact

import (
	"crypto/rand"
	"encoding/hex"
	"math"
	"strconv"
	"strings"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// Placeholders are what a withheld value is replaced with. A withheld field
// is never dropped: the agent must be able to tell "this was withheld" from
// "this does not exist", which is the same distinction rule 7 draws between a
// refusal and an empty result.
const (
	// RedactedPlaceholder marks a value removed because it must never be
	// seen — a credential, or a string carrying one.
	RedactedPlaceholder = "[redacted]"
	// DeniedPlaceholder marks a PRIVATE value withheld because the profile
	// is deny. It is deliberately distinct from RedactedPlaceholder: one is
	// a secret, the other is a configuration choice the owner can change.
	DeniedPlaceholder = "[denied]"
)

// maskedPrefix opens every masked token, so a masked state can never be
// mistaken for a real one. Without it an agent reads "state_A" as a state
// name and reasons about it as fact (PRIVATE-handling decision,
// Consequences).
const maskedPrefix = "[masked:"

// Token kinds name what was masked, so a masked history reads as a timeline
// rather than as noise.
const (
	kindState = "state"
	kindValue = "value"
	// kindText names free-form prose masked as a unit — a logbook message
	// composed by HA from a friendly name and a state, with no field
	// boundary inside it to redact (P3-09 decision, 2026-09-05).
	kindText = "text"
)

// maxDepth bounds the walk. HA data is untrusted (rule 6): a deeply nested
// payload — malformed, or crafted — must not exhaust the stack of a
// long-lived process. It matches internal/policy's classification bound, so
// the two walks agree on where a payload stops being readable; the deepest
// structure observed in real data is a trace's changed_variables at 6 levels
// (docs/research/2026-08-23-ha-automation-traces.md).
const maxDepth = 64

// Marker records that something was withheld, and where. The response carries
// these so "nothing was withheld" and "something was" are different answers,
// visible without diffing against a payload the agent never saw.
type Marker struct {
	// Path locates the field in the payload, e.g.
	// trace.trigger/1[0].changed_variables.this.attributes.access_token.
	Path        string
	Action      policy.Action
	Sensitivity policy.Sensitivity
	// Detail explains a non-obvious action — a coarsened coordinate is
	// masked but still present, and saying so keeps the agent from reading
	// it as exact.
	Detail string
}

// Result is one redacted payload and the record of what it cost.
type Result struct {
	Value   any
	Markers []Marker
}

// Redactor applies one response's redaction. It holds that response's mask
// token table, so the same underlying value maps to the same token
// throughout — transition counting is unreadable otherwise — and a fresh
// Redactor per response makes the tokens differ between responses, which
// stops them becoming a de-facto stable identifier that leaks the value by
// correlation (PRIVATE-handling decision, Consequences).
//
// One Redactor serves one invocation and is not safe for concurrent use by
// several goroutines; Text alone is, because it touches no state.
type Redactor struct {
	profile policy.Profile
	// secrets are literal values that must never appear in output whatever
	// key they hang off: SUPERVISOR_TOKEN planted in a friendly_name is not
	// caught by key classification, only by matching the value itself.
	secrets []string
	nonce   string
	tokens  map[string]string
	issued  int
}

// New returns a Redactor for one response under the given profile. Secrets
// are the exact literal values that must never be emitted; pass
// SUPERVISOR_TOKEN here. Empty strings are ignored — a zero-length secret
// would match everywhere.
func New(profile policy.Profile, secrets ...string) *Redactor {
	kept := make([]string, 0, len(secrets))
	for _, s := range secrets {
		if s != "" {
			kept = append(kept, s)
		}
	}
	return &Redactor{
		profile: profile,
		secrets: kept,
		nonce:   responseNonce(),
		tokens:  make(map[string]string),
	}
}

// responseNonce is what makes this response's mask tokens unlike the last
// one's. crypto/rand.Read is documented never to fail; an error here would
// still be silent predictability, so it degrades to a fixed nonce only in a
// world where randomness is unavailable, and the tokens remain marked masked
// either way.
func responseNonce() string {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "0000"
	}
	return hex.EncodeToString(b[:])
}

// Payload returns a redacted copy of a decoded HA payload. The input is never
// mutated: values crossing a layer are values, not shared structures
// (CLAUDE.md, Immutability at boundaries).
func (r *Redactor) Payload(v any) Result {
	w := &walker{r: r}
	return Result{Value: w.value("", v, subject{}, 0), Markers: w.markers}
}

// Config returns a redacted copy of a get_config payload. It differs from
// Payload in one place: the installation's own coordinates are coarsened to
// policy.CoordinateDecimals rather than masked, keeping sun-elevation,
// weather and timezone correlation working while removing address-level
// identification (PRIVATE-handling decision). location_name is the owner's
// own label and passes through, which is why internal/policy leaves it out of
// the config table.
func (r *Redactor) Config(v any) Result {
	cfg, ok := v.(map[string]any)
	if !ok {
		return r.Payload(v)
	}

	w := &walker{r: r}
	out := make(map[string]any, len(cfg))
	for key, val := range cfg {
		class := policy.ClassifyConfigField(key)
		action := r.profile.Decide(class)
		coord, isCoord := val.(float64)
		if action != policy.ActionMask || class != policy.SensitivityPrivate || !isCoord {
			out[key] = w.field(key, key, val, class, subject{}, 0)
			continue
		}
		out[key] = coarsen(coord)
		w.mark(key, policy.ActionMask, class, "coarsened to one decimal place")
	}
	return Result{Value: out, Markers: w.markers}
}

// coarsen rounds a coordinate to the precision internal/policy set.
func coarsen(v float64) float64 {
	factor := math.Pow(10, policy.CoordinateDecimals)
	return math.Round(v*factor) / factor
}

// MaskedText applies the profile to one free-form string whose class
// internal/policy has already decided, and whose subject entity scopes its
// mask token. It exists for text that has no field boundary to walk — a
// logbook message is prose, not an object — so Payload's key-driven
// classification has nothing to key on (P3-09 decision, 2026-09-05).
//
// This package still classifies nothing: the caller passes the class it got
// from internal/policy, exactly as the walker passes the one classOf
// returned.
func (r *Redactor) MaskedText(class policy.Sensitivity, entityID, s string) string {
	switch r.profile.Decide(class) {
	case policy.ActionRedact:
		return RedactedPlaceholder
	case policy.ActionDeny:
		return DeniedPlaceholder
	case policy.ActionMask:
		return r.token(kindText, entityID, s)
	default:
		return r.Text(s)
	}
}

// Text returns a string with every known secret literal replaced. Use it on
// anything free-form that reaches a response or a log: an error message, a
// log line, an HA-supplied text field.
func (r *Redactor) Text(s string) string {
	for _, secret := range r.secrets {
		s = strings.ReplaceAll(s, secret, RedactedPlaceholder)
	}
	return s
}

// Error returns err with its message scrubbed of secret literals. The
// original is kept in the chain so errors.Is and errors.As still separate
// "absent" from "cannot check" from "refused" all the way to the MCP
// response (CLAUDE.md, Error Handling) — the scrubbed message is what
// crosses the boundary, and Error() is the only thing a response renders.
func (r *Redactor) Error(err error) error {
	if err == nil {
		return nil
	}
	clean := r.Text(err.Error())
	if clean == err.Error() {
		return err
	}
	return &scrubbedError{msg: clean, err: err}
}

type scrubbedError struct {
	msg string
	err error
}

func (e *scrubbedError) Error() string { return e.msg }
func (e *scrubbedError) Unwrap() error { return e.err }

// walker carries one call's marker list. The token table lives on the
// Redactor, because stability is a property of the response, not of the walk.
type walker struct {
	r       *Redactor
	markers []Marker
}

func (w *walker) mark(path string, action policy.Action, class policy.Sensitivity, detail string) {
	w.markers = append(w.markers, Marker{Path: path, Action: action, Sensitivity: class, Detail: detail})
}

// subject is the entity the enclosing object describes: how private it is,
// which is what makes a bare "state" key private since the key alone says
// nothing, and which entity it is, which scopes that entity's mask tokens to
// its own timeline.
type subject struct {
	class policy.Sensitivity
	id    string
}

// with raises a subject to the stricter of two readings, keeping the id of
// whichever reading names an entity most locally.
func (s subject) with(other subject) subject {
	if other.id != "" {
		s.id = other.id
	}
	if other.class > s.class {
		s.class = other.class
	}
	return s
}

// value returns a redacted copy of v.
func (w *walker) value(path string, v any, subj subject, depth int) any {
	if depth > maxDepth {
		// Nothing below this point has been read, so nothing below it may
		// be reported as safe (rule 3, fail closed).
		w.mark(path, policy.ActionRedact, policy.SensitivitySecret, "payload nested deeper than the walk enters")
		return RedactedPlaceholder
	}

	switch t := v.(type) {
	case map[string]any:
		subj = subj.with(subjectOf(t))
		out := make(map[string]any, len(t))
		for key, val := range t {
			out[w.r.Text(key)] = w.field(join(path, key), key, val, classOf(key, subj.class), subj, depth)
		}
		return out
	case []any:
		// A history array may name its entity only in the first element
		// (HA's minimal_response), so the subject is taken across the whole
		// array before any element is walked.
		for _, el := range t {
			if m, ok := el.(map[string]any); ok {
				subj = subj.with(subjectOf(m))
			}
		}
		out := make([]any, len(t))
		for i, el := range t {
			out[i] = w.value(index(path, i), el, subj, depth+1)
		}
		return out
	case string:
		return w.r.Text(t)
	default:
		return v
	}
}

// field applies the profile's decision to one key's value, descending only
// when the value is allowed through. A withheld value is not walked: there is
// nothing below it the response is entitled to.
func (w *walker) field(path, key string, val any, class policy.Sensitivity, subj subject, depth int) any {
	switch action := w.r.profile.Decide(class); action {
	case policy.ActionRedact:
		w.mark(path, action, class, "")
		return RedactedPlaceholder
	case policy.ActionDeny:
		w.mark(path, action, class, "the deny profile withholds private values")
		return DeniedPlaceholder
	case policy.ActionMask:
		w.mark(path, action, class, "")
		return w.r.token(kindOf(key), subj.id, val)
	default:
		return w.value(path, val, subj.with(subjectFromID(key)), depth+1)
	}
}

// token returns this response's opaque token for a value, minting one on
// first sight. Equal values share a token so transitions stay countable;
// the response nonce keeps the mapping from outliving the response.
// Tokens are scoped to one entity: stability within a timeline is what makes
// transitions countable, while a token shared between two entities would say
// their states agree — meaning withheld, but correlation handed over anyway.
func (r *Redactor) token(kind, entityID string, v any) string {
	key := kind + "\x00" + entityID + "\x00" + valueKey(v)
	if tok, ok := r.tokens[key]; ok {
		return tok
	}
	tok := maskedPrefix + kind + "_" + r.nonce + letters(r.issued) + "]"
	r.issued++
	r.tokens[key] = tok
	return tok
}

// valueKey identifies a masked value for token reuse without retaining it in
// any form that reaches a response. Non-scalars collapse to their kind: a
// masked object has no shape worth preserving, only its presence.
func valueKey(v any) string {
	switch t := v.(type) {
	case string:
		return "s" + t
	case bool:
		if t {
			return "btrue"
		}
		return "bfalse"
	case float64:
		return "f" + strconv.FormatFloat(t, 'g', -1, 64)
	case nil:
		return "null"
	default:
		return "opaque"
	}
}

// letters counts in A, B, … Z, AA, AB, … so a masked timeline reads as a
// short sequence of distinct values rather than as opaque hashes.
func letters(n int) string {
	out := ""
	for {
		out = string(rune('A'+n%26)) + out
		n = n/26 - 1
		if n < 0 {
			return out
		}
	}
}

// classOf is the class of one key's value: what the key says about itself,
// raised to the enclosing entity's class when the key is the entity's state.
func classOf(key string, subjectClass policy.Sensitivity) policy.Sensitivity {
	class := policy.ClassifyAttribute(key)
	if key == "state" || key == "s" { // "s" is HA's minimal_response spelling
		if subjectClass > class {
			class = subjectClass
		}
	}
	return class
}

// kindOf names the token a masked key gets. Only the entity's own state is a
// state; everything else is a value, so "state_A" never labels a latitude.
func kindOf(key string) string {
	if key == "state" || key == "s" {
		return kindState
	}
	return kindValue
}

// subjectOf reads which entity an object describes and how private that
// entity is. Extracting the id and device class is mechanical; deciding what
// they mean is internal/policy's, which is why both lookups go back to it.
func subjectOf(m map[string]any) subject {
	id, _ := m["entity_id"].(string)
	if id == "" {
		return subject{}
	}
	deviceClass, _ := m["device_class"].(string)
	if deviceClass == "" {
		if attrs, ok := m["attributes"].(map[string]any); ok {
			deviceClass, _ = attrs["device_class"].(string)
		}
	}
	return subject{class: policy.ClassifyEntityWithClass(model.EntityID(id), deviceClass), id: id}
}

// subjectFromID reads a map key that is itself an entity id. A history
// payload keys its arrays that way, and HA's minimal_response drops
// entity_id from every element after the first — the key is then the only
// thing that says whose states these are.
func subjectFromID(key string) subject {
	class := policy.ClassifyEntity(model.EntityID(key))
	if class == policy.SensitivityNormal {
		return subject{}
	}
	return subject{class: class, id: key}
}

func index(path string, i int) string {
	return path + "[" + strconv.Itoa(i) + "]"
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}
