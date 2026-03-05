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

var codeCmd = &cobra.Command{
	Use:     "code",
	Aliases: []string{"run"},
	Short:   "Run the autonomous builder loop",
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

		rc := resolveConfig(cmd, dir)
		claudeRunner := newClaudeRunner(rc)

		result, err := runner.RunBuilderLoop(ctx, runner.BuilderConfig{
			Dir:           dir,
			MaxIterations: rc.MaxIterations,
			MaxToolCalls:      rc.MaxToolCalls,
			Model:         rc.Model,
			TaskOverride:  rc.Task,
			DryRun:        rc.DryRun,
			Verbose:       rc.Verbose,
			MCPEnabled:    rc.MCP,
			Parallel:      rc.Parallel,
			Runner:        claudeRunner,
		})
		if err != nil {
			return err
		}

		if result.Halted {
			return fmt.Errorf("loop halted: %s", result.HaltReason)
		}

		if rc.Review {
			fmt.Fprintln(os.Stderr, "\ngolem: chaining review pass...")
			_, err := runner.RunReview(ctx, dir, rc.MaxToolCalls, rc.Model, claudeRunner)
			return err
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(codeCmd)
	addAgentFlags(codeCmd)
	codeCmd.Flags().Bool("review", false, "run review pass after builder completes")
	codeCmd.Flags().Int("parallel", 1, "max parallel task sessions (1 = sequential)")
}
