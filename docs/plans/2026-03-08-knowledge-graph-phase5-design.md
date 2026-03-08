# Phase 5: Execution Graph (Runtime Intelligence)

## Goal

Track what agents do during `golem code`, `golem review`, and `golem qa` sessions. Capture shell commands, test results, errors, and link them to the existing static knowledge graph.

## Data Model

### New Node Types

| Type | ID Format | Fields |
|------|-----------|--------|
| `execution` | `exec:{session_id}` | session_id, started_at, ended_at, status |
| `command` | `cmd:{session_id}:{seq}` | command text, exit_code, working_dir |
| `test_result` | `test:{session_id}:{name}` | test_name, passed, duration_ms |
| `error` | `err:{session_id}:{seq}` | message, stack_trace |
| `output` | `out:{session_id}:{seq}` | stdout, stderr (truncated) |

### New Edge Types

| Edge | From → To | Meaning |
|------|-----------|---------|
| `RUNS` | execution → command | Session ran this command |
| `PRODUCES` | command → output | Command produced this output |
| `PRODUCES` | command → test_result | Command produced test results |
| `PRODUCES` | command → error | Command produced this error |
| `ACCESSES` | command → file node | Command referenced this file |
| `FAILS_IN` | error → file node | Stack trace points to this file |
| `TESTS` | test_result → fn/method node | Test exercises this function |

### Storage

Dedicated tables in the existing `.ctx/graph.db` (not the generic `nodes` table, since execution data is bulkier and has different lifecycle):

```sql
CREATE TABLE executions (
    session_id TEXT PRIMARY KEY,
    started_at INTEGER NOT NULL,
    ended_at   INTEGER,
    status     TEXT NOT NULL DEFAULT 'running'
);

CREATE TABLE commands (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL,
    seq         INTEGER NOT NULL,
    command     TEXT NOT NULL,
    exit_code   INTEGER,
    working_dir TEXT,
    FOREIGN KEY (session_id) REFERENCES executions(session_id)
);

CREATE TABLE outputs (
    command_id TEXT PRIMARY KEY,
    stdout     TEXT,
    stderr     TEXT,
    truncated  BOOLEAN DEFAULT FALSE,
    FOREIGN KEY (command_id) REFERENCES commands(id)
);

CREATE TABLE test_results (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL,
    name        TEXT NOT NULL,
    passed      BOOLEAN NOT NULL,
    duration_ms INTEGER,
    output      TEXT,
    FOREIGN KEY (session_id) REFERENCES executions(session_id)
);

CREATE TABLE errors (
    id          TEXT PRIMARY KEY,
    command_id  TEXT NOT NULL,
    message     TEXT NOT NULL,
    stack_trace TEXT,
    FOREIGN KEY (command_id) REFERENCES commands(id)
);
```

Cross-reference edges (ACCESSES, FAILS_IN, TESTS) go in the existing `edges` table, linking runtime nodes to static graph nodes.

## Capture Pipeline

### Approach: Parse Stream-JSON

No in-container instrumentation. Claude's `--output-format stream-json` already emits structured tool-use events, including bash commands with arguments, stdout, stderr, and exit codes. The `StreamParser` already processes this stream.

### Architecture

```
claude -p (stream-json)
        │
  StreamParser (existing)
        │
  ExecutionCollector (new)
   ├── filters bash tool-use events
   ├── creates command/output nodes
   └── delegates to RefExtractor
        │
  RefExtractor (new)
   ├── file path matching
   ├── Go/Python stack trace parsing
   ├── test result parsing
   └── creates edges to static graph
        │
  Store (existing, extended)
```

### ExecutionCollector

Receives parsed stream events from `StreamParser`. On each bash tool-use event:

1. Creates a `command` node with the command text and sequence number.
2. Waits for the tool result (stdout/stderr, exit code).
3. Creates an `output` node (truncated per rules below).
4. If exit code != 0, creates an `error` node from stderr.
5. Passes command text and output to `RefExtractor`.

### RefExtractor

Scans command text and output to find references to existing graph nodes:

