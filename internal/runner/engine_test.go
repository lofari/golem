package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEngine_RunID(t *testing.T) {
	e := NewEngine(EngineConfig{
		Dir:       t.TempDir(),
		AgentName: "test",
		Goal:      "test goal",
	})
	if e.RunID == "" {
		t.Error("RunID should not be empty")
	}
	if !strings.Contains(e.RunID, "run-") {
		t.Errorf("RunID should start with run-, got: %s", e.RunID)
	}
}

func TestEngine_InitialState(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	e := NewEngine(EngineConfig{
		Dir:       dir,
		AgentName: "test",
		Goal:      "Add authentication",
	})

	state := e.State()
	goal, ok := state["goal"].(string)
	if !ok || goal != "Add authentication" {
		t.Errorf("state[goal] = %v, want 'Add authentication'", state["goal"])
	}
}

func TestEngine_AgenticStep(t *testing.T) {
	dir := setupGitRepo(t)
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	bp := &Blueprint{
		Name:         "test",
		InitialState: []string{"goal"},
		Config:       map[string]any{},
		Errors:       ErrorHandlers{ContractViolation: ErrorHandler{Action: "halt"}},
	}
	bp.pipeline = &Pipeline{
		Nodes: []PipelineNode{
			{Step: &Step{Name: "plan", Type: StepTypeAgentic, Reads: []string{"goal"}, Writes: []string{"plan"}, Tools: []string{"semantic_search"}}},
		},
		StepDefs: map[string]*Step{},
	}

	mock := &smartMockRunner{
		responses: func(step string, callNum int) MockResponse {
			return MockResponse{
				Output:        "planned",
				SessionOutput: map[string]any{"plan": []any{map[string]any{"step": 1, "desc": "do thing"}}},
			}
		},
	}

	e := NewEngine(EngineConfig{
		Dir:       dir,
		AgentName: "test",
		Goal:      "Add auth",
		Blueprint: bp,
		Config:    map[string]any{},
		Runner:    mock,
		Model:     "test-model",
	})

	state, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("engine run error: %v", err)
	}
	if state["plan"] == nil {
		t.Error("plan should be in state after agentic step")
	}
	if len(mock.calls) != 1 {
		t.Errorf("expected 1 runner call, got %d", len(mock.calls))
	}
	if len(mock.calls[0].Tools) != 1 || mock.calls[0].Tools[0] != "semantic_search" {
		t.Errorf("tools = %v, want [semantic_search]", mock.calls[0].Tools)
	}
}
