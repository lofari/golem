package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const maxSnapshots = 10

// SaveSnapshot copies state.yaml to .ctx/snapshots/state-<iteration>.yaml.
func SaveSnapshot(dir string, iteration int) error {
	src := filepath.Join(dir, ".ctx", "state.yaml")
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading state for snapshot: %w", err)
	}

	snapDir := filepath.Join(dir, ".ctx", "snapshots")
	if err := os.MkdirAll(snapDir, 0755); err != nil {
		return fmt.Errorf("creating snapshots dir: %w", err)
	}

	dst := filepath.Join(snapDir, fmt.Sprintf("state-%03d.yaml", iteration))
	return os.WriteFile(dst, data, 0644)
}

// RestoreLatestSnapshot copies the most recent snapshot back to state.yaml.
// Returns (true, nil) if restored, (false, nil) if no snapshots exist.
func RestoreLatestSnapshot(dir string) (bool, error) {
	snapDir := filepath.Join(dir, ".ctx", "snapshots")
	matches, _ := filepath.Glob(filepath.Join(snapDir, "state-*.yaml"))
	if len(matches) == 0 {
		return false, nil
	}

	sort.Strings(matches)
	latest := matches[len(matches)-1]

	data, err := os.ReadFile(latest)
	if err != nil {
		return false, fmt.Errorf("reading snapshot %s: %w", latest, err)
	}

	dst := filepath.Join(dir, ".ctx", "state.yaml")
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return false, fmt.Errorf("restoring snapshot: %w", err)
	}

	return true, nil
}

// PruneSnapshots keeps only the most recent `keep` snapshots, deleting older ones.
func PruneSnapshots(dir string, keep int) {
	snapDir := filepath.Join(dir, ".ctx", "snapshots")
	matches, _ := filepath.Glob(filepath.Join(snapDir, "state-*.yaml"))
	if len(matches) <= keep {
		return
	}
	sort.Strings(matches)
	for _, f := range matches[:len(matches)-keep] {
		os.Remove(f)
	}
}
