# Unified UX Workflows Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make golem's plan-to-build workflow seamless — user opens UI, sets up project, plans a feature, agents execute tasks automatically.

**Architecture:** Six tasks across Go backend and Flutter frontend. (1) Rename builder agent to implementer, (2) add structured plan prompt, (3) embed execution discipline in implementer prompt, (4) add post-plan validation to golem plan, (5) create Flutter welcome screen, (6) redesign command bar with Plan/Build/Review actions.

**Tech Stack:** Go (cobra, yaml.v3, embed), Dart/Flutter (Riverpod, Material), stdlib testing

---

## File Structure

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `templates/agents/implementer.yaml` | Renamed builder agent |
| Remove | `templates/agents/builder.yaml` | Replaced by implementer |
| Create | `templates/prompts/plan-session.md` | Planning discipline prompt for golem plan |
| Modify | `templates/embed.go:6` | Verify new files are covered by globs |
| Modify | `cmd/plan.go` | Add system prompt + post-session validation |
| Modify | `cmd/agents.go` | Handle implementer alias if needed |
| Create | `ui/flutter/lib/views/welcome_view.dart` | State-aware welcome screen |
| Modify | `ui/flutter/lib/views/project_workspace.dart` | Integrate welcome view + command actions |
| Modify | `ui/flutter/lib/views/command_bar.dart` | Plan/Build/Review buttons |
| Modify | `ui/flutter/lib/views/agent_picker.dart` | Add implementer agent |

---

## Chunk 1: Implementer Agent + Plan Prompt (Tasks 1-3)

### Task 1: Rename builder agent to implementer

**Files:**
- Create: `templates/agents/implementer.yaml`
- Remove: `templates/agents/builder.yaml`

- [ ] **Step 1: Create implementer.yaml**

Copy `templates/agents/builder.yaml` to `templates/agents/implementer.yaml` and change the name and description:

```yaml
name: implementer
description: "Executes tasks from state.yaml with context assembly and strategy evaluation."
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
      max: 30
      steps:
        - pick-task:
            type: builtin
            optional-reads: [tasks]
            writes: [current-task]

        - build-context:
            type: builtin
            optional-reads: [current-task, project-context, log-context]
            writes: [task-context]

        - implement:
            type: agentic
            reads: [goal]
            optional-reads: [task-context, _error_context]
            writes: [code]
            prompt: |
              You are working on a software project autonomously. Each iteration you work on ONE task.
              You have no memory of previous iterations — all context is provided below.

              # Goal
              ${goal}

              # Context
              ${task-context}

              # Execution Discipline

              ## Orient
              - The context above contains decisions, pitfalls, and recent log entries.
              - Respect all decisions — do not contradict without exceptional documented reason.
              - Check pitfalls before making implementation choices.

              ## Focus
              - Work on exactly ONE task (the one described above).
              - If a documentation pointer is provided, read that section for detailed implementation steps.
              - Use graph tools (find_callers, semantic_search, etc.) if you need to trace code beyond the context map.

              ## Execute
              - Follow TDD: write failing test first, then implement, then verify.
              - Commit your work with clear commit messages after completing the task.

              ## Update State
              Use the golem MCP tools to update state:
              1. Call `mark_task` to update your task (set status to `done` and add notes).
              2. Call `add_decision` for any new architectural decisions.
              3. Call `add_pitfall` for any lessons learned.
              4. Call `set_status` to update current_focus to the next task.
              5. Call `log_session` with task name, outcome (success/partial/blocked), summary,
                 files_changed, and a handoff note for the next iteration.

              ## Partial Completion
              If you cannot finish the task:
              - Commit all working, tested code.
              - Keep task status as `in-progress` with notes explaining what remains.
              - Log with outcome `partial` and specific warnings_for_next.

              ## When Blocked
              - Set task to `blocked` with a specific `blocked_reason`.
              - Do NOT guess — stop and document what's missing.

              ## Previous Error Context
              ${_error_context}

        - run-tests:
            type: builtin
            optional-reads: [code]
            writes: [test-results]

        - sync-state:
            type: builtin
            writes: [project-context, tasks, log-context]

        - strategy-eval:
            type: builtin
            optional-reads: [tasks, log-context, current-task]
            writes: [_error_context]

errors:
  transient: { action: retry, max: 3 }
  malformed-output: { action: re-run, max: 2, hint: "Your task updates should be made via the golem MCP tools." }
  contract-violation: { action: halt }
```

