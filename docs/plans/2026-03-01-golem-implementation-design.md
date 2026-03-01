# Golem Implementation Design

## Scope

Phase 1 (Scaffolding & Inspection) + Phase 2 (Builder & Reviewer Loops).
Cost tracking deferred to a later phase.

## Tech Stack

- Go
- Cobra (CLI framework)
- gopkg.in/yaml.v3 (YAML handling)
- go:embed (template files)
- No other external dependencies

Module path: `github.com/winler/golem`

## Project Structure

```
golem/
├── cmd/
│   ├── root.go           # Cobra root, version
│   ├── init.go            # golem init
│   ├── plan.go            # golem plan
│   ├── run.go             # golem run
│   ├── review.go          # golem review
│   ├── status.go          # golem status
│   ├── log.go             # golem log
│   ├── decisions.go       # golem decisions
│   ├── pitfalls.go        # golem pitfalls
│   ├── lock.go            # golem lock
│   ├── addtask.go         # golem add-task
│   └── block.go           # golem block
├── internal/
│   ├── ctx/
│   │   ├── state.go       # State types, Read/Write, validation
│   │   ├── state_test.go
│   │   ├── log.go         # Log types, Read/Append
│   │   └── log_test.go
│   ├── runner/
│   │   ├── builder.go     # Builder loop
│   │   ├── reviewer.go    # Review pass
│   │   ├── prompt.go      # Template rendering
│   │   ├── validate.go    # Post-iteration validation
│   │   └── *_test.go
│   ├── scaffold/
│   │   ├── scaffold.go    # Init scaffolding + CLAUDE.md injection
│   │   └── scaffold_test.go
│   ├── git/
│   │   ├── git.go         # Git diff for locked path checks
│   │   └── git_test.go
│   └── display/
│       ├── display.go     # Pretty-print formatting
│       └── display_test.go
├── templates/             # Embedded via go:embed
│   ├── state.yaml
│   ├── log.yaml
│   ├── prompt.md
│   ├── review-prompt.md
│   └── claude.md
├── go.mod
├── go.sum
├── main.go
└── golem-design.md
```

## Data Layer (`internal/ctx`)

### State Types

```go
type State struct {
    Project   Project    `yaml:"project"`
    Status    Status     `yaml:"status"`
    Decisions []Decision `yaml:"decisions"`
    Locked    []Lock     `yaml:"locked"`
    Tasks     []Task     `yaml:"tasks"`
    Pitfalls  []string   `yaml:"pitfalls"`
}

type Project struct {
    Name     string `yaml:"name"`
    Summary  string `yaml:"summary"`
    Stack    string `yaml:"stack"`
    DocsPath string `yaml:"docs_path"`
}

type Status struct {
    CurrentFocus string `yaml:"current_focus"`
    Phase        string `yaml:"phase"`
    LastSession  string `yaml:"last_session"`
}

type Decision struct {
    What string `yaml:"what"`
    Why  string `yaml:"why"`
    When string `yaml:"when"`
}

type Lock struct {
    Path string `yaml:"path"`
    Note string `yaml:"note"`
}

type Task struct {
    Name          string `yaml:"name"`
    Status        string `yaml:"status"`
    Notes         string `yaml:"notes,omitempty"`
    DependsOn     string `yaml:"depends_on,omitempty"`
    BlockedReason string `yaml:"blocked_reason,omitempty"`
}
```

### Log Types

```go
type Log struct {
    Sessions []Session `yaml:"sessions"`
}

type Session struct {
    Iteration     int      `yaml:"iteration"`
    Timestamp     string   `yaml:"timestamp"`
    Task          string   `yaml:"task"`
    Outcome       string   `yaml:"outcome"`
    Summary       string   `yaml:"summary"`
    FilesChanged  []string `yaml:"files_changed"`
    DecisionsMade []string `yaml:"decisions_made"`
    PitfallsFound []string `yaml:"pitfalls_found"`
}
```

### Operations

- `ReadState(dir string) (State, error)` — reads `.ctx/state.yaml` relative to dir
- `WriteState(dir string, s State) error` — writes `.ctx/state.yaml`
- `ValidateState(s State) error` — checks required fields, valid enum values
- `ReadLog(dir string) (Log, error)` — reads `.ctx/log.yaml`
- `AppendSession(dir string, sess Session) error` — reads log, appends session, writes

