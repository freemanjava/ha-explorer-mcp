package model

import (
	"go/build"
	"testing"
)

// TestModel_ImportsNothingFromHA guards the dependency direction CLAUDE.md
// requires: internal/model imports nothing internal, so a change here can
// never reintroduce a raw HA JSON shape into the domain model by accident.
func TestModel_ImportsNothingFromHA(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("importing internal/model: %v", err)
	}
	for _, imp := range pkg.Imports {
		if imp == "github.com/freemanjava/ha-explorer-mcp/internal/ha" {
			t.Fatalf("internal/model imports internal/ha, violating the dependency direction (CLAUDE.md, Module Layout)")
		}
	}
}
