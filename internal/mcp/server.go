package mcp

import (
	"context"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lofari/golem/internal/graph/lsp"
)

// GolemServer wraps an MCP server that exposes state update tools.
type GolemServer struct {
	mcpServer       *server.MCPServer
	dir             string
	lspManager      *lsp.Manager // nil if LSP disabled
	registeredTools []string     // tracks names of registered tools
}

// NewServer creates a new MCP server with all golem tools registered.
// lspManager may be nil if LSP is not available.
func NewServer(dir string, lspManager *lsp.Manager) *GolemServer {
	s := server.NewMCPServer("golem", "1.0.0",
		server.WithToolCapabilities(true),
	)

	gs := &GolemServer{mcpServer: s, dir: dir, lspManager: lspManager}
	gs.registerTools()
	return gs
}

// ListTools returns the names of all registered tools.
func (gs *GolemServer) ListTools() []string {
	return gs.registeredTools
}

// ServeStdio runs the MCP server over stdin/stdout.
func (gs *GolemServer) ServeStdio() error {
	return server.ServeStdio(gs.mcpServer)
}

func (gs *GolemServer) registerTools() {
	allowed := os.Getenv("GOLEM_TOOLS")
	var allowedSet map[string]bool
	if allowed != "" {
		allowedSet = make(map[string]bool)
		for _, name := range strings.Split(allowed, ",") {
			allowedSet[strings.TrimSpace(name)] = true
		}
	}

	register := func(name string, tool mcp.Tool, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
		if allowedSet == nil || allowedSet[name] {
			gs.mcpServer.AddTool(tool, handler)
			gs.registeredTools = append(gs.registeredTools, name)
		}
	}

	register("mark_task", markTaskTool(), gs.handleMarkTask)
	register("set_phase", setPhaseTool(), gs.handleSetPhase)
	register("set_status", setStatusTool(), gs.handleSetStatus)
	register("add_decision", addDecisionTool(), gs.handleAddDecision)
	register("add_pitfall", addPitfallTool(), gs.handleAddPitfall)
	register("log_session", logSessionTool(), gs.handleLogSession)
	register("find_callers", findCallersTool(), gs.handleFindCallers)
	register("find_dependencies", findDependenciesTool(), gs.handleFindDependencies)
	register("find_dependents", findDependentsTool(), gs.handleFindDependents)
	register("graph_query", graphQueryTool(), gs.handleGraphQuery)
	register("semantic_search", semanticSearchTool(), gs.handleSemanticSearch)
	register("find_co_changed", findCoChangedTool(), gs.handleFindCoChanged)
	register("find_execution_failures", findExecutionFailuresTool(), gs.handleFindExecutionFailures)
	register("get_runtime_trace", getRuntimeTraceTool(), gs.handleGetRuntimeTrace)
	register("find_test_results", findTestResultsTool(), gs.handleFindTestResults)

	// LSP tools (only if manager is available)
	if gs.lspManager != nil {
		register("lsp_definition", lspDefinitionTool(), gs.handleLSPDefinition)
		register("lsp_references", lspReferencesTool(), gs.handleLSPReferences)
		register("lsp_hover", lspHoverTool(), gs.handleLSPHover)
		register("lsp_diagnostics", lspDiagnosticsTool(), gs.handleLSPDiagnostics)
	}
}
