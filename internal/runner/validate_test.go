// internal/runner/validate_test.go
package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lofari/golem/internal/ctx"
)

func TestValidatePostIteration_SchemaFailure(t *testing.T) {
	// State with missing project name is unfixable — should halt
	before := ctx.State{Project: ctx.Project{Name: "test"}}
	after := ctx.State{
		Project: ctx.Project{Name: ""}, // missing required field
		Tasks:   []ctx.Task{{Name: "t", Status: "todo"}},
	}

	result := ValidatePostIteration(t.TempDir(), before, after, ctx.Log{})
	if !result.Halted {
		t.Error("should halt on unfixable schema failure (missing project name)")
	}
}

func TestValidatePostIteration_AutoRepairPhase(t *testing.T) {
	dir := t.TempDir()
	setupState(t, dir, ctx.State{
		Project: ctx.Project{Name: "test"},
		Status:  ctx.Status{Phase: "review"}, // invalid phase
		Tasks:   []ctx.Task{{Name: "t", Status: "todo"}},
	})

	before := ctx.State{Project: ctx.Project{Name: "test"}}
	after := ctx.State{
		Project: ctx.Project{Name: "test"},
		Status:  ctx.Status{Phase: "review"},
		Tasks:   []ctx.Task{{Name: "t", Status: "todo"}},
	}

	result := ValidatePostIteration(dir, before, after, ctx.Log{})
	if result.Halted {
		t.Errorf("should auto-repair invalid phase, not halt; warnings: %v", result.Warnings)
	}
	if len(result.Warnings) == 0 {
		t.Error("should emit a warning about invalid phase")
	}
}

func TestValidatePostIteration_AutoRepairStatus(t *testing.T) {
	dir := t.TempDir()
	setupState(t, dir, ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks:   []ctx.Task{{Name: "bad", Status: "gibberish"}},
	})

	before := ctx.State{Project: ctx.Project{Name: "test"}}
	after := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks:   []ctx.Task{{Name: "bad", Status: "gibberish"}},
	}

	result := ValidatePostIteration(dir, before, after, ctx.Log{})
	if result.Halted {
		t.Errorf("should auto-repair invalid status, not halt; warnings: %v", result.Warnings)
	}

	// Verify repaired state was written
	repaired, err := ctx.ReadState(dir)
	if err != nil {
		t.Fatalf("reading repaired state: %v", err)
	}
	if repaired.Tasks[0].Status != "todo" {
		t.Errorf("repaired status = %q, want %q", repaired.Tasks[0].Status, "todo")
	}
}

func setupState(t *testing.T, dir string, s ctx.State) {
	t.Helper()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)
	if err := ctx.WriteState(dir, s); err != nil {
		t.Fatalf("setup state: %v", err)
	}
}

func TestValidatePostIteration_TaskRegression(t *testing.T) {
	before := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks: []ctx.Task{
			{Name: "auth", Status: "done"},
			{Name: "api", Status: "in-progress"},
		},
	}
	after := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks: []ctx.Task{
			{Name: "auth", Status: "in-progress"}, // regression!
			{Name: "api", Status: "done"},
		},
	}

	result := ValidatePostIteration(t.TempDir(), before, after, ctx.Log{})
	if result.Halted {
		t.Error("regression should warn, not halt")
	}

	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "auth") && strings.Contains(w, "regressed") {
			found = true
		}
	}
	if !found {
		t.Error("should warn about task regression")
	}
}

func TestDetectThrashing(t *testing.T) {
	// 3 consecutive sessions on the same task
	l := ctx.Log{Sessions: []ctx.Session{
		{Iteration: 1, Task: "payment"},
		{Iteration: 2, Task: "payment"},
		{Iteration: 3, Task: "payment"},
	}}

	thrashing := detectThrashing(l)
	if len(thrashing) != 1 || thrashing[0] != "payment" {
		t.Errorf("expected thrashing on 'payment', got %v", thrashing)
	}

	// Different tasks — no thrashing
	l2 := ctx.Log{Sessions: []ctx.Session{
		{Iteration: 1, Task: "auth"},
		{Iteration: 2, Task: "payment"},
		{Iteration: 3, Task: "auth"},
	}}
	if len(detectThrashing(l2)) != 0 {
		t.Error("should not detect thrashing with different tasks")
	}

	// Less than 3 sessions — no thrashing
	l3 := ctx.Log{Sessions: []ctx.Session{
		{Iteration: 1, Task: "payment"},
		{Iteration: 2, Task: "payment"},
	}}
	if len(detectThrashing(l3)) != 0 {
		t.Error("should not detect thrashing with < 3 sessions")
	}
}
