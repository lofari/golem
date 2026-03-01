// cmd/review.go
package cmd

import (
	"fmt"
	"os"

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

		maxTurns, _ := cmd.Flags().GetInt("max-turns")
		model, _ := cmd.Flags().GetString("model")
		_, err = runner.RunReview(cmd.Context(), dir, maxTurns, model, &runner.ClaudeRunner{})
		return err
	},
}

func init() {
	rootCmd.AddCommand(reviewCmd)
	reviewCmd.Flags().Int("max-turns", 50, "max turns for the review session")
}
