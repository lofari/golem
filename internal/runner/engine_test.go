package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lofari/golem/templates"
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

func TestEngine_WhileLoop(t *testing.T) {
	dir := setupGitRepo(t)
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	implementStep := &Step{Name: "implement", Type: StepTypeAgentic, Reads: []string{"goal"}, OptionalReads: []string{"review-feedback"}, Writes: []string{"code", "test-results"}, Prompt: "Implement: ${goal}"}
	reviewStep := &Step{Name: "review", Type: StepTypeAgentic, Reads: []string{"code"}, Writes: []string{"review-feedback"}, Prompt: "Review code changes"}

	bp := &Blueprint{
		Name:         "test",
		InitialState: []string{"goal"},
		Config:       map[string]any{},
		Errors:       ErrorHandlers{ContractViolation: ErrorHandler{Action: "halt"}},
	}
	bp.pipeline = &Pipeline{
		Nodes: []PipelineNode{
			{Step: implementStep},
			{Step: reviewStep},
			{ControlFlow: &ControlFlowNode{
				Type: ControlWhile, Predicate: "needs-work", Max: 3,
				StepRefs: []string{"implement", "review"},
			}},
		},
		StepDefs: map[string]*Step{"implement": implementStep, "review": reviewStep},
	}

	reviewCalls := 0
	smartMock := &smartMockRunner{
		responses: func(step string, callNum int) MockResponse {
			if step == "review" {
				reviewCalls++
				if reviewCalls == 1 {
					return MockResponse{SessionOutput: map[string]any{"review-feedback": map[string]any{"verdict": "needs-work"}}}
				}
				return MockResponse{SessionOutput: map[string]any{"review-feedback": map[string]any{"verdict": "approved"}}}
			}
			return MockResponse{SessionOutput: map[string]any{"test-results": map[string]any{"status": "pass"}}}
		},
	}

	e := NewEngine(EngineConfig{
		Dir: dir, AgentName: "test", Goal: "Test",
		Blueprint: bp, Config: map[string]any{}, Runner: smartMock, Model: "test",
	})

	state, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("engine error: %v", err)
	}
	verdict := getNestedString(state, "review-feedback", "verdict")
	if verdict != "approved" {
		t.Errorf("final verdict = %q, want approved", verdict)
	}
	if len(smartMock.calls) != 4 {
		t.Errorf("expected 4 runner calls, got %d", len(smartMock.calls))
	}
}

func TestEngine_IfThenElse(t *testing.T) {
	dir := setupGitRepo(t)
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	stepA := &Step{Name: "fast-path", Type: StepTypeAgentic, Reads: []string{"goal"}, Writes: []string{"result"}, Prompt: "Fast: ${goal}"}
	stepB := &Step{Name: "slow-path", Type: StepTypeAgentic, Reads: []string{"goal"}, Writes: []string{"result"}, Prompt: "Slow: ${goal}"}

	bp := &Blueprint{
		Name: "test", InitialState: []string{"goal"}, Config: map[string]any{"ci-enabled": true},
		Errors: ErrorHandlers{ContractViolation: ErrorHandler{Action: "halt"}},
	}
	bp.pipeline = &Pipeline{
		Nodes: []PipelineNode{
			{ControlFlow: &ControlFlowNode{
				Type: ControlIf, Predicate: "ci-enabled",
				ThenRefs: []string{"fast-path"},
				ElseRefs: []string{"slow-path"},
			}},
		},
		StepDefs: map[string]*Step{"fast-path": stepA, "slow-path": stepB},
	}

	mock := &smartMockRunner{
		responses: func(step string, callNum int) MockResponse {
			return MockResponse{SessionOutput: map[string]any{"result": "done"}}
		},
	}

	e := NewEngine(EngineConfig{
		Dir: dir, AgentName: "test", Goal: "Test",
		Blueprint: bp, Config: map[string]any{"ci-enabled": true}, Runner: mock, Model: "test",
	})

	_, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("engine error: %v", err)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
}

func TestEngine_TransientRetry(t *testing.T) {
	dir := setupGitRepo(t)
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	step := &Step{Name: "plan", Type: StepTypeAgentic, Reads: []string{"goal"}, Writes: []string{"plan"}}
	bp := &Blueprint{
		Name: "test", InitialState: []string{"goal"}, Config: map[string]any{},
		Errors: ErrorHandlers{
			Transient:         ErrorHandler{Action: "retry", Max: 2},
			ContractViolation: ErrorHandler{Action: "halt"},
		},
	}
	bp.pipeline = &Pipeline{
		Nodes:    []PipelineNode{{Step: step}},
		StepDefs: map[string]*Step{"plan": step},
	}

	callCount := 0
	failMock := &smartMockRunner{
		responses: func(step string, callNum int) MockResponse {
			callCount++
			if callCount <= 2 {
				return MockResponse{Err: fmt.Errorf("transient failure")}
			}
			return MockResponse{SessionOutput: map[string]any{"plan": "final plan"}}
		},
	}

	e := NewEngine(EngineConfig{
		Dir: dir, AgentName: "test", Goal: "Test",
		Blueprint: bp, Config: map[string]any{}, Runner: failMock, Model: "test",
	})

	_, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("engine should recover after retries, got: %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls (2 fails + 1 success), got %d", callCount)
	}
}

