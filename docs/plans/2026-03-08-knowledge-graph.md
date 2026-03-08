# Knowledge Graph Phase 1 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a structural code graph to Golem that agents query via MCP tools to understand code structure, dependencies, and call relationships.

**Architecture:** Tree-sitter parses source files into AST nodes/edges stored in SQLite at `.ctx/graph.db`. Agents query the graph via four new MCP tools. The graph is built explicitly with `golem graph build` and auto-synced at session start via git diff.

**Tech Stack:** Go, `github.com/smacker/go-tree-sitter` (AST parsing), `github.com/mattn/go-sqlite3` (storage), tree-sitter language grammars

**Design doc:** `docs/plans/2026-03-08-knowledge-graph-design.md`

---

### Task 0: Add dependencies

**Files:**
- Modify: `go.mod`

**Step 1: Add go-tree-sitter and go-sqlite3**

```bash
CGO_ENABLED=1 go get github.com/smacker/go-tree-sitter
CGO_ENABLED=1 go get github.com/smacker/go-tree-sitter/golang
CGO_ENABLED=1 go get github.com/smacker/go-tree-sitter/python
CGO_ENABLED=1 go get github.com/smacker/go-tree-sitter/javascript
CGO_ENABLED=1 go get github.com/smacker/go-tree-sitter/typescript/typescript
CGO_ENABLED=1 go get github.com/mattn/go-sqlite3
```

**Step 2: Verify build**

```bash
CGO_ENABLED=1 go build ./...
```

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "feat(graph): add tree-sitter and sqlite3 dependencies"
```

---

### Task 1: SQLite store — schema and basic operations

Create the graph store with schema creation, node/edge insertion, and basic queries.

**Files:**
- Create: `internal/graph/store.go`
- Create: `internal/graph/store_test.go`

**Step 1: Write the store tests**

Create `internal/graph/store_test.go`:

```go
package graph

import (
	"os"
	"path/filepath"
	"testing"
)

func tempDB(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	s, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenStore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	s, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Verify file was created
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database file not created: %v", err)
	}
}

func TestInsertAndQueryNodes(t *testing.T) {
	s := tempDB(t)

	nodes := []Node{
		{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go"},
		{ID: "fn:main.go:main", Type: "function", Name: "main", Path: "main.go", Line: 10},
		{ID: "fn:main.go:helper", Type: "function", Name: "helper", Path: "main.go", Line: 25},
	}
	edges := []Edge{
		{From: "file:main.go", To: "fn:main.go:main", Type: "DEFINES"},
		{From: "file:main.go", To: "fn:main.go:helper", Type: "DEFINES"},
		{From: "fn:main.go:main", To: "fn:main.go:helper", Type: "CALLS"},
	}

	if err := s.InsertBatch(nodes, edges); err != nil {
		t.Fatal(err)
	}

	// Query nodes by type
	fns, err := s.NodesByType("function")
	if err != nil {
		t.Fatal(err)
	}
	if len(fns) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(fns))
	}

	// Query edges from a node
	outEdges, err := s.EdgesFrom("fn:main.go:main")
	if err != nil {
		t.Fatal(err)
	}
	if len(outEdges) != 1 || outEdges[0].Type != "CALLS" {
		t.Fatalf("expected 1 CALLS edge, got %v", outEdges)
	}

	// Query edges to a node (reverse)
	inEdges, err := s.EdgesTo("fn:main.go:helper")
	if err != nil {
		t.Fatal(err)
	}
	if len(inEdges) != 2 { // DEFINES + CALLS
		t.Fatalf("expected 2 inbound edges, got %d", len(inEdges))
	}
}

func TestDeleteByPath(t *testing.T) {
	s := tempDB(t)

	nodes := []Node{
		{ID: "file:a.go", Type: "file", Name: "a.go", Path: "a.go"},
		{ID: "fn:a.go:Foo", Type: "function", Name: "Foo", Path: "a.go", Line: 1},
		{ID: "file:b.go", Type: "file", Name: "b.go", Path: "b.go"},
		{ID: "fn:b.go:Bar", Type: "function", Name: "Bar", Path: "b.go", Line: 1},
	}
	edges := []Edge{
		{From: "file:a.go", To: "fn:a.go:Foo", Type: "DEFINES"},
		{From: "fn:a.go:Foo", To: "fn:b.go:Bar", Type: "CALLS"},
	}
	s.InsertBatch(nodes, edges)

	// Delete nodes for a.go
	if err := s.DeleteByPath("a.go"); err != nil {
		t.Fatal(err)
	}

	// a.go nodes should be gone
	fns, _ := s.NodesByPath("a.go")
	if len(fns) != 0 {
		t.Fatalf("expected 0 nodes for a.go, got %d", len(fns))
	}

	// b.go nodes should remain
	fns, _ = s.NodesByPath("b.go")
	if len(fns) != 2 {
		t.Fatalf("expected 2 nodes for b.go, got %d", len(fns))
	}

	// Dangling edges from deleted nodes should be gone
	edges2, _ := s.EdgesFrom("fn:a.go:Foo")
	if len(edges2) != 0 {
		t.Fatalf("expected 0 edges from deleted node, got %d", len(edges2))
	}
}

