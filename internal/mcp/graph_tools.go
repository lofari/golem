package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/graph/embed"
	"github.com/lofari/golem/internal/graph/query"
)

// openGraph opens the graph database for the project directory.
// Returns nil store if no graph.db exists (not an error).
func (gs *GolemServer) openGraph() (*graph.Store, error) {
	dbPath := filepath.Join(gs.dir, ".ctx", "graph.db")
	return graph.OpenStore(dbPath)
}

// --- find_callers ---

func findCallersTool() mcp.Tool {
	return mcp.Tool{
		Name:        "find_callers",
		Description: "Find what calls a given function or method. Returns nodes that have CALLS edges pointing to the target.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"name":  map[string]string{"type": "string", "description": "Function or method name to search for"},
				"depth": map[string]string{"type": "integer", "description": "Traversal depth (default 1, max 5)"},
			},
			Required: []string{"name"},
		},
	}
}

func (gs *GolemServer) handleFindCallers(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	name := getStr(args, "name")
	depth := getInt(args, "depth", 1)

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("opening graph: %v", err)), nil
	}
	defer store.Close()

	result, err := query.Related(store, name, "callers", depth)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("querying: %v", err)), nil
	}

	if len(result.Nodes) <= 1 {
		return mcp.NewToolResultText(fmt.Sprintf("no callers found for %q", name)), nil
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// --- find_dependencies ---

func findDependenciesTool() mcp.Tool {
	return mcp.Tool{
		Name:        "find_dependencies",
		Description: "Find what a file or function depends on — imports, calls, and type usage.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"name": map[string]string{"type": "string", "description": "File path or function name to search for"},
			},
			Required: []string{"name"},
		},
	}
}

func (gs *GolemServer) handleFindDependencies(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	name := getStr(args, "name")

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("opening graph: %v", err)), nil
	}
	defer store.Close()

	result, err := query.Related(store, name, "dependencies", 1)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("querying: %v", err)), nil
	}

	if len(result.Nodes) <= 1 {
		return mcp.NewToolResultText(fmt.Sprintf("no dependencies found for %q", name)), nil
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// --- find_dependents ---

func findDependentsTool() mcp.Tool {
	return mcp.Tool{
		Name:        "find_dependents",
		Description: "Find what depends on a file or symbol — what breaks if you change it.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"name": map[string]string{"type": "string", "description": "File path or symbol name to search for"},
			},
			Required: []string{"name"},
		},
	}
}

func (gs *GolemServer) handleFindDependents(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	name := getStr(args, "name")

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("opening graph: %v", err)), nil
	}
	defer store.Close()

	result, err := query.Related(store, name, "dependents", 1)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("querying: %v", err)), nil
	}

	if len(result.Nodes) <= 1 {
		return mcp.NewToolResultText(fmt.Sprintf("no dependents found for %q", name)), nil
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// --- graph_query (general traversal) ---

func graphQueryTool() mcp.Tool {
	return mcp.Tool{
		Name:        "graph_query",
		Description: "General-purpose graph traversal. Find nodes and their relationships by ID, name, or path.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"node":       map[string]string{"type": "string", "description": "Node ID, name, or file path to start from"},
				"edge_types": map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "description": "Edge types to follow (e.g. CALLS, IMPORTS, DEFINES). All if empty."},
				"depth":      map[string]string{"type": "integer", "description": "Traversal depth (default 1, max 5)"},
				"direction":  map[string]string{"type": "string", "description": "Traversal direction: outbound (default), inbound, or both"},
			},
			Required: []string{"node"},
		},
	}
}

