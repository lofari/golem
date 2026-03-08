package embed

import (
	"context"
	"fmt"

	"github.com/lofari/golem/internal/graph"
)

const defaultBatchSize = 32

// Pipeline orchestrates embedding graph nodes.
type Pipeline struct {
	store     *graph.Store
	embedder  Embedder
	batchSize int
}

// NewPipeline creates an embedding pipeline.
func NewPipeline(store *graph.Store, embedder Embedder) *Pipeline {
	return &Pipeline{
		store:     store,
		embedder:  embedder,
		batchSize: defaultBatchSize,
	}
}

// embeddableTypes are the node types we generate embeddings for.
var embeddableTypes = []string{"function", "method", "type", "file", "document", "section"}

// EmbedAll clears existing embeddings and embeds all eligible nodes.
// Returns the number of nodes embedded.
func (p *Pipeline) EmbedAll(ctx context.Context) (int, error) {
	if err := p.store.ClearEmbeddings(); err != nil {
		return 0, fmt.Errorf("clear embeddings: %w", err)
	}

	var allNodes []graph.Node
	for _, typ := range embeddableTypes {
		nodes, err := p.store.NodesByType(typ)
		if err != nil {
			return 0, fmt.Errorf("query nodes of type %s: %w", typ, err)
		}
		allNodes = append(allNodes, nodes...)
	}

	return p.embedNodes(ctx, allNodes)
}

// EmbedByPath embeds nodes for the given file paths (for incremental sync).
func (p *Pipeline) EmbedByPath(ctx context.Context, paths []string) (int, error) {
	var allNodes []graph.Node
	for _, path := range paths {
		if err := p.store.DeleteEmbeddingsByPath(path); err != nil {
			return 0, fmt.Errorf("delete embeddings for %s: %w", path, err)
		}
		nodes, err := p.store.NodesByPath(path)
		if err != nil {
			return 0, fmt.Errorf("query nodes for %s: %w", path, err)
		}
		allNodes = append(allNodes, nodes...)
	}

	return p.embedNodes(ctx, allNodes)
}

func (p *Pipeline) embedNodes(ctx context.Context, nodes []graph.Node) (int, error) {
	if len(nodes) == 0 {
		return 0, nil
	}

	total := 0
	for i := 0; i < len(nodes); i += p.batchSize {
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}

		end := i + p.batchSize
		if end > len(nodes) {
			end = len(nodes)
		}
		batch := nodes[i:end]

		// Build text representations
		texts := make([]string, len(batch))
		for j, n := range batch {
			texts[j] = NodeText(n, "")
		}

		// Embed
		vectors, err := p.embedder.Embed(ctx, texts)
		if err != nil {
			return total, fmt.Errorf("embed batch at offset %d: %w", i, err)
		}

		// Store
		entries := make([]graph.EmbeddingEntry, len(batch))
		for j, n := range batch {
			entries[j] = graph.EmbeddingEntry{
				NodeID: n.ID,
				Vector: vectors[j],
			}
		}
		if err := p.store.InsertEmbeddingsBatch(entries); err != nil {
			return total, fmt.Errorf("store batch at offset %d: %w", i, err)
		}
		total += len(batch)
	}
	return total, nil
}
