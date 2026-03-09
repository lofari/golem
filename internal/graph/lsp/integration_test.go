package lsp_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/graph/lsp"
)

func TestIntegrationBuildWithLSP(t *testing.T) {
	goplsPath, err := exec.LookPath("gopls")
	if err != nil {
		t.Skip("gopls not installed")
	}
	_ = goplsPath

	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module testmod\n\ngo 1.21\n")
	writeFile(t, dir, "main.go", `package main

import "fmt"

func main() {
	msg := greet("world")
	fmt.Println(msg)
}

func greet(name string) string {
	return "Hello, " + name
}

type Server struct {
	Port int
}

func (s *Server) Start() error {
	return nil
}
`)

	// Init git repo for CO_CHANGED
	run(t, "git", "init", dir)
	run(t, "git", "-C", dir, "add", ".")
	run(t, "git", "-C", dir, "-c", "user.email=test@test.com", "-c", "user.name=Test", "commit", "-m", "init")

	dbPath := filepath.Join(dir, "graph.db")
	store, err := graph.OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Start LSP
	mgr := lsp.NewManager(dir)
	configs := []lsp.ServerConfig{
		{Language: "go", Binary: "gopls", Args: []string{"serve"}},
	}
	if err := mgr.Start(configs); err != nil {
		t.Fatal(err)
	}
	defer mgr.Shutdown()

	builder := graph.NewBuilder(store)
	builder.WithLSP(mgr)

	if err := builder.BuildFull(dir); err != nil {
		t.Fatalf("build: %v", err)
	}

	stats, _ := store.Stats()
	if stats.TotalNodes < 4 {
		t.Errorf("expected at least 4 nodes (file, main, greet, Server), got %d", stats.TotalNodes)
	}

	// Check for resolved CALLS edges (not call: prefix)
	edges, _ := store.EdgesFrom("fn:main.go:main")
	hasResolvedCall := false
	for _, e := range edges {
		if e.Type == "CALLS" && len(e.To) > 3 && e.To[:3] == "fn:" {
			hasResolvedCall = true
		}
	}
	if !hasResolvedCall {
		t.Error("expected resolved CALLS edge from main to greet")
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}
