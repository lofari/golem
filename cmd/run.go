// cmd/run.go
package cmd

import (
	"fmt"
	"os"

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

		maxIter, _ := cmd.Flags().GetInt("max-iterations")
		maxTurns, _ := cmd.Flags().GetInt("max-turns")
		task, _ := cmd.Flags().GetString("task")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		verbose, _ := cmd.Flags().GetBool("verbose")
		review, _ := cmd.Flags().GetBool("review")

		result, err := runner.RunBuilderLoop(runner.BuilderConfig{
			Dir:           dir,
			MaxIterations: maxIter,
			MaxTurns:      maxTurns,
			TaskOverride:  task,
			DryRun:        dryRun,
			Verbose:       verbose,
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
			return runReview(dir, maxTurns)
		}

		return nil
	},
}

// runReview is a placeholder — implemented in cmd/review.go (Task 13).
func runReview(dir string, maxTurns int) error {
	return fmt.Errorf("review not yet implemented — run `golem review` separately")
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
