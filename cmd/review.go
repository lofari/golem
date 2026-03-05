// cmd/review.go
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

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Run a single-pass code review",
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

		maxTurns, _ := cmd.Flags().GetInt("max-turns")
		model, _ := cmd.Flags().GetString("model")
		verbose, _ := cmd.Flags().GetBool("verbose")
		pluginDirs, _ := cmd.Flags().GetStringSlice("plugin-dir")
		sandbox, _ := cmd.Flags().GetBool("sandbox")
		sandboxTools, _ := cmd.Flags().GetStringSlice("sandbox-tools")
		sandboxTimeout, _ := cmd.Flags().GetString("sandbox-timeout")
		sandboxMemory, _ := cmd.Flags().GetString("sandbox-memory")
		mcpEnabled, _ := cmd.Flags().GetBool("mcp")

		claudeRunner := &runner.ClaudeRunner{
			Verbose:        verbose,
			StreamJSON:     true,
			PluginDirs:     pluginDirs,
			Sandbox:        sandbox,
			SandboxTools:   sandboxTools,
			SandboxTimeout: sandboxTimeout,
			SandboxMemory:  sandboxMemory,
		}

		if mcpEnabled {
			mcpPath, mcpErr := runner.WriteMCPConfig(dir)
			if mcpErr != nil {
				fmt.Fprintf(os.Stderr, "golem: warning: could not write MCP config: %v\n", mcpErr)
			} else {
				claudeRunner.MCPConfig = mcpPath
			}
		}

		_, err = runner.RunReview(ctx, dir, maxTurns, model, claudeRunner)
		return err
	},
}

func init() {
	rootCmd.AddCommand(reviewCmd)
	reviewCmd.Flags().Int("max-turns", 50, "max turns for the review session")
	reviewCmd.Flags().Bool("verbose", false, "show Claude tool calls and thinking (stream-json)")
	reviewCmd.Flags().Bool("sandbox", false, "run Claude inside a warden sandbox container")
	reviewCmd.Flags().StringSlice("sandbox-tools", nil, "additional warden tools for sandbox (e.g., go,node,python)")
	reviewCmd.Flags().String("sandbox-timeout", "", "sandbox execution timeout (e.g., 2h, 30m)")
	reviewCmd.Flags().String("sandbox-memory", "", "sandbox memory limit (e.g., 8g)")
	reviewCmd.Flags().Bool("mcp", true, "enable golem MCP server for structured state updates")
}
