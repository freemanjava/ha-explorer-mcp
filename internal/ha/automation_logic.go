package ha

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// Caps on one automation's logic (phase 09 Design Notes, "Cost"). Starting
// values, not measurements: P9-05 measures real automations on the Pi before
// any of these is tuned.
const (
	// maxLogicNodes bounds the triggers, conditions and actions mapped across
	// every nesting level. A hand-written automation has tens.
	maxLogicNodes = 500
	// maxLogicDepth bounds node nesting (choose → option → sequence counts
	// two levels). Real automations nest a handful.
	maxLogicDepth = 16
	// maxValuesPerNode bounds the typed values and templates one node carries,
	// so a node with thousands of keys cannot inflate a response.
	maxValuesPerNode = 64
	// maxValueDepth bounds how deep a non-logic object (target, data,
	// event_data) is flattened into dotted keys.
	maxValueDepth = 4
	// maxTemplateBytes caps one template's text (D-09-2). Long enough for the
	// threshold logic a template usually holds.
	maxTemplateBytes = 1024
	// maxPartialReasons caps the reasons joined into PartialReason, so a body
	// of hundreds of malformed items yields a short reason, not a long one.
	maxPartialReasons = 8
)

// The D-09-2 value grammars. A string passes as a typed fact only by matching
// one of these; anything else is free text and counted, never echoed.
var (
	tokenPattern    = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)
	numberPattern   = regexp.MustCompile(`^-?\d{1,15}(\.\d{1,15})?$`)
	timePattern     = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d(:[0-5]\d)?$`)
	durationPattern = regexp.MustCompile(`^-?\d{1,3}:[0-5]\d(:[0-5]\d(\.\d{1,6})?)?$`)
)

// Key-name sets steer which grammar a value is checked against. They branch
// on the config schema's own keys, never on HA-authored content (rule 6).
var (
	// strictKeys hold an id or a service name and accept only that grammar:
	// a device trigger's registry uuid under entity_id is withheld rather
	// than passed as a token, so every entity the tool must mask under deny
	// (D-09-4) is a ValueEntity.
	strictKeys = map[string]*regexp.Regexp{
		"entity_id": entityIDPattern,
		"device_id": dependencyIDPattern,
		"area_id":   dependencyIDPattern,
		"action":    entityIDPattern, // domain.service has the entity-id shape
		"service":   entityIDPattern,
	}
	strictKinds = map[string]model.ValueKind{
		"entity_id": model.ValueEntity,
		"device_id": model.ValueDevice,
		"area_id":   model.ValueArea,
		"action":    model.ValueService,
		"service":   model.ValueService,
	}
	// durationKeys take an HA duration, which shares the HH:MM shape with a
	// time of day but may exceed 24h or be negative (a sun offset).
	durationKeys = map[string]bool{"for": true, "delay": true, "timeout": true, "offset": true}
	// durationUnits are the fields of HA's object duration form, in seconds.
	durationUnits = map[string]float64{"days": 86400, "hours": 3600, "minutes": 60, "seconds": 1, "milliseconds": 0.001}
	// freeTextKeys are labels and messages: withheld even when the text
	// happens to fit the token grammar (D-09-2).
	freeTextKeys = map[string]bool{"alias": true, "description": true, "message": true, "title": true, "name": true}
)

// actionKinds names an action by the first of these keys it carries, in HA's
// own precedence. Several are values too (delay, event, wait_template).
var actionKinds = []struct{ key, kind string }{
	{"action", "service"}, {"service", "service"},
	{"choose", "choose"}, {"if", "if"}, {"repeat", "repeat"}, {"parallel", "parallel"},
	{"sequence", "sequence"}, {"wait_for_trigger", "wait_for_trigger"}, {"wait_template", "wait_template"},
	{"delay", "delay"}, {"event", "event"}, {"scene", "scene"}, {"stop", "stop"},
	{"variables", "variables"}, {"condition", "condition"}, {"device_id", "device"},
}

type logicRole int

const (
	roleTrigger logicRole = iota
	roleCondition
	roleAction
	roleOption
)

// optionKind is the Kind of one choose option, which HA does not name.
const optionKind = "option"

// MapAutomationLogicResult unwraps automation/config's {"config": {...}}
// envelope, as MapAutomationConfigResult does, and maps the inner object
// with MapAutomationLogic.
func MapAutomationLogicResult(raw json.RawMessage) (model.AutomationLogic, error) {
	var wire struct {
		Config map[string]any `json:"config"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return model.AutomationLogic{}, fmt.Errorf("ha: decoding automation/config: %w", err)
	}
	return MapAutomationLogic(wire.Config), nil
}

