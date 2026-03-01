// cmd/status.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/display"
	"github.com/lofari/golem/internal/scaffold"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current project state",
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

		log, err := ctx.ReadLog(dir)
		if err != nil {
			return err
		}

		display.PrintStatus(os.Stdout, state, len(log.Sessions))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
