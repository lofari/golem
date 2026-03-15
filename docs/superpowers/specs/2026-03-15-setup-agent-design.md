# Setup Agent Design

## Overview

A guided setup experience that auto-detects project characteristics and configures golem interactively. Two entry points: `golem init` (CLI) and `golem ui` (auto-triggers if no `.ctx/`). Replaces the current silent `golem init` with an intelligent, conversational setup.

## Problem

Today, after `golem init`, users must manually run `golem config set test-cmd "..."`, `golem config set lint-cmd "..."`, etc. Most of these values are discoverable from the project's existing files. Users shouldn't have to tell golem what a Makefile already says.

## Design

### Execution model: Direct interactive session (not blueprint engine)

**Critical design decision:** The setup agent runs as a direct interactive Claude session (like `golem plan`), NOT through the blueprint engine. This is because:

1. The blueprint engine runs `claude -p --dangerously-skip-permissions` in headless mode with no stdin — the user cannot respond to questions.
2. Setup requires back-and-forth conversation — the user needs to confirm or adjust proposals.
3. The `golem plan` command already implements the interactive pattern: `claude -p` without `--dangerously-skip-permissions`, with full TTY access.

The flow is:
1. `golem setup` spawns `claude -p <setup-prompt> --max-turns 30` (interactive, with permissions)
2. Claude reads the project, proposes config, negotiates with the user
3. Claude writes `session-output.json` with the final config proposal
4. A Go post-processor reads `session-output.json` and routes values to the correct destinations

### Agent behavior

The setup session:

1. Scans the filesystem for project signals (build files, CI configs, lint dotfiles)
2. Classifies the project richness:
   - **Rich project** (has manifest + build system) → full auto-detection, propose-and-confirm
   - **Sparse project** (has code files but no build system) → detect language from extensions, ask about build/test/lint
   - **Empty project** (no code) → ask what the user is building, set minimal config (stack + agent), skip test/lint, advise re-running setup after adding code
3. Presents findings and proposed config to the user
4. Negotiates — user can confirm, tweak, or ask questions
5. Writes final config values to `session-output.json`

### Detection matrix

