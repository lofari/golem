// internal/git/git.go
package git

import (
	"fmt"
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

// FileDiff represents a single file's diff stats.
type FileDiff struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch,omitempty"`
}

// DiffSummaryResult holds the cumulative diff.
type DiffSummaryResult struct {
	BaseRef      string     `json:"baseRef"`
	Files        []FileDiff `json:"files"`
	TotalAdded   int        `json:"totalAdded"`
	TotalRemoved int        `json:"totalRemoved"`
}

// DiffSummary returns a cumulative diff from baseRef to HEAD.
// If baseRef is empty, diffs against HEAD~1.
func DiffSummary(dir string, baseRef string) (*DiffSummaryResult, error) {
	if baseRef == "" {
		baseRef = "HEAD~1"
	}

	cmd := exec.Command("git", "diff", "--numstat", baseRef+"..HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return &DiffSummaryResult{BaseRef: baseRef}, nil
	}

	result := &DiffSummaryResult{BaseRef: baseRef}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		added := 0
		deleted := 0
		if parts[0] != "-" {
			fmt.Sscanf(parts[0], "%d", &added)
		}
		if parts[1] != "-" {
			fmt.Sscanf(parts[1], "%d", &deleted)
		}
		result.Files = append(result.Files, FileDiff{
			Path:      parts[2],
			Additions: added,
			Deletions: deleted,
		})
		result.TotalAdded += added
		result.TotalRemoved += deleted
	}

	return result, nil
}

// DiffPatch returns the unified diff for a specific file from baseRef to HEAD.
func DiffPatch(dir string, baseRef string, filePath string) (string, error) {
	if baseRef == "" {
		baseRef = "HEAD~1"
	}
	cmd := exec.Command("git", "diff", baseRef+"..HEAD", "--", filePath)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
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