// MapAutomationLogic walks an automation/config body into typed logic
// (D-09-2). It reads both schema forms — plural triggers/conditions/actions
// with trigger:/action: inside, and the legacy singular form with
// platform:/service: — into the same nodes. Values pass only by grammar;
// templates are copied verbatim, capped; every other string is counted in
// the node's Withheld. Nothing here interprets HA-authored text (rule 6), and
// no privacy work is done: the tool applies D-09-4/D-09-5.
func MapAutomationLogic(body map[string]any) model.AutomationLogic {
	w := &logicWalker{}
	if body == nil {
		w.partial("config missing")
	}
	// A blueprint automation's logic lives in the blueprint, not in this
	// config; empty sections would otherwise read as "none" (rule 7).
	if _, ok := body["use_blueprint"]; ok {
		w.partial("blueprint automation: triggers, conditions and actions are defined in the blueprint, not mapped")
	}
	logic := model.AutomationLogic{
		Triggers:   w.section(body, "trigger", roleTrigger, "triggers", "trigger"),
		Conditions: w.section(body, "condition", roleCondition, "conditions", "condition"),
		Actions:    w.section(body, "action", roleAction, "actions", "action"),
		Truncated:  w.truncated,
	}
	if len(w.reasons) > 0 {
		logic.Partial = true
		logic.PartialReason = strings.Join(w.reasons, "; ")
	}
	return logic
}

// logicWalker carries the caps' running state across one body. Partial
// reasons name only paths it generated itself, never payload text.
type logicWalker struct {
	nodes     int
	truncated bool
	reasons   []string
}

func (w *logicWalker) partial(reason string) {
	if len(w.reasons) < maxPartialReasons {
		w.reasons = append(w.reasons, reason)
	}
}

// section reads the first of keys present — plural before singular.
func (w *logicWalker) section(body map[string]any, path string, role logicRole, keys ...string) []model.LogicNode {
	for _, k := range keys {
		if v, ok := body[k]; ok {
			return w.list(v, path, 0, role)
		}
	}
	return nil
}

// list maps a sequence. HA accepts a bare object (or, for a condition, a
// template string) where it expects a one-element list.
func (w *logicWalker) list(raw any, path string, depth int, role logicRole) []model.LogicNode {
	var items []any
	switch v := raw.(type) {
	case nil:
		return nil
	case []any:
		items = v
	case map[string]any, string:
		items = []any{v}
	default:
		w.partial(path + " is not a list")
		return nil
	}
	var out []model.LogicNode
	for i, item := range items {
		if n, ok := w.node(item, fmt.Sprintf("%s/%d", path, i), depth, role); ok {
			out = append(out, n)
		}
	}
	return out
}

func (w *logicWalker) node(item any, path string, depth int, role logicRole) (model.LogicNode, bool) {
	if depth > maxLogicDepth || w.nodes >= maxLogicNodes {
		w.truncated = true
		return model.LogicNode{}, false
	}
	if s, ok := item.(string); ok && role == roleCondition && isTemplate(s) {
		w.nodes++
		n := model.LogicNode{Kind: "template", Path: path}
		w.addTemplate(&n, "value_template", s)
		return n, true
	}
	m, ok := item.(map[string]any)
	if !ok {
		w.partial(path + " is not an object")
		return model.LogicNode{}, false
	}
	w.nodes++
	n := model.LogicNode{Path: path}
	switch role {
	case roleTrigger:
		w.trigger(&n, m)
	case roleCondition:
		w.condition(&n, m, depth)
	case roleAction:
		w.action(&n, m, depth)
	case roleOption:
		w.option(&n, m, depth)
	}
	slices.SortStableFunc(n.Values, func(a, b model.TypedValue) int { return strings.Compare(a.Key, b.Key) })
	slices.SortStableFunc(n.Templates, func(a, b model.Template) int { return strings.Compare(a.Key, b.Key) })
	return n, true
}

