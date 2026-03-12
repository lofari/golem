// internal/git/git_test.go
package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDiffSummary(t *testing.T) {
	dir := t.TempDir()

	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s", args, out)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "test")
	os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	run("add", ".")
	run("commit", "-m", "initial")

	// Get the base ref
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	baseOut, _ := cmd.Output()
	baseRef := string(baseOut[:len(baseOut)-1])

	// Make a change
	os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"), 0644)
	run("add", ".")
	run("commit", "-m", "add hello")

	summary, err := DiffSummary(dir, baseRef)
	if err != nil {
		t.Fatal(err)
	}

	if len(summary.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(summary.Files))
	}
	if summary.Files[0].Path != "hello.go" {
		t.Errorf("expected hello.go, got %s", summary.Files[0].Path)
	}
	if summary.TotalAdded == 0 {
		t.Error("expected additions > 0")
	}
}
