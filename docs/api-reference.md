# golem API Reference

The golem HTTP/WebSocket server exposes project state, managed processes, knowledge graph queries, and real-time event streams. It is consumed by the Flutter desktop GUI and can be used directly from scripts or external tooling.

---

## Server Startup

```
golem serve [--addr <host:port>]
```

**Flag**

| Flag | Default | Description |
|------|---------|-------------|
| `--addr` | `:8314` | TCP listen address |

On startup, if the current working directory contains a `.ctx/` folder it is automatically registered as a project. The server handles CORS (`*`) for all origins.

```bash
golem serve
# golem serve: registered project at /home/alice/myapp
# golem serve: listening on :8314
```

---

## REST Endpoints

All endpoints return `application/json`. Error responses have the shape `{"error": "<message>"}`.

### Health

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/health` | Liveness check. Returns `{"status":"ok"}`. |

### Projects

A **project** is a directory with a `.ctx/` folder. Each project gets a stable ID derived from the SHA-256 hash of its absolute path (first 8 bytes, hex-encoded).

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/projects` | List all registered projects. |
| `POST` | `/api/projects` | Register a new project by path. |
| `GET` | `/api/projects/{id}/state` | Read the project's `.ctx/state.yaml`. |
| `GET` | `/api/projects/{id}/log` | Read the project's `.ctx/log.yaml`. |
| `GET` | `/api/projects/{id}/config` | Read the merged project config (global + project layer). |
| `PUT` | `/api/projects/{id}/config` | Write the project-layer config to `.ctx/config.yaml`. |

**POST /api/projects — request body**

```json
{ "path": "/absolute/path/to/project" }
```

Response (201): `{"id": "<project-id>"}`

**GET /api/projects — response**

```json
[
  {
    "id": "a3f8c1d2",
    "path": "/home/alice/myapp",
    "name": "myapp",
    "phase": "build"
  }
]
```

### Global Config

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/config` | Read the global config (`~/.config/golem/config.yaml`). |
| `PUT` | `/api/config` | Write the global config. Body: a `Config` JSON object. |

### Processes

A **process** is a golem subcommand (`code`, `review`, `qa`, or `plan`) launched inside a project directory under a PTY. Output is buffered (up to 1 MB) and streamed over WebSocket.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/projects/{id}/processes` | Launch a new golem process. |
| `GET` | `/api/projects/{id}/processes` | List processes for a project. |
| `DELETE` | `/api/projects/{id}/processes/{procId}` | Stop a running process (sends cancellation). |

**POST /api/projects/{id}/processes — request body**

```json
{
  "command": "code",
  "config": {
    "maxIterations": 10,
    "maxToolCalls": 50,
    "model": "claude-opus-4-5",
    "task": "add pagination to the API",
    "sandbox": false,
    "mcp": true,
    "parallel": 1,
    "pluginDir": "/path/to/plugins"
  }
}
```

Valid commands: `code`, `review`, `qa`, `plan`.

Response (201): `{"id": "<proc-id>"}`

**GET /api/projects/{id}/processes — response**

```json
[
  {
    "id": "a3f8c1d2-code-1741000000000",
    "command": "code",
    "status": "running",
    "startedAt": "2026-03-15T10:00:00Z",
    "pid": 12345
  }
]
```

Process `status` values: `running`, `stopped`, `failed`.

### Diff

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/projects/{id}/diff` | Return git diff summary or a single file patch. |

**Query parameters**

| Parameter | Description |
|-----------|-------------|
| `ref` | Base git ref to diff against (default: `HEAD`) |
| `file` | If set, return the unified patch for this file path only |

Without `file`: returns a diff summary object from `git diff --stat`.
With `file`: returns `{"patch": "<unified diff>"}`.

### Graph

The knowledge graph must be built first with `golem graph build`. All graph endpoints require the project graph database at `.ctx/graph.db`.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/projects/{id}/graph/related` | Find nodes related to a named symbol. |
| `POST` | `/api/projects/{id}/graph/search` | Semantic (embedding) search over graph nodes. |
| `GET` | `/api/projects/{id}/graph/runtime-path` | Retrieve execution traces or failure paths. |
| `GET` | `/api/projects/{id}/graph/stats` | Graph statistics (node/edge counts, embedding info). |
| `GET` | `/api/projects/{id}/graph/context-map` | Build a context map for a given task description. |

**GET /graph/related — query parameters**

| Parameter | Default | Description |
|-----------|---------|-------------|
| `name` | (required) | Symbol or node name to look up |
| `direction` | `all` | `callers`, `dependencies`, `dependents`, or `all` |
| `depth` | `1` | Traversal depth |

**POST /graph/search — request body**

```json
{ "query": "authentication middleware", "limit": 10, "types": ["function", "file"] }
```

**GET /graph/runtime-path — query parameters**

| Parameter | Default | Description |
|-----------|---------|-------------|
| `session` | | Session ID to trace |
| `mode` | `trace` | `trace` (full path) or `failures` (error nodes only) |
| `command_filter` | | Filter by command name |

**GET /graph/context-map — query parameters**

| Parameter | Default | Description |
|-----------|---------|-------------|
| `task` | (required) | Natural-language task description |
| `limit` | `15` | Maximum number of context nodes to return |

**GET /graph/stats — response**

```json
{
  "totalNodes": 1240,
  "totalEdges": 3876,
  "nodeTypes": {"function": 820, "file": 120},
  "edgeTypes": {"calls": 2100, "imports": 900},
  "embeddingCount": 1240,
  "embedModel": "all-minilm-l6-v2",
  "lastIndexed": "2026-03-15T09:30:00Z",
  "coChangeCount": 54,
  "executionCount": 312
}
```

---

## WebSocket Protocol

