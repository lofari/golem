// internal/runner/builder.go
package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	golemctx "github.com/lofari/golem/internal/ctx"
)

type BuilderConfig struct {
	Dir           string
	MaxIterations int
	MaxTurns      int
	Model         string
	TaskOverride  string
	DryRun        bool
	Verbose       bool
	Runner        CommandRunner
	Events        chan<- Event
}

func (cfg *BuilderConfig) emit(ev Event) {
	if cfg.Events != nil {
		cfg.Events <- ev
	}
}

type BuilderResult struct {
	Iterations int
	Duration   time.Duration
	Completed  bool // All tasks done
	Halted     bool // Stopped due to error
	HaltReason string
}

const completePromise = "<promise>COMPLETE</promise>"

func RunBuilderLoop(ctx context.Context, cfg BuilderConfig) (BuilderResult, error) {
	startTime := time.Now()
	var result BuilderResult

	state, err := golemctx.ReadState(cfg.Dir)
	if err != nil {
		return result, fmt.Errorf("reading initial state: %w", err)
	}

	if len(state.Tasks) == 0 {
		return result, fmt.Errorf("no tasks in state.yaml — run `golem plan` first")
	}

	remaining := state.RemainingTasks()
	fmt.Fprintf(os.Stderr, "golem: starting builder loop (max %d iterations)\n", cfg.MaxIterations)
	fmt.Fprintf(os.Stderr, "golem: %d tasks remaining\n\n", remaining)

Loop:
	for i := 1; i <= cfg.MaxIterations; i++ {
		select {
		case <-ctx.Done():
			result.Halted = true
			result.HaltReason = "interrupted by signal"
			break Loop
		default:
		}

		// Re-read state at start of each iteration
		state, err = golemctx.ReadState(cfg.Dir)
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
		cfg.emit(Event{Type: EventIterStart, Iter: i, MaxIter: cfg.MaxIterations})
		iterStart := time.Now()

		// Capture state before for regression detection
		stateBefore := state

		// Spawn claude
		output, err := cfg.Runner.Run(ctx, cfg.Dir, prompt, cfg.MaxTurns, cfg.Model)
		iterDuration := time.Since(iterStart)

		// Save raw session output (even on error — partial output aids debugging)
		SaveSessionOutput(cfg.Dir, "build", i, output)

		if err != nil {
			fmt.Fprintf(os.Stderr, "golem: iteration %d failed (%v) — continuing\n", i, err)
			cfg.emit(Event{Type: EventIterEnd, Iter: i, Err: err})
			result.Iterations = i
			continue
		}

		// Check for COMPLETE promise
		if strings.Contains(output, completePromise) {
			result.Completed = true
			result.Iterations = i
			fmt.Fprintf(os.Stderr, "golem: iteration %d complete (%s) — all tasks done\n", i, formatDuration(iterDuration))
			cfg.emit(Event{Type: EventIterEnd, Iter: i, Task: "all tasks", Outcome: "complete"})
			break
		}

		// Post-iteration: re-read state and validate
		stateAfter, readErr := golemctx.ReadState(cfg.Dir)
		if readErr != nil {
			result.Halted = true
			result.HaltReason = fmt.Sprintf("state.yaml unreadable after iteration %d: %v", i, readErr)
			result.Iterations = i
			break
		}

		log, _ := golemctx.ReadLog(cfg.Dir)

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
			cfg.emit(Event{Type: EventIterEnd, Iter: i, Task: lastSession.Task, Outcome: lastSession.Outcome})
		} else {
			cfg.emit(Event{Type: EventIterEnd, Iter: i})
		}

		result.Iterations = i
	}

	if !result.Completed && !result.Halted && result.Iterations >= cfg.MaxIterations {
		fmt.Fprintf(os.Stderr, "golem: max iterations (%d) reached\n", cfg.MaxIterations)
	}

	result.Duration = time.Since(startTime)

	// Final summary
	state, _ = golemctx.ReadState(cfg.Dir)
	remaining = state.RemainingTasks()
	if result.Completed {
		fmt.Fprintf(os.Stderr, "\ngolem: all tasks done! (%d iterations, %s)\n", result.Iterations, formatDuration(result.Duration))
	} else {
		fmt.Fprintf(os.Stderr, "\ngolem: stopped after %d iterations (%s), %d tasks remaining\n", result.Iterations, formatDuration(result.Duration), remaining)
	}

	cfg.emit(Event{Type: EventLoopDone, Result: &result})

	return result, nil
}

func lastLogSession(l golemctx.Log) *golemctx.Session {
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

// SaveSessionOutput writes raw session output to .ctx/sessions/<type>-<NNN>.md.
func SaveSessionOutput(dir string, sessionType string, iteration int, output string) error {
	sessDir := filepath.Join(dir, ".ctx", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		return err
	}
	filename := fmt.Sprintf("%s-%03d.md", sessionType, iteration)
	return os.WriteFile(filepath.Join(sessDir, filename), []byte(output), 0644)
}

// nextSessionNumber counts existing files matching the prefix in .ctx/sessions/
// and returns the next number (1-based).
func nextSessionNumber(dir string, prefix string) int {
	sessDir := filepath.Join(dir, ".ctx", "sessions")
	pattern := filepath.Join(sessDir, prefix+"-*.md")
	matches, _ := filepath.Glob(pattern)
	return len(matches) + 1
}
