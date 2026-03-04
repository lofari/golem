package mcp

import (
	"github.com/mark3labs/mcp-go/server"
)

// GolemServer wraps an MCP server that exposes state update tools.
type GolemServer struct {
	mcpServer *server.MCPServer
	dir       string
}

// NewServer creates a new MCP server with all golem tools registered.
func NewServer(dir string) *GolemServer {
	s := server.NewMCPServer("golem", "1.0.0",
		server.WithToolCapabilities(true),
	)

	gs := &GolemServer{mcpServer: s, dir: dir}
	gs.registerTools()
	return gs
}

// ListTools returns the names of all registered tools.
func (gs *GolemServer) ListTools() []string {
	// Tool names are registered in registerTools
	return []string{"mark_task", "set_phase", "add_decision", "add_pitfall", "add_locked", "log_session"}
}

// ServeStdio runs the MCP server over stdin/stdout.
func (gs *GolemServer) ServeStdio() error {
	return server.ServeStdio(gs.mcpServer)
}

func (gs *GolemServer) registerTools() {
	gs.mcpServer.AddTool(markTaskTool(), gs.handleMarkTask)
	gs.mcpServer.AddTool(setPhaseTool(), gs.handleSetPhase)
	gs.mcpServer.AddTool(addDecisionTool(), gs.handleAddDecision)
	gs.mcpServer.AddTool(addPitfallTool(), gs.handleAddPitfall)
	gs.mcpServer.AddTool(addLockedTool(), gs.handleAddLocked)
	gs.mcpServer.AddTool(logSessionTool(), gs.handleLogSession)
}
