//go:build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDSL_EndToEnd_ListAgents(t *testing.T) {
	if _, err := exec.LookPath("golem-dsl"); err != nil {
		t.Skip("golem-dsl not found on PATH")
	}

	cmd := exec.Command("golem-dsl", "list")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("golem-dsl list failed: %v", err)
	}
	if !strings.Contains(string(out), "build-feature") {
		t.Fatalf("expected build-feature in agent list, got: %s", out)
	}
}

func TestDSL_EventStream_Format(t *testing.T) {
	if _, err := exec.LookPath("golem-dsl"); err != nil {
		t.Skip("golem-dsl not found on PATH")
	}

	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	cmd := exec.Command("golem-dsl", "run", "build-feature", "--goal", "test", "--state-dir", dir, "--dry-run")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("golem-dsl run --dry-run failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		t.Fatal("expected NDJSON event output")
	}

	if !strings.Contains(lines[0], `"type"`) {
		t.Fatalf("expected JSON event with type field, got: %s", lines[0])
	}
}
