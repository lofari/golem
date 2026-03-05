package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

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

// ParallelResult holds results from one parallel task execution.
type ParallelResult struct {
	Task   string
	Output string
	Err    error
	Merged bool
}

// CreateWorktree creates a git worktree for a task.
// Returns the worktree path and branch name.
func CreateWorktree(projectDir string, taskName string) (string, string, error) {
	dirName := SanitizeTaskName(taskName)
	wtDir := filepath.Join(projectDir, ".ctx", "worktrees", dirName)
	branch := "golem/" + dirName

	cmd := exec.Command("git", "worktree", "add", "-b", branch, wtDir)
	cmd.Dir = projectDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("creating worktree: %s: %w", string(out), err)
	}

	return wtDir, branch, nil
}

// MergeWorktree merges a worktree branch back into the current branch.
// Returns true if merged successfully, false if conflict.
func MergeWorktree(projectDir string, branch string) (bool, error) {
	cmd := exec.Command("git", "merge", "--no-edit", branch)
	cmd.Dir = projectDir
	if out, err := cmd.CombinedOutput(); err != nil {
		// Abort the failed merge
		abortCmd := exec.Command("git", "merge", "--abort")
		abortCmd.Dir = projectDir
		abortCmd.Run()
		return false, fmt.Errorf("merge conflict: %s", string(out))
	}
	return true, nil
}

// CleanupWorktree removes a worktree and its branch.
func CleanupWorktree(projectDir string, wtDir string, branch string) {
	rmCmd := exec.Command("git", "worktree", "remove", "--force", wtDir)
	rmCmd.Dir = projectDir
	rmCmd.Run()
	brCmd := exec.Command("git", "branch", "-D", branch)
	brCmd.Dir = projectDir
	brCmd.Run()
}

// RunParallel executes N tasks concurrently in separate worktrees.
func RunParallel(ctx context.Context, cfg BuilderConfig, tasks []golemctx.Task, iteration int) []ParallelResult {
	results := make([]ParallelResult, len(tasks))
	var wg sync.WaitGroup

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t golemctx.Task) {
			defer wg.Done()
			results[idx] = runInWorktree(ctx, cfg, t, iteration)
		}(i, task)
	}

	wg.Wait()
	return results
}

func runInWorktree(ctx context.Context, cfg BuilderConfig, task golemctx.Task, iteration int) ParallelResult {
	result := ParallelResult{Task: task.Name}

	wtDir, branch, err := CreateWorktree(cfg.Dir, task.Name)
	if err != nil {
		result.Err = err
		return result
	}

	// Render prompt with forced task
	iterCtx := BuildIterationContext(iteration, cfg.MaxIterations, 1)
	taskOverride := BuildTaskOverride(task.Name)
	prompt, err := RenderPrompt(cfg.Dir, "prompt.md", PromptVars{
		IterationContext: iterCtx,
		TaskOverride:     taskOverride,
	})
	if err != nil {
		result.Err = fmt.Errorf("rendering prompt: %w", err)
		CleanupWorktree(cfg.Dir, wtDir, branch)
		return result
	}

	// Run claude in the worktree directory
	output, err := cfg.Runner.Run(ctx, wtDir, prompt, cfg.MaxToolCalls, cfg.Model)
	result.Output = output
	if err != nil {
		result.Err = err
		CleanupWorktree(cfg.Dir, wtDir, branch)
		return result
	}

	// Save session output
	SaveSessionOutput(cfg.Dir, fmt.Sprintf("parallel-%s", SanitizeTaskName(task.Name)), iteration, output)

	return result
}

// MergeParallelResults merges completed worktrees back, in alphabetical order.
func MergeParallelResults(projectDir string, results []ParallelResult) {
	// Sort by task name for deterministic merge order
	sort.Slice(results, func(i, j int) bool {
		return results[i].Task < results[j].Task
	})

	for i := range results {
		if results[i].Err != nil {
			continue
		}

		branch := "golem/" + SanitizeTaskName(results[i].Task)
		wtDir := filepath.Join(projectDir, ".ctx", "worktrees", SanitizeTaskName(results[i].Task))

		merged, err := MergeWorktree(projectDir, branch)
		if err != nil {
			fmt.Fprintf(os.Stderr, "golem: merge conflict for task %q — will retry next iteration\n", results[i].Task)
			results[i].Merged = false
		} else {
			results[i].Merged = merged
		}

		CleanupWorktree(projectDir, wtDir, branch)
	}
}
