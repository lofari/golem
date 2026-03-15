# Golem Architecture Guide

This document describes the internal architecture of golem for contributors. It covers how the packages fit together, how the blueprint engine executes, how state persists across sessions, and how the server/UI communicate.

---

## 1. Overview

Golem is a Go CLI that orchestrates autonomous Claude Code loops with persistent state. At its core it wraps `claude -p` (Claude's non-interactive headless mode) in a structured iteration loop where each session receives rendered context from `.ctx/` files and writes structured results back to them.

There are two distinct orchestration engines:

**Blueprint engine** (current default, `engine: blueprint`): A YAML-defined pipeline executor. An *agent* is a YAML file that declares named steps, control flow (while/when/if), and error handling policies. The engine parses the blueprint, validates it, then executes each node in order, managing a typed pipeline state (`map[string]any`) that flows between steps. Each step invokes `claude -p` or a built-in primitive and reads output from `session-output.json`. This is the engine used by `golem code` and `golem run`.

**Legacy builder engine** (`engine: go`): An iteration loop that repeatedly calls `claude -p` against a rendered Markdown prompt, reading task status from `.ctx/state.yaml` after each iteration. This engine preceded the blueprint system and is still selectable via config.

Both engines share the `CommandRunner` interface for Claude invocation, the `.ctx/` directory layout for state, and the HTTP/WebSocket server for UI integration.

---

## 2. Package Map

### `cmd/`

Each file contains one Cobra command registered into `rootCmd` in `root.go`. The pattern is uniform: resolve config with `resolveConfig(cmd, dir)`, construct the appropriate runner config, handle signals with `signal.NotifyContext`, then call into `internal/runner`.

Key commands:
- `code.go` — Reads config, selects between blueprint engine and legacy builder, wires an `EngineEvent` channel, calls `runner.NewEngine(...).Run(ctx)` (blueprint) or `runner.RunBuilderLoop(ctx, cfg)` (legacy).
- `run.go` — Directly invokes the blueprint engine with an explicit agent name and `--goal` flag.
- `serve.go` / `ui.go` — Start the HTTP server; `ui.go` also launches the Flutter desktop app.
- `agents.go` — Lists built-in and project-local agents.
- `runs.go` — Lists or attaches to active/recent runs by reading `.ctx/runs/`.
- `mcp_serve.go` — Starts the MCP server over stdio for use as a Claude Code plugin.
- `graph.go` — Subcommands to build, embed, and query the knowledge graph.

Agent YAML files are resolved in `loadAgent()`: first from `.ctx/agents/<name>.yaml`, then from the embedded `templates/agents/` directory.

### `internal/runner/`

The largest package. Contains both the blueprint engine and the legacy builder loop.

**`blueprint.go`** — Defines all blueprint data structures (`Blueprint`, `Step`, `ControlFlowNode`, `PipelineNode`, `Pipeline`, `ErrorHandlers`) and the `ParseBlueprint(data []byte)` function. Parsing happens in two passes: a `yaml.Node` tree walk for strict field validation (catching unknown keys with typo suggestions), followed by structured unmarshalling. Predicate expressions are compiled with `ParsePredicateExpr` at parse time and cached in `parsedPredicates`.

**`engine.go`** — Defines `EngineConfig`, `EngineEvent`, and `Engine`. `NewEngine` generates a timestamped run ID. `Run` creates the run directory, sets the `current` symlink, opens `log.json`, then iterates over `pipeline.Nodes` calling `execNode`. Step execution flows through `execStep → runStep → execAgenticStep/execBuiltinStep/execShellStep`. Errors are classified and dispatched to `handleError → handleTransient/handleMalformedOutput` with retry loops. State changes are persisted after every successful step via `saveState`, which writes both a versioned snapshot (`state-NNN.json`) and a canonical `state.json`.

**`command.go`** — Defines the `CommandRunner` interface and its production implementation `ClaudeRunner`. `ClaudeRunner` builds `claude -p` invocations with `--dangerously-skip-permissions`, `--max-turns`, and optional `--mcp-config` and `--plugin-dir` flags. When `Sandbox: true`, it wraps the invocation in a `warden run` container. When `StreamJSON: true`, it uses `--output-format stream-json` and routes output through `StreamParser` for live event collection.

**`events.go`** — The legacy event system for the builder TUI. Defines `EventType` (an `int` enum: `EventIterStart`, `EventOutputLine`, `EventIterEnd`, `EventLoopDone`) and `Event`. These are sent over a `chan Event` from `RunBuilderLoop` to the caller and are separate from `EngineEvent`.

**`primitives.go`** — Built-in step implementations: `git-setup`, `lint`, `run-tests`, `ci-tests`, `create-pr`. Each returns a `PrimitiveResult` (`map[string]any`) written into pipeline state. Also defines the three typed error classes: `TransientError`, `UnrecoverableError`, `MalformedOutputError`.

**`predicates.go`** — Built-in named predicates evaluated against state or config: `needs-work`, `failed`, `lint-failed`, `ci-enabled`, `ci-failed`. Returns `(bool, bool)` where the second value indicates whether the name was recognized.

**`predicate_expr.go`** — Custom predicate expression parser. Expressions follow the form `path.to.key == "value"`. Paths prefixed with `config.` resolve against the agent config map; all others resolve against pipeline state. Supports `==`, `!=`, `>`, `<`, `>=`, `<=` for strings, numbers, and booleans.

**`builder.go`** — The legacy builder loop (`RunBuilderLoop`). Reads `.ctx/state.yaml` and `.ctx/log.yaml` each iteration, renders a Markdown prompt, calls `cfg.Runner.Run`, validates the resulting state, and emits `Event` values to the caller. Includes parallel task execution via `RunParallel` and strategy-based halting.

**`validate.go`** — Post-iteration validation for the legacy builder: schema validation with auto-repair (invalid phases and statuses are normalized), task regression detection (a task moving from `done` to any other status triggers a warning), and snapshot restore on corruption.

**`snapshot.go`** — State snapshot management for the legacy builder, writing `.ctx/snapshots/state-NNN.yaml` files.

**`stream.go`** — `StreamParser` for `--output-format stream-json` output. Exposes `OnBashCommand` and `OnBashResult` callback hooks used by the execution collector to record tool use into the knowledge graph.

**`parallel.go`** — Parallel task execution: clones the git worktree per task, runs independent sessions concurrently, then merges results.

**`strategy.go`** — Loop strategy evaluation: detects stalled tasks, repeated failures, and decides whether to halt, skip tasks, or inject context.

**`prompt.go`** — Prompt rendering for the legacy builder loop, using Go `text/template` over `.ctx/` Markdown templates.

### `internal/server/`

**`server.go`** — Defines `Server` with a `*http.ServeMux`. `routes()` registers all endpoints. CORS is applied via a middleware wrapper. The server tracks `project` records (path → state path mapping) and `managedProcess` records (active `claude` subprocesses with PTY output buffering).

Route table:

| Method | Path | Handler |
|---|---|---|
| GET | `/api/health` | Health check |
| GET | `/api/projects` | List registered projects |
| POST | `/api/projects` | Register a project by path |
| GET | `/api/projects/{id}/state` | Read `.ctx/state.yaml` |
| GET | `/api/projects/{id}/log` | Read `.ctx/log.yaml` |
| GET | `/api/projects/{id}/config` | Read project config |
| PUT | `/api/projects/{id}/config` | Update project config |
| GET | `/api/config` | Read global config |
| PUT | `/api/config` | Update global config |
| POST | `/api/projects/{id}/processes` | Launch a golem subprocess |
| GET | `/api/projects/{id}/processes` | List processes |
| DELETE | `/api/projects/{id}/processes/{procId}` | Stop a process |
| GET | `/api/projects/{id}/processes/{procId}/stream` | WebSocket: PTY stream |
| GET | `/api/projects/{id}/watch` | WebSocket: state/event watch |
| GET | `/api/projects/{id}/diff` | Git diff |
| GET | `/api/projects/{id}/graph/related` | Graph: related nodes |
| POST | `/api/projects/{id}/graph/search` | Graph: semantic search |
| GET | `/api/projects/{id}/graph/runtime-path` | Graph: execution path |
| GET | `/api/projects/{id}/graph/stats` | Graph: statistics |
| GET | `/api/projects/{id}/graph/context-map` | Graph: context map |

**`websocket.go`** — Two WebSocket handlers.

`handleProcessStream` streams PTY output from a managed process to the client. It first sends backlog as a single base64 chunk (`{"type":"output","data":"<base64>"}`) then subscribes to new chunks. Inbound messages handle `"input"` (base64-encoded keystrokes written to the PTY) and `"resize"` (terminal dimension changes). On process exit it sends `{"type":"exit","code":0}`.

`handleStateWatch` watches the `.ctx/` directory with `fsnotify`. State and log changes are debounced at 200 ms and sent as `{"type":"state_changed","state":{...}}` and `{"type":"log_appended","session":{...}}`. Changes to `log.json` files under `.ctx/runs/` are forwarded immediately (no debounce) as `{"type":"engine_event","event":{...}}`, using `tailLogJSON` to stream only new NDJSON lines since the last read position.

### `internal/ctx/`

Defines the canonical data model for project state.

**`state.go`** — `State` struct (`Project`, `Status`, `Decisions`, `Tasks`, `Pitfalls`). `ReadState` / `WriteState` operate on `.ctx/state.yaml`. Includes normalization: `NormalizeTaskStatuses` maps synonyms (`complete → done`, `wip → in-progress`, etc.) on every read; `NormalizePhase` similarly maps phase synonyms. `ValidateState` enforces that task statuses and phases are within the allowed sets.

**`log.go`** — `Log` struct containing a slice of `Session`. Each session records iteration number, timestamp, task name, outcome, summary, handoff note, files changed, decisions made, and pitfalls found. `ReadLog` / `WriteLog` / `AppendSession` operate on `.ctx/log.yaml`.

### `internal/config/`

**`config.go`** — Two-layer configuration system. `Config` struct holds all settings with YAML and JSON tags. Resolution order: built-in defaults (`Defaults()`) → global file (`~/.config/golem/config.yaml`) → project file (`.ctx/config.yaml`) → CLI flags (applied in `cmd/`). The `configLayer` type uses pointer fields to distinguish "set" from "unset", enabling precise merging via `merge()`. `SetValue` / `GetValue` provide key-based access for the `golem config` subcommands.

Notable config keys: `engine` (selects orchestrator), `agent` (default blueprint agent), `mcp` (enables MCP server), `lsp` (enables LSP servers), `context-map` (enables graph-based context injection), `sandbox` (enables warden container), `parallel` (max concurrent task sessions).

### `internal/graph/`

A SQLite-backed knowledge graph for code intelligence.

**`store.go`** — `Store` wraps a `*sql.DB`. The schema holds `nodes` (id, type, name, path, line), `edges` (from/to/type/weight), `graph_meta` (key-value), a `vec_embeddings` virtual table using the `sqlite-vec` extension for 384-dimension float32 vectors, and execution tables (`executions`, `commands`, `outputs`, `test_results`, `errors`).

Node types represent code entities (functions, types, packages, files). Edge types include structural relationships (`CALLS`, `IMPORTS`, `DEFINED_IN`) and historical ones (`CO_CHANGED` with a weight counting co-occurrence frequency in git history).

Sub-packages:
- `treesitter/` — Tree-sitter parsers extract function/type/symbol nodes and edges from source files.
- `lsp/` — LSP client manager that spawns and communicates with language servers (gopls, etc.) to provide definition, references, hover, and diagnostics.
- `embed/` — ONNX embedding pipeline (all-MiniLM-L6-v2, 384 dimensions) for semantic similarity search.
- `execution/` — Collects bash commands and results from `StreamParser` callbacks and stores them in the execution tables.
- `context/` — `BuildContextMap` ranks symbols by semantic similarity to the current task and formats them for prompt injection.
- `query/` — Higher-level graph query helpers (callers, dependencies, dependents, co-changed files).

### `internal/mcp/`

**`server.go`** — `GolemServer` wraps `mcp-go`'s `MCPServer`. `NewServer` registers all tools; `ServeStdio` runs the JSON-RPC server over stdin/stdout so Claude Code can call it as a plugin. Tool registration respects the `GOLEM_TOOLS` environment variable (a comma-separated allowlist) so the engine can restrict which tools are available per step.

**`tools.go`** — State mutation tools: `mark_task`, `set_phase`, `set_status`, `add_decision`, `add_pitfall`, `log_session`. All state mutations use `flock`-based file locking to prevent races when parallel sessions write concurrently.

Graph query tools (in `graph_tools.go`): `find_callers`, `find_dependencies`, `find_dependents`, `graph_query`, `semantic_search`, `find_co_changed`, `find_execution_failures`, `get_runtime_trace`, `find_test_results`.

LSP tools (in `lsp_tools.go`, only when an LSP manager is provided): `lsp_definition`, `lsp_references`, `lsp_hover`, `lsp_diagnostics`.

### `internal/display/`

Plain-text terminal formatters for state, log, and run summaries. Used by `golem status` and similar read-only commands.

### `internal/scaffold/`

`scaffold.go` implements `golem init`: creates `.ctx/` with `state.yaml`, `log.yaml`, and `config.yaml` from embedded templates. `CtxExists(dir)` is used by commands to gate against running without initialization.

### `internal/git/`

`git.go` provides helpers for reading changed files (`ChangedFiles`) and generating diff summaries (`DiffSummary`) used by the display layer and graph builder.

### `templates/`

All files are embedded via `//go:embed` in `embed.go` and exposed as `templates.FS`. Structure:
- `agents/` — Built-in blueprint YAML files (`build-feature.yaml`, `fix-bug.yaml`, `one-shot.yaml`).
- `prompts/` — Step prompt Markdown templates keyed by step name (`plan.md`, `implement.md`, `review.md`, `reflect.md`, `research.md`). An agentic step without an inline `prompt:` field looks up `prompts/<step-name>.md` from this directory.
- `prompt.md` — Legacy builder prompt template.
- `state.yaml`, `log.yaml`, `claude.md` — Scaffold templates for `golem init`.

### `ui/flutter/`

A Flutter desktop application that communicates with `golem serve` over HTTP and WebSocket.

Key providers (Riverpod):
- `projects.dart` — Manages the project list via `GET /api/projects`.
- `project.dart` — Holds state/log for the selected project.
- `processes.dart` — Manages active processes; bridges PTY stream to `xterm.dart`.
- `runs.dart` — Lists run directories and tracks `EngineEvent` streams.
- `connection.dart` — WebSocket connection lifecycle.
- `graph.dart` — Graph query results.

Key views: `project_workspace.dart` (main layout), `detail_panel.dart` (right panel with tabs), `detail_terminal.dart` (embedded terminal via `xterm.dart`), `detail_timeline.dart` (engine event timeline), `pipeline_progress.dart` (blueprint step progress), `graph_explorer.dart` (interactive graph view).

---

## 3. Blueprint Engine Flow

This section traces a complete `golem run build-feature --goal "..."` invocation through the engine.

### 3.1 Parse phase

```
loadAgent(name, dir)
  → os.ReadFile(".ctx/agents/<name>.yaml")   // project-local first
  → templates.FS.ReadFile("agents/<name>.yaml")  // fallback to embedded
  → ParseBlueprint(data)
      → yaml.Unmarshal into yaml.Node for field validation
      → validateTopLevelFields: reject unknown keys, suggest corrections
      → yaml.Unmarshal into Blueprint struct
      → parseSteps: walk yaml.Node sequence
          → for each item: isControlFlow? → parseControlFlowNode
                           else          → parseStepNode
      → validateErrorHandlers: check action values
      → parsePredicates: compile custom predicate expressions
  → bp.ValidateContracts()
      → walk all nodes in declaration order
      → track available keys (initial-state + engine builtins: branch, base)
      → assert each step's reads are available from prior writes
      → writes inside control flow nodes are "conditional" (must use optional-reads)
```

### 3.2 Engine construction

```go
e := runner.NewEngine(runner.EngineConfig{
    Dir:        dir,
    AgentName:  agentName,
    Goal:       goal,
    Blueprint:  bp,
    Config:     mergedConfig,   // agent defaults + CLI agent-opts
    Runner:     cr,             // *ClaudeRunner
    Model:      model,
    Events:     events,         // chan<- EngineEvent, buffered 100
    MCPEnabled: true,
    LSPEnabled: true,
})
```

`NewEngine` generates a run ID of the form `run-20060102-150405-<3-byte-hex>`.

### 3.3 Run

`e.Run(ctx)`:

1. Creates `.ctx/runs/<run-id>/sessions/` and sets `.ctx/runs/current` symlink.
2. Opens `.ctx/runs/<run-id>/log.json` for NDJSON event writing.
3. Calls `injectProjectContext` to seed state with project metadata from `.ctx/state.yaml`.
4. If `MCPEnabled`, calls `setupMCP` which writes `.ctx/mcp_servers.json` pointing the golem binary as the MCP server, then sets `ClaudeRunner.MCPConfig`.
5. Calls `syncGraph` to incrementally update the knowledge graph.
6. Sets up the execution collector if graph and streaming are both enabled.
7. Calls `e.saveState()` then emits `pipeline-start`.
8. Iterates `pipeline.Nodes`, calling `execNode` for each.
9. On completion emits `pipeline-end` with status and duration.

### 3.4 Node execution

```
execNode(node)
  → node.Step != nil       → execStep → runStep
  → node.ControlFlow != nil → execControlFlow
      → ControlWhile → execWhile: loop until predicate false or max
      → ControlWhen  → execWhen:  run body if predicate true
      → ControlIf    → execIf:    run then/else branches
```

`execSubNodes` handles bodies of control flow nodes; it walks the `SubNodes` slice (which carries full `PipelineNode` structs resolved at parse time) and falls back to string ref lookup via `resolveStepRef` for programmatically constructed pipelines.

Predicate evaluation checks `parsedPredicates` first (custom expressions from blueprint YAML), then `evalBuiltinPredicate` (the named predicate switch in `predicates.go`).

### 3.5 Step execution

`runStep(step)`:

1. Emits `step-start` with step name and type.
2. Dispatches by `step.Type`:
   - `agentic`: loads prompt template (inline `prompt:` or `templates/prompts/<name>.md`), renders it with `RenderStepPrompt` (substituting `${key}` tokens from state and config), calls `Runner.RunWithTools`, reads `session-output.json`, merges declared `writes` keys into state.
   - `builtin`: dispatches to a named primitive function, merges result into state.
   - `shell`: runs `sh -c <command>`, stores output and status in state.
3. Emits `step-end` with status and duration.
4. On success calls `saveState()`.

### 3.6 Error handling

`handleError` classifies the error type and calls `resolveErrorHandler` which applies a three-level priority chain:

```
step.errors.<type>          // step-level override
blueprint.errors.<type>     // blueprint-level override
defaultErrorHandlers[type]  // built-in defaults
```

Built-in defaults:
- `transient` → `retry`, max 3
- `malformed-output` → `re-run`, max 2
- `unrecoverable` → `halt`
- `contract-violation` → `halt`

On retry or re-run, `_error_context` is injected into state so the next prompt rendering can include the previous failure message. This key is deleted on success.

### 3.7 Event emission

`emit(ev EngineEvent)` timestamps the event, writes it as a JSON line to `log.json` (for persistence and `handleStateWatch` tailing), and sends it to the `Events` channel if non-nil. The channel send is non-blocking (uses `select/default`) to avoid stalling the engine if the consumer is slow.

---

## 4. Key Interfaces and Types

### CommandRunner

```go
type CommandRunner interface {
    Run(ctx context.Context, dir string, prompt string, maxTurns int, model string) (string, error)
    RunWithTools(ctx context.Context, dir string, prompt string, maxTurns int, model string, tools []string) (string, error)
}
```

`Run` is used by the legacy builder. `RunWithTools` is used by the blueprint engine; it passes a comma-separated tool list as the `GOLEM_TOOLS` environment variable, which the spawned MCP server reads to filter which tools it registers. The production implementation is `*ClaudeRunner`; tests use lightweight stubs.

### Engine (struct, not interface)

```go
type Engine struct {
    RunID    string
    cfg      EngineConfig
    state    map[string]any
    runDir   string
    stateVer int
    logFile  *os.File
}
```

Not an interface. Callers hold a `*Engine` directly. `State()` returns the current pipeline state map.

### EngineEvent

```go
type EngineEvent struct {
    Type      string    `json:"type"`
    Timestamp time.Time `json:"timestamp"`
    Step      string    `json:"step,omitempty"`
    StepType  string    `json:"step-type,omitempty"`
    Status    string    `json:"status,omitempty"`
    Duration  int64     `json:"duration-ms,omitempty"`
    Agent     string    `json:"agent,omitempty"`
    Goal      string    `json:"goal,omitempty"`
    RunID     string    `json:"run-id,omitempty"`
    Line      string    `json:"line,omitempty"`
    Predicate string    `json:"predicate,omitempty"`
    Iteration int       `json:"iteration,omitempty"`
    Max       int       `json:"max,omitempty"`
    Reason    string    `json:"reason,omitempty"`
    ErrorType string    `json:"error-type,omitempty"`
    Action    string    `json:"action,omitempty"`
    Attempt   int       `json:"attempt,omitempty"`
}
```

Event type strings: `pipeline-start`, `pipeline-end`, `step-start`, `step-end`, `loop-enter`, `loop-exit`, `conditional-skip`, `error-occurred`, `error-retry`.

### EngineConfig

Key fields:
- `Dir` — absolute path to the project root.
- `Blueprint` — parsed and validated `*Blueprint`.
- `Goal` — passed into initial state as `state["goal"]`.
- `Config` — merged agent config (`map[string]any`), available in prompt templates as `${config.<key>}`.
- `Runner` — `CommandRunner` implementation.
- `Events` — send-only channel for event delivery; nil disables channel delivery (events still go to `log.json`).
- `MCPEnabled`, `LSPEnabled` — control whether the MCP server and LSP managers are started.
- `GraphPath` — path to the SQLite knowledge graph; defaults to `.ctx/graph.db`.

---

## 5. State Management

### `.ctx/` directory layout

```
.ctx/
  state.yaml          # project state (tasks, decisions, status)
  log.yaml            # session history (legacy builder)
  config.yaml         # project-level config overrides
  graph.db            # SQLite knowledge graph (optional)
  mcp_servers.json    # generated per-session, points to golem binary
  sessions/           # raw session output (legacy builder)
    build-001.md
    build-002.md
  snapshots/          # state snapshots (legacy builder)
    state-001.yaml
  runs/               # blueprint engine run artifacts
    current -> run-20060102-150405-abc123/   # symlink to active run
    run-20060102-150405-abc123/
      log.json        # NDJSON EngineEvent stream
      state.json      # latest pipeline state
      state-001.json  # versioned state after step 1
      state-002.json  # versioned state after step 2
      sessions/       # (reserved for per-step session output)
  agents/             # project-local blueprint YAML files
    my-agent.yaml
```

### Pipeline state vs. project state

The blueprint engine maintains its own `map[string]any` pipeline state distinct from `.ctx/state.yaml`. This state is initialized with `{"goal": cfg.Goal}` plus any keys injected by `injectProjectContext`. Steps read from and write to this map through declared `reads`/`writes` contracts. Pipeline state is written to `.ctx/runs/<run-id>/state.json` after each successful step.

`.ctx/state.yaml` is the persistent project state used by the legacy builder and the MCP tools. The blueprint engine injects it into pipeline state at startup but does not write back to it directly; that is the responsibility of the Claude sessions using the MCP `mark_task`, `set_phase`, and related tools.

### Reserved state keys

`code`, `branch`, and `base` are engine-managed reserved keys. Steps cannot freely write to them; `git-setup` writes `branch` and `base` directly, and `execAgenticStep` writes `code` by calling `detectCodeChanges` when the step declares `writes: [code]`.

---

## 6. Event System

There are two separate event systems in the codebase that serve different purposes.

### Legacy events (builder TUI)

Defined in `internal/runner/events.go`. Used only by the legacy `RunBuilderLoop`:

```go
type EventType int
const (
    EventIterStart  EventType = iota
    EventOutputLine
    EventIterEnd
    EventLoopDone
)
type Event struct { Type EventType; Iter, MaxIter int; Task, Outcome, Line string; Err error; Result *BuilderResult }
```

Sent over `BuilderConfig.Events chan<- Event`. The Flutter UI receives these through a `managedProcess` PTY stream rather than directly from this channel. The channel is also used by the `golem code` command for its own terminal display.

### Engine events (blueprint engine)

Defined in `internal/runner/engine.go`. Used by the blueprint engine:

```go
type EngineEvent struct { Type string; Timestamp time.Time; Step, StepType, Status string; Duration int64; ... }
```

Sent over `EngineConfig.Events chan<- EngineEvent` AND written line-by-line to `.ctx/runs/<run-id>/log.json` as NDJSON. The server's `handleStateWatch` tails this file with `tailLogJSON` and forwards new lines over WebSocket as `{"type":"engine_event","event":{...}}`. The Flutter UI's `detail_timeline.dart` and `pipeline_progress.dart` render these events.

The two systems have different consumers. Legacy events drive the old TUI; engine events drive the new Flutter timeline. They co-exist because `golem code` with `engine: go` uses the legacy system while `engine: blueprint` uses the new one.

---

## 7. Server/UI Architecture

`golem serve` starts an HTTP server on `:8314` (configurable). `golem ui` additionally launches the Flutter desktop app with the server address as an argument.

The Flutter app connects to the server on startup. It uses the REST endpoints to fetch the project list and project state, and opens two persistent WebSocket connections per active project:

1. `/api/projects/{id}/watch` — receives `state_changed`, `log_appended`, and `engine_event` messages. Drives the state panel and timeline view.
2. `/api/projects/{id}/processes/{procId}/stream` — receives raw PTY output for the terminal emulator (`xterm.dart`). Sends user keystrokes and terminal resize events back to the server.

The `WSMessage` type is the wire format:

```go
type WSMessage struct {
    Type    string      `json:"type"`
    Data    string      `json:"data,omitempty"`    // base64 PTY bytes
    Cols    int         `json:"cols,omitempty"`
    Rows    int         `json:"rows,omitempty"`
    Code    *int        `json:"code,omitempty"`    // exit code on "exit"
    State   interface{} `json:"state,omitempty"`
    Session interface{} `json:"session,omitempty"`
    Event   interface{} `json:"event,omitempty"`
    Error   string      `json:"error,omitempty"`
}
```

Engine events flow from the `Engine` to the file system to the server to the Flutter UI without passing through any in-process channels after the engine exits. This means the UI can reconnect and replay events by re-tailing `log.json` from the stored offset.

---

## 8. Knowledge Graph

The graph subsystem is optional. It is built with `golem graph build` and updated incrementally at the start of each engine or builder run.

**Storage**: SQLite with the `sqlite-vec` extension for vector similarity search. The database at `.ctx/graph.db` holds code structure nodes and edges, embedding vectors (384-dimension float32), and execution history.

**Graph construction** (`internal/graph/builder.go`):
1. Walk source files, parse with tree-sitter parsers for supported languages.
2. Extract function, type, method, and file nodes.
3. Build structural edges (`CALLS`, `IMPORTS`, `DEFINED_IN`, `BELONGS_TO`).
4. Walk git log to build `CO_CHANGED` edges weighted by co-occurrence frequency.

**Embeddings** (`internal/graph/embed/`):
- Model: `all-MiniLM-L6-v2` (384 dimensions), downloaded on first use via `EnsureModel`.
- Embedder: ONNX Runtime via `embed.NewONNXEmbedder`.
- `embed.Pipeline.EmbedAll` generates embeddings for nodes that lack them.
- `store.SearchSimilar` runs approximate nearest-neighbor search using `sqlite-vec`.

**LSP integration** (`internal/graph/lsp/`):
- `Manager` spawns language servers (e.g., `gopls`) on demand.
- Tools exposed through MCP: `lsp_definition`, `lsp_references`, `lsp_hover`, `lsp_diagnostics`.
- LSP can be disabled per-step via `--no-lsp` or the `lsp: false` config key.

**Context map** (`internal/graph/context/`):
- `BuildContextMap` embeds the current task description, searches for semantically similar code nodes, and formats the result as a Markdown table injected into the prompt.
- Controlled by `context-map: true` and `context-map-limit: 15` config keys.

**Execution history** (`internal/graph/execution/`):
- `Collector` hooks into `StreamParser.OnBashCommand` and `OnBashResult` callbacks.
- Records every bash invocation (command, working directory, output, exit code) into the `commands` / `outputs` tables.
- MCP tools `find_execution_failures` and `get_runtime_trace` query this data.

---

## 9. MCP Integration

The MCP server is a separate process spawned by Claude Code using the config written to `.ctx/mcp_servers.json`:

```json
{
  "mcpServers": {
    "golem": {
      "command": "/path/to/golem",
      "args": ["mcp-serve", "--dir", "/path/to/project"]
    }
  }
}
```

`WriteMCPConfig` in `command.go` generates this file before each Claude invocation. The `golem mcp-serve` command starts the MCP server, which registers tools from `internal/mcp/`.

**Tool filtering**: The `GOLEM_TOOLS` environment variable (a comma-separated list) is set on the Claude subprocess by the engine. When the spawned MCP server starts, it reads this variable and registers only the listed tools. Per-step tool control in blueprint YAML (`tools: [semantic_search, find_callers]`) maps directly to this mechanism.

**State tools** provide structured write access to `.ctx/state.yaml` and `.ctx/log.yaml` with file locking, validating inputs before applying changes. This prevents Claude from corrupting the YAML by writing invalid status values or malformed structures.

---

## 10. Extension Points

### Adding a CLI command

Create `cmd/<name>.go`, define a `*cobra.Command`, and register it in an `init()` function with `rootCmd.AddCommand(cmd)`. The `resolveConfig(cmd, dir)` helper in `cmd/helpers.go` handles the config merge and flag overrides.

### Adding a builtin primitive

Add a `case "<name>":` branch in `execBuiltinStep` in `engine.go` and implement the function in `primitives.go` following the `func primitiveXxx(ctx, dir, config, state) (PrimitiveResult, error)` signature. Return `TransientError` for retriable failures, `UnrecoverableError` for permanent ones.

### Adding a builtin predicate

Add a `case "<name>":` branch in `evalBuiltinPredicate` in `predicates.go`. The function signature is `func evalBuiltinPredicate(name string, state, config map[string]any) (bool, bool)` where the second return indicates whether the name was recognized (false triggers fallback to custom expression predicates).

Custom predicates can also be defined per-blueprint in the `predicates:` YAML section using the expression syntax `path.to.key == "value"` — no code changes required.

### Adding a step type

Add a constant in `blueprint.go` (`StepTypeXxx = "xxx"`) and a `case StepTypeXxx:` in `runStep`'s type switch in `engine.go`.

### Adding an EngineEvent type

Add a new `Type` string constant (by convention in `engine.go`), emit it with `e.emit(EngineEvent{Type: "my-type", ...})`, and handle it in the Flutter `detail_timeline.dart` event renderer.

### Adding an MCP tool

Implement a `func myTool() mcp.Tool` that returns the tool descriptor and a handler `func (gs *GolemServer) handleMyTool(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)`, then call `register("my_tool", myTool(), gs.handleMyTool)` in `registerTools()` in `mcp/server.go`.

### Adding a built-in agent

Add `<name>.yaml` to `templates/agents/` and if the agent uses custom step prompts, add corresponding `<step-name>.md` files to `templates/prompts/`. The `embed.go` file uses a blanket `//go:embed` directive that picks up all files under `templates/` automatically.
