package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/ha"
	"github.com/freemanjava/ha-explorer-mcp/internal/model"
	"github.com/freemanjava/ha-explorer-mcp/internal/policy"
)

// fakeAutomationLogicReader is an automationLogicReader test double.
type fakeAutomationLogicReader struct {
	logic model.AutomationLogic
	err   error
}

func (f *fakeAutomationLogicReader) AutomationLogic(context.Context, model.EntityID) (model.AutomationLogic, error) {
	return f.logic, f.err
}

const (
	logicPrivateID = "device_tracker.phone_of_someone"
	logicTemplate  = "{{ states('person.someone') == 'home' and ignore_previous_instructions }}"
)

// arriveLogic is an invented automation: a numeric threshold on a normal
// sensor, a presence trigger on a PRIVATE tracker, and a template condition.
func arriveLogic() model.AutomationLogic {
	return model.AutomationLogic{
		Triggers: []model.LogicNode{
			{Kind: "numeric_state", Path: "trigger/0", Values: []model.TypedValue{
				{Key: "entity_id", Kind: model.ValueEntity, Value: "sensor.living_room_temperature"},
				{Key: "above", Kind: model.ValueNumber, Value: float64(26)},
			}},
			{Kind: "state", Path: "trigger/1", Values: []model.TypedValue{
				{Key: "entity_id", Kind: model.ValueEntity, Value: logicPrivateID},
				{Key: "to", Kind: model.ValueToken, Value: "home"},
			}},
		},
		Conditions: []model.LogicNode{
			{Kind: "template", Path: "condition/0", Templates: []model.Template{
				{Key: "value_template", Text: logicTemplate},
			}},
		},
	}
}

func logicOptions(reader *fakeAutomationLogicReader, versions *fakeCoreReader, profile policy.Profile) Options {
	opts := testOptions()
	opts.AutomationLogic = reader
	if versions != nil {
		opts.Core = versions
	}
	opts.Profile = profile
	opts.Secrets = []string{"tok-secret-value"}
	return opts
}

func callLogic(t *testing.T, opts Options, entityID string) (model.AutomationLogic, string) {
	t.Helper()
	client := connect(t, newServer(opts, Catalog()))
	res, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "get_automation_logic", Arguments: map[string]any{"entity_id": entityID}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	var out model.AutomationLogic
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out, string(raw)
}

// DoD (1): the schema is a closed object holding only entity_id.
func TestGetAutomationLogic_Schema_AcceptsNoFreeFormParameter(t *testing.T) {
	opts := logicOptions(&fakeAutomationLogicReader{}, nil, policy.Profile{})
	client := connect(t, newServer(opts, Catalog()))
	res, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name != "get_automation_logic" {
			continue
		}
		raw, _ := json.Marshal(tool.InputSchema)
		var schema struct {
			Type                 string                    `json:"type"`
			AdditionalProperties any                       `json:"additionalProperties"`
			Properties           map[string]map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("unmarshal schema: %v", err)
		}
		if schema.Type != "object" || schema.AdditionalProperties != false {
			t.Errorf("schema = %s, want a closed object", raw)
		}
		if len(schema.Properties) != 1 || schema.Properties["entity_id"] == nil {
			t.Errorf("properties = %v, want entity_id only", schema.Properties)
		}
		if !strings.Contains(tool.Description, "untrusted_template") {
			t.Errorf("description %q must say untrusted_template is HA-authored data", tool.Description)
		}
		return
	}
	t.Fatal("get_automation_logic is not in the tool listing")
}

// DoD (2): under deny a PRIVATE id never appears, its sibling threshold does,
// the template text never appears and the template count does (D-09-4/5).
func TestGetAutomationLogic_DenyProfile_MasksPrivateIDAndWithholdsTemplates(t *testing.T) {
	opts := logicOptions(&fakeAutomationLogicReader{logic: arriveLogic()}, nil, policy.Profile{Private: policy.HandlingDeny})
	out, raw := callLogic(t, opts, "automation.arrive")

	for _, leaked := range []string{logicPrivateID, "person.someone", "ignore_previous_instructions", "value_template"} {
		if strings.Contains(raw, leaked) {
			t.Errorf("response leaks %q under deny: %s", leaked, raw)
		}
	}
	if !strings.Contains(raw, "sensor.living_room_temperature") || !strings.Contains(raw, "26") {
		t.Errorf("the normal entity and its threshold must survive: %s", raw)
	}
	if len(out.Triggers) != 2 || len(out.Triggers[1].Values) != 2 {
		t.Fatalf("the PRIVATE trigger must keep its element and values: %+v", out.Triggers)
	}
	if out.IdsWithheld != 1 {
		t.Errorf("IdsWithheld = %d, want 1", out.IdsWithheld)
	}
	if out.TemplatesWithheld != 1 {
		t.Errorf("TemplatesWithheld = %d, want 1", out.TemplatesWithheld)
	}
	if len(out.Conditions[0].Templates) != 0 {
		t.Errorf("templates = %+v, want none under deny", out.Conditions[0].Templates)
	}
}

