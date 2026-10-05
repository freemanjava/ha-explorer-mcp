package ha

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// Caps on one trace run (phase 09 Design Notes, "Cost"). Starting values, not
// measurements: P9-05 measures real runs on the Pi before either is tuned.
const (
	// maxTraceSteps bounds the steps one run maps, sub-steps included. A
	// repeat can run one step thousands of times; a hand-written automation's
	// run has tens.
	maxTraceSteps = 500
	// maxTracePathBytes bounds one step key. HA's real paths are tens of
	// bytes even for deeply nested choose/repeat blocks.
	maxTracePathBytes = 256
)

var (
	// tracePathPattern is a step key's grammar (phase 09 Design Notes). It
	// admits a bare "trigger", which HA writes for a run with no trigger index
	// (a manual run, F-54).
	tracePathPattern = regexp.MustCompile(`^(trigger|condition|action)(/[a-z0-9_]+)*$`)
	// entitySubStepPattern matches a state or numeric_state condition's
	// per-entity result, which HA traces under the condition's own path
	// (docs/research/2026-10-05-ha-trace-paths.md).
	entitySubStepPattern = regexp.MustCompile(`^(.+)/entity_id/\d+$`)
)

// droppedTraceKeys are removed at every depth before a step result is mapped.
// They hold whole entity states, the run's variables or the caller's
// identity (F-12): nothing in them is a step's outcome.
var droppedTraceKeys = map[string]bool{
	"changed_variables": true,
	"context":           true,
	"from_state":        true,
	"to_state":          true,
	"this":              true,
	"attributes":        true,
}

// traceRunWire is the strictly-typed run level of a trace/get result. As for
// trace/list (traceSummaryWire), a retyped run-level field fails the whole
// call. Steps are read separately and tolerantly: one malformed step marks
// the run partial instead. config, context, item_id and blueprint_inputs are
// not decoded at all.
type traceRunWire struct {
	RunID           string          `json:"run_id"`
	State           string          `json:"state"`
	ScriptExecution string          `json:"script_execution"`
	LastStep        string          `json:"last_step"`
	Error           json.RawMessage `json:"error"`
	Timestamp       struct {
		Start  time.Time  `json:"start"`
		Finish *time.Time `json:"finish"`
	} `json:"timestamp"`
	Trace json.RawMessage `json:"trace"`
}

// traceStepWire is one element of a trace path's list. Its own path field
// is ignored: the key it sits under is the one validated.
type traceStepWire struct {
	Timestamp time.Time       `json:"timestamp"`
	Error     json.RawMessage `json:"error"`
	Result    json.RawMessage `json:"result"`
}

// traceEntry is one key of the trace object, in HA's order.
type traceEntry struct {
	path  string
	steps json.RawMessage
}

// MapAutomationTraceRun maps a trace/get result to typed steps (P9-03,
// D-09-1). changed_variables, context and every embedded state object are
// dropped before a result is read (F-12). A step result passes only as
// D-09-2 grammar values; error text, templates and every other string are
// counted in a Withheld. Steps keep HA's execution order, and a condition's
// ".../entity_id/I" results nest under it as SubSteps.
func MapAutomationTraceRun(raw json.RawMessage) (model.AutomationTraceRun, error) {
	var wire traceRunWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return model.AutomationTraceRun{}, fmt.Errorf("%w: decoding trace/get: %v", ErrUnexpectedMessage, err)
	}
	entries, err := orderedTraceEntries(wire.Trace)
	if err != nil {
		return model.AutomationTraceRun{}, fmt.Errorf("%w: decoding trace/get trace: %v", ErrUnexpectedMessage, err)
	}

	run := model.AutomationTraceRun{TimestampStart: wire.Timestamp.Start.UTC()}
	if wire.Timestamp.Finish != nil {
		run.TimestampFinish = wire.Timestamp.Finish.UTC()
	}
	// A run id is opaque (a uuid hex, a ULID); the device/area id shape
	// admits exactly those.
	run.RunID = runField(&run, wire.RunID, dependencyIDPattern.MatchString)
	run.State = runField(&run, wire.State, tokenPattern.MatchString)
	run.ScriptExecution = runField(&run, wire.ScriptExecution, tokenPattern.MatchString)
	run.LastStep = runField(&run, wire.LastStep, validTracePath)
	if present(wire.Error) {
		run.Withheld++
	}

	m := &traceMapper{run: &run}
	m.steps(entries)
	if m.w.truncated {
		run.Truncated = true
	}
	if len(m.w.reasons) > 0 {
		run.Partial = true
		run.PartialReason = strings.Join(m.w.reasons, "; ")
	}
	return run, nil
}

// orderedTraceEntries decodes the trace object key by key. HA writes it in
// execution order, which a Go map would lose; the order is how a reader
// sees which step ran last.
func orderedTraceEntries(raw json.RawMessage) ([]traceEntry, error) {
	if raw == nil {
		return nil, errors.New("trace missing")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, errors.New("trace is not an object")
	}
	var out []traceEntry
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := tok.(string)
		var steps json.RawMessage
		if err := dec.Decode(&steps); err != nil {
			return nil, err
		}
		out = append(out, traceEntry{path: key, steps: steps})
	}
	return out, nil
}