| Signal | Source files | Config key | Destination |
|--------|-------------|------------|-------------|
| Language/stack | go.mod, package.json, Cargo.toml, pyproject.toml, *.csproj, mix.exs, file extensions | stack | `.ctx/state.yaml` project.stack |
| Test command | Makefile targets, package.json scripts, CI workflow steps, convention | test-cmd | `.ctx/config.yaml` |
| Lint command | Makefile targets, .eslintrc*, .golangci-lint.yml, ruff.toml, .flake8, CI steps | lint-cmd | `.ctx/config.yaml` |
| Lint fix command | eslint --fix, golangci-lint run --fix, ruff check --fix | lint-fix-cmd | `.ctx/config.yaml` |
| CI present | .github/workflows/*.yml, .gitlab-ci.yml, Jenkinsfile | ci-enabled | `.ctx/config.yaml` |
| Default branch | git symbolic-ref refs/remotes/origin/HEAD, or "main" | (used by git-setup) | — |
| Agent recommendation | repo file count, existing task structure | agent | `.ctx/config.yaml` |
| Sandbox available | `which warden` exit code | sandbox | `.ctx/config.yaml` |
| Model | user preference or omit (use Claude default) | model | `.ctx/config.yaml` |
| Graph worthwhile | file count > 100 | (advisory) | offers to run `golem graph build` |

### session-output.json schema

Claude writes this at the end of the conversation:

```json
{
  "config": {
    "test-cmd": "go test ./...",
    "lint-cmd": "golangci-lint run",
    "lint-fix-cmd": null,
    "ci-enabled": true,
    "sandbox": false,
    "agent": "build-feature",
    "model": ""
  },
  "state": {
    "stack": "go",
    "name": "myproject"
  },
  "graph": false
}
```

- `config` keys → written to `.ctx/config.yaml` via `config.SetValue`
- `state` keys → written to `.ctx/state.yaml` via `ctx.WriteState` (stack → project.stack, name → project.name)
- `graph` → if true, post-processor runs `golem graph build && golem graph embed`
- Null values are skipped (not written)

### Post-processor: `applySetupOutput`

A Go function in `cmd/setup.go` that runs after the Claude session completes:

```go
func applySetupOutput(dir string) error
```

1. Reads `session-output.json` from `dir`
2. Deletes the file after reading
3. For each key in `config`: calls `config.SetValue(config.ProjectPath(dir), key, value)`
4. For keys in `state`: reads state.yaml, sets fields, writes back
5. If `graph` is true: runs `golem graph build` and `golem graph embed` as subprocesses
6. Prints summary: "Configured: test-cmd, lint-cmd, ci-enabled. Stack: go."

### Prompt template (`templates/prompts/setup.md`)

Embedded template that instructs Claude to:

1. List files in the project root and key directories (ls, not recursive)
2. Read build files (Makefile, package.json, go.mod, etc.) if present
3. Check for CI configs (.github/workflows/)
4. Check for lint tool dotfiles
5. Classify the project (rich / sparse / empty)
6. For rich projects: present detected values and ask "Does this look right?"
7. For sparse/empty: ask focused questions about stack and tooling
8. Cover: test command, lint command, CI, sandbox preference, agent choice
9. At the end, write `session-output.json` with the agreed config

The prompt explicitly states:
- Do NOT modify any project files except session-output.json
- Do NOT install packages or run build commands
- Focus on detection and conversation only

### CLI command: `golem setup`

New standalone command:

```
golem setup [--max-turns N]
```

Requires `.ctx/` to exist (run `golem init` first, or `golem init` calls this automatically).

Implementation:
1. Render the setup prompt template
2. Spawn `claude -p <prompt> --max-turns 30` (interactive, TTY attached)
3. Wait for completion
4. Call `applySetupOutput(dir)`
5. Print summary

### CLI integration: `golem init`

Current scaffold behavior preserved. New behavior added after:

```
golem init [--name NAME] [--stack STACK] [--no-setup]
```

Flow:
1. Scaffold `.ctx/` directory and template files (existing logic)
2. Inject CLAUDE.md section (existing logic)
3. If `--no-setup` flag OR non-TTY stdin: stop here (backward compatible)
4. If `claude` CLI not found: print "Install Claude CLI for auto-configuration" and stop
5. Otherwise: run `golem setup` inline

### GUI integration: `golem ui`

In `cmd/ui.go`, when no `.ctx/` exists:

1. Run scaffold logic (same as `golem init --no-setup`) to create `.ctx/`
2. Register the project with the server
3. Launch `setup` as a managed process via `launchProcess(proj, LaunchRequest{Command: "setup"})`
4. The setup conversation appears in the terminal pane — user interacts there
5. When the process exits, the config is written and the project is ready

No new Flutter widgets needed. The `setup` command is added to the valid process commands.

### Additions to valid commands

In `internal/server/process.go`:
```go
validCommands := map[string]bool{"code": true, "review": true, "qa": true, "plan": true, "setup": true}
```

### What this does NOT do

- No new Flutter widgets — setup runs in the existing terminal pane
- No new event protocol — standard PTY streaming
- No blueprint engine changes — setup bypasses the engine entirely
- No forced setup — `golem code` still works if you configure manually
- No network calls during setup — all detection is local filesystem reads

### Edge cases

- **User cancels mid-setup (Ctrl+C)**: No `session-output.json` written, no config changes. `.ctx/` exists with defaults. User can re-run `golem setup`.
- **Claude writes malformed output**: `applySetupOutput` validates JSON structure, logs warning for unrecognized keys, skips invalid values. Prints what was configured and what was skipped.
- **No Claude CLI installed**: `golem init` scaffolds but skips setup with message: "Install Claude CLI for auto-configuration, or configure manually with `golem config set`."
- **Already configured**: `golem setup` on a configured project reads existing `.ctx/config.yaml` and injects current values into the prompt so Claude can say "I see you already have test-cmd=go test ./.... Want to keep or change?"
- **Non-TTY environment**: `golem init` detects non-TTY stdin and skips setup automatically (equivalent to `--no-setup`).

### Files to create or modify

| Action | Path | Purpose |
|--------|------|---------|
| Create | `templates/prompts/setup.md` | Detection + conversation prompt |
| Create | `cmd/setup.go` | `golem setup` command + `applySetupOutput` |
| Modify | `cmd/init.go` | Add `--no-setup` flag, chain to `golem setup` |
| Modify | `cmd/ui.go` | Auto-scaffold + launch setup process if no .ctx/ |
| Modify | `internal/server/process.go` | Add "setup" to valid commands |
| Create | `cmd/setup_test.go` | Test applySetupOutput with mock session-output.json |
