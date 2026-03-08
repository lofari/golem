package treesitter

import (
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/lofari/golem/internal/graph"
)

// Extract walks a parsed tree and produces graph nodes and edges.
func Extract(filePath, lang string, tree *sitter.Tree, src []byte) ([]graph.Node, []graph.Edge) {
	var nodes []graph.Node
	var edges []graph.Edge

	// File node
	fileID := fmt.Sprintf("file:%s", filePath)
	nodes = append(nodes, graph.Node{
		ID:   fileID,
		Type: "file",
		Name: filePath,
		Path: filePath,
		Line: 1,
	})

	root := tree.RootNode()
	walkNode(root, filePath, fileID, lang, src, &nodes, &edges)

	return nodes, edges
}

// ExtractFileOnly creates a file-only node for unsupported languages.
func ExtractFileOnly(filePath string) ([]graph.Node, []graph.Edge) {
	return []graph.Node{{
		ID:   fmt.Sprintf("file:%s", filePath),
		Type: "file",
		Name: filePath,
		Path: filePath,
		Line: 1,
	}}, nil
}

func walkNode(node *sitter.Node, filePath, fileID, lang string, src []byte, nodes *[]graph.Node, edges *[]graph.Edge) {
	nodeType := node.Type()

	switch lang {
	case "go":
		extractGo(node, nodeType, filePath, fileID, src, nodes, edges)
	case "python":
		extractPython(node, nodeType, filePath, fileID, src, nodes, edges)
	case "javascript", "typescript":
		extractJS(node, nodeType, filePath, fileID, src, nodes, edges)
	}

	// Recurse into children
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child != nil {
			walkNode(child, filePath, fileID, lang, src, nodes, edges)
		}
	}
}

// --- Go ---

func extractGo(node *sitter.Node, nodeType, filePath, fileID string, src []byte, nodes *[]graph.Node, edges *[]graph.Edge) {
	switch nodeType {
	case "function_declaration":
		name := childContentByField(node, "name", src)
		if name != "" {
			id := fmt.Sprintf("fn:%s:%s", filePath, name)
			*nodes = append(*nodes, graph.Node{
				ID:   id,
				Type: "function",
				Name: name,
				Path: filePath,
				Line: int(node.StartPoint().Row) + 1,
			})
			*edges = append(*edges, graph.Edge{From: fileID, To: id, Type: "DEFINES"})
		}

	case "method_declaration":
		name := childContentByField(node, "name", src)
		if name != "" {
			id := fmt.Sprintf("method:%s:%s", filePath, name)
			*nodes = append(*nodes, graph.Node{
				ID:   id,
				Type: "method",
				Name: name,
				Path: filePath,
				Line: int(node.StartPoint().Row) + 1,
			})
			*edges = append(*edges, graph.Edge{From: fileID, To: id, Type: "DEFINES"})
		}

	case "type_spec":
		name := childContentByField(node, "name", src)
		if name != "" {
			id := fmt.Sprintf("type:%s:%s", filePath, name)
			*nodes = append(*nodes, graph.Node{
				ID:   id,
				Type: "type",
				Name: name,
				Path: filePath,
				Line: int(node.StartPoint().Row) + 1,
			})
			*edges = append(*edges, graph.Edge{From: fileID, To: id, Type: "DEFINES"})
		}

	case "import_spec":
		path := node.Content(src)
		path = strings.Trim(path, "\"")
		if path != "" {
			pkgID := fmt.Sprintf("pkg:%s", path)
			*edges = append(*edges, graph.Edge{From: fileID, To: pkgID, Type: "IMPORTS"})
		}

	case "call_expression":
		fnNode := node.ChildByFieldName("function")
		if fnNode != nil {
			callName := fnNode.Content(src)
			// Only track simple calls (not method chains beyond first level)
			if callName != "" && !strings.Contains(callName, "(") {
				callID := fmt.Sprintf("call:%s", callName)
				// Find enclosing function to link CALLS edge
				parent := findEnclosingFunc(node, filePath, src)
				if parent != "" {
					*edges = append(*edges, graph.Edge{From: parent, To: callID, Type: "CALLS"})
				}
			}
		}
	}
}

// --- Python ---

