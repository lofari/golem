package runner

import (
	"testing"
)

func TestDSLRunner_BuildArgs(t *testing.T) {
	r := &DSLRunner{
		DSLCommand: "golem-dsl",
		Agent:      "build-feature",
		Goal:       "add auth",
		StateDir:   "/tmp/test",
	}
	args := r.buildArgs()
	expected := []string{"golem-dsl", "run", "build-feature", "--goal", "add auth", "--state-dir", "/tmp/test"}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, a := range expected {
		if args[i] != a {
			t.Fatalf("arg[%d]: expected %s, got %s", i, a, args[i])
		}
	}
}

func TestDSLRunner_BuildArgsWithOpts(t *testing.T) {
	r := &DSLRunner{
		DSLCommand: "golem-dsl",
		Agent:      "fix-bug",
		Goal:       "fix login",
		StateDir:   "/tmp/test",
		AgentOpts:  map[string]interface{}{"max_iterations": 3},
	}
	args := r.buildArgs()
	found := false
	for i, a := range args {
		if a == "--opt" && i+1 < len(args) && args[i+1] == "max_iterations=3" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected --opt max_iterations=3 in args: %v", args)
	}
}

func TestDSLRunner_CheckBinary_Missing(t *testing.T) {
	r := &DSLRunner{DSLCommand: "nonexistent-binary-xyz"}
	err := r.CheckBinary()
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}
