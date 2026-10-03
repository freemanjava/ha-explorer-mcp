package model

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// inferenceFieldNames are the field names that would let an Evidence value
// carry a conclusion alongside its measurement — the one-object shape D-05-1
// rejected. Compared after normalize.
var inferenceFieldNames = []string{
	"inference", "hypothesis", "hypotheses", "statement", "confidence",
	"recommendation", "nextaction", "conclusion", "cause", "rootcause",
}

// factFieldNames are the fields an agent could read off a Hypothesis as a
// measured fact rather than as an inference about one. Compared after
// normalize.
var factFieldNames = []string{
	"observation", "source", "measurements", "facts", "metrics", "value",
	"count", "from", "to", "period", "observedat",
}

func normalize(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", ""))
}

func fieldNames(t reflect.Type) []string {
	names := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		names = append(names, normalize(t.Field(i).Name))
	}
	return names
}

func assertNoField(t *testing.T, typ reflect.Type, forbidden []string) {
	t.Helper()
	for _, name := range fieldNames(typ) {
		for _, f := range forbidden {
			if name == f {
				t.Errorf("%s has field %q: fact and inference must stay separate types (D-05-1)", typ.Name(), name)
			}
		}
	}
}

func TestEvidence_HasNoInferenceField(t *testing.T) {
	assertNoField(t, reflect.TypeOf(Evidence{}), inferenceFieldNames)
}

func TestHypothesis_HasNoFactField(t *testing.T) {
	assertNoField(t, reflect.TypeOf(Hypothesis{}), factFieldNames)
}

func TestMissingEvidenceAndNextAction_HaveNoInferenceField(t *testing.T) {
	assertNoField(t, reflect.TypeOf(MissingEvidence{}), inferenceFieldNames)
	assertNoField(t, reflect.TypeOf(NextAction{}), []string{"inference", "hypothesis", "confidence", "cause", "rootcause"})
}

// TestHypothesis_FieldsUnexported guards the constructor: with every field
// unexported, a Hypothesis literal outside this package cannot set a
// statement or a citation, so NewHypothesis is the only way to build a
// meaningful one.
func TestHypothesis_FieldsUnexported(t *testing.T) {
	typ := reflect.TypeOf(Hypothesis{})
	for i := range typ.NumField() {
		if f := typ.Field(i); f.IsExported() {
			t.Errorf("Hypothesis.%s is exported; a literal could bypass NewHypothesis's citation check", f.Name)
		}
	}
}

func TestNewHypothesis_NoCitedEvidence_Refused(t *testing.T) {
	cases := map[string][]EvidenceID{
		"nil":      nil,
		"empty":    {},
		"blank id": {""},
	}
	for name, cites := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewHypothesis("integration outage", ConfidenceLow, cites...)
			if !errors.Is(err, ErrUncitedHypothesis) {
				t.Fatalf("err = %v, want ErrUncitedHypothesis", err)
			}
		})
	}
}

func TestNewHypothesis_InvalidInput_Refused(t *testing.T) {
	if _, err := NewHypothesis("", ConfidenceLow, "e1"); !errors.Is(err, ErrInvalidHypothesis) {
		t.Errorf("empty statement: err = %v, want ErrInvalidHypothesis", err)
	}
	if _, err := NewHypothesis("x", Confidence("certain"), "e1"); !errors.Is(err, ErrInvalidHypothesis) {
		t.Errorf("unknown confidence: err = %v, want ErrInvalidHypothesis", err)
	}
}

func TestNewHypothesis_Cited_KeepsCitationsInOrderWithoutDuplicates(t *testing.T) {
	h, err := NewHypothesis("integration outage", ConfidenceMedium, "e2", "e1", "e2")
	if err != nil {
		t.Fatalf("NewHypothesis: %v", err)
	}
	if got, want := h.Cites(), []EvidenceID{"e2", "e1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Cites() = %v, want %v", got, want)
	}
	if h.Statement() != "integration outage" || h.Confidence() != ConfidenceMedium {
		t.Errorf("accessors = %q/%q", h.Statement(), h.Confidence())
	}
}

func TestHypothesis_Cites_ReturnsCopy(t *testing.T) {
	h, err := NewHypothesis("x", ConfidenceHigh, "e1")
	if err != nil {
		t.Fatalf("NewHypothesis: %v", err)
	}
	h.Cites()[0] = "tampered"
	if h.Cites()[0] != "e1" {
		t.Fatal("mutating the returned slice changed the Hypothesis")
	}
}

// TestInternal_NoCauseField scans every struct in internal/ — field names and
// json tags — for cause/root_cause. D-05-1: an outage correlation is
// evidence, never an established cause, so no type anywhere may offer a slot
// to put one in. A source scan rather than reflection over registered types,
// because reflection cannot enumerate a package's types and a type nobody
// listed is exactly the one that would slip through.
func TestInternal_NoCauseField(t *testing.T) {
	forbidden := map[string]bool{"cause": true, "causes": true, "rootcause": true, "rootcauses": true}
	fset := token.NewFileSet()
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range st.Fields.List {
				for _, name := range structFieldNames(field) {
					if forbidden[normalize(name)] {
						t.Errorf("%s: struct field %q — no type may carry a cause (D-05-1)", fset.Position(field.Pos()), name)
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scanning internal/: %v", err)
	}
}

// structFieldNames returns a field's Go names plus its json tag name, so a
// field named innocuously but serialized as "root_cause" is caught too.
func structFieldNames(field *ast.Field) []string {
	var names []string
	for _, n := range field.Names {
		names = append(names, n.Name)
	}
	if field.Tag == nil {
		return names
	}
	tag, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		return names
	}
	if json, ok := reflect.StructTag(tag).Lookup("json"); ok {
		names = append(names, strings.Split(json, ",")[0])
	}
	return names
}