func (gs *GolemServer) handleGraphQuery(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	nodeRef := getStr(args, "node")
	edgeTypes := getStrSlice(args, "edge_types")
	depth := getInt(args, "depth", 1)
	direction := getStr(args, "direction")
	if depth < 1 {
		depth = 1
	}
	if depth > 5 {
		depth = 5
	}
	if direction == "" {
		direction = "outbound"
	}

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("opening graph: %v", err)), nil
	}
	defer store.Close()

	// Resolve starting node(s)
	var startIDs []string
	if n, _ := store.NodeByID(nodeRef); n != nil {
		startIDs = []string{n.ID}
	} else {
		nodes, _ := store.FindNodesByName(nodeRef)
		if len(nodes) == 0 {
			nodes, _ = store.NodesByPath(nodeRef)
		}
		for _, n := range nodes {
			startIDs = append(startIDs, n.ID)
		}
	}

	if len(startIDs) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no nodes found matching %q", nodeRef)), nil
	}

	edgeFilter := make(map[string]bool)
	for _, et := range edgeTypes {
		edgeFilter[strings.ToUpper(et)] = true
	}

	type result struct {
		Nodes []graph.Node `json:"nodes"`
		Edges []graph.Edge `json:"edges"`
	}

	res := result{}
	visited := make(map[string]bool)
	seenEdges := make(map[string]bool)

	// Add start nodes
	for _, id := range startIDs {
		visited[id] = true
		if n, _ := store.NodeByID(id); n != nil {
			res.Nodes = append(res.Nodes, *n)
		}
	}

	current := startIDs
	for d := 0; d < depth; d++ {
		var next []string
		for _, id := range current {
			var edges []graph.Edge
			if direction == "outbound" || direction == "both" {
				out, _ := store.EdgesFrom(id)
				edges = append(edges, out...)
			}
			if direction == "inbound" || direction == "both" {
				in, _ := store.EdgesTo(id)
				edges = append(edges, in...)
			}

			for _, e := range edges {
				if len(edgeFilter) > 0 && !edgeFilter[e.Type] {
					continue
				}
				edgeKey := fmt.Sprintf("%s-%s-%s", e.From, e.To, e.Type)
				if seenEdges[edgeKey] {
					continue
				}
				seenEdges[edgeKey] = true
				res.Edges = append(res.Edges, e)

				// Add target node
				targetID := e.To
				if targetID == id {
					targetID = e.From
				}
				if !visited[targetID] {
					visited[targetID] = true
					if n, _ := store.NodeByID(targetID); n != nil {
						res.Nodes = append(res.Nodes, *n)
					}
					next = append(next, targetID)
				}
			}
		}
		current = next
	}

	out, _ := json.MarshalIndent(res, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// --- semantic_search ---

func semanticSearchTool() mcp.Tool {
	return mcp.Tool{
		Name:        "semantic_search",
		Description: "Search code and documentation by natural language query. Returns the most semantically similar nodes in the knowledge graph.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Natural language search query (e.g. 'authentication logic', 'database connection handling')",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results (default: 10, max: 50)",
				},
				"types": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Filter by node types: function, method, type, file, document, section (default: all)",
				},
			},
			Required: []string{"query"},
		},
	}
}

func (gs *GolemServer) handleSemanticSearch(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()

	q, ok := args["query"].(string)
	if !ok || q == "" {
		return mcp.NewToolResultError("query is required"), nil
	}

	limit := getInt(args, "limit", 10)
	types := getStrSlice(args, "types")

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("open graph: %v", err)), nil
	}
	defer store.Close()

	modelDir, err := embed.EnsureModel(embed.DefaultModelID, embed.DefaultModelDir())
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("load model: %v", err)), nil
	}
	embedder, err := embed.NewONNXEmbedder(modelDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("create embedder: %v", err)), nil
	}
	defer embedder.Close()

	results, err := query.Search(store, embedder, q, limit, types)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	data, _ := json.Marshal(results)
	return mcp.NewToolResultText(string(data)), nil
}

// --- find_recent_changes ---

func findRecentChangesTool() mcp.Tool {
	return mcp.Tool{
		Name:        "find_recent_changes",
		Description: "Find recent commits that modified a file or directory. Returns commit details with changed files.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"path":  map[string]string{"type": "string", "description": "File or directory path"},
				"limit": map[string]string{"type": "integer", "description": "Maximum number of commits to return (default 10, max 50)"},
			},
			Required: []string{"path"},
		},
	}
}