Both WebSocket endpoints accept a standard HTTP upgrade. Messages are JSON text frames using the `WSMessage` schema.

### WSMessage schema

```json
{
  "type":    "string",
  "data":    "string (base64-encoded PTY bytes, output messages)",
  "cols":    0,
  "rows":    0,
  "code":    0,
  "state":   {},
  "session": {},
  "event":   {},
  "error":   "string"
}
```

Fields are omitted when not relevant to the message type.

### Process Stream

```
GET /api/projects/{id}/processes/{procId}/stream   (WebSocket)
```

Streams PTY output from a managed process. On connect, any buffered output (up to 1 MB) is replayed as a single initial `output` message.

**Server → client message types**

| `type` | Fields | Description |
|--------|--------|-------------|
| `output` | `data` (base64) | PTY output chunk. Decode from base64 to get raw terminal bytes. |
| `exit` | `code` | Process exited. `code` is `0` for clean exit, `1` for failure. |

**Client → server message types**

| `type` | Fields | Description |
|--------|--------|-------------|
| `input` | `data` (base64) | Keyboard input to write to the PTY. |
| `resize` | `cols`, `rows` | Resize the PTY window. |

### State Watch

```
GET /api/projects/{id}/watch   (WebSocket)
```

Watches `.ctx/state.yaml`, `.ctx/log.yaml`, and all run directories under `.ctx/runs/` for changes. An initial `state_changed` message is sent immediately on connect. State and log change notifications are debounced by 200 ms. Engine events from run `log.json` files are delivered immediately without debounce.

**Server → client message types**

| `type` | Fields | Description |
|--------|--------|-------------|
| `state_changed` | `state` | Full project state object; fired when `state.yaml` changes. |
| `log_appended` | `session` | The most recent session entry appended to `log.yaml`. |
| `engine_event` | `event` | Raw `EngineEvent` JSON object from a run's `log.json`. |
| `error` | `error` | Watcher-level error message. |

---

## EngineEvent Schema

Engine events are emitted to `{runDir}/log.json` (NDJSON) during blueprint pipeline execution and forwarded to State Watch subscribers as `engine_event` messages. Not all fields are present in every event.

```json
{
  "type":        "string",
  "timestamp":   "2026-03-15T10:05:00Z",
  "run-id":      "run-20260315-100500-a1b2c3",
  "agent":       "string",
  "goal":        "string",
  "step":        "string",
  "step-type":   "string",
  "status":      "string",
  "duration-ms": 0,
  "line":        "string",
  "predicate":   "string",
  "iteration":   0,
  "max":         0,
  "reason":      "string",
  "error-type":  "string",
  "action":      "string",
  "attempt":     0
}
```

**`type` values**

| Type | Key fields | Description |
|------|-----------|-------------|
| `pipeline-start` | `agent`, `goal`, `run-id` | Pipeline execution began. |
| `pipeline-end` | `status`, `duration-ms` | Pipeline finished. `status` is `success` or `error`. |
| `step-start` | `step`, `step-type` | A pipeline step started. |
| `step-end` | `step`, `status`, `duration-ms` | A pipeline step finished. |
| `loop-enter` | `predicate`, `iteration`, `max` | A `while` loop body entered for an iteration. |
| `loop-exit` | `predicate`, `reason` | Loop exited. `reason` is `false` (predicate) or `max` (iteration cap). |
| `conditional-skip` | `predicate` | A `when` block skipped because predicate was false. |
| `error-occurred` | `step`, `error-type`, `action` | An unrecoverable error halted a step. |
| `error-retry` | `step`, `error-type`, `attempt`, `action` | A step is being retried. |

---

## Examples

```bash
BASE=http://localhost:8314

# Health check
curl $BASE/api/health

# List registered projects
curl $BASE/api/projects

# Register a project
curl -X POST $BASE/api/projects \
  -H "Content-Type: application/json" \
  -d '{"path":"/home/alice/myapp"}'
# => {"id":"a3f8c1d2"}

# Read project state
curl $BASE/api/projects/a3f8c1d2/state

# Read session log
curl $BASE/api/projects/a3f8c1d2/log

# Git diff summary against main
curl "$BASE/api/projects/a3f8c1d2/diff?ref=main"

# Git diff for a single file
curl "$BASE/api/projects/a3f8c1d2/diff?ref=main&file=internal/runner/engine.go"

# Launch a code run with a task
curl -X POST $BASE/api/projects/a3f8c1d2/processes \
  -H "Content-Type: application/json" \
  -d '{"command":"code","config":{"task":"add pagination","maxIterations":5}}'
# => {"id":"a3f8c1d2-code-1741000000000"}

# List processes for a project
curl $BASE/api/projects/a3f8c1d2/processes

# Stop a process
curl -X DELETE $BASE/api/projects/a3f8c1d2/processes/a3f8c1d2-code-1741000000000

# Graph stats
curl $BASE/api/projects/a3f8c1d2/graph/stats

# Find callers of a function
curl "$BASE/api/projects/a3f8c1d2/graph/related?name=RunStep&direction=callers&depth=2"

# Semantic search
curl -X POST $BASE/api/projects/a3f8c1d2/graph/search \
  -H "Content-Type: application/json" \
  -d '{"query":"error retry logic","limit":5}'

# Context map for a task
curl "$BASE/api/projects/a3f8c1d2/graph/context-map?task=add+rate+limiting&limit=10"
```

**Connecting to the process stream (wscat)**

```bash
wscat -c "ws://localhost:8314/api/projects/a3f8c1d2/processes/a3f8c1d2-code-1741000000000/stream"
```

**Watching project state (wscat)**

```bash
wscat -c "ws://localhost:8314/api/projects/a3f8c1d2/watch"
```
