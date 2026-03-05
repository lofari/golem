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

		rc := resolveConfig(cmd, dir)
		claudeRunner := newClaudeRunner(rc)

		if rc.MCP {
			mcpPath, mcpErr := runner.WriteMCPConfig(dir)
			if mcpErr != nil {
				fmt.Fprintf(os.Stderr, "golem: warning: could not write MCP config: %v\n", mcpErr)
			} else {
				claudeRunner.MCPConfig = mcpPath
			}
		}

		_, err = runner.RunReview(ctx, dir, rc.MaxToolCalls, rc.Model, claudeRunner)
		return err
	},
}

func init() {
	rootCmd.AddCommand(reviewCmd)
	addAgentFlags(reviewCmd)
}