func TestEngine_UnrecoverableHalts(t *testing.T) {
	dir := setupGitRepo(t)
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	step := &Step{Name: "lint", Type: StepTypeBuiltin, Reads: []string{"code"}, Writes: []string{"lint-results"}}
	bp := &Blueprint{
		Name: "test", InitialState: []string{"goal"}, Config: map[string]any{},
		Errors: ErrorHandlers{ContractViolation: ErrorHandler{Action: "halt"}},
	}
	bp.pipeline = &Pipeline{
		Nodes:    []PipelineNode{{Step: step}},
		StepDefs: map[string]*Step{},
	}

	e := NewEngine(EngineConfig{
		Dir: dir, AgentName: "test", Goal: "Test",
		Blueprint: bp, Config: map[string]any{"lint-cmd": "nonexistent-cmd-xyz"}, Runner: nil, Model: "test",
	})

	_, err := e.Run(context.Background())
	if err == nil {
		t.Fatal("expected halt on unrecoverable error")
	}
}

func TestEngine_Integration_OneShot(t *testing.T) {
	dir := setupGitRepo(t)
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	data, err := templates.FS.ReadFile("agents/one-shot.yaml")
	if err != nil {
		t.Fatalf("read one-shot.yaml: %v", err)
	}
	bp, err := ParseBlueprint(data)
	if err != nil {
		t.Fatalf("parse blueprint: %v", err)
	}
	if err := bp.ValidateContracts(); err != nil {
		t.Fatalf("contract validation: %v", err)
	}

	mock := &smartMockRunner{
		responses: func(step string, callNum int) MockResponse {
			return MockResponse{
				SessionOutput: map[string]any{
					"test-results": map[string]any{"status": "pass", "summary": "all green"},
				},
			}
		},
	}

	config := map[string]any{"lint-cmd": "true", "test-cmd": "true", "ci-enabled": false}

	e := NewEngine(EngineConfig{
		Dir: dir, AgentName: "one-shot", Goal: "Add auth",
		Blueprint: bp, Config: config, Runner: mock, Model: "test",
	})

	state, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("engine error: %v", err)
	}

	for _, key := range []string{"goal", "branch", "base", "test-results", "lint-results"} {
		if state[key] == nil {
			t.Errorf("state[%q] should be present", key)
		}
	}
}

func TestEngine_Integration_BuildFeatureLoop(t *testing.T) {
	dir := setupGitRepo(t)
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	data, err := templates.FS.ReadFile("agents/build-feature.yaml")
	if err != nil {
		t.Fatalf("read build-feature.yaml: %v", err)
	}
	bp, err := ParseBlueprint(data)
	if err != nil {
		t.Fatalf("parse blueprint: %v", err)
	}

	reviewCalls := 0
	mock := &smartMockRunner{
		responses: func(step string, callNum int) MockResponse {
			if step == "review" {
				reviewCalls++
				if reviewCalls == 1 {
					return MockResponse{SessionOutput: map[string]any{"review-feedback": map[string]any{"verdict": "needs-work", "comments": "fix tests"}}}
				}
				return MockResponse{SessionOutput: map[string]any{"review-feedback": map[string]any{"verdict": "approved"}}}
			}
			if step == "plan" {
				return MockResponse{SessionOutput: map[string]any{"plan": []any{map[string]any{"step": 1, "desc": "do it"}}}}
			}
			return MockResponse{SessionOutput: map[string]any{"test-results": map[string]any{"status": "pass"}}}
		},
	}

	config := map[string]any{"lint-cmd": "true", "test-cmd": "true", "ci-enabled": false}
	e := NewEngine(EngineConfig{
		Dir: dir, AgentName: "build-feature", Goal: "Add auth",
		Blueprint: bp, Config: config, Runner: mock, Model: "test",
	})

	state, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("engine error: %v", err)
	}

	if reviewCalls != 2 {
		t.Errorf("review was called %d times, want 2", reviewCalls)
	}
	verdict := getNestedString(state, "review-feedback", "verdict")
	if verdict != "approved" {
		t.Errorf("final verdict = %q, want approved", verdict)
	}
}
