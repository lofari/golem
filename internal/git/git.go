// internal/git/git.go
package git

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// ChangedFiles returns the list of files changed in the most recent commit.
// Returns empty slice if not in a git repo or no commits.
func ChangedFiles(dir string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", "HEAD~1", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		// Not a git repo or no previous commit — not an error for us
		return nil, nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// CheckLockedPaths returns files that were changed under locked paths.
func CheckLockedPaths(changedFiles []string, lockedPaths []string) []string {
	var violations []string
	for _, file := range changedFiles {
		for _, locked := range lockedPaths {
			// Normalize: ensure locked path ends with / for directory matching
			locked = strings.TrimSuffix(locked, "/") + "/"
			if strings.HasPrefix(file, locked) || file == strings.TrimSuffix(locked, "/") {
				violations = append(violations, file)
				break
			}
		}
	}
	return violations
}

// HasUncommittedChanges checks if there are uncommitted changes in the given path.
func HasUncommittedChanges(dir string, path string) bool {
	cmd := exec.Command("git", "diff", "--name-only", "--", path)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// IsGitRepo checks if the directory is inside a git repository.
func IsGitRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	return cmd.Run() == nil
}

// StateFileModified checks if .ctx/state.yaml was modified (staged or unstaged).
func StateFileModified(dir string) bool {
	statePath := filepath.Join(".ctx", "state.yaml")
	// Check unstaged
	cmd := exec.Command("git", "diff", "--name-only", "--", statePath)
	cmd.Dir = dir
	out, _ := cmd.Output()
	if strings.TrimSpace(string(out)) != "" {
		return true
	}
	// Check staged
	cmd = exec.Command("git", "diff", "--cached", "--name-only", "--", statePath)
	cmd.Dir = dir
	out, _ = cmd.Output()
	return strings.TrimSpace(string(out)) != ""
}
