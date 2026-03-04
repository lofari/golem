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

	// 1. Schema validation — normalize fixable issues, only halt on truly broken state
	if err := ctx.ValidateState(stateAfter); err != nil {
		// Attempt auto-repair: clear invalid phase, fix statuses, then re-validate
		repaired := stateAfter
		if repaired.Status.Phase != "" {
			if _, ok := ctx.ValidPhases()[repaired.Status.Phase]; !ok {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("WARNING — invalid phase %q, resetting to \"building\"", repaired.Status.Phase))
				repaired.Status.Phase = "building"
			}
		}
		for i := range repaired.Tasks {
			if _, ok := ctx.ValidTaskStatuses()[repaired.Tasks[i].Status]; !ok {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("WARNING — task %q has invalid status %q, resetting to \"todo\"", repaired.Tasks[i].Name, repaired.Tasks[i].Status))
				repaired.Tasks[i].Status = "todo"
			}
			if repaired.Tasks[i].Status == "blocked" && repaired.Tasks[i].BlockedReason == "" {
				repaired.Tasks[i].BlockedReason = "no reason provided by agent"
			}
		}
		if err2 := ctx.ValidateState(repaired); err2 != nil {
			// Try snapshot restore before giving up
			restored, restoreErr := RestoreLatestSnapshot(dir)
			if restoreErr != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("WARNING — snapshot restore failed: %v", restoreErr))
			}
			if restored {
				result.Warnings = append(result.Warnings, "WARNING — state corrupted beyond repair, restored from snapshot")
				// Don't halt — next iteration will re-read restored state
				return result
			}
			// No snapshot available — halt
			result.Halted = true
			result.Warnings = append(result.Warnings, fmt.Sprintf("FATAL: state.yaml validation failed (no snapshot to restore): %v", err2))
			return result
		}
		// Write repaired state back
		if writeErr := ctx.WriteState(dir, repaired); writeErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("WARNING — could not write repaired state: %v", writeErr))
		}
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
