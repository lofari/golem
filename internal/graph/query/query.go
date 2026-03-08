package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/graph/model"
)

// RelatedResult holds nodes and edges from a related-code traversal.
type RelatedResult struct {
	Nodes []graph.Node `json:"nodes"`
	Edges []graph.Edge `json:"edges"`
}

// Related finds code related to a name by traversing the graph.
// direction: "callers", "dependencies", "dependents", or "all".
// depth: 1-5.
func Related(store *graph.Store, name string, direction string, depth int) (RelatedResult, error) {
	if depth < 1 {
		depth = 1
	}
	if depth > 5 {
		depth = 5
	}

	// Resolve starting nodes
	targets, err := store.FindNodesByName(name)
	if err != nil {
		return RelatedResult{}, fmt.Errorf("finding nodes: %w", err)
	}
	if len(targets) == 0 {
		targets, _ = store.NodesByPath(name)
	}
	if len(targets) == 0 {
		return RelatedResult{}, nil
	}

	result := RelatedResult{}
	visited := make(map[string]bool)
	seenEdges := make(map[string]bool)

	// Add start nodes
	startIDs := make([]string, 0, len(targets))
	for _, t := range targets {
		visited[t.ID] = true
		result.Nodes = append(result.Nodes, t)
		startIDs = append(startIDs, t.ID)
	}

	current := startIDs
	for d := 0; d < depth; d++ {
		var next []string
		for _, id := range current {
			var edges []graph.Edge

			switch direction {
			case "callers":
				e, _ := store.EdgesToOfType(id, "CALLS")
				edges = append(edges, e...)
			case "dependencies":
				e, _ := store.EdgesFrom(id)
				edges = append(edges, e...)
			case "dependents":
				e, _ := store.EdgesTo(id)
				for _, edge := range e {
					if edge.Type != "DEFINES" {
						edges = append(edges, edge)
					}
				}
			case "all":
				out, _ := store.EdgesFrom(id)
				in, _ := store.EdgesTo(id)
				edges = append(edges, out...)
				edges = append(edges, in...)
			}

			for _, e := range edges {
				edgeKey := fmt.Sprintf("%s-%s-%s", e.From, e.To, e.Type)
				if seenEdges[edgeKey] {
					continue
				}
				seenEdges[edgeKey] = true
				result.Edges = append(result.Edges, e)

				targetID := e.To
				if targetID == id {
					targetID = e.From
				}
				if !visited[targetID] {
					visited[targetID] = true
					if n, _ := store.NodeByID(targetID); n != nil {
						result.Nodes = append(result.Nodes, *n)
					}
					next = append(next, targetID)
				}
			}
		}
		current = next
	}

	return result, nil
}

// SearchResult holds a single semantic search hit.
type SearchResult struct {
	Name  string  `json:"name"`
	Path  string  `json:"path"`
	Line  int     `json:"line,omitempty"`
	Type  string  `json:"type"`
	Score float32 `json:"score"`
}

// Embedder generates vector embeddings for text.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// Search performs semantic search over the graph using embeddings.
func Search(store *graph.Store, embedder Embedder, query string, limit int, types []string) ([]SearchResult, error) {
	if limit < 1 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	count, err := store.EmbeddingCount()
	if err != nil || count == 0 {
		return nil, fmt.Errorf("no embeddings found — run 'golem graph embed' first")
	}

	vecs, err := embedder.Embed(context.Background(), []string{query})
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}

	similar, err := store.SearchSimilar(vecs[0], limit)
	if err != nil {
		return nil, fmt.Errorf("searching: %w", err)
	}

	typeFilter := make(map[string]bool)
	for _, t := range types {
		typeFilter[t] = true
	}

	var results []SearchResult
	for _, r := range similar {
		node, err := store.NodeByID(r.NodeID)
		if err != nil || node == nil {
			continue
		}
		if len(typeFilter) > 0 && !typeFilter[node.Type] {
			continue
		}
		results = append(results, SearchResult{
			Name:  node.Name,
			Path:  node.Path,
			Line:  node.Line,
			Type:  node.Type,
			Score: 1.0 - r.Distance,
		})
	}

	return results, nil
}

// TraceResult holds the full execution trace.
type TraceResult struct {
	SessionID string       `json:"sessionId"`
	Status    string       `json:"status"`
	Commands  []TraceEntry `json:"commands"`
}

