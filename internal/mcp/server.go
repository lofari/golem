package mcp

import (
	"github.com/mark3labs/mcp-go/server"

	"github.com/lofari/golem/internal/graph/lsp"
)

// GolemServer wraps an MCP server that exposes state update tools.
type GolemServer struct {
	mcpServer  *server.MCPServer
	dir        string
	lspManager *lsp.Manager // nil if LSP disabled
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
	tools := []string{"mark_task", "set_phase", "set_status", "add_decision", "add_pitfall", "add_locked", "log_session", "find_callers", "find_dependencies", "find_dependents", "graph_query", "semantic_search", "find_co_changed", "find_execution_failures", "get_runtime_trace", "find_test_results"}
	if gs.lspManager != nil {
		tools = append(tools, "lsp_definition", "lsp_references", "lsp_hover", "lsp_diagnostics")
	}
	return tools
}

// ServeStdio runs the MCP server over stdin/stdout.
func (gs *GolemServer) ServeStdio() error {
	return server.ServeStdio(gs.mcpServer)
}

func (gs *GolemServer) registerTools() {
	gs.mcpServer.AddTool(markTaskTool(), gs.handleMarkTask)
	gs.mcpServer.AddTool(setPhaseTool(), gs.handleSetPhase)
	gs.mcpServer.AddTool(setStatusTool(), gs.handleSetStatus)
	gs.mcpServer.AddTool(addDecisionTool(), gs.handleAddDecision)
	gs.mcpServer.AddTool(addPitfallTool(), gs.handleAddPitfall)
	gs.mcpServer.AddTool(addLockedTool(), gs.handleAddLocked)
	gs.mcpServer.AddTool(logSessionTool(), gs.handleLogSession)
	gs.mcpServer.AddTool(findCallersTool(), gs.handleFindCallers)
	gs.mcpServer.AddTool(findDependenciesTool(), gs.handleFindDependencies)
	gs.mcpServer.AddTool(findDependentsTool(), gs.handleFindDependents)
	gs.mcpServer.AddTool(graphQueryTool(), gs.handleGraphQuery)
	gs.mcpServer.AddTool(semanticSearchTool(), gs.handleSemanticSearch)
	gs.mcpServer.AddTool(findCoChangedTool(), gs.handleFindCoChanged)
	gs.mcpServer.AddTool(findExecutionFailuresTool(), gs.handleFindExecutionFailures)
	gs.mcpServer.AddTool(getRuntimeTraceTool(), gs.handleGetRuntimeTrace)
	gs.mcpServer.AddTool(findTestResultsTool(), gs.handleFindTestResults)

	// LSP tools (only if manager is available)
	if gs.lspManager != nil {
		gs.mcpServer.AddTool(lspDefinitionTool(), gs.handleLSPDefinition)
		gs.mcpServer.AddTool(lspReferencesTool(), gs.handleLSPReferences)
		gs.mcpServer.AddTool(lspHoverTool(), gs.handleLSPHover)
		gs.mcpServer.AddTool(lspDiagnosticsTool(), gs.handleLSPDiagnostics)
	}
}
