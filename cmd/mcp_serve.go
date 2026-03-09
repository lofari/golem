package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/lofari/golem/internal/graph/lsp"
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

		noLSP, _ := cmd.Flags().GetBool("no-lsp")

		var lspMgr *lsp.Manager
		if !noLSP {
			files := collectMCPFileExtensions(dir)
			detected := lsp.DetectLanguages(files)
			available, _ := lsp.CheckAvailability(detected)
			if len(available) > 0 {
				lspMgr = lsp.NewManager(dir)
				if err := lspMgr.Start(available); err != nil {
					fmt.Fprintf(os.Stderr, "golem: LSP start error: %v\n", err)
				} else {
					fmt.Fprintf(os.Stderr, "golem: LSP servers started: %v\n", lspMgr.Languages())
					defer lspMgr.Shutdown()
				}
			}
		}

		s := golemmcp.NewServer(dir, lspMgr)
		fmt.Fprintln(os.Stderr, "golem: MCP server starting on stdio")
		return s.ServeStdio()
	},
}

func init() {
	rootCmd.AddCommand(mcpServeCmd)
	mcpServeCmd.Flags().String("dir", "", "project directory")
	mcpServeCmd.Flags().Bool("no-lsp", false, "disable LSP servers")
}

// collectMCPFileExtensions walks a directory for language detection in the MCP server.
func collectMCPFileExtensions(dir string) []string {
	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	// Quick scan — just top-level and one level deep for language detection
	for _, e := range entries {
		if e.IsDir() {
			if e.Name()[0] == '.' || e.Name() == "vendor" || e.Name() == "node_modules" {
				continue
			}
			subEntries, _ := os.ReadDir(fmt.Sprintf("%s/%s", dir, e.Name()))
			for _, se := range subEntries {
				if !se.IsDir() {
					files = append(files, se.Name())
				}
			}
			continue
		}
		files = append(files, e.Name())
	}
	return files
}
