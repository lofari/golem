// internal/runner/validate_test.go
package runner

import (
	"strings"
	"testing"

	"github.com/lofari/golem/internal/ctx"
)

func TestValidatePostIteration_SchemaFailure(t *testing.T) {
	// State with invalid task status should halt
	before := ctx.State{Project: ctx.Project{Name: "test"}}
	after := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks:   []ctx.Task{{Name: "bad", Status: "invalid"}},
	}

	result := ValidatePostIteration(t.TempDir(), before, after, ctx.Log{})
	if !result.Halted {
		t.Error("should halt on schema validation failure")
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
