package query

import (
	"path/filepath"
	"testing"

	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/graph/model"
)

func setupTestStore(t *testing.T) *graph.Store {
	t.Helper()
	dir := t.TempDir()
	store, err := graph.OpenStore(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	// Seed: fileA -> defines FuncA, FuncA calls FuncB, FuncB defined in fileB
	store.InsertBatch(
		[]graph.Node{
			{ID: "file:a.go", Type: "file", Name: "a.go", Path: "a.go"},
			{ID: "fn:a.go:FuncA", Type: "function", Name: "FuncA", Path: "a.go", Line: 10},
			{ID: "file:b.go", Type: "file", Name: "b.go", Path: "b.go"},
			{ID: "fn:b.go:FuncB", Type: "function", Name: "FuncB", Path: "b.go", Line: 5},
		},
		[]graph.Edge{
			{From: "file:a.go", To: "fn:a.go:FuncA", Type: "DEFINES"},
			{From: "file:b.go", To: "fn:b.go:FuncB", Type: "DEFINES"},
			{From: "fn:a.go:FuncA", To: "fn:b.go:FuncB", Type: "CALLS"},
		},
	)
	return store
}

func TestRelated_Callers(t *testing.T) {
	store := setupTestStore(t)

	result, err := Related(store, "FuncB", "callers", 1)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Nodes) == 0 {
		t.Fatal("expected caller nodes")
	}

	foundFuncA := false
	for _, n := range result.Nodes {
		if n.Name == "FuncA" {
			foundFuncA = true
		}
	}
	if !foundFuncA {
		t.Fatal("expected FuncA as caller of FuncB")
	}
}

func TestRelated_Dependencies(t *testing.T) {
	store := setupTestStore(t)

	result, err := Related(store, "FuncA", "dependencies", 1)
	if err != nil {
		t.Fatal(err)
	}

	foundFuncB := false
	for _, n := range result.Nodes {
		if n.Name == "FuncB" {
			foundFuncB = true
		}
	}
	if !foundFuncB {
		t.Fatal("expected FuncB as dependency of FuncA")
	}
}

func TestRelated_Dependents(t *testing.T) {
	store := setupTestStore(t)

	// FuncB's dependents should include FuncA (which calls it)
	result, err := Related(store, "FuncB", "dependents", 1)
	if err != nil {
		t.Fatal(err)
	}

	foundFuncA := false
	for _, n := range result.Nodes {
		if n.Name == "FuncA" {
			foundFuncA = true
		}
	}
	if !foundFuncA {
		t.Fatal("expected FuncA as dependent of FuncB")
	}
}

func TestRelated_All(t *testing.T) {
	store := setupTestStore(t)

	result, err := Related(store, "FuncA", "all", 1)
	if err != nil {
		t.Fatal(err)
	}

	// Should have edges in both directions
	if len(result.Edges) == 0 {
		t.Fatal("expected edges")
	}
}

func TestRelated_NotFound(t *testing.T) {
	store := setupTestStore(t)

	result, err := Related(store, "NonExistent", "callers", 1)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Nodes) != 0 {
		t.Fatalf("expected empty result, got %d nodes", len(result.Nodes))
	}
}

func setupTestExecution(t *testing.T) *graph.Store {
	t.Helper()
	dir := t.TempDir()
	store, err := graph.OpenStore(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	// Seed file nodes
	store.InsertBatch(
		[]graph.Node{
			{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go"},
		},
		nil,
	)

	// Seed execution data
	store.InsertExecution(model.Execution{SessionID: "s1", StartedAt: 1000, Status: "completed"})
	store.InsertCommand(model.Command{ID: "cmd:s1:1", SessionID: "s1", Seq: 1, Command: "go test ./...", ExitCode: 1})
	store.InsertOutput(model.Output{CommandID: "cmd:s1:1", Stdout: "--- FAIL: TestFoo (0.01s)\nFAIL", Stderr: ""})
	store.InsertError(model.Error{ID: "err:s1:1", CommandID: "cmd:s1:1", Message: "test failed"})
	store.InsertTestResult(model.TestResult{ID: "test:s1:TestFoo", SessionID: "s1", Name: "TestFoo", Passed: false, DurationMs: 10})
	store.InsertBatch(nil, []graph.Edge{
		{From: "cmd:s1:1", To: "file:main.go", Type: "ACCESSES"},
	})
	store.FinalizeExecution("s1", 2000, "completed")

	return store
}

func TestRuntimePath_Trace(t *testing.T) {
	store := setupTestExecution(t)

	result, err := RuntimePath(store, "s1", "trace", "")
	if err != nil {
		t.Fatal(err)
	}

	trace, ok := result.(*TraceResult)
	if !ok {
		t.Fatal("expected *TraceResult")
	}

	if trace.SessionID != "s1" {
		t.Fatalf("expected session s1, got %s", trace.SessionID)
	}
	if len(trace.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(trace.Commands))
	}
	if trace.Commands[0].Command != "go test ./..." {
		t.Fatalf("unexpected command: %s", trace.Commands[0].Command)
	}
}

func TestRuntimePath_Failures(t *testing.T) {
	store := setupTestExecution(t)

	result, err := RuntimePath(store, "s1", "failures", "")
	if err != nil {
		t.Fatal(err)
	}

	fr, ok := result.(*FailureResult)
	if !ok {
		t.Fatal("expected *FailureResult")
	}

	if len(fr.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(fr.Failures))
	}
	if fr.Failures[0].ErrorMessage != "test failed" {
		t.Fatalf("unexpected error: %s", fr.Failures[0].ErrorMessage)
	}
	if len(fr.FailedTests) != 1 || fr.FailedTests[0].Name != "TestFoo" {
		t.Fatalf("unexpected failed tests: %v", fr.FailedTests)
	}
}

func TestRuntimePath_LatestSession(t *testing.T) {
	store := setupTestExecution(t)

	// Empty session should resolve to latest
	result, err := RuntimePath(store, "", "trace", "")
	if err != nil {
		t.Fatal(err)
	}

	trace := result.(*TraceResult)
	if trace.SessionID != "s1" {
		t.Fatalf("expected latest session s1, got %s", trace.SessionID)
	}
}

func TestRuntimePath_CommandFilter(t *testing.T) {
	store := setupTestExecution(t)

	result, err := RuntimePath(store, "s1", "trace", "nonexistent")
	if err != nil {
		t.Fatal(err)
	}

	trace := result.(*TraceResult)
	if len(trace.Commands) != 0 {
		t.Fatalf("expected 0 commands with filter, got %d", len(trace.Commands))
	}
}