// TraceEntry is one command in a trace.
type TraceEntry struct {
	Command       string   `json:"command"`
	ExitCode      int      `json:"exitCode"`
	FilesAccessed []string `json:"filesAccessed,omitempty"`
	OutputPreview string   `json:"outputPreview,omitempty"`
}

// FailureResult holds failures from an execution session.
type FailureResult struct {
	SessionID   string         `json:"sessionId"`
	Failures    []FailureEntry `json:"failures"`
	FailedTests []TestEntry    `json:"failedTests"`
}

// FailureEntry is one failed command.
type FailureEntry struct {
	Command       string   `json:"command"`
	ExitCode      int      `json:"exitCode"`
	ErrorMessage  string   `json:"errorMessage,omitempty"`
	StackTrace    string   `json:"stackTrace,omitempty"`
	FilesInvolved []string `json:"filesInvolved,omitempty"`
}

// TestEntry is one test result.
type TestEntry struct {
	Name       string `json:"name"`
	Passed     bool   `json:"passed"`
	DurationMs int    `json:"durationMs,omitempty"`
	Output     string `json:"output,omitempty"`
}

// RuntimePath returns execution data for a session.
// mode: "trace" for full timeline, "failures" for errors only.
func RuntimePath(store *graph.Store, sessionID string, mode string, cmdFilter string) (interface{}, error) {
	if sessionID == "" {
		latest, err := store.LatestExecution()
		if err != nil || latest == nil {
			return nil, fmt.Errorf("no execution sessions found")
		}
		sessionID = latest.SessionID
	}

	exec, _ := store.LatestExecution()

	switch mode {
	case "failures":
		return runtimeFailures(store, sessionID)
	default:
		return runtimeTrace(store, sessionID, exec, cmdFilter)
	}
}

func runtimeTrace(store *graph.Store, sessionID string, exec *model.Execution, cmdFilter string) (*TraceResult, error) {
	cmds, err := store.QueryCommandsBySession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying commands: %w", err)
	}

	result := &TraceResult{SessionID: sessionID}
	if exec != nil {
		result.Status = exec.Status
	}

	for _, cmd := range cmds {
		if cmdFilter != "" && !strings.Contains(cmd.Command, cmdFilter) {
			continue
		}

		entry := TraceEntry{
			Command:  cmd.Command,
			ExitCode: cmd.ExitCode,
		}

		edges, _ := store.EdgesFrom(cmd.ID)
		for _, e := range edges {
			if e.Type == "ACCESSES" {
				entry.FilesAccessed = append(entry.FilesAccessed, strings.TrimPrefix(e.To, "file:"))
			}
		}

		if out, _ := store.QueryOutput(cmd.ID); out != nil && out.Stdout != "" {
			lines := strings.SplitN(out.Stdout, "\n", 6)
			if len(lines) > 5 {
				lines = lines[:5]
			}
			entry.OutputPreview = strings.Join(lines, "\n")
		}

		result.Commands = append(result.Commands, entry)
	}

	return result, nil
}

func runtimeFailures(store *graph.Store, sessionID string) (*FailureResult, error) {
	result := &FailureResult{SessionID: sessionID}

	failedCmds, err := store.QueryFailedCommands(sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying failures: %w", err)
	}

	errors, _ := store.QueryErrorsBySession(sessionID)

	for _, cmd := range failedCmds {
		var errMsg, stackTrace string
		for _, e := range errors {
			if e.CommandID == cmd.ID {
				errMsg = e.Message
				stackTrace = e.StackTrace
				break
			}
		}

		edges, _ := store.EdgesFrom(cmd.ID)
		var files []string
		for _, e := range edges {
			if e.Type == "ACCESSES" {
				files = append(files, strings.TrimPrefix(e.To, "file:"))
			}
		}

		result.Failures = append(result.Failures, FailureEntry{
			Command:       cmd.Command,
			ExitCode:      cmd.ExitCode,
			ErrorMessage:  errMsg,
			StackTrace:    stackTrace,
			FilesInvolved: files,
		})
	}

	// Failed tests
	tests, _ := store.QueryTestResults(sessionID, "failed")
	for _, tr := range tests {
		result.FailedTests = append(result.FailedTests, TestEntry{
			Name:       tr.Name,
			Passed:     tr.Passed,
			DurationMs: tr.DurationMs,
			Output:     tr.Output,
		})
	}

	return result, nil
}
