# Knowledge Graph Phase 2 & 3 Design

## Overview

Phase 2 adds semantic vector embeddings to the code knowledge graph, enabling natural-language queries like "find authentication logic." Phase 3 adds a documentation graph that extracts structure from markdown files and links it to code. Both phases share the embedding infrastructure — doc sections are embedded alongside code nodes for unified semantic search.

## Phase 2: Embedding Layer

### Architecture

```
golem graph embed            golem code / review / qa
      |                              |
      v                              v
 Embed Pipeline               MCP Server
      |                              |
+-----+-----+              semantic_search tool
|           |                        |
Embedder IF  sqlite-vec              v
|           |              query vec_embeddings
ONNX Provider               + resolve nodes
(hugot + BGE)
      |
      v
 vec_embeddings in graph.db
```

### Embedder Interface

```go
// internal/graph/embed/embedder.go
package embed

type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dimensions() int
    Close() error
}
```

Abstract interface allows future providers (API-based, different models) without changing consumers.

### ONNX Provider

**Stack:** `knights-analytics/hugot` + BGE-small-en-v1.5 (ONNX int8 quantized, ~32MB)

```go
// internal/graph/embed/onnx.go
type ONNXEmbedder struct {
    session *hugot.Session
    pipe    *pipelines.FeatureExtractionPipeline
    dims    int
}

func NewONNXEmbedder(modelDir string) (*ONNXEmbedder, error)
func (e *ONNXEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error)
func (e *ONNXEmbedder) Dimensions() int  // 384
func (e *ONNXEmbedder) Close() error
```

- Model stored at `~/.config/golem/models/bge-small-en-v1.5-int8/`
- Auto-downloaded from HuggingFace on first use
- Batch inference: default 32 texts per call
- ~5-20ms per embedding on CPU

### Model Management

```go
// internal/graph/embed/manager.go
func EnsureModel(modelName string) (string, error)  // returns path to model dir
func DefaultModelDir() string                         // ~/.config/golem/models/
```

Downloads model.onnx, tokenizer.json, config.json if not already cached. Progress bar during download.

### Storage Schema

sqlite-vec virtual table in graph.db, created alongside existing schema:

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS vec_embeddings USING vec0(
    node_id TEXT PRIMARY KEY,
    embedding float[384]
);
```

Metadata tracked in existing `graph_meta` table:
- `embed_model` — model name (e.g. "bge-small-en-v1.5-int8")
- `embed_last_indexed` — RFC3339 timestamp
- `embed_node_count` — number of embedded nodes

### Store API Additions

```go
func (s *Store) InsertEmbedding(nodeID string, vector []float32) error
func (s *Store) InsertEmbeddingsBatch(entries []EmbeddingEntry) error
func (s *Store) SearchSimilar(queryVec []float32, limit int) ([]SimilarResult, error)
func (s *Store) DeleteEmbeddingsByPath(path string) error
func (s *Store) ClearEmbeddings() error

type EmbeddingEntry struct {
    NodeID string
    Vector []float32
}

