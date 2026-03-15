package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lofari/golem/templates"
)

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "List available agents",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Built-in agents from embedded templates
		entries, err := templates.FS.ReadDir("agents")
		if err == nil && len(entries) > 0 {
			fmt.Println("Built-in agents:")
			for _, e := range entries {
				name := strings.TrimSuffix(e.Name(), ".yaml")
				desc := ""
				// Quick parse for description
				if data, err := templates.FS.ReadFile("agents/" + e.Name()); err == nil {
					for _, line := range strings.Split(string(data), "\n") {
						if strings.HasPrefix(line, "description:") {
							desc = strings.Trim(strings.TrimPrefix(line, "description:"), " \"")
							break
						}
					}
				}
				fmt.Printf("  %-20s %s\n", name, desc)
			}
		}

		// Project-local agents
		dir, _ := os.Getwd()
		agentsDir := filepath.Join(dir, ".ctx", "agents")
		local, err := findProjectAgents(agentsDir)
		if err == nil && len(local) > 0 {
			fmt.Println("\nProject agents:")
			for _, name := range local {
				fmt.Printf("  %-20s .ctx/agents/%s.yaml\n", name, name)
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
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			agents = append(agents, strings.TrimSuffix(e.Name(), ".yaml"))
		}
	}
	return agents, nil
}

func init() {
	rootCmd.AddCommand(agentsCmd)
}
