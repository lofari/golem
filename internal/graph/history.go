package graph

import (
	"os/exec"
	"strconv"
	"strings"
)

// coChangedMinCount is the minimum number of commits two files must appear
// together in before a CO_CHANGED edge is created.
const coChangedMinCount = 3

// ComputeCoChanged parses git log and creates CO_CHANGED edges for files
// that frequently change together. Does not store commits or authors.
func ComputeCoChanged(store *Store, projectPath string, depth int) error {
	if depth <= 0 {
		depth = 500
	}

	// Clear existing CO_CHANGED edges
	store.db.Exec("DELETE FROM edges WHERE type = 'CO_CHANGED'")

	// Get file lists per commit from git log
	cmd := exec.Command("git", "log",
		"--format=%H",
		"--name-only",
		"-n", strconv.Itoa(depth),
	)
	cmd.Dir = projectPath
	out, err := cmd.Output()
	if err != nil {
		return err
	}

	// Parse: SHA line, then file lines, then blank line
	commitFiles := parseCommitFiles(string(out))

	// Count pair co-occurrences
	pairCount := make(map[[2]string]int)
	for _, files := range commitFiles {
		if len(files) < 2 {
			continue
		}
		for i := 0; i < len(files); i++ {
			for j := i + 1; j < len(files); j++ {
				a, b := files[i], files[j]
				if a > b {
					a, b = b, a
				}
				pairCount[[2]string{a, b}]++
			}
		}
	}

	for pair, count := range pairCount {
		if count >= coChangedMinCount {
			store.InsertEdgeWithWeight("file:"+pair[0], "file:"+pair[1], "CO_CHANGED", count)
		}
	}

	return nil
}

func parseCommitFiles(output string) [][]string {
	var result [][]string
	var currentFiles []string

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(currentFiles) > 0 {
				result = append(result, currentFiles)
				currentFiles = nil
			}
			continue
		}
		if isSHA(line) {
			if len(currentFiles) > 0 {
				result = append(result, currentFiles)
				currentFiles = nil
			}
			continue
		}
		currentFiles = append(currentFiles, line)
	}
	if len(currentFiles) > 0 {
		result = append(result, currentFiles)
	}
	return result
}

// isSHA returns true if the string looks like a full git SHA (40 hex chars).
func isSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