func (w *logicWalker) trigger(n *model.LogicNode, m map[string]any) {
	w.kind(n, m, "trigger", "platform")
	w.values(n, m, "", 0, "trigger", "platform")
}

func (w *logicWalker) condition(n *model.LogicNode, m map[string]any, depth int) {
	w.kind(n, m, "condition")
	n.Children = w.list(m["conditions"], n.Path+"/conditions", depth+1, roleCondition)
	w.values(n, m, "", 0, "condition", "conditions")
}

func (w *logicWalker) option(n *model.LogicNode, m map[string]any, depth int) {
	n.Kind = optionKind
	n.Children = append(w.list(m["conditions"], n.Path+"/conditions", depth+1, roleCondition),
		w.list(m["sequence"], n.Path+"/sequence", depth+1, roleAction)...)
	w.values(n, m, "", 0, "conditions", "sequence")
}

// action names the node by actionKinds and walks every structural key it
// carries, whatever its kind, so a malformed mix is still fully reported.
// Child paths follow HA's trace convention.
func (w *logicWalker) action(n *model.LogicNode, m map[string]any, depth int) {
	for _, ak := range actionKinds {
		if _, ok := m[ak.key]; ok {
			n.Kind = ak.kind
			break
		}
	}
	p, d := n.Path, depth+1
	children := [][]model.LogicNode{
		w.list(m["choose"], p+"/choose", d, roleOption),
		w.list(m["default"], p+"/default", d, roleAction),
		w.list(m["if"], p+"/if/condition", d, roleCondition),
		w.list(m["then"], p+"/then", d, roleAction),
		w.list(m["else"], p+"/else", d, roleAction),
		w.repeat(n, m["repeat"], p+"/repeat", d),
		w.list(m["parallel"], p+"/parallel", d, roleAction),
		w.list(m["sequence"], p+"/sequence", d, roleAction),
		w.list(m["wait_for_trigger"], p+"/wait_for_trigger", d, roleTrigger),
	}
	if n.Kind == "condition" {
		children = append(children, w.list(m["conditions"], p+"/conditions", d, roleCondition))
	}
	for _, c := range children {
		n.Children = append(n.Children, c...)
	}
	// The legacy service: key is reported as action:, so both schema forms
	// map to the same values.
	if v, ok := m["service"]; ok {
		w.value(n, "action", "action", v, 0)
	}
	w.values(n, m, "", 0, "service", "choose", "default", "if", "then", "else", "repeat",
		"parallel", "sequence", "wait_for_trigger", "conditions")
}

// repeat reads a repeat block's count/for_each as the action's values and
// returns its while, until and sequence as children.
func (w *logicWalker) repeat(n *model.LogicNode, raw any, path string, depth int) []model.LogicNode {
	if raw == nil {
		return nil
	}
	rm, ok := raw.(map[string]any)
	if !ok {
		w.partial(path + " is not an object")
		return nil
	}
	w.values(n, rm, "", 0, "while", "until", "sequence")
	return slices.Concat(
		w.list(rm["while"], path+"/while", depth, roleCondition),
		w.list(rm["until"], path+"/until", depth, roleCondition),
		w.list(rm["sequence"], path+"/sequence", depth, roleAction),
	)
}

// kind sets the node's Kind from the first of keys present, by the token
// grammar. A trigger or condition with no kind key is malformed.
func (w *logicWalker) kind(n *model.LogicNode, m map[string]any, keys ...string) {
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		if s, ok := v.(string); ok && tokenPattern.MatchString(s) {
			n.Kind = s
		} else {
			n.Withheld++
		}
		return
	}
	w.partial(n.Path + " has no kind")
}

