package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/lofari/golem/internal/runner"
	"github.com/lofari/golem/internal/scaffold"
)

var runAgentCmd = &cobra.Command{
	Use:   "run <agent-name>",
	Short: "Run a blueprint agent",
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
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		rc := resolveConfig(cmd, dir)

		agentData, err := loadAgent(agentName, dir)
		if err != nil {
			return err
		}
		bp, err := runner.ParseBlueprint(agentData)
		if err != nil {
			return err
		}
		if err := bp.ValidateContracts(); err != nil {
			return err
		}

		mergedConfig := mergeAgentConfig(bp.Config, rc.AgentOpts)
		cr := newClaudeRunner(rc)
		events := make(chan runner.EngineEvent, 100)

		go displayEngineEvents(events)

		e := runner.NewEngine(runner.EngineConfig{
			Dir:        dir,
			AgentName:  agentName,
			Goal:       goal,
			Blueprint:  bp,
			Config:     mergedConfig,
			Runner:     cr,
			Model:      rc.Model,
			Events:     events,
			Verbose:    rc.Verbose,
			MCPEnabled: rc.MCP,
			LSPEnabled: rc.LSP,
		})

		state, err := e.Run(ctx)
		close(events)
		if err != nil {
			return fmt.Errorf("blueprint engine: %w", err)
		}

		printRunSummary(agentName, e.RunID, state)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(runAgentCmd)
	runAgentCmd.Flags().String("goal", "", "Goal description for the agent (required)")
	addAgentFlags(runAgentCmd)
}
