// cmd/pitfalls.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/display"
	"github.com/lofari/golem/internal/scaffold"
)

var pitfallsCmd = &cobra.Command{
	Use:   "pitfalls",
	Short: "List discovered pitfalls",
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

		display.PrintPitfalls(os.Stdout, state.Pitfalls)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pitfallsCmd)
}
