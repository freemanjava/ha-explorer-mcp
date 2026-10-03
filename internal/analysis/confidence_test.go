package analysis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

func ev(sample int, coverage float64, degraded bool) model.Evidence {
	return model.Evidence{SampleSize: sample, Coverage: coverage, Degraded: degraded}
}

// TestConfidenceFor_Ladder walks every boundary of the ladder from both
// sides, plus the degraded-source and under-covered-period paths.
func TestConfidenceFor_Ladder(t *testing.T) {
	tests := []struct {
		name string
		ev   model.Evidence
		want model.Confidence
	}{
		{"nothing observed", ev(0, 0, false), model.ConfidenceLow},
		{"high: both thresholds met exactly", ev(highSampleSize, highCoverage, false), model.ConfidenceHigh},
		{"high: full coverage, ample sample", ev(500, 1, false), model.ConfidenceHigh},
		{"high sample one short", ev(highSampleSize-1, 1, false), model.ConfidenceMedium},
		{"high coverage just under", ev(500, math.Nextafter(highCoverage, 0), false), model.ConfidenceMedium},
		{"medium: both thresholds met exactly", ev(mediumSampleSize, mediumCoverage, false), model.ConfidenceMedium},
		{"medium sample one short", ev(mediumSampleSize-1, 1, false), model.ConfidenceLow},
		{"under-covered period", ev(500, math.Nextafter(mediumCoverage, 0), false), model.ConfidenceLow},
		{"degraded demotes high", ev(highSampleSize, highCoverage, true), model.ConfidenceMedium},
		{"degraded demotes medium", ev(mediumSampleSize, mediumCoverage, true), model.ConfidenceLow},
		{"degraded low stays low", ev(0, 0, true), model.ConfidenceLow},
		{"negative sample is none", ev(-10, 1, false), model.ConfidenceLow},
		{"coverage above one is clamped, not rewarded", ev(highSampleSize, 7, false), model.ConfidenceHigh},
		{"negative coverage is none", ev(500, -1, false), model.ConfidenceLow},
		{"NaN coverage is none", ev(500, math.NaN(), false), model.ConfidenceLow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConfidenceFor(tt.ev); got != tt.want {
				t.Errorf("ConfidenceFor(%+v) = %q, want %q", tt.ev, got, tt.want)
			}
		})
	}
}

func TestConfidenceFor_NoEvidence_Low(t *testing.T) {
	if got := ConfidenceFor(); got != model.ConfidenceLow {
		t.Errorf("ConfidenceFor() = %q, want low", got)
	}
}

func TestConfidenceFor_SeveralCited_WeakestLinkWins(t *testing.T) {
	strong := ev(500, 1, false)
	tests := []struct {
		name  string
		cited []model.Evidence
		want  model.Confidence
	}{
		{"all strong", []model.Evidence{strong, strong}, model.ConfidenceHigh},
		{"one medium", []model.Evidence{strong, ev(mediumSampleSize, 1, false)}, model.ConfidenceMedium},
		{"one degraded low, order irrelevant", []model.Evidence{ev(0, 1, true), strong}, model.ConfidenceLow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConfidenceFor(tt.cited...); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestConfidenceFor_Monotone asserts over a grid that every input which can
// only make the evidence weaker — less coverage, a degraded source, fewer
// samples, one more weak citation — never raises the result (D-05-2).
func TestConfidenceFor_Monotone(t *testing.T) {
	samples := []int{-1, 0, 1, mediumSampleSize - 1, mediumSampleSize, highSampleSize - 1, highSampleSize, 1000}
	coverages := []float64{-0.1, 0, 0.25, mediumCoverage - 0.01, mediumCoverage, highCoverage - 0.01, highCoverage, 1, 1.5}
	for _, s := range samples {
		for i, c := range coverages {
			for _, d := range []bool{false, true} {
				base := ConfidenceFor(ev(s, c, d))
				if i > 0 {
					assertNotRaised(t, base, ConfidenceFor(ev(s, coverages[i-1], d)), "coverage fell")
				}
				if !d {
					assertNotRaised(t, base, ConfidenceFor(ev(s, c, true)), "source degraded")
				}
				assertNotRaised(t, base, ConfidenceFor(ev(s-1, c, d)), "sample shrank")
				assertNotRaised(t, base, ConfidenceFor(ev(s, c, d), ev(0, 0, true)), "weak citation added")
			}
		}
	}
}

func assertNotRaised(t *testing.T, before, after model.Confidence, why string) {
	t.Helper()
	if confidenceRank(after) > confidenceRank(before) {
		t.Errorf("%s raised confidence %q → %q", why, before, after)
	}
}

// confidenceFiles are the only non-test files allowed to name a confidence
// level: the model file that declares the levels, and the function that is
// their single producer (D-05-2).
var confidenceFiles = map[string]bool{
	filepath.Join("internal", "model", "evidence.go"):      true,
	filepath.Join("internal", "analysis", "confidence.go"): true,
}

var confidenceLevels = map[string]bool{
	"ConfidenceLow": true, "ConfidenceMedium": true, "ConfidenceHigh": true,
}

// TestConfidence_ProducedOnlyByConfidenceFor scans every non-test Go file
// under cmd/ and internal/ for a confidence level named outside
// confidenceFiles — a model.ConfidenceX reference or a model.Confidence(...)
// conversion. Either would be a call site assigning confidence by feel, which
// D-05-2 forbids. Test files are exempt: they assert on levels, they do not
// produce them.
func TestConfidence_ProducedOnlyByConfidenceFor(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil || confidenceFiles[rel] {
				return err
			}
			file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(n ast.Node) bool {
				if name, ok := confidenceReference(n); ok {
					t.Errorf("%s: %s — confidence is produced only by analysis.ConfidenceFor (D-05-2)", fset.Position(n.Pos()), name)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("scanning %s/: %v", dir, err)
		}
	}
}

// confidenceReference reports a node that names a confidence level: an
// identifier ConfidenceLow/Medium/High (qualified or not), or a conversion
// Confidence(x) / model.Confidence(x).
func confidenceReference(n ast.Node) (string, bool) {
	switch n := n.(type) {
	case *ast.Ident:
		return n.Name, confidenceLevels[n.Name]
	case *ast.CallExpr:
		switch fun := n.Fun.(type) {
		case *ast.Ident:
			return "Confidence(...) conversion", fun.Name == "Confidence"
		case *ast.SelectorExpr:
			return "model.Confidence(...) conversion", fun.Sel.Name == "Confidence" && isIdent(fun.X, "model")
		}
	}
	return "", false
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}