### Validation Rules

- `project.name` must be non-empty
- `status.phase` must be: planning | building | fixing | polishing
- Each task `status` must be: todo | in-progress | done | blocked
- Blocked tasks must have `blocked_reason`

## Scaffold (`internal/scaffold`)

### `golem init`

Idempotent — skips files that exist, always updates CLAUDE.md section.

1. Create `.ctx/` directory (no-op if exists)
2. Write template files only if they don't exist:
   - `.ctx/state.yaml` (pre-fill name, stack, docs_path from flags)
   - `.ctx/log.yaml`
   - `.ctx/prompt.md`
   - `.ctx/review-prompt.md`
3. Create or update `CLAUDE.md`:
   - If markers `<!-- golem:start -->` / `<!-- golem:end -->` found: replace between them
   - If CLAUDE.md exists without markers: append section
   - If no CLAUDE.md: create with section

Flags: `--name`, `--stack`, `--docs`

## Display (`internal/display`)

Read-only formatting for:

- **`golem status`** — project info, task list with icons (✓ done, ◐ in-progress, ○ todo, ✗ blocked), summary counts
- **`golem log`** — iteration history table. Flags: `--last N`, `--failures`
- **`golem decisions`** — date + description + rationale
- **`golem pitfalls`** — bullet list

## Builder Loop (`internal/runner`)

### `golem run`

1. Read state.yaml — check for non-done tasks
2. If all done → print success, exit
3. Render prompt: read `.ctx/prompt.md`, replace template variables
4. Spawn `claude -p "<prompt>" --max-turns N`
5. Stream stdout/stderr live to terminal, also buffer for COMPLETE detection
6. Wait for process exit
7. Run post-iteration checks
8. Increment iteration, check max → loop or exit

### Prompt Rendering

Reads `.ctx/prompt.md` from disk (user may customize). Replaces:
- `{{DOCS_PATH}}` → `project.docs_path` from state.yaml
- `{{ITERATION_CONTEXT}}` → "You are on iteration X of Y. There are Z tasks remaining."
- `{{TASK_OVERRIDE}}` → task override text if `--task` flag, empty otherwise

### Post-Iteration Validation

1. **Schema validation** — re-parse state.yaml. Halt if corrupted.
2. **Locked path detection** — `git diff --name-only HEAD~1`, check against locked paths. Warn.
3. **Task regression** — compare before/after task statuses. Warn if done → non-done.
4. **Thrashing** — check log for same task in-progress 3+ consecutive iterations. Warn.

### Flags

- `--max-iterations N` (default: 20)
- `--max-turns N` (default: 50, passed to claude)
- `--task "name"` — force specific task
- `--review` — chain review after builder loop
- `--dry-run` — show rendered prompt, don't execute
- `--verbose` — extra detail in output

### Error Handling

- Claude Code crashes → log error, continue to next iteration
- State corrupted (unparseable) → halt loop
- State not modified → warn, continue
- Locked path violation → warn, continue
- Task regression → warn, continue
- Thrashing (3+ iterations same task) → warn, continue

## Reviewer (`internal/runner`)

### `golem review`

1. Count existing `[review]` tasks in state.yaml
2. Read `.ctx/review-prompt.md`, render `{{DOCS_PATH}}`
3. Spawn `claude -p "<prompt>" --max-turns N`
4. Stream output live
5. Scan for `<promise>APPROVED</promise>` or `<promise>NEEDS_WORK</promise>`
6. Count new `[review]` tasks, compare to pre-review count
7. Print result with comparison

## Plan Command

1. Check `.ctx/` exists
2. Spawn `claude` (interactive, no `-p`)
3. Wait for user to exit

## State Manipulation Commands

- **`golem lock <path>`** — append to locked in state.yaml. Optional `--note`.
- **`golem add-task "desc"`** — append task with status todo. Optional `--depends-on`.
- **`golem block <task-name> "reason"`** — set task status to blocked with reason.

## Deferred (Phase 3+4)

- `golem reset` — archive and re-scaffold
- `golem rollback --to-iteration N` — git revert + state reconstruction
- Cost tracking (`--max-cost`, token parsing)
- `--output-format stream-json` for structured output