func (gs *GolemServer) handleFindRecentChanges(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	path := getStr(args, "path")
	limit := getInt(args, "limit", 10)
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("opening graph: %v", err)), nil
	}
	defer store.Close()

	// Try exact file match first, then prefix match for directories
	nodeID := "file:" + path
	commits, err := store.QueryRecentChanges(nodeID, false, limit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("querying commits: %v", err)), nil
	}

	// If no exact match, try prefix match (directory)
	if len(commits) == 0 {
		prefix := "file:" + path
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		commits, err = store.QueryRecentChanges(prefix, true, limit)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("querying commits: %v", err)), nil
		}
	}

	if len(commits) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no recent changes found for %q", path)), nil
	}

	type commitResult struct {
		SHA       string   `json:"sha"`
		Message   string   `json:"message"`
		Author    string   `json:"author"`
		Timestamp int64    `json:"timestamp"`
		Files     []string `json:"files"`
	}

	var results []commitResult
	for _, c := range commits {
		author := c.AuthorEmail
		if a, _ := store.QueryAuthorByEmail(c.AuthorEmail); a != nil {
			author = a.Name
		}
		files, _ := store.QueryFilesModifiedByCommit(c.SHA)
		results = append(results, commitResult{
			SHA:       c.SHA,
			Message:   c.Message,
			Author:    author,
			Timestamp: c.Timestamp,
			Files:     files,
		})
	}

	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// --- find_file_history ---

func findFileHistoryTool() mcp.Tool {
	return mcp.Tool{
		Name:        "find_file_history",
		Description: "Get the commit history for a specific file. Shows who changed it and when.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"file":  map[string]string{"type": "string", "description": "File path"},
				"limit": map[string]string{"type": "integer", "description": "Maximum number of commits to return (default 20, max 100)"},
			},
			Required: []string{"file"},
		},
	}
}

func (gs *GolemServer) handleFindFileHistory(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	file := getStr(args, "file")
	limit := getInt(args, "limit", 20)
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("opening graph: %v", err)), nil
	}
	defer store.Close()

	commits, err := store.QueryCommitsByFile(file, limit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("querying file history: %v", err)), nil
	}

	if len(commits) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no commit history found for %q", file)), nil
	}

	type historyEntry struct {
		SHA       string `json:"sha"`
		Message   string `json:"message"`
		Author    string `json:"author"`
		Timestamp int64  `json:"timestamp"`
	}

	var results []historyEntry
	for _, c := range commits {
		author := c.AuthorEmail
		if a, _ := store.QueryAuthorByEmail(c.AuthorEmail); a != nil {
			author = a.Name
		}
		results = append(results, historyEntry{
			SHA:       c.SHA,
			Message:   c.Message,
			Author:    author,
			Timestamp: c.Timestamp,
		})
	}

	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// --- find_co_changed ---

func findCoChangedTool() mcp.Tool {
	return mcp.Tool{
		Name:        "find_co_changed",
		Description: "Find files that frequently change together with a given file. Identifies coupling between files.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"file":      map[string]string{"type": "string", "description": "File path"},
				"min_count": map[string]string{"type": "integer", "description": "Minimum co-change count to include (default 3)"},
			},
			Required: []string{"file"},
		},
	}
}

func (gs *GolemServer) handleFindCoChanged(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	file := getStr(args, "file")
	minCount := getInt(args, "min_count", 3)
	if minCount < 1 {
		minCount = 1
	}

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("opening graph: %v", err)), nil
	}
	defer store.Close()

	results, err := store.QueryCoChanged(file, minCount)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("querying co-changed files: %v", err)), nil
	}

	if len(results) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no co-changed files found for %q with min_count=%d", file, minCount)), nil
	}

	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// --- find_execution_failures ---

func findExecutionFailuresTool() mcp.Tool {
	return mcp.Tool{
		Name:        "find_execution_failures",
		Description: "Find commands that failed during agent execution. Returns error details, stack traces, and affected files.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"session": map[string]string{"type": "string", "description": "Session ID (default: latest session)"},
				"file":    map[string]string{"type": "string", "description": "Filter failures by file path"},
			},
		},
	}
}

