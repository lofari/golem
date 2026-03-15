# Project Documentation Design

## Summary

Add thorough documentation for golem targeting both end users (blueprint authoring, troubleshooting) and contributors (architecture, code conventions, extension points). Includes root-level project files (CONTRIBUTING, CHANGELOG, LICENSE) and focused guides in `docs/`.

## Motivation

The README is strong for getting started but drops off after that. Users who want to write custom blueprints have no guide. Contributors who want to understand the codebase have only CLAUDE.md (67 lines). The project lacks standard open-source files (CONTRIBUTING, CHANGELOG, LICENSE) that signal maturity and make contribution possible.

## File Structure

### Root-level (Go/OSS conventions)

| File | Purpose | ~Lines |
|------|---------|--------|
| `CONTRIBUTING.md` | PR process, code conventions, testing | 100 |
| `CHANGELOG.md` | Version history (keep-a-changelog format) | 50 |
| `LICENSE` | MIT license text | 21 |

### Guides in `docs/` (alongside existing `plans/`, `superpowers/`, `research/`)

| File | Purpose | ~Lines |
|------|---------|--------|
| `docs/blueprint-authoring.md` | Writing custom agents, predicates, error handlers | 300 |
| `docs/architecture.md` | Codebase deep-dive for contributors | 400 |
| `docs/api-reference.md` | REST + WebSocket endpoints | 200 |
| `docs/troubleshooting.md` | Common errors, debugging, FAQ | 150 |

### Updates to existing files

| File | Change |
|------|--------|
| `README.md` | Add "Documentation" section with links; remove "DSL Agents (Experimental)" section; update architecture snippet if it references golem-dsl |
| `CLAUDE.md` | Add cross-reference to `docs/architecture.md` for deeper codebase understanding |

## Document Specifications

### CONTRIBUTING.md

- **Build & test**: `go build ./...`, `go test ./...`, `go run . --help`
- **Code conventions**: commit message format (`type(scope): description`), tests next to source (`foo.go`/`foo_test.go`), stdlib `testing` only, no external test frameworks, templates embedded via `embed.go`
- **PR process**: branch from `main`, conventional commits, what reviewers look for
- **Project layout**: one-paragraph description of each top-level package (`cmd/`, `internal/runner/`, `internal/server/`, `internal/ctx/`, `internal/config/`, `internal/graph/`, `internal/mcp/`, `internal/scaffold/`, `internal/display/`, `internal/git/`, `templates/`, `ui/flutter/`)
- **Link to**: `docs/architecture.md` for deeper understanding

### CHANGELOG.md

