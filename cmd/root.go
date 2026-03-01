// cmd/root.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "golem",
	Short: "Goal-Oriented Loop Execution Manager",
	Long:  "golem runs autonomous AI coding agent loops with persistent state across iterations.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Version = version
	rootCmd.PersistentFlags().String("model", "", "Claude model to use (sonnet, opus, haiku)")
}