func (gs *GolemServer) handleFindExecutionFailures(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	sessionID := getStr(args, "session")
	fileFilter := getStr(args, "file")

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("opening graph: %v", err)), nil
	}
	defer store.Close()

	result, err := query.RuntimePath(store, sessionID, "failures", "")
	if err != nil {
		return mcp.NewToolResultText(err.Error()), nil
	}

	fr, ok := result.(*query.FailureResult)
	if !ok || (len(fr.Failures) == 0 && len(fr.FailedTests) == 0) {
		return mcp.NewToolResultText(fmt.Sprintf("no failures found in session %q", sessionID)), nil
	}

	// Apply file filter if specified
	if fileFilter != "" {
		var filtered []query.FailureEntry
		for _, f := range fr.Failures {
			matched := false
			for _, file := range f.FilesInvolved {
				if strings.Contains(file, fileFilter) {
					matched = true
					break
				}
			}
			if matched {
				filtered = append(filtered, f)
			}
		}
		fr.Failures = filtered
		if len(fr.Failures) == 0 && len(fr.FailedTests) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf("no failures found in session %q matching %q", sessionID, fileFilter)), nil
		}
	}

	out, _ := json.MarshalIndent(fr, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// --- get_runtime_trace ---

func getRuntimeTraceTool() mcp.Tool {
	return mcp.Tool{
		Name:        "get_runtime_trace",
		Description: "Get a trace of commands executed during an agent session. Shows what happened and in what order.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"session":        map[string]string{"type": "string", "description": "Session ID (default: latest session)"},
				"command_filter": map[string]string{"type": "string", "description": "Filter commands by substring match"},
			},
		},
	}
}

func (gs *GolemServer) handleGetRuntimeTrace(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	sessionID := getStr(args, "session")
	cmdFilter := getStr(args, "command_filter")

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("opening graph: %v", err)), nil
	}
	defer store.Close()

	result, err := query.RuntimePath(store, sessionID, "trace", cmdFilter)
	if err != nil {
		return mcp.NewToolResultText(err.Error()), nil
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// --- find_test_results ---

func findTestResultsTool() mcp.Tool {
	return mcp.Tool{
		Name:        "find_test_results",
		Description: "Find test results from agent execution. Shows which tests passed/failed and what functions they exercise.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"session": map[string]string{"type": "string", "description": "Session ID (default: latest session)"},
				"status":  map[string]string{"type": "string", "description": "Filter by status: passed, failed, or all (default: all)"},
				"name":    map[string]string{"type": "string", "description": "Filter by test name substring"},
			},
		},
	}
}

func (gs *GolemServer) handleFindTestResults(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	sessionID := getStr(args, "session")
	status := getStr(args, "status")
	nameFilter := getStr(args, "name")

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("opening graph: %v", err)), nil
	}
	defer store.Close()

	if sessionID == "" {
		latest, err := store.LatestExecution()
		if err != nil || latest == nil {
			return mcp.NewToolResultText("no execution sessions found"), nil
		}
		sessionID = latest.SessionID
	}

	tests, err := store.QueryTestResults(sessionID, status)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("querying test results: %v", err)), nil
	}

	type exercisedFn struct {
		Function string `json:"function"`
		Path     string `json:"path"`
		Line     int    `json:"line,omitempty"`
	}

	type testEntry struct {
		Name       string        `json:"name"`
		Passed     bool          `json:"passed"`
		DurationMs int           `json:"durationMs,omitempty"`
		Output     string        `json:"output,omitempty"`
		Exercises  []exercisedFn `json:"exercises,omitempty"`
	}

	var results []testEntry
	for _, tr := range tests {
		if nameFilter != "" && !strings.Contains(tr.Name, nameFilter) {
			continue
		}

		entry := testEntry{
			Name:       tr.Name,
			Passed:     tr.Passed,
			DurationMs: tr.DurationMs,
			Output:     tr.Output,
		}

		// Find TESTS edges from this test result
		edges, _ := store.EdgesFrom(tr.ID)
		for _, e := range edges {
			if e.Type == "TESTS" {
				if n, _ := store.NodeByID(e.To); n != nil {
					entry.Exercises = append(entry.Exercises, exercisedFn{
						Function: n.Name,
						Path:     n.Path,
						Line:     n.Line,
					})
				}
			}
		}

		results = append(results, entry)
	}

	if len(results) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no test results found in session %q", sessionID)), nil
	}

	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// getInt extracts an int from MCP arguments with a default.
func getInt(args map[string]any, key string, defaultVal int) int {
	v, ok := args[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return defaultVal
	}
}
