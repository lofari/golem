package graph

import (
	"os"
	"path/filepath"
	"testing"
)

func tempDB(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	s, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenStore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	s, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Verify file was created
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database file not created: %v", err)
	}
}

func TestInsertAndQueryNodes(t *testing.T) {
	s := tempDB(t)

	nodes := []Node{
		{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go"},
		{ID: "fn:main.go:main", Type: "function", Name: "main", Path: "main.go", Line: 10},
		{ID: "fn:main.go:helper", Type: "function", Name: "helper", Path: "main.go", Line: 25},
	}
	edges := []Edge{
		{From: "file:main.go", To: "fn:main.go:main", Type: "DEFINES"},
		{From: "file:main.go", To: "fn:main.go:helper", Type: "DEFINES"},
		{From: "fn:main.go:main", To: "fn:main.go:helper", Type: "CALLS"},
	}

	if err := s.InsertBatch(nodes, edges); err != nil {
		t.Fatal(err)
	}

	// Query nodes by type
	fns, err := s.NodesByType("function")
	if err != nil {
		t.Fatal(err)
	}
	if len(fns) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(fns))
	}

	// Query edges from a node
	outEdges, err := s.EdgesFrom("fn:main.go:main")
	if err != nil {
		t.Fatal(err)
	}
	if len(outEdges) != 1 || outEdges[0].Type != "CALLS" {
		t.Fatalf("expected 1 CALLS edge, got %v", outEdges)
	}

	// Query edges to a node (reverse)
	inEdges, err := s.EdgesTo("fn:main.go:helper")
	if err != nil {
		t.Fatal(err)
	}
	if len(inEdges) != 2 { // DEFINES + CALLS
		t.Fatalf("expected 2 inbound edges, got %d", len(inEdges))
	}
}

func TestDeleteByPath(t *testing.T) {
	s := tempDB(t)

	nodes := []Node{
		{ID: "file:a.go", Type: "file", Name: "a.go", Path: "a.go"},
		{ID: "fn:a.go:Foo", Type: "function", Name: "Foo", Path: "a.go", Line: 1},
		{ID: "file:b.go", Type: "file", Name: "b.go", Path: "b.go"},
		{ID: "fn:b.go:Bar", Type: "function", Name: "Bar", Path: "b.go", Line: 1},
	}
	edges := []Edge{
		{From: "file:a.go", To: "fn:a.go:Foo", Type: "DEFINES"},
		{From: "fn:a.go:Foo", To: "fn:b.go:Bar", Type: "CALLS"},
	}
	s.InsertBatch(nodes, edges)

	// Delete nodes for a.go
	if err := s.DeleteByPath("a.go"); err != nil {
		t.Fatal(err)
	}

	// a.go nodes should be gone
	fns, _ := s.NodesByPath("a.go")
	if len(fns) != 0 {
		t.Fatalf("expected 0 nodes for a.go, got %d", len(fns))
	}

	// b.go nodes should remain
	fns, _ = s.NodesByPath("b.go")
	if len(fns) != 2 {
		t.Fatalf("expected 2 nodes for b.go, got %d", len(fns))
	}

	// Dangling edges from deleted nodes should be gone
	edges2, _ := s.EdgesFrom("fn:a.go:Foo")
	if len(edges2) != 0 {
		t.Fatalf("expected 0 edges from deleted node, got %d", len(edges2))
	}
}

func TestSetAndGetMeta(t *testing.T) {
	s := tempDB(t)

	if err := s.SetMeta("last_commit", "abc123"); err != nil {
		t.Fatal(err)
	}
	val, err := s.GetMeta("last_commit")
	if err != nil {
		t.Fatal(err)
	}
	if val != "abc123" {
		t.Fatalf("expected abc123, got %q", val)
	}

	// Update
	s.SetMeta("last_commit", "def456")
	val, _ = s.GetMeta("last_commit")
	if val != "def456" {
		t.Fatalf("expected def456, got %q", val)
	}

	// Missing key
	val, _ = s.GetMeta("nonexistent")
	if val != "" {
		t.Fatalf("expected empty string, got %q", val)
	}
}

func TestStats(t *testing.T) {
	s := tempDB(t)

	nodes := []Node{
		{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go"},
		{ID: "fn:main.go:main", Type: "function", Name: "main", Path: "main.go", Line: 10},
	}
	edges := []Edge{
		{From: "file:main.go", To: "fn:main.go:main", Type: "DEFINES"},
	}
	s.InsertBatch(nodes, edges)

	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalNodes != 2 {
		t.Fatalf("expected 2 nodes, got %d", stats.TotalNodes)
	}
	if stats.TotalEdges != 1 {
		t.Fatalf("expected 1 edge, got %d", stats.TotalEdges)
	}
}
