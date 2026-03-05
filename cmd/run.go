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
		review, _ := cmd.Flags().GetBool("review")
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
		parallel := cfg.Parallel
		if cmd.Flags().Changed("parallel") {
			parallel, _ = cmd.Flags().GetInt("parallel")
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
			Dir:           dir,
			MaxIterations: maxIter,
			MaxTurns:      maxTurns,
			Model:         model,
			TaskOverride:  task,
			DryRun:        dryRun,
			Verbose:       verbose,
			MCPEnabled:    mcpEnabled,
			Parallel:      parallel,
			Runner:        claudeRunner,
		})
		if err != nil {
			return err
		}

		if result.Halted {
			return fmt.Errorf("loop halted: %s", result.HaltReason)
		}

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
	runCmd.Flags().Int("max-turns", 200, "max turns per Claude Code session")
	runCmd.Flags().String("task", "", "force agent to work on a specific task")
	runCmd.Flags().Bool("dry-run", false, "show rendered prompt without executing")
	runCmd.Flags().Bool("verbose", false, "extra output detail")
	runCmd.Flags().Bool("review", false, "run review pass after builder completes")
	runCmd.Flags().Bool("sandbox", false, "run Claude inside a warden sandbox container")
	runCmd.Flags().StringSlice("sandbox-tools", nil, "additional warden tools for sandbox (e.g., go,node,python)")
	runCmd.Flags().String("sandbox-timeout", "", "sandbox execution timeout (e.g., 2h, 30m)")
	runCmd.Flags().String("sandbox-memory", "", "sandbox memory limit (e.g., 8g)")
	runCmd.Flags().Bool("mcp", true, "enable golem MCP server for structured state updates")
	runCmd.Flags().Int("parallel", 1, "max parallel task sessions (1 = sequential)")
}
