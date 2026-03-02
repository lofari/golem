// cmd/run.go
package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lofari/golem/internal/runner"
	"github.com/lofari/golem/internal/scaffold"
	"github.com/lofari/golem/internal/tui"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the autonomous builder loop",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		maxIter, _ := cmd.Flags().GetInt("max-iterations")
		maxTurns, _ := cmd.Flags().GetInt("max-turns")
		task, _ := cmd.Flags().GetString("task")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		verbose, _ := cmd.Flags().GetBool("verbose")
		review, _ := cmd.Flags().GetBool("review")
		model, _ := cmd.Flags().GetString("model")
		noTUI, _ := cmd.Flags().GetBool("no-tui")

		useTUI := !noTUI && !dryRun && term.IsTerminal(int(os.Stdout.Fd()))

		if useTUI {
			return runWithTUI(ctx, dir, maxIter, maxTurns, task, verbose, review, model)
		}

		return runWithoutTUI(ctx, dir, maxIter, maxTurns, task, dryRun, verbose, review, model)
	},
}

func runWithoutTUI(ctx context.Context, dir string, maxIter, maxTurns int, task string, dryRun, verbose, review bool, model string) error {
	claudeRunner := &runner.ClaudeRunner{Verbose: verbose}

	result, err := runner.RunBuilderLoop(ctx, runner.BuilderConfig{
		Dir:           dir,
		MaxIterations: maxIter,
		MaxTurns:      maxTurns,
		Model:         model,
		TaskOverride:  task,
		DryRun:        dryRun,
		Verbose:       verbose,
		Runner:        claudeRunner,
	})
	if err != nil {
		return err
	}

	if result.Halted {
		return fmt.Errorf("loop halted: %s", result.HaltReason)
	}

	if review {
		fmt.Fprintln(os.Stderr, "\ngolem: chaining review pass...")
		_, err := runner.RunReview(ctx, dir, maxTurns, model, claudeRunner)
		return err
	}

	return nil
}

func runWithTUI(ctx context.Context, dir string, maxIter, maxTurns int, task string, verbose, review bool, model string) error {
	events := make(chan runner.Event, 100)
	outputCh := make(chan string, 1000)

	outputWriter := tui.NewLineWriter(outputCh)
	claudeRunner := &runner.ClaudeRunner{
		Verbose:      verbose,
		OutputWriter: outputWriter,
		ErrWriter:    io.Discard,
	}

	// Run builder loop in background goroutine
	go func() {
		defer close(outputCh)
		defer close(events)
		defer outputWriter.Flush()

		result, err := runner.RunBuilderLoop(ctx, runner.BuilderConfig{
			Dir:           dir,
			MaxIterations: maxIter,
			MaxTurns:      maxTurns,
			Model:         model,
			TaskOverride:  task,
			Verbose:       verbose,
			Runner:        claudeRunner,
			Events:        events,
		})
		_ = result
		_ = err
		// EventLoopDone is emitted inside RunBuilderLoop
	}()

	m := tui.NewRunModel(dir, events, outputCh)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().Int("max-iterations", 20, "maximum number of iterations")
	runCmd.Flags().Int("max-turns", 50, "max turns per Claude Code session")
	runCmd.Flags().String("task", "", "force agent to work on a specific task")
	runCmd.Flags().Bool("dry-run", false, "show rendered prompt without executing")
	runCmd.Flags().Bool("verbose", false, "extra output detail")
	runCmd.Flags().Bool("review", false, "run review pass after builder completes")
	runCmd.Flags().Bool("no-tui", false, "disable terminal UI (plain text output)")
}
