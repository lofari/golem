# Handoff Notes Design

**Goal:** Let each iteration pass a freeform note to the next, so the next agent can orient itself without broad investigation.

## Problem

Every iteration starts by reading design docs, state, and log to figure out where the project stands. This broad investigation wastes time when the previous iteration already knows exactly what to say: what it worked on, where it left off, and what to do next.

## Solution

Add a `handoff` field to the session log. The agent writes it at end of session via `log_session`. The builder loop reads the last session's handoff and injects it into the next iteration's prompt through the existing `{{INJECTED_CONTEXT}}` mechanism.

## Data Flow

```
Iteration N ends
  → agent calls log_session(handoff: "worked on X, left off at Y, next step is Z")
  → handoff stored in log.yaml as part of the session entry

Iteration N+1 starts
  → builder reads last session from log.yaml
  → extracts handoff, wraps it in a "## Handoff from Previous Iteration" section
  → prepends to injectedContext (coexists with strategy-injected context)
  → rendered into prompt via {{INJECTED_CONTEXT}}
```

## Changes

### 1. `internal/ctx/log.go` — Add field to Session

Add `Handoff string` to the `Session` struct with `yaml:"handoff,omitempty"`.

### 2. `internal/mcp/tools.go` — Accept handoff in log_session

Add `handoff` as an optional string parameter to the `log_session` tool schema. Wire it into the `Session` struct when appending to the log.

### 3. `internal/runner/builder.go` — Inject handoff into next prompt

Before the loop starts and at the end of each iteration, read the last session's `Handoff` from the log. If non-empty, format it as a markdown section and prepend it to `injectedContext`. Strategy context and handoff context coexist — both are concatenated.

### 4. `templates/prompt.md` — Instruct agent to write handoff

Add to the "End of Session" section:

> Write a handoff note for the next iteration. Include: what you worked on, where you left off, what to do next, and any gotchas the next iteration should know. Pass it as the `handoff` parameter when calling `log_session`.

### 5. Tests

- `internal/ctx/log_test.go` — verify handoff round-trips through YAML
- `internal/mcp/tools_test.go` — verify log_session accepts and stores handoff
- `internal/runner/builder_test.go` — verify handoff injection into prompt
