// cmd/lock.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/scaffold"
)

var lockCmd = &cobra.Command{
	Use:   "lock <path>",
	Short: "Lock a path to prevent agent modification",
	Args:  cobra.ExactArgs(1),
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

		note, _ := cmd.Flags().GetString("note")
		path := args[0]

		// Check for duplicates
		for _, l := range state.Locked {
			if l.Path == path {
				return fmt.Errorf("path %q is already locked", path)
			}
		}

		state.Locked = append(state.Locked, ctx.Lock{Path: path, Note: note})
		if err := ctx.WriteState(dir, state); err != nil {
			return err
		}

		fmt.Printf("Locked: %s\n", path)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(lockCmd)
	lockCmd.Flags().String("note", "", "reason for locking")
}
