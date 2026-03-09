package lsp

import (
	"os/exec"
	"testing"
)

func TestExtract(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}

	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module testmod\n\ngo 1.21\n")
	writeTestFile(t, dir, "main.go", `package main

func main() {
	helper()
}

func helper() string {
	return "hello"
}

type Config struct {
	Name string
}
`)

	cfg := ServerConfig{
		Language: "go",
		Binary:   "gopls",
		Args:     []string{"serve"},
	}
	client, err := StartClient(cfg, dir)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer client.Shutdown()

	nodes, edges, err := Extract(client, dir, "main.go")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	// Should have: file node, main, helper, Config
	nodeNames := make(map[string]bool)
	for _, n := range nodes {
		nodeNames[n.Name] = true
	}

	for _, expected := range []string{"main", "helper", "Config"} {
		if !nodeNames[expected] {
			t.Errorf("missing node %q", expected)
		}
	}

	// Should have DEFINES edges
	hasDefines := false
	for _, e := range edges {
		if e.Type == "DEFINES" {
			hasDefines = true
			break
		}
	}
	if !hasDefines {
		t.Error("expected DEFINES edges")
	}

	// Should have a CALLS edge from main to helper (resolved)
	hasCalls := false
	for _, e := range edges {
		if e.Type == "CALLS" {
			hasCalls = true
			// Verify it's resolved — target should not start with "call:"
			if len(e.To) > 5 && e.To[:5] == "call:" {
				t.Error("CALLS edge target should be resolved, not a call: prefix")
			}
			break
		}
	}
	if !hasCalls {
		t.Error("expected CALLS edges")
	}
}
