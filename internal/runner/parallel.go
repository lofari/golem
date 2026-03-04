package runner

import (
	"regexp"
	"strings"

	golemctx "github.com/lofari/golem/internal/ctx"
)

const maxWorktreeNameLen = 70

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9-]+`)
var multiDash = regexp.MustCompile(`-{2,}`)

// EligibleTasks returns tasks that can be worked on: todo or in-progress,
// not blocked, and all dependencies are done.
func EligibleTasks(tasks []golemctx.Task) []golemctx.Task {
	doneSet := make(map[string]bool)
	for _, t := range tasks {
		if t.Status == "done" {
			doneSet[t.Name] = true
		}
	}

	var eligible []golemctx.Task
	for _, t := range tasks {
		if t.Status != "todo" && t.Status != "in-progress" {
			continue
		}
		// Check all dependencies are done
		allDepsDone := true
		for _, dep := range t.DependsOn {
			if !doneSet[dep] {
				allDepsDone = false
				break
			}
		}
		if allDepsDone {
			eligible = append(eligible, t)
		}
	}
	return eligible
}

// SanitizeTaskName converts a task name into a safe directory name for worktrees.
func SanitizeTaskName(name string) string {
	s := strings.ToLower(name)
	s = nonAlphaNum.ReplaceAllString(s, "-")
	s = multiDash.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > maxWorktreeNameLen {
		s = s[:maxWorktreeNameLen]
		s = strings.TrimRight(s, "-")
	}
	return s
}
