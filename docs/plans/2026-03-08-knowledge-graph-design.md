# Golem Knowledge Graph — Phase 1 Design

## Overview

Golem introduces a Code Knowledge Graph that represents static code structure as a queryable graph. Agents access it via MCP tools during sessions to perform impact analysis, dependency traversal, and code navigation — without reading entire files.

Phase 1 delivers the structural foundation: tree-sitter AST extraction, SQLite storage, and MCP query tools.

## Architecture

```
golem graph build          golem code / review / qa
      |                           |
      v                           v
 Graph Builder              Builder Loop
      |                           |
      v                       (auto-sync)
 tree-sitter parse                |
      |                           v
      v                      MCP Server
 SQLite (.ctx/graph.db)    +-----+------+
      ^                    |            |
      |               find_callers  find_deps
      +--------------------+
```

### Components

- `internal/graph/` — schema, storage, builder, queries
- `internal/graph/treesitter/` — tree-sitter parsing, language-agnostic extraction
- `internal/mcp/` — graph query tools added to existing MCP server
- `cmd/graph.go` — `golem graph build` and `golem graph status` commands

### Storage

SQLite at `.ctx/graph.db`. Sits alongside existing state.yaml and log.yaml. Already mounted rw in warden sandboxes.

## SQLite Schema

```sql
CREATE TABLE nodes (
    id   TEXT PRIMARY KEY,   -- e.g. "fn:server.go:StartServer"
    type TEXT NOT NULL,       -- file, function, class, method, package, interface, variable, test
    name TEXT NOT NULL,
    path TEXT,                -- file path relative to project root
    line INTEGER,             -- start line number
    metadata JSON
);

CREATE TABLE edges (
    from_node TEXT NOT NULL,
    to_node   TEXT NOT NULL,
    type      TEXT NOT NULL,   -- CONTAINS, IMPORTS, DEFINES, CALLS, EXTENDS, IMPLEMENTS, RETURNS, USES
    metadata  JSON,
    PRIMARY KEY (from_node, to_node, type)
);

CREATE TABLE graph_meta (
    key   TEXT PRIMARY KEY,
    value TEXT
);

CREATE INDEX idx_edges_from ON edges(from_node);
CREATE INDEX idx_edges_to ON edges(to_node);
CREATE INDEX idx_edges_type ON edges(type);
CREATE INDEX idx_nodes_type ON nodes(type);
CREATE INDEX idx_nodes_path ON nodes(path);
```

**Node ID format:** `{type}:{path}:{name}` — deterministic, so re-indexing produces the same IDs.

**graph_meta** tracks `last_commit` (SHA) and `last_indexed` (timestamp) for incremental sync.

## Tree-sitter Extraction

Language-agnostic AST parsing using `github.com/smacker/go-tree-sitter`.

### How it works

1. **Language detection** — file extension maps to tree-sitter grammar (`.go` -> Go, `.py` -> Python, `.ts` -> TypeScript, `.gd` -> GDScript, etc.)
2. **Parse file** — tree-sitter produces a syntax tree
3. **Extract nodes** — walk the tree with language-specific `.scm` query files
4. **Extract edges** — analyze call expressions, import statements, type references

### Query files

Each language gets a tree-sitter query file that maps AST node types to graph node types:

```scheme
;; Go example (queries/go.scm)
(function_declaration name: (identifier) @function.name) @function.def
(call_expression function: (identifier) @call.name) @call.expr
(import_spec path: (interpreted_string_literal) @import.path)
```

```scheme
;; Python example (queries/python.scm)
(function_definition name: (identifier) @function.name) @function.def
(call (identifier) @call.name) @call.expr
(import_from_statement module_name: (dotted_name) @import.path)
```

### Extraction output per file

- **Nodes:** file, functions, classes, methods, interfaces, variables, tests
- **Edges:** DEFINES (file->symbol), CALLS (function->function), IMPORTS (file->package), CONTAINS (class->method), EXTENDS, IMPLEMENTS, RETURNS, USES

### Unsupported languages

Files without a grammar are indexed as file nodes only (no internal structure). The graph still tracks them via IMPORTS edges from other files.

### Package structure

```
internal/graph/treesitter/
    parser.go          -- parse file, return AST
    extractor.go       -- walk AST, produce nodes + edges
    queries/           -- embedded .scm query files per language
        go.scm
        python.scm
        typescript.scm
        gdscript.scm
        ...
```

## Graph Builder & Sync

Two modes: full build and incremental sync.

### Full build (`golem graph build`)

1. Walk project directory, skip `.ctx/`, `.git/`, `node_modules/`, `vendor/`, etc.
2. For each source file: parse with tree-sitter, extract nodes + edges
3. Write to SQLite in a single transaction (wipe and rebuild)
4. Store current HEAD commit SHA in `graph_meta`

### Incremental sync (auto, at session start)

1. Builder loop reads `graph_meta.last_commit`
2. Run `git diff --name-only <last_commit>..HEAD` to get changed files
3. For each changed file: delete old nodes/edges for that path, re-extract
4. For deleted files: remove their nodes and dangling edges
5. Update `last_commit` in `graph_meta`

### Edge cases

- No `.ctx/graph.db` exists -> auto-trigger full build on first session
- No git repo -> full rebuild every time (no diff baseline)
- Dirty working tree -> sync includes uncommitted changes by scanning modified files from `git status`

### Performance expectations

