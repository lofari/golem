package runner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// findDocSection scans markdown files in docsPath for a section header matching taskName.
// Returns a hint string like "docs/impl.md section '## Task 4: Write Tests'" or "" if not found.
func findDocSection(projectDir, docsPath, taskName string) string {
	absDocsPath := filepath.Join(projectDir, docsPath)
	info, err := os.Stat(absDocsPath)
	if err != nil || !info.IsDir() {
		return ""
	}

	type match struct {
		file    string
		heading string
		modTime int64
	}
	var matches []match

	filepath.Walk(absDocsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "#") {
				continue
			}
			if matchesTaskHeading(line, taskName) {
				relPath, _ := filepath.Rel(projectDir, path)
				matches = append(matches, match{
					file:    relPath,
					heading: strings.TrimSpace(line),
					modTime: info.ModTime().UnixNano(),
				})
				break
			}
		}
		return nil
	})

	if len(matches) == 0 {
		return ""
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].modTime > matches[j].modTime
	})

	return fmt.Sprintf("%s section '%s'", matches[0].file, matches[0].heading)
}

var (
	taskPrefixRe = regexp.MustCompile(`(?i)^#{2,3}\s+Task\s*\d*[.:]*\s*`)
	numberedRe   = regexp.MustCompile(`(?i)^#{2,3}\s+\d+\.\s*`)
)

func matchesTaskHeading(line, taskName string) bool {
	taskNameLower := strings.ToLower(taskName)

	if loc := taskPrefixRe.FindStringIndex(line); loc != nil {
		remainder := strings.TrimSpace(line[loc[1]:])
		if strings.EqualFold(remainder, taskName) {
			return true
		}
		if strings.Contains(strings.ToLower(remainder), taskNameLower) {
			return true
		}
	}

	if loc := numberedRe.FindStringIndex(line); loc != nil {
		remainder := strings.TrimSpace(line[loc[1]:])
		if strings.Contains(strings.ToLower(remainder), taskNameLower) {
			return true
		}
	}

	headingText := strings.TrimLeft(line, "# ")
	headingText = strings.TrimSpace(headingText)
	if strings.Contains(strings.ToLower(headingText), taskNameLower) {
		return true
	}

	return false
}
