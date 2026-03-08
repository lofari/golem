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

const structuralDecay = 0.7

// structuralExpansion adds 1-hop callers/callees of seed candidates.
func structuralExpansion(store *graph.Store, seeds []candidate) []candidate {
	seen := make(map[string]bool)
	for _, s := range seeds {
		seen[s.Node.ID] = true
	}

	var expanded []candidate
	for _, seed := range seeds {
		// Outgoing edges (callees, dependencies)
		outEdges, _ := store.EdgesFrom(seed.Node.ID)
		for _, e := range outEdges {
			if seen[e.To] {
				continue
			}
			seen[e.To] = true
			if node, _ := store.NodeByID(e.To); node != nil {
				expanded = append(expanded, candidate{
					Node:  *node,
					Score: seed.Score * structuralDecay,
				})
			}
		}

		// Incoming edges (callers, dependents)
		inEdges, _ := store.EdgesTo(seed.Node.ID)
		for _, e := range inEdges {
			if seen[e.From] {
				continue
			}
			seen[e.From] = true
			if node, _ := store.NodeByID(e.From); node != nil {
				expanded = append(expanded, candidate{
					Node:  *node,
					Score: seed.Score * structuralDecay,
				})
			}
		}
	}

	return expanded
}
