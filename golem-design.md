# GOLEM — Design & Implementation Document
### Goal-Oriented Loop Execution Manager

## Overview

`golem` is a personal CLI tool that runs autonomous AI coding agent loops (inspired by the Ralph Loop pattern) with a structured state layer that maintains context across iterations. It solves the core problem of information loss at iteration boundaries — where fresh agent sessions lose track of decisions, progress, pitfalls, and architectural intent.

### Core Principles

- **Fresh context every iteration.** Each loop iteration spawns a new Claude Code session with clean context. No conversation history carries over. This eliminates context rot.
- **State lives in the filesystem.** Progress, decisions, pitfalls, and project context persist in structured files (`.ctx/`), not in the LLM's memory.
- **The agent maintains its own state.** The agent reads and updates `.ctx/state.yaml` as part of its work. No external tooling analyzes or modifies state between iterations — the agent is responsible.
- **Three information layers.** The state (current truth), the log (full history), and the plan (design intent) serve different purposes and have different lifecycles.
- **Minimal friction.** The tool does the mechanical work (spawning sessions, checking completion, scaffolding). The human focuses on planning and review.

### Problems Solved

| Problem | How golem addresses it |
|---|---|
| Agent redoes work or reverses past decisions | `decisions` registry with rationale; `locked` paths the agent must not touch |
| Losing track of what's done vs remaining | `tasks` list with status tracking in `state.yaml` |
| Re-explaining context every new session | Agent reads `state.yaml` + `plan.md` at start of every iteration |
| Agent makes conflicting architectural choices | `decisions` include *why*, so the agent understands the reasoning and won't override |
| No visibility into what happened across iterations | Append-only `log.yaml` with per-session structured records |

---

## Architecture

### File Structure

Every project using `golem` has a `.ctx/` directory at the project root:

```
project-root/
├── .ctx/
│   ├── state.yaml       # Current project state (agent reads/writes)
│   ├── log.yaml         # Append-only session history
│   ├── prompt.md        # Prompt template for each loop iteration
│   └── review-prompt.md # Prompt template for review pass
├── docs/plans/          # Design & implementation docs (path configured in state.yaml)
│   ├── 2026-02-26-comisio-design.md
│   └── 2026-02-26-comisio-implementation.md
├── CLAUDE.md            # Claude Code instructions (golem injects a section)
└── ... (project files)
```

Note: The docs location is not owned by `golem`. It's whatever path your project already uses, referenced via `project.docs_path` in `state.yaml`. The example above uses `docs/plans/` but it could be `specs/`, `docs/`, or anything else.

### Information Layers

**Layer 1: The Docs (referenced by `project.docs_path`)**
- Design documents and implementation plans created during interactive planning sessions
- Can include separate design docs (architecture, data models, flows) and implementation docs (ordered tasks, steps)
- Written collaboratively by the human and agent
- `golem` does not manage these files — it just tells the agent where to find them
- The agent reads ALL files in the docs path for full design context

**Layer 2: The State (`state.yaml`)**
- The live, structured truth about where the project stands
- Agent reads it at the start of every iteration
- Agent updates it at the end of every iteration
- Contains: project info, current focus, tasks, decisions, locked paths, pitfalls
- Updated in place (not append-only)

**Layer 3: The Log (`log.yaml`)**
- Append-only session history
- One entry per iteration with structured data
- Never deleted, never modified
- Used by the human to review what happened
- Used by `golem` tooling for inspection commands
- The agent appends to this at the end of each session

### How They Relate

```
docs/*.md ──→ define WHAT to build and HOW (design + implementation docs)
                │
                ▼
state.yaml ──→ tracks WHERE we are (dynamic, agent-maintained)
                │
                ▼
log.yaml ───→ records WHAT HAPPENED (append-only, historical)
```

The docs are the source of truth for intent and approach. The state is derived from executing against the docs. The log is a record of execution. If state ever gets corrupted, a human can review the log and docs to reconstruct it.

---

## State Schema

### `.ctx/state.yaml`

