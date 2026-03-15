package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/lofari/golem/internal/config"
	"github.com/lofari/golem/internal/runner"
	"github.com/lofari/golem/internal/scaffold"
)

// resolvedConfig holds the final config after merging config files + flags.
type resolvedConfig struct {
	config.Config
	Task   string
	Goal   string
	DryRun bool
	Review bool
}

// addAgentFlags registers the common set of flags for agent commands (code, review, qa).
func addAgentFlags(cmd *cobra.Command) {
	cmd.Flags().Int("max-iterations", 20, "maximum number of iterations")
	cmd.Flags().Int("max-tool-calls", 200, "max tool calls per Claude Code session")
	cmd.Flags().String("task", "", "force agent to work on a specific task")
	cmd.Flags().Bool("dry-run", false, "show rendered prompt without executing")
	cmd.Flags().Bool("verbose", false, "extra output detail")
	cmd.Flags().Bool("sandbox", false, "run Claude inside a warden sandbox container")
	cmd.Flags().StringSlice("sandbox-tools", nil, "additional warden tools for sandbox (e.g., go,node,python)")
	cmd.Flags().String("sandbox-timeout", "", "sandbox execution timeout (e.g., 2h, 30m)")
	cmd.Flags().String("sandbox-memory", "", "sandbox memory limit (e.g., 8g)")
	cmd.Flags().Bool("mcp", true, "enable golem MCP server for structured state updates")
	cmd.Flags().Bool("no-context-map", false, "disable context map injection")
	cmd.Flags().Bool("no-lsp", false, "disable LSP servers during sessions")
	// Only add these if not already defined (run.go defines --goal separately)
	if cmd.Flags().Lookup("goal") == nil {
		cmd.Flags().String("goal", "", "goal for the blueprint engine (populates initial pipeline state)")
	}
	if cmd.Flags().Lookup("agent") == nil {
		cmd.Flags().String("agent", "", "agent to run (e.g., build-feature, fix-bug, one-shot)")
	}
	if cmd.Flags().Lookup("engine") == nil {
		cmd.Flags().String("engine", "", "execution engine: go (legacy builder), blueprint (YAML pipeline)")
	}
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
	if cmd.Flags().Changed("max-tool-calls") {
		cfg.MaxToolCalls, _ = cmd.Flags().GetInt("max-tool-calls")
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
	if cmd.Flags().Changed("no-context-map") {
		noCtx, _ := cmd.Flags().GetBool("no-context-map")
		if noCtx {
			cfg.ContextMap = false
		}
	}
	if cmd.Flags().Changed("no-lsp") {
		noLSP, _ := cmd.Flags().GetBool("no-lsp")
		if noLSP {
			cfg.LSP = false
		}
	}

	if cmd.Flags().Changed("agent") {
		cfg.Agent, _ = cmd.Flags().GetString("agent")
	}
	if cmd.Flags().Changed("engine") {
		cfg.Engine, _ = cmd.Flags().GetString("engine")
	}

	rc := resolvedConfig{Config: cfg}
	rc.Task, _ = cmd.Flags().GetString("task")
	rc.Goal, _ = cmd.Flags().GetString("goal")
	if rc.Goal == "" {
		rc.Goal = rc.Task
	}
	rc.DryRun, _ = cmd.Flags().GetBool("dry-run")
	rc.Review, _ = cmd.Flags().GetBool("review")
	return rc
}

// displayEngineEvents reads engine events and prints progress to stderr.
// Run in a goroutine; returns when the channel is closed.
func displayEngineEvents(events <-chan runner.EngineEvent) {
	for ev := range events {
		switch ev.Type {
		case "pipeline-start":
			fmt.Fprintf(os.Stderr, "golem: starting agent=%s goal=%q run=%s\n", ev.Agent, ev.Goal, ev.RunID)
		case "step-start":
			fmt.Fprintf(os.Stderr, "golem: [%s] %s starting...\n", ev.StepType, ev.Step)
		case "step-end":
			fmt.Fprintf(os.Stderr, "golem: [%s] %s %s (%.1fs)\n", ev.StepType, ev.Step, ev.Status, float64(ev.Duration)/1000)
		case "loop-enter":
			fmt.Fprintf(os.Stderr, "golem: loop %s iteration %d/%d\n", ev.Predicate, ev.Iteration, ev.Max)
		case "loop-exit":
			fmt.Fprintf(os.Stderr, "golem: loop %s exited (%s)\n", ev.Predicate, ev.Reason)
		case "conditional-skip":
			fmt.Fprintf(os.Stderr, "golem: skipped (predicate %s = false)\n", ev.Predicate)
		case "error-retry":
			fmt.Fprintf(os.Stderr, "golem: %s %s attempt %d (%s)\n", ev.ErrorType, ev.Step, ev.Attempt, ev.Action)
		case "pipeline-end":
			fmt.Fprintf(os.Stderr, "golem: pipeline %s (%.1fs)\n", ev.Status, float64(ev.Duration)/1000)
		}
	}
}

// printRunSummary prints the final state of a blueprint run.
func printRunSummary(agentName, runID string, state map[string]any) {
	fmt.Fprintf(os.Stderr, "\ngolem: run complete — %s (%s)\n", agentName, runID)
	if pr, ok := state["pr"].(map[string]any); ok {
		if url, ok := pr["url"].(string); ok {
			fmt.Fprintf(os.Stderr, "golem: PR: %s\n", url)
		}
	}
	if branch, ok := state["branch"].(string); ok {
		fmt.Fprintf(os.Stderr, "golem: branch: %s\n", branch)
	}
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
