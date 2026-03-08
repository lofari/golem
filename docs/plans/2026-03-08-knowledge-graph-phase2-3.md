# Knowledge Graph Phase 2 & 3 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add semantic vector embeddings and a documentation graph to Golem's knowledge graph, enabling natural-language code search and doc-code linking.

**Architecture:** Phase 2 adds an abstract `Embedder` interface with an ONNX provider (hugot + BGE-small-en-v1.5), sqlite-vec storage in graph.db, a `semantic_search` MCP tool, and `golem graph embed` CLI. Phase 3 adds markdown parsing into document/section nodes with REFERENCES/EXPLAINS edges to code, integrated into the existing build pipeline.

**Tech Stack:** Go, `knights-analytics/hugot` (ONNX inference), `asg017/sqlite-vec-go-bindings/cgo` (vector search), `mattn/go-sqlite3` (existing), `smacker/go-tree-sitter` (existing)

**Design doc:** `docs/plans/2026-03-08-knowledge-graph-phase2-3-design.md`

---

### Task 0: Add Phase 2 dependencies

**Files:**
- Modify: `go.mod`

**Step 1: Add hugot and sqlite-vec**

```bash
CGO_ENABLED=1 go get github.com/knights-analytics/hugot
CGO_ENABLED=1 go get github.com/asg017/sqlite-vec-go-bindings/cgo
```

**Step 2: Verify build**

```bash
CGO_ENABLED=1 go build ./...
```

If hugot requires build tags, use:
```bash
CGO_ENABLED=1 go build -tags "GOB" ./...
```

Check hugot's README for which build tag is needed. `GOB` is the pure-Go backend; `ORT` requires the ONNX Runtime shared library. Start with `GOB` (pure Go) for simpler builds. If `GOB` doesn't support feature extraction, fall back to `ORT`.

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "feat(graph): add hugot and sqlite-vec dependencies"
```

---

### Task 1: Embedder interface

Create the abstract embedding provider interface.

**Files:**
- Create: `internal/graph/embed/embedder.go`
- Create: `internal/graph/embed/embedder_test.go`

**Step 1: Write the interface and types**

```go
// internal/graph/embed/embedder.go
package embed

import "context"

// Embedder generates vector embeddings from text.
type Embedder interface {
	// Embed converts texts to float32 vectors. Returns one vector per input text.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// Dimensions returns the output vector dimensionality (e.g. 384).
	Dimensions() int
	// Close releases model resources.
	Close() error
}

// EmbeddingEntry pairs a graph node ID with its embedding vector.
type EmbeddingEntry struct {
	NodeID string
	Vector []float32
}

// SimilarResult is a node returned by similarity search.
type SimilarResult struct {
	NodeID   string  `json:"nodeId"`
	Distance float32 `json:"distance"`
}
```

**Step 2: Write a test using a mock embedder**

```go
// internal/graph/embed/embedder_test.go
package embed

import (
	"context"
	"testing"
)

type mockEmbedder struct {
	dims int
}

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = make([]float32, m.dims)
		for j := range out[i] {
			out[i][j] = float32(len(texts[i])) / 100.0 // deterministic dummy
		}
	}
	return out, nil
}

func (m *mockEmbedder) Dimensions() int    { return m.dims }
func (m *mockEmbedder) Close() error       { return nil }

func TestMockEmbedder(t *testing.T) {
	e := &mockEmbedder{dims: 384}
	vecs, err := e.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vecs))
	}
	if len(vecs[0]) != 384 {
		t.Fatalf("expected 384 dims, got %d", len(vecs[0]))
	}
	if e.Dimensions() != 384 {
		t.Fatal("expected Dimensions() == 384")
	}
}
```

**Step 3: Run test to verify it passes**

```bash
CGO_ENABLED=1 go test ./internal/graph/embed/ -v -count=1
```

Expected: PASS

**Step 4: Commit**

```bash
git add internal/graph/embed/
git commit -m "feat(graph): add Embedder interface and types"
```

---

### Task 2: sqlite-vec integration in store

Extend the graph store with sqlite-vec virtual table for vector storage and KNN search.

**Files:**
- Modify: `internal/graph/store.go` (schema creation, new methods)
- Modify: `internal/graph/store_test.go` (new test cases)

**Step 1: Add sqlite-vec import and initialization**

In `internal/graph/store.go`, add to imports:

```go
import (
	// existing imports...
	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)
```

Add an `init()` function at the top of the file (after imports):

```go
func init() {
	sqlite_vec.Auto()
}
```

This registers the sqlite-vec extension with all future SQLite connections.

**Step 2: Add vec_embeddings virtual table to schema**

In `internal/graph/store.go`, find the `createSchema` method (around line 37). After the existing `CREATE INDEX` statements, add:

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS vec_embeddings USING vec0(
    node_id TEXT PRIMARY KEY,
    embedding float[384]
);
```

Note: The dimension (384) is hardcoded for BGE-small-en-v1.5. If we need to support different models later, we'd need to recreate the table.

**Step 3: Add embedding store methods**

Add these methods to `internal/graph/store.go` after the existing `Stats()` method:

