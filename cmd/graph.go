package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/scaffold"
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Manage the code knowledge graph",
}

var graphBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build or rebuild the code knowledge graph",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		dbPath := filepath.Join(dir, ".ctx", "graph.db")
		store, err := graph.OpenStore(dbPath)
		if err != nil {
			return fmt.Errorf("opening graph db: %w", err)
		}
		defer store.Close()

		builder := graph.NewBuilder(store)

		fmt.Fprintf(os.Stderr, "golem: building code graph...\n")
		if err := builder.BuildFull(dir); err != nil {
			return fmt.Errorf("building graph: %w", err)
		}

		stats, _ := store.Stats()
		fmt.Fprintf(os.Stderr, "golem: graph built — %d nodes, %d edges\n", stats.TotalNodes, stats.TotalEdges)

		// Print type breakdown
		for t, count := range stats.NodeTypes {
			fmt.Fprintf(os.Stderr, "golem:   %s: %d\n", t, count)
		}

		return nil
	},
}

var graphStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show knowledge graph statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		dbPath := filepath.Join(dir, ".ctx", "graph.db")
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			fmt.Println("No graph database found. Run `golem graph build` first.")
			return nil
		}

		store, err := graph.OpenStore(dbPath)
		if err != nil {
			return fmt.Errorf("opening graph db: %w", err)
		}
		defer store.Close()

		stats, err := store.Stats()
		if err != nil {
			return err
		}

		lastCommit, _ := store.GetMeta("last_commit")
		lastIndexed, _ := store.GetMeta("last_indexed")

		fmt.Printf("Graph Database: %s\n", dbPath)
		if lastIndexed != "" {
			fmt.Printf("Last indexed:   %s\n", lastIndexed)
		}
		if lastCommit != "" {
			fmt.Printf("Last commit:    %s\n", lastCommit[:min(len(lastCommit), 12)])
		}
		fmt.Printf("\nNodes: %d\n", stats.TotalNodes)
		for t, count := range stats.NodeTypes {
			fmt.Printf("  %-12s %d\n", t, count)
		}
		fmt.Printf("\nEdges: %d\n", stats.TotalEdges)
		for t, count := range stats.EdgeTypes {
			fmt.Printf("  %-12s %d\n", t, count)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(graphCmd)
	graphCmd.AddCommand(graphBuildCmd)
	graphCmd.AddCommand(graphStatusCmd)
}
