package model

// AutomationLogic is what an automation's triggers, conditions and actions
// say, reduced to grammar-checked values (D-09-2). It is built from
// automation/config by the logic mapper and carries no free text: a value
// that fails every grammar is counted in a node's Withheld, never echoed.
//
// Truncated means the node cap or the depth cap was hit and part of the body
// was not walked. Provenance.Partial means the body had a section of the
// wrong shape, mapped as far as it could be.
type AutomationLogic struct {
	Triggers   []LogicNode
	Conditions []LogicNode
	Actions    []LogicNode

	Truncated bool

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
	Text      string
	Truncated bool
}
