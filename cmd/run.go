// cmd/run.go
package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/lofari/golem/internal/runner"
	"github.com/lofari/golem/internal/scaffold"
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

		// Chain review if requested
		if review {
			fmt.Fprintln(os.Stderr, "\ngolem: chaining review pass...")
			_, err := runner.RunReview(ctx, dir, maxTurns, model, claudeRunner)
			return err
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().Int("max-iterations", 20, "maximum number of iterations")
	runCmd.Flags().Int("max-turns", 50, "max turns per Claude Code session")
	runCmd.Flags().String("task", "", "force agent to work on a specific task")
	runCmd.Flags().Bool("dry-run", false, "show rendered prompt without executing")
	runCmd.Flags().Bool("verbose", false, "extra output detail")
	runCmd.Flags().Bool("review", false, "run review pass after builder completes")
}
