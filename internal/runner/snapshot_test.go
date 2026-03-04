package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveSnapshot(t *testing.T) {
	dir := t.TempDir()
	ctxDir := filepath.Join(dir, ".ctx")
	os.MkdirAll(ctxDir, 0755)
	os.WriteFile(filepath.Join(ctxDir, "state.yaml"), []byte("project:\n  name: test\n"), 0644)

	if err := SaveSnapshot(dir, 1); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	snapPath := filepath.Join(ctxDir, "snapshots", "state-001.yaml")
	data, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("snapshot not created: %v", err)
	}
	if string(data) != "project:\n  name: test\n" {
		t.Errorf("snapshot content = %q", string(data))
	}
}

func TestRestoreSnapshot(t *testing.T) {
	dir := t.TempDir()
	ctxDir := filepath.Join(dir, ".ctx")
	snapDir := filepath.Join(ctxDir, "snapshots")
	os.MkdirAll(snapDir, 0755)

	// Create snapshot
	os.WriteFile(filepath.Join(snapDir, "state-001.yaml"), []byte("good state"), 0644)
	// Corrupt current state
	os.WriteFile(filepath.Join(ctxDir, "state.yaml"), []byte("corrupted"), 0644)

	restored, err := RestoreLatestSnapshot(dir)
	if err != nil {
		t.Fatalf("RestoreLatestSnapshot: %v", err)
	}
	if !restored {
		t.Fatal("expected restore to succeed")
	}

	data, _ := os.ReadFile(filepath.Join(ctxDir, "state.yaml"))
	if string(data) != "good state" {
		t.Errorf("restored content = %q, want %q", string(data), "good state")
	}
}

func TestRestoreSnapshot_NoSnapshots(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	restored, err := RestoreLatestSnapshot(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if restored {
		t.Fatal("expected no restore when no snapshots exist")
	}
}

func TestPruneSnapshots(t *testing.T) {
	dir := t.TempDir()
	snapDir := filepath.Join(dir, ".ctx", "snapshots")
	os.MkdirAll(snapDir, 0755)

	// Create 12 snapshots
	for i := 1; i <= 12; i++ {
		os.WriteFile(filepath.Join(snapDir, fmt.Sprintf("state-%03d.yaml", i)), []byte("data"), 0644)
	}

	PruneSnapshots(dir, 10)

	entries, _ := filepath.Glob(filepath.Join(snapDir, "state-*.yaml"))
	if len(entries) != 10 {
		t.Errorf("after prune: %d snapshots, want 10", len(entries))
	}
	// Oldest (001, 002) should be removed
	if _, err := os.Stat(filepath.Join(snapDir, "state-001.yaml")); !os.IsNotExist(err) {
		t.Error("state-001.yaml should have been pruned")
	}
}
