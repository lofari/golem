package query

import (
	"path/filepath"
	"testing"

	"github.com/lofari/golem/internal/graph"
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
