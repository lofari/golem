package context

import (
	gocontext "context"
	"fmt"

	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/graph/embed"
)

// candidate is an internal scored symbol during ranking.
type candidate struct {
	Node  graph.Node
	Score float64
}

// semanticCandidates finds nodes semantically similar to the task text.
func semanticCandidates(store *graph.Store, embedder embed.Embedder, taskText string, limit int) ([]candidate, error) {
	count, err := store.EmbeddingCount()
	if err != nil || count == 0 {
		return nil, nil // no embeddings — graceful degradation
	}

	vecs, err := embedder.Embed(gocontext.Background(), []string{taskText})
	if err != nil {
		return nil, fmt.Errorf("embedding task text: %w", err)
	}

	similar, err := store.SearchSimilar(vecs[0], limit)
	if err != nil {
		return nil, fmt.Errorf("searching similar: %w", err)
	}

	var candidates []candidate
	for _, r := range similar {
		node, err := store.NodeByID(r.NodeID)
		if err != nil || node == nil {
			continue
		}
		score := float64(1.0 - r.Distance)
		if score < 0 {
			score = 0
		}
		candidates = append(candidates, candidate{
			Node:  *node,
			Score: score,
		})
	}

	return candidates, nil
}
