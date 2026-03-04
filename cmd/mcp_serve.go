package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	golemmcp "github.com/lofari/golem/internal/mcp"
)

var mcpServeCmd = &cobra.Command{
	Use:    "mcp-serve",
	Short:  "Run the golem MCP server (stdio)",
	Hidden: true, // internal — spawned by runner, not user-facing
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, _ := cmd.Flags().GetString("dir")
		if dir == "" {
			var err error
			dir, err = os.Getwd()
			if err != nil {
				return err
			}
		}

		s := golemmcp.NewServer(dir)
		fmt.Fprintln(os.Stderr, "golem: MCP server starting on stdio")
		return s.ServeStdio()
	},
}

func init() {
	rootCmd.AddCommand(mcpServeCmd)
	mcpServeCmd.Flags().String("dir", "", "project directory")
}