func TestSetAndGetMeta(t *testing.T) {
	s := tempDB(t)

	if err := s.SetMeta("last_commit", "abc123"); err != nil {
		t.Fatal(err)
	}
	val, err := s.GetMeta("last_commit")
	if err != nil {
		t.Fatal(err)
	}
	if val != "abc123" {
		t.Fatalf("expected abc123, got %q", val)
	}

	// Update
	s.SetMeta("last_commit", "def456")
	val, _ = s.GetMeta("last_commit")
	if val != "def456" {
		t.Fatalf("expected def456, got %q", val)
	}

	// Missing key
	val, _ = s.GetMeta("nonexistent")
	if val != "" {
		t.Fatalf("expected empty string, got %q", val)
	}
}

func TestStats(t *testing.T) {
	s := tempDB(t)

	nodes := []Node{
		{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go"},
		{ID: "fn:main.go:main", Type: "function", Name: "main", Path: "main.go", Line: 10},
	}
	edges := []Edge{
		{From: "file:main.go", To: "fn:main.go:main", Type: "DEFINES"},
	}
	s.InsertBatch(nodes, edges)

	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalNodes != 2 {
		t.Fatalf("expected 2 nodes, got %d", stats.TotalNodes)
	}
	if stats.TotalEdges != 1 {
		t.Fatalf("expected 1 edge, got %d", stats.TotalEdges)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
CGO_ENABLED=1 go test ./internal/graph/ -v -count=1
```

Expected: compilation error (types not defined yet).

**Step 3: Implement the store**

Create `internal/graph/store.go`:

```go
package graph

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// Node represents a code entity in the graph.
type Node struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

// Edge represents a relationship between two nodes.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// Stats holds graph statistics.
type Stats struct {
	TotalNodes int            `json:"totalNodes"`
	TotalEdges int            `json:"totalEdges"`
	NodeTypes  map[string]int `json:"nodeTypes"`
	EdgeTypes  map[string]int `json:"edgeTypes"`
}

// Store is the SQLite-backed graph storage.
type Store struct {
	db *sql.DB
}

// OpenStore opens or creates a graph database at the given path.
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening graph db: %w", err)
	}
	s := &Store{db: db}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) init() error {
	schema := `
		CREATE TABLE IF NOT EXISTS nodes (
			id   TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			name TEXT NOT NULL,
			path TEXT,
			line INTEGER
		);
		CREATE TABLE IF NOT EXISTS edges (
			from_node TEXT NOT NULL,
			to_node   TEXT NOT NULL,
			type      TEXT NOT NULL,
			PRIMARY KEY (from_node, to_node, type)
		);
		CREATE TABLE IF NOT EXISTS graph_meta (
			key   TEXT PRIMARY KEY,
			value TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_edges_from ON edges(from_node);
		CREATE INDEX IF NOT EXISTS idx_edges_to ON edges(to_node);
		CREATE INDEX IF NOT EXISTS idx_edges_type ON edges(type);
		CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes(type);
		CREATE INDEX IF NOT EXISTS idx_nodes_path ON nodes(path);
	`
	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// InsertBatch inserts nodes and edges in a single transaction.
func (s *Store) InsertBatch(nodes []Node, edges []Edge) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	nodeStmt, err := tx.Prepare("INSERT OR REPLACE INTO nodes (id, type, name, path, line) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer nodeStmt.Close()

	for _, n := range nodes {
		if _, err := nodeStmt.Exec(n.ID, n.Type, n.Name, n.Path, n.Line); err != nil {
			return fmt.Errorf("inserting node %s: %w", n.ID, err)
		}
	}

	edgeStmt, err := tx.Prepare("INSERT OR REPLACE INTO edges (from_node, to_node, type) VALUES (?, ?, ?)")
	if err != nil {
		return err
	}
	defer edgeStmt.Close()

	for _, e := range edges {
		if _, err := edgeStmt.Exec(e.From, e.To, e.Type); err != nil {
			return fmt.Errorf("inserting edge %s->%s: %w", e.From, e.To, err)
		}
	}

	return tx.Commit()
}

// DeleteByPath removes all nodes with the given path and their edges.
func (s *Store) DeleteByPath(path string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete edges where either end is a node from this path
	_, err = tx.Exec(`
		DELETE FROM edges WHERE from_node IN (SELECT id FROM nodes WHERE path = ?)
		OR to_node IN (SELECT id FROM nodes WHERE path = ?)
	`, path, path)
	if err != nil {
		return err
	}

	// Delete nodes
	_, err = tx.Exec("DELETE FROM nodes WHERE path = ?", path)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// Clear removes all nodes and edges.
func (s *Store) Clear() error {
	_, err := s.db.Exec("DELETE FROM nodes; DELETE FROM edges;")
	return err
}

// NodesByType returns all nodes of the given type.
func (s *Store) NodesByType(nodeType string) ([]Node, error) {
	return s.queryNodes("SELECT id, type, name, path, line FROM nodes WHERE type = ?", nodeType)
}

// NodesByPath returns all nodes with the given path.
func (s *Store) NodesByPath(path string) ([]Node, error) {
	return s.queryNodes("SELECT id, type, name, path, line FROM nodes WHERE path = ?", path)
}

// NodeByID returns a single node by ID.
func (s *Store) NodeByID(id string) (*Node, error) {
	nodes, err := s.queryNodes("SELECT id, type, name, path, line FROM nodes WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, nil
	}
	return &nodes[0], nil
}

// FindNodesByName returns nodes matching the given name (exact match).
func (s *Store) FindNodesByName(name string) ([]Node, error) {
	return s.queryNodes("SELECT id, type, name, path, line FROM nodes WHERE name = ?", name)
}

// EdgesFrom returns all edges originating from the given node.
func (s *Store) EdgesFrom(nodeID string) ([]Edge, error) {
	return s.queryEdges("SELECT from_node, to_node, type FROM edges WHERE from_node = ?", nodeID)
}

// EdgesTo returns all edges pointing to the given node.
func (s *Store) EdgesTo(nodeID string) ([]Edge, error) {
	return s.queryEdges("SELECT from_node, to_node, type FROM edges WHERE to_node = ?", nodeID)
}

// EdgesOfType returns all edges of the given type from a node.
func (s *Store) EdgesOfType(nodeID string, edgeType string) ([]Edge, error) {
	return s.queryEdges("SELECT from_node, to_node, type FROM edges WHERE from_node = ? AND type = ?", nodeID, edgeType)
}

// EdgesToOfType returns all edges of the given type pointing to a node.
func (s *Store) EdgesToOfType(nodeID string, edgeType string) ([]Edge, error) {
	return s.queryEdges("SELECT from_node, to_node, type FROM edges WHERE to_node = ? AND type = ?", nodeID, edgeType)
}

// SetMeta sets a key-value pair in graph_meta.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO graph_meta (key, value) VALUES (?, ?)", key, value)
	return err
}

// GetMeta retrieves a value from graph_meta.
func (s *Store) GetMeta(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM graph_meta WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// Stats returns graph statistics.
func (s *Store) Stats() (Stats, error) {
	var stats Stats
	stats.NodeTypes = make(map[string]int)
	stats.EdgeTypes = make(map[string]int)

	s.db.QueryRow("SELECT COUNT(*) FROM nodes").Scan(&stats.TotalNodes)
	s.db.QueryRow("SELECT COUNT(*) FROM edges").Scan(&stats.TotalEdges)

	rows, err := s.db.Query("SELECT type, COUNT(*) FROM nodes GROUP BY type")
	if err != nil {
		return stats, err
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		var c int
		rows.Scan(&t, &c)
		stats.NodeTypes[t] = c
	}

	rows, err = s.db.Query("SELECT type, COUNT(*) FROM edges GROUP BY type")
	if err != nil {
		return stats, err
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		var c int
		rows.Scan(&t, &c)
		stats.EdgeTypes[t] = c
	}

	return stats, nil
}

func (s *Store) queryNodes(query string, args ...any) ([]Node, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Type, &n.Name, &n.Path, &n.Line); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func (s *Store) queryEdges(query string, args ...any) ([]Edge, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.From, &e.To, &e.Type); err != nil {
			return nil, err
		}
		edges = append(edges, e)
	}
	return edges, nil
}
```

**Step 4: Run tests**

```bash
CGO_ENABLED=1 go test ./internal/graph/ -v -count=1
```

Expected: all pass.

**Step 5: Commit**

```bash
git add internal/graph/store.go internal/graph/store_test.go
git commit -m "feat(graph): add SQLite graph store with schema, CRUD, and tests"
```

---

### Task 2: Tree-sitter parser and language detection

Create the parser that detects languages from file extensions and parses files into ASTs.

**Files:**
- Create: `internal/graph/treesitter/parser.go`
- Create: `internal/graph/treesitter/parser_test.go`

**Step 1: Write the parser tests**

Create `internal/graph/treesitter/parser_test.go`:

```go
package treesitter

import (
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"server.py", "python"},
		{"app.ts", "typescript"},
		{"index.js", "javascript"},
		{"unknown.xyz", ""},
		{"Makefile", ""},
	}
	for _, tt := range tests {
		got := DetectLanguage(tt.path)
		if got != tt.want {
			t.Errorf("DetectLanguage(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestParseGo(t *testing.T) {
	src := []byte(`package main

import "fmt"

func Hello() {
	fmt.Println("hello")
}

func main() {
	Hello()
}
`)
	tree, lang, err := ParseBytes(src, "go")
	if err != nil {
		t.Fatal(err)
	}
	if tree == nil {
		t.Fatal("expected non-nil tree")
	}
	if lang != "go" {
		t.Fatalf("expected lang 'go', got %q", lang)
	}
	root := tree.RootNode()
	if root.Type() != "source_file" {
		t.Fatalf("expected root type 'source_file', got %q", root.Type())
	}
}

func TestParseUnsupportedLanguage(t *testing.T) {
	_, _, err := ParseBytes([]byte("hello"), "unknown")
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
CGO_ENABLED=1 go test ./internal/graph/treesitter/ -v -count=1
```

Expected: compilation error.

**Step 3: Implement the parser**

Create `internal/graph/treesitter/parser.go`:

```go
package treesitter

import (
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	tsTypescript "github.com/smacker/go-tree-sitter/typescript/typescript"
)

// Supported language extensions.
var extToLang = map[string]string{
	".go":   "go",
	".py":   "python",
	".js":   "javascript",
	".jsx":  "javascript",
	".ts":   "typescript",
	".tsx":  "typescript",
	".mjs":  "javascript",
	".cjs":  "javascript",
}

// langToSitter maps language names to tree-sitter language objects.
var langToSitter = map[string]*sitter.Language{
	"go":         golang.GetLanguage(),
	"python":     python.GetLanguage(),
	"javascript": javascript.GetLanguage(),
	"typescript": tsTypescript.GetLanguage(),
}

// DetectLanguage returns the language name for a file path, or "" if unsupported.
func DetectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	return extToLang[ext]
}

// Supported returns true if the given language is supported.
func Supported(lang string) bool {
	_, ok := langToSitter[lang]
	return ok
}

// ParseBytes parses source code bytes for the given language.
// Returns the parsed tree and the language name.
func ParseBytes(src []byte, lang string) (*sitter.Tree, string, error) {
	tsLang, ok := langToSitter[lang]
	if !ok {
		return nil, "", fmt.Errorf("unsupported language: %q", lang)
	}

	parser := sitter.NewParser()
	parser.SetLanguage(tsLang)
	tree, err := parser.ParseCtx(nil, nil, src)
	if err != nil {
		return nil, "", fmt.Errorf("parsing %s: %w", lang, err)
	}
	return tree, lang, nil
}

// ParseFile reads and parses a file, auto-detecting the language.
// Returns nil tree if the language is unsupported (not an error).
func ParseFile(path string, src []byte) (*sitter.Tree, string, error) {
	lang := DetectLanguage(path)
	if lang == "" {
		return nil, "", nil
	}
	tree, _, err := ParseBytes(src, lang)
	return tree, lang, err
}
```

**Step 4: Run tests**

```bash
CGO_ENABLED=1 go test ./internal/graph/treesitter/ -v -count=1
```

Expected: all pass.

**Step 5: Commit**

```bash
git add internal/graph/treesitter/parser.go internal/graph/treesitter/parser_test.go
git commit -m "feat(graph): add tree-sitter parser with language detection"
```

---

### Task 3: AST extractor — extract nodes and edges from parsed trees

Walk tree-sitter ASTs to produce graph nodes and edges. Language-agnostic framework with per-language node type mappings.

**Files:**
- Create: `internal/graph/treesitter/extractor.go`
- Create: `internal/graph/treesitter/extractor_test.go`

**Step 1: Write extractor tests**

Create `internal/graph/treesitter/extractor_test.go`:

```go
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
```

**Step 2: Run tests to verify they fail**

```bash
CGO_ENABLED=1 go test ./internal/graph/treesitter/ -v -count=1
```

Expected: compilation error.

**Step 3: Implement the extractor**

Create `internal/graph/treesitter/extractor.go`:

```go
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
		content := node.Content(src)
		// Extract the string literal
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
```

**Step 4: Run tests**

```bash
CGO_ENABLED=1 go test ./internal/graph/treesitter/ -v -count=1
```

Expected: all pass.

**Step 5: Commit**

```bash
git add internal/graph/treesitter/extractor.go internal/graph/treesitter/extractor_test.go
git commit -m "feat(graph): add AST extractor for Go, Python, and JS/TS"
```

---

### Task 4: Graph builder — full build and incremental sync

Walk a project directory, parse all source files, and populate the graph. Support incremental sync via git diff.

**Files:**
- Create: `internal/graph/builder.go`
- Create: `internal/graph/builder_test.go`

**Step 1: Write builder tests**

Create `internal/graph/builder_test.go`:

```go
package graph

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create a simple Go project
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

import "fmt"

func main() {
	fmt.Println("hello")
	helper()
}

func helper() {}
`), 0644)

	os.WriteFile(filepath.Join(dir, "util.go"), []byte(`package main

func Util() {}
`), 0644)

	// Create a Python file
	os.WriteFile(filepath.Join(dir, "script.py"), []byte(`import os

def greet():
    print("hello")
`), 0644)

	// Create an unsupported file
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Hello"), 0644)

	return dir
}

func TestBuildFull(t *testing.T) {
	dir := setupTestProject(t)
	dbPath := filepath.Join(t.TempDir(), "graph.db")

	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	b := NewBuilder(store)
	if err := b.BuildFull(dir); err != nil {
		t.Fatal(err)
	}

	stats, _ := store.Stats()

	// Should have file nodes + function nodes + type nodes
	if stats.TotalNodes < 5 {
		t.Fatalf("expected at least 5 nodes, got %d", stats.TotalNodes)
	}
	if stats.TotalEdges < 3 {
		t.Fatalf("expected at least 3 edges, got %d", stats.TotalEdges)
	}

	// Verify specific nodes exist
	nodes, _ := store.FindNodesByName("main")
	if len(nodes) == 0 {
		t.Error("expected to find 'main' function")
	}

	nodes, _ = store.FindNodesByName("greet")
	if len(nodes) == 0 {
		t.Error("expected to find 'greet' function")
	}
}

func TestBuildFullSkipsDirs(t *testing.T) {
	dir := setupTestProject(t)

	// Create a node_modules directory that should be skipped
	nmDir := filepath.Join(dir, "node_modules", "pkg")
	os.MkdirAll(nmDir, 0755)
	os.WriteFile(filepath.Join(nmDir, "index.js"), []byte("function foo() {}"), 0644)

	dbPath := filepath.Join(t.TempDir(), "graph.db")
	store, _ := OpenStore(dbPath)
	defer store.Close()

	b := NewBuilder(store)
	b.BuildFull(dir)

	// node_modules files should not be indexed
	nodes, _ := store.FindNodesByName("foo")
	if len(nodes) != 0 {
		t.Error("should not index node_modules")
	}
}

func TestBuildFullIdempotent(t *testing.T) {
	dir := setupTestProject(t)
	dbPath := filepath.Join(t.TempDir(), "graph.db")
	store, _ := OpenStore(dbPath)
	defer store.Close()

	b := NewBuilder(store)
	b.BuildFull(dir)
	stats1, _ := store.Stats()

	// Build again — should produce same counts
	b.BuildFull(dir)
	stats2, _ := store.Stats()

	if stats1.TotalNodes != stats2.TotalNodes {
		t.Errorf("node count changed: %d -> %d", stats1.TotalNodes, stats2.TotalNodes)
	}
	if stats1.TotalEdges != stats2.TotalEdges {
		t.Errorf("edge count changed: %d -> %d", stats1.TotalEdges, stats2.TotalEdges)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
CGO_ENABLED=1 go test ./internal/graph/ -v -count=1 -run TestBuild
```

Expected: compilation error.

**Step 3: Implement the builder**

Create `internal/graph/builder.go`:

```go
package graph

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lofari/golem/internal/graph/treesitter"
)

// skipDirs are directories that should never be indexed.
var skipDirs = map[string]bool{
	".ctx":         true,
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	".venv":        true,
	"__pycache__":  true,
	".godot":       true,
	"build":        true,
	"dist":         true,
	".next":        true,
}

// Builder constructs and updates the code graph.
type Builder struct {
	store *Store
}

// NewBuilder creates a new graph builder.
func NewBuilder(store *Store) *Builder {
	return &Builder{store: store}
}

// BuildFull does a complete rebuild of the graph from the project directory.
func (b *Builder) BuildFull(projectPath string) error {
	if err := b.store.Clear(); err != nil {
		return fmt.Errorf("clearing graph: %w", err)
	}

	var allNodes []Node
	var allEdges []Edge

	err := filepath.WalkDir(projectPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}

		// Skip excluded directories
		if d.IsDir() {
			if skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Get relative path
		relPath, _ := filepath.Rel(projectPath, path)

		// Read file
		src, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable files
		}

		// Parse and extract
		lang := treesitter.DetectLanguage(relPath)
		if lang == "" {
			// Unsupported language — create file node only
			nodes, edges := treesitter.ExtractFileOnly(relPath)
			allNodes = append(allNodes, nodes...)
			allEdges = append(allEdges, edges...)
			return nil
		}

		tree, _, err := treesitter.ParseBytes(src, lang)
		if err != nil {
			return nil // skip parse errors
		}

		nodes, edges := treesitter.Extract(relPath, lang, tree, src)
		allNodes = append(allNodes, nodes...)
		allEdges = append(allEdges, edges...)

		return nil
	})
	if err != nil {
		return fmt.Errorf("walking project: %w", err)
	}

	if err := b.store.InsertBatch(allNodes, allEdges); err != nil {
		return fmt.Errorf("inserting graph: %w", err)
	}

	// Record indexing metadata
	b.store.SetMeta("last_indexed", time.Now().Format(time.RFC3339))
	if sha := gitHeadSHA(projectPath); sha != "" {
		b.store.SetMeta("last_commit", sha)
	}

	return nil
}

// Sync performs an incremental update based on git changes since last index.
// Falls back to full build if no baseline exists.
func (b *Builder) Sync(projectPath string) error {
	lastCommit, _ := b.store.GetMeta("last_commit")
	if lastCommit == "" {
		return b.BuildFull(projectPath)
	}

	// Get changed files since last indexed commit
	changed, err := gitChangedFiles(projectPath, lastCommit)
	if err != nil {
		// Can't diff — do full rebuild
		return b.BuildFull(projectPath)
	}

	// Also include uncommitted changes
	dirty, _ := gitDirtyFiles(projectPath)
	changed = append(changed, dirty...)
	changed = dedupe(changed)

	if len(changed) == 0 {
		return nil // nothing to update
	}

	for _, relPath := range changed {
		// Remove old nodes/edges for this file
		b.store.DeleteByPath(relPath)

		// Check if file still exists
		fullPath := filepath.Join(projectPath, relPath)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			continue // file was deleted
		}

		// Re-parse and insert
		src, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		lang := treesitter.DetectLanguage(relPath)
		if lang == "" {
			nodes, edges := treesitter.ExtractFileOnly(relPath)
			b.store.InsertBatch(nodes, edges)
			continue
		}

		tree, _, err := treesitter.ParseBytes(src, lang)
		if err != nil {
			continue
		}

		nodes, edges := treesitter.Extract(relPath, lang, tree, src)
		b.store.InsertBatch(nodes, edges)
	}

	// Update metadata
	b.store.SetMeta("last_indexed", time.Now().Format(time.RFC3339))
	if sha := gitHeadSHA(projectPath); sha != "" {
		b.store.SetMeta("last_commit", sha)
	}

	return nil
}

// --- Git helpers ---

func gitHeadSHA(dir string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitChangedFiles(dir, sinceCommit string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", sinceCommit+"..HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return splitLines(string(out)), nil
}

func gitDirtyFiles(dir string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	staged := exec.Command("git", "diff", "--name-only", "--cached")
	staged.Dir = dir
	out2, _ := staged.Output()
	files := splitLines(string(out))
	files = append(files, splitLines(string(out2))...)
	return files, nil
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func dedupe(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
```

**Step 4: Run tests**

```bash
CGO_ENABLED=1 go test ./internal/graph/ -v -count=1
```

Expected: all pass (store tests + builder tests).

**Step 5: Commit**

```bash
git add internal/graph/builder.go internal/graph/builder_test.go
git commit -m "feat(graph): add graph builder with full build and incremental sync"
```

---

### Task 5: CLI commands — `golem graph build` and `golem graph status`

**Files:**
- Create: `cmd/graph.go`

**Step 1: Implement the graph commands**

Create `cmd/graph.go`:

```go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/scaffold"
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Manage the code knowledge graph",
}

var graphBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build or rebuild the code knowledge graph",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		dbPath := filepath.Join(dir, ".ctx", "graph.db")
		store, err := graph.OpenStore(dbPath)
		if err != nil {
			return fmt.Errorf("opening graph db: %w", err)
		}
		defer store.Close()

		builder := graph.NewBuilder(store)

		fmt.Fprintf(os.Stderr, "golem: building code graph...\n")
		if err := builder.BuildFull(dir); err != nil {
			return fmt.Errorf("building graph: %w", err)
		}

		stats, _ := store.Stats()
		fmt.Fprintf(os.Stderr, "golem: graph built — %d nodes, %d edges\n", stats.TotalNodes, stats.TotalEdges)

		// Print type breakdown
		for t, count := range stats.NodeTypes {
			fmt.Fprintf(os.Stderr, "golem:   %s: %d\n", t, count)
		}

		return nil
	},
}

var graphStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show knowledge graph statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		dbPath := filepath.Join(dir, ".ctx", "graph.db")
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			fmt.Println("No graph database found. Run `golem graph build` first.")
			return nil
		}

		store, err := graph.OpenStore(dbPath)
		if err != nil {
			return fmt.Errorf("opening graph db: %w", err)
		}
		defer store.Close()

		stats, err := store.Stats()
		if err != nil {
			return err
		}

		lastCommit, _ := store.GetMeta("last_commit")
		lastIndexed, _ := store.GetMeta("last_indexed")

		fmt.Printf("Graph Database: %s\n", dbPath)
		if lastIndexed != "" {
			fmt.Printf("Last indexed:   %s\n", lastIndexed)
		}
		if lastCommit != "" {
			fmt.Printf("Last commit:    %s\n", lastCommit[:min(len(lastCommit), 12)])
		}
		fmt.Printf("\nNodes: %d\n", stats.TotalNodes)
		for t, count := range stats.NodeTypes {
			fmt.Printf("  %-12s %d\n", t, count)
		}
		fmt.Printf("\nEdges: %d\n", stats.TotalEdges)
		for t, count := range stats.EdgeTypes {
			fmt.Printf("  %-12s %d\n", t, count)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(graphCmd)
	graphCmd.AddCommand(graphBuildCmd)
	graphCmd.AddCommand(graphStatusCmd)
}
```

**Step 2: Verify build**

```bash
CGO_ENABLED=1 go build ./...
```

**Step 3: Smoke test**

```bash
CGO_ENABLED=1 go run . graph build
CGO_ENABLED=1 go run . graph status
```

Expected: graph builds successfully on the golem codebase itself, status shows node/edge counts.

**Step 4: Commit**

```bash
git add cmd/graph.go
git commit -m "feat(cli): add golem graph build and golem graph status commands"
```

---

### Task 6: MCP query tools

Add four graph query tools to the existing MCP server: `find_callers`, `find_dependencies`, `find_dependents`, `graph_query`.

**Files:**
- Create: `internal/mcp/graph_tools.go`
- Modify: `internal/mcp/server.go`

**Step 1: Implement graph tools**

Create `internal/mcp/graph_tools.go`:

```go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/lofari/golem/internal/graph"
)

// openGraph opens the graph database for the project directory.
// Returns nil store if no graph.db exists (not an error).
func (gs *GolemServer) openGraph() (*graph.Store, error) {
	dbPath := filepath.Join(gs.dir, ".ctx", "graph.db")
	return graph.OpenStore(dbPath)
}

// --- find_callers ---

func findCallersTool() mcp.Tool {
	return mcp.Tool{
		Name:        "find_callers",
		Description: "Find what calls a given function or method. Returns nodes that have CALLS edges pointing to the target.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"name":  map[string]string{"type": "string", "description": "Function or method name to search for"},
				"depth": map[string]string{"type": "integer", "description": "Traversal depth (default 1, max 5)"},
			},
			Required: []string{"name"},
		},
	}
}

func (gs *GolemServer) handleFindCallers(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	name := getStr(args, "name")
	depth := getInt(args, "depth", 1)
	if depth < 1 {
		depth = 1
	}
	if depth > 5 {
		depth = 5
	}

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("opening graph: %v", err)), nil
	}
	defer store.Close()

	// Find target nodes by name
	targets, err := store.FindNodesByName(name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("querying graph: %v", err)), nil
	}
	if len(targets) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no nodes found matching %q", name)), nil
	}

	// BFS to find callers
	type caller struct {
		Node  graph.Node `json:"node"`
		Via   string     `json:"via"`
		Depth int        `json:"depth"`
	}

	var callers []caller
	visited := make(map[string]bool)

	// Seed with target node IDs
	current := make([]string, 0, len(targets))
	for _, t := range targets {
		current = append(current, t.ID)
		visited[t.ID] = true
	}

	for d := 1; d <= depth; d++ {
		var next []string
		for _, nodeID := range current {
			edges, _ := store.EdgesToOfType(nodeID, "CALLS")
			for _, e := range edges {
				if visited[e.From] {
					continue
				}
				visited[e.From] = true
				node, _ := store.NodeByID(e.From)
				if node != nil {
					callers = append(callers, caller{
						Node:  *node,
						Via:   fmt.Sprintf("CALLS:%s", nodeID),
						Depth: d,
					})
					next = append(next, e.From)
				}
			}
		}
		current = next
	}

	if len(callers) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no callers found for %q", name)), nil
	}

	out, _ := json.MarshalIndent(callers, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// --- find_dependencies ---

func findDependenciesTool() mcp.Tool {
	return mcp.Tool{
		Name:        "find_dependencies",
		Description: "Find what a file or function depends on — imports, calls, and type usage.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"name": map[string]string{"type": "string", "description": "File path or function name to search for"},
			},
			Required: []string{"name"},
		},
	}
}

func (gs *GolemServer) handleFindDependencies(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	name := getStr(args, "name")

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("opening graph: %v", err)), nil
	}
	defer store.Close()

	// Try to find as file first, then by name
	targets, _ := store.NodesByPath(name)
	if len(targets) == 0 {
		targets, _ = store.FindNodesByName(name)
	}
	if len(targets) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no nodes found matching %q", name)), nil
	}

	type deps struct {
		Imports []string `json:"imports,omitempty"`
		Calls   []string `json:"calls,omitempty"`
		Uses    []string `json:"uses,omitempty"`
	}

	result := deps{}
	seen := make(map[string]bool)

	for _, t := range targets {
		edges, _ := store.EdgesFrom(t.ID)
		for _, e := range edges {
			key := e.Type + ":" + e.To
			if seen[key] {
				continue
			}
			seen[key] = true

			// Resolve target node name
			label := e.To
			if n, _ := store.NodeByID(e.To); n != nil {
				label = fmt.Sprintf("%s (%s:%d)", n.Name, n.Path, n.Line)
			}

			switch e.Type {
			case "IMPORTS":
				result.Imports = append(result.Imports, label)
			case "CALLS":
				result.Calls = append(result.Calls, label)
			case "USES":
				result.Uses = append(result.Uses, label)
			}
		}
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// --- find_dependents ---

func findDependentsTool() mcp.Tool {
	return mcp.Tool{
		Name:        "find_dependents",
		Description: "Find what depends on a file or symbol — what breaks if you change it.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"name": map[string]string{"type": "string", "description": "File path or symbol name to search for"},
			},
			Required: []string{"name"},
		},
	}
}

func (gs *GolemServer) handleFindDependents(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	name := getStr(args, "name")

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("opening graph: %v", err)), nil
	}
	defer store.Close()

	// Find target nodes
	targets, _ := store.NodesByPath(name)
	if len(targets) == 0 {
		targets, _ = store.FindNodesByName(name)
	}
	if len(targets) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no nodes found matching %q", name)), nil
	}

	type dependent struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Line int    `json:"line,omitempty"`
		Via  string `json:"via"`
	}

	var dependents []dependent
	seen := make(map[string]bool)

	for _, t := range targets {
		// Also find all symbols defined in this file
		searchIDs := []string{t.ID}
		if t.Type == "file" {
			defined, _ := store.EdgesOfType(t.ID, "DEFINES")
			for _, e := range defined {
				searchIDs = append(searchIDs, e.To)
			}
		}

		for _, id := range searchIDs {
			inEdges, _ := store.EdgesTo(id)
			for _, e := range inEdges {
				if e.Type == "DEFINES" {
					continue // skip DEFINES edges (same file)
				}
				if seen[e.From] {
					continue
				}
				seen[e.From] = true

				node, _ := store.NodeByID(e.From)
				if node != nil {
					dependents = append(dependents, dependent{
						Name: node.Name,
						Path: node.Path,
						Line: node.Line,
						Via:  fmt.Sprintf("%s:%s", e.Type, id),
					})
				}
			}
		}
	}

	if len(dependents) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no dependents found for %q", name)), nil
	}

	out, _ := json.MarshalIndent(dependents, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// --- graph_query (general traversal) ---

func graphQueryTool() mcp.Tool {
	return mcp.Tool{
		Name:        "graph_query",
		Description: "General-purpose graph traversal. Find nodes and their relationships by ID, name, or path.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"node":       map[string]string{"type": "string", "description": "Node ID, name, or file path to start from"},
				"edge_types": map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "description": "Edge types to follow (e.g. CALLS, IMPORTS, DEFINES). All if empty."},
				"depth":      map[string]string{"type": "integer", "description": "Traversal depth (default 1, max 5)"},
				"direction":  map[string]string{"type": "string", "description": "Traversal direction: outbound (default), inbound, or both"},
			},
			Required: []string{"node"},
		},
	}
}

func (gs *GolemServer) handleGraphQuery(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	nodeRef := getStr(args, "node")
	edgeTypes := getStrSlice(args, "edge_types")
	depth := getInt(args, "depth", 1)
	direction := getStr(args, "direction")
	if depth < 1 {
		depth = 1
	}
	if depth > 5 {
		depth = 5
	}
	if direction == "" {
		direction = "outbound"
	}

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("opening graph: %v", err)), nil
	}
	defer store.Close()

	// Resolve starting node(s)
	var startIDs []string
	if n, _ := store.NodeByID(nodeRef); n != nil {
		startIDs = []string{n.ID}
	} else {
		nodes, _ := store.FindNodesByName(nodeRef)
		if len(nodes) == 0 {
			nodes, _ = store.NodesByPath(nodeRef)
		}
		for _, n := range nodes {
			startIDs = append(startIDs, n.ID)
		}
	}

	if len(startIDs) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no nodes found matching %q", nodeRef)), nil
	}

	edgeFilter := make(map[string]bool)
	for _, et := range edgeTypes {
		edgeFilter[strings.ToUpper(et)] = true
	}

	type result struct {
		Nodes []graph.Node `json:"nodes"`
		Edges []graph.Edge `json:"edges"`
	}

	res := result{}
	visited := make(map[string]bool)
	seenEdges := make(map[string]bool)

	// Add start nodes
	for _, id := range startIDs {
		visited[id] = true
		if n, _ := store.NodeByID(id); n != nil {
			res.Nodes = append(res.Nodes, *n)
		}
	}

	current := startIDs
	for d := 0; d < depth; d++ {
		var next []string
		for _, id := range current {
			var edges []graph.Edge
			if direction == "outbound" || direction == "both" {
				out, _ := store.EdgesFrom(id)
				edges = append(edges, out...)
			}
			if direction == "inbound" || direction == "both" {
				in, _ := store.EdgesTo(id)
				edges = append(edges, in...)
			}

			for _, e := range edges {
				if len(edgeFilter) > 0 && !edgeFilter[e.Type] {
					continue
				}
				edgeKey := fmt.Sprintf("%s-%s-%s", e.From, e.To, e.Type)
				if seenEdges[edgeKey] {
					continue
				}
				seenEdges[edgeKey] = true
				res.Edges = append(res.Edges, e)

				// Add target node
				targetID := e.To
				if targetID == id {
					targetID = e.From
				}
				if !visited[targetID] {
					visited[targetID] = true
					if n, _ := store.NodeByID(targetID); n != nil {
						res.Nodes = append(res.Nodes, *n)
					}
					next = append(next, targetID)
				}
			}
		}
		current = next
	}

	out, _ := json.MarshalIndent(res, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// getInt extracts an int from MCP arguments with a default.
func getInt(args map[string]any, key string, defaultVal int) int {
	v, ok := args[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return defaultVal
	}
}
```

**Step 2: Register graph tools in server.go**

Modify `internal/mcp/server.go` — update `registerTools()` and `ListTools()`:

In `registerTools()`, add after the existing `gs.mcpServer.AddTool(logSessionTool(), gs.handleLogSession)`:

```go
	gs.mcpServer.AddTool(findCallersTool(), gs.handleFindCallers)
	gs.mcpServer.AddTool(findDependenciesTool(), gs.handleFindDependencies)
	gs.mcpServer.AddTool(findDependentsTool(), gs.handleFindDependents)
	gs.mcpServer.AddTool(graphQueryTool(), gs.handleGraphQuery)
```

In `ListTools()`, update the return slice to include the new tools:

```go
return []string{"mark_task", "set_phase", "set_status", "add_decision", "add_pitfall", "add_locked", "log_session", "find_callers", "find_dependencies", "find_dependents", "graph_query"}
```

**Step 3: Verify build**

```bash
CGO_ENABLED=1 go build ./...
```

**Step 4: Run all tests**

```bash
CGO_ENABLED=1 go test ./... -count=1
```

Expected: all pass.

**Step 5: Commit**

```bash
git add internal/mcp/graph_tools.go internal/mcp/server.go
git commit -m "feat(mcp): add graph query tools — find_callers, find_dependencies, find_dependents, graph_query"
```

---

### Task 7: Builder loop integration — auto-sync at session start

Wire the graph sync into the builder loop so agents always have a fresh graph.

**Files:**
- Modify: `internal/runner/builder.go`

**Step 1: Add graph sync before first iteration**

In `internal/runner/builder.go`, add to the import block:

```go
"github.com/lofari/golem/internal/graph"
```

Then add a graph sync block after the initial state read (after `state, err := golemctx.ReadState(cfg.Dir)` and the tasks check), before the `remaining := state.RemainingTasks()` line:

```go
	// Sync knowledge graph if it exists
	graphPath := filepath.Join(cfg.Dir, ".ctx", "graph.db")
	if _, statErr := os.Stat(graphPath); statErr == nil {
		if gStore, gErr := graph.OpenStore(graphPath); gErr == nil {
			gBuilder := graph.NewBuilder(gStore)
			if syncErr := gBuilder.Sync(cfg.Dir); syncErr != nil {
				fmt.Fprintf(os.Stderr, "golem: warning: graph sync failed: %v\n", syncErr)
			} else {
				fmt.Fprintf(os.Stderr, "golem: graph synced\n")
			}
			gStore.Close()
		}
	}
```

**Step 2: Verify build**

```bash
CGO_ENABLED=1 go build ./...
```

**Step 3: Run all tests**

```bash
CGO_ENABLED=1 go test ./... -count=1
```

Expected: all pass.

**Step 4: Commit**

```bash
git add internal/runner/builder.go
git commit -m "feat(runner): auto-sync knowledge graph at session start"
```

---

### Task 8: End-to-end verification

**Step 1: Build and test everything**

```bash
CGO_ENABLED=1 go build ./... && CGO_ENABLED=1 go test ./... -count=1
```

Expected: all tests pass.

**Step 2: Test on golem's own codebase**

```bash
CGO_ENABLED=1 go run . graph build
CGO_ENABLED=1 go run . graph status
```

Expected: graph builds with function/type/file nodes and DEFINES/IMPORTS/CALLS edges from the golem Go codebase.

**Step 3: Test MCP tools manually**

Verify the MCP server starts and lists graph tools:

```bash
CGO_ENABLED=1 go run . mcp-serve --help
```

**Step 4: Install**

```bash
CGO_ENABLED=1 go install .
```

**Step 5: Commit any fixes**

```bash
git add -A
git commit -m "feat(graph): complete knowledge graph Phase 1"
```
