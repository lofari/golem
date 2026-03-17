package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	golemctx "github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/templates"
)

type mockBuilderRunner struct {
	callCount int
}

func (m *mockBuilderRunner) Run(ctx context.Context, dir, prompt string, maxTurns int, model string) (string, error) {
	return m.RunWithTools(ctx, dir, prompt, maxTurns, model, nil)
}

func (m *mockBuilderRunner) RunWithTools(ctx context.Context, dir, prompt string, maxTurns int, model string, tools []string) (string, error) {
	m.callCount++

	// Read state to find first actionable task
	state, err := golemctx.ReadState(dir)
	if err != nil {
		return "", err
	}

	for i, t := range state.Tasks {
		if t.Status == "todo" || t.Status == "in-progress" {
			state.Tasks[i].Status = "done"
			state.Tasks[i].Notes = "completed by mock"

			if err := golemctx.WriteState(dir, state); err != nil {
				return "", err
			}
			if err := golemctx.AppendSession(dir, golemctx.Session{
				Iteration: m.callCount,
				Task:      t.Name,
				Outcome:   "done",
				Summary:   "Mock completed " + t.Name,
				Handoff:   "Move to next task",
			}); err != nil {
				return "", err
			}
			break
		}
	}

	// Write empty session-output.json (implement step reads it)
	os.WriteFile(filepath.Join(dir, "session-output.json"), []byte(`{}`), 0644)

	return "mock output", nil
}

func TestBuilderBlueprintIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	dir := setupGitRepo(t)
	ctxDir := filepath.Join(dir, ".ctx")
	os.MkdirAll(ctxDir, 0755)

	// Set up state with 2 tasks
	state := golemctx.State{
		Project: golemctx.Project{Name: "test-project", DocsPath: "docs/"},
		Status:  golemctx.Status{Phase: "building"},
		Tasks: []golemctx.Task{
			{Name: "task-1", Status: "todo", Notes: "first task"},
			{Name: "task-2", Status: "todo", Notes: "second task"},
		},
	}
	if err := golemctx.WriteState(dir, state); err != nil {
		t.Fatalf("write state: %v", err)
	}
	if err := golemctx.WriteLog(dir, golemctx.Log{}); err != nil {
		t.Fatalf("write log: %v", err)
	}

	// Load and parse implementer blueprint
	data, err := templates.FS.ReadFile("agents/implementer.yaml")
	if err != nil {
		t.Fatalf("reading implementer.yaml: %v", err)
	}
	bp, err := ParseBlueprint(data)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	mock := &mockBuilderRunner{}
	e := NewEngine(EngineConfig{
		Dir:       dir,
		AgentName: "implementer",
		Goal:      "complete all tasks",
		Blueprint: bp,
		Config:    bp.Config,
		Runner:    mock,
		Model:     "test-model",
	})

	result, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("engine run: %v", err)
	}

	// Verify both tasks were completed
	finalState, err := golemctx.ReadState(dir)
	if err != nil {
		t.Fatalf("read final state: %v", err)
	}
	for _, task := range finalState.Tasks {
		if task.Status != "done" {
			t.Errorf("task %q status = %s, want done", task.Name, task.Status)
		}
	}

	// Verify mock was called at least 2 times (one per task)
	if mock.callCount < 2 {
		t.Errorf("mock called %d times, want >= 2", mock.callCount)
	}

	// Verify log has sessions
	log, err := golemctx.ReadLog(dir)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if len(log.Sessions) < 2 {
		t.Errorf("log sessions = %d, want >= 2", len(log.Sessions))
	}

	_ = result
}
