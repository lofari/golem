// internal/runner/validate.go
package runner

import (
	"fmt"

	"github.com/lofari/golem/internal/ctx"
	gitpkg "github.com/lofari/golem/internal/git"
)

type ValidationResult struct {
	Halted   bool     // If true, the loop should stop
	Warnings []string // Non-fatal warnings to print
}

// ValidatePostIteration runs all post-iteration checks.
func ValidatePostIteration(dir string, stateBefore, stateAfter ctx.State, log ctx.Log) ValidationResult {
	var result ValidationResult

	// 1. Schema validation
	if err := ctx.ValidateState(stateAfter); err != nil {
		result.Halted = true
		result.Warnings = append(result.Warnings, fmt.Sprintf("FATAL: state.yaml validation failed: %v", err))
		return result
	}

	// 2. Locked path violation detection
	lockedPaths := make([]string, len(stateAfter.Locked))
	for i, l := range stateAfter.Locked {
		lockedPaths[i] = l.Path
	}
	if len(lockedPaths) > 0 {
		changedFiles, err := gitpkg.ChangedFiles(dir)
		if err == nil && len(changedFiles) > 0 {
			violations := gitpkg.CheckLockedPaths(changedFiles, lockedPaths)
			for _, v := range violations {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("WARNING — modified %s which is under a locked path", v))
			}
		}
	}

	// 3. Task regression detection
	beforeStatuses := make(map[string]string)
	for _, t := range stateBefore.Tasks {
		beforeStatuses[t.Name] = t.Status
	}
	for _, t := range stateAfter.Tasks {
		prev, exists := beforeStatuses[t.Name]
		if exists && prev == "done" && t.Status != "done" {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("WARNING — task %q regressed from done to %s", t.Name, t.Status))
		}
	}

	// 4. Thrashing detection
	thrashing := detectThrashing(log)
	for _, taskName := range thrashing {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("WARNING — task %q has been in-progress for 3+ consecutive iterations", taskName))
	}

	return result
}

// detectThrashing checks if any task has been the subject of 3+ consecutive
// sessions in the log.
func detectThrashing(l ctx.Log) []string {
	if len(l.Sessions) < 3 {
		return nil
	}

	var thrashing []string
	last3 := l.Sessions[len(l.Sessions)-3:]

	// Check if the same task appears in the last 3 consecutive entries
	task := last3[0].Task
	if task != "" && last3[1].Task == task && last3[2].Task == task {
		thrashing = append(thrashing, task)
	}

	return thrashing
}
