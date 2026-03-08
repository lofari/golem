package context

import (
	gocontext "context"
	"fmt"
	"strings"

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

const (
	coChangeBoostPerLink = 0.1
	recencyBoostMax      = 0.15
	failureBoostValue    = 0.2
)

// coChangeBoost boosts candidates whose files co-change with other candidates' files.
func coChangeBoost(store *graph.Store, candidates []candidate) []candidate {
	// Collect all file paths in candidate set
	filePaths := make(map[string]bool)
	for _, c := range candidates {
		if c.Node.Path != "" {
			filePaths[c.Node.Path] = true
		}
	}

	for i, c := range candidates {
		if c.Node.Path == "" {
			continue
		}
		coChanged, err := store.QueryCoChanged(c.Node.Path, 1)
		if err != nil {
			continue
		}
		for _, cc := range coChanged {
			if filePaths[cc.File] {
				candidates[i].Score += coChangeBoostPerLink
			}
		}
	}

	return candidates
}

// recencyBoost boosts candidates whose files were modified in recent commits.
func recencyBoost(store *graph.Store, candidates []candidate, recentN int) []candidate {
	for i, c := range candidates {
		if c.Node.Path == "" {
			continue
		}
		commits, err := store.QueryCommitsByFile(c.Node.Path, recentN)
		if err != nil || len(commits) == 0 {
			continue
		}
		// Boost decays by position: most recent commit = full boost
		boost := recencyBoostMax * (1.0 - float64(0)/float64(recentN))
		candidates[i].Score += boost
	}

	return candidates
}

// failureBoost boosts candidates that match recent test failures.
func failureBoost(store *graph.Store, candidates []candidate) []candidate {
	// Get latest execution session
	latest, err := store.LatestExecution()
	if err != nil || latest == nil {
		return candidates
	}

	// Get failed tests from latest session
	failedTests, err := store.QueryTestResults(latest.SessionID, "failed")
	if err != nil {
		return candidates
	}

	failedNames := make(map[string]bool)
	for _, t := range failedTests {
		failedNames[strings.ToLower(t.Name)] = true
	}

	for i, c := range candidates {
		candidateLower := strings.ToLower(c.Node.Name)
		for name := range failedNames {
			if strings.Contains(name, candidateLower) || strings.Contains(candidateLower, name) {
				candidates[i].Score += failureBoostValue
				break
			}
		}
	}

	return candidates
}
