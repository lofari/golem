package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	golemctx "github.com/lofari/golem/internal/ctx"
)

// mockRunner returns canned output for each call.
type mockRunner struct {
	outputs []string
	calls   int
}

func (m *mockRunner) Run(_ context.Context, _ string, _ string, _ int, _ string) (string, error) {
	if m.calls >= len(m.outputs) {
		return "", nil
	}
	out := m.outputs[m.calls]
	m.calls++
	return out, nil
}

func setupTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ctxDir := filepath.Join(dir, ".ctx")
	os.MkdirAll(ctxDir, 0755)

	state := golemctx.State{
		Project: golemctx.Project{Name: "test", DocsPath: "docs/"},
		Status:  golemctx.Status{Phase: "building"},
		Tasks: []golemctx.Task{
			{Name: "task1", Status: "todo"},
		},
	}
	golemctx.WriteState(dir, state)
	golemctx.WriteLog(dir, golemctx.Log{})

	// Write a minimal prompt template
	os.WriteFile(filepath.Join(ctxDir, "prompt.md"), []byte("Do work. {{ITERATION_CONTEXT}} {{TASK_OVERRIDE}} {{DOCS_PATH}}"), 0644)
	return dir
}

func TestBuilderLoop_CompletePromise(t *testing.T) {
	dir := setupTestProject(t)
	mock := &mockRunner{outputs: []string{"done <promise>COMPLETE</promise>"}}

	result, err := RunBuilderLoop(context.Background(), BuilderConfig{
		Dir:           dir,
		MaxIterations: 5,
		MaxToolCalls:      10,
		Runner:        mock,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Completed {
		t.Error("expected Completed=true")
	}
	if mock.calls != 1 {
		t.Errorf("expected 1 call, got %d", mock.calls)
	}
}

func TestBuilderLoop_MaxIterations(t *testing.T) {
	dir := setupTestProject(t)
	mock := &mockRunner{outputs: []string{"partial", "partial", "partial"}}

	result, err := RunBuilderLoop(context.Background(), BuilderConfig{
		Dir:           dir,
		MaxIterations: 3,
		MaxToolCalls:      10,
		Runner:        mock,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Completed {
		t.Error("should not be completed")
	}
	if result.Iterations != 3 {
		t.Errorf("expected 3 iterations, got %d", result.Iterations)
	}
}

func TestBuilderLoop_DryRun(t *testing.T) {
	dir := setupTestProject(t)
	mock := &mockRunner{}

	result, err := RunBuilderLoop(context.Background(), BuilderConfig{
		Dir:           dir,
		MaxIterations: 1,
		MaxToolCalls:      10,
		DryRun:        true,
		Runner:        mock,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.calls != 0 {
		t.Error("dry-run should not call runner")
	}
	_ = result
}

func TestBuilderLoop_EmitsEvents(t *testing.T) {
	dir := setupTestProject(t)
	mock := &mockRunner{outputs: []string{"done <promise>COMPLETE</promise>"}}

	events := make(chan Event, 100)
	result, err := RunBuilderLoop(context.Background(), BuilderConfig{
		Dir:           dir,
		MaxIterations: 5,
		MaxToolCalls:      10,
		Runner:        mock,
		Events:        events,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Completed {
		t.Error("expected Completed=true")
	}

	// Collect all events
	close(events)
	var types []EventType
	for ev := range events {
		types = append(types, ev.Type)
	}

	// Should have: IterStart, IterEnd, LoopDone
	if len(types) < 3 {
		t.Fatalf("expected at least 3 events, got %d: %v", len(types), types)
	}
	if types[0] != EventIterStart {
		t.Errorf("first event should be EventIterStart, got %d", types[0])
	}
	// Last event should be LoopDone
	if types[len(types)-1] != EventLoopDone {
		t.Errorf("last event should be EventLoopDone, got %d", types[len(types)-1])
	}
}

func TestBuilderLoop_SkipsStuckTask(t *testing.T) {
	dir := setupTestProject(t)

	// Set up state with two tasks
	state := golemctx.State{
		Project: golemctx.Project{Name: "test", DocsPath: "docs/"},
		Status:  golemctx.Status{Phase: "building"},
		Tasks: []golemctx.Task{
			{Name: "stuck-task", Status: "todo"},
			{Name: "good-task", Status: "todo"},
		},
	}
	golemctx.WriteState(dir, state)

	// Pre-seed log with a failure on stuck-task (strategy will see this on each iteration)
	golemctx.WriteLog(dir, golemctx.Log{Sessions: []golemctx.Session{
		{Iteration: 1, Task: "stuck-task", Outcome: "blocked", Summary: "failed"},
	}})

	// Need 3 iterations: iter 1 → Retry (count=1), iter 2 → Skip (count=2), iter 3 → COMPLETE
	mock := &mockRunner{outputs: []string{"partial", "partial", "done <promise>COMPLETE</promise>"}}

	result, err := RunBuilderLoop(context.Background(), BuilderConfig{
		Dir:           dir,
		MaxIterations: 5,
		MaxToolCalls:  10,
		Runner:        mock,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify stuck-task was marked blocked
	finalState, _ := golemctx.ReadState(dir)
	for _, task := range finalState.Tasks {
		if task.Name == "stuck-task" && task.Status != "blocked" {
			t.Errorf("stuck-task should be blocked, got %q", task.Status)
		}
	}
	_ = result
}

func TestBuilderLoop_ContextCancellation(t *testing.T) {
	dir := setupTestProject(t)
	mock := &mockRunner{outputs: []string{"partial"}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result, err := RunBuilderLoop(ctx, BuilderConfig{
		Dir:           dir,
		MaxIterations: 5,
		MaxToolCalls:      10,
		Runner:        mock,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Halted {
		t.Error("expected Halted=true when context is cancelled")
	}
	if result.HaltReason != "interrupted by signal" {
		t.Errorf("expected 'interrupted by signal', got %q", result.HaltReason)
	}
	if mock.calls != 0 {
		t.Error("should not call runner when context is already cancelled")
	}
}