- **File paths** — regex for project-relative paths (e.g., `internal/runner/builder.go`), validated against existing `file:` nodes in the graph.
- **Go stack traces** — patterns like `file.go:42` in goroutine dumps. Creates `FAILS_IN` edges to file nodes.
- **Go test output** — `--- FAIL: TestFoo` / `--- PASS: TestFoo` patterns. Creates `test_result` nodes and attempts `TESTS` edges to matching `fn:` nodes.
- **Python tracebacks** — `File "path.py", line N` patterns. Creates `FAILS_IN` edges.
- **Generic errors** — non-zero exit code with no recognized pattern: store last N lines of stderr as the error message.

File path matching is reliable. Function-level linking (TESTS edges) is best-effort.

### Output Truncation

- **Successful commands** (exit 0): first 50 + last 50 lines of stdout. Mark `truncated=true` if original exceeds 100 lines.
- **Failed commands** (exit != 0): store full stdout and stderr.

## Lifecycle Management

### Session Scoping

Each `golem code`/`review`/`qa` invocation creates one `execution` node. All commands within that session reference it.

### Pruning

On each new session start:

1. Count existing sessions in `executions` table.
2. If count exceeds the retention limit (default 5), delete the oldest sessions.
3. Cascade: delete commands, outputs, test_results, errors for pruned sessions.
4. Remove associated edges from the `edges` table.

### Configuration

```yaml
execution-history: 5  # sessions to retain, default 5
```

## MCP Tools

### `find_execution_failures`

- **Input:** optional `session` (default: latest), optional `file` (filter by file path)
- **Returns:** `[{command, exit_code, error_message, files_involved, stack_trace}]`
- **Answers:** "What failed in the last run?", "What errors involved auth.go?"

### `get_runtime_trace`

- **Input:** optional `session` (default: latest), optional `command_filter` (substring match)
- **Returns:** `{session_id, status, commands: [{command, exit_code, files_accessed, output_preview}]}`
- **Answers:** "What happened during the last session?", "What test commands ran?"

### `find_test_results`

- **Input:** optional `session` (default: latest), optional `status` ("passed"/"failed"/"all"), optional `name` (substring filter)
- **Returns:** `[{name, passed, duration_ms, output, exercises: [{function, path, line}]}]`
- **Answers:** "Which tests failed?", "Which tests exercise the Login function?"

### Existing Tool Enhancement

The new edge types (`ACCESSES`, `FAILS_IN`, `TESTS`) are stored in the standard `edges` table, so `graph_query` works with them automatically. For example, `graph_query` with `edge_types: ["TESTS"]` on a function node answers "which tests exercise this function?" with no extra code.

## Integration Points

### Builder Loop

In `runner/builder.go`, the builder loop already processes stream events. The `ExecutionCollector` plugs in as an additional consumer. No changes to the `CommandRunner` interface.

### Stream-JSON Requirement

Execution capture requires `StreamJSON: true` on the runner. When `.ctx/graph.db` exists, golem should enable stream-json automatically for all agent commands (`code`, `review`, `qa`).

### Graph Status

`golem graph status` should report execution stats: sessions retained, total commands captured, failure count.

## New Files

| File | Purpose |
|------|---------|
| `internal/graph/execution/collector.go` | Stream event consumer |
| `internal/graph/execution/collector_test.go` | Collector tests |
| `internal/graph/execution/refs.go` | File/stack-trace/test extraction |
| `internal/graph/execution/refs_test.go` | Extraction tests |
| `internal/graph/execution/prune.go` | Session pruning |
| `internal/graph/execution/prune_test.go` | Pruning tests |

Plus extensions to:
- `internal/graph/store.go` — new table schemas, insert/query/prune methods
- `internal/mcp/graph_tools.go` — three new tool registrations
- `internal/runner/builder.go` — wire up ExecutionCollector
- `cmd/graph.go` — execution stats in `golem graph status`

## What We Are Not Building

- No in-container shim or command wrapper — stream-json provides sufficient data.
- No syscall tracing — too invasive for the value it delivers.
- No real-time execution dashboard — data is queryable after the fact via MCP tools.
- No vector embeddings for execution nodes — runtime data is transient and not worth embedding.
