# Blueprint Builder: Unifying the Legacy Builder Loop as a Blueprint Agent

## Problem

Golem has two orchestration engines with separate context pipelines:

1. **Blueprint engine** — typed pipeline state, explicit `reads`/`writes` contracts, `RenderStepPrompt` with `${key}` substitution, contract validation at parse time.
2. **Legacy builder loop** — `RenderPrompt` with `{{VAR}}` substitution, free-form state.yaml that Claude reads itself, handoff notes, strategy layer.

Context improvements must be implemented twice. The legacy builder relies on the agent to read state.yaml and docs correctly, with no guarantee it does. Over 15+ iteration runs, the prompt tells the agent to "read all design and implementation docs" every iteration — wasting tool calls on docs for completed phases. Handoff notes are agent-written and variable quality. Decisions and pitfalls from state.yaml are not injected into the prompt; the agent must discover them.

## Design

Express the legacy builder loop as a blueprint YAML agent (`builder.yaml`) with explicit context assembly steps. The iteration loop becomes a `while` loop with builtin steps that control exactly what context reaches the agent.

### Blueprint Structure

```yaml
name: builder
description: "Multi-task iteration loop with context assembly and strategy evaluation."
initial-state: [goal]

config:
  max-iterations: 20
  lint-cmd: null
  lint-fix-cmd: null
  test-cmd: null

steps:
  - git-setup:
      type: builtin

  - init-state:
      type: builtin
      writes: [project-context, tasks, log-context]

  - while:
      predicate: tasks-remaining
      max: 30  # hard cap; effective limit comes from config.max-iterations via strategy-eval
      steps:
        - pick-task:
            type: builtin
            reads: [tasks]
            writes: [current-task]

        - build-context:
            type: builtin
            reads: [current-task, project-context]
            optional-reads: [log-context]
            writes: [task-context]

        - implement:
            type: agentic
            reads: [goal, task-context]
            optional-reads: [_error_context]
            writes: [code]
            prompt: |
              ... # inline prompt (see "Implement Step Prompt" section below)

        - run-tests:
            type: builtin
            reads: [code]
            writes: [test-results]

        - sync-state:
            type: builtin
            writes: [project-context, tasks, log-context]

        - strategy-eval:
            type: builtin
            reads: [tasks, log-context]
            optional-reads: [current-task]
            writes: [_error_context]

errors:
  transient: { action: retry, max: 3 }
  malformed-output: { action: re-run, max: 2, hint: "Your task updates should be made via the golem MCP tools." }
  contract-violation: { action: halt }
```

### Pipeline State Keys

| Key | Type | Produced by | Description |
|---|---|---|---|
| `goal` | `string` | initial-state | User-provided goal from `--goal` flag |
| `project-context` | `object` | `init-state` / `sync-state` | Decisions, pitfalls, project metadata from state.yaml |
| `tasks` | `array` | `init-state` / `sync-state` | Current task list with statuses |
| `log-context` | `object` | `init-state` / `sync-state` | Last session's handoff note, outcome, task name |
| `current-task` | `object` | `pick-task` | Selected task: name, notes, status, doc reference |
| `task-context` | `string` | `build-context` | Assembled markdown context block for the agent |
| `code` | `object` | engine (auto) | Git diff info, set by `detectCodeChanges` |
| `test-results` | `object` | `run-tests` | Test pass/fail status and summary |
| `_error_context` | `string` | `strategy-eval` | Injected on retry/failure, cleared on success |

### Builtin Steps

#### `init-state` and `sync-state`

Two builtins that share the same output shape but have different responsibilities. Splitting them avoids the duplicate step name problem (the blueprint parser rejects duplicate names).

**`init-state`** — Runs once before the loop. Reads `.ctx/state.yaml` and `.ctx/log.yaml` into pipeline state. Saves a state snapshot for rollback. Records the current HEAD commit hash into pipeline state as `_head_before` (used by `last_diff_stat`).

**`sync-state`** — Runs inside the loop after each iteration. Re-reads `.ctx/state.yaml` and `.ctx/log.yaml` into pipeline state (refreshing from what the agent wrote via MCP tools). Validates state for corruption and auto-repairs invalid phases/statuses, matching the existing `ValidatePostIteration` logic. Detects if the agent failed to call MCP tools — if `log-context.iteration` did not increment since the last call, flags this for `strategy-eval` to write a synthetic error log session. Re-records `_head_before` with the current HEAD hash so the next iteration's `last_diff_stat` covers only the most recent changes.

Both write the same output keys:

