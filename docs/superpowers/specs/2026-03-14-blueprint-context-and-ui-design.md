# Blueprint Context Integration & Unified UI

This spec covers two connected concerns: integrating the legacy context management system with the blueprint engine, and redesigning the Golem UI (CLI + Flutter GUI) around blueprint-centric workflows.

## Background

Golem has two execution engines:

- **Legacy builder loop** (`golem code`): Iterates over a task list in `state.yaml`. Each iteration gives Claude rich context — MCP tools for state mutation and graph queries, handoff notes, context map injection, decisions/pitfalls. Claude decides what to work on.
- **Blueprint engine** (`golem run`): Executes declarative YAML pipelines. Steps have typed reads/writes contracts. The engine decides what runs when. Currently has minimal context — no MCP, no graph, no project-level state.

The blueprint engine has superior execution semantics but is context-starved compared to the legacy loop. This spec bridges that gap and designs a unified UI for both workflows.

## Context Integration

### Three-Layer Model

**Project-level context** (persistent, cross-run):
- `state.yaml`: decisions, pitfalls, project metadata (name, summary, stack, docs_path)
- `graph.db`: code knowledge graph with embeddings
- Persists across runs. Accumulates project wisdom over time.
- Available to blueprint steps via MCP server.

**Run-level context** (per-pipeline execution):
- Engine's `map[string]any` state with reads/writes contracts
- `log.json` event stream (NDJSON)
- State snapshots (`.ctx/runs/{runID}/state-NNN.json`)
- Managed entirely by the engine. Steps interact through contracts.

**Step-level context** (per Claude session):
- Prompt template with `${key}` tokens resolved from pipeline state
- MCP tools filtered by the step's `tools:` declaration via `GOLEM_TOOLS`
- `session-output.json` for writing results back to pipeline state

### What Changes in the Engine

**MCP server for blueprint runs.** At the start of `Run()`:
1. Call `WriteMCPConfig(dir, noLSP)` to write `.ctx/mcp_servers.json`.
2. Assert the runner is a `*ClaudeRunner` and set `claudeRunner.MCPConfig = mcpPath`.
3. Every agentic step's `claude -p` invocation gets `--mcp-config` automatically.
4. Claude spawns and tears down the MCP server subprocess per session.

This requires adding `MCPEnabled bool` and `LSPEnabled bool` to `EngineConfig`.

State mutation tools (`add_decision`, `add_pitfall`) are available to blueprint steps. These write to `state.yaml` — the project's persistent layer. This doesn't conflict with pipeline state because they serve different purposes: pipeline state is run-scoped data flow, `state.yaml` is project-scoped knowledge.

Tools that are irrelevant to blueprints (`mark_task`, `set_phase`) are harmless — they operate on `state.yaml` task lists which blueprint runs don't use. No filtering needed beyond what `GOLEM_TOOLS` already provides.

**Knowledge graph sync at run start.** Before executing the pipeline:
1. Check if `.ctx/graph.db` exists. If not, skip.
2. Open the graph store via `graph.OpenStore()`.
3. Call `graph.NewBuilder(store).Sync(dir)` to update the graph from current source files.
4. If embeddings exist (`store.EmbeddingCount() > 0`), run incremental embedding via `embed.NewPipeline(store, embedder).EmbedAll()`.
5. Close the store after sync completes.

This requires adding `GraphPath string` to `EngineConfig` (defaults to `.ctx/graph.db`).

**Execution collector for agentic steps.** If graph.db exists and the runner is a `*ClaudeRunner` with `StreamJSON` enabled:
1. Open graph store, create `execution.NewCollector(store, sessionID)`.
2. Set `ClaudeRunner.SetupStreamCallbacks` to register `collector.OnBashCommand` and `collector.OnBashResult` on the `StreamParser`.
3. On engine completion, call `collector.Finish(status)` and close the store.
4. Prune old sessions via `execution.PruneSessions(store, keepCount)`.

**Project context as readable state.** At engine start, if `state.yaml` exists, read decisions and pitfalls and inject them into the initial state as `project-context`. Steps can declare `optional-reads: [project-context]` to receive this. Token `${project-context}` resolves to a formatted summary of decisions and pitfalls.

### What Doesn't Change

- Blueprint YAML format stays the same
- Pipeline state contracts (reads/writes) stay the same
- Error handling (retry/re-run/halt) stays the same
- Predicates stay the same
- `session-output.json` mechanism stays the same
- Existing agent templates work without modification (they already declare graph tools in their `tools:` fields)

## Unified UI Design

