# Phase 2 & 3 Implementation Handoff

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

## Status

**Branch:** `feat/knowledge-graph` (11 commits ahead of main)
**Plan file:** `docs/plans/2026-03-08-knowledge-graph-phase2-3.md` (14 tasks, Tasks 0-13)
**Design doc:** `docs/plans/2026-03-08-knowledge-graph-phase2-3-design.md`

### Completed
- **Task 0:** Dependencies added (hugot v0.6.4, sqlite-vec-go-bindings v0.1.6). Build passes.

### Not Started (Tasks 1-13)
All remaining tasks from the plan need implementation. Start with Task 1.

## Key API Findings (from exploring hugot source code)

These findings are CRITICAL — the plan's code snippets are approximate. Use these exact APIs:

### hugot API (v0.6.4)
```go
import (
    "github.com/knights-analytics/hugot"
    "github.com/knights-analytics/hugot/pipelines"
)

// Session creation — use NewGoSession (pure Go, no ONNX Runtime needed, no build tags)
session, err := hugot.NewGoSession()
defer session.Destroy()

// Model download — saves to destination/BAAI_bge-small-en-v1.5/
modelPath, err := hugot.DownloadModel("BAAI/bge-small-en-v1.5", "./models/", hugot.NewDownloadOptions())

// Pipeline creation — generic function, explicit type parameter required
config := hugot.FeatureExtractionConfig{
    ModelPath:    modelPath,
    Name:         "golem-embedder",
    OnnxFilename: "model.onnx",
}
pipeline, err := hugot.NewPipeline[*pipelines.FeatureExtractionPipeline](session, config)

// Embed text — returns FeatureExtractionOutput with Embeddings [][]float32
result, err := pipeline.RunPipeline([]string{"text1", "text2"})
embeddings := result.Embeddings // [][]float32, one vector per input

// Optional: normalize embeddings
config.Options = []hugot.FeatureExtractionOption{pipelines.WithNormalization()}
```

### sqlite-vec API
```go
import sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"

// Enable sqlite-vec globally (call once, e.g. in init())
sqlite_vec.Auto()

// Serialize vectors for insertion
blob, err := sqlite_vec.SerializeFloat32(vector)

// SQL patterns:
// CREATE VIRTUAL TABLE IF NOT EXISTS vec_embeddings USING vec0(node_id TEXT PRIMARY KEY, embedding float[384]);
// INSERT OR REPLACE INTO vec_embeddings(node_id, embedding) VALUES (?, ?)  -- pass blob
// SELECT node_id, distance FROM vec_embeddings WHERE embedding MATCH ? AND k = ? ORDER BY distance  -- pass blob
```

### Import cycle note
`EmbeddingEntry` and `SimilarResult` types should live in `internal/graph/store.go` (not in `embed` package) to avoid import cycles, since `embed/pipeline.go` needs to import `graph.Store`.

## Build & Test Commands
```bash
CGO_ENABLED=1 go build ./...
CGO_ENABLED=1 go test ./... -count=1
```

## What to Do
1. Read the plan: `docs/plans/2026-03-08-knowledge-graph-phase2-3.md`
2. Use `superpowers:executing-plans` skill
3. Start from Task 1 (Embedder interface)
4. Execute in batches of 3, verify between batches
5. After Task 13, use `superpowers:finishing-a-development-branch` to merge
