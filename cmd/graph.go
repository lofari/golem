package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/graph/embed"
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

		depth, _ := cmd.Flags().GetInt("depth")
		builder := graph.NewBuilder(store, depth)

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

		// Print history stats
		commitCount, _ := store.CommitCount()
		authorCount, _ := store.AuthorCount()
		coCount, _ := store.CoChangedCount()
		if commitCount > 0 {
			fmt.Fprintf(os.Stderr, "golem: history — %d commits, %d authors, %d co-change pairs\n", commitCount, authorCount, coCount)
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

		// Documentation stats
		docCount := stats.NodeTypes["document"]
		secCount := stats.NodeTypes["section"]
		if docCount > 0 || secCount > 0 {
			fmt.Printf("\nDocumentation: %d documents, %d sections\n", docCount, secCount)
		}

		// Embedding stats
		embedCount, err := store.EmbeddingCount()
		if err == nil && embedCount > 0 {
			embedModel, _ := store.GetMeta("embed_model")
			embedTime, _ := store.GetMeta("embed_last_indexed")
			fmt.Printf("\nEmbeddings: %d nodes embedded", embedCount)
			if embedModel != "" {
				fmt.Printf(" (model: %s)", embedModel)
			}
			fmt.Println()
			if embedTime != "" {
				fmt.Printf("Last embedded: %s\n", embedTime)
			}
		}

		// History stats
		commitCount, _ := store.CommitCount()
		authorCount, _ := store.AuthorCount()
		coCount, _ := store.CoChangedCount()
		if commitCount > 0 {
			fmt.Printf("\nHistory: %d commits, %d authors\n", commitCount, authorCount)
			fmt.Printf("Co-change pairs: %d\n", coCount)
			historyCommit, _ := store.GetMeta("history_last_sha")
			if historyCommit != "" {
				fmt.Printf("Last history commit: %s\n", historyCommit[:min(len(historyCommit), 12)])
			}
		}

		// Execution stats
		execCount, _ := store.ExecutionCount()
		if execCount > 0 {
			cmdCount, _ := store.CommandCount()
			failCount, _ := store.FailedCommandCount()
			fmt.Printf("\nExecution: %d sessions, %d commands (%d failed)\n", execCount, cmdCount, failCount)
			if latest, _ := store.LatestExecution(); latest != nil {
				fmt.Printf("Latest session: %s (status: %s)\n", latest.SessionID, latest.Status)
			}
		}

		return nil
	},
}

var graphEmbedCmd = &cobra.Command{
	Use:   "embed",
	Short: "Generate embeddings for graph nodes",
	Long:  "Generates vector embeddings for all code nodes in the knowledge graph using a local ONNX model.",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		dbPath := filepath.Join(dir, ".ctx", "graph.db")
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			return fmt.Errorf("no graph database found — run 'golem graph build' first")
		}

		store, err := graph.OpenStore(dbPath)
		if err != nil {
			return fmt.Errorf("open graph: %w", err)
		}
		defer store.Close()

		// Ensure model is downloaded
		modelDir, err := embed.EnsureModel(embed.DefaultModelID, embed.DefaultModelDir())
		if err != nil {
			return fmt.Errorf("ensure model: %w", err)
		}

		// Create embedder
		embedder, err := embed.NewONNXEmbedder(modelDir)
		if err != nil {
			return fmt.Errorf("create embedder: %w", err)
		}
		defer embedder.Close()

		p := embed.NewPipeline(store, embedder)

		fmt.Fprintf(os.Stderr, "Embedding all graph nodes...\n")
		count, err := p.EmbedAll(cmd.Context())
		if err != nil {
			return fmt.Errorf("embed: %w", err)
		}
		_ = store.SetMeta("embed_model", embed.DefaultModelID)
		_ = store.SetMeta("embed_last_indexed", time.Now().Format(time.RFC3339))
		fmt.Fprintf(os.Stderr, "Embedded %d nodes\n", count)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(graphCmd)
	graphCmd.AddCommand(graphBuildCmd)
	graphCmd.AddCommand(graphStatusCmd)
	graphCmd.AddCommand(graphEmbedCmd)

	graphBuildCmd.Flags().IntP("depth", "d", 500, "number of git commits to index for history")
}
