# UX Simplification Design

## Problem

Running golem requires too many flags. A typical invocation:

```bash
golem run --max-iterations 20 --verbose --plugin-dir ~/projects/superpowers-main/superpowers-main \
  --sandbox --max-turns 100 --mcp --no-tui
```

The TUI is hard to copy from and loses session output on exit. The tool also needs a framework for new agent modes beyond build and review.

## Design

### 1. Config System

Two-layer config: global defaults and project overrides. Flags override both.

**Global** (`~/.config/golem/config.yaml`):

```yaml
verbose: true
sandbox: true
sandbox-memory: "8g"
sandbox-timeout: "2h"
max-turns: 200
plugin-dir:
  - ~/projects/superpowers-main/superpowers-main
```

**Project** (`.ctx/config.yaml`):

```yaml
max-iterations: 10
sandbox-tools:
  - go
  - node
```

**Resolution order:** flag > project config > global config > built-in default.

**Management commands:**

```bash
golem config set verbose true              # project .ctx/config.yaml
golem config set --global verbose true     # ~/.config/golem/config.yaml
golem config get verbose                   # resolved value + source
golem config list                          # all resolved config
```

**Changed defaults:**

- `max-turns`: 50 -> 200
- TUI: removed entirely

### 2. TUI Removal

**Deleted:**

- `internal/tui/` (entire directory)
- `--no-tui` flag
- Bubble Tea dependency

**Kept:**

- `internal/display/` (plain text formatters, now the only output mode)
- `golem status` (plain text snapshot)
- `.ctx/sessions/` (session output files)

**Enhancement:** `golem status --watch` re-renders every 2 seconds, replacing the live TUI dashboard.

```bash
# Tab 1: agent output
golem code

# Tab 2: live status
golem status --watch
```

### 3. Agent Modes & Renaming

**Rename:** `golem run` -> `golem code` (keep `run` as hidden alias temporarily).

**Modes:**

| Command | Purpose |
|---------|---------|
| `golem plan` | Interactive planning (unchanged) |
| `golem code` | Autonomous builder, writes code |
| `golem review` | Reads code, checks quality against plan |
| `golem qa` | Runs the app, tests user flows |

**Shared across all modes:**

- Core iteration loop
- `.ctx/` state (state.yaml, log.yaml)
- MCP server
- Config system
- Sandbox/plugin-dir/model flags

**Differs per mode:**

- Prompt template (`.ctx/prompt.md`, `.ctx/review-prompt.md`, `.ctx/qa-prompt.md`)
- Default config values
- Outcome handling

**State sharing:** All modes read and write the same `.ctx/state.yaml`. QA finding a bug blocks the task with a reason. Natural feedback loop:

```
golem plan -> golem code -> golem review -> golem qa
                 ^______________v               v
                     (fix issues from review/qa)
```

### 4. Flag Cleanup

**Flags (all modes):**

- `--max-iterations` (default: 20)
- `--max-turns` (default: 200)
- `--task` (force specific task)
- `--dry-run`
- `--verbose`
- `--sandbox`
- `--sandbox-tools`
- `--sandbox-timeout`
- `--sandbox-memory`
- `--mcp` (default: true)
- `--parallel` (code mode only)
- `--review` (code mode only, AFK chaining)

**Removed:** `--no-tui`

**Global flags:** `--model`, `--plugin-dir`

Every flag is configurable via config file. Flags are for one-off overrides only.

**Typical daily use after config setup:**

```bash
golem code
golem code --task "fix auth bug"
golem review
golem qa
```
