# Phase 7: Context Engine Design

## Overview

Smart context pre-fetching that uses the knowledge graph to build a ranked "context map" for each agent iteration. Instead of injecting raw code, the engine tells the agent *where to look* and *what symbols matter*, so it explores efficiently rather than burning tokens on discovery.

## Design Decisions

- **Context map, not code injection** — Symbol-level pointers with relationships, not raw code snippets. The agent decides what to read.
- **Fully automatic** — Task text analysis via semantic search + graph signals. No user annotations required.
- **On by default** — Always generated when embeddings exist. Opt out with `--no-context-map` or config.
- **Rebuilt each iteration** — Fresh map every iteration based on current task and latest graph state.
- **Fixed limit** — Top 15 symbols by default, configurable via `context_map_limit`.

## Package Structure

New package: `internal/graph/context/`

```go
package context

type ContextMap struct {
    Task    string
    Symbols []SymbolEntry
}

type SymbolEntry struct {
    Name      string
    Kind      string   // function, method, type, file
    Path      string
    Line      int
    Relations []string // e.g. ["calls CheckPassword", "called by LoginHandler"]
}

func BuildContextMap(store *graph.Store, embedder embed.Embedder, task ctx.Task, limit int) (*ContextMap, error)

func (cm *ContextMap) Format() string
```

### Format Output

```markdown
## Relevant Context

The following symbols are relevant to your current task. Start here.

- `ValidateCredentials` function (auth/login.go:45) — calls CheckPassword, called by LoginHandler
- `SessionMiddleware` method (middleware/session.go:12) — calls ValidateToken
- `CheckPassword` function (auth/crypto.go:30) — called by ValidateCredentials
```

## Ranking Pipeline

Multi-stage pipeline producing a ranked symbol list:

1. **Semantic search** — Query graph embeddings with task text, get top 30 candidates. Score: cosine similarity (0-1).
2. **Structural expansion** — For each candidate, fetch 1-hop callers/callees/dependents. Score: 0.7 x parent's semantic score.
3. **Co-change boost** — If candidate's file has co-change edges with other candidates, boost score by 0.1 per co-change link.
4. **Recency boost** — If file was modified in last 10 commits, boost by 0.15 (decaying by commit distance).
5. **Failure boost** — If symbol has failing tests or errors in last session, boost by 0.2.
6. **Deduplicate and rank** — Merge duplicates (keep highest score), sort descending, take top N.

Weights are package constants — easy to tune but not user-configurable.

## Integration

### Builder Loop

```
graph.Sync()
    |
context.BuildContextMap(store, embedder, currentTask, limit)   <- NEW
    |
RenderPrompt(dir, template, vars)   <- vars.ContextMap populated
    |
runner.Run(ctx, dir, prompt, ...)
```

### Changes Required

1. **`builder.go`** — After graph sync, call `BuildContextMap`. Pass result into `PromptVars`.
2. **`prompt.go`** — Add `ContextMap string` field to `PromptVars`. Replace `{{CONTEXT_MAP}}` in template.
3. **`templates/prompt.md`** — Add `{{CONTEXT_MAP}}` placeholder after the iteration context section.
4. **`config/`** — Add `context_map_limit` (int, default 15) and `context_map` (bool, default true).
5. **`cmd/code.go`** — Add `--no-context-map` flag.

### Graceful Degradation

When the graph has no embeddings (first run, or `golem graph build` not yet run with `--embed`), `BuildContextMap` returns an empty map. No error, no prompt section injected.

## Testing

Tests in `internal/graph/context/context_test.go`, stdlib `testing` only:

1. **`TestBuildContextMap_SemanticMatch`** — Known nodes + embeddings, verify semantic matches rank highest.
2. **`TestBuildContextMap_StructuralExpansion`** — Verify callers/callees of matches appear with reduced scores.
3. **`TestBuildContextMap_CoChangeBoost`** — Co-change edges boost ranking.
4. **`TestBuildContextMap_RecencyBoost`** — Recently modified symbols rank higher.
5. **`TestBuildContextMap_FailureBoost`** — Failing tests/errors boost ranking.
6. **`TestBuildContextMap_Limit`** — Output respects configured limit.
7. **`TestBuildContextMap_NoEmbeddings`** — Empty map returned gracefully.
8. **`TestContextMap_Format`** — Markdown output format correct.
9. **`TestBuildContextMap_Deduplication`** — Same symbol via multiple paths yields single entry.

Integration test in `builder_test.go`: verify `{{CONTEXT_MAP}}` replacement in rendered prompt.
