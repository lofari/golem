package embed

import (
	"fmt"
	"strings"

	"github.com/lofari/golem/internal/graph/model"
)

// NodeText creates a text representation of a graph node suitable for embedding.
// src is an optional source snippet (e.g., function signature). Pass "" if unavailable.
func NodeText(node model.Node, src string) string {
	src = strings.TrimSpace(src)
	switch node.Type {
	case "function":
		if src != "" {
			return fmt.Sprintf("Function %s in %s: %s", node.Name, node.Path, src)
		}
		return fmt.Sprintf("Function %s in %s", node.Name, node.Path)
	case "method":
		if src != "" {
			return fmt.Sprintf("Method %s in %s: %s", node.Name, node.Path, src)
		}
		return fmt.Sprintf("Method %s in %s", node.Name, node.Path)
	case "type":
		if src != "" {
			return fmt.Sprintf("Type %s in %s: %s", node.Name, node.Path, src)
		}
		return fmt.Sprintf("Type %s in %s", node.Name, node.Path)
	case "file":
		return fmt.Sprintf("File %s", node.Path)
	case "document":
		return fmt.Sprintf("Document %s", node.Path)
	case "section":
		if src != "" {
			return fmt.Sprintf("Section %s in %s: %s", node.Name, node.Path, src)
		}
		return fmt.Sprintf("Section %s in %s", node.Name, node.Path)
	default:
		return fmt.Sprintf("%s %s in %s", node.Type, node.Name, node.Path)
	}
}
