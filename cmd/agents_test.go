package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindProjectAgents_FindsYaml(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".ctx", "agents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "my-flow.yaml"), []byte("name: my-flow"), 0644)

	agents, err := findProjectAgents(agentsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0] != "my-flow" {
		t.Fatalf("expected [my-flow], got %v", agents)
	}
}

func TestFindProjectAgents_IgnoresNonYaml(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("hi"), 0644)
	os.WriteFile(filepath.Join(dir, "agent.yaml"), []byte("name: agent"), 0644)

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