func extractPython(node *sitter.Node, nodeType, filePath, fileID string, src []byte, nodes *[]graph.Node, edges *[]graph.Edge) {
	switch nodeType {
	case "function_definition":
		name := childContentByField(node, "name", src)
		if name != "" {
			// Check if it's a method (inside a class)
			nType := "function"
			prefix := "fn"
			if isInsideClass(node) {
				nType = "method"
				prefix = "method"
			}
			id := fmt.Sprintf("%s:%s:%s", prefix, filePath, name)
			*nodes = append(*nodes, graph.Node{
				ID:   id,
				Type: nType,
				Name: name,
				Path: filePath,
				Line: int(node.StartPoint().Row) + 1,
			})
			*edges = append(*edges, graph.Edge{From: fileID, To: id, Type: "DEFINES"})
		}

	case "class_definition":
		name := childContentByField(node, "name", src)
		if name != "" {
			id := fmt.Sprintf("type:%s:%s", filePath, name)
			*nodes = append(*nodes, graph.Node{
				ID:   id,
				Type: "type",
				Name: name,
				Path: filePath,
				Line: int(node.StartPoint().Row) + 1,
			})
			*edges = append(*edges, graph.Edge{From: fileID, To: id, Type: "DEFINES"})
		}

	case "import_statement", "import_from_statement":
		content := node.Content(src)
		content = strings.TrimPrefix(content, "from ")
		content = strings.TrimPrefix(content, "import ")
		parts := strings.Fields(content)
		if len(parts) > 0 {
			mod := parts[0]
			pkgID := fmt.Sprintf("pkg:%s", mod)
			*edges = append(*edges, graph.Edge{From: fileID, To: pkgID, Type: "IMPORTS"})
		}
	}
}

// --- JavaScript / TypeScript ---

func extractJS(node *sitter.Node, nodeType, filePath, fileID string, src []byte, nodes *[]graph.Node, edges *[]graph.Edge) {
	switch nodeType {
	case "function_declaration":
		name := childContentByField(node, "name", src)
		if name != "" {
			id := fmt.Sprintf("fn:%s:%s", filePath, name)
			*nodes = append(*nodes, graph.Node{
				ID:   id,
				Type: "function",
				Name: name,
				Path: filePath,
				Line: int(node.StartPoint().Row) + 1,
			})
			*edges = append(*edges, graph.Edge{From: fileID, To: id, Type: "DEFINES"})
		}

	case "class_declaration":
		name := childContentByField(node, "name", src)
		if name != "" {
			id := fmt.Sprintf("type:%s:%s", filePath, name)
			*nodes = append(*nodes, graph.Node{
				ID:   id,
				Type: "type",
				Name: name,
				Path: filePath,
				Line: int(node.StartPoint().Row) + 1,
			})
			*edges = append(*edges, graph.Edge{From: fileID, To: id, Type: "DEFINES"})
		}

	case "method_definition":
		name := childContentByField(node, "name", src)
		if name != "" {
			id := fmt.Sprintf("method:%s:%s", filePath, name)
			*nodes = append(*nodes, graph.Node{
				ID:   id,
				Type: "method",
				Name: name,
				Path: filePath,
				Line: int(node.StartPoint().Row) + 1,
			})
			*edges = append(*edges, graph.Edge{From: fileID, To: id, Type: "DEFINES"})
		}

	case "import_statement":
		// Handle: import x from 'y' / import 'y'
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil && child.Type() == "string" {
				mod := strings.Trim(child.Content(src), "\"'`")
				if mod != "" {
					pkgID := fmt.Sprintf("pkg:%s", mod)
					*edges = append(*edges, graph.Edge{From: fileID, To: pkgID, Type: "IMPORTS"})
				}
			}
		}
	}
}

// --- Helpers ---

func childContentByField(node *sitter.Node, field string, src []byte) string {
	child := node.ChildByFieldName(field)
	if child == nil {
		return ""
	}
	return child.Content(src)
}

func findEnclosingFunc(node *sitter.Node, filePath string, src []byte) string {
	for p := node.Parent(); p != nil; p = p.Parent() {
		switch p.Type() {
		case "function_declaration":
			name := childContentByField(p, "name", src)
			if name != "" {
				return fmt.Sprintf("fn:%s:%s", filePath, name)
			}
		case "method_declaration", "method_definition":
			name := childContentByField(p, "name", src)
			if name != "" {
				return fmt.Sprintf("method:%s:%s", filePath, name)
			}
		case "function_definition": // python
			name := childContentByField(p, "name", src)
			if name != "" {
				if isInsideClass(p) {
					return fmt.Sprintf("method:%s:%s", filePath, name)
				}
				return fmt.Sprintf("fn:%s:%s", filePath, name)
			}
		}
	}
	return ""
}

func isInsideClass(node *sitter.Node) bool {
	for p := node.Parent(); p != nil; p = p.Parent() {
		if p.Type() == "class_definition" || p.Type() == "class_declaration" {
			return true
		}
	}
	return false
}