// values emits every key of m not in skip, in sorted key order.
func (w *logicWalker) values(n *model.LogicNode, m map[string]any, prefix string, depth int, skip ...string) {
	keys := make([]string, 0, len(m))
	for k := range m {
		if !slices.Contains(skip, k) {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	for _, k := range keys {
		if !tokenPattern.MatchString(k) || freeTextKeys[k] {
			if m[k] != nil {
				n.Withheld++
			}
			continue
		}
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		w.value(n, key, k, m[k], depth)
	}
}

// value classifies one value under key; last is the key's final component,
// which picks the grammar.
func (w *logicWalker) value(n *model.LogicNode, key, last string, v any, depth int) {
	switch x := v.(type) {
	case nil:
	case string:
		w.str(n, key, last, x)
	case bool, float64:
		if _, strict := strictKeys[last]; strict {
			n.Withheld++
			return
		}
		kind := model.ValueNumber
		if _, ok := x.(bool); ok {
			kind = model.ValueBool
		}
		w.add(n, model.TypedValue{Key: key, Kind: kind, Value: x})
	case []any:
		for _, item := range x {
			switch item.(type) {
			case map[string]any, []any:
				n.Withheld++
			default:
				w.value(n, key, last, item, depth)
			}
		}
	case map[string]any:
		if d, ok := durationObject(x); ok && durationKeys[last] {
			w.add(n, model.TypedValue{Key: key, Kind: model.ValueDuration, Value: d})
			return
		}
		if depth >= maxValueDepth {
			n.Withheld++
			return
		}
		w.values(n, x, key, depth+1)
	default:
		n.Withheld++
	}
}

func (w *logicWalker) str(n *model.LogicNode, key, last, s string) {
	if isTemplate(s) {
		w.addTemplate(n, key, s)
		return
	}
	if grammar, strict := strictKeys[last]; strict {
		// HA accepts a comma-separated id list in one string, as the
		// dependency extractor does.
		for part := range strings.SplitSeq(s, ",") {
			part = strings.TrimSpace(part)
			if !grammar.MatchString(part) {
				n.Withheld++
				continue
			}
			w.add(n, model.TypedValue{Key: key, Kind: strictKinds[last], Value: part})
		}
		return
	}
	switch {
	case numberPattern.MatchString(s):
		f, _ := strconv.ParseFloat(s, 64)
		w.add(n, model.TypedValue{Key: key, Kind: model.ValueNumber, Value: f})
	case durationKeys[last] && durationPattern.MatchString(s):
		w.add(n, model.TypedValue{Key: key, Kind: model.ValueDuration, Value: s})
	case timePattern.MatchString(s):
		w.add(n, model.TypedValue{Key: key, Kind: model.ValueTime, Value: s})
	case entityIDPattern.MatchString(s):
		w.add(n, model.TypedValue{Key: key, Kind: model.ValueEntity, Value: s})
	case tokenPattern.MatchString(s):
		w.add(n, model.TypedValue{Key: key, Kind: model.ValueToken, Value: s})
	default:
		n.Withheld++
	}
}

func (w *logicWalker) add(n *model.LogicNode, v model.TypedValue) {
	if len(n.Values)+len(n.Templates) >= maxValuesPerNode {
		w.truncated = true
		return
	}
	n.Values = append(n.Values, v)
}

func (w *logicWalker) addTemplate(n *model.LogicNode, key, text string) {
	if len(n.Values)+len(n.Templates) >= maxValuesPerNode {
		w.truncated = true
		return
	}
	t := model.Template{Key: key, Text: text}
	if len(text) > maxTemplateBytes {
		cut := maxTemplateBytes
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		t.Text, t.Truncated = text[:cut], true
	}
	n.Templates = append(n.Templates, t)
}

// isTemplate is D-09-2's template test: Jinja expression or statement
// delimiters anywhere in the string.
func isTemplate(s string) bool {
	return strings.Contains(s, "{{") || strings.Contains(s, "{%")
}

// durationObject normalizes HA's {hours, minutes, seconds, …} form to
// "HH:MM:SS". Every field must be a known unit holding a number; a template
// inside one fails this and is flattened as a template instead.
func durationObject(m map[string]any) (string, bool) {
	if len(m) == 0 {
		return "", false
	}
	var total float64
	for k, v := range m {
		unit, ok := durationUnits[k]
		f, isNum := v.(float64)
		if !ok || !isNum {
			return "", false
		}
		total += f * unit
	}
	sign := ""
	if total < 0 {
		sign, total = "-", -total
	}
	whole := int64(total)
	h, mnt, sec := whole/3600, whole%3600/60, whole%60
	if frac := total - math.Floor(total); frac > 0 {
		return fmt.Sprintf("%s%02d:%02d:%06.3f", sign, h, mnt, float64(sec)+frac), true
	}
	return fmt.Sprintf("%s%02d:%02d:%02d", sign, h, mnt, sec), true
}