```
Reads: .ctx/state.yaml, .ctx/log.yaml
Writes:
  project-context:
    decisions: [{what, why, when}, ...]
    pitfalls: [{what, fix}, ...]
    phase: string
    current_focus: string
    docs_path: string
  tasks: [{name, status, notes, depends_on, blocked_reason}, ...]
  log-context:
    last_task: string
    last_outcome: string
    last_handoff: string
    last_diff_stat: string (from git diff --stat <_head_before>..HEAD)
    iteration: int (count of sessions in log.yaml)
    agent_logged: bool (whether iteration count incremented since last sync)
```

Implementation: both builtins call the same shared function (e.g., `readProjectState(dir, state)`) with a flag distinguishing init vs. sync behavior. This keeps the logic DRY while satisfying the parser's unique-name requirement.

#### `pick-task`

Deterministic task selection from the task list. Replaces the current approach of hoping the agent picks correctly.

Selection priority:
1. If `--task` override is set in config, select that task
2. Prefer tasks with status `in-progress` (agent started but didn't finish)
3. Then `todo` tasks with all dependencies satisfied (`depends_on` entries are `done`)
4. Skip `blocked` and `done` tasks
5. Among eligible tasks, prefer earlier position in the list (preserves author ordering)

Output:
```
current-task:
  name: "2x2 Sprite Rendering"
  status: todo
  notes: "Rewrite MapRenderer.render() for 2x2 sprite blocks"
  doc_hint: "docs/plans/impl.md section '## Task 4'"  # if docs_path exists and doc has matching section
```

The `doc_hint` field is populated by scanning files in `docs_path` for section headers that match the task name. The scanner tries multiple heading patterns:
- `## Task <name>` or `### Task <name>` (exact task name after "Task" prefix)
- `## <N>. <name>` or `## Task <N>: <name>` (numbered headings)
- Case-insensitive substring match on the heading text as a fallback

If multiple matches are found, prefer the one in the most recently modified file (likely the current phase's doc). If no match is found, the field is omitted — the agent works from task notes alone.

#### `build-context`

Assembles the `task-context` string — a single markdown block that the agent receives as front-loaded orientation context. This is the primary lever for controlling context quality.

Sections included (in order):

**1. Your Task** (always)
```markdown
## Your Task
Name: "{name}"
Status: {status}
Notes: {notes}
```

**2. Documentation Pointer** (if `doc_hint` exists on `current-task`)
```markdown
## Documentation
Read the implementation details at: {doc_hint}
Do NOT read other sections or other doc files — they cover completed work.
```

**3. Previous Iteration** (if `log-context.last_handoff` exists)
```markdown
## Handoff from Previous Iteration
{last_handoff}

Last task: {last_task} — outcome: {last_outcome}
```

**4. Recent Changes** (if `log-context.last_diff_stat` is non-empty)
```markdown
## Recent Changes (last iteration)
{last_diff_stat}
```

**5. Decisions & Pitfalls** (if any exist in `project-context`)
```markdown
## Project Decisions
- {what} — {why}
...

## Known Pitfalls
- {what}: {fix}
...
```

**6. Context Map** (if knowledge graph exists and has embeddings)

Queries the graph for symbols semantically relevant to the current task. Includes callers and dependencies for the top results. Limited to `context-map-limit` symbols (default 15).

```markdown
## Context Map (pre-loaded)
These symbols are relevant to your task — no need to search for them:
- MapRenderer.render() (src/rendering/MapRenderer.kt:45)
  Callers: Main.gameLoop()
  Dependencies: SpriteResolver, Viewport
...

Graph tools (find_callers, semantic_search, etc.) are available for deeper exploration.
```

**Size budget**: `build-context` should estimate the total token count of `task-context` and truncate lower-priority sections (context map first, then decisions/pitfalls, then diff stat) if the total exceeds a configurable threshold. Default threshold: 4000 tokens (~16KB of text). This prevents context rot on projects with many decisions/pitfalls or large context maps.

**Note on serialization**: `RenderStepPrompt` serializes state values via `json.Marshal`. For `current-task`, this means the agent would see raw JSON. To keep the prompt clean, `build-context` pre-formats the task info as markdown within the `task-context` string rather than relying on `${current-task}` serialization in the prompt template. The `current-task` key remains in pipeline state as a structured object (for `pick-task` and `strategy-eval` to read), but the implement prompt reads it through `task-context` where it has been pre-rendered.

#### `strategy-eval`

Post-iteration strategy evaluation. Adapts the existing `Strategy` logic from `strategy.go` into a builtin step.

Reads `tasks` and `log-context` to detect (also checks `config.max-iterations` against the current iteration count from `log-context.iteration` to enforce the user's configured limit, halting before the `while` loop's hard cap):
- **Thrashing**: same task attempted 3+ consecutive times without completion
- **Repeated failure**: task failed/blocked 2+ times
- **Unproductive streak**: 3+ consecutive unproductive outcomes
- **Deadlock**: all remaining tasks are blocked or depend on blocked tasks

Actions:
- **Continue**: clear `_error_context`, proceed to next iteration
- **Retry with context**: write failure details into `_error_context` including a truncated excerpt of what went wrong, so the agent knows *what* to change
- **Skip task**: mark task as blocked in state.yaml via direct write, inject skip notice into `_error_context`
- **Halt**: set a `_halt` flag in pipeline state; the `tasks-remaining` predicate checks this flag

`strategy-eval` **always** clears `_error_context` first, then sets it if needed. This takes ownership of the key across iterations. The engine's own `_error_context` (from `handleTransient`/`handleMalformedOutput`) operates *within* a single step's retry loop and is cleared on success before `strategy-eval` runs, so there is no conflict — the engine handles intra-step retries, strategy handles inter-iteration context.

Also writes a synthetic log session to `.ctx/log.yaml` when `sync-state` sets `agent_logged: false` (iteration count didn't increment). The synthetic session is written using `golemctx.AppendSession()` — the same function the MCP `log_session` tool calls internally — to avoid format inconsistencies or race conditions. This is safe because `strategy-eval` runs after `sync-state` (which has already finished reading log.yaml), and no concurrent writers exist at this point in the pipeline.

### Predicate: `tasks-remaining`

A new builtin predicate that checks `tasks` in pipeline state:

```go
func evalTasksRemaining(state map[string]any) bool {
    // Returns true if any task has status "todo" or "in-progress"
    // Also returns false if strategy-eval set a halt flag
}
```

### Implement Step Prompt

The implement step uses an inline `prompt:` field in the YAML rather than a shared template file. This avoids colliding with the existing `templates/prompts/implement.md` used by `build-feature` and `fix-bug` agents, and keeps the builder's prompt self-contained. The step inherits `implement`'s defaults from `stepDefaults` (max-turns: 200, timeout: 30 min) since name matching still works.

The prompt is lean. It provides the *what* (goal, task, context) and lets the agent discover the *how*.

```markdown
You are working on a software project autonomously. Each iteration you work on ONE task.
You have no memory of previous iterations — all context is provided below.

# Goal
${goal}

# Context
${task-context}

# Instructions
1. If a documentation pointer is provided above, read that section for implementation details.
2. Use graph tools (find_callers, semantic_search, etc.) if you need to trace code beyond what the context map provides.
3. Implement the task. Write or update tests. Make sure they pass.
4. Commit your work with clear commit messages.

# End of Session
Use the golem MCP tools to update state:
1. Call `mark_task` to update your task (set status and notes).
2. Call `add_decision` for any new architectural decisions.
3. Call `add_pitfall` for any lessons learned.
4. Call `set_status` to update current_focus.
5. Call `log_session` with task name, outcome, summary, files_changed, and a handoff note.
   The handoff should be specific: what you did, where you left off, what to do next, and any gotchas.
   Include file paths and line numbers when relevant.

## Previous Error Context
${_error_context}
```

Key differences from the current `prompt.md`:
- No "read all docs" instruction — `build-context` provides a targeted doc pointer
- No "read state.yaml" instruction — the task and context are already in the prompt
- No "pick a task" instruction — `pick-task` already selected it
- Explicit instruction for high-quality handoff notes (with file paths and line numbers)
- Graph tools remain available but the context map provides a head start

### Agent MCP Tools

The agent retains access to all current MCP tools:

**State tools** (required): `mark_task`, `set_phase`, `set_status`, `add_decision`, `add_pitfall`, `log_session`

**Graph tools** (available for deeper exploration): `find_callers`, `find_dependencies`, `find_dependents`, `semantic_search`, `find_co_changed`, `find_execution_failures`, `get_runtime_trace`

**LSP tools** (if enabled): `lsp_definition`, `lsp_references`, `lsp_hover`, `lsp_diagnostics`

The agent uses graph tools reactively when it needs to trace something beyond what the pre-loaded context map provides.

## Implementation Scope

### New Code

| Component | Location | Description |
|---|---|---|
| `pick-task` builtin | `internal/runner/primitives.go` | Task selection logic (~50 lines) |
| `build-context` builtin | `internal/runner/primitives.go` | Context assembly (~150 lines) |
| `init-state` builtin | `internal/runner/primitives.go` | Initial state load + snapshot (~40 lines) |
| `sync-state` builtin | `internal/runner/primitives.go` | State refresh + validation (~60 lines, shares core with `init-state`) |
| `strategy-eval` builtin | `internal/runner/primitives.go` | Strategy evaluation as builtin (~100 lines, mostly moved from `strategy.go`) |
| `tasks-remaining` predicate | `internal/runner/predicates.go` | New predicate (~15 lines) |
| `builder.yaml` agent | `templates/agents/builder.yaml` | Blueprint definition (~50 lines) |
| Doc section scanner | `internal/runner/primitives.go` | Scan docs for matching task section headers (~40 lines) |

### Modified Code

| Component | Change |
|---|---|
| `engine.go` `execBuiltinStep` | Add cases for new builtins; fix result storage model (see below) |
| `predicates.go` `evalBuiltinPredicate` | Add `tasks-remaining` case |
| `engine_context.go` | Extend `syncGraph` and `buildContextMapString` reuse for `build-context` |
| `cmd/code.go` | Wire `builder` as default agent when `engine: blueprint` |

#### Engine change: `execBuiltinStep` result storage

The current result storage in `execBuiltinStep` stores the *entire* `PrimitiveResult` map as the value for every non-reserved write key. This means `sync-state` writing `[project-context, tasks, log-context]` would store the same blob under all three keys.

**Fix**: Change the storage logic to check if the result contains a key matching each write key name. If it does, store that specific value. If it doesn't, fall back to storing the full result (backward-compatible with existing single-key builtins).

```go
// Before (broken for multi-key writes):
e.state[key] = map[string]any(result)

// After (per-key extraction):
if val, ok := result[key]; ok {
    e.state[key] = val
} else {
    e.state[key] = map[string]any(result)
}
```

All new builtins return results keyed by their write key names. Existing builtins (`run-tests`, `lint`, etc.) continue to work because their single write key gets the full result as before (the key won't match, so it falls through to the full-result path). `git-setup` is unaffected because it uses the no-declared-writes flat storage path.

### Not Changed

- Legacy builder loop (`builder.go`, `prompt.go`, `validate.go`, `snapshot.go`) — remains available via `engine: go` config for backward compatibility
- Existing blueprint agents (`build-feature.yaml`, `fix-bug.yaml`, `one-shot.yaml`) — unaffected
- MCP tools — no changes
- State/log format — no schema changes
- Flutter UI — already handles engine events

### Known Limitations (v1)

- **No parallel execution.** The legacy builder supports `parallel > 1` for running multiple tasks concurrently in git worktrees. The blueprint builder's `while` loop is strictly sequential. Parallel execution would require a new control flow type (e.g., `parallel:`) in the blueprint engine. This can be added later without changing the builder agent's structure.
- **`run-tests` reads `[code]`** as a contract-ordering constraint, not a data dependency. The `primitiveRunTests` implementation does not use the `code` value from pipeline state — the declaration just ensures tests run after implementation.
- **Contract validation does not recurse into nested `SubNodes`.** `validateControlFlowContracts` only iterates `InlineSteps`, not `SubNodes`/`ThenNodes`/`ElseNodes`. This is not blocking for the builder (all steps are inline), but is a latent engine bug that should be fixed separately if blueprints with deeply nested control flow are planned.

## Migration Path

1. Ship `builder.yaml` as a new built-in agent alongside existing agents
2. Users can run it explicitly: `golem run builder --goal "..."`
3. Once validated, make `engine: blueprint` + `agent: builder` the default in `cmd/code.go`
4. Legacy builder remains selectable via `engine: go` indefinitely
5. Eventually deprecate and remove the legacy builder once the blueprint builder is proven

## Testing Strategy

- Unit tests for each new builtin (task selection priority, context assembly, strategy decisions)
- Unit test for doc section scanner with various heading formats
- Integration test: run the `builder.yaml` pipeline with a mock `CommandRunner` and verify state transitions
- Manual test: run on TROGUE project (Phase 3 or a new feature) and compare iteration efficiency vs. legacy builder

## Context Budget Example

For a TROGUE iteration at task 45 (Phase 2, Task 4):

| Section | Estimated size |
|---|---|
| Task block | ~50 tokens |
| Doc pointer | ~30 tokens |
| Handoff from previous | ~150 tokens |
| Recent changes (diff stat) | ~100 tokens |
| Decisions (7 entries) | ~200 tokens |
| Pitfalls (5 entries) | ~150 tokens |
| Context map (15 symbols) | ~600 tokens |
| **Total task-context** | **~1,280 tokens** |

Compare to current approach where the agent reads ~2,500 lines of docs (~6,000 tokens) plus state.yaml (~800 tokens) across 6+ tool calls. The blueprint builder provides more relevant context in ~1/5 the token budget with zero tool calls for orientation.
