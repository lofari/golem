package context

import (
	gocontext "context"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/graph/embed"
)

const (
	semanticSearchLimit = 30
	recentCommitWindow  = 10
)

// BuildContextMap produces a ranked context map for the given task.
// Returns an empty map (not an error) when embeddings are unavailable.
func BuildContextMap(store *graph.Store, embedder embed.Embedder, taskText string, limit int) (*ContextMap, error) {
	cm := &ContextMap{Task: taskText}

	if limit < 1 {
		limit = 15
	}

	// Stage 1: Semantic search
	seeds, err := semanticCandidates(store, embedder, taskText, semanticSearchLimit)
	if err != nil {
		return cm, fmt.Errorf("semantic search: %w", err)
	}
	if len(seeds) == 0 {
		return cm, nil
	}

	// Stage 2: Structural expansion
	expanded := structuralExpansion(store, seeds)

	// Merge seeds + expanded
	all := append(seeds, expanded...)

	// Stage 3: Co-change boost
	all = coChangeBoost(store, all)

	// Stage 4: Recency boost
	all = recencyBoost(store, all, recentCommitWindow)

	// Stage 5: Failure boost
	all = failureBoost(store, all)

	// Deduplicate: keep highest score per node ID
	best := make(map[string]candidate)
	for _, c := range all {
		if existing, ok := best[c.Node.ID]; !ok || c.Score > existing.Score {
			best[c.Node.ID] = c
		}
	}

	// Convert to slice and sort
	deduped := make([]candidate, 0, len(best))
	for _, c := range best {
		deduped = append(deduped, c)
	}
	sort.Slice(deduped, func(i, j int) bool {
		return deduped[i].Score > deduped[j].Score
	})

	// Limit
	if len(deduped) > limit {
		deduped = deduped[:limit]
	}

	// Build relations and convert to SymbolEntries
	for _, c := range deduped {
		relations := buildRelations(store, c.Node)
		cm.Symbols = append(cm.Symbols, SymbolEntry{
			Name:      c.Node.Name,
			Kind:      c.Node.Type,
			Path:      c.Node.Path,
			Line:      c.Node.Line,
			Score:     c.Score,
			Relations: relations,
		})
	}

	return cm, nil
}

// buildRelations produces human-readable relation strings for a node.
func buildRelations(store *graph.Store, node graph.Node) []string {
	var relations []string

	outEdges, _ := store.EdgesFrom(node.ID)
	for _, e := range outEdges {
		if e.Type == "CALLS" {
			if target, _ := store.NodeByID(e.To); target != nil {
				relations = append(relations, "calls "+target.Name)
			}
		}
	}

	inEdges, _ := store.EdgesTo(node.ID)
	for _, e := range inEdges {
		if e.Type == "CALLS" {
			if source, _ := store.NodeByID(e.From); source != nil {
				relations = append(relations, "called by "+source.Name)
			}
		}
	}

	return relations
}

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
// Uses git directly rather than stored commit data.
func recencyBoost(store *graph.Store, candidates []candidate, recentN int) []candidate {
	// Get project dir from store metadata (last_commit implies git repo)
	projectDir, _ := store.GetMeta("project_dir")
	if projectDir == "" {
		return candidates
	}

	for i, c := range candidates {
		if c.Node.Path == "" {
			continue
		}
		cmd := exec.Command("git", "log", "--format=%H", "--since=7d", "-n", "1", "--", c.Node.Path)
		cmd.Dir = projectDir
		out, err := cmd.Output()
		if err != nil || len(strings.TrimSpace(string(out))) == 0 {
			continue
		}
		candidates[i].Score += recencyBoostMax
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
