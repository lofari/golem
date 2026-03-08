package treesitter

import (
	"testing"

	"github.com/lofari/golem/internal/graph"
)

func TestExtractGo(t *testing.T) {
	src := []byte(`package main

import "fmt"

type Server struct{}

func (s *Server) Start() {
	fmt.Println("starting")
}

func main() {
	s := &Server{}
	s.Start()
}
`)
	tree, lang, err := ParseBytes(src, "go")
	if err != nil {
		t.Fatal(err)
	}

	nodes, edges := Extract("main.go", lang, tree, src)

	// Should have: file node + function nodes + type node
	nodeMap := make(map[string]graph.Node)
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}

	// File node
	if _, ok := nodeMap["file:main.go"]; !ok {
		t.Error("missing file node")
	}

	// Function: main
	if n, ok := nodeMap["fn:main.go:main"]; !ok {
		t.Error("missing function node for main")
	} else if n.Type != "function" {
		t.Errorf("main should be function, got %q", n.Type)
	}

	// Method: Start
	if _, ok := nodeMap["method:main.go:Start"]; !ok {
		t.Error("missing method node for Start")
	}

	// Type: Server
	if _, ok := nodeMap["type:main.go:Server"]; !ok {
		t.Error("missing type node for Server")
	}

	// Should have DEFINES edges
	hasDefines := false
	for _, e := range edges {
		if e.Type == "DEFINES" && e.From == "file:main.go" {
			hasDefines = true
			break
		}
	}
	if !hasDefines {
		t.Error("missing DEFINES edge from file")
	}

	// Should have IMPORTS edge
	hasImport := false
	for _, e := range edges {
		if e.Type == "IMPORTS" {
			hasImport = true
			break
		}
	}
	if !hasImport {
		t.Error("missing IMPORTS edge")
	}
}

func TestExtractPython(t *testing.T) {
	src := []byte(`import os

class MyClass:
    def method(self):
        pass

def hello():
    os.path.exists("/tmp")
`)
	tree, lang, err := ParseBytes(src, "python")
	if err != nil {
		t.Fatal(err)
	}

	nodes, edges := Extract("app.py", lang, tree, src)

	nodeMap := make(map[string]graph.Node)
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}

	if _, ok := nodeMap["file:app.py"]; !ok {
		t.Error("missing file node")
	}
	if _, ok := nodeMap["fn:app.py:hello"]; !ok {
		t.Error("missing function node for hello")
	}
	if _, ok := nodeMap["type:app.py:MyClass"]; !ok {
		t.Error("missing class node for MyClass")
	}

	_ = edges // edges structure verified by Go test above
}

func TestExtractUnsupportedReturnsFileOnly(t *testing.T) {
	// For unsupported languages, we can't parse but should still make a file node
	nodes, edges := ExtractFileOnly("README.md")
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Type != "file" {
		t.Fatalf("expected file node, got %q", nodes[0].Type)
	}
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges, got %d", len(edges))
	}
}