```go
// InsertEmbedding stores a single embedding vector for a node.
func (s *Store) InsertEmbedding(nodeID string, vector []float32) error {
	blob, err := sqlite_vec.SerializeFloat32(vector)
	if err != nil {
		return fmt.Errorf("serialize vector: %w", err)
	}
	_, err = s.db.Exec("INSERT OR REPLACE INTO vec_embeddings(node_id, embedding) VALUES (?, ?)", nodeID, blob)
	return err
}

// InsertEmbeddingsBatch stores multiple embeddings in a single transaction.
func (s *Store) InsertEmbeddingsBatch(entries []EmbeddingEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT OR REPLACE INTO vec_embeddings(node_id, embedding) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range entries {
		blob, err := sqlite_vec.SerializeFloat32(e.Vector)
		if err != nil {
			return fmt.Errorf("serialize vector for %s: %w", e.NodeID, err)
		}
		if _, err := stmt.Exec(e.NodeID, blob); err != nil {
			return fmt.Errorf("insert embedding for %s: %w", e.NodeID, err)
		}
	}
	return tx.Commit()
}

// SearchSimilar finds the k most similar nodes to the query vector.
func (s *Store) SearchSimilar(queryVec []float32, limit int) ([]SimilarResult, error) {
	blob, err := sqlite_vec.SerializeFloat32(queryVec)
	if err != nil {
		return nil, fmt.Errorf("serialize query: %w", err)
	}
	rows, err := s.db.Query(
		"SELECT node_id, distance FROM vec_embeddings WHERE embedding MATCH ? AND k = ? ORDER BY distance",
		blob, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SimilarResult
	for rows.Next() {
		var r SimilarResult
		if err := rows.Scan(&r.NodeID, &r.Distance); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// DeleteEmbeddingsByPath removes embeddings for all nodes with the given file path.
func (s *Store) DeleteEmbeddingsByPath(path string) error {
	_, err := s.db.Exec(
		"DELETE FROM vec_embeddings WHERE node_id IN (SELECT id FROM nodes WHERE path = ?)",
		path,
	)
	return err
}

// ClearEmbeddings removes all embeddings.
func (s *Store) ClearEmbeddings() error {
	_, err := s.db.Exec("DELETE FROM vec_embeddings")
	return err
}

// EmbeddingCount returns the number of stored embeddings.
func (s *Store) EmbeddingCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT count(*) FROM vec_embeddings").Scan(&count)
	return count, err
}
```

Also add the `EmbeddingEntry` and `SimilarResult` types to the store file (or import from embed package — decide based on import cycle concerns):

```go
// EmbeddingEntry pairs a node ID with its vector.
type EmbeddingEntry struct {
	NodeID string
	Vector []float32
}

// SimilarResult is returned by SearchSimilar.
type SimilarResult struct {
	NodeID   string  `json:"nodeId"`
	Distance float32 `json:"distance"`
}
```

If there's an import cycle with `embed` package, keep these types in `store.go` directly. Remove the duplicates from `embed/embedder.go` and import from `graph` package instead (or keep them in `model/model.go`).

**Step 4: Write tests for embedding store methods**

Add to `internal/graph/store_test.go`:

```go
func TestInsertAndSearchEmbeddings(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Insert some nodes first
	nodes := []Node{
		{ID: "fn:main.go:foo", Type: "function", Name: "foo", Path: "main.go", Line: 1},
		{ID: "fn:main.go:bar", Type: "function", Name: "bar", Path: "main.go", Line: 10},
		{ID: "fn:util.go:baz", Type: "function", Name: "baz", Path: "util.go", Line: 1},
	}
	if err := store.InsertBatch(nodes, nil); err != nil {
		t.Fatal(err)
	}

	// Insert embeddings — foo and baz are similar, bar is different
	entries := []EmbeddingEntry{
		{NodeID: "fn:main.go:foo", Vector: makeVec(384, 0.1)},
		{NodeID: "fn:main.go:bar", Vector: makeVec(384, 0.9)},
		{NodeID: "fn:util.go:baz", Vector: makeVec(384, 0.11)},
	}
	if err := store.InsertEmbeddingsBatch(entries); err != nil {
		t.Fatal(err)
	}

	// Search for vectors similar to foo's
	results, err := store.SearchSimilar(makeVec(384, 0.1), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// foo should be closest (distance 0), then baz
	if results[0].NodeID != "fn:main.go:foo" {
		t.Errorf("expected foo first, got %s", results[0].NodeID)
	}

	// Test count
	count, err := store.EmbeddingCount()
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("expected 3 embeddings, got %d", count)
	}

	// Test delete by path
	if err := store.DeleteEmbeddingsByPath("main.go"); err != nil {
		t.Fatal(err)
	}
	count, err = store.EmbeddingCount()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 embedding after delete, got %d", count)
	}
}

func makeVec(dims int, val float32) []float32 {
	v := make([]float32, dims)
	for i := range v {
		v[i] = val
	}
	return v
}
```

**Step 5: Run tests**

```bash
CGO_ENABLED=1 go test ./internal/graph/ -v -count=1 -run TestInsertAndSearchEmbeddings
```

Expected: PASS

**Step 6: Run all existing tests to verify no regressions**

```bash
CGO_ENABLED=1 go test ./... -count=1
```

Expected: All pass

**Step 7: Commit**

```bash
git add internal/graph/store.go internal/graph/store_test.go
git commit -m "feat(graph): add sqlite-vec embedding storage and KNN search"
```

---

### Task 3: ONNX embedder provider

Implement the hugot-based ONNX embedding provider.

**Files:**
- Create: `internal/graph/embed/onnx.go`
- Create: `internal/graph/embed/manager.go`
- Create: `internal/graph/embed/onnx_test.go`

**Step 1: Implement model manager**

```go
// internal/graph/embed/manager.go
package embed

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/knights-analytics/hugot"
)

const (
	DefaultModel   = "BAAI/bge-small-en-v1.5"
	DefaultModelID = "bge-small-en-v1.5"
)

// DefaultModelDir returns ~/.config/golem/models/
func DefaultModelDir() string {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		cfgDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(cfgDir, "golem", "models")
}

// EnsureModel downloads the model if not already cached. Returns the path to the model directory.
func EnsureModel(modelID, modelDir string) (string, error) {
	modelPath := filepath.Join(modelDir, modelID)
	if _, err := os.Stat(filepath.Join(modelPath, "tokenizer.json")); err == nil {
		return modelPath, nil // already downloaded
	}

	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return "", fmt.Errorf("create model dir: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Downloading embedding model %s...\n", modelID)
	downloadedPath, err := hugot.DownloadModel(DefaultModel, modelDir, hugot.NewDownloadOptions())
	if err != nil {
		return "", fmt.Errorf("download model: %w", err)
	}
	return downloadedPath, nil
}
```

