# Resilience & Parallelism Design

Date: 2026-03-04

Four features to improve golem's reliability, observability, and throughput.

## Feature 1: Surface Halt Reasons in TUI

**Problem:** When the builder loop halts, the TUI just stops. The user cannot see why.

**Solution:**
- `EventLoopDone` already carries `BuilderResult.HaltReason` — display it.
- When `result.Halted == true`, store the halt reason in the TUI model.
- Render a red status line at the bottom of the sidebar: `HALTED: <reason>`.
- Append a final line to the output panel so it's visible in scrollback.
- For iteration-level errors (`EventIterEnd` with `Err != nil`), show a yellow warning in the output panel: `iteration N failed: <error>`.

**Files:**
- `internal/tui/run.go` — store halt reason, render on `EventLoopDone`
- `internal/tui/components.go` — red halt banner in sidebar

**Scope:** ~20 lines changed.

---

## Feature 2: State Snapshots with Auto-Rollback

**Problem:** If the agent corrupts `state.yaml` beyond auto-repair, the loop halts with no recovery path.

**Solution:**
- Before each iteration, copy `state.yaml` to `.ctx/snapshots/state-<iteration>.yaml`.
- Keep the last 10 snapshots; prune older ones.
- In `ValidatePostIteration`, after auto-repair fails:
  1. Find the most recent snapshot.
  2. Restore it as `state.yaml`.
  3. Emit a warning: `"state corrupted — restored from snapshot (iteration N)"`.
  4. Continue the loop. The next iteration re-reads state, picking up the restored version.
- Only halt if no snapshot exists to restore from (first iteration corruption).

**Files:**
- `internal/runner/snapshot.go` — new: save, restore, prune logic (~50 lines)
- `internal/runner/builder.go` — call snapshot before each iteration
- `internal/runner/validate.go` — try restore before halting

---

## Feature 3: Golem MCP Server for Structured State Updates

**Problem:** Claude edits `state.yaml` as raw YAML, causing normalization failures, invalid statuses/phases, malformed YAML, and missing fields. Normalization is a workaround, not a fix.

**Solution:** A stdio-based MCP server that exposes constrained tools for state updates. Claude reads state.yaml directly but writes through validated tools.

### Architecture

- New package: `internal/mcp/` — MCP server written in Go.
- New subcommand: `golem mcp-serve --dir <project-dir>` — runs the stdio MCP server.
- Before each claude session, golem writes a temporary `mcp_servers.json` config pointing to `golem mcp-serve`.
- Claude gets `--mcp-config <path>` flag, connecting it to the server.
- Server dies when claude's session ends (stdin closes).

### Tools Exposed

| Tool | Parameters | Effect |
|------|-----------|--------|
| `mark_task` | `name`, `status`, `notes?` | Set task status (validated against canonical values) |
| `set_phase` | `phase` | Set `status.phase` (validated) |
| `add_decision` | `what`, `why` | Append to decisions list, auto-set `when` to today |
| `add_pitfall` | `what`, `fix?` | Append to pitfalls list |
| `add_locked` | `path`, `note?` | Append to locked list |
| `log_session` | `task`, `outcome`, `summary`, `files_changed` | Append session entry to `log.yaml` |

### Prompt Changes

- "End of Session" instructions change from "edit state.yaml" to "use the golem tools to update state."
- Remove YAML formatting instructions, valid status lists, etc. — tools enforce correctness.
- Keep read instructions ("Read `.ctx/state.yaml`") since reading is safe.

### What This Eliminates

- Status/phase normalization hacks (tools reject invalid values at the source).
- State corruption from malformed YAML edits.
- The blocked-without-reason problem (tool requires `blocked_reason` when `status == "blocked"`).
- Most auto-repair logic in validation (kept as safety net for direct edits).

### Server Lifecycle

- Same binary: `golem mcp-serve` subcommand.
- Spawned per-session (fresh state each time, no orphan processes).
- Dies when claude's stdin closes.

**Files:**
- `internal/mcp/server.go` — MCP server implementation
- `internal/mcp/tools.go` — tool handlers (mark_task, set_phase, etc.)
- `cmd/mcp_serve.go` — `golem mcp-serve` subcommand
- `internal/runner/command.go` — write mcp config, pass `--mcp-config` to claude
- `templates/prompt.md` — update end-of-session instructions

---

## Feature 4: Parallel Independent Tasks via Git Worktrees

**Problem:** Each iteration does one task sequentially. For independent tasks (especially review fixes), this wastes time.

**Solution:** Run N claude sessions concurrently, each in its own git worktree, forced to a specific task.

### Flow

1. At iteration start, builder reads state and identifies **eligible tasks** — `todo` status, no unresolved `depends_on`.
2. If `--parallel > 1` and 2+ eligible tasks exist:
   - Create N git worktrees under `.ctx/worktrees/<sanitized-task-name>/`.
   - Sanitize: lowercase, replace spaces/special chars with `-`, trim length.
   - Each worktree gets its own claude session with `--task <name>`.
   - Sessions run concurrently via goroutines.
3. When all sessions complete:
   - Merge each worktree back into the main branch, one at a time.
   - Merge order: alphabetical by task name for determinism.
   - If merge conflicts: `git merge --abort`, mark task as `in-progress` with conflict note, leave for next sequential iteration.
   - Clean up merged worktrees.
4. Run post-iteration validation once on the merged result.

### Conflict Mitigation

- Git worktrees provide natural isolation — each session has its own working copy.
- First merge always succeeds. Subsequent merges may conflict.
- On conflict: abort merge, keep worktree, re-queue task. No work is lost.
- State updates go through the MCP server. Each worktree gets its own server instance, all pointing to the same `.ctx/state.yaml` in the main tree. File locking (`flock`) on `state.yaml` ensures atomic writes.

### Configuration

- `--parallel N` flag on `golem run` (default 1 = current sequential behavior).
- Max parallelism capped at eligible task count.
- Only during `golem run`, not `golem review`.

### Limitations (v1)

- No file-level prediction — relies on git merge to detect conflicts.
- No shared context between parallel sessions — each is independent.
- Parallel iterations count as 1 iteration toward `--max-iterations`.

**Files:**
- `internal/runner/parallel.go` — new: worktree create, concurrent execution, merge (~150 lines)
- `internal/runner/builder.go` — parallel dispatch alongside existing sequential loop
- `cmd/run.go` — add `--parallel` flag
- `internal/runner/prompt.go` — reuse `BuildTaskOverride` for forced task assignment

---

## Implementation Order

1. **TUI halt reasons** — smallest, immediate value, no dependencies
2. **State snapshots** — small, improves resilience before MCP changes
3. **MCP server** — largest change, but independent of parallelism
4. **Parallel tasks** — depends on MCP server (for flock on state.yaml)
