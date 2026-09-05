package analysis

import (
	"go/build"
	"strings"
	"testing"
)

// TestAnalysis_ImportsOnlyModel guards the dependency direction CLAUDE.md
// requires: internal/analysis depends on internal/model and nothing else
// internal, so a metric can never start fetching its own data. The test
// files are exempt on purpose — they read captured fixtures through
// internal/ha's mapper, which is what makes the numbers reproducible from a
// real payload rather than from hand-built structs.
func TestAnalysis_ImportsOnlyModel(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("importing internal/analysis: %v", err)
	}
	const prefix = "github.com/freemanjava/ha-explorer-mcp/internal/"
	for _, imp := range pkg.Imports {
		if !strings.HasPrefix(imp, prefix) {
			continue
		}
		if strings.TrimPrefix(imp, prefix) != "model" {
			t.Errorf("internal/analysis imports %s; only internal/model is allowed (CLAUDE.md, Module Layout)", imp)
		}
	}
}