Note: The hugot `DownloadModel` function downloads from HuggingFace and saves to the specified directory. The exact model name may need adjustment — check if hugot uses `BAAI/bge-small-en-v1.5` or needs a different identifier. Test this in Step 4.

**Step 2: Implement ONNX embedder**

```go
// internal/graph/embed/onnx.go
package embed

import (
	"context"
	"fmt"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/pipelines"
)

// ONNXEmbedder generates embeddings using a local ONNX model via hugot.
type ONNXEmbedder struct {
	session  *hugot.Session
	pipeline *pipelines.FeatureExtractionPipeline
	dims     int
}

// NewONNXEmbedder creates an embedder from a downloaded model directory.
// modelDir should contain model.onnx and tokenizer.json.
func NewONNXEmbedder(modelDir string) (*ONNXEmbedder, error) {
	// Use pure-Go backend (no ONNX Runtime shared lib needed)
	session, err := hugot.NewGoSession()
	if err != nil {
		return nil, fmt.Errorf("create hugot session: %w", err)
	}

	config := pipelines.FeatureExtractionConfig{
		ModelPath:    modelDir,
		Name:         "golem-embedder",
		OnnxFilename: "model.onnx",
	}

	pipeline, err := hugot.NewPipeline(session, config)
	if err != nil {
		session.Destroy()
		return nil, fmt.Errorf("create pipeline: %w", err)
	}

	return &ONNXEmbedder{
		session:  session,
		pipeline: pipeline,
		dims:     384, // BGE-small-en-v1.5 output dimension
	}, nil
}

func (e *ONNXEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	result, err := e.pipeline.RunPipeline(texts)
	if err != nil {
		return nil, fmt.Errorf("run pipeline: %w", err)
	}
	return result.Embeddings, nil
}

func (e *ONNXEmbedder) Dimensions() int { return e.dims }

func (e *ONNXEmbedder) Close() error {
	return e.session.Destroy()
}
```

**Important notes for implementation:**
- Start with `hugot.NewGoSession()` (pure Go, no ONNX Runtime dependency). If it doesn't support FeatureExtraction, switch to `hugot.NewORTSession()` which requires the ONNX Runtime shared library.
- The `pipelines.FeatureExtractionConfig` struct name and `hugot.NewPipeline` generic function may differ — check the actual hugot API. The pattern shown in tests is `NewPipeline[*pipelines.FeatureExtractionPipeline](session, config)`.
- Build tags may be required: `-tags GOB` for Go backend or `-tags ORT` for ONNX Runtime.
- `result.Embeddings` should be `[][]float32` — verify by inspecting the hugot `FeatureExtractionOutput` type.

**Step 3: Write integration test**

```go
// internal/graph/embed/onnx_test.go
package embed

import (
	"context"
	"os"
	"testing"
)

func TestONNXEmbedder(t *testing.T) {
	modelDir := os.Getenv("GOLEM_TEST_MODEL_DIR")
	if modelDir == "" {
		t.Skip("set GOLEM_TEST_MODEL_DIR to run ONNX embedding tests")
	}

	e, err := NewONNXEmbedder(modelDir)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	if e.Dimensions() != 384 {
		t.Fatalf("expected 384 dims, got %d", e.Dimensions())
	}

	vecs, err := e.Embed(context.Background(), []string{
		"Function StartServer handles HTTP server initialization",
		"password validation and authentication logic",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vecs))
	}
	if len(vecs[0]) != 384 {
		t.Fatalf("expected 384 dims, got %d", len(vecs[0]))
	}

	// Vectors should be non-zero
	allZero := true
	for _, v := range vecs[0] {
		if v != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Fatal("embedding vector is all zeros")
	}
}
```

**Step 4: Download model and run integration test**

```bash
# First, download the model manually for testing
mkdir -p ~/.config/golem/models
# Use hugot's download or manual HuggingFace download
# This step may need adjustment based on hugot's actual download API

# Run integration test
GOLEM_TEST_MODEL_DIR=~/.config/golem/models/bge-small-en-v1.5 CGO_ENABLED=1 go test ./internal/graph/embed/ -v -count=1 -run TestONNXEmbedder
```

If the test fails due to API differences, adjust the code. The hugot API may require:
- Different config struct names (check `pipelines` package)
- Build tags (`-tags GOB` or `-tags ORT`)
- Different output access pattern (check `FeatureExtractionOutput` struct)

**Step 5: Run all tests**

```bash
CGO_ENABLED=1 go test ./... -count=1
```

Expected: All pass (ONNX test skipped without env var)

**Step 6: Commit**

```bash
git add internal/graph/embed/
git commit -m "feat(graph): add ONNX embedder with hugot and model manager"
```

---

### Task 4: Text representation builder

Build the component that converts graph nodes into text suitable for embedding.

**Files:**
- Create: `internal/graph/embed/text.go`
- Create: `internal/graph/embed/text_test.go`

**Step 1: Write failing test**

```go
// internal/graph/embed/text_test.go
package embed

import (
	"testing"

	"github.com/lofari/golem/internal/graph/model"
)

func TestNodeText(t *testing.T) {
	tests := []struct {
		name     string
		node     model.Node
		src      string // source snippet for functions
		expected string
	}{
		{
			name:     "function",
			node:     model.Node{ID: "fn:main.go:StartServer", Type: "function", Name: "StartServer", Path: "main.go", Line: 10},
			src:      "func StartServer(cfg Config) error {",
			expected: "Function StartServer in main.go: func StartServer(cfg Config) error {",
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
```

**Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/graph/embed/ -v -count=1 -run TestNodeText
```

Expected: FAIL — `NodeText` not defined

**Step 3: Implement NodeText**

```go
// internal/graph/embed/text.go
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
	default:
		return fmt.Sprintf("%s %s in %s", node.Type, node.Name, node.Path)
	}
}
```

**Step 4: Run tests**

```bash
CGO_ENABLED=1 go test ./internal/graph/embed/ -v -count=1 -run TestNodeText
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/graph/embed/text.go internal/graph/embed/text_test.go
git commit -m "feat(graph): add node-to-text representation for embeddings"
```

---

### Task 5: Embed pipeline — orchestrate embedding of graph nodes

Build the pipeline that reads nodes from the store, generates text, embeds in batches, and stores vectors.

**Files:**
- Create: `internal/graph/embed/pipeline.go`
- Create: `internal/graph/embed/pipeline_test.go`

**Step 1: Write failing test**

```go
// internal/graph/embed/pipeline_test.go
package embed

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lofari/golem/internal/graph"
)

func TestEmbedPipeline(t *testing.T) {
	dir := t.TempDir()
	store, err := graph.OpenStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Insert test nodes
	nodes := []graph.Node{
		{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go", Line: 1},
		{ID: "fn:main.go:Foo", Type: "function", Name: "Foo", Path: "main.go", Line: 5},
		{ID: "fn:main.go:Bar", Type: "function", Name: "Bar", Path: "main.go", Line: 15},
		{ID: "type:main.go:Cfg", Type: "type", Name: "Cfg", Path: "main.go", Line: 25},
	}
	if err := store.InsertBatch(nodes, nil); err != nil {
		t.Fatal(err)
	}

	embedder := &mockEmbedder{dims: 384}
	p := NewPipeline(store, embedder)

	count, err := p.EmbedAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Errorf("expected 4 embedded, got %d", count)
	}

	ec, err := store.EmbeddingCount()
	if err != nil {
		t.Fatal(err)
	}
	if ec != 4 {
		t.Errorf("expected 4 stored embeddings, got %d", ec)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/graph/embed/ -v -count=1 -run TestEmbedPipeline
```

Expected: FAIL — `NewPipeline` not defined

**Step 3: Implement the pipeline**

```go
// internal/graph/embed/pipeline.go
package embed

import (
	"context"
	"fmt"

	"github.com/lofari/golem/internal/graph"
)

const defaultBatchSize = 32

// Pipeline orchestrates embedding graph nodes.
type Pipeline struct {
	store     *graph.Store
	embedder  Embedder
	batchSize int
}

// NewPipeline creates an embedding pipeline.
func NewPipeline(store *graph.Store, embedder Embedder) *Pipeline {
	return &Pipeline{
		store:     store,
		embedder:  embedder,
		batchSize: defaultBatchSize,
	}
}

// embeddableTypes are the node types we generate embeddings for.
var embeddableTypes = []string{"function", "method", "type", "file"}

// EmbedAll clears existing embeddings and embeds all eligible nodes.
// Returns the number of nodes embedded.
func (p *Pipeline) EmbedAll(ctx context.Context) (int, error) {
	if err := p.store.ClearEmbeddings(); err != nil {
		return 0, fmt.Errorf("clear embeddings: %w", err)
	}

	var allNodes []graph.Node
	for _, typ := range embeddableTypes {
		nodes, err := p.store.NodesByType(typ)
		if err != nil {
			return 0, fmt.Errorf("query nodes of type %s: %w", typ, err)
		}
		allNodes = append(allNodes, nodes...)
	}

	return p.embedNodes(ctx, allNodes)
}

// EmbedByPath embeds nodes for the given file paths (for incremental sync).
func (p *Pipeline) EmbedByPath(ctx context.Context, paths []string) (int, error) {
	var allNodes []graph.Node
	for _, path := range paths {
		// Delete old embeddings for this path
		if err := p.store.DeleteEmbeddingsByPath(path); err != nil {
			return 0, fmt.Errorf("delete embeddings for %s: %w", path, err)
		}
		nodes, err := p.store.NodesByPath(path)
		if err != nil {
			return 0, fmt.Errorf("query nodes for %s: %w", path, err)
		}
		allNodes = append(allNodes, nodes...)
	}

	return p.embedNodes(ctx, allNodes)
}

func (p *Pipeline) embedNodes(ctx context.Context, nodes []graph.Node) (int, error) {
	if len(nodes) == 0 {
		return 0, nil
	}

	total := 0
	for i := 0; i < len(nodes); i += p.batchSize {
		end := i + p.batchSize
		if end > len(nodes) {
			end = len(nodes)
		}
		batch := nodes[i:end]

		// Build text representations
		texts := make([]string, len(batch))
		for j, n := range batch {
			texts[j] = NodeText(n, "") // No source snippet in initial version
		}

		// Embed
		vectors, err := p.embedder.Embed(ctx, texts)
		if err != nil {
			return total, fmt.Errorf("embed batch at offset %d: %w", i, err)
		}

		// Store
		entries := make([]graph.EmbeddingEntry, len(batch))
		for j, n := range batch {
			entries[j] = graph.EmbeddingEntry{
				NodeID: n.ID,
				Vector: vectors[j],
			}
		}
		if err := p.store.InsertEmbeddingsBatch(entries); err != nil {
			return total, fmt.Errorf("store batch at offset %d: %w", i, err)
		}
		total += len(batch)
	}
	return total, nil
}
```

**Step 4: Run tests**

```bash
CGO_ENABLED=1 go test ./internal/graph/embed/ -v -count=1 -run TestEmbedPipeline
```

Expected: PASS

**Step 5: Run all tests**

```bash
CGO_ENABLED=1 go test ./... -count=1
```

Expected: All pass

**Step 6: Commit**

```bash
git add internal/graph/embed/pipeline.go internal/graph/embed/pipeline_test.go
git commit -m "feat(graph): add embedding pipeline with batch processing"
```

---

### Task 6: CLI command — `golem graph embed`

Add the `golem graph embed` command.

**Files:**
- Modify: `cmd/graph.go` (add embed subcommand)

**Step 1: Add embed subcommand**

In `cmd/graph.go`, after the existing `graphStatusCmd` definition, add:

```go
var graphEmbedCmd = &cobra.Command{
	Use:   "embed",
	Short: "Generate embeddings for graph nodes",
	Long:  "Generates vector embeddings for all code nodes in the knowledge graph using a local ONNX model.",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		dbPath := filepath.Join(dir, ".ctx", "graph.db")
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			return fmt.Errorf("no graph database found — run 'golem graph build' first")
		}

		store, err := graph.OpenStore(dbPath)
		if err != nil {
			return fmt.Errorf("open graph: %w", err)
		}
		defer store.Close()

		// Ensure model is downloaded
		modelDir, err := embed.EnsureModel(embed.DefaultModelID, embed.DefaultModelDir())
		if err != nil {
			return fmt.Errorf("ensure model: %w", err)
		}

		// Create embedder
		embedder, err := embed.NewONNXEmbedder(modelDir)
		if err != nil {
			return fmt.Errorf("create embedder: %w", err)
		}
		defer embedder.Close()

		p := embed.NewPipeline(store, embedder)

		sync, _ := cmd.Flags().GetBool("sync")
		if sync {
			// Incremental: get changed paths from graph metadata
			lastCommit, _ := store.GetMeta("embed_last_indexed_commit")
			if lastCommit == "" {
				fmt.Fprintf(os.Stderr, "No previous embed baseline, running full embed...\n")
				sync = false
			} else {
				// Reuse git diff logic from builder
				// This is a simplified version — may need to import git helpers
				fmt.Fprintf(os.Stderr, "Incremental embed not yet implemented, running full...\n")
				sync = false
			}
		}

		if !sync {
			fmt.Fprintf(os.Stderr, "Embedding all graph nodes...\n")
			count, err := p.EmbedAll(cmd.Context())
			if err != nil {
				return fmt.Errorf("embed: %w", err)
			}
			store.SetMeta("embed_model", embed.DefaultModelID)
			store.SetMeta("embed_last_indexed", time.Now().Format(time.RFC3339))
			fmt.Fprintf(os.Stderr, "Embedded %d nodes\n", count)
		}

		return nil
	},
}

