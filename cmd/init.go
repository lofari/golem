// cmd/init.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/lofari/golem/internal/scaffold"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize .ctx/ directory and CLAUDE.md",
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
		fmt.Println("\nRun `golem plan` to start an interactive planning session.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().String("name", "", "project name")
	initCmd.Flags().String("stack", "", "tech stack")
	initCmd.Flags().String("docs", "docs/", "path to design/implementation docs")
}
