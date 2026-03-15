package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/lofari/golem/internal/runner"
	"github.com/lofari/golem/internal/scaffold"
	"github.com/lofari/golem/templates"
)

var codeCmd = &cobra.Command{
	Use:     "code",
	Aliases: []string{"build"},
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

		if rc.Engine == "blueprint" {
			agentName := rc.Agent
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
				Goal:       rc.Goal,
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
		}

		claudeRunner := newClaudeRunner(rc)

		noLSP, _ := cmd.Flags().GetBool("no-lsp")
		result, err := runner.RunBuilderLoop(ctx, runner.BuilderConfig{
			Dir:           dir,
			MaxIterations: rc.MaxIterations,
			MaxToolCalls:      rc.MaxToolCalls,
			Model:         rc.Model,
			TaskOverride:  rc.Task,
			DryRun:        rc.DryRun,
			Verbose:       rc.Verbose,
			MCPEnabled:    rc.MCP,
			Parallel:         rc.Parallel,
			ExecutionHistory: rc.ExecutionHistory,
			ContextMap:       rc.ContextMap,
			ContextMapLimit:  rc.ContextMapLimit,
			LSPEnabled:       rc.LSP && !noLSP,
			Runner:           claudeRunner,
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


func loadAgent(name, dir string) ([]byte, error) {
	// 1. Check .ctx/agents/<name>.yaml
	projectPath := filepath.Join(dir, ".ctx", "agents", name+".yaml")
	if data, err := os.ReadFile(projectPath); err == nil {
		return data, nil
	}
	// 2. Check embedded templates
	data, err := templates.FS.ReadFile("agents/" + name + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("agent %q not found. Searched: .ctx/agents/, built-in templates", name)
	}
	return data, nil
}

func mergeAgentConfig(agentDefaults map[string]any, agentOpts map[string]interface{}) map[string]any {
	merged := make(map[string]any)
	for k, v := range agentDefaults {
		merged[k] = v
	}
	for k, v := range agentOpts {
		merged[k] = v
	}
	return merged
}

func init() {
	rootCmd.AddCommand(codeCmd)
	addAgentFlags(codeCmd)
	codeCmd.Flags().Bool("review", false, "run review pass after builder completes")
	codeCmd.Flags().Int("parallel", 1, "max parallel task sessions (1 = sequential)")
}
