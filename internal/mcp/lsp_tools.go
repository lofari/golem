package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/lofari/golem/internal/graph/lsp"
)

// --- lsp_definition ---

func lspDefinitionTool() mcp.Tool {
	return mcp.Tool{
		Name:        "lsp_definition",
		Description: "Jump to where a symbol is defined. Provide file path, line, and column of the symbol reference.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"file":   map[string]string{"type": "string", "description": "File path (relative to project root)"},
				"line":   map[string]string{"type": "integer", "description": "Line number (1-indexed)"},
				"column": map[string]string{"type": "integer", "description": "Column number (1-indexed)"},
			},
			Required: []string{"file", "line", "column"},
		},
	}
}

func (gs *GolemServer) handleLSPDefinition(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if gs.lspManager == nil {
		return mcp.NewToolResultError("LSP not available — start golem with LSP enabled"), nil
	}

	args := req.GetArguments()
	file := getStr(args, "file")
	line := getInt(args, "line", 0)
	col := getInt(args, "column", 0)

	client := gs.clientForFile(file)
	if client == nil {
		cfg := lsp.ConfigForExt(filepath.Ext(file))
		hint := ""
		if cfg != nil {
			hint = fmt.Sprintf(" Install with: %s", cfg.InstallHint)
		}
		return mcp.NewToolResultError(fmt.Sprintf("No LSP server for %s.%s", file, hint)), nil
	}

	absPath := filepath.Join(gs.dir, file)
	locs, err := client.Definition(absPath, line-1, col-1) // convert to 0-indexed
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("definition: %v", err)), nil
	}

	if len(locs) == 0 {
		return mcp.NewToolResultText("no definition found"), nil
	}

	type defResult struct {
		File   string `json:"file"`
		Line   int    `json:"line"`
		Column int    `json:"column"`
	}

	var results []defResult
	for _, l := range locs {
		relPath := uriToRel(l.URI, gs.dir)
		results = append(results, defResult{
			File:   relPath,
			Line:   l.Line + 1,
			Column: l.Col + 1,
		})
	}

	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// --- lsp_references ---

func lspReferencesTool() mcp.Tool {
	return mcp.Tool{
		Name:        "lsp_references",
		Description: "Find all usages of a symbol. Returns every location where the symbol at the given position is referenced.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"file":   map[string]string{"type": "string", "description": "File path (relative to project root)"},
				"line":   map[string]string{"type": "integer", "description": "Line number (1-indexed)"},
				"column": map[string]string{"type": "integer", "description": "Column number (1-indexed)"},
			},
			Required: []string{"file", "line", "column"},
		},
	}
}

func (gs *GolemServer) handleLSPReferences(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if gs.lspManager == nil {
		return mcp.NewToolResultError("LSP not available"), nil
	}

	args := req.GetArguments()
	file := getStr(args, "file")
	line := getInt(args, "line", 0)
	col := getInt(args, "column", 0)

	client := gs.clientForFile(file)
	if client == nil {
		return mcp.NewToolResultError(fmt.Sprintf("No LSP server for %s", file)), nil
	}

	absPath := filepath.Join(gs.dir, file)
	locs, err := client.References(absPath, line-1, col-1)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("references: %v", err)), nil
	}

	if len(locs) == 0 {
		return mcp.NewToolResultText("no references found"), nil
	}

	type refResult struct {
		File   string `json:"file"`
		Line   int    `json:"line"`
		Column int    `json:"column"`
	}

	var results []refResult
	for _, l := range locs {
		results = append(results, refResult{
			File:   uriToRel(l.URI, gs.dir),
			Line:   l.Line + 1,
			Column: l.Col + 1,
		})
	}

	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// --- lsp_hover ---

func lspHoverTool() mcp.Tool {
	return mcp.Tool{
		Name:        "lsp_hover",
		Description: "Get type information, signature, and documentation for a symbol at a position.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"file":   map[string]string{"type": "string", "description": "File path (relative to project root)"},
				"line":   map[string]string{"type": "integer", "description": "Line number (1-indexed)"},
				"column": map[string]string{"type": "integer", "description": "Column number (1-indexed)"},
			},
			Required: []string{"file", "line", "column"},
		},
	}
}

func (gs *GolemServer) handleLSPHover(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if gs.lspManager == nil {
		return mcp.NewToolResultError("LSP not available"), nil
	}

	args := req.GetArguments()
	file := getStr(args, "file")
	line := getInt(args, "line", 0)
	col := getInt(args, "column", 0)

	client := gs.clientForFile(file)
	if client == nil {
		return mcp.NewToolResultError(fmt.Sprintf("No LSP server for %s", file)), nil
	}

	absPath := filepath.Join(gs.dir, file)
	result, err := client.Hover(absPath, line-1, col-1)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("hover: %v", err)), nil
	}

	if result == nil || result.Contents == "" {
		return mcp.NewToolResultText("no hover information"), nil
	}

	return mcp.NewToolResultText(result.Contents), nil
}

// --- lsp_diagnostics ---

func lspDiagnosticsTool() mcp.Tool {
	return mcp.Tool{
		Name:        "lsp_diagnostics",
		Description: "Get type errors, lint warnings, and other diagnostics for a file.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"file": map[string]string{"type": "string", "description": "File path (relative to project root)"},
			},
			Required: []string{"file"},
		},
	}
}

func (gs *GolemServer) handleLSPDiagnostics(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if gs.lspManager == nil {
		return mcp.NewToolResultError("LSP not available"), nil
	}

	args := req.GetArguments()
	file := getStr(args, "file")

	client := gs.clientForFile(file)
	if client == nil {
		return mcp.NewToolResultError(fmt.Sprintf("No LSP server for %s", file)), nil
	}

	absPath := filepath.Join(gs.dir, file)
	diags, err := client.Diagnostics(absPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("diagnostics: %v", err)), nil
	}

	if len(diags) == 0 {
		return mcp.NewToolResultText("no diagnostics"), nil
	}

	out, _ := json.MarshalIndent(diags, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// helpers

func (gs *GolemServer) clientForFile(relPath string) *lsp.Client {
	if gs.lspManager == nil {
		return nil
	}
	cfg := lsp.ConfigForExt(filepath.Ext(relPath))
	if cfg == nil {
		return nil
	}
	return gs.lspManager.ClientFor(cfg.Language)
}

func uriToRel(fileURI, baseDir string) string {
	path := strings.TrimPrefix(fileURI, "file://")
	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return path
	}
	return rel
}
