package graph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create a simple Go project
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

import "fmt"

func main() {
	fmt.Println("hello")
	helper()
}

func helper() {}
`), 0644)

	os.WriteFile(filepath.Join(dir, "util.go"), []byte(`package main

func Util() {}
`), 0644)

	// Create a Python file
	os.WriteFile(filepath.Join(dir, "script.py"), []byte(`import os

def greet():
    print("hello")
`), 0644)

	// Create an unsupported file
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Hello"), 0644)

	return dir
}

func TestBuildFull(t *testing.T) {
	dir := setupTestProject(t)
	dbPath := filepath.Join(t.TempDir(), "graph.db")

	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	b := NewBuilder(store)
	if err := b.BuildFull(dir); err != nil {
		t.Fatal(err)
	}

	stats, _ := store.Stats()

	// Should have file nodes + function nodes + type nodes
	if stats.TotalNodes < 5 {
		t.Fatalf("expected at least 5 nodes, got %d", stats.TotalNodes)
	}
	if stats.TotalEdges < 3 {
		t.Fatalf("expected at least 3 edges, got %d", stats.TotalEdges)
	}

	// Verify specific nodes exist
	nodes, _ := store.FindNodesByName("main")
	if len(nodes) == 0 {
		t.Error("expected to find 'main' function")
	}

	nodes, _ = store.FindNodesByName("greet")
	if len(nodes) == 0 {
		t.Error("expected to find 'greet' function")
	}
}

func TestBuildFullSkipsDirs(t *testing.T) {
	dir := setupTestProject(t)

	// Create a node_modules directory that should be skipped
	nmDir := filepath.Join(dir, "node_modules", "pkg")
	os.MkdirAll(nmDir, 0755)
	os.WriteFile(filepath.Join(nmDir, "index.js"), []byte("function foo() {}"), 0644)

	dbPath := filepath.Join(t.TempDir(), "graph.db")
	store, _ := OpenStore(dbPath)
	defer store.Close()

	b := NewBuilder(store)
	b.BuildFull(dir)

	// node_modules files should not be indexed
	nodes, _ := store.FindNodesByName("foo")
	if len(nodes) != 0 {
		t.Error("should not index node_modules")
	}
}

func TestBuildFullWithDocs(t *testing.T) {
	dir := t.TempDir()

	// Create a Go source file with a function
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func StartServer() {}
`), 0644)

	// Create a markdown file that references the function
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# My Project\n\nUse `StartServer` to start.\n\n## Setup\n\nRun the server.\n"), 0644)

	dbPath := filepath.Join(t.TempDir(), "graph.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	b := NewBuilder(store)
	if err := b.BuildFull(dir); err != nil {
		t.Fatal(err)
	}

	// Check document node exists
	docNodes, err := store.NodesByType("document")
	if err != nil {
		t.Fatal(err)
	}
	if len(docNodes) != 1 {
		t.Fatalf("expected 1 document node, got %d", len(docNodes))
	}
	if docNodes[0].Name != "README.md" {
		t.Errorf("doc name = %q, want %q", docNodes[0].Name, "README.md")
	}

	// Check section nodes
	secNodes, err := store.NodesByType("section")
	if err != nil {
		t.Fatal(err)
	}
	if len(secNodes) != 2 {
		t.Fatalf("expected 2 section nodes, got %d", len(secNodes))
	}

	// Check REFERENCES edge exists (README references StartServer)
	edges, err := store.EdgesOfType("sec:README.md:My Project", "REFERENCES")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range edges {
		if strings.Contains(e.To, "StartServer") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected REFERENCES edge to StartServer, edges = %v", edges)
	}
}

func TestBuildFullIdempotent(t *testing.T) {
	dir := setupTestProject(t)
	dbPath := filepath.Join(t.TempDir(), "graph.db")
	store, _ := OpenStore(dbPath)
	defer store.Close()

	b := NewBuilder(store)
	b.BuildFull(dir)
	stats1, _ := store.Stats()

	// Build again — should produce same counts
	b.BuildFull(dir)
	stats2, _ := store.Stats()

	if stats1.TotalNodes != stats2.TotalNodes {
		t.Errorf("node count changed: %d -> %d", stats1.TotalNodes, stats2.TotalNodes)
	}
	if stats1.TotalEdges != stats2.TotalEdges {
		t.Errorf("edge count changed: %d -> %d", stats1.TotalEdges, stats2.TotalEdges)
	}
}