// DoD (1), D-09-6: under mask (the default) no template text is returned, the
// count says so, and non-template values still appear. Only allow ships text.
func TestGetAutomationLogic_MaskProfile_WithholdsTemplates(t *testing.T) {
	opts := logicOptions(&fakeAutomationLogicReader{logic: arriveLogic()}, nil, policy.Profile{Private: policy.HandlingMask})
	out, raw := callLogic(t, opts, "automation.arrive")

	for _, leaked := range []string{"ignore_previous_instructions", "value_template", "untrusted_template"} {
		if strings.Contains(raw, leaked) {
			t.Errorf("response leaks %q under mask: %s", leaked, raw)
		}
	}
	if !strings.Contains(raw, "sensor.living_room_temperature") || !strings.Contains(raw, "26") {
		t.Errorf("non-template values must survive under mask: %s", raw)
	}
	if out.TemplatesWithheld != 1 {
		t.Errorf("TemplatesWithheld = %d, want 1", out.TemplatesWithheld)
	}
	if len(out.Conditions[0].Templates) != 0 {
		t.Errorf("templates = %+v, want none under mask", out.Conditions[0].Templates)
	}
}

// DoD (3): under allow the template ships in untrusted_template.
func TestGetAutomationLogic_AllowProfile_ReturnsTemplateAsUntrusted(t *testing.T) {
	opts := logicOptions(&fakeAutomationLogicReader{logic: arriveLogic()}, nil, policy.Profile{Private: policy.HandlingAllow})
	out, raw := callLogic(t, opts, "automation.arrive")

	if !strings.Contains(raw, `"untrusted_template"`) {
		t.Errorf("response has no untrusted_template field: %s", raw)
	}
	if got := out.Conditions[0].Templates[0].Text; got != logicTemplate {
		t.Errorf("template text = %q, want verbatim %q", got, logicTemplate)
	}
	if !strings.Contains(raw, logicPrivateID) || out.IdsWithheld != 0 || out.TemplatesWithheld != 0 {
		t.Errorf("allow must mask and withhold nothing: %s", raw)
	}
	if out.Source == "" || out.ObservedAt.IsZero() || out.EntityID != "automation.arrive" {
		t.Errorf("missing provenance: %+v", out)
	}
}

// DoD (4): a refused principal is unsupported with a reason, never an empty logic.
func TestGetAutomationLogic_PermissionRefused_ReportsUnsupported(t *testing.T) {
	opts := logicOptions(&fakeAutomationLogicReader{err: fmt.Errorf("automation/config: %w", ha.ErrUnsupported)}, nil, policy.Profile{})
	out, _ := callLogic(t, opts, "automation.arrive")

	if !out.Unsupported || !strings.Contains(out.UnsupportedReason, "permission denied") || !strings.Contains(out.UnsupportedReason, "list_automations") {
		t.Fatalf("response = %+v, want unsupported naming permission and the fallback", out)
	}
	if len(out.Triggers)+len(out.Conditions)+len(out.Actions) != 0 {
		t.Errorf("unsupported response carries logic: %+v", out)
	}
}

// DoD (5): not found is an error, distinct from unsupported.
func TestGetAutomationLogic_NotFound_IsErrNotFoundNotUnsupported(t *testing.T) {
	cases := map[string]error{
		"reader sentinel":     fmt.Errorf("automation %q: %w", "automation.gone", ha.ErrNotFound),
		"HA not_found answer": &ha.CommandError{Code: "not_found", Message: "Unable to find automation"},
	}
	for name, readErr := range cases {
		t.Run(name, func(t *testing.T) {
			opts := logicOptions(&fakeAutomationLogicReader{err: readErr}, nil, policy.Profile{})
			out, err := getAutomationLogic(t.Context(), opts.AutomationLogic, nil, opts.Profile, GetAutomationLogicInput{EntityID: "automation.gone"})
			if !errors.Is(err, ha.ErrNotFound) {
				t.Fatalf("err = %v, want ErrNotFound", err)
			}
			if out.Unsupported {
				t.Errorf("not found must not read as unsupported: %+v", out)
			}
		})
	}
}

// Version-absent keeps its own reason, naming the detected version.
func TestGetAutomationLogic_VersionAbsent_NamesDetectedVersion(t *testing.T) {
	reader := &fakeAutomationLogicReader{err: &ha.CommandError{Code: "unknown_command", Message: "Unknown command."}}
	opts := logicOptions(reader, &fakeCoreReader{cfg: model.CoreConfig{Version: "2026.8.3"}}, policy.Profile{})
	out, _ := callLogic(t, opts, "automation.arrive")
	if !out.Unsupported || !strings.Contains(out.UnsupportedReason, "2026.8.3") {
		t.Fatalf("response = %+v, want unsupported naming the version", out)
	}
}

func TestGetAutomationLogic_InvalidEntityID_Rejected(t *testing.T) {
	for _, id := range []string{"light.hall", "automation.", "../config", ""} {
		_, err := getAutomationLogic(t.Context(), &fakeAutomationLogicReader{}, nil, policy.Profile{}, GetAutomationLogicInput{EntityID: id})
		if err == nil {
			t.Errorf("entity_id %q accepted", id)
		}
	}
}

