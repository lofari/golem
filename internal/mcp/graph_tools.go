package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/lofari/golem/internal/graph"
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
	if depth < 1 {
		depth = 1
	}
	if depth > 5 {
		depth = 5
	}

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("opening graph: %v", err)), nil
	}
	defer store.Close()

	// Find target nodes by name
	targets, err := store.FindNodesByName(name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("querying graph: %v", err)), nil
	}
	if len(targets) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no nodes found matching %q", name)), nil
	}

	// BFS to find callers
	type caller struct {
		Node  graph.Node `json:"node"`
		Via   string     `json:"via"`
		Depth int        `json:"depth"`
	}

	var callers []caller
	visited := make(map[string]bool)

	// Seed with target node IDs
	current := make([]string, 0, len(targets))
	for _, t := range targets {
		current = append(current, t.ID)
		visited[t.ID] = true
	}

	for d := 1; d <= depth; d++ {
		var next []string
		for _, nodeID := range current {
			edges, _ := store.EdgesToOfType(nodeID, "CALLS")
			for _, e := range edges {
				if visited[e.From] {
					continue
				}
				visited[e.From] = true
				node, _ := store.NodeByID(e.From)
				if node != nil {
					callers = append(callers, caller{
						Node:  *node,
						Via:   fmt.Sprintf("CALLS:%s", nodeID),
						Depth: d,
					})
					next = append(next, e.From)
				}
			}
		}
		current = next
	}

	if len(callers) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no callers found for %q", name)), nil
	}

	out, _ := json.MarshalIndent(callers, "", "  ")
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

	// Try to find as file first, then by name
	targets, _ := store.NodesByPath(name)
	if len(targets) == 0 {
		targets, _ = store.FindNodesByName(name)
	}
	if len(targets) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no nodes found matching %q", name)), nil
	}

	type deps struct {
		Imports []string `json:"imports,omitempty"`
		Calls   []string `json:"calls,omitempty"`
		Uses    []string `json:"uses,omitempty"`
	}

	result := deps{}
	seen := make(map[string]bool)

	for _, t := range targets {
		edges, _ := store.EdgesFrom(t.ID)
		for _, e := range edges {
			key := e.Type + ":" + e.To
			if seen[key] {
				continue
			}
			seen[key] = true

			// Resolve target node name
			label := e.To
			if n, _ := store.NodeByID(e.To); n != nil {
				label = fmt.Sprintf("%s (%s:%d)", n.Name, n.Path, n.Line)
			}

			switch e.Type {
			case "IMPORTS":
				result.Imports = append(result.Imports, label)
			case "CALLS":
				result.Calls = append(result.Calls, label)
			case "USES":
				result.Uses = append(result.Uses, label)
			}
		}
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

	// Find target nodes
	targets, _ := store.NodesByPath(name)
	if len(targets) == 0 {
		targets, _ = store.FindNodesByName(name)
	}
	if len(targets) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no nodes found matching %q", name)), nil
	}

	type dependent struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Line int    `json:"line,omitempty"`
		Via  string `json:"via"`
	}

	var dependents []dependent
	seen := make(map[string]bool)

	for _, t := range targets {
		// Also find all symbols defined in this file
		searchIDs := []string{t.ID}
		if t.Type == "file" {
			defined, _ := store.EdgesOfType(t.ID, "DEFINES")
			for _, e := range defined {
				searchIDs = append(searchIDs, e.To)
			}
		}

		for _, id := range searchIDs {
			inEdges, _ := store.EdgesTo(id)
			for _, e := range inEdges {
				if e.Type == "DEFINES" {
					continue // skip DEFINES edges (same file)
				}
				if seen[e.From] {
					continue
				}
				seen[e.From] = true

				node, _ := store.NodeByID(e.From)
				if node != nil {
					dependents = append(dependents, dependent{
						Name: node.Name,
						Path: node.Path,
						Line: node.Line,
						Via:  fmt.Sprintf("%s:%s", e.Type, id),
					})
				}
			}
		}
	}

	if len(dependents) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no dependents found for %q", name)), nil
	}

	out, _ := json.MarshalIndent(dependents, "", "  ")
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