```yaml
# Project identity — written once during init/planning
project:
  name: ""
  summary: ""
  stack: ""
  docs_path: "docs/"  # path to design + implementation docs (relative to project root)

# Current session focus — updated every iteration
status:
  current_focus: ""
  phase: planning  # planning | building | fixing | polishing
  last_session: ""

# Architectural decisions — append only, never delete
# Agent adds new decisions during work, includes rationale
decisions: []
  # - what: "description of the decision"
  #   why: "rationale for the decision"
  #   when: "YYYY-MM-DD"

# Completed modules — agent must not modify these paths
locked: []
  # - path: "src/auth/"
  #   note: "auth flow is complete and tested"

# Work items — agent picks from these, updates status
tasks: []
  # - name: "task description"
  #   status: todo        # todo | in-progress | done | blocked
  #   notes: ""           # optional context
  #   depends_on: ""      # optional dependency on another task name
  #   blocked_reason: ""  # if status is blocked

# Lessons learned — things that went wrong, gotchas discovered
pitfalls: []
  # - "description of something to avoid or watch out for"
```

### Design Decisions on the Schema

**Why flat YAML, not JSON?**
Human-readable and human-editable. You should be able to open `state.yaml`, scan it in 10 seconds, and know where things stand. YAML comments help too.

**Why `decisions` have `why`?**
This is the key to preventing conflicting architectural choices. If the agent sees *why* a decision was made, it won't rationalize overriding it. "Use IndexedDB" is weak. "Use IndexedDB because we need offline support and want to avoid server costs" is strong.

**Why `locked` is path-based?**
Simple and enforceable. The prompt tells the agent "do not modify files under locked paths." As modules stabilize, they get locked. This prevents the agent from refactoring working code.

**Why `pitfalls` are unstructured strings?**
They're tribal knowledge. "ML pagination is infinite scroll, not page links" doesn't need schema. It just needs to be seen by the agent before it makes implementation choices.

**Why no task IDs?**
This is a personal tool, not a project management system. Task names are unique enough. If you need to reference a task, use its name. Keeping it simple means lower friction for both you and the agent.

---

## Log Schema

### `.ctx/log.yaml`

```yaml
sessions:
  - iteration: 1
    timestamp: "2026-02-27T14:30:00Z"
    task: "task name or description of what was worked on"
    outcome: done       # done | partial | blocked | unproductive
    summary: "brief description of what happened"
    files_changed:
      - "src/services/payment.ts"
      - "tests/payment.test.ts"
    decisions_made:
      - "description of any new decision"
    pitfalls_found:
      - "description of any new pitfall"
```

### Log Design Notes

