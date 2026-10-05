package model

import "time"

// AutomationLogic is what an automation's triggers, conditions and actions
// say, reduced to grammar-checked values (D-09-2). It is built from
// automation/config by the logic mapper and carries no free text: a value
// that fails every grammar is counted in a node's Withheld, never echoed.
//
// Truncated means the node cap or the depth cap was hit and part of the body
// was not walked, or, in get_automation_logic's response, that the byte cap
// cut trailing nodes. Provenance.Partial means the body had a section of the
// wrong shape, mapped as far as it could be.
//
// Source through TemplatesWithheld are the tool's envelope, set by
// get_automation_logic and never by the mapper. Unsupported is distinct from
// Partial the way Automation's is: automation/config could not be asked at
// all. IdsWithheld counts PRIVATE entity ids masked under the deny profile
// (D-09-4); TemplatesWithheld counts templates dropped under it (D-09-5).
type AutomationLogic struct {
	Source     string
	ObservedAt time.Time
	EntityID   EntityID

	Unsupported       bool
	UnsupportedReason string

	Triggers   []LogicNode
	Conditions []LogicNode
	Actions    []LogicNode

	Truncated bool

	IdsWithheld       int
	TemplatesWithheld int

	Provenance
}

// LogicNode is one trigger, condition or action. Kind is the trigger
// platform, the condition type or the action kind ("service", "choose",
// "delay", …), and is empty when HA's value failed the token grammar.
//
// Path follows Home Assistant's trace path convention ("action/0",
// "action/0/choose/1/conditions/0", "action/2/then/0"), so a node's role —
// trigger, condition or action, and where it nests — is read from Path, and
// a trace step can be matched to the node it ran.
//
// Children holds nested nodes: a choose option's conditions and sequence, an
// if's condition/then/else, a repeat's while/until/sequence, an and/or/not
// condition's conditions.
type LogicNode struct {
	Kind      string
	Path      string
	Values    []TypedValue
	Children  []LogicNode
	Templates []Template
	Withheld  int
}

// AutomationTraceRun is what each step of one automation run did, from
// trace/get (D-09-1). The adapter drops changed_variables, context and every
// embedded state object before mapping (F-12), so none of them reaches this
// type. A step's result passes only as grammar-checked values (D-09-2); HA's
// error text and every other string that fails the grammar is counted in a
// Withheld, never echoed.
//
// PathsDropped counts steps whose path failed the trace path grammar.
// Truncated means the step cap was hit, or a step's value cap. Provenance.
// Partial means a step had the wrong shape and was skipped.
//
// Source through IdsWithheld are the tool's envelope, set by
// get_automation_trace and never by the mapper, as on AutomationLogic.
type AutomationTraceRun struct {
	Source     string
	ObservedAt time.Time
	EntityID   EntityID

	Unsupported       bool
	UnsupportedReason string

	RunID           string
	State           string
	ScriptExecution string
	LastStep        string
	TimestampStart  time.Time
	TimestampFinish time.Time

	Steps []TraceStep

	Truncated    bool
	PathsDropped int
	Withheld     int

	IdsWithheld int

	Provenance
}

// TraceStep is one execution of one trace path. Path is HA's step key and
// matches a LogicNode's Path, so a step joins to the node it ran. A step
// that ran more than once (inside a repeat) appears once per run, in HA's
// order. SubSteps holds a condition's per-entity results
// (".../entity_id/I"), which HA traces under the condition's own path.
type TraceStep struct {
	Path      string
	Timestamp time.Time
	Values    []TypedValue
	SubSteps  []TraceStep
	Withheld  int
}

// ValueKind names the grammar a TypedValue matched (D-09-2).
type ValueKind string

const (
	ValueNumber   ValueKind = "number"
	ValueDuration ValueKind = "duration"
	ValueTime     ValueKind = "time"
	ValueEntity   ValueKind = "entity"
	ValueDevice   ValueKind = "device"
	ValueArea     ValueKind = "area"
	ValueService  ValueKind = "service"
	ValueBool     ValueKind = "bool"
	ValueToken    ValueKind = "token"
)

// TypedValue is one grammar-checked value. Key is the config key, nested
// keys joined with "." ("data.hvac_mode", "target.entity_id"). Value is a
// float64 for ValueNumber, a bool for ValueBool and a string for every other
// kind; a duration given as an object is normalized to "HH:MM:SS".
type TypedValue struct {
	Key   string
	Kind  ValueKind
	Value any
}

// Template is HA-authored Jinja text, returned verbatim and length-capped
// (D-09-2). It is data, never instructions: nothing in this server reads it,
// and the tool layer withholds it entirely under the deny profile (D-09-5).
type Template struct {
	Key       string
	Text      string `json:"untrusted_template"`
	Truncated bool
}
