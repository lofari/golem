# Phase 6: Agent Query Layer (REST API) Design

Expose graph queries via REST for Flutter UI and external tools. Three consolidated endpoints share query logic with MCP tools through a service layer.

## Architecture

Extract graph query logic from MCP handlers into `internal/graph/query/` service layer. Add 3 REST endpoints to the existing HTTP server under `/api/projects/{id}/graph/`. Both MCP tools and REST handlers become thin wrappers over the shared query service.

```
Flutter UI / external tools
        |
        v
  internal/server/graph.go     (REST handlers -- new)
        |
        v
  internal/graph/query/        (shared query service -- new)
        ^
        |
  internal/mcp/graph_tools.go  (MCP handlers -- refactored to use query service)
        |
        v
  internal/graph/store.go      (SQLite store -- unchanged)
```

## Endpoints

### GET /api/projects/{id}/graph/related

Traverse related code from a file or symbol.

Query params:
- `name` (required) -- file path or symbol name
- `direction` -- `callers`, `dependencies`, `dependents`, `all` (default: `all`)
- `depth` -- traversal depth, 1-5 (default: 1)

Response:
```json
{
  "nodes": [{ "id", "type", "name", "path", "line" }],
  "edges": [{ "from", "to", "type" }]
}
```

### POST /api/projects/{id}/graph/search

Semantic search over code and documentation.

JSON body:
- `query` (required) -- natural language search string
- `limit` -- max results, 1-50 (default: 10)
- `types` -- filter by node types (e.g. `["function", "method"]`)

Response:
```json
[{ "name", "path", "line", "type", "score" }]
```

### GET /api/projects/{id}/graph/runtime-path

Execution trace from agent sessions.

Query params:
- `session` -- session ID (default: latest)
- `mode` -- `trace` (full timeline) or `failures` (errors + failed tests only). Default: `trace`
- `command_filter` -- substring filter on commands

Response for `mode=trace`:
```json
{
  "sessionId": "...",
  "status": "...",
  "commands": [{
    "command": "go test ./...",
    "exitCode": 0,
    "filesAccessed": ["main.go"],
    "outputPreview": "..."
  }]
}
```

Response for `mode=failures`:
```json
{
  "sessionId": "...",
  "failures": [{
    "command": "go test ./...",
    "exitCode": 1,
    "errorMessage": "...",
    "stackTrace": "...",
    "filesInvolved": ["main.go"]
  }],
  "failedTests": [{
    "name": "TestFoo",
    "passed": false,
    "durationMs": 42,
    "output": "..."
  }]
}
```

## Query Service Layer

Package: `internal/graph/query/`

Three exported functions matching the endpoints:

- `Related(store, name, direction, depth) -> RelatedResult`
- `Search(store, embedder, query, limit, types) -> []SearchResult`
- `RuntimePath(store, session, mode, filter) -> RuntimeResult`

Each composes existing `Store` methods. MCP handlers get refactored to call these instead of hitting the store directly.

## Error Handling

- Missing graph DB: `404` with `{"error": "graph not found -- run golem graph build"}`
- Invalid params: `400` with `{"error": "..."}`
- Internal errors: `500` with `{"error": "..."}`

## Testing

- Unit tests for query service functions (no HTTP, just store)
- HTTP handler tests using `httptest` (consistent with existing server_test.go)