- Each session appends one entry. The log grows but each entry is small.
- `outcome` has four values: `done` (task completed), `partial` (progress made), `blocked` (couldn't proceed), `unproductive` (thrashed, no meaningful progress).
- The agent writes this entry as part of its end-of-session state update.
- The log is committed to git along with the code changes.

---

## Prompt Template

### `.ctx/prompt.md`

This is the prompt fed to Claude Code on each loop iteration:

```markdown
You are working on this project autonomously as part of a loop.
Each iteration you get fresh context — you have no memory of previous iterations.
All persistent state is in `.ctx/`.

{{ITERATION_CONTEXT}}

## Start of Session
1. Read all design and implementation docs in `{{DOCS_PATH}}` for project context.
2. Read `.ctx/state.yaml` for current progress, decisions, and constraints.
3. Respect ALL entries in `decisions` — do not contradict them without exceptional reason.
4. Do NOT modify files under paths listed in `locked`.
5. Review `pitfalls` before making implementation choices.

## During Session
{{TASK_OVERRIDE}}
1. Pick ONE task from `tasks` (prefer `in-progress` over `todo`).
2. If a task depends on another task that isn't `done`, skip it.
3. Find the matching `## Task` section in the implementation doc for detailed steps and code.
4. Follow the implementation doc's steps for this task. Write tests. Make sure they pass.
5. Commit your work with clear commit messages.

## End of Session
Before exiting, update `.ctx/state.yaml`:
1. Update the task you worked on (status, notes).
2. Mark task as `done` if fully complete and tested.
3. Add any new `decisions` with `what`, `why`, and `when`.
4. Add any new `pitfalls` discovered.
5. Add to `locked` any completed, tested modules that should not be modified.
6. Update `status.current_focus` and `status.last_session` with today's date and summary.

Then append a session entry to `.ctx/log.yaml` under `sessions:`:
- iteration: (increment from last entry)
- timestamp: (current ISO timestamp)
- task: (what you worked on)
- outcome: done | partial | blocked | unproductive
- summary: (brief description)
- files_changed: (list of files you modified)
- decisions_made: (list, if any)
- pitfalls_found: (list, if any)

## Completion
If ALL tasks in `state.yaml` have status `done`, output:
<promise>COMPLETE</promise>
```

### Prompt Template Variables

The loop engine replaces template variables before passing the prompt to Claude Code:

**`{{DOCS_PATH}}`** — Replaced with the value of `project.docs_path` from `state.yaml`. Tells the agent where to find design and implementation documents.

**`{{ITERATION_CONTEXT}}`** — Injected by `golem run` each iteration:
```
You are on iteration 7 of 30. There are 8 tasks remaining.
If you are running low on iterations, prioritize finishing in-progress tasks cleanly over starting new ones.
```

**`{{TASK_OVERRIDE}}`** — Injected only when `golem run --task "specific task"` is used:
```
IMPORTANT: You MUST work on the following task this iteration: "WebSocket reconnection with exponential backoff"
Do not pick a different task.
```
When not set, this variable is replaced with an empty string, and the agent picks its own task.

### `.ctx/review-prompt.md`

This is the prompt fed to Claude Code for the review pass:

```markdown
You are reviewing this project. You are a QA/code reviewer, NOT a builder.
DO NOT modify any code, tests, or project files.
Your only job is to read, analyze, and write findings.

## What to Read
1. All design and implementation docs in `{{DOCS_PATH}}` — the original design intent
2. `.ctx/state.yaml` — what the builder claims is done
3. `.ctx/log.yaml` — what happened during building
4. The actual codebase

## What to Check
1. **Plan alignment:** Does the implementation match the design doc? Are requirements missed?
2. **Implementation completeness:** For each task marked `done` in state.yaml, check the corresponding `## Task` section in the implementation doc — were all steps completed?
3. **Task accuracy:** Are tasks marked `done` actually complete? Is state.yaml honest?
4. **Test quality:** Are tests meaningful or just coverage padding? Missing edge cases?
5. **Code quality:** Bugs, inconsistencies, error handling gaps, security issues?
6. **Decision consistency:** Are architectural decisions from state.yaml respected throughout?
7. **Pitfall awareness:** Did the builder fall into any known pitfalls?

## What NOT to Flag
- **Style preferences.** Do not flag formatting, naming conventions, or code organization unless they cause functional problems.
- **Intentional decisions.** If something looks unusual but is explained in `decisions`, it's not an issue. Respect the rationale.
- **Speculative refactors.** Do not suggest rewrites or alternative architectures unless the current implementation has a concrete bug or fails a requirement from the plan.
- **Missing features not in the docs.** Only flag what was specified in the design and implementation docs. Don't invent new requirements.
- **Minor TODOs.** Small improvements that don't affect correctness are not review issues.

## Output
For each issue found, add a task to `.ctx/state.yaml` under `tasks:`:
- Prefix the name with `[review]`
- Set status to `todo`
- Include a clear description in `notes`

Append a session entry to `.ctx/log.yaml`:
- task: "code review"
- outcome: done
- summary: describe what you found (or "no issues found")

If you found issues that need builder attention:
  output <promise>NEEDS_WORK</promise>

If everything looks good:
  output <promise>APPROVED</promise>
```

---

## CLAUDE.md Integration

`golem init` injects a section into the project's `CLAUDE.md` (creates the file if it doesn't exist). This ensures the conventions are active even during interactive sessions (not just loop iterations).

```markdown
<!-- golem:start -->
## Context Engineering (auto-managed by golem)

This project uses `.ctx/` for persistent state across sessions.

- **Design & implementation docs** — See `project.docs_path` in `.ctx/state.yaml` for location. Read ALL docs for project intent and architecture.
- **`.ctx/state.yaml`** — Current state: tasks, decisions, locked paths, pitfalls. Read at start, update at end of every session.
- **`.ctx/log.yaml`** — Session history. Append an entry at the end of every session.

### Task Mapping Convention
Each `## Task` section in the implementation doc must have a corresponding entry in `state.yaml` tasks. Use the task title from the implementation doc as the task name in state.yaml. The implementation doc contains the detailed steps and code — state.yaml tracks progress.

When planning: create tasks in state.yaml that match 1:1 with the implementation doc sections.
When building: find the matching implementation doc section for your current task and follow its steps.

### Rules
- Respect all `decisions` in state.yaml — do not contradict without exceptional reason.
- Do not modify files under `locked` paths.
- Check `pitfalls` before implementation choices.
- Update state.yaml and log.yaml at the end of every session.
<!-- golem:end -->
```

The markers `<!-- golem:start -->` and `<!-- golem:end -->` let `golem` update this section without touching the rest of CLAUDE.md.

---

## CLI Commands

### `golem init`

Scaffolds the `.ctx/` directory and injects the CLAUDE.md section.

**Behavior:**
1. Create `.ctx/` directory
2. Create `.ctx/state.yaml` with empty schema (including `docs_path`)
3. Create `.ctx/log.yaml` with empty `sessions: []`
4. Create `.ctx/prompt.md` with default template
5. Create `.ctx/review-prompt.md` with default review template
6. Create or update `CLAUDE.md` — inject the golem section between markers

**Flags:**
- `--name "Project Name"` — pre-fill project name in state.yaml
- `--stack "Kotlin, Go"` — pre-fill stack in state.yaml
- `--docs "docs/plans"` — set the docs path in state.yaml (default: `docs/`)

**Output:**
```
Initialized .ctx/ in /path/to/project
  created .ctx/state.yaml (docs_path: docs/plans)
  created .ctx/log.yaml
  created .ctx/prompt.md
  created .ctx/review-prompt.md
  updated CLAUDE.md

Run `golem plan` to start an interactive planning session.
```

### `golem plan`

Opens an interactive Claude Code session with `.ctx/` awareness.

**Behavior:**
1. Check that `.ctx/` exists (error if not — run `golem init` first)
2. Launch `claude` in interactive mode (no `-p` flag, no looping)
3. The session inherits CLAUDE.md conventions, so the agent knows about `.ctx/`
4. The human drives the session — uses their own brainstorming workflow
5. When the session ends, design/implementation docs should exist in the docs path and tasks should be populated in `.ctx/state.yaml`

**Notes:**
- `golem plan` does NOT pass a structured prompt. It trusts that CLAUDE.md gives the agent enough context about the `.ctx/` conventions.
- The human decides where to put docs and how to name them. `golem` doesn't manage doc files — it just references them via `project.docs_path`.
- Can be run multiple times — for adding features, re-planning, etc.
- New tasks from subsequent planning sessions are added alongside existing tasks in state.yaml.

### `golem run`

The core loop. Spawns autonomous Claude Code iterations until all tasks are done.

**Behavior:**
1. Check that `.ctx/` exists and `state.yaml` has tasks
2. Check that `project.docs_path` has at least one file (warn if not)
3. Render prompt template — replace `{{DOCS_PATH}}`, `{{ITERATION_CONTEXT}}`, `{{TASK_OVERRIDE}}`
4. Enter loop:
   a. Read `state.yaml` — check if any tasks are not `done`
   b. If all tasks are `done` → exit with success message
   c. Spawn `claude -p "<rendered prompt>"` with appropriate flags
   d. Wait for exit
   e. Check for `<promise>COMPLETE</promise>` in output → exit if found
   f. Check if `state.yaml` was modified (sanity check)
   g. Increment iteration counter
   h. If max iterations reached → exit with warning
   i. Loop back to (a)

**Flags:**
- `--max-iterations N` — cap on iterations (default: 20)
- `--max-turns N` — max turns per Claude Code session (passed to claude, default: 50)
- `--max-cost N` — halt loop if cumulative estimated cost exceeds N dollars (e.g., `--max-cost 10`)
- `--task "task name"` — force the agent to work on a specific task this run (injects override into prompt)
- `--review` — automatically run `golem review` after the builder loop completes
- `--dry-run` — show what would happen without executing
- `--verbose` — print iteration details as they happen

**Output during run:**
```
golem: starting builder loop (max 20 iterations, max cost $10.00)
golem: 14 tasks remaining

golem: iteration 1 starting...
golem: iteration 1 complete (duration: 4m12s, cost: ~$0.45)
golem:   task: "Go project scaffolding"
golem:   outcome: done
golem:   files changed: 12
golem: iteration 2 starting...
...
golem: iteration 5 complete — all tasks done
golem: total: 5 iterations, 14m30s, ~$2.80
```

**Error handling:**
- If Claude Code crashes → log the error, continue to next iteration
- If `state.yaml` fails schema validation → halt the loop (state is corrupted)
- If `state.yaml` isn't updated after an iteration → warn but continue
- If locked path violation detected → warn but continue
- If task regression detected → warn but continue
- If same task is `in-progress` for 3+ consecutive iterations → warn (potential thrashing)
- If `--max-cost` exceeded → halt the loop

### `golem review`

Single-pass code review. Spawns a fresh Claude Code session in reviewer role. Does not modify code — only reads and writes findings.

**Behavior:**
1. Check that `.ctx/` exists
2. Count existing `[review]` tasks in state.yaml (from previous reviews, if any)
3. Spawn `claude -p "$(cat .ctx/review-prompt.md)"` with appropriate flags
4. Wait for exit
5. Check output for `<promise>APPROVED</promise>` or `<promise>NEEDS_WORK</promise>`
6. Count new `[review]` tasks added — compare against previous review count
7. Report result with comparison

**Output (first review):**
```
golem: starting review...
golem: review complete (duration: 2m15s)
golem: result: NEEDS_WORK
golem: 3 review tasks added to state.yaml
golem:   [review] payment validation missing edge case for expired cards
golem:   [review] inconsistent error handling in auth vs payment modules
golem:   [review] no test for concurrent user creation
```

**Output (subsequent review after fixes):**
```
golem: starting review...
golem: review complete (duration: 1m50s)
golem: result: NEEDS_WORK
golem: 1 review task added to state.yaml (previous review found 3)
golem:   [review] error handling still inconsistent in payment module
```

**Output (clean review):**
```
golem: starting review...
golem: review complete (duration: 1m45s)
golem: result: APPROVED
golem: no issues found (previous review found 1)
```

The comparison gives you a convergence signal — issues should be going down across review cycles. If they're going up, something is wrong and you should intervene manually.

**Notes:**
- The reviewer does NOT modify code, tests, or project files.
- The reviewer CAN write to `state.yaml` (adding `[review]` tasks) and `log.yaml` (appending a review session entry).
- Review tasks are regular tasks — the builder loop picks them up on the next `golem run` like any other task.
- Running `golem review` multiple times against unchanged code will produce similar findings. Run it again after the builder has addressed issues.
- The comparison counts are based on `[review]` tasks in state.yaml — resolved review tasks (status: done) are not counted.

### `golem status`

Pretty-prints the current state.

**Example output:**
```
Project: MercadoEdge
Phase: building
Focus: competitor price tracking

Tasks:
  ✓ auth module
  ✓ user data models
  ◐ competitor price tracking — "scraping works, need pagination"
  ○ price history charts (depends on: competitor price tracking)
  ○ listing optimization suggestions
  ✗ shipping integration — blocked: "external API schema pending"

Decisions: 4 recorded
Pitfalls: 3 noted
Locked paths: 2
Sessions: 7 logged
```

### `golem log`

Shows iteration history.

**Flags:**
- `--last N` — show only the last N entries (default: all)
- `--failures` — show only sessions with outcome `blocked` or `unproductive`

**Example output:**
```
#7  2026-02-28 14:30  partial       "payment validation — ACH handling"
#6  2026-02-28 14:10  unproductive  "thrashed on shipping, blocked"
#5  2026-02-28 13:45  done          "payment validation — credit cards"
#4  2026-02-28 13:20  done          "database migrations"
#3  2026-02-28 12:50  done          "auth module"
```

### `golem decisions`

Lists all architectural decisions with rationale.

**Example output:**
```
2026-02-25  Use IndexedDB for price history storage
            → keep it working offline, avoid server costs

2026-02-26  MutationObserver for ML page changes, not polling
            → polling caused rate limiting from ML
```

### `golem pitfalls`

Lists all discovered pitfalls.

**Example output:**
```
• Don't use fetch() for ML pages — CORS blocks it. Use chrome.scripting
• ML pagination is infinite scroll, not page links — need scroll observer
• Don't batch more than 5 requests to ML — triggers captcha
```

### `golem lock <path>`

Manually locks a path. Adds the path to `locked` in `state.yaml`.

### `golem add-task "description"`

Adds a task to `state.yaml` with `status: todo`.

**Flags:**
- `--depends-on "other task"` — set a dependency

### `golem block <task-name> "reason"`

Marks a task as blocked with a reason.

### `golem reset`

Clears state for a fresh start.

**Behavior:**
1. Archive current `.ctx/` to `.ctx/archive/YYYY-MM-DD/`
2. Re-scaffold with empty state and log
3. Preserve `prompt.md` and `review-prompt.md` (templates, not state)
4. Preserve `.ctx/plans/` (design intent is still relevant)

### `golem rollback --to-iteration N`

Rolls back the project to the state after a specific iteration.

**Behavior:**
1. Read `.ctx/log.yaml` to find the git commits associated with iteration N
2. Revert all git commits made after iteration N
3. Reconstruct `.ctx/state.yaml` by replaying log entries 1 through N:
   - Re-derive task statuses from log outcomes
   - Preserve decisions and pitfalls recorded up to iteration N
   - Remove decisions/pitfalls added after iteration N
4. Truncate `.ctx/log.yaml` to only include entries up to iteration N

**Example:**
```bash
$ golem rollback --to-iteration 11
golem: rolling back to state after iteration 11
golem: reverting 3 git commits (iterations 12, 13, 14)
golem: restored state.yaml to iteration 11
golem: truncated log.yaml (removed 3 entries)
golem: rollback complete — you are now at iteration 11
```

**Notes:**
- This is a destructive operation — it reverts git commits. The tool asks for confirmation before proceeding.
- If state.yaml can't be perfectly reconstructed from the log, the tool does its best and warns about any gaps.
- Useful when the builder introduces a regression or goes down the wrong path for several iterations.

---

## The Loop: Detailed Mechanics

### Iteration Lifecycle

```
┌─────────────────────────────────────────────────┐
│                golem run                           │
│                                                  │
│  ┌───────────────────────────────────────────┐   │
│  │ Pre-iteration checks                      │   │
│  │  - Are there remaining tasks?              │   │
│  │  - Max iterations reached?                 │   │
│  └────────────────────┬──────────────────────┘   │
│                       │                          │
│                       ▼                          │
│  ┌───────────────────────────────────────────┐   │
│  │ Spawn Claude Code                         │   │
│  │  claude -p "$(cat .ctx/prompt.md)"        │   │
│  │       --max-turns 50                      │   │
│  │                                           │   │
│  │  Agent:                                   │   │
│  │   1. Reads plan.md + state.yaml           │   │
│  │   2. Picks a task                         │   │
│  │   3. Implements it                        │   │
│  │   4. Updates state.yaml                   │   │
│  │   5. Appends to log.yaml                  │   │
│  │   6. Commits code                         │   │
│  │   7. Outputs COMPLETE if all done         │   │
│  └────────────────────┬──────────────────────┘   │
│                       │                          │
│                       ▼                          │
│  ┌───────────────────────────────────────────┐   │
│  │ Post-iteration checks                     │   │
│  │  - Validate state.yaml (schema, parse)    │   │
│  │  - Locked path violation detection         │   │
│  │  - Task regression detection               │   │
│  │  - Was state.yaml modified?               │   │
│  │  - COMPLETE promise in output?             │   │
│  │  - Thrashing detection (same task 3x)?     │   │
│  │  - Accumulate cost estimate                │   │
│  │  - Check --max-cost limit                  │   │
│  └────────────────────┬──────────────────────┘   │
│                       │                          │
│                       ▼                          │
│               loop or exit                       │
└─────────────────────────────────────────────────┘
```

### Claude Code Invocation

```bash
claude -p "<rendered prompt>" \
  --max-turns 50 \
  --output-format stream-json
```

Key flags:
- `-p` — print mode, non-interactive, runs to completion
- `--max-turns` — limits tool calls per session (prevents runaway)
- `--output-format stream-json` — structured output for parsing completion signals and token usage

The loop captures stdout and scans for `<promise>COMPLETE</promise>`.

### Post-Iteration Validation

The loop engine (not the agent) performs these checks between iterations. This is a safety net — the agent is trusted to maintain state correctly, but the runner catches problems early before they compound.

**1. Schema Validation**
Verify `state.yaml` parses as valid YAML and contains required fields (`project`, `status`, `tasks`). If it's corrupted, halt the loop — the agent broke its own state file and continuing will make it worse.

**2. Locked Path Violation Detection**
Compare the git diff from this iteration against the `locked` paths in state.yaml. If the agent modified files under a locked path, warn:
```
golem: WARNING — iteration 7 modified src/auth/handler.go which is under locked path src/auth/
golem: this may indicate the agent ignored the locked path constraint
```
This doesn't halt the loop — the change might be legitimate (e.g., fixing an import). But it flags it for human review.

**3. Task Regression Detection**
Compare task statuses before and after the iteration. If a task that was `done` is now `in-progress` or `todo` without a corresponding `[review]` task or log entry explaining the regression, warn:
```
golem: WARNING — task "auth module" regressed from done to in-progress
golem: this may indicate the agent undid previous work
```

**4. Cost Tracking**
Parse token usage from Claude Code's stream-json output. Accumulate per-iteration costs using approximate model pricing. Display running total in verbose mode. If `--max-cost` is set and the cumulative cost exceeds the limit, halt the loop:
```
golem: STOPPED — cumulative cost $12.50 exceeded --max-cost $10.00
golem: 8 iterations completed, 6 tasks remaining
```

### Thrashing Detection

If the same task has been `in-progress` for 3+ consecutive iterations (determined by reading log.yaml), print a warning:

```
golem: WARNING — task "payment validation" has been in-progress for 3 iterations
golem: consider manually intervening, adding a pitfall, or breaking the task down
```

This doesn't stop the loop — it alerts. The human decides what to do.

---

## Implementation Plan

### Tech Stack

- **Language:** Go
- **CLI framework:** Cobra
- **YAML handling:** `gopkg.in/yaml.v3`
- **No external dependencies beyond these**

### Build Order

#### Phase 1: Scaffolding & Inspection
1. Go module setup (`golem` binary)
2. `golem init` — create `.ctx/`, empty state/log/prompt/review-prompt, inject CLAUDE.md
3. `golem status` — read and pretty-print state.yaml
4. `golem log` — read and pretty-print log.yaml
5. `golem decisions` — list decisions
6. `golem pitfalls` — list pitfalls

#### Phase 2: The Loops
7. `golem run` — builder loop implementation
   - Render prompt template with `{{ITERATION_CONTEXT}}` and `{{TASK_OVERRIDE}}`
   - Spawn Claude Code with rendered prompt
   - Capture output, detect COMPLETE
   - Iteration counting, timing
   - Max iteration cap
8. Post-iteration validation
   - Schema validation (state.yaml parses, required fields present)
   - Locked path violation detection (diff locked paths against git diff)
   - Task regression detection (done → non-done without explanation)
   - Thrashing detection (same task in-progress 3+ iterations)
9. Cost tracking
   - Parse token usage from stream-json output
   - Accumulate per-iteration cost estimates
   - `--max-cost` halt condition
10. `golem plan` — launch interactive Claude Code session
11. `golem review` — single-pass review
    - Spawn Claude Code with review-prompt
    - Capture output, detect APPROVED/NEEDS_WORK
    - Report findings with comparison to previous review
12. `golem run --review` — chain builder loop into review
13. `golem run --task` — task override injection into prompt

#### Phase 3: State Manipulation Commands
13. `golem lock <path>` — lock a path
14. `golem add-task` — add a task
15. `golem block` — mark task blocked
16. `golem reset` — archive and re-scaffold

#### Phase 4: Advanced
17. `golem rollback --to-iteration N` — revert git commits + reconstruct state from log
18. `--verbose` and `--dry-run` flags for `golem run`
19. Iteration cost/time tracking
20. Better error messages and edge case handling

---

## Project Structure

```
golem/
├── cmd/
│   ├── root.go          # cobra root command
│   ├── init.go          # golem init
│   ├── plan.go          # golem plan
│   ├── run.go           # golem run (the builder loop)
│   ├── review.go        # golem review (single-pass review)
│   ├── status.go        # golem status
│   ├── log.go           # golem log
│   ├── decisions.go     # golem decisions
│   ├── pitfalls.go      # golem pitfalls
│   ├── lock.go          # golem lock
│   ├── addtask.go       # golem add-task
│   ├── block.go         # golem block
│   ├── reset.go         # golem reset
│   └── rollback.go      # golem rollback
├── internal/
│   ├── state/
│   │   ├── state.go     # state.yaml read/write + types
│   │   └── log.go       # log.yaml read/append + types
│   ├── loop/
│   │   ├── builder.go   # spawn claude for building, detect COMPLETE, iterate
│   │   └── reviewer.go  # spawn claude for review, detect APPROVED/NEEDS_WORK
│   ├── validate/
│   │   └── validate.go  # post-iteration validation (schema, locked paths, regressions)
│   ├── cost/
│   │   └── cost.go      # parse token usage from stream-json, estimate cost, track cumulative
│   ├── prompt/
│   │   └── render.go    # template rendering (iteration context, task override)
│   ├── scaffold/
│   │   └── scaffold.go  # create .ctx/, templates, CLAUDE.md injection
│   ├── git/
│   │   └── git.go       # diff tracking, commit revert for rollback
│   └── display/
│       └── display.go   # pretty-print formatting for status/log/etc
├── templates/
│   ├── state.yaml       # empty starter state
│   ├── log.yaml         # empty starter log
│   ├── prompt.md        # default builder prompt template
│   ├── review-prompt.md # default review prompt template
│   └── claude.md        # CLAUDE.md section to inject
├── go.mod
├── go.sum
└── main.go
```

---

## Open Questions & Future Considerations

### Resolved

- **Who maintains state?** The agent. No external tool modifies state between iterations.
- **One file or three?** Three layers: plan (static), state (dynamic), log (append-only).
- **How does planning work?** `golem plan` opens interactive Claude Code. User drives brainstorming with their own workflow/plugins.
- **Go or bash?** Go — single binary, structured YAML parsing, extensible CLI.
- **Name?** `golem`.
- **Review phase?** Yes — single-pass, read-only. Reviewer does not modify code. Findings become `[review]` tasks in state.yaml for the builder to address.
- **Review automation?** Single pass always. No review loop. Can be chained with `golem run --review` or run separately.

---

## User Workflow

### Typical Project Lifecycle

```
1. INIT
   $ golem init --name "MyProject" --stack "Kotlin, Go" --docs "docs/plans"
   → scaffolds .ctx/, injects CLAUDE.md

2. PLAN (interactive)
   $ golem plan
   → opens Claude Code interactive session
   → user brainstorms using their preferred workflow (e.g. /superpowers:brainstorming)
   → agent writes design + implementation docs to docs/plans/
   → agent populates state.yaml with tasks
   → user exits when satisfied

3. BUILD (autonomous)
   $ golem run
   → builder loop iterates until all tasks done
   → agent picks tasks, implements, tests, commits, updates state
   → user is AFK or monitoring

4. REVIEW
   $ golem review
   → single-pass audit of the codebase against the plan
   → findings written as [review] tasks in state.yaml
   → APPROVED → done
   → NEEDS_WORK → proceed to step 5

5. FIX (autonomous)
   $ golem run
   → builder picks up [review] tasks and addresses them
   → loop completes when review tasks are done

6. RE-REVIEW (optional)
   $ golem review
   → verify fixes, check for new issues

7. INSPECT (anytime)
   $ golem status        → where are we?
   $ golem log --last 5  → what happened recently?
   $ golem decisions     → what was decided?
   $ golem pitfalls      → what should we avoid?
```

### AFK Workflow

```
$ golem run --review
→ builder loop runs to completion
→ automatically followed by review pass
→ come back to: build results + audit findings
```

### Mid-Project Feature Addition

```
$ golem plan
→ interactive session to design the new feature
→ agent creates new docs (e.g. docs/plans/2026-03-15-push-notifications-design.md)
→ agent adds new tasks to existing state.yaml
→ existing decisions, pitfalls, and locked paths are preserved

$ golem run
→ builder reads ALL docs in docs_path for context
→ picks up new tasks alongside any remaining work
```

### Forcing a Specific Task

```
$ golem run --task "WebSocket reconnection with exponential backoff"
→ builder loop with task override — agent works on this specific task
→ useful when the agent keeps picking the wrong task or you want to control order
```

### Rolling Back a Bad Run

```
$ golem status
→ notice iteration 12 introduced a regression

$ golem rollback --to-iteration 11
→ reverts git commits from iterations 12+
→ restores state.yaml to post-iteration-11
→ truncates log.yaml

$ golem run
→ builder resumes from clean state
```

### Open

- **Should `.ctx/log.yaml` be committed to git?** Probably yes — it's part of the project history. But it could get large. Consider: commit state.yaml and log.yaml, gitignore prompt.md (it's a template).

- **What if state.yaml gets corrupted by the agent?** The log is the backup source of truth. `golem rollback` can reconstruct state from log entries. Manual fix + git revert is also an option.

- **Should `golem run` pass any dynamic context beyond prompt.md?** For example, injecting the current git diff or test results into the prompt. For v1, no — keep it simple. The agent can run tests itself.

- **How to handle multiple concurrent features?** Currently the state tracks one set of tasks. If you want to work on two features, you'd either: (a) put all tasks in one state, (b) use git branches with separate `.ctx/` states. TBD — not needed for v1.

- **Should the prompt template be customizable per-project?** Yes — it's already a file (`prompt.md`). The user can edit it. `golem init` provides a sensible default.