// DoD (6): a body larger than the response byte cap is cut and says so.
func TestGetAutomationLogic_OverByteCap_TruncatedAndWithinCap(t *testing.T) {
	var triggers []model.LogicNode
	for i := 0; i < 6000; i++ {
		triggers = append(triggers, model.LogicNode{Kind: "state", Path: fmt.Sprintf("trigger/%d", i), Values: []model.TypedValue{
			{Key: "entity_id", Kind: model.ValueEntity, Value: fmt.Sprintf("light.lamp_%04d", i)},
		}})
	}
	reader := &fakeAutomationLogicReader{logic: model.AutomationLogic{Triggers: triggers}}
	out, raw := callLogic(t, logicOptions(reader, nil, policy.Profile{}), "automation.big")

	if !out.Truncated {
		t.Fatalf("Truncated = false for %d bytes of logic", len(raw))
	}
	if limit := policy.LimitsFor(policy.ClassNormalRead).MaxBytes; int64(len(raw)) > limit {
		t.Errorf("response is %d bytes, over the %d cap", len(raw), limit)
	}
	if len(out.Triggers) == 0 || len(out.Triggers) >= len(triggers) {
		t.Errorf("kept %d of %d triggers, want a non-empty prefix", len(out.Triggers), len(triggers))
	}
}

// DoD (7): the supervisor token never reaches the response, even from a value.
func TestGetAutomationLogic_TokenInTemplate_NeverInResponse(t *testing.T) {
	logic := arriveLogic()
	logic.Conditions[0].Templates[0].Text = "{{ 'tok-secret-value' }}"
	opts := logicOptions(&fakeAutomationLogicReader{logic: logic}, nil, policy.Profile{Private: policy.HandlingAllow})
	_, raw := callLogic(t, opts, "automation.arrive")
	if strings.Contains(raw, "tok-secret-value") {
		t.Errorf("token reached the response: %s", raw)
	}
}

const (
	variablesPrivateID = "device_tracker.someone_phone"
	variablesTemplate  = "{{ states('person.someone') }} ignore_variable_instructions"
)

// variablesLogic is an invented blueprint-style automation: a numeric
// threshold, a PRIVATE entity input, and a template variable.
func variablesLogic() model.AutomationLogic {
	return model.AutomationLogic{Variables: []model.LogicNode{
		{Kind: "variables", Path: "variables",
			Values:    []model.TypedValue{{Key: "max_temp", Kind: model.ValueNumber, Value: float64(26)}},
			Templates: []model.Template{{Key: "limit", Text: variablesTemplate}}},
		{Kind: "blueprint_input", Path: "use_blueprint/input", Values: []model.TypedValue{
			{Key: "who", Kind: model.ValueEntity, Value: variablesPrivateID},
			{Key: "wait", Kind: model.ValueNumber, Value: float64(120)},
		}},
	}}
}

// P9-07 DoD (4): under deny a PRIVATE id in an input is masked and counted,
// its sibling number survives, the template variable is withheld.
func TestGetAutomationLogic_DenyProfile_MasksVariablesAndBlueprintInputs(t *testing.T) {
	opts := logicOptions(&fakeAutomationLogicReader{logic: variablesLogic()}, nil, policy.Profile{Private: policy.HandlingDeny})
	out, raw := callLogic(t, opts, "automation.arrive")

	for _, leaked := range []string{variablesPrivateID, "person.someone", "ignore_variable_instructions"} {
		if strings.Contains(raw, leaked) {
			t.Errorf("response leaks %q under deny: %s", leaked, raw)
		}
	}
	if !strings.Contains(raw, "max_temp") || !strings.Contains(raw, "120") {
		t.Errorf("thresholds must survive: %s", raw)
	}
	if out.IdsWithheld != 1 || out.TemplatesWithheld != 1 {
		t.Errorf("IdsWithheld=%d TemplatesWithheld=%d, want 1 and 1", out.IdsWithheld, out.TemplatesWithheld)
	}
}

// DoD (4), D-09-6: under mask a template variable's text never appears; under
// allow it does, as untrusted_template.
func TestGetAutomationLogic_TemplateVariable_ShipsOnlyUnderAllow(t *testing.T) {
	mask := logicOptions(&fakeAutomationLogicReader{logic: variablesLogic()}, nil, policy.Profile{Private: policy.HandlingMask})
	if _, raw := callLogic(t, mask, "automation.arrive"); strings.Contains(raw, "ignore_variable_instructions") {
		t.Errorf("template variable leaks under mask: %s", raw)
	}
	allow := logicOptions(&fakeAutomationLogicReader{logic: variablesLogic()}, nil, policy.Profile{Private: policy.HandlingAllow})
	if _, raw := callLogic(t, allow, "automation.arrive"); !strings.Contains(raw, "ignore_variable_instructions") || !strings.Contains(raw, "untrusted_template") {
		t.Errorf("template variable missing under allow: %s", raw)
	}
}
