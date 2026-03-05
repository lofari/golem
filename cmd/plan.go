package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/lofari/golem/internal/config"
	"github.com/lofari/golem/internal/scaffold"
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

		claudeArgs := []string{}
		if model != "" {
			claudeArgs = append(claudeArgs, "--model", model)
		}
		for _, d := range pluginDirs {
			claudeArgs = append(claudeArgs, "--plugin-dir", d)
		}

		claude := exec.Command("claude", claudeArgs...)
		claude.Dir = dir
		claude.Stdin = os.Stdin
		claude.Stdout = os.Stdout
		claude.Stderr = os.Stderr

		return claude.Run()
	},
}

func init() {
	rootCmd.AddCommand(planCmd)
}
