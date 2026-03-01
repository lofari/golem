// cmd/block.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/scaffold"
)

var blockCmd = &cobra.Command{
	Use:   "block <task-name> <reason>",
	Short: "Mark a task as blocked",
	Args:  cobra.ExactArgs(2),
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

		task := state.FindTask(args[0])
		if task == nil {
			return fmt.Errorf("task %q not found", args[0])
		}

		task.Status = "blocked"
		task.BlockedReason = args[1]

		if err := ctx.WriteState(dir, state); err != nil {
			return err
		}

		fmt.Printf("Blocked: %s — %q\n", args[0], args[1])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(blockCmd)
}