func init() {
	graphEmbedCmd.Flags().Bool("sync", false, "Only embed changed nodes since last embed")
	graphCmd.AddCommand(graphEmbedCmd)
}
```

Add necessary imports at the top of `cmd/graph.go`:

```go
import (
	// existing imports...
	"time"
	"github.com/lofari/golem/internal/graph/embed"
)
```

**Step 2: Build and test manually**

```bash
CGO_ENABLED=1 go build ./...
./golem graph embed --help
```

Expected: Help text shows embed command with --sync flag

**Step 3: Run all tests**

```bash
CGO_ENABLED=1 go test ./... -count=1
```

Expected: All pass

**Step 4: Commit**

```bash
git add cmd/graph.go
git commit -m "feat(cli): add golem graph embed command"
```

---

### Task 7: MCP tool — `semantic_search`

Add the semantic_search tool to the MCP server.

**Files:**
- Modify: `internal/mcp/graph_tools.go` (add tool definition and handler)
- Modify: `internal/mcp/server.go` (register new tool)
- Modify: `internal/mcp/server_test.go` (add test)

**Step 1: Add semantic_search tool definition and handler**

In `internal/mcp/graph_tools.go`, add after the existing `graph_query` tool:

```go
func semanticSearchTool() mcp.Tool {
	return mcp.Tool{
		Name:        "semantic_search",
		Description: "Search code and documentation by natural language query. Returns the most semantically similar nodes in the knowledge graph.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Natural language search query (e.g. 'authentication logic', 'database connection handling')",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results (default: 10, max: 50)",
				},
				"types": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Filter by node types: function, method, type, file, document, section (default: all)",
				},
			},
			Required: []string{"query"},
		},
	}
}