- Format: [Keep a Changelog](https://keepachangelog.com/) with `## [Unreleased]` section
- Seed with current state as baseline, not exhaustive history:
  - Added: blueprint engine, custom predicates, error handler priority chain, knowledge graph, MCP server, Flutter desktop UI, WebSocket event broadcast
  - Removed: Clojure DSL runtime
- Future entries added per release

### LICENSE

- Standard MIT license text
- Copyright holder and year from README or git history

### docs/blueprint-authoring.md

Sections:

1. **What are blueprints** — YAML pipelines that orchestrate Claude Code sessions with state management, control flow, and error recovery
2. **Quick example** — Annotated walkthrough of `build-feature.yaml`
3. **Blueprint structure** — Top-level fields: `name`, `description`, `initial-state`, `config`, `predicates`, `steps`, `errors`
4. **Step types**:
   - Agentic: Claude Code session with prompt template, reads/writes, tools, max-turns, timeout, model
   - Builtin: `git-setup`, `lint`, `run-tests`, `ci-tests`, `create-pr` — what each does, required config
   - Shell: arbitrary commands with `command` field, timeout, exit code handling
5. **State contracts** — reads/writes/optional-reads, how validation works, conditional writes in control flow, error messages and how to fix them
6. **Control flow** — `while` (predicate + max), `when` (conditional), `if/then/else` — with YAML examples
7. **Predicates**:
   - Built-in 5: `needs-work`, `failed`, `lint-failed`, `ci-enabled`, `ci-failed`
   - Custom predicates: expression syntax (`path.to.key == "value"`), operators, supported types, config path prefix
8. **Error handling**:
   - Error types: transient, malformed-output, unrecoverable, contract-violation
   - Handler config: action (retry/re-run/halt), max, hint
   - Priority chain: step-level > blueprint-level > built-in defaults
   - `_error_context` injection on retries
9. **Prompt templates** — `${key}` interpolation, optional-read line removal, `${config.key}`, `${agent.name}`, `${run.id}`, where templates live (`templates/prompts/`)
10. **Creating your own agent** — Step-by-step: create YAML in `.ctx/agents/`, define steps with reads/writes, reference or inline prompts, test with `golem run <name> --goal "..."`, iterate
11. **Complete examples** — 2-3 blueprints for common workflows (e.g., documentation generator, refactoring agent, migration agent)

### docs/architecture.md

Sections:

1. **Overview** — golem wraps `claude -p` in structured iteration loops; each session gets fresh context from `.ctx/` files; the blueprint engine adds pipeline orchestration with state, control flow, and error recovery
2. **Package map** — One paragraph per package covering responsibility, key types, and how it connects to other packages:
   - `cmd/` — Cobra commands, flag parsing, runner dispatch
   - `internal/runner/` — Blueprint engine (engine.go, blueprint.go), legacy builder loop, command runner abstraction, primitives, predicates, events, validation, strategy
   - `internal/server/` — HTTP/WebSocket server for Flutter UI, state watch, event broadcast
   - `internal/ctx/` — State and log YAML parsing/writing
   - `internal/config/` — Two-layer config system (global + project)
   - `internal/graph/` — Knowledge graph: tree-sitter indexing, embeddings, LSP, query pipeline
   - `internal/mcp/` — MCP server for structured state updates and graph tools
   - `internal/display/` — Plain-text formatters
   - `internal/scaffold/` — `golem init` scaffolding
   - `internal/git/` — Git helpers (changed files, diff summaries)
   - `templates/` — Embedded templates: agents, prompts, state.yaml, log.yaml, claude.md
   - `ui/flutter/` — Flutter desktop GUI
3. **Blueprint engine flow** — Parse YAML → validate fields/typos → validate contracts (reads vs writes) → create pipeline (nodes + step defs) → execute: for each node, dispatch by type (agentic/builtin/shell), save state snapshot, emit events → handle errors with priority chain → pipeline-end event
4. **Key interfaces**:
   - `CommandRunner` — abstraction over `claude -p` invocation; production impl `ClaudeRunner`, test impl `smartMockRunner`
   - `Engine` — pipeline executor with RunID, state map, run directory, log file
   - `EngineEvent` — structured events for logging and UI updates
5. **State management** — `.ctx/` directory layout, `state.yaml` and `log.yaml` formats, run directories (`.ctx/runs/<run-id>/`), state snapshots (`state-001.json`, `state-002.json`), `current` symlink
6. **Event system** — Note: there are two event systems. The legacy `Event`/`EventType` (builder TUI, in `events.go`) and the `EngineEvent` (blueprint engine, in `engine.go`). Cover EngineEvent types (pipeline-start/end, step-start/end, loop-enter/exit, error-occurred/retry, conditional-skip), delivery (JSON log file + send-only channel), WebSocket broadcast via server
7. **Server/UI architecture** — HTTP routes, WebSocket upgrade, event fan-out, Flutter desktop connection via `golem ui` (starts server + launches Flutter)
8. **Knowledge graph** — tree-sitter parsing, embedding pipeline, LSP integration, 5-stage ranking for context injection, graph.db schema
9. **MCP integration** — MCP config generation, tool registration, how tools map to graph queries
10. **Extension points** — Adding a new command (cmd/ file + cobra), adding a builtin primitive (primitives.go switch), adding a built-in predicate (predicates.go switch), adding a step type, adding an event type

### docs/api-reference.md

Sections:

1. **Server startup** — `golem serve` flags, default port, CORS
2. **REST endpoints** — Table of all routes from `internal/server/` with method, path, request/response schemas
3. **WebSocket protocol** — Connection URL, upgrade handshake, event stream format (JSON lines), reconnection behavior
4. **EngineEvent schema** — Complete JSON structure with all fields, types, and which events include which fields
5. **State endpoints** — Reading project state, watching for changes
6. **Examples** — curl for REST, wscat for WebSocket, practical usage patterns

### docs/troubleshooting.md

Sections:

1. **Common errors**:
   - "session-output.json not found" — step didn't write output; check prompt instructs Claude to write it
   - "contract violation: step X reads Y which is not produced" — fix reads/writes/optional-reads
   - "unresolved tokens ${...}" — template references a key not in reads or config
   - Timeout errors — adjust step timeout or max-turns
   - "unknown builtin primitive" — typo in step name
2. **Debugging tips**:
   - Read run logs: `.ctx/runs/<run-id>/log.json` (JSON lines of engine events)
   - Read state snapshots: `.ctx/runs/<run-id>/state-001.json` etc.
   - Use `golem status --watch` for live state
   - Use `golem log` for iteration history
   - Verbose mode for detailed output
3. **Configuration issues**:
   - Config precedence: flags > project (`.ctx/config.yaml`) > global (`~/.config/golem/config.yaml`)
   - Check effective config: `golem config list`
   - Common config mistakes (wrong key names, YAML formatting)
4. **Graph issues**:
   - `golem graph build` to re-index
   - `golem graph embed` to regenerate embeddings
   - `golem graph status` to check health
   - Stale graph after branch switches
5. **Performance tips**:
   - Large repos: scope graph indexing to relevant directories
   - Long sessions: tune max-turns and timeout per step
   - Graph size: check `golem graph status` for file count

### README.md updates

1. Add a "Documentation" section after the Quick Start (or near the end before Design Principles):
   ```markdown
   ## Documentation

   - [Blueprint Authoring Guide](docs/blueprint-authoring.md) — Write custom agents and pipelines
   - [Architecture Guide](docs/architecture.md) — Codebase deep-dive for contributors
   - [API Reference](docs/api-reference.md) — REST and WebSocket endpoints
   - [Troubleshooting](docs/troubleshooting.md) — Common issues and debugging
   - [Contributing](CONTRIBUTING.md) — How to contribute to golem
   - [Changelog](CHANGELOG.md) — Version history
   ```

2. Remove the "DSL Agents (Experimental)" section entirely

3. Update the architecture/project structure if it mentions `golem-dsl/`

## Known Gaps / Future Documentation

These are out of scope for this round but should be addressed later:

- **Godoc inline comments** — Systematic pass over exported types/functions to add Go-standard documentation comments
- **Example projects directory** — `examples/` with sample `.ctx/` setups showing real workflows (e.g., "add auth to a Go API", "migrate from Express to Fastify")
- **Plugin development guide** — How to build Claude Code plugins that work with golem's `--plugin-dir` flag
- **Flutter UI user guide** — Screenshots, feature walkthrough, keyboard shortcuts
- **Deployment guide** — Running golem server in CI, Docker setup, multi-user scenarios
- **Config schema reference** — Machine-readable schema for all config keys with types, defaults, and descriptions
- **Security model** — Deep-dive on `--dangerously-skip-permissions`, sandbox/warden, what golem can and cannot access
- **Migration guide** — Moving from legacy builder to blueprint engine

## Backward Compatibility

No code changes. Documentation only. The README "DSL Agents" removal reflects code already removed in the DSL extraction work.
