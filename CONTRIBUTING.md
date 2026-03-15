# Contributing to golem

## Getting Started

### Prerequisites
- Go 1.22+
- git

### Build & Test

```bash
go build ./...       # build
go test ./...        # all tests
go run . --help      # run from source
```

## Code Conventions

### Commit Messages
Follow the format: `type(scope): description`

Types:
- `feat` — new feature
- `fix` — bug fix
- `refactor` — code refactoring
- `test` — test additions or changes
- `docs` — documentation

Example: `feat(runner): add blueprint validation`

### Tests
- Tests live next to source: `foo.go` / `foo_test.go`
- Use stdlib `testing` only — no external test frameworks
- Write tests for new functionality
- All tests must pass before submitting a PR

### Templates
- Embed templates via `embed.go` in `templates/`
- Keep templates simple and focused

### General
- Prefer simple, direct code over abstractions
- Keep functions focused and readable
- Document exported APIs

## Pull Request Process

1. Branch from `main`
2. One feature or fix per PR
3. Write tests for new functionality
4. All tests must pass: `go test ./...`
5. Update docs if behavior changes
6. Push and open a PR with a clear description

## Project Layout

- `cmd/` — CLI commands (cobra). Each file = one command.
- `internal/runner/` — Core engine: blueprint parsing, pipeline execution, primitives, predicates, events, error handling, validation.
- `internal/server/` — HTTP/WebSocket server for desktop UI (state watch, event broadcast).
- `internal/ctx/` — State and log YAML parsing/writing.
- `internal/config/` — Two-layer config (global `~/.config/golem/config.yaml` + project `.ctx/config.yaml`).
- `internal/graph/` — Knowledge graph: tree-sitter indexing, embeddings, LSP, query pipeline.
- `internal/mcp/` — MCP server for structured state updates and graph tools.
- `internal/display/` — Plain-text formatters for terminal output.
- `internal/scaffold/` — `golem init` project scaffolding.
- `internal/git/` — Git helpers (changed files, diff summaries).
- `templates/` — Embedded templates: agent blueprints, prompt templates, state/log schemas.
- `ui/flutter/` — Flutter desktop GUI (Riverpod, xterm.dart, WebSocket).

## Further Reading

- [Architecture Guide](docs/architecture.md) for a deeper codebase walkthrough.
