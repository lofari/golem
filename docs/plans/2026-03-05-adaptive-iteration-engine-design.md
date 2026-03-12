# Adaptive Iteration Engine — Design

## Problem

The builder loop treats every iteration the same. When Claude gets stuck on a task, the loop keeps feeding it back with no additional context. This burns iterations and can exhaust the max-iterations budget without progress.

## Solution

A reactive `Strategy` layer that runs between iterations. It observes outcomes and intervenes only when things go wrong — injecting failure context into retries, auto-skipping stuck tasks, and halting early when nothing is actionable.

### Core Principle

Claude still picks tasks and does the work. The strategy is a guardian, not a planner.

### Rules (evaluated in order)

1. **Consecutive failure**: track per-task failure count. First failure = retry with context injection. Second failure = skip (mark blocked).
2. **Dependency deadlock**: if all remaining todo tasks depend on blocked/non-done tasks, halt early.
3. **Thrashing**: 3 consecutive sessions on same task = force-skip. Replaces existing `detectThrashing` warning.
4. **No-progress**: if an iteration produces no file or state changes, count as unproductive. 2 consecutive = inject warning. 3 consecutive = halt.

### Context Injection

On retry, a `## Previous Iteration Context` section is prepended to the prompt with the failed task name, outcome, and summary from `log.yaml`.

### What Changes

| File | Change |
|------|--------|
| `internal/runner/strategy.go` | New. Strategy struct, Evaluate(), all rules |
| `internal/runner/strategy_test.go` | New. Unit tests for each rule |
| `internal/runner/builder.go` | Call strategy.Evaluate() between iterations, apply Decision |
| `internal/runner/validate.go` | Remove detectThrashing (moved to strategy) |
| `internal/runner/prompt.go` | Add InjectedContext to PromptVars |
| `templates/prompt.md` | Add `{{INJECTED_CONTEXT}}` placeholder |

### What Does NOT Change

- `CommandRunner` interface
- Task selection (Claude picks)
- Config keys (retry count hardcoded at 2)
- CLI commands
