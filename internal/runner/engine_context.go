package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	golemctx "github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/graph/embed"
	"github.com/lofari/golem/internal/graph/execution"
)

// setupMCP writes the MCP config file and configures the runner.
// lspEnabled controls whether the MCP server starts LSP servers.
func setupMCP(dir string, cr *ClaudeRunner, lspEnabled bool) error {
	mcpPath, err := WriteMCPConfig(dir, !lspEnabled)
	if err != nil {
		return err
	}
	cr.MCPConfig = mcpPath
	return nil
}

// syncGraph updates the knowledge graph from current source files.
// If graphPath is empty, defaults to .ctx/graph.db.
// If the graph doesn't exist, this is a no-op.
func syncGraph(dir, graphPath string) error {
	if graphPath == "" {
		graphPath = filepath.Join(dir, ".ctx", "graph.db")
	}
	if _, err := os.Stat(graphPath); os.IsNotExist(err) {
		return nil
	}

	store, err := graph.OpenStore(graphPath)
	if err != nil {
		log.Printf("golem: warning: could not open graph: %v", err)
		return nil
	}
	defer store.Close()

	builder := graph.NewBuilder(store)
	if err := builder.Sync(dir); err != nil {
		log.Printf("golem: warning: graph sync failed: %v", err)
		return nil
	}
	fmt.Fprintf(os.Stderr, "golem: graph synced\n")

	// Incremental embed if embeddings exist
	eCount, _ := store.EmbeddingCount()
	if eCount > 0 {
		modelDir, mErr := embed.EnsureModel(embed.DefaultModelID, embed.DefaultModelDir())
		if mErr != nil {
			return nil
		}
		embedder, oErr := embed.NewONNXEmbedder(modelDir)
		if oErr != nil {
			return nil
		}
		defer embedder.Close()

		p := embed.NewPipeline(store, embedder)
		if _, eErr := p.EmbedAll(context.Background()); eErr != nil {
			log.Printf("golem: warning: embed sync failed: %v", eErr)
		} else {
			fmt.Fprintf(os.Stderr, "golem: embeddings synced\n")
		}
	}

	return nil
}

// setupCollector wires an execution collector into the ClaudeRunner's stream parser.
// If graphPath is empty, defaults to .ctx/graph.db relative to dir.
// Returns a cleanup function that should be deferred, or nil if setup was skipped.
func setupCollector(dir, graphPath string, runner CommandRunner, keepSessions int) func(status string) {
	cr, ok := runner.(*ClaudeRunner)
	if !ok || !cr.StreamJSON {
		return nil
	}

	if graphPath == "" {
		graphPath = filepath.Join(dir, ".ctx", "graph.db")
	}
	if _, err := os.Stat(graphPath); os.IsNotExist(err) {
		return nil
	}

	store, err := graph.OpenStore(graphPath)
	if err != nil {
		log.Printf("golem: warning: could not open graph for collector: %v", err)
		return nil
	}

	sessionID := fmt.Sprintf("blueprint-%d", time.Now().Unix())
	collector := execution.NewCollector(store, sessionID)

	if keepSessions < 1 {
		keepSessions = 5
	}
	if _, pErr := execution.PruneSessions(store, keepSessions); pErr != nil {
		log.Printf("golem: warning: prune sessions: %v", pErr)
	}

	collector.Start()

	cr.SetupStreamCallbacks = func(parser *StreamParser) {
		parser.OnBashCommand = collector.OnBashCommand
		parser.OnBashResult = collector.OnBashResult
	}

	return func(status string) {
		collector.Finish(status)
		store.Close()
	}
}

// injectProjectContext reads decisions and pitfalls from state.yaml and
// injects them into the pipeline state as the "project-context" key.
// Steps can access this via optional-reads: [project-context] and ${project-context}.
func injectProjectContext(dir string, state map[string]any) {
	s, err := golemctx.ReadState(dir)
	if err != nil {
		return
	}
	if len(s.Decisions) == 0 && len(s.Pitfalls) == 0 {
		return
	}

	var b strings.Builder
	if len(s.Decisions) > 0 {
		b.WriteString("## Project Decisions\n\n")
		for _, d := range s.Decisions {
			b.WriteString(fmt.Sprintf("- **%s** — %s (%s)\n", d.What, d.Why, d.When))
		}
		b.WriteString("\n")
	}
	if len(s.Pitfalls) > 0 {
		b.WriteString("## Known Pitfalls\n\n")
		for _, p := range s.Pitfalls {
			b.WriteString(fmt.Sprintf("- %s\n", p.String()))
		}
		b.WriteString("\n")
	}

	state["project-context"] = b.String()
}