// traceMapper reuses the logic walker's value grammar, caps and partial
// reasons for one run's steps.
type traceMapper struct {
	run   *model.AutomationTraceRun
	w     logicWalker
	count int
}

// steps maps every entry whose path passes the grammar, then nests each
// entity sub-step under its condition. A sub-step whose condition has no
// step of its own stays top-level rather than being lost.
func (m *traceMapper) steps(entries []traceEntry) {
	valid := make(map[string]bool, len(entries))
	for _, e := range entries {
		if validTracePath(e.path) {
			valid[e.path] = true
		} else {
			m.run.PathsDropped++
		}
	}

	type sub struct {
		parent string
		iter   int
		step   model.TraceStep
	}
	var subs []sub
	byPath := map[string][]int{}
	for _, e := range entries {
		if !valid[e.path] {
			continue
		}
		parent := ""
		if g := entitySubStepPattern.FindStringSubmatch(e.path); g != nil && valid[g[1]] {
			parent = g[1]
		}
		for i, s := range m.elements(e) {
			if parent != "" {
				subs = append(subs, sub{parent: parent, iter: i, step: s})
				continue
			}
			byPath[e.path] = append(byPath[e.path], len(m.run.Steps))
			m.run.Steps = append(m.run.Steps, s)
		}
	}
	// A condition inside a repeat runs once per iteration, and so do its
	// entity checks: the i-th sub-step result belongs to the i-th run of the
	// condition.
	for _, s := range subs {
		idx := byPath[s.parent]
		if len(idx) == 0 {
			m.run.Steps = append(m.run.Steps, s.step)
			continue
		}
		p := &m.run.Steps[idx[min(s.iter, len(idx)-1)]]
		p.SubSteps = append(p.SubSteps, s.step)
	}
}

// elements maps one path's list, stopping at the step cap.
func (m *traceMapper) elements(e traceEntry) []model.TraceStep {
	var raws []json.RawMessage
	if err := json.Unmarshal(e.steps, &raws); err != nil {
		m.w.partial(e.path + " is not a list")
		return nil
	}
	var out []model.TraceStep
	for i, r := range raws {
		if m.count >= maxTraceSteps {
			m.w.truncated = true
			return out
		}
		var sw traceStepWire
		if err := json.Unmarshal(r, &sw); err != nil {
			m.w.partial(e.path + "[" + strconv.Itoa(i) + "] is malformed")
			continue
		}
		m.count++
		step := model.TraceStep{Path: e.path, Timestamp: sw.Timestamp.UTC()}
		if present(sw.Error) {
			step.Withheld++
		}
		m.result(&step, sw.Result)
		out = append(out, step)
	}
	return out
}

// result reads a step's result object into grammar values. Templates are
// counted, not kept: the trace carries no untrusted_template field, so no
// privacy profile has to be applied to one (D-09-6).
func (m *traceMapper) result(step *model.TraceStep, raw json.RawMessage) {
	if !present(raw) {
		return
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		step.Withheld++
		return
	}
	r, ok := scrubTrace(v).(map[string]any)
	if !ok {
		step.Withheld++
		return
	}
	if e, ok := r["error"]; ok {
		if e != nil {
			step.Withheld++
		}
		delete(r, "error")
	}
	if p, ok := r["params"].(map[string]any); ok {
		joinServiceParams(p)
	}
	n := model.LogicNode{Path: step.Path}
	m.w.values(&n, r, "", 0)
	slices.SortStableFunc(n.Values, func(a, b model.TypedValue) int { return strings.Compare(a.Key, b.Key) })
	step.Values = n.Values
	step.Withheld += n.Withheld + len(n.Templates)
}

// joinServiceParams reports a service call's domain and service as one
// action: value, as the logic mapper reports the legacy service: key, so the
// value is checked by the service grammar rather than as two tokens.
func joinServiceParams(p map[string]any) {
	d, dok := p["domain"].(string)
	s, sok := p["service"].(string)
	if !dok || !sok {
		return
	}
	delete(p, "domain")
	delete(p, "service")
	p["action"] = d + "." + s
}

// scrubTrace returns v without droppedTraceKeys and without any embedded
// state object, at every depth. It decides by the payload's schema keys,
// never by HA-authored content (rule 6).
func scrubTrace(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, c := range x {
			if droppedTraceKeys[k] || isStateObject(c) {
				continue
			}
			out[k] = scrubTrace(c)
		}
		return out
	case []any:
		out := make([]any, 0, len(x))
		for _, c := range x {
			if !isStateObject(c) {
				out = append(out, scrubTrace(c))
			}
		}
		return out
	default:
		return v
	}
}

// isStateObject recognizes HA's State.as_dict shape wherever it is embedded
// under a key droppedTraceKeys does not name.
func isStateObject(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	_, id := m["entity_id"]
	_, st := m["state"]
	_, attrs := m["attributes"]
	return id && st && attrs
}

// runField returns s when it passes ok; otherwise it counts a non-empty s
// as withheld and returns "". Run-level text is HA data like any other.
func runField(run *model.AutomationTraceRun, s string, ok func(string) bool) string {
	if s == "" || ok(s) {
		return s
	}
	run.Withheld++
	return ""
}

func validTracePath(p string) bool {
	return len(p) <= maxTracePathBytes && tracePathPattern.MatchString(p)
}

// present reports whether a raw JSON field was given a non-null value.
func present(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}
