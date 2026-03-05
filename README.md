# golem

Goal-Oriented Loop Execution Manager — a CLI that runs autonomous AI coding agent loops with persistent state across iterations.

golem wraps [Claude Code](https://docs.anthropic.com/en/docs/claude-code) in a structured loop where each iteration gets fresh context from filesystem state. The agent reads design docs, picks a task, implements it, updates state, and repeats until done.

## Why

Running an AI agent in a loop without structure leads to:
- Redoing work or reversing earlier decisions
- Losing track of what's done vs remaining
- Re-explaining context every session
- Conflicting architectural choices across iterations
- No visibility into what happened

golem solves this with three information layers:
1. **Design docs** — Static intent (user-written plans and specs)
2. **State** (`.ctx/state.yaml`) — Live progress: tasks, decisions, locked paths, pitfalls
3. **Log** (`.ctx/log.yaml`) — Append-only session history

The agent reads all three at the start of every iteration and updates state/log at the end. No conversation history needed.

## Install

```bash
go install github.com/lofari/golem@latest
```

Or build from source:

```bash
git clone https://github.com/lofari/golem.git
cd golem
go build -o golem .
```

Requires [Claude Code](https://docs.anthropic.com/en/docs/claude-code) (`claude` CLI) on your PATH.

## Quick Start

```bash
# Initialize project context
golem init --name "MyProject" --stack "Go, React" --docs "docs/"

# Plan interactively (opens Claude Code session)
golem plan

# Run the autonomous builder loop
golem code

# Review the result
golem review

# Run QA testing
golem qa

# Check status anytime
golem status
```

### Permissions

`golem code` and `golem review` pass `--dangerously-skip-permissions` to Claude Code, giving the agent unrestricted tool access. This is necessary because headless mode (`claude -p`) has no TTY to prompt for approval.

Use `--sandbox` to run Claude inside a [Warden](https://github.com/anthropics/warden) container, isolating filesystem and network access:

```bash
golem code --sandbox
golem review --sandbox
```

Without `--sandbox`, run golem in an isolated environment (Docker container, VM, or disposable worktree) to limit blast radius.

## Workflow

```
          golem init
              |
          golem plan        <-- interactive: create design docs, populate tasks
              |
          golem code        <-- autonomous: agent loops until all tasks done
              |
          golem review      <-- autonomous: read-only code review
             / \
     APPROVED   NEEDS_WORK
        |           |
      done      golem code  <-- agent fixes review issues, then re-review
                    |
                golem qa     <-- autonomous: test user flows, report bugs
```

### AFK mode

```bash
golem code --review
```

Runs the builder loop and automatically chains a review pass when done.

## Configuration

golem supports a two-layer config system. Settings are resolved in order: **defaults < global < project < flags**.

```bash
# Set global defaults
golem config set --global max-tool-calls 300
golem config set --global sandbox true

# Set project-specific overrides
golem config set verbose true

# View resolved config
golem config list

# Get a single value
golem config get max-tool-calls
```

| File | Scope |
|------|-------|
| `~/.config/golem/config.yaml` | Global defaults |
| `.ctx/config.yaml` | Project overrides |

Any flag passed on the command line overrides both config files.

## Commands

### `golem init`

Creates the `.ctx/` directory and injects conventions into `CLAUDE.md`.

```bash
golem init --name "MyProject" --stack "Kotlin, Go" --docs "docs/plans/"
```

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | `""` | Project name |
| `--stack` | `""` | Tech stack |
| `--docs` | `"docs/"` | Path to design/implementation docs |

Creates: `.ctx/state.yaml`, `.ctx/log.yaml`, `.ctx/prompt.md`, `.ctx/review-prompt.md`, `.ctx/qa-prompt.md`, and a `CLAUDE.md` section.

### `golem plan`

Opens an interactive Claude Code session for planning. The agent has access to `.ctx/` conventions via `CLAUDE.md` and can create design docs, populate tasks, and set up the project.

```bash
golem plan
golem plan --model opus
```

### `golem code`

The core loop. Spawns autonomous Claude Code iterations until all tasks are done or limits are reached. Also available as `golem run` (alias).

```bash
golem code
golem code --max-iterations 10 --model sonnet
golem code --task "WebSocket reconnection"
golem code --dry-run
golem code --review
```

| Flag | Default | Description |
|------|---------|-------------|
| `--max-iterations` | `20` | Maximum number of iterations |
| `--max-tool-calls` | `200` | Max turns per Claude Code session |
| `--task` | `""` | Force agent to work on a specific task |
| `--dry-run` | `false` | Show rendered prompt without executing |
| `--verbose` | `false` | Extra output detail |
| `--review` | `false` | Chain a review pass after builder completes |
| `--sandbox` | `false` | Run Claude inside a warden sandbox container |
| `--mcp` | `true` | Enable golem MCP server for structured state updates |
| `--parallel` | `1` | Max parallel task sessions (1 = sequential) |

Each iteration:
1. Reads state and remaining tasks
2. Snapshots state for rollback protection
3. Renders prompt template with iteration context
4. Spawns `claude -p` with the rendered prompt (and MCP server if enabled)
5. Checks for `<promise>COMPLETE</promise>` in output
6. Validates post-iteration (schema, locked paths, regressions, thrashing)
7. Prints summary and continues

### `golem review`

Single-pass code review. Spawns Claude Code in a read-only reviewer role. Does not modify code — only reads the codebase and writes `[review]` tasks to `state.yaml`.

```bash
golem review
golem review --model opus
```

| Flag | Default | Description |
|------|---------|-------------|
| `--max-tool-calls` | `200` | Max turns for the review session |
| `--sandbox` | `false` | Run Claude inside a warden sandbox container |

The reviewer checks: plan alignment, implementation completeness, test quality, code quality, decision consistency, and pitfall awareness. Issues become `[review]` tasks that the builder picks up on the next `golem code`.

### `golem qa`

Autonomous QA testing. Spawns Claude Code as a QA tester that builds and runs the application, tests user flows from design docs, tries edge cases, and reports bugs as `[qa]` tasks.

```bash
golem qa
golem qa --task "test auth flow"
golem qa --sandbox
```

| Flag | Default | Description |
|------|---------|-------------|
| `--max-iterations` | `20` | Maximum number of iterations |
| `--max-tool-calls` | `200` | Max turns per session |
| `--task` | `""` | Force agent to test a specific area |
| `--sandbox` | `false` | Run Claude inside a warden sandbox container |

### `golem status`

Pretty-prints current project state. Use `--watch` to continuously refresh every 2 seconds.

```bash
golem status
golem status --watch
```

```
Project: MyProject
Phase: building
Focus: competitor price tracking

Tasks:
  ✓ auth module
  ◐ price tracking — "scraping works, need pagination"
  ○ price history charts (depends on: price tracking)
  ✗ shipping integration — blocked: "external API pending"

Decisions: 4 recorded
Pitfalls: 3 noted
Locked paths: 2
Sessions: 7 logged
```

### `golem config`

Manage golem configuration.

```bash
golem config set max-tool-calls 300          # set in project config
golem config set --global sandbox true  # set in global config
golem config get max-tool-calls              # show resolved value
golem config list                       # show all resolved values
```

### `golem log`

Shows iteration history.

```bash
golem log
golem log --last 5
golem log --failures
```

| Flag | Default | Description |
|------|---------|-------------|
| `--last` | `0` (all) | Show only last N entries |
| `--failures` | `false` | Show only blocked/unproductive sessions |

### `golem decisions`

Lists architectural decisions with rationale.

```
2026-02-25  Use IndexedDB for price history storage
            → keep it working offline, avoid server costs
```

### `golem pitfalls`

Lists discovered pitfalls.

```
• Don't batch more than 5 requests to ML — triggers captcha
• ML pagination is infinite scroll, not page links
```

### `golem add-task`

```bash
golem add-task "implement auth module"
golem add-task "price charts" --depends-on "price tracking"
```

### `golem lock`

```bash
golem lock src/auth/ --note "auth flow is complete and tested"
```

### `golem block`

```bash
golem block "shipping integration" "external API schema pending"
```

## Global Flags

| Flag | Description |
|------|-------------|
| `--model` | Claude model to use (`sonnet`, `opus`, `haiku`) |
| `--plugin-dir` | Local plugin directory to pass to Claude (repeatable) |
| `--version` | Print version |

```bash
golem --model opus code --max-iterations 10
golem --model sonnet review
golem --plugin-dir ~/my-plugin code
```

### Using with golem-superpowers

[golem-superpowers](https://github.com/lofari/golem-superpowers) is a companion Claude Code plugin that adds workflow skills (TDD, debugging, planning) tuned for golem iterations. Load it via `--plugin-dir`:

```bash
golem --plugin-dir ~/projects/golem-superpowers code
```

If the agent has both `superpowers` and `golem-superpowers` installed, it will prefer the golem-aware variants automatically.

## Project Structure

```
your-project/
├── .ctx/
│   ├── state.yaml          # Current state (tasks, decisions, locks, pitfalls)
│   ├── log.yaml            # Append-only session history
│   ├── config.yaml         # Project-specific config overrides
│   ├── prompt.md           # Builder prompt template (customizable)
│   ├── review-prompt.md    # Review prompt template (customizable)
│   ├── qa-prompt.md        # QA prompt template (customizable)
│   ├── snapshots/          # Auto-managed state snapshots for rollback
│   ├── sessions/           # Raw session output from each iteration
│   └── mcp_servers.json    # Auto-generated MCP config (when --mcp is enabled)
├── CLAUDE.md               # Injected conventions (golem section)
└── docs/                   # Your design and implementation docs
```

### State (`state.yaml`)

```yaml
project:
  name: "MyProject"
  summary: "A brief description"
  stack: "Go, React"
  docs_path: "docs/"

status:
  current_focus: "what agent is working on"
  phase: building          # planning | building | fixing | polishing
  last_session: "2026-03-01"

decisions:
  - what: "Use PostgreSQL for persistence"
    why: "Need relational queries for price history"
    when: "2026-02-25"

locked:
  - path: "src/auth/"
    note: "complete and tested"

tasks:
  - name: "implement auth"
    status: done           # todo | in-progress | done | blocked
  - name: "price tracking"
    status: in-progress
    notes: "scraping works, need pagination"
  - name: "price charts"
    status: todo
    depends_on: "price tracking"
  - name: "shipping"
    status: blocked
    blocked_reason: "external API schema pending"

pitfalls:
  - "Don't batch more than 5 requests — triggers captcha"
```

### Log (`log.yaml`)

```yaml
sessions:
  - iteration: 1
    timestamp: "2026-03-01T14:30:00Z"
    task: "implement auth"
    outcome: done            # done | partial | blocked | unproductive
    summary: "implemented JWT auth with refresh tokens"
    files_changed:
      - "src/auth/handler.go"
      - "src/auth/handler_test.go"
    decisions_made:
      - "Use JWT with 15min expiry"
    pitfalls_found: []
```

## Prompt Templates

Templates in `.ctx/` are customizable. They use three variables:

| Variable | Description |
|----------|-------------|
| `{{DOCS_PATH}}` | Path to design docs (from `state.yaml`) |
| `{{ITERATION_CONTEXT}}` | Auto-generated: "Iteration X of Y, Z tasks remaining" |
| `{{TASK_OVERRIDE}}` | Injected when `--task` flag is used |

Edit `.ctx/prompt.md`, `.ctx/review-prompt.md`, or `.ctx/qa-prompt.md` to customize agent behavior.

## MCP Server

golem includes a built-in [MCP](https://modelcontextprotocol.io/) server that gives the agent structured tools for updating state. Instead of editing YAML directly (which risks corruption), the agent calls typed tools:

| Tool | Description |
|------|-------------|
| `mark_task` | Update a task's status and notes |
| `set_phase` | Change the project phase |
| `add_decision` | Record an architectural decision |
| `add_pitfall` | Record a lesson learned |
| `add_locked` | Lock a completed module path |
| `log_session` | Append a session entry to the log |

The MCP server is enabled by default (`--mcp`). golem writes a temporary `mcp_servers.json` and passes it to Claude via `--mcp-config`. The server runs as a subprocess (`golem mcp-serve`) using stdio transport and uses file locking to prevent concurrent writes.

If MCP tools are unavailable (e.g. older Claude Code version), the agent falls back to direct YAML editing.

## State Snapshots

Before each iteration, golem snapshots `state.yaml` to `.ctx/snapshots/`. If the agent corrupts state beyond repair, golem automatically restores the latest snapshot and continues instead of halting the loop.

- Snapshots are named `state-NNN.yaml` (one per iteration)
- Only the 5 most recent snapshots are kept (older ones are pruned)
- Restoration happens during post-iteration validation, before the corruption halt

This provides resilience against agent errors without losing progress.

## Parallel Execution

With `--parallel N` (where N > 1), golem can run multiple tasks concurrently using git worktrees:

```bash
golem code --parallel 3
```

Each parallel iteration:
1. Identifies eligible tasks (todo/in-progress, dependencies resolved)
2. Creates a git worktree per task with its own branch (`golem/<task-name>`)
3. Spawns independent Claude sessions in each worktree
4. Merges completed branches back in alphabetical order
5. Aborts and retries on merge conflicts (next iteration)
6. Cleans up worktrees and branches

Parallel execution requires the project to be a git repository. Tasks with unresolved dependencies are automatically excluded from parallel batches.

## Safety

golem runs post-iteration validation after every builder iteration:

| Check | Severity | Trigger |
|-------|----------|---------|
| Schema validation | **Halts loop** | `state.yaml` fails to parse or has invalid values |
| State snapshot restore | Auto-recovery | State corrupted beyond repair — restores from snapshot |
| Locked path violation | Warning | Agent modified files under a locked path |
| Task regression | Warning | Task status went from `done` to non-done |
| Thrashing detection | Warning | Same task in-progress for 3+ consecutive iterations |

Signal handling: `SIGINT`/`SIGTERM` gracefully cancel the current iteration and stop the loop.

## Design Principles

- **Fresh context every iteration** — No conversation history, only filesystem state
- **Agent maintains state** — No external tool modifies state between iterations
- **Flat YAML** — Human-readable, scannable in 10 seconds
- **Decisions have "why"** — Prevents agent from rationalizing contradictions
- **Locked paths** — Completed modules stay untouched
- **Append-only log** — Full visibility into what happened

## License

MIT