### App Shell: Collapsible Rail + Tabs

Left side: thin icon rail (40px). Top icon (⚡) opens the Activity feed. Below are project icons (single letter, colored) with status dots:
- Green dot: idle (no active runs)
- Yellow dot: running (active pipeline)
- Red dot: failed (most recent run failed)

Hover expands the rail to show project names. Click a project icon to open its workspace as a tab. Click ⚡ to open the Activity feed.

Tabs run along the top of the main area. Activity is the first tab, always available. Project tabs can be opened and closed. State persists across app restarts.

Bottom: status bar showing `golem serve` connection status and count of active runs across all projects.

### Activity Feed (⚡ tab)

The app's landing page. A chronological feed of all runs across all registered projects, newest first.

**Filter bar:** All / Running / Failed toggle chips at top-right.

**Run cards:** Each card shows:
- Status dot (yellow pulsing = running, green = success, red = failed)
- Project badge (colored chip with project name)
- Agent name and goal text
- Timestamp (relative: "2m ago", "1h ago")
- For active runs: inline pipeline progress bar (segments colored green/yellow/gray)
- For completed runs: PR link if available
- For failed runs: halt reason

Clicking a run card opens that project's workspace tab and selects the run in the detail panel.

### Project Workspace (project tabs)

Three zones: command bar (top), run feed (left), detail panel (right).

**Command bar:**
- Goal text input field with placeholder "Describe what you want to build..."
- Agent picker chips: `build-feature`, `one-shot`, `fix-bug`, plus any custom agents from `.ctx/agents/`. Selected agent highlighted.
- Green "Run" button to launch.
- This is the primary action surface. Type goal, pick agent, click run.

**Run feed (left, ~55% width):**
- "ACTIVE" section: running pipelines with agent name, goal, elapsed time, pipeline progress bar (segmented, colored per step status), step labels below the bar.
- "RECENT" section: completed/failed runs as compact cards. Status dot, agent, goal, duration, PR link or halt reason.
- Click a run card to select it — detail panel updates.

**Detail panel (right, ~45% width):**
Three tabs at top: **State** / **Terminal** / **Timeline**.

*State tab:*
- Current step: name, type (agentic/builtin/shell), elapsed time, tool call count
- Pipeline state: key-value pairs from engine state map (goal, plan, branch, test-results, etc.)
- Changes: diff stat (+N/-N, file count), file list
- Project context: decisions and pitfalls counts, expandable

*Terminal tab:*
- For interactive sessions (`golem plan`): full bidirectional terminal. User converses with Claude.
- For running blueprint steps: attach to Claude's raw stream output (read-only, like `docker logs -f`).
- For ad-hoc sessions: a conversational interface to Claude with the project's full context (graph, state, decisions). Developer can ask questions, explore code, or informally kick off a run.

*Timeline tab:*
- Event log from `log.json`. Same rendering as `golem runs inspect`.
- Step-by-step timeline with status, duration, loop iterations, retries.

### Terminal as Agent Interface

The terminal isn't just a passive viewer — it's a conversational interface to Claude. Use cases:

- **Interactive planning**: `golem plan` opens a terminal session where user and Claude collaborate on a plan. Claude uses MCP tools to write tasks. When done, the workspace shows tasks ready to be dispatched as blueprint runs.
- **Attach to running step**: While a blueprint is running, switch to Terminal tab to see Claude's raw output for the current agentic step. Observe, don't interact.
- **Ad-hoc agent** (future): Type questions or instructions directly. Claude conversational interface with project context for exploration and Q&A. Blueprint dispatch from within a conversation is a future extension — not in scope for this spec.

### Multi-Project Support

Projects are registered with `golem serve` (existing mechanism). The rail shows all registered projects. Each project can have independent runs happening concurrently. The Activity feed shows everything in one timeline.

Opening a project tab loads its `.ctx/` state and connects to its event stream. Multiple project tabs can be open simultaneously.

## CLI Experience

The CLI produces the same information as the GUI, rendered for the terminal:

```
$ golem run build-feature --goal "Add JWT auth"
golem: starting agent=build-feature goal="Add JWT auth" run=run-20260314-...
golem: [builtin] git-setup success (0.3s)
golem: [agentic] plan success (42s)
golem: [agentic] implement running...
golem: [agentic] implement success (4m 31s)
golem: [builtin] lint success (2.1s)
golem: [builtin] run-tests success (8.4s)
golem: [agentic] review success (35s)
golem: loop needs-work exited (false)
golem: pipeline success (6m 12s)

golem: run complete — build-feature (run-20260314-...)
golem: branch: golem/build-feature-20260314-142305
golem: PR: https://github.com/you/repo/pull/42
```

