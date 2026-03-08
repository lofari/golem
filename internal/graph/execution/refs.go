package execution

import (
	"regexp"
	"strconv"
	"strings"
)

// FileRef represents a file reference found in output.
type FileRef struct {
	Path string
	Line int // 0 if unknown
}

// ParsedTestResult represents a test result parsed from output.
type ParsedTestResult struct {
	Name       string
	Passed     bool
	DurationMs int
}

// ExtractFilePaths finds project file paths in text.
// knownFiles is a set of known project-relative file paths.
func ExtractFilePaths(text string, knownFiles map[string]bool) []string {
	re := regexp.MustCompile(`(?:^|\s|["'(])([a-zA-Z0-9_./\-]+\.\w+)(?::\d+)?`)
	matches := re.FindAllStringSubmatch(text, -1)

	seen := make(map[string]bool)
	var result []string
	for _, m := range matches {
		path := m[1]
		if knownFiles[path] && !seen[path] {
			seen[path] = true
			result = append(result, path)
		}
	}
	return result
}

// ExtractGoTestResults parses Go test output for pass/fail results.
func ExtractGoTestResults(output string) []ParsedTestResult {
	re := regexp.MustCompile(`--- (PASS|FAIL): (\S+) \((\d+\.\d+)s\)`)
	matches := re.FindAllStringSubmatch(output, -1)

	var results []ParsedTestResult
	for _, m := range matches {
		duration, _ := strconv.ParseFloat(m[3], 64)
		results = append(results, ParsedTestResult{
			Name:       m[2],
			Passed:     m[1] == "PASS",
			DurationMs: int(duration * 1000),
		})
	}
	return results
}

// ExtractGoStackFiles extracts file paths from Go stack traces.
func ExtractGoStackFiles(output string) []FileRef {
	re := regexp.MustCompile(`\t(/[^\s]+\.go):(\d+)`)
	matches := re.FindAllStringSubmatch(output, -1)

	seen := make(map[string]bool)
	var refs []FileRef
	for _, m := range matches {
		absPath := m[1]
		line, _ := strconv.Atoi(m[2])

		relPath := toProjectRelative(absPath)
		if !seen[relPath] {
			seen[relPath] = true
			refs = append(refs, FileRef{Path: relPath, Line: line})
		}
	}
	return refs
}

// ExtractPythonTracebackFiles extracts file paths from Python tracebacks.
func ExtractPythonTracebackFiles(output string) []FileRef {
	re := regexp.MustCompile(`File "([^"]+\.py)", line (\d+)`)
	matches := re.FindAllStringSubmatch(output, -1)

	seen := make(map[string]bool)
	var refs []FileRef
	for _, m := range matches {
		path := m[1]
		line, _ := strconv.Atoi(m[2])
		if !seen[path] {
			seen[path] = true
			refs = append(refs, FileRef{Path: path, Line: line})
		}
	}
	return refs
}

// toProjectRelative attempts to convert an absolute path to a project-relative one.
func toProjectRelative(absPath string) string {
	markers := []string{"/internal/", "/cmd/", "/pkg/", "/src/", "/lib/", "/test/", "/tests/"}
	for _, m := range markers {
		if idx := strings.Index(absPath, m); idx >= 0 {
			return absPath[idx+1:]
		}
	}
	for _, prefix := range []string{"/home/", "/root/", "/tmp/"} {
		if idx := strings.Index(absPath, prefix); idx >= 0 {
			rest := absPath[idx+len(prefix):]
			parts := strings.SplitN(rest, "/", 3)
			if len(parts) >= 3 {
				return parts[2]
			}
		}
	}
	if idx := strings.LastIndex(absPath, "/"); idx >= 0 {
		return absPath[idx+1:]
	}
	return absPath
}
