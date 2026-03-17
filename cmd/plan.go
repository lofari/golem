package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/lofari/golem/internal/config"
	golemctx "github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/runner"
	"github.com/lofari/golem/internal/scaffold"
	"github.com/lofari/golem/templates"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Open an interactive Claude Code session for planning",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		fmt.Fprintln(os.Stderr, "golem: launching interactive Claude Code session...")
		fmt.Fprintln(os.Stderr, "golem: CLAUDE.md conventions are active — the agent knows about .ctx/")
		fmt.Fprintln(os.Stderr, "golem: exit the session when planning is complete")
		fmt.Fprintln(os.Stderr)

		// Load config, then let flags override
		globalPath := config.GlobalPath()
		projectPath := config.ProjectPath(dir)
		cfg := config.Load(globalPath, projectPath)

		model := cfg.Model
		if cmd.Flags().Changed("model") {
			model, _ = cmd.Flags().GetString("model")
		}
		pluginDirs := cfg.PluginDir
		if cmd.Flags().Changed("plugin-dir") {
			pluginDirs, _ = cmd.Flags().GetStringSlice("plugin-dir")
		}

		planPrompt, err := templates.FS.ReadFile("prompts/plan-session.md")
		if err != nil {
			fmt.Fprintf(os.Stderr, "golem: warning: could not read plan prompt: %v\n", err)
		}

		claudeArgs := []string{}
		if len(planPrompt) > 0 {
			claudeArgs = append(claudeArgs, "--append-system-prompt", string(planPrompt))
		}
		if model != "" {
			claudeArgs = append(claudeArgs, "--model", model)
		}
		for _, d := range pluginDirs {
			claudeArgs = append(claudeArgs, "--plugin-dir", d)
		}
		if cfg.MCP {
			mcpPath, mcpErr := runner.WriteMCPConfig(dir, true)
			if mcpErr != nil {
				fmt.Fprintf(os.Stderr, "golem: warning: could not write MCP config: %v\n", mcpErr)
			} else {
				claudeArgs = append(claudeArgs, "--mcp-config", mcpPath)
			}
		}

		claude := exec.Command("claude", claudeArgs...)
		claude.Dir = dir
		claude.Stdin = os.Stdin
		claude.Stdout = os.Stdout
		claude.Stderr = os.Stderr

		if err := claude.Run(); err != nil {
			return err
		}

		// Validate that planning produced tasks
		state, stateErr := golemctx.ReadState(dir)
		if stateErr == nil {
			todoCount := 0
			for _, t := range state.Tasks {
				if t.Status == "todo" {
					todoCount++
				}
			}
			if todoCount == 0 {
				fmt.Fprintln(os.Stderr, "golem: warning: no tasks with status 'todo' found in state.yaml")
				fmt.Fprintln(os.Stderr, "golem: the implementer agent needs tasks to work on")
				fmt.Fprintln(os.Stderr, "golem: run `golem plan` again to create tasks, or add them manually")
			} else {
				fmt.Fprintf(os.Stderr, "golem: plan complete — %d tasks ready\n", todoCount)
				fmt.Fprintln(os.Stderr, "golem: run `golem run implementer --goal '<goal>'` to start building")
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(planCmd)
}
