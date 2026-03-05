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

var qaCmd = &cobra.Command{
	Use:   "qa",
	Short: "Run QA testing — execute the app and test user flows",
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
			Dir:            dir,
			MaxIterations:  rc.MaxIterations,
			MaxTurns:       rc.MaxTurns,
			Model:          rc.Model,
			TaskOverride:   rc.Task,
			DryRun:         rc.DryRun,
			Verbose:        rc.Verbose,
			MCPEnabled:     rc.MCP,
			Runner:         claudeRunner,
			PromptTemplate: "qa-prompt.md",
		})
		if err != nil {
			return err
		}
		if result.Halted {
			return fmt.Errorf("qa halted: %s", result.HaltReason)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(qaCmd)
	addAgentFlags(qaCmd)
}