- [ ] **Step 2: Remove builder.yaml**

```bash
rm templates/agents/builder.yaml
```

- [ ] **Step 3: Verify embed picks up the new file**

The embed glob at `templates/embed.go:6` uses `agents/*.yaml` which will pick up `implementer.yaml` automatically.

Run: `go build ./...`
Expected: Clean build

- [ ] **Step 4: Run tests**

Run: `go test ./internal/runner/ -v -run "TestAgent|TestBlueprint" 2>&1 | tail -20`

Check if any tests reference `builder` by name. If they do, update them to use `implementer`.

Run: `grep -r '"builder"' internal/runner/ --include='*_test.go'`

- [ ] **Step 5: Commit**

```bash
git add templates/agents/implementer.yaml
git rm templates/agents/builder.yaml
git commit -m "feat(agents): rename builder to implementer with execution discipline

The implementer agent reads tasks from state.yaml and works through
them sequentially. The prompt now includes explicit execution
discipline: TDD, state updates, partial completion protocol,
and blocked handling."
```

---

### Task 2: Create plan-session prompt template

**Files:**
- Create: `templates/prompts/plan-session.md`

- [ ] **Step 1: Create the prompt**

```markdown
You are a planning assistant for a software project managed by golem.

Your job is to collaborate with the user to design a feature, then create a structured implementation plan that golem's implementer agent can execute autonomously.

## Your Workflow

1. **Understand the goal** — ask clarifying questions about what the user wants to build
2. **Explore the codebase** — read relevant files to understand current architecture
3. **Design the approach** — propose 2-3 approaches with trade-offs, get user approval
4. **Write the implementation doc** — create a detailed plan document
5. **Seed state.yaml** — create tasks that match 1:1 with the plan's sections

## Writing the Implementation Doc

Create a markdown file at the project's docs path (check `project.docs_path` in `.ctx/state.yaml`, default: `docs/`).

Name it: `YYYY-MM-DD-<feature-name>.md`

Structure each task as:

```
## Task N: Component Name

**Files:**
- Create/Modify: exact/path/to/file
- Test: exact/path/to/test_file

### Steps
1. Write failing test for [specific behavior]
2. Implement [specific thing]
3. Run tests, verify passing
4. Commit
```

Each task should be completable in one iteration by an autonomous agent.

## Seeding state.yaml

After writing the implementation doc, update `.ctx/state.yaml`:

1. Add a task entry for each `## Task N` section:
   ```yaml
   tasks:
     - name: "Task 1: Component Name"
       status: todo
       notes: "See docs/YYYY-MM-DD-feature.md section 'Task 1'"
   ```

2. Set `status.phase` to `building`
3. Set `status.current_focus` to the first task name

