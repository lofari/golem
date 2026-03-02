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
  runner/      Core loop logic: builder, reviewer, prompt rendering, validation, events
  ctx/         State and log YAML parsing/writing
  tui/         Bubbletea models for run and status
  display/     Plain-text formatters (non-TUI fallback)
  scaffold/    golem init scaffolding
  git/         Git helpers (changed files, locked path checks)
templates/     Embedded templates: prompt.md, review-prompt.md, state.yaml, log.yaml, claude.md
```

Key interfaces:
- `runner.CommandRunner` — abstracts Claude CLI invocation. Production impl: `ClaudeRunner`.
- `runner.Event` — emitted by builder loop, consumed by TUI via channel.

## Conventions

- Commit messages: `type(scope): description` (feat, fix, refactor, test, docs)
- Tests live next to source: `foo.go` / `foo_test.go`
- No external test frameworks — stdlib `testing` only
- Templates are embedded via `embed.go` in `templates/`
- TUI defaults on when a terminal is detected; `--no-tui` disables it
- `--plugin-dir` flag passes local Claude Code plugins through to `claude`
- `golem run` and `golem review` pass `--dangerously-skip-permissions` to `claude -p` (headless, no TTY)
- `golem plan` is interactive and does NOT skip permissions
