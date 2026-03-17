# Unified UX Workflows Design

## Goal

Make golem's plan-to-build workflow seamless: user opens the UI, sets up the project, plans a feature, and agents execute — all without leaving the app or bridging steps manually.

## Problem

Today the plan→build flow is broken:
- `golem plan` produces no structured output — tasks may or may not end up in state.yaml
- The default agent (`build-feature`) ignores state.yaml tasks entirely
- The `builder` agent reads tasks but is not discoverable in the UI
- The welcome experience is a one-line "No runs yet" message
- Plan/Setup are hidden behind a `⋮` menu
- There's no continuity between planning and execution

## Design

### 1. Implementer Agent

Rename the existing `builder` agent to `implementer`. It already implements the task-driven workflow: reads state.yaml tasks, finds matching implementation doc sections via `doc_scanner`, works through them sequentially.

**Changes:**
- Rename `templates/agents/builder.yaml` → `templates/agents/implementer.yaml`
- Keep the same pipeline: `init-state` → `pick-task` → `build-context` → `implement` → validation loop
- Update `templates/agents/builder.yaml` to be an alias or remove it
- Add `implementer` to the Flutter UI agent picker as the first option
- Keep `build-feature` for "quick build" (no planning needed)

### 2. Structured Plan Output

Add a system prompt to `golem plan` so sessions produce consistent, machine-readable output.

**Changes to `cmd/plan.go`:**
- Add `--append-system-prompt` with `templates/prompts/plan-session.md`
- After session exits, read state.yaml and validate at least one task with status `todo` exists
- Print warning if no tasks found

**`templates/prompts/plan-session.md` content (key behaviors from superpowers brainstorming + writing-plans):**
- Discuss the goal with the user, explore approaches, present design
- Write an implementation doc at the project's `docs_path` (from state.yaml `project.docs_path`)
- Create tasks in state.yaml matching 1:1 with implementation doc sections
- Each task has: name, status (`todo`), and notes with the doc section reference
- Set `status.phase` to `building` and `status.current_focus` to the first task
- Do NOT start implementing — planning only

### 3. Execution Discipline Integration

Embed the key behaviors from the superpowers `golem-execution` skill into the implementer agent's prompt templates. This removes the dependency on the external superpowers plugin for autonomous execution.

**Changes to implementer's `implement` step prompt:**
The prompt (rendered by `build-context` primitive) will include execution discipline:

- **Orient:** Read state.yaml for decisions, pitfalls, locked paths. Check log.yaml last 3 entries for repeated failures.
- **Focus:** Work on exactly one task. Read only the matching doc section.
- **Execute:** Follow TDD. Commit after completing the task.
- **Update:** Mark task status in state.yaml. Append session to log.yaml with outcome, approach, files_changed, warnings_for_next.
- **Partial completion:** If task can't be finished, commit working code, update notes with what remains, set outcome to `partial`.
- **Blocked:** If blocked, set `blocked_reason`, don't guess.

This is already partially implemented in `builder_primitives.go` (`primitiveInitState`, `primitiveSyncState`, `primitiveStrategyEval`). The missing piece is the prompt text that tells the Claude session how to behave.

### 4. Welcome Screen

Replace the empty "No runs yet" message with a state-aware welcome view.

**Detection logic in `ProjectWorkspace`:**

| Condition | State | Actions |
|-----------|-------|---------|
| No config or config empty | Setup needed | [Set up project] [Skip] |
| Config exists, no `todo` tasks | Ready to plan | [Plan a feature] [Quick build] |
| Tasks exist with status `todo` | Ready to build | [Launch implementer] [Review plan] |
| Active run or recent runs | Normal workspace | RunFeed + DetailPanel |

**Data source:** `GET /api/projects/{id}/state` via existing `projectStateFamily` provider. Need to add a `GET /api/projects/{id}/config` check (or infer from state).

**New Flutter widget:** `WelcomeView` in `ui/flutter/lib/views/welcome_view.dart`
- Shows contextual message and action buttons based on detected state
- Buttons launch the appropriate process (setup/plan/implementer) via the server API
- After launching, transitions to normal workspace view with terminal tab active

### 5. Command Bar Redesign

Replace the single "Run" flow with three primary actions.

**New layout:**
```
[Plan] [Build] [Review]  |  goal input...  [Go]  [⋮]
                          |  Agent: [implementer ▾]
```

- **Plan** — launches `golem plan` as an interactive process in the terminal
- **Build** — launches selected agent with the goal from the input field
- **Review** — launches `golem review` as a process in the terminal
- Goal input and agent picker only relevant for Build action
- `⋮` menu retains advanced options (Setup, model override, launch dialog)

**Agent picker update:** `implementer`, `build-feature`, `one-shot`, `fix-bug` (in that order). Ideally fetched from an API, but hardcoded list is acceptable for now.

### 6. Post-Plan Detection

After a `golem plan` process exits, detect new tasks and prompt the user.

**Flow:**
1. Process list polling detects plan process changed from `running` to `stopped`
2. UI re-fetches state.yaml via `projectStateFamily`
3. If state.yaml now has `todo` tasks, workspace transitions to "Ready to build" welcome state
4. User clicks "Launch implementer" → `golem run implementer --goal "Execute tasks from plan"`

**No new endpoints needed.** Uses existing process polling + state fetch.

## File Impact Summary

| Layer | Files | Change |
|-------|-------|--------|
| Agent | `templates/agents/implementer.yaml` | New (rename from builder.yaml) |
| Agent | `templates/agents/builder.yaml` | Remove or alias |
| Prompt | `templates/prompts/plan-session.md` | New — planning discipline |
| Prompt | `templates/prompts/implement.md` | Modify — add execution discipline |
| CLI | `cmd/plan.go` | Add system prompt, post-session validation |
| Server | `internal/server/server.go` | Possible new endpoint for config check |
| Flutter | `ui/flutter/lib/views/welcome_view.dart` | New widget |
| Flutter | `ui/flutter/lib/views/project_workspace.dart` | Integrate welcome view |
| Flutter | `ui/flutter/lib/views/command_bar.dart` | Redesign with Plan/Build/Review |
| Flutter | `ui/flutter/lib/views/agent_picker.dart` | Add implementer, reorder |
| Flutter | `ui/flutter/lib/providers/project.dart` | May need config status provider |

## Out of Scope

- Parallel task execution (future optimization)
- Replacing the superpowers plugin entirely (it still works for interactive sessions)
- Dynamic agent discovery via API (hardcoded list is fine for now)
- `golem review` connected to specific runs (standalone is acceptable)
- Distribution as a pre-built binary (building from source is fine)
