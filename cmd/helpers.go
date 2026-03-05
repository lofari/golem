package cmd

import (
	"github.com/spf13/cobra"

	"github.com/lofari/golem/internal/config"
	"github.com/lofari/golem/internal/runner"
	"github.com/lofari/golem/internal/scaffold"
)

// resolvedConfig holds the final config after merging config files + flags.
type resolvedConfig struct {
	config.Config
	Task   string
	DryRun bool
	Review bool
}

// addAgentFlags registers the common set of flags for agent commands (code, review, qa).
func addAgentFlags(cmd *cobra.Command) {
	cmd.Flags().Int("max-iterations", 20, "maximum number of iterations")
	cmd.Flags().Int("max-turns", 200, "max turns per Claude Code session")
	cmd.Flags().String("task", "", "force agent to work on a specific task")
	cmd.Flags().Bool("dry-run", false, "show rendered prompt without executing")
	cmd.Flags().Bool("verbose", false, "extra output detail")
	cmd.Flags().Bool("sandbox", false, "run Claude inside a warden sandbox container")
	cmd.Flags().StringSlice("sandbox-tools", nil, "additional warden tools for sandbox (e.g., go,node,python)")
	cmd.Flags().String("sandbox-timeout", "", "sandbox execution timeout (e.g., 2h, 30m)")
	cmd.Flags().String("sandbox-memory", "", "sandbox memory limit (e.g., 8g)")
	cmd.Flags().Bool("mcp", true, "enable golem MCP server for structured state updates")
}

// resolveConfig loads config files and applies flag overrides.
func resolveConfig(cmd *cobra.Command, dir string) resolvedConfig {
	globalPath := config.GlobalPath()
	projectPath := ""
	if scaffold.CtxExists(dir) {
		projectPath = config.ProjectPath(dir)
	}
	cfg := config.Load(globalPath, projectPath)

	if cmd.Flags().Changed("max-iterations") {
		cfg.MaxIterations, _ = cmd.Flags().GetInt("max-iterations")
	}
	if cmd.Flags().Changed("max-turns") {
		cfg.MaxTurns, _ = cmd.Flags().GetInt("max-turns")
	}
	if cmd.Flags().Changed("verbose") {
		cfg.Verbose, _ = cmd.Flags().GetBool("verbose")
	}
	if cmd.Flags().Changed("sandbox") {
		cfg.Sandbox, _ = cmd.Flags().GetBool("sandbox")
	}
	if cmd.Flags().Changed("sandbox-tools") {
		cfg.SandboxTools, _ = cmd.Flags().GetStringSlice("sandbox-tools")
	}
	if cmd.Flags().Changed("sandbox-timeout") {
		cfg.SandboxTimeout, _ = cmd.Flags().GetString("sandbox-timeout")
	}
	if cmd.Flags().Changed("sandbox-memory") {
		cfg.SandboxMemory, _ = cmd.Flags().GetString("sandbox-memory")
	}
	if cmd.Flags().Changed("mcp") {
		cfg.MCP, _ = cmd.Flags().GetBool("mcp")
	}
	if cmd.Flags().Changed("model") {
		cfg.Model, _ = cmd.Flags().GetString("model")
	}
	if cmd.Flags().Changed("plugin-dir") {
		cfg.PluginDir, _ = cmd.Flags().GetStringSlice("plugin-dir")
	}
	if cmd.Flags().Changed("parallel") {
		cfg.Parallel, _ = cmd.Flags().GetInt("parallel")
	}

	rc := resolvedConfig{Config: cfg}
	rc.Task, _ = cmd.Flags().GetString("task")
	rc.DryRun, _ = cmd.Flags().GetBool("dry-run")
	rc.Review, _ = cmd.Flags().GetBool("review")
	return rc
}

// newClaudeRunner creates a ClaudeRunner from resolved config.
func newClaudeRunner(cfg resolvedConfig) *runner.ClaudeRunner {
	return &runner.ClaudeRunner{
		Verbose:        cfg.Verbose,
		StreamJSON:     true,
		PluginDirs:     cfg.PluginDir,
		Sandbox:        cfg.Sandbox,
		SandboxTools:   cfg.SandboxTools,
		SandboxTimeout: cfg.SandboxTimeout,
		SandboxMemory:  cfg.SandboxMemory,
	}
}
