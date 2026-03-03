// cmd/addtask.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/scaffold"
)

var addTaskCmd = &cobra.Command{
	Use:   "add-task <description>",
	Short: "Add a task to state.yaml",
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

		dependsOn, _ := cmd.Flags().GetString("depends-on")

		task := ctx.Task{
			Name:   args[0],
			Status: "todo",
		}
		if dependsOn != "" {
			task.DependsOn = ctx.FlexString{dependsOn}
		}
		state.Tasks = append(state.Tasks, task)
		if err := ctx.WriteState(dir, state); err != nil {
			return err
		}

		fmt.Printf("Added task: %s\n", args[0])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(addTaskCmd)
	addTaskCmd.Flags().String("depends-on", "", "task this depends on")
}
