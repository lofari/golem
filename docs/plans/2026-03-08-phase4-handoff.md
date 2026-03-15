# Phase 4: Git History Graph — Brainstorming Handoff

> **For Claude:** Use superpowers:brainstorming to explore this feature before writing a plan.

## Branch

`feat/knowledge-graph` — Phase 1-3 complete (structural graph, embeddings, documentation graph). All tests pass.

## What Exists

The knowledge graph currently has:

- **Node types:** file, function, method, type, document, section
- **Edge types:** DEFINES, CALLS, IMPORTS, USES, CONTAINS, REFERENCES
- **Storage:** SQLite at `.ctx/graph.db` with `nodes`, `edges`, `graph_meta` tables + `vec_embeddings` virtual table
- **Builder:** `internal/graph/builder.go` — `BuildFull` (full rebuild) and `Sync` (incremental via git diff)
- **MCP tools:** `find_callers`, `find_dependencies`, `find_dependents`, `graph_query`, `semantic_search`
- **CLI:** `golem graph build`, `golem graph status`, `golem graph embed`

## Phase 4 Vision (from design doc)

Add repository evolution data to the graph:

- **New node types:** Commit, Author
- **New edge types:** MODIFIES (commit→file), INTRODUCES (commit→function), AUTHORED_BY (commit→author), CO_CHANGED (files frequently modified together)
- **Data sources:** `git log`, `git blame`, `git diff`
- **New MCP tools:** `find_recent_changes`, `find_author`, `find_co_changed`
- **Enables:** "who owns this module?", "which commit introduced this bug?", "what files change together?"

## Questions to Explore During Brainstorming

### Scope & Node Design
- How many commits to index? All history? Last N? Since a date? Configurable?
- Should Author nodes use email, name, or both as ID? How to handle multiple emails for the same person?
- Do we need a separate `commit` table or can commits be regular `nodes`? Commit metadata (message, date, stats) is richer than typical nodes.
- Should CO_CHANGED be computed eagerly at build time or lazily on query?

### Data Extraction
- `git log --format` can get commit metadata efficiently. What format?
- `git blame` is expensive per-file. When to run it? Only on demand? Cache results?
- INTRODUCES edges (commit→function): requires diffing ASTs between commits — is this feasible or too expensive? Alternative: approximate via line ranges from `git blame`.
- How to handle merge commits, rebases, squash commits?

### Incremental Sync
- Current `Sync` uses `git diff --name-only` against `last_commit`. Git history sync would need to index new commits since `last_commit`. How does this interact with the existing sync flow?
- Should git history indexing be part of `golem graph build` or a separate `golem graph history` command?

### MCP Tool Design
- `find_recent_changes`: what's "recent"? Last N commits? Time range? Per-file or global?
- `find_author`: return commit history per author, or ownership percentages?
- `find_co_changed`: threshold for "frequently modified together"? Minimum co-change count?
- Any other tools that would be valuable? `find_blame` (who last touched this line)?

### Performance
- Large repos can have 10k+ commits. What's the storage/performance impact?
- Should we limit indexing depth by default and let users opt into full history?
- git operations can be slow — should indexing be async/background?

### Integration with Existing Graph
- MODIFIES edges connect commits to existing file nodes. What if a file was renamed/moved?
- Do we create edges from commits to function/method nodes (INTRODUCES), or only to files?
- How does this interact with the documentation graph? Commits that modify docs?

## Key Files to Read

- `internal/graph/store.go` — Store schema, node/edge types, query methods
- `internal/graph/builder.go` — BuildFull, Sync, git helpers (gitHeadSHA, gitChangedFiles, gitDirtyFiles)
- `internal/graph/model/model.go` — Node, Edge, Stats types
- `internal/mcp/graph_tools.go` — existing MCP tool patterns
- `cmd/graph.go` — existing CLI commands

## Build & Test

```bash
CGO_ENABLED=1 go build ./...
CGO_ENABLED=1 go test ./... -count=1
```
