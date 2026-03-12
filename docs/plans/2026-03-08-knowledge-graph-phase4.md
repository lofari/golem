# Phase 4: Git History Graph — Design

## Decisions

| Question | Decision |
|----------|----------|
| Storage model | Dedicated `commits` and `authors` tables |
| History depth | Last 500 commits (configurable via `--depth`) |
| Blame / INTRODUCES | Skip; MODIFIES edges only (commit→file) |
| CO_CHANGED | Eager computation at build time |
| CLI integration | Part of `golem graph build` (no separate command) |
| Author identity | Email as unique ID, latest name as display |
| Code structure | Separate `HistoryBuilder` in `history.go` |
| MCP tools | `find_recent_changes`, `find_file_history`, `find_co_changed` |

## Schema

Two new tables in `graph.db`:

```sql
CREATE TABLE IF NOT EXISTS commits (
    sha          TEXT PRIMARY KEY,
    message      TEXT NOT NULL,
    author_email TEXT NOT NULL,
    timestamp    INTEGER NOT NULL,  -- unix epoch
    additions    INTEGER DEFAULT 0,
    deletions    INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS authors (
    email TEXT PRIMARY KEY,
    name  TEXT NOT NULL
);
```

New edge types in the existing `edges` table:

- **MODIFIES**: `commit:<sha>` → `file:<path>` — commit modified this file
- **AUTHORED_BY**: `commit:<sha>` → `author:<email>` — who authored the commit
- **CO_CHANGED**: `file:<pathA>` → `file:<pathB>` — files frequently modified together

The `edges` table gains an optional `weight` column (INTEGER, nullable) for CO_CHANGED edge counts. Existing edges leave it NULL.

New `graph_meta` keys:

- `history_depth` — configured max commits
- `history_last_sha` — last indexed commit SHA for incremental sync

## Data Extraction Pipeline

### HistoryBuilder

New type in `internal/graph/history.go`:

```go
type HistoryBuilder struct {
    store *Store
    depth int  // max commits, default 500
}
```

### Full Build

1. Run `git log --format='%H%n%ae%n%an%n%at%n%s' -n 500 --name-only` in one invocation.
   - Output format: SHA, author email, author name, unix timestamp, subject line, followed by modified file paths, separated by blank lines between commits.
   - Parse output in a single pass.

2. For each commit:
   - Insert into `commits` table.
   - Upsert into `authors` table (latest name wins for a given email).

3. Create edges:
   - **MODIFIES**: `commit:<sha>` → `file:<path>` for each modified file that exists as a node in the graph.
   - **AUTHORED_BY**: `commit:<sha>` → `author:<email>`.

4. Compute **CO_CHANGED**:
   - SQL query over MODIFIES edges: find pairs of files that appear together in the same commit.
   - Filter by minimum co-occurrence count (default: 3).
   - Store as CO_CHANGED edges with `weight` = co-occurrence count.
   - Canonical direction: lower path lexicographically as `from_node`.

5. Record `history_last_sha` and `history_depth` in `graph_meta`.

### Incremental Sync

1. Read `history_last_sha` from `graph_meta`.
2. Run `git log <last_sha>..HEAD --format=... --name-only` to get only new commits.
3. Insert new commits, authors, MODIFIES, AUTHORED_BY edges.
4. Recompute CO_CHANGED from scratch (drop existing CO_CHANGED edges, recompute from all MODIFIES edges). This is fast since it's pure SQL over local data.
5. Update `history_last_sha`.

## MCP Tools

Three new tools, following the existing pattern in `graph_tools.go`:

### `find_recent_changes`

Find recent commits that touched a file or directory.

- **Input**: `path` (file or directory path), `limit` (default 10, max 50)
- **Query**: Join `commits` + MODIFIES edges where target file matches `path` (exact match for files, prefix match for directories).
- **Returns**: `[{sha, message, author, timestamp, files}]` ordered by timestamp descending.

### `find_file_history`

Get the commit history for a specific file.

- **Input**: `file` (file path), `limit` (default 20, max 100)
- **Query**: MODIFIES edges pointing to this file node, joined with `commits` table.
- **Returns**: `[{sha, message, author, timestamp}]` in chronological order (newest first).

### `find_co_changed`

Find files that frequently change together with a given file.

- **Input**: `file` (file path), `min_count` (default 3)
- **Query**: CO_CHANGED edges from/to this file node, filtered by `weight >= min_count`.
- **Returns**: `[{file, count}]` ordered by count descending.

## CLI Changes

### `golem graph build`

- Unchanged command surface, but now calls `HistoryBuilder.Build()` after code+docs indexing.
- New `--depth` flag (default 500) controls how many commits to index.
- Status output adds: `"golem: history — %d commits, %d authors, %d co-change pairs"`.

### `golem graph status`

- Adds history statistics section:
  ```
  History: 500 commits, 12 authors
  Co-change pairs: 47
  ```

## File Layout

| File | Change |
|------|--------|
| `internal/graph/history.go` | New — `HistoryBuilder` with `Build()` and `Sync()` |
| `internal/graph/history_test.go` | New — tests with fixture git repo |
| `internal/graph/store.go` | Add `commits`/`authors` table schema, `weight` column on edges, new query methods |
| `internal/mcp/graph_tools.go` | Add 3 new tool registrations + handlers |
| `cmd/graph.go` | Add `--depth` flag, update build/status output |

## Integration with Existing Graph

- MODIFIES edges reference existing `file:<path>` node IDs. If a file doesn't exist in the graph (deleted or not indexed), the edge is still created — the commit data is preserved even if the file node is gone.
- Renamed files: each path is treated independently. Git's rename detection is not used — the old path and new path appear as separate MODIFIES edges on the same commit.
- `Builder.BuildFull()` calls `HistoryBuilder.Build()` as its final step.
- `Builder.Sync()` calls `HistoryBuilder.Sync()` as its final step.

## Performance

- 500 commits: single `git log` invocation, ~100ms on typical repos.
- CO_CHANGED computation: single SQL aggregation query, negligible.
- Storage: ~500 commits + ~5000 MODIFIES edges + ~100 CO_CHANGED edges ≈ minimal DB growth.
- Full rebuild is idempotent: tables are cleared and repopulated.
