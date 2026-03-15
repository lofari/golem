//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lofari/golem/internal/ctx"
)

// TestDSL_StateYaml_RoundTrip verifies that YAML written by golem-dsl's sync
// module is correctly parsed by Go's ctx.ReadState.
func TestDSL_StateYaml_RoundTrip(t *testing.T) {
	// This YAML matches the exact output of golem.dsl.sync/project-state-yaml
	// with dsl-state {:goal "implement auth"}, agent-name "build-feature", phase "building"
	yamlContent := `project:
  name: ""
  summary: ""
status:
  current_focus: implement auth
  phase: building
  last_session: build-feature
tasks: []
decisions: []
pitfalls: []
`

	dir := t.TempDir()
	ctxDir := filepath.Join(dir, ".ctx")
	os.MkdirAll(ctxDir, 0755)
	os.WriteFile(filepath.Join(ctxDir, "state.yaml"), []byte(yamlContent), 0644)

	state, err := ctx.ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState failed: %v", err)
	}

	if state.Status.CurrentFocus != "implement auth" {
		t.Errorf("current_focus: expected %q, got %q", "implement auth", state.Status.CurrentFocus)
	}
	if state.Status.Phase != "building" {
		t.Errorf("phase: expected %q, got %q", "building", state.Status.Phase)
	}
	if state.Status.LastSession != "build-feature" {
		t.Errorf("last_session: expected %q, got %q", "build-feature", state.Status.LastSession)
	}
	if len(state.Tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(state.Tasks))
	}
	if len(state.Decisions) != 0 {
		t.Errorf("expected 0 decisions, got %d", len(state.Decisions))
	}
}

// TestDSL_StateYaml_PhaseNormalization verifies that phase aliases written by
// the DSL are normalized by Go's ReadState.
func TestDSL_StateYaml_PhaseNormalization(t *testing.T) {
	tests := []struct {
		written    string
		normalized string
	}{
		{"building", "building"},
		{"build", "building"},
		{"planning", "planning"},
		{"plan", "planning"},
		{"fixing", "fixing"},
		{"debugging", "fixing"},
		{"polishing", "polishing"},
		{"review", "polishing"},
	}

	for _, tt := range tests {
		t.Run(tt.written, func(t *testing.T) {
			yaml := `project:
  name: ""
  summary: ""
status:
  current_focus: test
  phase: ` + tt.written + `
  last_session: test-agent
tasks: []
decisions: []
pitfalls: []
`
			dir := t.TempDir()
			ctxDir := filepath.Join(dir, ".ctx")
			os.MkdirAll(ctxDir, 0755)
			os.WriteFile(filepath.Join(ctxDir, "state.yaml"), []byte(yaml), 0644)

			state, err := ctx.ReadState(dir)
			if err != nil {
				t.Fatalf("ReadState failed: %v", err)
			}
			if state.Status.Phase != tt.normalized {
				t.Errorf("phase %q: expected normalized to %q, got %q", tt.written, tt.normalized, state.Status.Phase)
			}
		})
	}
}

// TestDSL_StateYaml_Validation documents that the DSL currently writes
// project.name="" which fails ValidateState. This is a known contract gap.
func TestDSL_StateYaml_Validation(t *testing.T) {
	yamlContent := `project:
  name: ""
  summary: ""
status:
  current_focus: test
  phase: building
  last_session: test-agent
tasks: []
decisions: []
pitfalls: []
`
	dir := t.TempDir()
	ctxDir := filepath.Join(dir, ".ctx")
	os.MkdirAll(ctxDir, 0755)
	os.WriteFile(filepath.Join(ctxDir, "state.yaml"), []byte(yamlContent), 0644)

	state, err := ctx.ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState failed: %v", err)
	}

	err = ctx.ValidateState(state)
	if err == nil {
		t.Fatal("expected validation error for empty project.name, but got nil — if this passes, the DSL contract gap has been fixed")
	}
	// This documents the current gap: DSL writes name="" which fails validation
	t.Logf("Known contract gap: %v", err)
}

// TestDSL_StateYaml_GoalWithSpecialChars verifies that goals containing
// colons and quotes round-trip correctly through the DSL's yaml-escape.
func TestDSL_StateYaml_GoalWithSpecialChars(t *testing.T) {
	// The DSL's yaml-escape wraps strings with : or " in quotes
	yamlContent := `project:
  name: ""
  summary: ""
status:
  current_focus: "fix: handle edge case"
  phase: fixing
  last_session: fix-bug
tasks: []
decisions: []
pitfalls: []
`
	dir := t.TempDir()
	ctxDir := filepath.Join(dir, ".ctx")
	os.MkdirAll(ctxDir, 0755)
	os.WriteFile(filepath.Join(ctxDir, "state.yaml"), []byte(yamlContent), 0644)

	state, err := ctx.ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState failed: %v", err)
	}
	if state.Status.CurrentFocus != "fix: handle edge case" {
		t.Errorf("current_focus: expected %q, got %q", "fix: handle edge case", state.Status.CurrentFocus)
	}
}
