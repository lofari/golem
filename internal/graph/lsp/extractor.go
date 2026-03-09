package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.lsp.dev/protocol"

	"github.com/lofari/golem/internal/graph/model"
	"github.com/lofari/golem/internal/graph/treesitter"
)

// Extract uses the LSP client for symbol extraction and definition resolution.
// Tree-sitter is used for call-site detection. The filePath should be relative
// to the project root; absDir is the absolute project root.
func Extract(client *Client, absDir, relPath string) ([]model.Node, []model.Edge, error) {
	absPath := filepath.Join(absDir, relPath)
	var nodes []model.Node
	var edges []model.Edge

	// File node
	fileID := fmt.Sprintf("file:%s", relPath)
	nodes = append(nodes, model.Node{
		ID:   fileID,
		Type: "file",
		Name: relPath,
		Path: relPath,
		Line: 1,
	})

	// Step 1: Get symbols from LSP
	symbols, err := client.DocumentSymbols(absPath)
	if err != nil {
		return nodes, edges, fmt.Errorf("document symbols: %w", err)
	}

	for _, sym := range symbols {
		nodeType, prefix := symbolKindToNodeType(sym.Kind)
		if nodeType == "" {
			continue
		}

		id := fmt.Sprintf("%s:%s:%s", prefix, relPath, sym.Name)
		nodes = append(nodes, model.Node{
			ID:   id,
			Type: nodeType,
			Name: sym.Name,
			Path: relPath,
			Line: sym.Line + 1, // convert 0-indexed to 1-indexed
		})
		edges = append(edges, model.Edge{From: fileID, To: id, Type: "DEFINES"})
	}

	// Step 2: Use tree-sitter to find call sites
	src, err := os.ReadFile(absPath)
	if err != nil {
		return nodes, edges, nil // can't read — skip call extraction
	}

	lang := treesitter.DetectLanguage(relPath)
	if lang == "" {
		return nodes, edges, nil // unsupported for tree-sitter call detection
	}

	tree, _, err := treesitter.ParseBytes(src, lang)
	if err != nil {
		return nodes, edges, nil
	}

	callSites := treesitter.ExtractCallSites(relPath, lang, tree, src)

	// Step 3: Resolve each call site via LSP definition
	for _, cs := range callSites {
		locs, err := client.Definition(absPath, cs.Line, cs.Col)
		if err != nil || len(locs) == 0 {
			// Fallback: unresolved call edge
			edges = append(edges, model.Edge{
				From: cs.CallerID,
				To:   fmt.Sprintf("call:%s", cs.Name),
				Type: "CALLS",
			})
			continue
		}

		// Resolve to target node ID
		targetPath := uriToRelPath(locs[0].URI, absDir)
		targetLine := locs[0].Line + 1 // convert to 1-indexed

		targetID := resolveTargetID(nodes, targetPath, targetLine)
		if targetID == "" {
			// Target is in another file — construct best-guess ID
			targetID = fmt.Sprintf("fn:%s:%s", targetPath, cs.Name)
		}

		edges = append(edges, model.Edge{
			From: cs.CallerID,
			To:   targetID,
			Type: "CALLS",
		})
	}

	return nodes, edges, nil
}

func symbolKindToNodeType(kind protocol.SymbolKind) (nodeType, prefix string) {
	switch kind {
	case protocol.SymbolKindFunction:
		return "function", "fn"
	case protocol.SymbolKindMethod:
		return "method", "method"
	case protocol.SymbolKindClass:
		return "type", "type"
	case protocol.SymbolKindStruct:
		return "type", "type"
	case protocol.SymbolKindInterface:
		return "type", "type"
	case protocol.SymbolKindEnum:
		return "type", "type"
	default:
		return "", ""
	}
}

func uriToRelPath(fileURI string, absDir string) string {
	// Strip file:// prefix
	path := strings.TrimPrefix(fileURI, "file://")
	rel, err := filepath.Rel(absDir, path)
	if err != nil {
		return path
	}
	return rel
}

func resolveTargetID(nodes []model.Node, targetPath string, targetLine int) string {
	for _, n := range nodes {
		if n.Path == targetPath && n.Line == targetLine {
			return n.ID
		}
	}
	return ""
}
