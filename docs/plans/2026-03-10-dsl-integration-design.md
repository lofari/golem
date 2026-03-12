# DSL Integration Design

> Date: 2026-03-10
> Status: Approved

## Summary

The Clojure DSL replaces the Go builder loop as golem's orchestration engine. The Go binary becomes the infrastructure layer (sessions, knowledge graph, MCP, git), while the DSL defines composable agent flows. Two binaries communicate via CLI calls and shared filesystem.

## Decisions

- **DSL replaces Go loop** — `golem code` delegates to the DSL rather than running `RunBuilderLoop`. The current Go loop is a single hardcoded flow; the DSL makes all flows composable and user-modifiable.
- **Sidecar binary architecture** — `golem` (Go) and `golem-dsl` (GraalVM native) are separate binaries. Clean separation, independent evolution, no JVM runtime dependency for users.
- **DSL-native state, Go infrastructure** — The DSL manages immutable EDN state with compile-time contracts. Go provides knowledge graph, MCP server, session execution, git helpers. State syncs to `.ctx/state.yaml` for human readability.
- **GraalVM native-image** — DSL compiles to a standalone binary. No JVM required at runtime.
- **Built-ins + project-local agents** — Agents ship embedded in `golem-dsl`. Users add custom agents to `.ctx/agents/`.
- **Phased migration** — DSL coexists first (opt-in), then becomes default, then Go loop is removed.

## Binary Communication Protocol

### DSL → Go

| Command | Purpose |
|---------|---------|
| `golem session --prompt <file> --dir <dir> --max-turns N` | Spawn a Claude session |
| `golem graph query <term>` | Semantic search over knowledge graph |
| `golem graph dependents <symbol>` | Find dependents of a symbol |
| `golem mcp-serve` | Start MCP server for session config |

### Go → DSL

| Command | Purpose |
|---------|---------|
| `golem-dsl run <agent> --goal "..." --state-dir .ctx` | Run an agent |
| `golem-dsl list` | List available agents |
| `golem-dsl inspect <run-id>` | Inspect run state |

### Shared Filesystem

```
.ctx/
  state.yaml          # Human-readable dashboard (synced by DSL)
  config.yaml         # Project config (read by both)
  agents/             # Project-local .clj agent definitions
  runs/               # DSL execution state
    run-001/
      state-v0.edn
      state-v1.edn
      log.edn
  sessions/           # Claude session outputs (written by Go)
  snapshots/          # State snapshots
```

## `golem code` Delegation

```
golem code --goal "add user auth"
  ↓
1. Read .ctx/config.yaml for agent setting (default: "build-feature")
2. Sync knowledge graph
3. Call: golem-dsl run build-feature --goal "add user auth" --state-dir .ctx
4. Stream events from DSL stdout
5. On completion, print summary
```

### Responsibility Split

**Stays in Go:**
- Knowledge graph building/syncing
- MCP server lifecycle
- Session execution (`golem session`)
- Git helpers
- `golem status`, `golem config`, `golem init`, `golem plan`
- Event display / TUI

**Moves to DSL:**
- Orchestration logic (step ordering, state flow)
- Strategy decisions (retry, skip, halt)
- Contract validation
- Error classification and recovery

### Default Agent

The `build-feature` agent replaces `RunBuilderLoop`:

```clojure
(defagent build-feature
  {:initial-state [:goal]}
  (plan)
  (implement)
  (review)
  (while needs-work? {:max 5}
    (implement)
    (review))
  (on-error :transient (retry {:max 3}))
  (on-error :contract-violation (snapshot-and-halt)))
```

## Event Streaming

The DSL emits newline-delimited JSON on stdout. Go parses these and maps them to its existing `EventType` system.

```json
{"type":"step-start","step":"plan","iteration":1,"agent":"build-feature"}
{"type":"session-start","step":"plan","session-id":"run-001-v1"}
{"type":"session-end","step":"plan","outcome":"ok","duration-ms":42000}
{"type":"step-end","step":"plan","state-version":1}
{"type":"error","step":"implement","error-type":"transient","action":"retry","attempt":2}
{"type":"agent-done","agent":"build-feature","outcome":"complete","total-steps":5}
```

### State Sync

After each DSL step, a summary projection is written to `.ctx/state.yaml`:
- Task statuses
- `status.phase`
- `status.last_session`
- New decisions (if any)

This keeps `golem status --watch` working unchanged.

## Agent Resolution

Resolution order:
1. `.ctx/agents/<name>.clj` — project-local
2. Built-in agents embedded in `golem-dsl` binary

### Built-in Agents (v1)

| Agent | Description |
|-------|-------------|
| `build-feature` | Default. Plan → implement → review loop |
| `fix-bug` | Research → implement → test loop |
| `write-docs` | Documentation generation |
| `review` | Single-pass code review |

### Config

```yaml
# .ctx/config.yaml
agent: build-feature
agent_opts:
  max_iterations: 5
```

### New Commands

- `golem run <agent> --goal "..."` — run a specific agent
- `golem agents` — list available agents with source (built-in/project)

### Scaffolding

`golem init --with-agents` drops an example `.ctx/agents/custom.clj`.

## Migration Path

### Phase 1 — Coexistence

- `golem code` uses Go builder loop by default
- `golem run <agent>` delegates to DSL
- Config `engine: dsl` opts `golem code` into DSL
- `golem-dsl` checked on PATH; helpful install message if missing

### Phase 2 — DSL Default

- `golem code` delegates to DSL by default
- Config `engine: legacy` keeps Go loop as fallback
- Deprecation warning on legacy engine

### Phase 3 — Remove Go Loop

- Delete `internal/runner/builder.go`, strategy, validation logic
- DSL handles all orchestration
- Go binary slims to infrastructure + session execution

### Unchanged Across Phases

- `.ctx/` directory structure
- `golem status`, `golem config`, `golem plan`
- `golem session`
- MCP tools available to Claude sessions
- Knowledge graph

## Build & Distribution

### GraalVM Native Image

- Clojure project with `deps.edn` + GraalVM native-image
- Reflection config generated via tooling
- ~30-50MB standalone binary per platform

### Release

- Single GitHub release includes both binaries
- Platform variants: linux-amd64, linux-arm64, darwin-amd64, darwin-arm64
- Install script downloads both to same location

### CI

- GraalVM setup step
- `clj -M:test` for DSL tests
- Native-image build per platform
- Integration test: full binary communication exercise

### Dev Workflow

- Run via `clj -M:run` during development (no native-image needed)
- Config `dsl_command: "clj -M:run"` overrides binary path