The `displayEngineEvents()` function already produces this output. The `log.json` NDJSON stream is the shared data layer — CLI reads it for `golem runs inspect/watch`, Flutter GUI tails it for live updates.

Management commands:
```
golem agents                        # list available agents
golem runs list                     # list recent runs across project
golem runs inspect <run-id>         # timeline + final state
golem runs watch <run-id>           # event timeline (currently one-shot; live tail is future work)
golem runs clean --keep 5           # prune old runs
```

## Workflow Convergence

Two paths into the same engine:

**Blueprint path (zero-setup):**
- CLI: `golem run build-feature --goal "Add JWT auth"`
- GUI: Type goal in command bar, pick agent, click Run
- No planning phase needed. The agent YAML defines the workflow. Planning happens inside the pipeline as a step.

**Plan-then-execute path (interactive planning):**
- CLI: `golem plan` then `golem code` (legacy loop) or multiple `golem run one-shot --goal "<task>"` invocations
- GUI: Click Plan button, interactive terminal session, Claude writes tasks via MCP. When done, the workspace shows tasks. User can dispatch each as an independent blueprint run, or run the legacy loop.
- Future: `golem plan` outputs a structured plan that the GUI renders as launchable blueprint runs. "Run all" button spawns N `one-shot` agents in parallel.

## Decision Log

| Decision | Choice | Rationale |
|----------|--------|-----------|
| App shell | Collapsible rail + tabs | Maximizes workspace, scales to many projects, familiar pattern |
| Mission control | Chronological feed | Simple, scannable, unified across projects — like a CI dashboard |
| Project workspace | Command bar + run feed + detail panel | B/C hybrid: launching and monitoring in one view |
| Detail panel | Tabbed: State/Terminal/Timeline | Terminal stays as interactive agent interface |
| MCP in blueprints | Spawn once per run via config file | Same as legacy loop; Claude manages MCP server lifecycle per session |
| State mutation tools | Available, not filtered | Decisions/pitfalls write to project-level state.yaml; task tools are harmless no-ops |
| Context model | Three layers: project → run → step | Clean separation; each layer has its own persistence |
| CLI ↔ GUI data | Shared log.json NDJSON stream | One source of truth, two renderings |
| Graph sync | At run start, before pipeline | Same as legacy loop behavior |
| Project context | Injected as optional-reads state key | Steps opt-in to project context via `${project-context}` |

## Implementation Scope

This spec should be planned as two independent workstreams that share the `log.json` NDJSON data layer. They can be implemented in parallel.

### Workstream 1: Blueprint Context Integration (Go)

Engine changes in `internal/runner/`:
1. Add `MCPEnabled`, `LSPEnabled`, `GraphPath` fields to `EngineConfig`
2. Wire MCP server spawn into `engine.go` `Run()`: call `WriteMCPConfig`, set `ClaudeRunner.MCPConfig`
3. Add graph sync at engine start: open store, sync, embed, close
4. Wire execution collector for stream-json agentic steps
5. Inject project context (decisions/pitfalls from `state.yaml`) into initial engine state as `project-context` key
6. Add `printRunSummary` and `displayEngineEvents` to CLI output (done)
7. Update `golem run` to use blueprint engine (done)

CLI changes in `cmd/`:
1. Pass `MCPEnabled`/`LSPEnabled` from resolved config to `EngineConfig`
2. `golem run` wired to blueprint engine (done)
3. Event display during runs (done)
4. Run summary with state output (done)

### Workstream 2: Unified UI (Flutter)

Data contracts — the Flutter GUI consumes `EngineEvent` structs from `log.json` NDJSON. Required event types: `pipeline-start`, `pipeline-end`, `step-start`, `step-end`, `loop-enter`, `loop-exit`, `conditional-skip`, `error-retry`. All already emitted by the engine.

Flutter rebuild in `ui/flutter/`:
1. New app shell: collapsible rail + tabs replacing current `ShellView`
2. Activity feed view: chronological run feed across projects, replacing `DashboardView`
3. Project workspace: command bar + run feed (left) + detail panel (right)
4. Detail panel tabs: State / Terminal / Timeline
5. Agent picker widget: reads built-in agents via `golem agents --json` + scans `.ctx/agents/`
6. Run card widget with segmented pipeline progress bar
7. Multi-project state management via Riverpod: project registry, per-project event streams
8. Event stream consumption: tail `.ctx/runs/{runID}/log.json` or subscribe via `golem serve` websocket
