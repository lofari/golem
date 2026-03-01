// cmd/decisions.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/display"
	"github.com/lofari/golem/internal/scaffold"
)

var decisionsCmd = &cobra.Command{
	Use:   "decisions",
	Short: "List architectural decisions",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		state, err := ctx.ReadState(dir)
		if err != nil {
			return err
		}

		display.PrintDecisions(os.Stdout, state.Decisions)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(decisionsCmd)
}
