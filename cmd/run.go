package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/lofari/golem/internal/runner"
)

var runAgentCmd = &cobra.Command{
	Use:   "run <agent-name>",
	Short: "Run a DSL-defined agent",
	Long:  "Run a named agent (built-in or project-local from .ctx/agents/)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		agentName := args[0]
		goal, _ := cmd.Flags().GetString("goal")
		if goal == "" {
			return fmt.Errorf("--goal is required")
		}

		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		rc := resolveConfig(cmd, dir)

		dsl := &runner.DSLRunner{
			DSLCommand: rc.DSLCommand,
			Agent:      agentName,
			Goal:       goal,
			StateDir:   dir,
			AgentOpts:  rc.AgentOpts,
			MaxIter:    rc.MaxIterations,
		}

		if err := dsl.CheckBinary(); err != nil {
			return err
		}

		result, err := dsl.Run(ctx)
		if err != nil {
			return err
		}

		if result.Completed {
			fmt.Printf("Agent %s completed successfully (%d steps)\n", agentName, result.Iterations)
		} else {
			fmt.Printf("Agent %s halted: %s\n", agentName, result.HaltReason)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(runAgentCmd)
	runAgentCmd.Flags().String("goal", "", "Goal description for the agent (required)")
	addAgentFlags(runAgentCmd)
}
