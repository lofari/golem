# CLAUDE.md

## Project

golem is a Go CLI that orchestrates autonomous Claude Code loops with persistent state. It wraps `claude -p` in a structured iteration loop where each session gets fresh context from `.ctx/` files.

## Build & Test

```bash
go build ./...       # build
go test ./...        # all tests
go run . --help      # run from source
```

## Architecture

```
cmd/           CLI commands (cobra). Each file = one command.
internal/
  runner/      Core loop logic: blueprint engine, builder, reviewer, prompt rendering, validation, events
  server/      HTTP/WebSocket server for UI (state watch, engine event broadcast)
  ctx/         State and log YAML parsing/writing
  config/      Two-layer config system (global + project)
  display/     Plain-text formatters
  scaffold/    golem init scaffolding
  git/         Git helpers (changed files, diff summaries)
  mcp/         MCP server (structured state updates + graph tools)
  graph/       Knowledge graph (embeddings, tree-sitter, LSP, queries)
templates/     Embedded templates: agents/*.yaml, prompts/*.md, state.yaml, log.yaml, claude.md
ui/flutter/    Flutter desktop GUI (Riverpod, xterm.dart, WebSocket)
golem-dsl/     Clojure DSL for agent workflows (experimental)
```

Key interfaces:
- `runner.CommandRunner` — abstracts Claude CLI invocation. Production impl: `ClaudeRunner`.
- `runner.Engine` — blueprint pipeline executor with state management and event emission.
- `runner.EngineEvent` — structured events emitted to log.json and event channels.

## Commands

- `golem code` (alias: `build`) — blueprint engine loop (default), also supports legacy/DSL engines
- `golem run <agent> <goal>` — run a specific blueprint agent with a goal
- `golem agents` — list available agents (built-in + project-local)
- `golem runs [list|attach]` — view/attach to active or recent runs
- `golem review` — single-pass code review
- `golem qa` — autonomous QA testing
- `golem plan` — interactive Claude Code session
- `golem serve` — start HTTP/WebSocket server for UI
- `golem ui` — launch server + Flutter desktop GUI
- `golem config set/get/list` — manage configuration
- `golem status [--watch]` — show project state
- `golem graph build/embed/status` — knowledge graph management

## Config

Two-layer config: `~/.config/golem/config.yaml` (global) < `.ctx/config.yaml` (project). Flags override both.

## Conventions

- Commit messages: `type(scope): description` (feat, fix, refactor, test, docs)
- Tests live next to source: `foo.go` / `foo_test.go`
- No external test frameworks — stdlib `testing` only
- Templates are embedded via `embed.go` in `templates/`
- `--plugin-dir` flag passes local Claude Code plugins through to `claude`
- `golem code` and `golem review` pass `--dangerously-skip-permissions` to `claude -p` (headless, no TTY)
- `golem plan` is interactive and does NOT skip permissions
