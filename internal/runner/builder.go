// internal/runner/builder.go
package runner

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/lofari/golem/internal/ctx"
)

type BuilderConfig struct {
	Dir           string
	MaxIterations int
	MaxTurns      int
	TaskOverride  string
	DryRun        bool
	Verbose       bool
}

type BuilderResult struct {
	Iterations int
	Duration   time.Duration
	Completed  bool // All tasks done
	Halted     bool // Stopped due to error
	HaltReason string
}

const completePromise = "<promise>COMPLETE</promise>"

func RunBuilderLoop(cfg BuilderConfig) (BuilderResult, error) {
	startTime := time.Now()
	var result BuilderResult

	state, err := ctx.ReadState(cfg.Dir)
	if err != nil {
		return result, fmt.Errorf("reading initial state: %w", err)
	}

	if len(state.Tasks) == 0 {
		return result, fmt.Errorf("no tasks in state.yaml — run `golem plan` first")
	}

	remaining := state.RemainingTasks()
	fmt.Fprintf(os.Stderr, "golem: starting builder loop (max %d iterations)\n", cfg.MaxIterations)
	fmt.Fprintf(os.Stderr, "golem: %d tasks remaining\n\n", remaining)

	for i := 1; i <= cfg.MaxIterations; i++ {
		// Re-read state at start of each iteration
		state, err = ctx.ReadState(cfg.Dir)
		if err != nil {
			result.Halted = true
			result.HaltReason = fmt.Sprintf("reading state before iteration %d: %v", i, err)
			break
		}

		remaining = state.RemainingTasks()
		if remaining == 0 {
			result.Completed = true
			break
		}

		// Render prompt
		iterCtx := BuildIterationContext(i, cfg.MaxIterations, remaining)
		taskOverride := BuildTaskOverride(cfg.TaskOverride)
		prompt, err := RenderPrompt(cfg.Dir, "prompt.md", PromptVars{
			DocsPath:         state.Project.DocsPath,
			IterationContext: iterCtx,
			TaskOverride:     taskOverride,
		})
		if err != nil {
			result.Halted = true
			result.HaltReason = fmt.Sprintf("rendering prompt: %v", err)
			break
		}

		if cfg.DryRun {
			fmt.Fprintf(os.Stderr, "golem: [dry-run] iteration %d would run with prompt:\n%s\n", i, prompt)
			continue
		}

		fmt.Fprintf(os.Stderr, "golem: iteration %d starting...\n", i)
		iterStart := time.Now()

		// Capture state before for regression detection
		stateBefore := state

		// Spawn claude
		output, err := spawnClaude(cfg.Dir, prompt, cfg.MaxTurns)
		iterDuration := time.Since(iterStart)

		if err != nil {
			fmt.Fprintf(os.Stderr, "golem: iteration %d failed (%v) — continuing\n", i, err)
			result.Iterations = i
			continue
		}

		// Check for COMPLETE promise
		if strings.Contains(output, completePromise) {
			result.Completed = true
			result.Iterations = i
			fmt.Fprintf(os.Stderr, "golem: iteration %d complete (%s) — all tasks done\n", i, formatDuration(iterDuration))
			break
		}

		// Post-iteration: re-read state and validate
		stateAfter, readErr := ctx.ReadState(cfg.Dir)
		if readErr != nil {
			result.Halted = true
			result.HaltReason = fmt.Sprintf("state.yaml unreadable after iteration %d: %v", i, readErr)
			result.Iterations = i
			break
		}

		log, _ := ctx.ReadLog(cfg.Dir)

		validation := ValidatePostIteration(cfg.Dir, stateBefore, stateAfter, log)
		for _, w := range validation.Warnings {
			fmt.Fprintf(os.Stderr, "golem: %s\n", w)
		}
		if validation.Halted {
			result.Halted = true
			result.HaltReason = validation.Warnings[0]
			result.Iterations = i
			break
		}

		// Print iteration summary
		lastSession := lastLogSession(log)
		fmt.Fprintf(os.Stderr, "golem: iteration %d complete (%s)\n", i, formatDuration(iterDuration))
		if lastSession != nil {
			fmt.Fprintf(os.Stderr, "golem:   task: %q\n", lastSession.Task)
			fmt.Fprintf(os.Stderr, "golem:   outcome: %s\n", lastSession.Outcome)
			fmt.Fprintf(os.Stderr, "golem:   files changed: %d\n", len(lastSession.FilesChanged))
		}

		result.Iterations = i
	}

	if !result.Completed && !result.Halted && result.Iterations >= cfg.MaxIterations {
		fmt.Fprintf(os.Stderr, "golem: max iterations (%d) reached\n", cfg.MaxIterations)
	}

	result.Duration = time.Since(startTime)

	// Final summary
	state, _ = ctx.ReadState(cfg.Dir)
	remaining = state.RemainingTasks()
	if result.Completed {
		fmt.Fprintf(os.Stderr, "\ngolem: all tasks done! (%d iterations, %s)\n", result.Iterations, formatDuration(result.Duration))
	} else {
		fmt.Fprintf(os.Stderr, "\ngolem: stopped after %d iterations (%s), %d tasks remaining\n", result.Iterations, formatDuration(result.Duration), remaining)
	}

	return result, nil
}

func spawnClaude(dir string, prompt string, maxTurns int) (string, error) {
	args := []string{"-p", prompt, "--max-turns", fmt.Sprintf("%d", maxTurns)}

	cmd := exec.Command("claude", args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr

	// Stream stdout live while also capturing it
	var outputBuf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)

	if err := cmd.Run(); err != nil {
		return outputBuf.String(), fmt.Errorf("claude exited with error: %w", err)
	}

	return outputBuf.String(), nil
}

func lastLogSession(l ctx.Log) *ctx.Session {
	if len(l.Sessions) == 0 {
		return nil
	}
	s := l.Sessions[len(l.Sessions)-1]
	return &s
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%02ds", mins, secs)
}
