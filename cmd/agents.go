package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var builtinAgents = []struct {
	Name string
	Desc string
}{
	{"build-feature", "Plan → implement → review loop"},
	{"fix-bug", "Research → fix → test loop"},
	{"write-docs", "Documentation generator"},
	{"review", "Single-pass code review"},
}

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "List available DSL agents",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Built-in agents:")
		for _, a := range builtinAgents {
			fmt.Printf("  %-20s %s\n", a.Name, a.Desc)
		}

		agentsDir := filepath.Join(".ctx", "agents")
		local, err := findProjectAgents(agentsDir)
		if err == nil && len(local) > 0 {
			fmt.Println("\nProject agents:")
			for _, name := range local {
				fmt.Printf("  %-20s .ctx/agents/%s.clj\n", name, name)
			}
		}
		return nil
	},
}

func findProjectAgents(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var agents []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".clj") {
			agents = append(agents, strings.TrimSuffix(e.Name(), ".clj"))
		}
	}
	return agents, nil
}

func init() {
	rootCmd.AddCommand(agentsCmd)
}