- Full build of a 10k-file repo: ~5-10 seconds
- Incremental sync: sub-second for typical 5-20 changed files

### Builder interface

```go
type Builder struct {
    db     *Store
    parser *Parser
}

func (b *Builder) BuildFull(projectPath string) error
func (b *Builder) Sync(projectPath string) error
```

## MCP Query Tools

Four tools added to the existing MCP server.

### find_callers

"What calls this function?"

```json
Input:  { "name": "StartServer" }
Output: [{ "name": "main", "path": "cmd/main.go", "line": 12 }]
```

Traverses CALLS edges inward. Depth 1 by default, optional `depth` param for transitive callers.

### find_dependencies

"What does this file/function depend on?"

```json
Input:  { "path": "server.go" }
Output: { "imports": [...], "calls": [...], "uses_types": [...] }
```

Traverses IMPORTS, CALLS, USES edges outward.

### find_dependents

"What breaks if I change this?"

```json
Input:  { "path": "auth/password.go" }
Output: [{ "path": "auth/login.go", "via": "CALLS:ValidatePassword" }]
```

Reverse traversal — finds everything that depends on the given file or symbol.

### graph_query

General-purpose traversal.

```json
Input:  { "node": "fn:server.go:StartServer", "edge_types": ["CALLS", "USES"], "depth": 2, "direction": "outbound" }
Output: { "nodes": [...], "edges": [...] }
```

### Integration

Tools registered alongside existing MCP tools (`mark_task`, `set_phase`, etc.). Graph Store opened read-only during sessions.

## CLI Commands

### golem graph build

Full graph rebuild. Parses all source files, writes to `.ctx/graph.db`.

### golem graph status

Shows node/edge counts, last indexed commit, detected languages, database size.

## Dependencies

| Dependency | Purpose |
|---|---|
| `github.com/smacker/go-tree-sitter` | AST parsing (CGo) |
| `github.com/mattn/go-sqlite3` | SQLite storage (CGo) |
| Language grammars (go-tree-sitter subpackages) | Per-language parsing |

## Future Phases

These are deferred from Phase 1 but planned for future implementation.

### Phase 2: Embedding Layer

Semantic vector embeddings for code nodes. Enables natural-language queries like "find authentication logic."

- **Embedding model:** local model (BGE-small-en, 384 dimensions) via Python sidecar process
- **Sidecar architecture:** Go spawns a Python HTTP server on demand during indexing, talks over localhost, kills when done
- **Storage:** `embeddings` table in graph.db with `node_id TEXT, vector BLOB`, using `sqlite-vec` for vector search
- **Embedding targets:** functions (signature + body), classes, files, modules, documentation
- **Context builder:** enriches code with imports/file path before embedding for better quality
- **Chunking:** functions and classes embedded whole; files chunked at 200-400 lines
- **New MCP tool:** `semantic_search` — takes natural language query, returns ranked code nodes

### Phase 3: Documentation Graph

Extract knowledge from markdown, README, comments, and ADRs.

- **Nodes:** Document, Section, Concept
- **Edges:** EXPLAINS, REFERENCES, RELATES_TO
- **Links documentation to code:** README mentions AuthService -> `doc:README.md EXPLAINS module:AuthService`
- **Agents can combine** structural traversal with documentation to understand architectural intent

### Phase 4: Git History Graph

Repository evolution over time.

- **Nodes:** Commit, Author
- **Edges:** MODIFIES (commit->file), INTRODUCES (commit->function), AUTHORED_BY (commit->author)
- **Derived edges:** CO_CHANGED (files frequently modified together)
- **Sources:** `git log`, `git blame`, `git diff`
- **New MCP tools:** `find_recent_changes`, `find_author`, `find_co_changed`
- **Enables:** "who owns this module?", "which commit introduced this bug?", "what files change together?"

### Phase 5: Execution Graph (Runtime Intelligence)

Track what agents actually do inside Docker/warden sandboxes.

- **Nodes:** Execution, Container, Command, Test, Output, Error
- **Edges:** RUNS, EXECUTES, ACCESSES, FAILS_IN, PRODUCES
- **Capture:** commands executed, file access, stack traces, test results, stdout/stderr, exit codes
- **Instrumentation:** command wrapper inside sandbox that logs events
- **Links runtime to static graph:** Command ACCESSES File -> connects to existing file node
- **New MCP tools:** `find_execution_failures`, `get_runtime_trace`
- **Enables:** "what code caused this runtime error?", "which tests exercise this function?"

### Phase 6: Agent Query Layer (REST API)

Expose graph queries via REST for Flutter UI and external tools.

- `GET /api/graph/related` — related code traversal
- `POST /api/graph/search` — semantic search (requires Phase 2)
- `GET /api/graph/runtime-path` — execution trace (requires Phase 5)
- Shared query logic with MCP tools

### Phase 7: Context Engine

Smart context pre-fetching that uses the graph to build minimal, relevant context for each agent iteration.

- Analyzes current task + graph to determine which code the agent needs
- Injects relevant code snippets into the prompt automatically
- Reduces token usage by avoiding full-file reads
- Combines structural, semantic, and runtime data for optimal context

### Future Extensions (Post-Phase 7)

- **Architecture graph:** detect layers (API, Service, Repository, Database)
- **Test coverage graph:** connect tests to functions for gap analysis
- **Performance graph:** attach profiling data (latency, memory per function)
- **Learning graph:** track agent actions, fix success rates, failure patterns for self-improvement
