package graph

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initTestRepo creates a temp dir with a git repo and returns its path.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@example.com")
	run(t, dir, "git", "config", "user.name", "Test User")
	return dir
}

// commitFile creates or overwrites a file and commits it.
func commitFile(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", name)
	run(t, dir, "git", "commit", "-m", msg)
}

// commitFiles creates or overwrites multiple files in a single commit.
func commitFiles(t *testing.T, dir string, files map[string]string, msg string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, dir, "git", "add", name)
	}
	run(t, dir, "git", "commit", "-m", msg)
}

// run executes a command in the given dir and fails on error.
func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}

// openTestStore opens a temporary graph store for testing.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestComputeCoChanged(t *testing.T) {
	dir := initTestRepo(t)
	store := openTestStore(t)

	// Create 4 commits where files a.go and b.go always change together
	for i := 0; i < 4; i++ {
		commitFiles(t, dir, map[string]string{
			"a.go": fmt.Sprintf("package main // v%d", i),
			"b.go": fmt.Sprintf("package main // v%d", i),
		}, fmt.Sprintf("change a and b v%d", i))
	}

	if err := ComputeCoChanged(store, dir, 500); err != nil {
		t.Fatalf("ComputeCoChanged failed: %v", err)
	}

	// Verify CO_CHANGED edge exists
	coChangedCount, err := store.CoChangedCount()
	if err != nil {
		t.Fatal(err)
	}
	if coChangedCount != 1 {
		t.Errorf("expected 1 CO_CHANGED edge, got %d", coChangedCount)
	}

	// Verify the weight via QueryCoChanged
	results, err := store.QueryCoChanged("a.go", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 co-changed result, got %d", len(results))
	}
	if results[0].File != "b.go" {
		t.Errorf("expected co-changed file b.go, got %s", results[0].File)
	}
	if results[0].Count != 4 {
		t.Errorf("expected co-changed count 4, got %d", results[0].Count)
	}
}

func TestComputeCoChangedBelowThreshold(t *testing.T) {
	dir := initTestRepo(t)
	store := openTestStore(t)

	// Only 2 commits — below coChangedMinCount of 3
	for i := 0; i < 2; i++ {
		commitFiles(t, dir, map[string]string{
			"x.go": fmt.Sprintf("package main // v%d", i),
			"y.go": fmt.Sprintf("package main // v%d", i),
		}, fmt.Sprintf("change x and y v%d", i))
	}

	if err := ComputeCoChanged(store, dir, 500); err != nil {
		t.Fatalf("ComputeCoChanged failed: %v", err)
	}

	coChangedCount, err := store.CoChangedCount()
	if err != nil {
		t.Fatal(err)
	}
	if coChangedCount != 0 {
		t.Errorf("expected 0 CO_CHANGED edges (below threshold), got %d", coChangedCount)
	}
}

func TestComputeCoChangedDepthLimit(t *testing.T) {
	dir := initTestRepo(t)
	store := openTestStore(t)

	// Create 10 commits with a+b together
	for i := 0; i < 10; i++ {
		commitFiles(t, dir, map[string]string{
			"a.go": fmt.Sprintf("package main // v%d", i),
			"b.go": fmt.Sprintf("package main // v%d", i),
		}, fmt.Sprintf("change v%d", i))
	}

	// With depth=2, only 2 commits, below threshold — no CO_CHANGED
	if err := ComputeCoChanged(store, dir, 2); err != nil {
		t.Fatalf("ComputeCoChanged failed: %v", err)
	}

	coChangedCount, err := store.CoChangedCount()
	if err != nil {
		t.Fatal(err)
	}
	if coChangedCount != 0 {
		t.Errorf("expected 0 CO_CHANGED edges with depth=2, got %d", coChangedCount)
	}
}
