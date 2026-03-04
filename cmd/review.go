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
		_, err = runner.RunReview(ctx, dir, maxTurns, model, &runner.ClaudeRunner{
			Verbose:    verbose,
			StreamJSON: sandbox, // stream-json flushes reliably through docker
			PluginDirs: pluginDirs,
			Sandbox:    sandbox,
		})
		return err
	},
}

func init() {
	rootCmd.AddCommand(reviewCmd)
	reviewCmd.Flags().Int("max-turns", 50, "max turns for the review session")
	reviewCmd.Flags().Bool("verbose", false, "show Claude tool calls and thinking (stream-json)")
	reviewCmd.Flags().Bool("sandbox", false, "run Claude inside a warden sandbox container")
}
