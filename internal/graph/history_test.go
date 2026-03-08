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
	// Ensure parent directory exists
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

func TestHistoryBuildBasic(t *testing.T) {
	dir := initTestRepo(t)
	store := openTestStore(t)

	// Create 3 commits touching different files
	commitFile(t, dir, "main.go", "package main", "initial commit")
	commitFile(t, dir, "utils.go", "package main", "add utils")
	commitFile(t, dir, "config.go", "package main", "add config")

	hb := NewHistoryBuilder(store, 500)
	if err := hb.Build(dir); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Verify commits
	commitCount, err := store.CommitCount()
	if err != nil {
		t.Fatal(err)
	}
	if commitCount != 3 {
		t.Errorf("expected 3 commits, got %d", commitCount)
	}

	// Verify authors
	authorCount, err := store.AuthorCount()
	if err != nil {
		t.Fatal(err)
	}
	if authorCount != 1 {
		t.Errorf("expected 1 author, got %d", authorCount)
	}

	// Verify MODIFIES edges exist: each commit should have at least one
	stats, err := store.Stats()
	if err != nil {
		t.Fatal(err)
	}
	modifiesCount := stats.EdgeTypes["MODIFIES"]
	if modifiesCount < 3 {
		t.Errorf("expected at least 3 MODIFIES edges, got %d", modifiesCount)
	}

	// Verify AUTHORED_BY edges
	authoredByCount := stats.EdgeTypes["AUTHORED_BY"]
	if authoredByCount != 3 {
		t.Errorf("expected 3 AUTHORED_BY edges, got %d", authoredByCount)
	}

	// Verify metadata
	lastSHA, err := store.GetMeta("history_last_sha")
	if err != nil {
		t.Fatal(err)
	}
	if lastSHA == "" {
		t.Error("history_last_sha should not be empty")
	}

	depth, err := store.GetMeta("history_depth")
	if err != nil {
		t.Fatal(err)
	}
	if depth != "500" {
		t.Errorf("expected history_depth=500, got %s", depth)
	}
}

func TestHistoryBuildCoChanged(t *testing.T) {
	dir := initTestRepo(t)
	store := openTestStore(t)

	// Create 3+ commits where files A and B always change together
	for i := 0; i < 4; i++ {
		commitFiles(t, dir, map[string]string{
			"a.go": fmt.Sprintf("package main // v%d", i),
			"b.go": fmt.Sprintf("package main // v%d", i),
		}, fmt.Sprintf("change a and b v%d", i))
	}

	hb := NewHistoryBuilder(store, 500)
	if err := hb.Build(dir); err != nil {
		t.Fatalf("Build failed: %v", err)
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

func TestHistoryBuildDepthLimit(t *testing.T) {
	dir := initTestRepo(t)
	store := openTestStore(t)

	// Create 10 commits
	for i := 0; i < 10; i++ {
		commitFile(t, dir, fmt.Sprintf("file%d.go", i),
			fmt.Sprintf("package main // %d", i),
			fmt.Sprintf("commit %d", i))
	}

	// Build with depth=5
	hb := NewHistoryBuilder(store, 5)
	if err := hb.Build(dir); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	commitCount, err := store.CommitCount()
	if err != nil {
		t.Fatal(err)
	}
	if commitCount != 5 {
		t.Errorf("expected 5 commits with depth=5, got %d", commitCount)
	}
}

func TestHistorySyncIncremental(t *testing.T) {
	dir := initTestRepo(t)
	store := openTestStore(t)

	// Create initial commits with co-changing files
	for i := 0; i < 3; i++ {
		commitFiles(t, dir, map[string]string{
			"a.go": fmt.Sprintf("package main // v%d", i),
			"b.go": fmt.Sprintf("package main // v%d", i),
		}, fmt.Sprintf("change a and b v%d", i))
	}

	hb := NewHistoryBuilder(store, 500)
	if err := hb.Build(dir); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	commitCountBefore, _ := store.CommitCount()
	if commitCountBefore != 3 {
		t.Fatalf("expected 3 commits before sync, got %d", commitCountBefore)
	}

	// CO_CHANGED should exist: a.go and b.go changed together 3 times
	coCount, _ := store.CoChangedCount()
	if coCount != 1 {
		t.Errorf("expected 1 CO_CHANGED edge before sync, got %d", coCount)
	}

	// Add 2 more commits
	commitFile(t, dir, "c.go", "package main", "add c")
	commitFiles(t, dir, map[string]string{
		"a.go": "package main // v_new",
		"b.go": "package main // v_new",
	}, "change a and b again")

	if err := hb.Sync(dir); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	commitCountAfter, _ := store.CommitCount()
	if commitCountAfter != 5 {
		t.Errorf("expected 5 commits after sync, got %d", commitCountAfter)
	}

	// CO_CHANGED should be recomputed: a.go+b.go now changed together 4 times
	results, err := store.QueryCoChanged("a.go", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 co-changed result after sync, got %d", len(results))
	}
	if results[0].Count != 4 {
		t.Errorf("expected co-changed count 4 after sync, got %d", results[0].Count)
	}
}

func TestHistorySyncFallback(t *testing.T) {
	dir := initTestRepo(t)
	store := openTestStore(t)

	// Create some commits without prior Build
	commitFile(t, dir, "main.go", "package main", "initial")
	commitFile(t, dir, "lib.go", "package main", "add lib")

	hb := NewHistoryBuilder(store, 500)

	// Sync with no prior build should fall back to full build
	if err := hb.Sync(dir); err != nil {
		t.Fatalf("Sync fallback failed: %v", err)
	}

	commitCount, err := store.CommitCount()
	if err != nil {
		t.Fatal(err)
	}
	if commitCount != 2 {
		t.Errorf("expected 2 commits after sync fallback, got %d", commitCount)
	}

	// Verify metadata was set
	lastSHA, err := store.GetMeta("history_last_sha")
	if err != nil {
		t.Fatal(err)
	}
	if lastSHA == "" {
		t.Error("history_last_sha should be set after sync fallback")
	}
}