func (gs *GolemServer) handleSemanticSearch(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()

	query, ok := args["query"].(string)
	if !ok || query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}

	limit := getInt(args, "limit", 10)
	if limit > 50 {
		limit = 50
	}

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("open graph: %v", err)), nil
	}
	defer store.Close()

	// Check if embeddings exist
	count, err := store.EmbeddingCount()
	if err != nil || count == 0 {
		return mcp.NewToolResultError("no embeddings found — run 'golem graph embed' first"), nil
	}

	// Open embedder for query embedding
	modelDir, err := embed.EnsureModel(embed.DefaultModelID, embed.DefaultModelDir())
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("load model: %v", err)), nil
	}
	embedder, err := embed.NewONNXEmbedder(modelDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("create embedder: %v", err)), nil
	}
	defer embedder.Close()

	// Embed query
	vecs, err := embedder.Embed(context.Background(), []string{query})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("embed query: %v", err)), nil
	}

	// Search
	results, err := store.SearchSimilar(vecs[0], limit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("search: %v", err)), nil
	}

	// Resolve nodes and apply type filter
	typeFilter := map[string]bool{}
	if types, ok := args["types"].([]any); ok {
		for _, t := range types {
			if s, ok := t.(string); ok {
				typeFilter[s] = true
			}
		}
	}

	type searchResult struct {
		Name     string  `json:"name"`
		Path     string  `json:"path"`
		Line     int     `json:"line,omitempty"`
		Type     string  `json:"type"`
		Score    float32 `json:"score"`
	}

	var output []searchResult
	for _, r := range results {
		node, err := store.NodeByID(r.NodeID)
		if err != nil || node == nil {
			continue
		}
		if len(typeFilter) > 0 && !typeFilter[node.Type] {
			continue
		}
		output = append(output, searchResult{
			Name:  node.Name,
			Path:  node.Path,
			Line:  node.Line,
			Type:  node.Type,
			Score: 1.0 - r.Distance, // convert distance to similarity
		})
	}

	data, _ := json.Marshal(output)
	return mcp.NewToolResultText(string(data)), nil
}
```

**Note:** Loading the embedder per-call is expensive. A production optimization would be caching the embedder on the GolemServer struct. For Phase 2 MVP, per-call loading is acceptable — optimize later if needed.

**Step 2: Register the tool in server.go**

In `internal/mcp/server.go`, find the `registerTools()` method. Add:

```go
gs.mcpServer.AddTool(semanticSearchTool(), gs.handleSemanticSearch)
```

Also update the `ListTools()` method to include `"semantic_search"`.

**Step 3: Add import**

In `internal/mcp/graph_tools.go`, add:

```go
import (
	// existing imports...
	"github.com/lofari/golem/internal/graph/embed"
)
```

**Step 4: Build and run tests**

```bash
CGO_ENABLED=1 go build ./...
CGO_ENABLED=1 go test ./internal/mcp/ -v -count=1
```

Expected: All pass

**Step 5: Commit**

```bash
git add internal/mcp/graph_tools.go internal/mcp/server.go
git commit -m "feat(mcp): add semantic_search tool for natural language code search"
```

---

### Task 8: Extend `golem graph status` with embedding info

**Files:**
- Modify: `cmd/graph.go` (extend status output)

**Step 1: Add embedding stats to status command**

In `cmd/graph.go`, find the `graphStatusCmd` handler. After the existing stats output, add:

```go
// Embedding stats
embedCount, err := store.EmbeddingCount()
if err == nil && embedCount > 0 {
	embedModel, _ := store.GetMeta("embed_model")
	embedTime, _ := store.GetMeta("embed_last_indexed")
	fmt.Printf("\nEmbeddings: %d nodes embedded", embedCount)
	if embedModel != "" {
		fmt.Printf(" (model: %s)", embedModel)
	}
	fmt.Println()
	if embedTime != "" {
		fmt.Printf("Last embedded: %s\n", embedTime)
	}
}
```

**Step 2: Build and test**

```bash
CGO_ENABLED=1 go build ./...
CGO_ENABLED=1 go test ./... -count=1
```

Expected: All pass

**Step 3: Commit**

```bash
git add cmd/graph.go
git commit -m "feat(cli): show embedding stats in golem graph status"
```

---

### Task 9: Auto-embed on session start

Extend the builder loop to incrementally embed changed nodes after graph sync.

**Files:**
- Modify: `internal/runner/builder.go` (add embed sync after graph sync)

**Step 1: Add auto-embed logic**

In `internal/runner/builder.go`, find the graph sync block (around line 69-81). After the `gStore.Close()` line, add a new block:

```go
// Incremental embed if embeddings exist
if syncErr == nil {
	// Reopen store for embedding check
	if eStore, eErr := graph.OpenStore(graphPath); eErr == nil {
		if eCount, _ := eStore.EmbeddingCount(); eCount > 0 {
			modelDir, mErr := embed.EnsureModel(embed.DefaultModelID, embed.DefaultModelDir())
			if mErr == nil {
				if embedder, oErr := embed.NewONNXEmbedder(modelDir); oErr == nil {
					p := embed.NewPipeline(eStore, embedder)
					// Get changed paths from git (reuse builder's sync logic)
					// For simplicity, re-embed all paths that were synced
					// A more precise approach would track which paths the graph sync touched
					if _, eErr := p.EmbedAll(context.Background()); eErr != nil {
						fmt.Fprintf(os.Stderr, "golem: warning: embed sync failed: %v\n", eErr)
					} else {
						fmt.Fprintf(os.Stderr, "golem: embeddings synced\n")
					}
					embedder.Close()
				}
			}
		}
		eStore.Close()
	}
}
```

**Note:** This is a simple approach that re-embeds everything. A better approach would track which paths the graph `Sync()` touched and only re-embed those. This optimization can be done later — for now, re-embedding all is acceptable since it's fast (~5-10 seconds for a 500-node graph).

**Step 2: Add imports**

```go
import (
	// existing imports...
	"context"
	"github.com/lofari/golem/internal/graph/embed"
)
```

**Step 3: Build and test**

```bash
CGO_ENABLED=1 go build ./...
CGO_ENABLED=1 go test ./... -count=1
```

Expected: All pass

**Step 4: Commit**

```bash
git add internal/runner/builder.go
git commit -m "feat(runner): auto-sync embeddings at session start"
```

---

### Task 10: Markdown parser

Parse markdown files into document/section structure.

**Files:**
- Create: `internal/graph/markdown/parser.go`
- Create: `internal/graph/markdown/parser_test.go`

**Step 1: Write failing test**

```go
// internal/graph/markdown/parser_test.go
package markdown

import "testing"

