package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestLSPToolDefinitions(t *testing.T) {
	tools := []struct {
		name string
		fn   func() mcp.Tool
	}{
		{"lsp_definition", lspDefinitionTool},
		{"lsp_references", lspReferencesTool},
		{"lsp_hover", lspHoverTool},
		{"lsp_diagnostics", lspDiagnosticsTool},
	}

	for _, tt := range tools {
		tool := tt.fn()
		if tool.Name != tt.name {
			t.Errorf("expected name %s, got %s", tt.name, tool.Name)
		}
		if tool.InputSchema.Type != "object" {
			t.Errorf("%s: expected object schema", tt.name)
		}
		if _, ok := tool.InputSchema.Properties["file"]; !ok {
			t.Errorf("%s: missing 'file' parameter", tt.name)
		}
	}
}
