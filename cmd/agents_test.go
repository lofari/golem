package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindProjectAgents_FindsClj(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".ctx", "agents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "my-flow.clj"), []byte("(defagent my-flow)"), 0644)

	agents, err := findProjectAgents(agentsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0] != "my-flow" {
		t.Fatalf("expected [my-flow], got %v", agents)
	}
}

func TestFindProjectAgents_IgnoresNonClj(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("hi"), 0644)
	os.WriteFile(filepath.Join(dir, "agent.clj"), []byte("ok"), 0644)

	agents, err := findProjectAgents(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0] != "agent" {
		t.Fatalf("expected [agent], got %v", agents)
	}
}

func TestFindProjectAgents_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	agents, err := findProjectAgents(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 0 {
		t.Fatalf("expected empty, got %v", agents)
	}
}

func TestFindProjectAgents_MissingDir(t *testing.T) {
	_, err := findProjectAgents("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for missing dir")
	}
}
