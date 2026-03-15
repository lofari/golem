// cmd/init.go
package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lofari/golem/internal/scaffold"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize .ctx/ directory and configure the project",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		name, _ := cmd.Flags().GetString("name")
		stack, _ := cmd.Flags().GetString("stack")
		docs, _ := cmd.Flags().GetString("docs")

		result, err := scaffold.Init(dir, scaffold.InitOptions{
			Name:     name,
			Stack:    stack,
			DocsPath: docs,
		})
		if err != nil {
			return err
		}

		fmt.Printf("Initialized .ctx/ in %s\n", dir)
		for _, f := range result.Created {
			fmt.Printf("  created %s\n", f)
		}
		for _, f := range result.Skipped {
			fmt.Printf("  skipped %s (already exists)\n", f)
		}
		for _, f := range result.Updated {
			fmt.Printf("  %s\n", f)
		}

		// Chain to setup unless --no-setup or non-interactive
		noSetup, _ := cmd.Flags().GetBool("no-setup")
		if noSetup {
			fmt.Println("\nRun `golem setup` to auto-configure, or `golem plan` to start planning.")
			return nil
		}

		if !term.IsTerminal(int(os.Stdin.Fd())) {
			fmt.Println("\nRun `golem setup` to auto-configure.")
			return nil
		}

		// Check if claude is available
		if _, err := exec.LookPath("claude"); err != nil {
			fmt.Println("\nClaude CLI not found. Install it for auto-configuration,")
			fmt.Println("or configure manually with `golem config set`.")
			return nil
		}

		fmt.Println()
		return setupCmd.RunE(cmd, nil)
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().String("name", "", "project name")
	initCmd.Flags().String("stack", "", "tech stack")
	initCmd.Flags().String("docs", "docs/", "path to design/implementation docs")
	initCmd.Flags().Bool("no-setup", false, "skip interactive setup agent")
}