func TestParseMarkdown(t *testing.T) {
	content := []byte(`# My Project

This is the intro about ` + "`StartServer`" + ` and ` + "`Config`" + `.

## Installation

Run ` + "`go build`" + `.

## Usage

Call ` + "`StartServer`" + ` with a ` + "`Config`" + ` struct.

### Advanced Usage

See ` + "`internal/graph/store.go`" + ` for details.
`)

	sections, err := ParseMarkdown("README.md", content)
	if err != nil {
		t.Fatal(err)
	}

	if len(sections) != 4 {
		t.Fatalf("expected 4 sections, got %d", len(sections))
	}

	// Check first section
	if sections[0].Heading != "My Project" {
		t.Errorf("section 0 heading = %q, want %q", sections[0].Heading, "My Project")
	}
	if sections[0].Level != 1 {
		t.Errorf("section 0 level = %d, want 1", sections[0].Level)
	}
	if sections[0].Line != 1 {
		t.Errorf("section 0 line = %d, want 1", sections[0].Line)
	}
	// Check refs
	if len(sections[0].Refs) != 2 {
		t.Fatalf("section 0 refs = %v, want [StartServer Config]", sections[0].Refs)
	}
	if sections[0].Refs[0] != "StartServer" || sections[0].Refs[1] != "Config" {
		t.Errorf("section 0 refs = %v", sections[0].Refs)
	}

	// Check section 2 (Usage)
	if sections[2].Heading != "Usage" {
		t.Errorf("section 2 heading = %q, want %q", sections[2].Heading, "Usage")
	}
	if sections[2].Level != 2 {
		t.Errorf("section 2 level = %d, want 2", sections[2].Level)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/graph/markdown/ -v -count=1 -run TestParseMarkdown
```

Expected: FAIL — package doesn't exist

**Step 3: Implement markdown parser**

```go
// internal/graph/markdown/parser.go
package markdown

import (
	"regexp"
	"strings"
)

// DocSection represents a section of a markdown document.
type DocSection struct {
	Heading string   // Section heading text
	Level   int      // H1=1, H2=2, etc.
	Line    int      // 1-indexed line number
	Body    string   // Section content (between this heading and next)
	Refs    []string // Backtick-quoted identifiers found in body
}

var (
	headingRe  = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	backtickRe = regexp.MustCompile("`([^`]+)`")
)

// ParseMarkdown splits a markdown file into sections at heading boundaries.
// Each section captures its heading, level, body text, and backtick-quoted references.
func ParseMarkdown(path string, content []byte) ([]DocSection, error) {
	lines := strings.Split(string(content), "\n")

	var sections []DocSection
	var currentBody strings.Builder
	currentIdx := -1

	for i, line := range lines {
		if m := headingRe.FindStringSubmatch(line); m != nil {
			// Close previous section
			if currentIdx >= 0 {
				body := strings.TrimSpace(currentBody.String())
				sections[currentIdx].Body = body
				sections[currentIdx].Refs = extractRefs(body)
			}

			// Start new section
			sections = append(sections, DocSection{
				Heading: strings.TrimSpace(m[2]),
				Level:   len(m[1]),
				Line:    i + 1, // 1-indexed
			})
			currentIdx = len(sections) - 1
			currentBody.Reset()
		} else if currentIdx >= 0 {
			currentBody.WriteString(line)
			currentBody.WriteString("\n")
		}
	}

	// Close last section
	if currentIdx >= 0 {
		body := strings.TrimSpace(currentBody.String())
		sections[currentIdx].Body = body
		sections[currentIdx].Refs = extractRefs(body)
	}

	return sections, nil
}

// extractRefs finds all backtick-quoted identifiers in text.
// Filters out common non-code references (shell commands, file extensions, etc.)
func extractRefs(text string) []string {
	matches := backtickRe.FindAllStringSubmatch(text, -1)
	seen := map[string]bool{}
	var refs []string
	for _, m := range matches {
		ref := m[1]
		// Skip things that look like shell commands or paths
		if strings.ContainsAny(ref, " /\\") {
			continue
		}
		if !seen[ref] {
			seen[ref] = true
			refs = append(refs, ref)
		}
	}
	return refs
}
```

**Step 4: Run test**

```bash
CGO_ENABLED=1 go test ./internal/graph/markdown/ -v -count=1 -run TestParseMarkdown
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/graph/markdown/
git commit -m "feat(graph): add markdown parser for documentation graph"
```

---

### Task 11: Documentation graph builder

Integrate markdown parsing into the graph builder to create doc/section nodes and edges.

**Files:**
- Modify: `internal/graph/builder.go` (add doc indexing)
- Modify: `internal/graph/builder_test.go` (add test for doc indexing)

**Step 1: Write failing test**

Add to `internal/graph/builder_test.go`:

```go
func TestBuildFullWithDocs(t *testing.T) {
	dir := t.TempDir()

	// Create a Go source file
	os.MkdirAll(filepath.Join(dir, "cmd"), 0o755)
	os.WriteFile(filepath.Join(dir, "cmd", "main.go"), []byte(`package main

func StartServer() {}
`), 0o644)

	// Create a markdown file that references the function
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# My Project\n\nUse `StartServer` to start.\n\n## Setup\n\nRun the server.\n"), 0o644)

	// Init git repo for builder
	exec.Command("git", "-C", dir, "init").Run()
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "init").Run()

	dbPath := filepath.Join(dir, "graph.db")
	store, err := graph.OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	b := graph.NewBuilder(store)
	if err := b.BuildFull(dir); err != nil {
		t.Fatal(err)
	}

	// Check document node exists
	docNodes, err := store.NodesByType("document")
	if err != nil {
		t.Fatal(err)
	}
	if len(docNodes) != 1 {
		t.Fatalf("expected 1 document node, got %d", len(docNodes))
	}
	if docNodes[0].Name != "README.md" {
		t.Errorf("doc name = %q, want %q", docNodes[0].Name, "README.md")
	}

	// Check section nodes
	secNodes, err := store.NodesByType("section")
	if err != nil {
		t.Fatal(err)
	}
	if len(secNodes) != 2 {
		t.Fatalf("expected 2 section nodes, got %d", len(secNodes))
	}

	// Check REFERENCES edge exists (README references StartServer)
	edges, err := store.EdgesOfType("sec:README.md:My Project", "REFERENCES")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range edges {
		if strings.Contains(e.To, "StartServer") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected REFERENCES edge to StartServer, edges = %v", edges)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/graph/ -v -count=1 -run TestBuildFullWithDocs
```

Expected: FAIL — no document nodes created

**Step 3: Implement doc indexing in builder**

In `internal/graph/builder.go`, add an import for the markdown package:

```go
import (
	// existing imports...
	"github.com/lofari/golem/internal/graph/markdown"
)
```

Add a new method to the Builder:

```go
// indexDocs walks markdown files, parses them, and creates document/section nodes with edges.
func (b *Builder) indexDocs(projectPath string, existingNodes []Node) error {
	// Build name->node lookup for code linking
	nameIndex := map[string][]Node{}
	for _, n := range existingNodes {
		if n.Type == "function" || n.Type == "method" || n.Type == "type" {
			nameIndex[n.Name] = append(nameIndex[n.Name], n)
		}
	}

	var docNodes []Node
	var docEdges []Edge

	err := filepath.WalkDir(projectPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			for _, skip := range skipDirs {
				if base == skip {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		relPath, _ := filepath.Rel(projectPath, path)
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		sections, err := markdown.ParseMarkdown(relPath, content)
		if err != nil {
			return nil
		}
		if len(sections) == 0 {
			return nil
		}

		// Create document node
		docID := fmt.Sprintf("doc:%s", relPath)
		docNodes = append(docNodes, Node{
			ID:   docID,
			Type: "document",
			Name: filepath.Base(relPath),
			Path: relPath,
			Line: 1,
		})

		for _, sec := range sections {
			secID := fmt.Sprintf("sec:%s:%s", relPath, sec.Heading)
			docNodes = append(docNodes, Node{
				ID:   secID,
				Type: "section",
				Name: sec.Heading,
				Path: relPath,
				Line: sec.Line,
			})

			// CONTAINS edge: document -> section
			docEdges = append(docEdges, Edge{From: docID, To: secID, Type: "CONTAINS"})

			// REFERENCES edges: section -> code symbols found in backtick refs
			for _, ref := range sec.Refs {
				if targets, ok := nameIndex[ref]; ok {
					for _, target := range targets {
						docEdges = append(docEdges, Edge{From: secID, To: target.ID, Type: "REFERENCES"})
					}
				}
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	if len(docNodes) > 0 {
		return b.store.InsertBatch(docNodes, docEdges)
	}
	return nil
}
```

Then modify `BuildFull` to call `indexDocs` after the main code indexing. After the `InsertBatch` call for code nodes (around line 94), add:

```go
// Index documentation
if err := b.indexDocs(projectPath, allNodes); err != nil {
	return fmt.Errorf("index docs: %w", err)
}
```

Where `allNodes` is the slice of code nodes that was just inserted. You may need to refactor `BuildFull` to keep the `allNodes` slice accessible at that point.

Similarly, in `Sync`, after processing changed files, re-index docs for changed `.md` files:

```go
// Re-index changed markdown files
for _, f := range changedFiles {
	if strings.HasSuffix(f, ".md") {
		b.store.DeleteByPath(f)
	}
}
// Get current code nodes for linking
allCodeNodes := []Node{}
for _, typ := range []string{"function", "method", "type"} {
	nodes, _ := b.store.NodesByType(typ)
	allCodeNodes = append(allCodeNodes, nodes...)
}
if err := b.indexDocs(projectPath, allCodeNodes); err != nil {
	return fmt.Errorf("re-index docs: %w", err)
}
```

**Step 4: Run test**

```bash
CGO_ENABLED=1 go test ./internal/graph/ -v -count=1 -run TestBuildFullWithDocs
```

Expected: PASS

**Step 5: Run all tests**

```bash
CGO_ENABLED=1 go test ./... -count=1
```

Expected: All pass

**Step 6: Commit**

```bash
git add internal/graph/builder.go internal/graph/builder_test.go
git commit -m "feat(graph): add documentation graph with doc/section nodes and code linking"
```

---

### Task 12: Extend graph status with doc stats

**Files:**
- Modify: `cmd/graph.go` (show doc node counts)

**Step 1: Update status output**

In the `graphStatusCmd` handler, the existing code already shows node type breakdown via `stats.NodeTypes`. Since document and section nodes are now regular node types, they'll appear automatically in the output. Verify this.

If the output needs formatting adjustment (e.g., separate "Code" vs "Docs" sections), modify the status display:

```go
// After existing node stats
docCount := stats.NodeTypes["document"]
secCount := stats.NodeTypes["section"]
if docCount > 0 || secCount > 0 {
	fmt.Printf("\nDocumentation: %d documents, %d sections\n", docCount, secCount)
}
```

**Step 2: Build and test**

```bash
CGO_ENABLED=1 go build ./...
CGO_ENABLED=1 go test ./... -count=1
```

Expected: All pass

**Step 3: Commit**

```bash
git add cmd/graph.go
git commit -m "feat(cli): show documentation stats in golem graph status"
```

---

### Task 13: Integration test — end-to-end

Run the full pipeline on golem's own codebase to verify everything works together.

**Step 1: Build the graph**

```bash
CGO_ENABLED=1 go build -o golem .
./golem graph build
./golem graph status
```

Expected: Shows code nodes + document/section nodes. README.md and docs/ markdown files should be indexed.

**Step 2: Generate embeddings**

```bash
./golem graph embed
./golem graph status
```

Expected: Shows embedding count and model info alongside graph stats.

**Step 3: Verify all tests pass**

```bash
CGO_ENABLED=1 go test ./... -count=1
```

Expected: All pass

**Step 4: Final commit if any fixes needed**

If any adjustments were made during integration testing, commit them:

```bash
git add -A
git commit -m "fix(graph): integration test fixes for Phase 2 & 3"
```
