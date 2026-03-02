// internal/runner/reviewer.go
package runner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	golemctx "github.com/lofari/golem/internal/ctx"
)

type ReviewResult struct {
	Duration       time.Duration
	Approved       bool
	NeedsWork      bool
	NewReviewTasks int
	OldReviewTasks int
}

const (
	approvedPromise  = "<promise>APPROVED</promise>"
	needsWorkPromise = "<promise>NEEDS_WORK</promise>"
)

func RunReview(ctx context.Context, dir string, maxTurns int, model string, runner CommandRunner) (ReviewResult, error) {
	var result ReviewResult
	startTime := time.Now()

	// Count existing review tasks
	state, err := golemctx.ReadState(dir)
	if err != nil {
		return result, fmt.Errorf("reading state: %w", err)
	}
	result.OldReviewTasks = countReviewTasks(state)

	// Render review prompt
	prompt, err := RenderPrompt(dir, "review-prompt.md", PromptVars{
		DocsPath: state.Project.DocsPath,
	})
	if err != nil {
		return result, fmt.Errorf("rendering review prompt: %w", err)
	}

	fmt.Fprintf(os.Stderr, "golem: starting review...\n")

	output, err := runner.Run(ctx, dir, prompt, maxTurns, model)
	result.Duration = time.Since(startTime)

	// Save raw session output (even on error — partial output aids debugging)
	SaveSessionOutput(dir, "review", nextSessionNumber(dir, "review"), output)

	if err != nil {
		return result, fmt.Errorf("review failed: %w", err)
	}

	// Detect result
	result.Approved = strings.Contains(output, approvedPromise)
	result.NeedsWork = strings.Contains(output, needsWorkPromise)

	// Count new review tasks
	stateAfter, err := golemctx.ReadState(dir)
	if err != nil {
		return result, fmt.Errorf("reading state after review: %w", err)
	}
	newCount := countReviewTasks(stateAfter)
	result.NewReviewTasks = newCount - result.OldReviewTasks

	// Print results
	fmt.Fprintf(os.Stderr, "golem: review complete (%s)\n", formatDuration(result.Duration))
	if result.Approved {
		fmt.Fprintf(os.Stderr, "golem: result: APPROVED\n")
	} else if result.NeedsWork {
		fmt.Fprintf(os.Stderr, "golem: result: NEEDS_WORK\n")
	} else {
		fmt.Fprintf(os.Stderr, "golem: result: no promise detected (review may have been incomplete)\n")
	}

	if result.NewReviewTasks > 0 {
		fmt.Fprintf(os.Stderr, "golem: %d review tasks added to state.yaml", result.NewReviewTasks)
		if result.OldReviewTasks > 0 {
			fmt.Fprintf(os.Stderr, " (previous review found %d)", result.OldReviewTasks)
		}
		fmt.Fprintln(os.Stderr)

		// Print the new review tasks
		for _, t := range stateAfter.Tasks {
			if strings.HasPrefix(t.Name, "[review]") {
				// Only print if it's new (wasn't in old state)
				if state.FindTask(t.Name) == nil {
					fmt.Fprintf(os.Stderr, "golem:   %s\n", t.Name)
				}
			}
		}
	} else if result.Approved {
		fmt.Fprintf(os.Stderr, "golem: no issues found")
		if result.OldReviewTasks > 0 {
			fmt.Fprintf(os.Stderr, " (previous review found %d)", result.OldReviewTasks)
		}
		fmt.Fprintln(os.Stderr)
	}

	return result, nil
}

func countReviewTasks(state golemctx.State) int {
	count := 0
	for _, t := range state.Tasks {
		if strings.HasPrefix(t.Name, "[review]") && t.Status != "done" {
			count++
		}
	}
	return count
}