type SimilarResult struct {
    NodeID   string
    Distance float32
    Node     Node
}
```

### What Gets Embedded

| Node Type | Text Representation |
|-----------|-------------------|
| function | `"Function {name} in {path}: {signature}"` |
| method | `"Method ({receiver}) {name} in {path}: {signature}"` |
| type | `"Type {name} in {path} with fields: {field list}"` |
| file | `"File {path} imports: {imports}. Defines: {exported symbols}"` |

Text is built from source bytes available during tree-sitter extraction. Enriched with path context for better embedding quality.

### MCP Tool: semantic_search

```json
{
  "name": "semantic_search",
  "input": {
    "query": "authentication and password validation",
    "limit": 10,
    "types": ["function", "method"]
  },
  "output": [
    { "name": "ValidatePassword", "path": "auth/password.go", "line": 15, "type": "function", "score": 0.87 },
    { "name": "HashPassword", "path": "auth/password.go", "line": 42, "type": "function", "score": 0.82 }
  ]
}
```

- Embeds query text using the same Embedder
- Queries sqlite-vec for top-k similar vectors
- Optional `types` filter applied post-search
- Returns resolved node details with similarity scores (1.0 = identical, 0.0 = unrelated)
- Embedder loaded once per MCP server lifetime (not per-call)

### CLI Commands

**`golem graph embed`** — Generate embeddings for all graph nodes
- Requires graph.db (run `golem graph build` first)
- Loads embeddable nodes, builds text representations, batches through ONNX
- Progress: `Embedding 557 nodes... [=====>    ] 320/557`
- Stores in vec_embeddings, updates graph_meta

**`golem graph embed --sync`** — Incremental: only embed changed nodes
- Same git-diff approach as `graph.Sync()`

**`golem graph status`** — Extended output:
```
Graph: 557 nodes, 2397 edges
Embeddings: 423 nodes embedded (model: bge-small-en-v1.5-int8)
Last embedded: 2026-03-08T14:30:00Z
```

### Auto-Embed at Session Start

After graph sync in builder.go, run incremental embed for changed nodes. Only if embeddings already exist (don't auto-trigger initial full embed — that requires explicit `golem graph embed`).

---

## Phase 3: Documentation Graph

### Node Types

| Type | ID Format | Example |
|------|-----------|---------|
| `document` | `doc:{path}` | `doc:README.md` |
| `section` | `sec:{path}:{heading}` | `sec:docs/architecture.md:Graph Builder` |

### Edge Types

| Edge | Meaning | Example |
|------|---------|---------|
| `CONTAINS` | Document contains section | `doc:README.md CONTAINS sec:README.md:Installation` |
| `REFERENCES` | Section references code symbol | `sec:README.md:Usage REFERENCES fn:cmd/graph.go:graphBuildCmd` |
| `EXPLAINS` | Document explains a code file | `doc:docs/graph.md EXPLAINS file:internal/graph/store.go` |

### Markdown Parser

```go
// internal/graph/markdown/parser.go
package markdown

type DocNode struct {
    Heading string   // Section heading text
    Level   int      // H1=1, H2=2, etc.
    Line    int      // Start line number
    Body    string   // Section content text
    Refs    []string // Backtick-quoted identifiers found in body
}

func ParseMarkdown(path string, content []byte) ([]DocNode, error)
```

Simple heading + backtick scanner:
- Splits document at heading boundaries (`# `, `## `, etc.)
- Extracts section body text between headings
- Finds backtick-quoted identifiers: `` `StartServer` ``, `` `Store.InsertBatch` ``

### Code Linking

For each backtick reference in a section:
1. Look up identifier in `nodes` table by name
2. Exactly one match -> REFERENCES edge
3. Multiple matches -> REFERENCES to all (caller disambiguates by path)
4. No match -> skip

EXPLAINS edges: created when a doc file path corresponds to a code directory pattern (e.g., `docs/graph.md` EXPLAINS files in `internal/graph/`).

### Builder Integration

```go
// In builder.go, after tree-sitter extraction:
func (b *Builder) indexDocs(projectPath string) error
```

- Walks `*.md` files (skips .ctx/, .git/, node_modules/)
- Parses each markdown file into DocNode list
- Creates document + section nodes
- Creates CONTAINS edges (document -> sections)
- Resolves backtick refs against existing code nodes -> REFERENCES edges
- Detects file path patterns -> EXPLAINS edges

Integrated into `BuildFull()` and `Sync()` — no separate command needed.

### Documentation Embeddings

Section nodes get embedded using the same pipeline as code nodes.

Text for embedding: `"Section '{heading}' in {path}: {body first 500 chars}"`

`semantic_search` returns both code and documentation results, ranked together by relevance. The `types` filter supports `document` and `section` alongside code types.

### CLI

`golem graph build` — Extended to also index markdown files
`golem graph status` — Shows doc node counts:
```
Graph: 580 nodes (557 code, 23 docs), 2420 edges
```

---

## Dependencies

| Dependency | Purpose | Phase |
|---|---|---|
| `knights-analytics/hugot` | ONNX inference pipeline (tokenizer + model + pooling) | 2 |
| `asg017/sqlite-vec-go-bindings/cgo` | sqlite-vec for vector search in SQLite | 2 |
| ONNX Runtime shared library | Required by hugot at runtime | 2 |
| BGE-small-en-v1.5 ONNX model | Embedding model (~32MB int8) | 2 |

Phase 3 adds no new external dependencies — markdown parsing is hand-rolled or uses a lightweight Go library.

## Performance Expectations

- Full embed of 500-node graph: ~5-10 seconds (batch inference)
- Incremental embed of 10 changed nodes: <1 second
- Semantic search query: <100ms (embed query + sqlite-vec KNN)
- Markdown indexing of 20 doc files: <1 second
- Full build with docs: adds ~0.5 seconds to existing build time
