package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/lofari/golem/internal/config"
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

		// Load config, then let flags override
		globalPath := config.GlobalPath()
		projectPath := config.ProjectPath(dir)
		cfg := config.Load(globalPath, projectPath)

		maxIter := cfg.MaxIterations
		if cmd.Flags().Changed("max-iterations") {
			maxIter, _ = cmd.Flags().GetInt("max-iterations")
		}
		maxTurns := cfg.MaxTurns
		if cmd.Flags().Changed("max-turns") {
			maxTurns, _ = cmd.Flags().GetInt("max-turns")
		}
		task, _ := cmd.Flags().GetString("task")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		verbose := cfg.Verbose
		if cmd.Flags().Changed("verbose") {
			verbose, _ = cmd.Flags().GetBool("verbose")
		}
		model := cfg.Model
		if cmd.Flags().Changed("model") {
			model, _ = cmd.Flags().GetString("model")
		}
		pluginDirs := cfg.PluginDir
		if cmd.Flags().Changed("plugin-dir") {
			pluginDirs, _ = cmd.Flags().GetStringSlice("plugin-dir")
		}
		sandbox := cfg.Sandbox
		if cmd.Flags().Changed("sandbox") {
			sandbox, _ = cmd.Flags().GetBool("sandbox")
		}
		sandboxTools := cfg.SandboxTools
		if cmd.Flags().Changed("sandbox-tools") {
			sandboxTools, _ = cmd.Flags().GetStringSlice("sandbox-tools")
		}
		sandboxTimeout := cfg.SandboxTimeout
		if cmd.Flags().Changed("sandbox-timeout") {
			sandboxTimeout, _ = cmd.Flags().GetString("sandbox-timeout")
		}
		sandboxMemory := cfg.SandboxMemory
		if cmd.Flags().Changed("sandbox-memory") {
			sandboxMemory, _ = cmd.Flags().GetString("sandbox-memory")
		}
		mcpEnabled := cfg.MCP
		if cmd.Flags().Changed("mcp") {
			mcpEnabled, _ = cmd.Flags().GetBool("mcp")
		}

		claudeRunner := &runner.ClaudeRunner{
			Verbose:        verbose,
			StreamJSON:     true,
			PluginDirs:     pluginDirs,
			Sandbox:        sandbox,
			SandboxTools:   sandboxTools,
			SandboxTimeout: sandboxTimeout,
			SandboxMemory:  sandboxMemory,
		}

		result, err := runner.RunBuilderLoop(ctx, runner.BuilderConfig{
			Dir:            dir,
			MaxIterations:  maxIter,
			MaxTurns:       maxTurns,
			Model:          model,
			TaskOverride:   task,
			DryRun:         dryRun,
			Verbose:        verbose,
			MCPEnabled:     mcpEnabled,
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
	qaCmd.Flags().Int("max-iterations", 20, "maximum number of iterations")
	qaCmd.Flags().Int("max-turns", 200, "max turns per Claude Code session")
	qaCmd.Flags().String("task", "", "force agent to test a specific task")
	qaCmd.Flags().Bool("dry-run", false, "show rendered prompt without executing")
	qaCmd.Flags().Bool("verbose", false, "extra output detail")
	qaCmd.Flags().Bool("sandbox", false, "run Claude inside a warden sandbox container")
	qaCmd.Flags().StringSlice("sandbox-tools", nil, "additional warden tools (e.g., go,node)")
	qaCmd.Flags().String("sandbox-timeout", "", "sandbox timeout (e.g., 2h)")
	qaCmd.Flags().String("sandbox-memory", "", "sandbox memory limit (e.g., 8g)")
	qaCmd.Flags().Bool("mcp", true, "enable golem MCP server")
}
