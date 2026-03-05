package runner

import (
	"strings"
	"testing"

	"github.com/lofari/golem/internal/ctx"
)

func TestNewStrategy(t *testing.T) {
	s := NewStrategy()
	if s == nil {
		t.Fatal("NewStrategy returned nil")
	}
	d := s.Evaluate(ctx.State{}, ctx.Log{}, "")
	if d.Action != ActionContinue {
		t.Errorf("empty state should return Continue, got %v", d.Action)
	}
}

func TestStrategy_FirstFailureRetries(t *testing.T) {
	s := NewStrategy()
	log := ctx.Log{Sessions: []ctx.Session{
		{Task: "auth", Outcome: "blocked", Summary: "jwt library not found"},
	}}
	state := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks:   []ctx.Task{{Name: "auth", Status: "todo"}},
	}

	d := s.Evaluate(state, log, "error: could not resolve jwt")
	if d.Action != ActionRetry {
		t.Errorf("first failure should retry, got %v", d.Action)
	}
	if !strings.Contains(d.InjectContext, "auth") {
		t.Error("inject context should mention the failed task")
	}
	if !strings.Contains(d.InjectContext, "jwt library not found") {
		t.Error("inject context should include the summary")
	}
}

func TestStrategy_SecondFailureSkips(t *testing.T) {
	s := NewStrategy()
	state := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks:   []ctx.Task{{Name: "auth", Status: "todo"}},
	}

	// First failure
	log1 := ctx.Log{Sessions: []ctx.Session{
		{Task: "auth", Outcome: "blocked", Summary: "jwt not found"},
	}}
	s.Evaluate(state, log1, "")

	// Second failure
	log2 := ctx.Log{Sessions: []ctx.Session{
		{Task: "auth", Outcome: "blocked", Summary: "jwt not found"},
		{Task: "auth", Outcome: "blocked", Summary: "still can't find jwt"},
	}}
	d := s.Evaluate(state, log2, "")
	if d.Action != ActionSkip {
		t.Errorf("second failure should skip, got %v", d.Action)
	}
	if len(d.SkipTasks) != 1 || d.SkipTasks[0] != "auth" {
		t.Errorf("should skip 'auth', got %v", d.SkipTasks)
	}
}

func TestStrategy_SuccessResetsFailureCount(t *testing.T) {
	s := NewStrategy()
	state := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks:   []ctx.Task{{Name: "auth", Status: "todo"}},
	}

	// One failure
	log1 := ctx.Log{Sessions: []ctx.Session{
		{Task: "auth", Outcome: "blocked"},
	}}
	s.Evaluate(state, log1, "")

	// Then success on same task
	log2 := ctx.Log{Sessions: []ctx.Session{
		{Task: "auth", Outcome: "blocked"},
		{Task: "auth", Outcome: "done"},
	}}
	s.Evaluate(state, log2, "")

	// Another failure should be treated as first
	log3 := ctx.Log{Sessions: []ctx.Session{
		{Task: "auth", Outcome: "blocked"},
		{Task: "auth", Outcome: "done"},
		{Task: "auth", Outcome: "blocked"},
	}}
	d := s.Evaluate(state, log3, "")
	if d.Action != ActionRetry {
		t.Errorf("after success reset, next failure should retry, got %v", d.Action)
	}
}

func TestStrategy_DeadlockHalts(t *testing.T) {
	s := NewStrategy()
	state := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks: []ctx.Task{
			{Name: "auth", Status: "blocked", BlockedReason: "stuck"},
			{Name: "api", Status: "todo", DependsOn: ctx.FlexString{"auth"}},
			{Name: "ui", Status: "todo", DependsOn: ctx.FlexString{"api"}},
		},
	}
	log := ctx.Log{Sessions: []ctx.Session{{Task: "auth", Outcome: "done"}}}

	d := s.Evaluate(state, log, "")
	if d.Action != ActionHalt {
		t.Errorf("all tasks blocked by deps should halt, got %v", d.Action)
	}
}

func TestStrategy_NoDeadlockWhenActionable(t *testing.T) {
	s := NewStrategy()
	state := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks: []ctx.Task{
			{Name: "auth", Status: "blocked", BlockedReason: "stuck"},
			{Name: "api", Status: "todo", DependsOn: ctx.FlexString{"auth"}},
			{Name: "docs", Status: "todo"}, // no dependency — actionable
		},
	}
	log := ctx.Log{Sessions: []ctx.Session{{Task: "auth", Outcome: "done"}}}

	d := s.Evaluate(state, log, "")
	if d.Action == ActionHalt {
		t.Error("should not halt when an actionable task exists")
	}
}

func TestStrategy_ThrashingSkips(t *testing.T) {
	s := NewStrategy()
	state := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks:   []ctx.Task{{Name: "payment", Status: "in-progress"}},
	}
	log := ctx.Log{Sessions: []ctx.Session{
		{Task: "payment", Outcome: "partial"},
		{Task: "payment", Outcome: "partial"},
		{Task: "payment", Outcome: "partial"},
	}}

	d := s.Evaluate(state, log, "")
	if d.Action != ActionSkip {
		t.Errorf("3 consecutive same task should skip, got %v", d.Action)
	}
	if len(d.SkipTasks) != 1 || d.SkipTasks[0] != "payment" {
		t.Errorf("should skip 'payment', got %v", d.SkipTasks)
	}
}

func TestStrategy_NoThrashingDifferentTasks(t *testing.T) {
	s := NewStrategy()
	state := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks:   []ctx.Task{{Name: "a", Status: "todo"}, {Name: "b", Status: "todo"}},
	}
	log := ctx.Log{Sessions: []ctx.Session{
		{Task: "a", Outcome: "partial"},
		{Task: "b", Outcome: "partial"},
		{Task: "a", Outcome: "partial"},
	}}

	d := s.Evaluate(state, log, "")
	if d.Action == ActionSkip {
		t.Error("different tasks should not trigger thrashing")
	}
}
