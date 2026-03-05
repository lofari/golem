// internal/scaffold/scaffold_test.go
package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lofari/golem/internal/ctx"
)

func TestInit(t *testing.T) {
	dir := t.TempDir()

	result, err := Init(dir, InitOptions{
		Name:     "TestProject",
		Stack:    "Go",
		DocsPath: "docs/plans",
	})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Check files created
	if len(result.Created) != 5 {
		t.Errorf("Created %d files, want 5: %v", len(result.Created), result.Created)
	}

	// Check state.yaml was pre-filled
	state, err := ctx.ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if state.Project.Name != "TestProject" {
		t.Errorf("Project.Name = %q, want %q", state.Project.Name, "TestProject")
	}
	if state.Project.DocsPath != "docs/plans" {
		t.Errorf("DocsPath = %q, want %q", state.Project.DocsPath, "docs/plans")
	}

	// Check CLAUDE.md created
	claudeData, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("reading CLAUDE.md: %v", err)
	}
	if !strings.Contains(string(claudeData), "<!-- golem:start -->") {
		t.Error("CLAUDE.md missing golem markers")
	}
}

func TestInitIdempotent(t *testing.T) {
	dir := t.TempDir()

	// First init
	_, err := Init(dir, InitOptions{Name: "First"})
	if err != nil {
		t.Fatalf("first Init: %v", err)
	}

	// Modify state to verify it's preserved
	state, _ := ctx.ReadState(dir)
	state.Tasks = []ctx.Task{{Name: "existing", Status: "done"}}
	ctx.WriteState(dir, state)

	// Second init — should skip existing files
	result, err := Init(dir, InitOptions{Name: "Second"})
	if err != nil {
		t.Fatalf("second Init: %v", err)
	}

	// state.yaml should be in skipped (not overwritten)
	hasSkipped := false
	for _, s := range result.Skipped {
		if s == ".ctx/state.yaml" {
			hasSkipped = true
		}
	}
	if !hasSkipped {
		t.Error("second init should skip existing state.yaml")
	}

	// But the state should still have our task (file wasn't overwritten)
	state2, _ := ctx.ReadState(dir)
	if len(state2.Tasks) != 1 {
		t.Errorf("existing tasks lost after second init")
	}
}

func TestInjectClaudeMDReplace(t *testing.T) {
	dir := t.TempDir()
	claudePath := filepath.Join(dir, "CLAUDE.md")

	// Create CLAUDE.md with existing markers and surrounding content
	existing := "# My Project\n\nSome content.\n\n<!-- golem:start -->\nold section\n<!-- golem:end -->\n\nMore content.\n"
	os.WriteFile(claudePath, []byte(existing), 0644)

	action, err := injectClaudeMD(dir)
	if err != nil {
		t.Fatalf("injectClaudeMD: %v", err)
	}
	if action != "updated" {
		t.Errorf("action = %q, want %q", action, "updated")
	}

	data, _ := os.ReadFile(claudePath)
	content := string(data)
	if !strings.Contains(content, "# My Project") {
		t.Error("lost content before markers")
	}
	if !strings.Contains(content, "More content.") {
		t.Error("lost content after markers")
	}
	if strings.Contains(content, "old section") {
		t.Error("old section should have been replaced")
	}
	if !strings.Contains(content, "Context Engineering") {
		t.Error("new section not injected")
	}
}

func TestCtxExists(t *testing.T) {
	dir := t.TempDir()
	if CtxExists(dir) {
		t.Error("CtxExists should be false before init")
	}
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)
	if !CtxExists(dir) {
		t.Error("CtxExists should be true after creating .ctx/")
	}
}
