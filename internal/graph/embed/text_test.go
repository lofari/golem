package embed

import (
	"testing"

	"github.com/lofari/golem/internal/graph/model"
)

func TestNodeText(t *testing.T) {
	tests := []struct {
		name     string
		node     model.Node
		src      string
		expected string
	}{
		{
			name:     "function with source",
			node:     model.Node{ID: "fn:main.go:StartServer", Type: "function", Name: "StartServer", Path: "main.go", Line: 10},
			src:      "func StartServer(cfg Config) error {",
			expected: "Function StartServer in main.go: func StartServer(cfg Config) error {",
		},
		{
			name:     "function without source",
			node:     model.Node{ID: "fn:main.go:StartServer", Type: "function", Name: "StartServer", Path: "main.go", Line: 10},
			src:      "",
			expected: "Function StartServer in main.go",
		},
		{
			name:     "method",
			node:     model.Node{ID: "method:store.go:InsertBatch", Type: "method", Name: "InsertBatch", Path: "store.go", Line: 71},
			src:      "func (s *Store) InsertBatch(nodes []Node, edges []Edge) error {",
			expected: "Method InsertBatch in store.go: func (s *Store) InsertBatch(nodes []Node, edges []Edge) error {",
		},
		{
			name:     "type",
			node:     model.Node{ID: "type:config.go:Config", Type: "type", Name: "Config", Path: "config.go", Line: 5},
			src:      "",
			expected: "Type Config in config.go",
		},
		{
			name:     "file",
			node:     model.Node{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go", Line: 1},
			src:      "",
			expected: "File main.go",
		},
		{
			name:     "document",
			node:     model.Node{ID: "doc:README.md", Type: "document", Name: "README.md", Path: "README.md", Line: 1},
			src:      "",
			expected: "Document README.md",
		},
		{
			name:     "section",
			node:     model.Node{ID: "sec:README.md:Usage", Type: "section", Name: "Usage", Path: "README.md", Line: 5},
			src:      "",
			expected: "Section Usage in README.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NodeText(tt.node, tt.src)
			if got != tt.expected {
				t.Errorf("NodeText() = %q, want %q", got, tt.expected)
			}
		})
	}
}
