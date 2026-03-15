# Project Documentation Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add thorough documentation for golem covering both end users and contributors.

**Architecture:** Seven new documentation files (3 root-level OSS files + 4 guides in docs/) plus updates to README.md and CLAUDE.md. Each document is independent and self-contained. No code changes.

**Tech Stack:** Markdown

**Spec:** `docs/superpowers/specs/2026-03-15-project-documentation-design.md`

---

## Chunk 1: Root-Level Project Files

### Task 1: Create LICENSE

**Files:**
- Create: `LICENSE`

- [ ] **Step 1: Write the LICENSE file**

Standard MIT license. Copyright year 2026 (earliest commit). Copyright holder from git history.

- [ ] **Step 2: Commit**

```bash
git add LICENSE
git commit -m "docs: add MIT LICENSE file"
```

---

### Task 2: Create CHANGELOG.md

**Files:**
- Create: `CHANGELOG.md`

- [ ] **Step 1: Write CHANGELOG.md**

Use [Keep a Changelog](https://keepachangelog.com/) format. Seed with current state as baseline:

```markdown
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Blueprint engine: YAML-defined pipelines that orchestrate Claude Code sessions with state management, control flow, and error recovery
- Custom predicates: expression-based predicates in blueprint YAML (`path.to.key == "value"`)
- Error handler priority chain: step-level > blueprint-level > built-in defaults, with `_error_context` injection on retries
- Knowledge graph: tree-sitter indexing, embeddings, LSP integration, 5-stage context ranking
- MCP server: structured state updates and graph query tools for Claude Code sessions
- Flutter desktop UI: run management, event timeline, terminal emulation
- WebSocket event broadcast: real-time engine event streaming for UI integration
- HTTP API server: REST endpoints for project state, config, processes, and graph queries

### Removed
- Clojure DSL runtime: patterns extracted into Go blueprint engine (custom predicates, error classification)
```

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: add CHANGELOG.md with current state baseline"
```

---

### Task 3: Create CONTRIBUTING.md

**Files:**
- Create: `CONTRIBUTING.md`

- [ ] **Step 1: Write CONTRIBUTING.md**

Cover these sections:

**Getting Started:**
- Prerequisites: Go 1.22+, git
- Build: `go build ./...`
- Test: `go test ./...`
- Run from source: `go run . --help`

**Code Conventions:**
- Commit messages: `type(scope): description` (types: feat, fix, refactor, test, docs)
- Tests live next to source: `foo.go` / `foo_test.go`
- No external test frameworks — stdlib `testing` only
- Templates embedded via `embed.go` in `templates/`
- No unnecessary abstractions — prefer simple, direct code

**Pull Request Process:**
- Branch from `main`
- Keep PRs focused — one feature or fix per PR
- All tests must pass (`go test ./...`)
- Include tests for new functionality
- Update documentation if behavior changes

**Project Layout** (one line each):
- `cmd/` — CLI commands (cobra). Each file = one command.
- `internal/runner/` — Core engine: blueprint parsing, pipeline execution, primitives, predicates, events, error handling, validation
- `internal/server/` — HTTP/WebSocket server for desktop UI (state watch, event broadcast)
- `internal/ctx/` — State and log YAML parsing/writing
- `internal/config/` — Two-layer config system (global `~/.config/golem/config.yaml` + project `.ctx/config.yaml`)
- `internal/graph/` — Knowledge graph: tree-sitter indexing, embeddings, LSP integration, query pipeline
- `internal/mcp/` — MCP server for structured state updates and graph tools
- `internal/display/` — Plain-text formatters for terminal output
- `internal/scaffold/` — `golem init` project scaffolding
- `internal/git/` — Git helpers (changed files, diff summaries)
- `templates/` — Embedded templates: agent blueprints, prompt templates, state/log schemas
- `ui/flutter/` — Flutter desktop GUI (Riverpod, xterm.dart, WebSocket)

**Further Reading:**
- Link to `docs/architecture.md` for codebase deep-dive

- [ ] **Step 2: Commit**

```bash
git add CONTRIBUTING.md
git commit -m "docs: add CONTRIBUTING.md with conventions and project layout"
```

---

## Chunk 2: Blueprint Authoring Guide

### Task 4: Create docs/blueprint-authoring.md

**Files:**
- Create: `docs/blueprint-authoring.md`

**Source material to read before writing:**
- `templates/agents/build-feature.yaml` — reference blueprint
- `templates/agents/fix-bug.yaml` — reference blueprint
- `templates/agents/one-shot.yaml` — reference blueprint
- `internal/runner/blueprint.go` — step types, fields, validation, contract checking
- `internal/runner/engine.go` — execution flow, error handling, state management
- `internal/runner/predicates.go` — built-in predicates
- `internal/runner/predicate_expr.go` — custom predicate expression parser
- `internal/runner/primitives.go` — built-in primitives (git-setup, lint, run-tests, ci-tests, create-pr)
- `templates/prompts/*.md` — prompt templates

- [ ] **Step 1: Write the guide**

Sections (refer to spec for detailed outlines):

1. **What are blueprints** — YAML pipelines orchestrating Claude Code sessions
2. **Quick example** — Annotated `build-feature.yaml` walkthrough
3. **Blueprint structure** — All top-level fields: `name`, `description`, `initial-state`, `config`, `predicates`, `steps`, `errors`
4. **Step types** — Agentic (with defaults table: plan/implement/review/reflect/research max-turns and timeouts), builtin (each primitive explained), shell (command, timeout, exit handling)
5. **State contracts** — reads/writes/optional-reads with examples. How validation works. Conditional writes in control flow. Common error messages and fixes.
6. **Control flow** — while/when/if with YAML examples showing predicate + max + nested steps
7. **Predicates** — Built-in 5 with what state keys they check. Custom predicate expression syntax: path resolution, operators (==, !=, >, <, >=, <=), value types (strings, numbers, booleans), config. prefix for config map access.
8. **Error handling** — Error types with causes. Handler config (action/max/hint). Priority chain. _error_context on retries.
9. **Prompt templates** — ${key} interpolation, optional-read line removal, config/agent/run variables, template file locations
10. **Creating your own agent** — Step-by-step: create `.ctx/agents/my-agent.yaml`, define steps, run with `golem run my-agent --goal "..."`, iterate
11. **Complete examples** — 2-3 practical blueprints (e.g., docs generator, refactoring agent)

The guide should include actual YAML snippets from the built-in agents and real prompt template examples.

- [ ] **Step 2: Verify all YAML examples are syntactically valid**

Read through the document and check that YAML snippets match the actual schema used in `blueprint.go`.

- [ ] **Step 3: Commit**

```bash
git add docs/blueprint-authoring.md
git commit -m "docs: add blueprint authoring guide"
```

---

## Chunk 3: Architecture Guide

### Task 5: Create docs/architecture.md

**Files:**
- Create: `docs/architecture.md`

**Source material to read before writing:**
- `internal/runner/engine.go` — Engine struct, Run(), execNode(), handleError(), emit()
- `internal/runner/blueprint.go` — Blueprint struct, ParseBlueprint(), ValidateContracts(), Pipeline
- `internal/runner/command.go` — CommandRunner interface, ClaudeRunner
- `internal/runner/events.go` — Legacy Event/EventType (builder TUI)
- `internal/runner/builder.go` — Legacy builder loop
- `internal/server/server.go` — Route registration, server setup
- `internal/server/websocket.go` — WebSocket handlers
- `internal/graph/` — Graph builder, query pipeline
- `internal/mcp/` — MCP server
- `internal/config/config.go` — Config struct, Defaults(), merge()

- [ ] **Step 1: Write the guide**

Sections (refer to spec for detailed outlines):

1. **Overview** — What golem is and how it works at a high level
2. **Package map** — One paragraph per package (see CONTRIBUTING layout but deeper: key types, files, dependencies)
3. **Blueprint engine flow** — Detailed walkthrough: ParseBlueprint → validateTopLevelFields → validateStepFields → parseSteps → ValidateContracts → NewEngine → Run → execNode → execStep/execControlFlow → handleError → emit → saveState. Include the key structs: Blueprint, Step, Pipeline, PipelineNode, ControlFlowNode.
4. **Key interfaces** — CommandRunner (Run/RunWithTools), Engine (struct not interface), EngineEvent (all fields documented), EngineConfig
5. **State management** — .ctx/ directory layout, state.yaml format, run directories, state snapshots, current symlink
6. **Event system** — Two systems: legacy Event (EventIterStart/EventOutputLine/EventIterEnd/EventLoopDone for builder TUI) and EngineEvent (pipeline-start/end, step-start/end, loop-enter/exit, error-occurred/retry, conditional-skip for blueprint engine). JSON log + channel delivery.
7. **Server/UI architecture** — All HTTP routes (from server.go), WebSocket protocol for process streaming and state watching, Flutter connection via golem ui
8. **Knowledge graph** — tree-sitter, embeddings, LSP, context ranking pipeline
9. **MCP integration** — Config generation, tool registration, graph query tools
10. **Extension points** — How to add: commands, primitives, predicates, step types, event types

- [ ] **Step 2: Verify package descriptions match actual code**

Spot-check that key types and files mentioned in the doc actually exist.

- [ ] **Step 3: Commit**

```bash
git add docs/architecture.md
git commit -m "docs: add architecture guide for contributors"
```

---

## Chunk 4: API Reference and Troubleshooting

### Task 6: Create docs/api-reference.md

**Files:**
- Create: `docs/api-reference.md`

**Source material:**
- `internal/server/server.go` — All route registrations (lines 39-71)
- `internal/server/websocket.go` — WebSocket handlers
- `internal/runner/engine.go` — EngineEvent struct (lines 38-56)

- [ ] **Step 1: Write the API reference**

Sections:

1. **Server startup** — `golem serve` (default port 8314), flags, CORS behavior
2. **REST endpoints** — Complete table:

| Method | Path | Description |
|--------|------|-------------|
| GET | /api/health | Health check |
| GET | /api/projects | List projects |
| POST | /api/projects | Create/register project |
| GET | /api/projects/{id}/state | Get project state |
| GET | /api/projects/{id}/log | Get project log |
| GET | /api/projects/{id}/config | Get project config |
| PUT | /api/projects/{id}/config | Update project config |
| GET | /api/config | Get global config |
| PUT | /api/config | Update global config |
| POST | /api/projects/{id}/processes | Start a process |
| GET | /api/projects/{id}/processes | List processes |
| DELETE | /api/projects/{id}/processes/{procId} | Stop a process |
| GET | /api/projects/{id}/diff | Get git diff |
| GET | /api/projects/{id}/graph/related | Query related files |
| POST | /api/projects/{id}/graph/search | Semantic search |
| GET | /api/projects/{id}/graph/runtime-path | Runtime path analysis |
| GET | /api/projects/{id}/graph/stats | Graph statistics |
| GET | /api/projects/{id}/graph/context-map | Context map |

3. **WebSocket endpoints:**
   - `/api/projects/{id}/processes/{procId}/stream` — PTY output streaming (base64-encoded chunks)
   - `/api/projects/{id}/watch` — State file change notifications

4. **EngineEvent schema** — All fields from the EngineEvent struct with types and which event types include which fields

5. **State endpoints** — Reading project state via GET /api/projects/{id}/state, watching for changes via WebSocket /api/projects/{id}/watch, how state updates are delivered

6. **Examples** — curl commands for common operations, wscat for WebSocket

- [ ] **Step 2: Verify routes match server.go**

Cross-check the table against actual `mux.HandleFunc` registrations in `server.go`.

- [ ] **Step 3: Commit**

```bash
git add docs/api-reference.md
git commit -m "docs: add API reference for REST and WebSocket endpoints"
```

---

### Task 7: Create docs/troubleshooting.md

**Files:**
- Create: `docs/troubleshooting.md`

**Source material:**
- `internal/runner/engine.go` — Error types (TransientError, MalformedOutputError, UnrecoverableError)
- `internal/runner/blueprint.go` — Contract validation error messages
- `internal/runner/blueprint.go` — Field validation, typo suggestions

- [ ] **Step 1: Write the troubleshooting guide**

Sections:

1. **Common errors** — Each error message with cause and fix:
   - `session-output.json not found` (MalformedOutputError) — step didn't write output
   - `contract violation: step X reads Y which is not produced` — missing writes in prior step
   - `contract violation: step X reads Y which is only conditionally written` — use optional-reads
   - `template error: unresolved tokens ${...}` — key not in reads/optional-reads or config
   - `unknown field "X" (did you mean "Y"?)` — typo in blueprint YAML
   - `agentic step X: context deadline exceeded` — increase timeout
   - `unknown builtin primitive: X` — check step name spelling

2. **Debugging tips**
   - Run logs: `.ctx/runs/<run-id>/log.json`
   - State snapshots: `.ctx/runs/<run-id>/state-001.json` etc.
   - Live state: `golem status --watch`
   - Iteration history: `golem log`

3. **Configuration issues**
   - Precedence: flags > project `.ctx/config.yaml` > global `~/.config/golem/config.yaml`
   - Check config: `golem config list`
   - Common mistakes

4. **Graph issues**
   - Re-index: `golem graph build`
   - Re-embed: `golem graph embed`
   - Check health: `golem graph status`

5. **Performance tips**
   - Tune max-turns and timeout per step
   - Check graph size: `golem graph status`

- [ ] **Step 2: Verify error messages match actual code**

Spot-check that the error strings in the guide match what `blueprint.go` and `engine.go` actually produce.

- [ ] **Step 3: Commit**

```bash
git add docs/troubleshooting.md
git commit -m "docs: add troubleshooting guide"
```

---

## Chunk 5: Update Existing Files

### Task 8: Update README.md and CLAUDE.md

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add Documentation section to README.md**

Add a "## Documentation" section (after Quick Start or before Design Principles) with links:

```markdown
## Documentation

- [Blueprint Authoring Guide](docs/blueprint-authoring.md) — Write custom agents and pipelines
- [Architecture Guide](docs/architecture.md) — Codebase deep-dive for contributors
- [API Reference](docs/api-reference.md) — REST and WebSocket endpoints
- [Troubleshooting](docs/troubleshooting.md) — Common issues and debugging
- [Contributing](CONTRIBUTING.md) — How to contribute to golem
- [Changelog](CHANGELOG.md) — Version history
```

- [ ] **Step 2: Remove "DSL Agents (Experimental)" section from README.md**

Remove lines 725-794 (the entire "## DSL Agents (Experimental)" section including all subsections, code blocks, and configuration examples). The next section "## Design Principles" should follow directly after "## Blueprint Agents".

- [ ] **Step 3: Remove golem-dsl from architecture/project structure references**

Search README.md and CLAUDE.md for any remaining references to `golem-dsl` or the Clojure DSL and remove them. In CLAUDE.md, remove the `golem-dsl/` line from the architecture diagram.

- [ ] **Step 4: Add architecture cross-reference to CLAUDE.md**

Add a line at the end of the Architecture section in CLAUDE.md:

```markdown
For a deeper codebase walkthrough, see [docs/architecture.md](docs/architecture.md).
```

- [ ] **Step 5: Verify all links work**

Check that all markdown links in the Documentation section point to files that exist.

- [ ] **Step 6: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs: add documentation links to README, remove DSL section, update CLAUDE.md"
```