## Rules
- Do NOT start implementing — planning only
- Each task must be independently testable
- Tasks should be ordered by dependency (earlier tasks don't depend on later ones)
- Be specific: exact file paths, exact function names, exact test cases
- Keep tasks small enough for one autonomous iteration (30-60 minutes of work)
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./...`
Expected: Clean build (glob `prompts/*.md` covers the new file)

- [ ] **Step 3: Commit**

```bash
git add templates/prompts/plan-session.md
git commit -m "feat(plan): add structured plan session prompt

Instructs the planning session to create an implementation doc
and seed state.yaml with tasks that the implementer agent can
execute autonomously."
```

---

### Task 3: Add system prompt and post-session validation to golem plan

**Files:**
- Modify: `cmd/plan.go`

- [ ] **Step 1: Read the prompt template and add it as system prompt**

In `cmd/plan.go`, after the config loading block (line 44) and before building `claudeArgs` (line 46), add:

```go
	// Load plan session prompt
	planPrompt, err := templates.FS.ReadFile("prompts/plan-session.md")
	if err != nil {
		fmt.Fprintf(os.Stderr, "golem: warning: could not read plan prompt: %v\n", err)
	}
```

Update the imports to include `"github.com/lofari/golem/templates"`.

Update `claudeArgs` construction (line 46):

```go
	claudeArgs := []string{}
	if len(planPrompt) > 0 {
		claudeArgs = append(claudeArgs, "--append-system-prompt", string(planPrompt))
	}
```

- [ ] **Step 2: Add post-session validation**

After `claude.Run()` returns (line 68), add validation before returning:

```go
	if err := claude.Run(); err != nil {
		return err
	}

	// Validate that planning produced tasks
	state, stateErr := golemctx.ReadState(dir)
	if stateErr == nil {
		todoCount := 0
		for _, t := range state.Tasks {
			if t.Status == "todo" {
				todoCount++
			}
		}
		if todoCount == 0 {
			fmt.Fprintln(os.Stderr, "golem: warning: no tasks with status 'todo' found in state.yaml")
			fmt.Fprintln(os.Stderr, "golem: the implementer agent needs tasks to work on")
			fmt.Fprintln(os.Stderr, "golem: run `golem plan` again to create tasks, or add them manually")
		} else {
			fmt.Fprintf(os.Stderr, "golem: plan complete — %d tasks ready\n", todoCount)
			fmt.Fprintln(os.Stderr, "golem: run `golem run implementer --goal '<goal>'` to start building")
		}
	}

	return nil
```

Add `golemctx "github.com/lofari/golem/internal/ctx"` to imports.

- [ ] **Step 3: Build and test**

Run: `go build ./...`
Expected: Clean build

Run: `go test ./cmd/ -v`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/plan.go
git commit -m "feat(plan): add structured prompt and post-session validation

golem plan now appends a system prompt that guides the session to
create an implementation doc and seed state.yaml tasks. After the
session exits, validates that todo tasks exist and prints guidance."
```

---

## Chunk 2: Flutter UI Changes (Tasks 4-6)

### Task 4: Create welcome view

**Files:**
- Create: `ui/flutter/lib/views/welcome_view.dart`

- [ ] **Step 1: Create the welcome view widget**

```dart
import 'package:flutter/material.dart';
import '../theme.dart';

enum ProjectPhase { needsSetup, readyToPlan, readyToBuild, active }

class WelcomeView extends StatelessWidget {
  final ProjectPhase phase;
  final int taskCount;
  final VoidCallback onSetup;
  final VoidCallback onPlan;
  final VoidCallback onBuild;
  final VoidCallback onSkip;

  const WelcomeView({
    super.key,
    required this.phase,
    this.taskCount = 0,
    required this.onSetup,
    required this.onPlan,
    required this.onBuild,
    required this.onSkip,
  });

  @override
  Widget build(BuildContext context) {
    return Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 400),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              _icon,
              size: 48,
              color: GolemTheme.accent,
            ),
            const SizedBox(height: 16),
            Text(
              _title,
              style: const TextStyle(
                fontSize: 18,
                fontWeight: FontWeight.bold,
                color: GolemTheme.textPrimary,
              ),
            ),
            const SizedBox(height: 8),
            Text(
              _subtitle,
              style: const TextStyle(
                fontSize: 13,
                color: GolemTheme.textSecondary,
              ),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 24),
            ..._actions,
          ],
        ),
      ),
    );
  }

  IconData get _icon => switch (phase) {
        ProjectPhase.needsSetup => Icons.settings_suggest,
        ProjectPhase.readyToPlan => Icons.architecture,
        ProjectPhase.readyToBuild => Icons.rocket_launch,
        ProjectPhase.active => Icons.play_arrow,
      };

  String get _title => switch (phase) {
        ProjectPhase.needsSetup => 'Welcome to Golem',
        ProjectPhase.readyToPlan => 'Project configured',
        ProjectPhase.readyToBuild => 'Plan ready',
        ProjectPhase.active => '',
      };

  String get _subtitle => switch (phase) {
        ProjectPhase.needsSetup =>
          'This project needs configuration. Set up your stack, test commands, and preferences.',
        ProjectPhase.readyToPlan =>
          'Plan a feature to create tasks, or jump straight into a quick build.',
        ProjectPhase.readyToBuild =>
          '$taskCount tasks ready from your plan. Launch the implementer to start building.',
        ProjectPhase.active => '',
      };

  List<Widget> get _actions => switch (phase) {
        ProjectPhase.needsSetup => [
            _primaryButton('Set up project', onSetup),
            const SizedBox(height: 8),
            _secondaryButton('Skip', onSkip),
          ],
        ProjectPhase.readyToPlan => [
            _primaryButton('Plan a feature', onPlan),
            const SizedBox(height: 8),
            _secondaryButton('Quick build', onSkip),
          ],
        ProjectPhase.readyToBuild => [
            _primaryButton('Launch implementer', onBuild),
            const SizedBox(height: 8),
            _secondaryButton('Review plan first', onPlan),
          ],
        ProjectPhase.active => [],
      };

  Widget _primaryButton(String label, VoidCallback onPressed) {
    return SizedBox(
      width: double.infinity,
      child: ElevatedButton(
        onPressed: onPressed,
        style: ElevatedButton.styleFrom(
          backgroundColor: GolemTheme.green,
          foregroundColor: Colors.white,
          padding: const EdgeInsets.symmetric(vertical: 12),
        ),
        child: Text(label),
      ),
    );
  }

  Widget _secondaryButton(String label, VoidCallback onPressed) {
    return SizedBox(
      width: double.infinity,
      child: OutlinedButton(
        onPressed: onPressed,
        style: OutlinedButton.styleFrom(
          foregroundColor: GolemTheme.textSecondary,
          side: const BorderSide(color: GolemTheme.border),
          padding: const EdgeInsets.symmetric(vertical: 12),
        ),
        child: Text(label),
      ),
    );
  }
}
```

- [ ] **Step 2: Verify it builds**

Run: `cd ui/flutter && flutter build linux`
Expected: Clean build

- [ ] **Step 3: Commit**

```bash
git add ui/flutter/lib/views/welcome_view.dart
git commit -m "feat(ui): add state-aware welcome view

Shows contextual actions based on project phase: setup needed,
ready to plan, or ready to build. Replaces the empty 'No runs yet'
message with guided next steps."
```

---

### Task 5: Integrate welcome view into ProjectWorkspace

**Files:**
- Modify: `ui/flutter/lib/views/project_workspace.dart`

- [ ] **Step 1: Add welcome view detection and rendering**

Replace the entire `project_workspace.dart` with:

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../providers/connection.dart';
import '../providers/processes.dart';
import '../providers/project.dart';
import '../providers/runs.dart';
import '../models/process.dart';
import '../theme.dart';
import 'command_bar.dart';
import 'detail_panel.dart';
import 'run_feed.dart';
import 'welcome_view.dart';

class ProjectWorkspace extends ConsumerStatefulWidget {
  final String projectId;

  const ProjectWorkspace({super.key, required this.projectId});

  @override
  ConsumerState<ProjectWorkspace> createState() => _ProjectWorkspaceState();
}

class _ProjectWorkspaceState extends ConsumerState<ProjectWorkspace> {
  String? _selectedRunId;
  bool _skipWelcome = false;

  static const _defaultAgents = ['implementer', 'build-feature', 'one-shot', 'fix-bug'];

  Future<void> _launchRun(String agent, String goal) async {
    try {
      final api = ref.read(apiClientProvider);
      final id = await api.launchProcess(
        widget.projectId,
        LaunchRequest(
          command: 'run',
          config: LaunchConfig(),
          agentName: agent,
          goal: goal,
        ),
      );
      ref.read(processesProvider.notifier).refresh();
      ref.read(selectedProcessIdProvider.notifier).state = id;
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Failed to launch: $e')),
        );
      }
    }
  }

  Future<void> _launchCommand(String command) async {
    try {
      final api = ref.read(apiClientProvider);
      final id = await api.launchProcess(
        widget.projectId,
        LaunchRequest(
          command: command,
          config: LaunchConfig(),
        ),
      );
      ref.read(processesProvider.notifier).refresh();
      ref.read(selectedProcessIdProvider.notifier).state = id;
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Failed to launch $command: $e')),
        );
      }
    }
  }

  ProjectPhase _detectPhase(ProjectState? state, List runs) {
    if (runs.isNotEmpty || _skipWelcome) return ProjectPhase.active;

    if (state == null) return ProjectPhase.needsSetup;

    // Check if project has been configured (stack is set)
    if (state.project.stack.isEmpty) return ProjectPhase.needsSetup;

    // Check if there are todo tasks from a plan
    final todoCount = state.tasks.where((t) => t.status == 'todo').length;
    if (todoCount > 0) return ProjectPhase.readyToBuild;

    return ProjectPhase.readyToPlan;
  }

  @override
  Widget build(BuildContext context) {
    final allRuns = ref.watch(runsProvider);
    final projectRuns =
        allRuns.where((r) => r.projectId == widget.projectId).toList();
    final projectState = ref.watch(projectStateFamily(widget.projectId));
    final phase = _detectPhase(projectState, projectRuns);

    if (phase != ProjectPhase.active) {
      final todoCount = projectState?.tasks.where((t) => t.status == 'todo').length ?? 0;
      return WelcomeView(
        phase: phase,
        taskCount: todoCount,
        onSetup: () => _launchCommand('setup'),
        onPlan: () => _launchCommand('plan'),
        onBuild: () => _launchRun('implementer', 'Execute tasks from plan'),
        onSkip: () => setState(() => _skipWelcome = true),
      );
    }

    final selectedRun = _selectedRunId != null
        ? projectRuns.where((r) => r.runId == _selectedRunId).firstOrNull
        : projectRuns.isNotEmpty
            ? projectRuns.first
            : null;

    final standaloneProcessId = ref.watch(selectedProcessIdProvider);

    final processes = ref.watch(processesProvider);
    final String? processId;
    if (standaloneProcessId != null) {
      processId = standaloneProcessId;
    } else if (selectedRun != null) {
      processId = processes
          .where((p) => p.runId == selectedRun.runId)
          .firstOrNull
          ?.id;
    } else {
      processId = null;
    }

    return Column(
      children: [
        CommandBar(
          agents: _defaultAgents,
          onLaunch: _launchRun,
          onPlan: () => _launchCommand('plan'),
          onReview: () => _launchCommand('review'),
        ),
        Expanded(
          child: Row(
            children: [
              Expanded(
                flex: 55,
                child: Container(
                  decoration: const BoxDecoration(
                    border: Border(
                        right: BorderSide(color: GolemTheme.border)),
                  ),
                  child: RunFeed(
                    runs: projectRuns,
                    selectedRunId: _selectedRunId,
                    onSelect: (run) {
                      ref.read(selectedProcessIdProvider.notifier).state = null;
                      setState(() => _selectedRunId = run.runId);
                    },
                  ),
                ),
              ),
              Expanded(
                flex: 45,
                child: DetailPanel(
                  selectedRun: standaloneProcessId != null ? null : selectedRun,
                  processId: processId,
                  events: selectedRun != null && standaloneProcessId == null
                      ? ref.watch(runEventsFamily(selectedRun.runId))
                      : const [],
                  initialTab: standaloneProcessId != null ? 1 : null,
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}
```

Key changes:
- Added `_launchCommand` helper for plan/review/setup (non-agent commands)
- Added `_detectPhase` that reads `ProjectState` to determine welcome state
- Shows `WelcomeView` when phase is not `active`
- Updated `_defaultAgents` to include `implementer` first
- `CommandBar` now receives `onPlan` and `onReview` callbacks

- [ ] **Step 2: Verify it builds**

Run: `cd ui/flutter && flutter build linux`
Expected: Clean build

- [ ] **Step 3: Commit**

```bash
git add ui/flutter/lib/views/project_workspace.dart
git commit -m "feat(ui): integrate welcome view with phase detection

ProjectWorkspace now detects project phase from state.yaml and
shows the welcome view for new/unconfigured projects. Transitions
to normal workspace once runs exist or user skips."
```

---

### Task 6: Redesign command bar with Plan/Build/Review

**Files:**
- Modify: `ui/flutter/lib/views/command_bar.dart`

- [ ] **Step 1: Update CommandBar to accept plan/review callbacks and show action buttons**

Replace `command_bar.dart`:

```dart
import 'package:flutter/material.dart';

import '../theme.dart';
import 'agent_picker.dart';
import 'launch_dialog.dart';

/// Top bar: [Plan] [Build] [Review] | goal input + agent picker + Go + menu
class CommandBar extends StatefulWidget {
  final List<String> agents;
  final void Function(String agent, String goal) onLaunch;
  final VoidCallback? onPlan;
  final VoidCallback? onReview;

  const CommandBar({
    super.key,
    required this.agents,
    required this.onLaunch,
    this.onPlan,
    this.onReview,
  });

  @override
  State<CommandBar> createState() => _CommandBarState();
}

class _CommandBarState extends State<CommandBar> {
  final _goalController = TextEditingController();
  late String _selectedAgent;

  @override
  void initState() {
    super.initState();
    _selectedAgent =
        widget.agents.isNotEmpty ? widget.agents.first : 'implementer';
  }

  @override
  void dispose() {
    _goalController.dispose();
    super.dispose();
  }

  void _launch() {
    final goal = _goalController.text.trim();
    if (goal.isEmpty) return;
    widget.onLaunch(_selectedAgent, goal);
    _goalController.clear();
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: const BoxDecoration(
        color: GolemTheme.bgSurface,
        border: Border(bottom: BorderSide(color: GolemTheme.border)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              // Action buttons
              _actionButton('Plan', Icons.architecture, widget.onPlan),
              const SizedBox(width: 6),
              _actionButton('Review', Icons.rate_review, widget.onReview),
              const SizedBox(width: 12),
              Container(
                width: 1,
                height: 28,
                color: GolemTheme.border,
              ),
              const SizedBox(width: 12),
              // Goal input
              Expanded(
                child: TextField(
                  controller: _goalController,
                  style: const TextStyle(fontSize: 13),
                  decoration: InputDecoration(
                    hintText: 'Describe what you want to build...',
                    hintStyle: TextStyle(
                      fontSize: 13,
                      color: GolemTheme.textSecondary.withValues(alpha: 0.5),
                    ),
                    isDense: true,
                    contentPadding: const EdgeInsets.symmetric(
                        horizontal: 12, vertical: 10),
                    border: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(6),
                      borderSide:
                          const BorderSide(color: GolemTheme.border),
                    ),
                    enabledBorder: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(6),
                      borderSide:
                          const BorderSide(color: GolemTheme.border),
                    ),
                    focusedBorder: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(6),
                      borderSide:
                          const BorderSide(color: GolemTheme.accent),
                    ),
                  ),
                  onSubmitted: (_) => _launch(),
                ),
              ),
              const SizedBox(width: 8),
              ElevatedButton(
                onPressed: _launch,
                style: ElevatedButton.styleFrom(
                  backgroundColor: GolemTheme.green,
                  foregroundColor: Colors.white,
                  padding: const EdgeInsets.symmetric(
                      horizontal: 16, vertical: 10),
                  minimumSize: Size.zero,
                ),
                child: const Text('Go', style: TextStyle(fontSize: 12)),
              ),
              const SizedBox(width: 4),
              IconButton(
                icon: const Icon(Icons.more_vert, size: 18),
                color: GolemTheme.textSecondary,
                tooltip: 'Advanced launch options',
                splashRadius: 16,
                padding: EdgeInsets.zero,
                constraints: const BoxConstraints(minWidth: 32, minHeight: 32),
                onPressed: () {
                  showDialog(
                    context: context,
                    builder: (_) => const LaunchDialog(),
                  );
                },
              ),
            ],
          ),
          const SizedBox(height: 8),
          AgentPicker(
            agents: widget.agents,
            selected: _selectedAgent,
            onChanged: (agent) =>
                setState(() => _selectedAgent = agent),
          ),
        ],
      ),
    );
  }

  Widget _actionButton(String label, IconData icon, VoidCallback? onPressed) {
    return OutlinedButton.icon(
      onPressed: onPressed,
      icon: Icon(icon, size: 14),
      label: Text(label, style: const TextStyle(fontSize: 12)),
      style: OutlinedButton.styleFrom(
        foregroundColor: GolemTheme.textPrimary,
        side: const BorderSide(color: GolemTheme.border),
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
        minimumSize: Size.zero,
      ),
    );
  }
}
```

- [ ] **Step 2: Verify it builds**

Run: `cd ui/flutter && flutter build linux`
Expected: Clean build

- [ ] **Step 3: Run full Go test suite**

Run: `go test ./... -count=1`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add ui/flutter/lib/views/command_bar.dart
git commit -m "feat(ui): redesign command bar with Plan/Build/Review actions

Plan and Review are now prominent buttons in the command bar instead
of hidden behind the advanced menu. Goal input and agent picker are
for the Build action. Implementer is the first agent in the picker."
```

---

## Execution Order

**Group A** (Go backend): Tasks 1 → 2 → 3 (sequential — each builds on prior)
**Group B** (Flutter UI): Tasks 4 → 5 → 6 (sequential — welcome view used by workspace)

Groups A and B are independent and can run in parallel.

After all tasks: `go install . && golem ui install && golem ui` to verify end-to-end.
