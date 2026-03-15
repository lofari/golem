# Unified UI — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the Golem Flutter GUI around blueprint-centric workflows: collapsible rail + tabs app shell, chronological activity feed, project workspace with command bar + run feed + detail panel (State/Terminal/Timeline tabs).

**Architecture:** Replace the current `ShellView` (top bar + process tabs + content area) with a new app shell: left icon rail (projects + activity), top tab bar, and routed content area. The Activity feed replaces `DashboardView`. Each project tab opens a workspace with three zones: command bar (top), run feed (left ~55%), detail panel (right ~45% with State/Terminal/Timeline tabs). Data flows from `golem serve` via existing WebSocket + REST API. New models for blueprint runs (`RunInfo`, `EngineEvent`) supplement existing `ProcessInfo`. Riverpod providers manage per-project run state and event streams.

**Tech Stack:** Flutter 3.x, Dart 3, Riverpod, xterm.dart, existing `GolemApiClient` + `GolemWebSocket`.

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| **Models** | | |
| `lib/models/run.dart` | Create | `RunInfo`, `EngineEvent`, `PipelineProgress` data classes for blueprint runs |
| `lib/models/process.dart` | Modify | Add `agentName`, `goal`, `runId` fields to `ProcessInfo` and `LaunchRequest` |
| **Providers** | | |
| `lib/providers/runs.dart` | Create | `runsProvider` (per-project run list), `runEventsProvider` (NDJSON event stream), `activeRunsProvider` |
| `lib/providers/projects.dart` | Create | Multi-project registry extracted from `project.dart`, with status dots |
| `lib/providers/processes.dart` | Modify | Add agent/goal/runId to process creation |
| **Views — App Shell** | | |
| `lib/views/app_shell.dart` | Create | New root: collapsible `NavigationRail` + `TabBar` + content router. Replaces `ShellView` |
| `lib/views/status_bar.dart` | Create | Bottom status bar (connection + active runs count). Extracted from `ShellView._StatusBar` |
| **Views — Activity** | | |
| `lib/views/activity_feed.dart` | Create | Chronological run feed across all projects with filter chips |
| `lib/views/run_card.dart` | Create | Shared run card widget: status dot, project badge, agent, goal, progress bar, PR link |
| **Views — Project Workspace** | | |
| `lib/views/project_workspace.dart` | Create | Three-zone layout: command bar + run feed + detail panel |
| `lib/views/command_bar.dart` | Create | Goal input + agent picker chips + Run button |
| `lib/views/run_feed.dart` | Create | Active + Recent run sections in project workspace |
| `lib/views/detail_panel.dart` | Create | Tabbed panel: State / Terminal / Timeline |
| `lib/views/detail_state.dart` | Create | State tab: current step, pipeline state KV, changes, project context |
| `lib/views/detail_terminal.dart` | Create | Terminal tab: xterm.dart for interactive/attach/ad-hoc |
| `lib/views/detail_timeline.dart` | Create | Timeline tab: event log from log.json |
| **Views — Widgets** | | |
| `lib/views/pipeline_progress.dart` | Create | Segmented progress bar widget (green/yellow/gray segments) |
| `lib/views/agent_picker.dart` | Create | Agent selection chips widget |
| **Cleanup** | | |
| `lib/views/shell.dart` | Remove | Replaced by `app_shell.dart` |
| `lib/views/dashboard.dart` | Remove | Replaced by `activity_feed.dart` + `project_workspace.dart` |

**Retained views** (not modified, still accessible from new UI):
- `lib/views/settings_dialog.dart` — opened from rail footer gear icon
- `lib/views/graph_explorer.dart` — opened from rail footer graph icon
- `lib/views/process_view.dart` — used by detail terminal for existing processes
- `lib/views/project_switcher.dart` — no longer needed (rail replaces it), can be deleted later
- `lib/views/launch_dialog.dart` — no longer needed (command bar replaces it), can be deleted later
| `lib/main.dart` | Modify | Replace `ShellView` with `AppShell` |

---

## Chunk 1: Models and Providers

### Task 1: Create run models

**Files:**
- Create: `ui/flutter/lib/models/run.dart`

- [ ] **Step 1: Create RunInfo and EngineEvent models**

```dart
// lib/models/run.dart

class EngineEvent {
  final String type;
  final DateTime timestamp;
  final String? step;
  final String? stepType;
  final String? status;
  final int? durationMs;
  final String? agent;
  final String? goal;
  final String? runId;
  final String? predicate;
  final int? iteration;
  final int? max;
  final String? reason;
  final String? errorType;
  final String? action;
  final int? attempt;

  const EngineEvent({
    required this.type,
    required this.timestamp,
    this.step,
    this.stepType,
    this.status,
    this.durationMs,
    this.agent,
    this.goal,
    this.runId,
    this.predicate,
    this.iteration,
    this.max,
    this.reason,
    this.errorType,
    this.action,
    this.attempt,
  });

  factory EngineEvent.fromJson(Map<String, dynamic> json) {
    return EngineEvent(
      type: json['type'] as String,
      timestamp: DateTime.parse(json['timestamp'] as String),
      step: json['step'] as String?,
      stepType: json['step-type'] as String?,
      status: json['status'] as String?,
      durationMs: (json['duration-ms'] as num?)?.toInt(),
      agent: json['agent'] as String?,
      goal: json['goal'] as String?,
      runId: json['run-id'] as String?,
      predicate: json['predicate'] as String?,
      iteration: (json['iteration'] as num?)?.toInt(),
      max: (json['max'] as num?)?.toInt(),
      reason: json['reason'] as String?,
      errorType: json['error-type'] as String?,
      action: json['action'] as String?,
      attempt: (json['attempt'] as num?)?.toInt(),
    );
  }
}

class RunInfo {
  final String runId;
  final String agentName;
  final String goal;
  final String projectId;
  final String projectName;
  final String status; // running, success, error
  final DateTime startedAt;
  final Duration? duration;
  final String? prUrl;
  final String? branch;
  final String? haltReason;
  final List<StepProgress> steps;

  const RunInfo({
    required this.runId,
    required this.agentName,
    required this.goal,
    required this.projectId,
    required this.projectName,
    required this.status,
    required this.startedAt,
    this.duration,
    this.prUrl,
    this.branch,
    this.haltReason,
    required this.steps,
  });
}

class StepProgress {
  final String name;
  final String type; // agentic, builtin, shell
  final String status; // pending, running, success, error, skipped
  final Duration? duration;
  final DateTime? startedAt; // for elapsed time on running steps
  final int toolCallCount;

  const StepProgress({
    required this.name,
    required this.type,
    required this.status,
    this.duration,
    this.startedAt,
    this.toolCallCount = 0,
  });
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd ui/flutter && dart analyze lib/models/run.dart`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add ui/flutter/lib/models/run.dart
git commit -m "feat(ui): add RunInfo and EngineEvent models for blueprint runs"
```

---

### Task 2: Add agent/goal/runId to ProcessInfo and LaunchRequest

**Files:**
- Modify: `ui/flutter/lib/models/process.dart`

- [ ] **Step 1: Add fields to ProcessInfo**

Add `agentName`, `goal`, and `runId` as optional fields to `ProcessInfo`:

```dart
class ProcessInfo {
  final String id;
  final String command;
  final String status;
  final String? startedAt;
  final int? pid;
  final String? agentName;
  final String? goal;
  final String? runId;

  ProcessInfo({
    required this.id,
    required this.command,
    required this.status,
    this.startedAt,
    this.pid,
    this.agentName,
    this.goal,
    this.runId,
  });

  factory ProcessInfo.fromJson(Map<String, dynamic> json) {
    return ProcessInfo(
      id: json['id'] as String,
      command: json['command'] as String,
      status: json['status'] as String? ?? 'unknown',
      startedAt: json['startedAt'] as String?,
      pid: json['pid'] as int?,
      agentName: json['agentName'] as String?,
      goal: json['goal'] as String?,
      runId: json['runId'] as String?,
    );
  }
}
```

- [ ] **Step 2: Add agent/goal to LaunchRequest**

Add `agentName` and `goal` to `LaunchRequest`:

```dart
class LaunchRequest {
  final String command;
  final LaunchConfig config;
  final String? agentName;
  final String? goal;

  LaunchRequest({
    required this.command,
    required this.config,
    this.agentName,
    this.goal,
  });

  Map<String, dynamic> toJson() => {
    'command': command,
    'config': config.toJson(),
    if (agentName != null) 'agentName': agentName,
    if (goal != null) 'goal': goal,
  };
}
```

- [ ] **Step 3: Verify build**

Run: `cd ui/flutter && dart analyze lib/models/process.dart`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add ui/flutter/lib/models/process.dart
git commit -m "feat(ui): add agentName, goal, runId to ProcessInfo and LaunchRequest"
```

---

### Task 3: Create runs provider

**Files:**
- Create: `ui/flutter/lib/providers/runs.dart`

- [ ] **Step 1: Create RunsNotifier and event stream provider**

```dart
// lib/providers/runs.dart
import 'dart:async';
import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/websocket.dart';
import '../models/run.dart';
import 'connection.dart';

/// Tracks all runs across a single project, built from engine events.
class RunsNotifier extends StateNotifier<List<RunInfo>> {
  RunsNotifier() : super([]);

  void processEvent(EngineEvent event) {
    switch (event.type) {
      case 'pipeline-start':
        final run = RunInfo(
          runId: event.runId ?? '',
          agentName: event.agent ?? '',
          goal: event.goal ?? '',
          projectId: '', // filled by caller
          projectName: '', // filled by caller
          status: 'running',
          startedAt: event.timestamp,
          steps: [],
        );
        state = [run, ...state];
      case 'step-start':
        _updateCurrentRun(event.runId, (run) {
          final steps = [...run.steps];
          steps.add(StepProgress(
            name: event.step ?? '',
            type: event.stepType ?? '',
            status: 'running',
            startedAt: event.timestamp,
          ));
          return _copyRun(run, steps: steps);
        });
      case 'step-end':
        _updateCurrentRun(event.runId, (run) {
          final steps = run.steps.map((s) {
            if (s.name == event.step && s.status == 'running') {
              return StepProgress(
                name: s.name,
                type: s.type,
                status: event.status ?? 'success',
                duration: event.durationMs != null
                    ? Duration(milliseconds: event.durationMs!)
                    : null,
              );
            }
            return s;
          }).toList();
          return _copyRun(run, steps: steps);
        });
      case 'pipeline-end':
        _updateCurrentRun(event.runId, (run) {
          return _copyRun(
            run,
            status: event.status == 'success' ? 'success' : 'error',
            duration: event.durationMs != null
                ? Duration(milliseconds: event.durationMs!)
                : null,
          );
        });
      case 'conditional-skip':
        // Mark skipped steps — no action needed for now
        break;
      default:
        break;
    }
  }

  void _updateCurrentRun(String? runId, RunInfo Function(RunInfo) updater) {
    if (runId == null) return;
    state = state.map((r) {
      if (r.runId == runId) return updater(r);
      return r;
    }).toList();
  }

  RunInfo _copyRun(
    RunInfo run, {
    String? status,
    Duration? duration,
    List<StepProgress>? steps,
    String? prUrl,
    String? branch,
    String? haltReason,
  }) {
    return RunInfo(
      runId: run.runId,
      agentName: run.agentName,
      goal: run.goal,
      projectId: run.projectId,
      projectName: run.projectName,
      status: status ?? run.status,
      startedAt: run.startedAt,
      duration: duration ?? run.duration,
      prUrl: prUrl ?? run.prUrl,
      branch: branch ?? run.branch,
      haltReason: haltReason ?? run.haltReason,
      steps: steps ?? run.steps,
    );
  }
}

final runsProvider =
    StateNotifierProvider<RunsNotifier, List<RunInfo>>((ref) {
  return RunsNotifier();
});

/// Active runs only (status == 'running').
final activeRunsProvider = Provider<List<RunInfo>>((ref) {
  final runs = ref.watch(runsProvider);
  return runs.where((r) => r.status == 'running').toList();
});

/// Connects to golem serve WebSocket and feeds EngineEvents into RunsNotifier.
/// Initialize this provider once in AppShell to start consuming events.
final runEventStreamProvider = Provider<void>((ref) {
  final api = ref.watch(apiClientProvider);
  final runs = ref.read(runsProvider.notifier);

  // Subscribe to the state watch WebSocket which carries engine events.
  // The server broadcasts NDJSON EngineEvent lines on the same channel.
  final ws = GolemWebSocket(
    url: api.stateWatchUrl(),
    onMessage: (data) {
      if (data['type'] == 'engine-event') {
        final event = EngineEvent.fromJson(
            data['payload'] as Map<String, dynamic>);
        runs.processEvent(event);
      }
    },
  );
  ws.connect();
  ref.onDispose(() => ws.dispose());
});
```

- [ ] **Step 2: Verify it compiles**

Run: `cd ui/flutter && dart analyze lib/providers/runs.dart`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add ui/flutter/lib/providers/runs.dart
git commit -m "feat(ui): add runs provider with engine event processing"
```

---

### Task 4: Create multi-project provider

**Files:**
- Create: `ui/flutter/lib/providers/projects.dart`

- [ ] **Step 1: Create provider**

The existing `projectListProvider` in `providers/graph.dart` fetches projects for the switcher. Extract and extend it:

```dart
// lib/providers/projects.dart
import 'package:flutter_riverpod/flutter_riverpod.dart';

// Re-export projectListProvider from graph.dart as projectsProvider.
// The existing ProjectListNotifier in providers/graph.dart already
// calls api.listProjects() — no need to duplicate it.
import 'graph.dart' show projectListProvider;

final projectsProvider = projectListProvider;

/// Currently selected project tab ID (null = Activity feed).
final selectedProjectIdProvider = StateProvider<String?>((ref) => null);

/// Set of open project tab IDs (persists during session).
final openProjectTabsProvider = StateProvider<Set<String>>((ref) => {});
```

- [ ] **Step 2: Verify it compiles**

Run: `cd ui/flutter && dart analyze lib/providers/projects.dart`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add ui/flutter/lib/providers/projects.dart
git commit -m "feat(ui): add multi-project provider with tab state"
```

---

## Chunk 2: App Shell and Activity Feed

### Task 5: Create pipeline progress bar widget

**Files:**
- Create: `ui/flutter/lib/views/pipeline_progress.dart`

- [ ] **Step 1: Implement segmented progress bar**

```dart
// lib/views/pipeline_progress.dart
import 'package:flutter/material.dart';

import '../models/run.dart';
import '../theme.dart';

/// Segmented pipeline progress bar. Each segment = one step.
/// Colors: green (success), yellow pulsing (running), red (error), gray (pending).
class PipelineProgressBar extends StatelessWidget {
  final List<StepProgress> steps;
  final double height;

  const PipelineProgressBar({
    super.key,
    required this.steps,
    this.height = 6,
  });

  Color _stepColor(String status) {
    return switch (status) {
      'success' => GolemTheme.green,
      'running' => GolemTheme.yellow,
      'error' => GolemTheme.red,
      'skipped' => GolemTheme.textSecondary,
      _ => GolemTheme.bgElevated, // pending
    };
  }

  @override
  Widget build(BuildContext context) {
    if (steps.isEmpty) return const SizedBox.shrink();

    return ClipRRect(
      borderRadius: BorderRadius.circular(3),
      child: SizedBox(
        height: height,
        child: Row(
          children: steps.map((s) {
            return Expanded(
              child: Container(
                margin: const EdgeInsets.symmetric(horizontal: 0.5),
                color: _stepColor(s.status),
              ),
            );
          }).toList(),
        ),
      ),
    );
  }
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd ui/flutter && dart analyze lib/views/pipeline_progress.dart`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add ui/flutter/lib/views/pipeline_progress.dart
git commit -m "feat(ui): add pipeline progress bar widget"
```

---

### Task 6: Create run card widget

**Files:**
- Create: `ui/flutter/lib/views/run_card.dart`

- [ ] **Step 1: Implement run card**

```dart
// lib/views/run_card.dart
import 'package:flutter/material.dart';

import '../models/run.dart';
import '../theme.dart';
import 'pipeline_progress.dart';

/// Card for a single run in activity feed or project run feed.
/// Uses a pulsing animation for running status dots per spec.
class RunCard extends StatefulWidget {
  final RunInfo run;
  final VoidCallback? onTap;
  final bool showProjectBadge;

  const RunCard({
    super.key,
    required this.run,
    this.onTap,
    this.showProjectBadge = false,
  });

  @override
  State<RunCard> createState() => _RunCardState();
}

class _RunCardState extends State<RunCard>
    with SingleTickerProviderStateMixin {
  AnimationController? _pulseController;
  Animation<double>? _pulseAnimation;

  RunInfo get run => widget.run;
  bool get showProjectBadge => widget.showProjectBadge;

  @override
  void initState() {
    super.initState();
    if (run.status == 'running') {
      _pulseController = AnimationController(
        vsync: this,
        duration: const Duration(seconds: 1),
      )..repeat(reverse: true);
      _pulseAnimation = Tween<double>(begin: 0.3, end: 1.0).animate(
        CurvedAnimation(parent: _pulseController!, curve: Curves.easeInOut),
      );
    }
  }

  @override
  void dispose() {
    _pulseController?.dispose();
    super.dispose();
  }

  Color _statusColor() {
    return switch (run.status) {
      'running' => GolemTheme.yellow,
      'success' => GolemTheme.green,
      'error' => GolemTheme.red,
      _ => GolemTheme.textSecondary,
    };
  }

  String _relativeTime() {
    final diff = DateTime.now().difference(run.startedAt);
    if (diff.inSeconds < 60) return '${diff.inSeconds}s ago';
    if (diff.inMinutes < 60) return '${diff.inMinutes}m ago';
    if (diff.inHours < 24) return '${diff.inHours}h ago';
    return '${diff.inDays}d ago';
  }

  String _formatDuration(Duration d) {
    if (d.inSeconds < 60) return '${d.inSeconds}s';
    final mins = d.inMinutes;
    final secs = d.inSeconds % 60;
    return '${mins}m${secs.toString().padLeft(2, '0')}s';
  }

  Widget _buildStatusDot() {
    final dot = Container(
      width: 8,
      height: 8,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: _statusColor(),
      ),
    );
    if (run.status == 'running' && _pulseAnimation != null) {
      return FadeTransition(opacity: _pulseAnimation!, child: dot);
    }
    return dot;
  }

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: const EdgeInsets.symmetric(vertical: 4),
      child: InkWell(
        onTap: widget.onTap,
        borderRadius: BorderRadius.circular(8),
        child: Padding(
          padding: const EdgeInsets.all(12),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  // Status dot (pulsing when running)
                  _buildStatusDot(),
                  const SizedBox(width: 8),
                  // Project badge (optional)
                  if (showProjectBadge && run.projectName.isNotEmpty) ...[
                    Container(
                      padding: const EdgeInsets.symmetric(
                          horizontal: 6, vertical: 2),
                      decoration: BoxDecoration(
                        color: GolemTheme.accent.withValues(alpha: 0.15),
                        borderRadius: BorderRadius.circular(4),
                      ),
                      child: Text(
                        run.projectName,
                        style: const TextStyle(
                          fontSize: 10,
                          fontWeight: FontWeight.w600,
                          color: GolemTheme.accent,
                        ),
                      ),
                    ),
                    const SizedBox(width: 8),
                  ],
                  // Agent name
                  Text(
                    run.agentName,
                    style: const TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                  const Spacer(),
                  // Time
                  Text(
                    run.status == 'running'
                        ? _relativeTime()
                        : run.duration != null
                            ? _formatDuration(run.duration!)
                            : _relativeTime(),
                    style: GolemTheme.metaStyle(fontSize: 11),
                  ),
                ],
              ),
              const SizedBox(height: 4),
              // Goal text
              Text(
                run.goal,
                style: const TextStyle(fontSize: 12),
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
              ),
              // Progress bar for running, or PR link/halt reason for completed
              if (run.status == 'running' && run.steps.isNotEmpty) ...[
                const SizedBox(height: 8),
                PipelineProgressBar(steps: run.steps),
                const SizedBox(height: 4),
                // Step labels
                Wrap(
                  spacing: 8,
                  children: run.steps.map((s) {
                    return Text(
                      s.name,
                      style: TextStyle(
                        fontSize: 9,
                        color: s.status == 'running'
                            ? GolemTheme.yellow
                            : s.status == 'success'
                                ? GolemTheme.green
                                : GolemTheme.textSecondary,
                      ),
                    );
                  }).toList(),
                ),
              ],
              if (run.status == 'success' && run.prUrl != null) ...[
                const SizedBox(height: 4),
                Text(
                  run.prUrl!,
                  style: const TextStyle(
                    fontSize: 11,
                    color: GolemTheme.accent,
                    decoration: TextDecoration.underline,
                  ),
                ),
              ],
              if (run.status == 'error' && run.haltReason != null) ...[
                const SizedBox(height: 4),
                Text(
                  run.haltReason!,
                  style: const TextStyle(fontSize: 11, color: GolemTheme.red),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd ui/flutter && dart analyze lib/views/run_card.dart`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add ui/flutter/lib/views/run_card.dart
git commit -m "feat(ui): add run card widget with progress bar and status"
```

---

### Task 7: Create activity feed view

**Files:**
- Create: `ui/flutter/lib/views/activity_feed.dart`

- [ ] **Step 1: Implement activity feed**

```dart
// lib/views/activity_feed.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/run.dart';
import '../providers/runs.dart';
import '../theme.dart';
import 'run_card.dart';

class ActivityFeed extends ConsumerStatefulWidget {
  final void Function(RunInfo run)? onRunTap;

  const ActivityFeed({super.key, this.onRunTap});

  @override
  ConsumerState<ActivityFeed> createState() => _ActivityFeedState();
}

class _ActivityFeedState extends ConsumerState<ActivityFeed> {
  String _filter = 'all'; // all, running, failed

  @override
  Widget build(BuildContext context) {
    final runs = ref.watch(runsProvider);

    final filtered = switch (_filter) {
      'running' => runs.where((r) => r.status == 'running').toList(),
      'failed' => runs.where((r) => r.status == 'error').toList(),
      _ => runs,
    };

    return Column(
      children: [
        // Filter bar
        Padding(
          padding: const EdgeInsets.fromLTRB(24, 16, 24, 8),
          child: Row(
            children: [
              const Text(
                'Activity',
                style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600),
              ),
              const Spacer(),
              ...['all', 'running', 'failed'].map((f) => Padding(
                    padding: const EdgeInsets.only(left: 4),
                    child: ChoiceChip(
                      label: Text(
                        f[0].toUpperCase() + f.substring(1),
                        style: const TextStyle(fontSize: 11),
                      ),
                      selected: _filter == f,
                      onSelected: (_) => setState(() => _filter = f),
                      selectedColor: GolemTheme.accent.withValues(alpha: 0.2),
                      backgroundColor: GolemTheme.bgPrimary,
                      side: const BorderSide(color: GolemTheme.border),
                      padding: EdgeInsets.zero,
                      labelPadding:
                          const EdgeInsets.symmetric(horizontal: 6),
                      visualDensity: VisualDensity.compact,
                    ),
                  )),
            ],
          ),
        ),
        // Run list
        Expanded(
          child: filtered.isEmpty
              ? Center(
                  child: Text(
                    _filter == 'all'
                        ? 'No runs yet. Launch an agent to get started.'
                        : 'No ${_filter} runs.',
                    style: const TextStyle(
                      fontSize: 13,
                      color: GolemTheme.textSecondary,
                    ),
                  ),
                )
              : ListView.builder(
                  padding: const EdgeInsets.symmetric(horizontal: 24),
                  itemCount: filtered.length,
                  itemBuilder: (context, index) {
                    final run = filtered[index];
                    return RunCard(
                      run: run,
                      showProjectBadge: true,
                      onTap: () => widget.onRunTap?.call(run),
                    );
                  },
                ),
        ),
      ],
    );
  }
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd ui/flutter && dart analyze lib/views/activity_feed.dart`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add ui/flutter/lib/views/activity_feed.dart
git commit -m "feat(ui): add activity feed view with filter chips"
```

---

### Task 8: Create app shell with collapsible rail + tabs

**Files:**
- Create: `ui/flutter/lib/views/app_shell.dart`
- Create: `ui/flutter/lib/views/status_bar.dart`
- Modify: `ui/flutter/lib/main.dart`

- [ ] **Step 1: Create status bar (extracted from ShellView)**

```dart
// lib/views/status_bar.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../providers/connection.dart';
import '../providers/runs.dart';
import '../theme.dart';

class StatusBar extends ConsumerWidget {
  const StatusBar({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final connected = ref.watch(connectionProvider);
    final activeRuns = ref.watch(activeRunsProvider);

    return Container(
      height: 28,
      padding: const EdgeInsets.symmetric(horizontal: 12),
      decoration: const BoxDecoration(
        color: GolemTheme.bgSurface,
        border: Border(top: BorderSide(color: GolemTheme.border)),
      ),
      child: Row(
        children: [
          Container(
            width: 8,
            height: 8,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: connected ? GolemTheme.green : GolemTheme.red,
            ),
          ),
          const SizedBox(width: 8),
          Text(
            connected ? 'golem serve' : 'Disconnected',
            style: const TextStyle(
              fontSize: 11,
              color: GolemTheme.textSecondary,
            ),
          ),
          const Spacer(),
          if (activeRuns.isNotEmpty)
            Text(
              '${activeRuns.length} active run${activeRuns.length != 1 ? "s" : ""}',
              style: const TextStyle(
                fontSize: 11,
                color: GolemTheme.yellow,
              ),
            ),
        ],
      ),
    );
  }
}
```

- [ ] **Step 2: Verify status bar compiles**

Run: `cd ui/flutter && dart analyze lib/views/status_bar.dart`
Expected: no errors.

- [ ] **Step 3: Create app shell**

```dart
// lib/views/app_shell.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/project.dart';
import '../models/run.dart';
import '../providers/projects.dart';
import '../providers/runs.dart';
import '../theme.dart';
import 'activity_feed.dart';
import 'graph_explorer.dart';
import 'settings_dialog.dart';
import 'status_bar.dart';

class AppShell extends ConsumerStatefulWidget {
  const AppShell({super.key});

  @override
  ConsumerState<AppShell> createState() => _AppShellState();
}

class _AppShellState extends ConsumerState<AppShell> {
  bool _railExpanded = false;

  Color _projectStatusColor(String projectId) {
    final runs = ref.read(runsProvider);
    final projectRuns = runs.where((r) => r.projectId == projectId);
    if (projectRuns.any((r) => r.status == 'error')) return GolemTheme.red;
    if (projectRuns.any((r) => r.status == 'running')) return GolemTheme.yellow;
    return GolemTheme.green;
  }

  void _openProject(String projectId) {
    final openTabs = ref.read(openProjectTabsProvider);
    ref.read(openProjectTabsProvider.notifier).state = {...openTabs, projectId};
    ref.read(selectedProjectIdProvider.notifier).state = projectId;
  }

  void _closeProject(String projectId) {
    final openTabs = ref.read(openProjectTabsProvider);
    final newTabs = {...openTabs}..remove(projectId);
    ref.read(openProjectTabsProvider.notifier).state = newTabs;
    final selected = ref.read(selectedProjectIdProvider);
    if (selected == projectId) {
      ref.read(selectedProjectIdProvider.notifier).state =
          newTabs.isEmpty ? null : newTabs.last;
    }
  }

  void _onRunTap(RunInfo run) {
    _openProject(run.projectId);
  }

  @override
  Widget build(BuildContext context) {
    // Initialize event stream — watching ensures the WebSocket stays alive.
    ref.watch(runEventStreamProvider);

    final projects = ref.watch(projectsProvider);
    final selectedId = ref.watch(selectedProjectIdProvider);
    final openTabs = ref.watch(openProjectTabsProvider);

    return Scaffold(
      body: Column(
        children: [
          Expanded(
            child: Row(
              children: [
                // Left rail
                MouseRegion(
                  onEnter: (_) => setState(() => _railExpanded = true),
                  onExit: (_) => setState(() => _railExpanded = false),
                  child: AnimatedContainer(
                    duration: const Duration(milliseconds: 150),
                    width: _railExpanded ? 160 : 40,
                    decoration: const BoxDecoration(
                      color: GolemTheme.bgSurface,
                      border: Border(
                          right: BorderSide(color: GolemTheme.border)),
                    ),
                    child: Column(
                      children: [
                        // Activity icon
                        _RailItem(
                          icon: Icons.bolt,
                          label: 'Activity',
                          expanded: _railExpanded,
                          selected: selectedId == null,
                          onTap: () => ref
                              .read(selectedProjectIdProvider.notifier)
                              .state = null,
                        ),
                        const Divider(
                            height: 1, color: GolemTheme.border),
                        // Project icons
                        Expanded(
                          child: ListView(
                            children: projects.map((p) {
                              return _RailItem(
                                icon: null,
                                letter: p.name.isNotEmpty
                                    ? p.name[0].toUpperCase()
                                    : '?',
                                label: p.name,
                                expanded: _railExpanded,
                                selected: selectedId == p.id,
                                statusColor: _projectStatusColor(p.id),
                                onTap: () => _openProject(p.id),
                              );
                            }).toList(),
                          ),
                        ),
                        const Divider(
                            height: 1, color: GolemTheme.border),
                        // Footer: graph explorer + settings
                        _RailItem(
                          icon: Icons.account_tree,
                          label: 'Graph',
                          expanded: _railExpanded,
                          selected: false,
                          onTap: () => showDialog(
                            context: context,
                            builder: (_) => const GraphExplorer(),
                          ),
                        ),
                        _RailItem(
                          icon: Icons.settings,
                          label: 'Settings',
                          expanded: _railExpanded,
                          selected: false,
                          onTap: () => showDialog(
                            context: context,
                            builder: (_) => const SettingsDialog(),
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
                // Main content area
                Expanded(
                  child: Column(
                    children: [
                      // Tab bar
                      if (openTabs.isNotEmpty)
                        _TabBar(
                          projects: projects,
                          openTabs: openTabs,
                          selectedId: selectedId,
                          onSelect: (id) => ref
                              .read(selectedProjectIdProvider.notifier)
                              .state = id,
                          onClose: _closeProject,
                          onActivity: () => ref
                              .read(selectedProjectIdProvider.notifier)
                              .state = null,
                        ),
                      // Content
                      Expanded(
                        child: selectedId == null
                            ? ActivityFeed(onRunTap: _onRunTap)
                            : Center(
                                child: Text(
                                  'Project workspace: $selectedId',
                                  style: const TextStyle(
                                    color: GolemTheme.textSecondary,
                                  ),
                                ),
                              ),
                        // TODO: Replace Center with ProjectWorkspace in Task 11
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),
          const StatusBar(),
        ],
      ),
    );
  }
}

class _RailItem extends StatelessWidget {
  final IconData? icon;
  final String? letter;
  final String label;
  final bool expanded;
  final bool selected;
  final Color? statusColor;
  final VoidCallback onTap;

  const _RailItem({
    this.icon,
    this.letter,
    required this.label,
    required this.expanded,
    required this.selected,
    this.statusColor,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      child: Container(
        height: 40,
        padding: const EdgeInsets.symmetric(horizontal: 8),
        color: selected ? GolemTheme.accent.withValues(alpha: 0.1) : null,
        child: Row(
          children: [
            SizedBox(
              width: 24,
              child: Stack(
                children: [
                  Center(
                    child: icon != null
                        ? Icon(icon, size: 18,
                            color: selected
                                ? GolemTheme.accent
                                : GolemTheme.textSecondary)
                        : Container(
                            width: 24,
                            height: 24,
                            decoration: BoxDecoration(
                              color: GolemTheme.bgElevated,
                              borderRadius: BorderRadius.circular(4),
                            ),
                            alignment: Alignment.center,
                            child: Text(
                              letter ?? '?',
                              style: TextStyle(
                                fontSize: 12,
                                fontWeight: FontWeight.w600,
                                color: selected
                                    ? GolemTheme.accent
                                    : GolemTheme.textPrimary,
                              ),
                            ),
                          ),
                  ),
                  if (statusColor != null)
                    Positioned(
                      right: 0,
                      bottom: 6,
                      child: Container(
                        width: 6,
                        height: 6,
                        decoration: BoxDecoration(
                          shape: BoxShape.circle,
                          color: statusColor,
                        ),
                      ),
                    ),
                ],
              ),
            ),
            if (expanded) ...[
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  label,
                  style: TextStyle(
                    fontSize: 12,
                    color: selected
                        ? GolemTheme.accent
                        : GolemTheme.textPrimary,
                  ),
                  overflow: TextOverflow.ellipsis,
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _TabBar extends StatelessWidget {
  final List<ProjectInfo> projects;
  final Set<String> openTabs;
  final String? selectedId;
  final ValueChanged<String> onSelect;
  final ValueChanged<String> onClose;
  final VoidCallback onActivity;

  const _TabBar({
    required this.projects,
    required this.openTabs,
    required this.selectedId,
    required this.onSelect,
    required this.onClose,
    required this.onActivity,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 36,
      padding: const EdgeInsets.symmetric(horizontal: 8),
      decoration: const BoxDecoration(
        border: Border(bottom: BorderSide(color: GolemTheme.border)),
      ),
      child: Row(
        children: [
          // Activity tab
          _Tab(
            label: 'Activity',
            icon: Icons.bolt,
            selected: selectedId == null,
            onTap: onActivity,
          ),
          // Project tabs
          ...openTabs.map((tabId) {
            final project =
                projects.where((p) => p.id == tabId).firstOrNull;
            return _Tab(
              label: project?.name ?? tabId,
              selected: selectedId == tabId,
              onTap: () => onSelect(tabId),
              onClose: () => onClose(tabId),
            );
          }),
        ],
      ),
    );
  }
}

class _Tab extends StatelessWidget {
  final String label;
  final IconData? icon;
  final bool selected;
  final VoidCallback onTap;
  final VoidCallback? onClose;

  const _Tab({
    required this.label,
    this.icon,
    required this.selected,
    required this.onTap,
    this.onClose,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 2),
      child: GestureDetector(
        onTap: onTap,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
          decoration: BoxDecoration(
            color: selected
                ? GolemTheme.accent.withValues(alpha: 0.15)
                : Colors.transparent,
            borderRadius: BorderRadius.circular(6),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (icon != null) ...[
                Icon(icon, size: 14,
                    color: selected
                        ? GolemTheme.accent
                        : GolemTheme.textSecondary),
                const SizedBox(width: 4),
              ],
              Text(
                label,
                style: TextStyle(
                  fontSize: 12,
                  color: selected
                      ? GolemTheme.accent
                      : GolemTheme.textSecondary,
                ),
              ),
              if (onClose != null) ...[
                const SizedBox(width: 4),
                GestureDetector(
                  onTap: onClose,
                  child: Icon(Icons.close, size: 12,
                      color: GolemTheme.textSecondary),
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}
```

- [ ] **Step 4: Verify app shell compiles**

Run: `cd ui/flutter && dart analyze lib/views/app_shell.dart`
Expected: no errors.

- [ ] **Step 5: Update main.dart to use AppShell**

In `lib/main.dart`, replace `ShellView` with `AppShell`:

```dart
import 'views/app_shell.dart';
// Remove: import 'views/shell.dart';
```

And change the `home:` parameter:

```dart
home: const AppShell(),
// Was: home: const ShellView(),
```

- [ ] **Step 6: Verify full app compiles**

Run: `cd ui/flutter && dart analyze lib/`
Expected: no errors (warnings about unused imports from old shell.dart are OK — we'll clean up next).

- [ ] **Step 7: Commit**

```bash
git add ui/flutter/lib/views/app_shell.dart ui/flutter/lib/views/status_bar.dart ui/flutter/lib/main.dart
git commit -m "feat(ui): new app shell with collapsible rail, tabs, and status bar"
```

---

## Chunk 3: Project Workspace

### Task 9: Create agent picker widget

**Files:**
- Create: `ui/flutter/lib/views/agent_picker.dart`

- [ ] **Step 1: Implement agent picker chips**

```dart
// lib/views/agent_picker.dart
import 'package:flutter/material.dart';

import '../theme.dart';

/// Horizontal chip row for selecting an agent.
class AgentPicker extends StatelessWidget {
  final List<String> agents;
  final String selected;
  final ValueChanged<String> onChanged;

  const AgentPicker({
    super.key,
    required this.agents,
    required this.selected,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: 6,
      children: agents.map((name) {
        final isSelected = name == selected;
        return ChoiceChip(
          label: Text(name, style: const TextStyle(fontSize: 11)),
          selected: isSelected,
          onSelected: (_) => onChanged(name),
          selectedColor: GolemTheme.green.withValues(alpha: 0.2),
          backgroundColor: GolemTheme.bgPrimary,
          side: BorderSide(
            color: isSelected ? GolemTheme.green : GolemTheme.border,
          ),
          padding: EdgeInsets.zero,
          labelPadding: const EdgeInsets.symmetric(horizontal: 8),
          visualDensity: VisualDensity.compact,
        );
      }).toList(),
    );
  }
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd ui/flutter && dart analyze lib/views/agent_picker.dart`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add ui/flutter/lib/views/agent_picker.dart
git commit -m "feat(ui): add agent picker chips widget"
```

---

### Task 10: Create command bar

**Files:**
- Create: `ui/flutter/lib/views/command_bar.dart`

- [ ] **Step 1: Implement command bar**

```dart
// lib/views/command_bar.dart
import 'package:flutter/material.dart';

import '../theme.dart';
import 'agent_picker.dart';

/// Top bar in project workspace: goal input + agent picker + Run button.
class CommandBar extends StatefulWidget {
  final List<String> agents;
  final void Function(String agent, String goal) onLaunch;

  const CommandBar({
    super.key,
    required this.agents,
    required this.onLaunch,
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
        widget.agents.isNotEmpty ? widget.agents.first : 'build-feature';
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
              ElevatedButton.icon(
                onPressed: _launch,
                icon: const Icon(Icons.play_arrow, size: 16),
                label: const Text('Run', style: TextStyle(fontSize: 12)),
                style: ElevatedButton.styleFrom(
                  backgroundColor: GolemTheme.green,
                  foregroundColor: Colors.white,
                  padding: const EdgeInsets.symmetric(
                      horizontal: 16, vertical: 10),
                  minimumSize: Size.zero,
                ),
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
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd ui/flutter && dart analyze lib/views/command_bar.dart`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add ui/flutter/lib/views/command_bar.dart
git commit -m "feat(ui): add command bar with goal input, agent picker, and Run button"
```

---

### Task 11: Create detail panel tabs

**Files:**
- Create: `ui/flutter/lib/views/detail_state.dart`
- Create: `ui/flutter/lib/views/detail_terminal.dart`
- Create: `ui/flutter/lib/views/detail_timeline.dart`
- Create: `ui/flutter/lib/views/detail_panel.dart`

- [ ] **Step 1: Create State tab**

```dart
// lib/views/detail_state.dart
import 'package:flutter/material.dart';

import '../models/graph.dart';
import '../models/project.dart';
import '../models/run.dart';
import '../theme.dart';

/// State tab in detail panel — shows current step, pipeline state KV, changes, project context.
class DetailState extends StatelessWidget {
  final RunInfo? run;
  final Map<String, dynamic>? pipelineState;
  final DiffSummary? diff;
  final List<Decision>? decisions;
  final List<Pitfall>? pitfalls;

  const DetailState({
    super.key,
    this.run,
    this.pipelineState,
    this.diff,
    this.decisions,
    this.pitfalls,
  });

  @override
  Widget build(BuildContext context) {
    if (run == null) {
      return const Center(
        child: Text(
          'Select a run to view state',
          style: TextStyle(fontSize: 13, color: GolemTheme.textSecondary),
        ),
      );
    }

    final currentStep =
        run!.steps.where((s) => s.status == 'running').firstOrNull;

    return SingleChildScrollView(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Current step
          if (currentStep != null) ...[
            const Text('Current Step',
                style: TextStyle(
                    fontSize: 12, fontWeight: FontWeight.w600)),
            const SizedBox(height: 4),
            Container(
              padding: const EdgeInsets.all(8),
              decoration: BoxDecoration(
                color: GolemTheme.bgElevated,
                borderRadius: BorderRadius.circular(4),
              ),
              child: Row(
                children: [
                  Container(
                    width: 6,
                    height: 6,
                    decoration: const BoxDecoration(
                      shape: BoxShape.circle,
                      color: GolemTheme.yellow,
                    ),
                  ),
                  const SizedBox(width: 8),
                  Text(currentStep.name,
                      style: const TextStyle(
                          fontSize: 12, fontWeight: FontWeight.w500)),
                  const SizedBox(width: 8),
                  Text('[${currentStep.type}]',
                      style: GolemTheme.metaStyle(fontSize: 10)),
                  if (currentStep.startedAt != null) ...[
                    const SizedBox(width: 8),
                    Text(
                      '${DateTime.now().difference(currentStep.startedAt!).inSeconds}s',
                      style: GolemTheme.metaStyle(fontSize: 10),
                    ),
                  ],
                  if (currentStep.toolCallCount > 0) ...[
                    const SizedBox(width: 8),
                    Text(
                      '${currentStep.toolCallCount} tools',
                      style: GolemTheme.metaStyle(fontSize: 10),
                    ),
                  ],
                ],
              ),
            ),
            const SizedBox(height: 16),
          ],
          // Pipeline state
          if (pipelineState != null && pipelineState!.isNotEmpty) ...[
            const Text('Pipeline State',
                style: TextStyle(
                    fontSize: 12, fontWeight: FontWeight.w600)),
            const SizedBox(height: 4),
            ...pipelineState!.entries.map((e) => Padding(
                  padding: const EdgeInsets.symmetric(vertical: 2),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      SizedBox(
                        width: 120,
                        child: Text(e.key,
                            style: GolemTheme.monoStyle(fontSize: 11)),
                      ),
                      Expanded(
                        child: Text(
                          '${e.value}',
                          style: const TextStyle(fontSize: 11),
                          maxLines: 3,
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                    ],
                  ),
                )),
            const SizedBox(height: 16),
          ],
          // Changes (diff stat)
          if (diff != null && diff!.files.isNotEmpty) ...[
            const Text('Changes',
                style: TextStyle(
                    fontSize: 12, fontWeight: FontWeight.w600)),
            const SizedBox(height: 4),
            Row(
              children: [
                Text('${diff!.files.length} file${diff!.files.length != 1 ? "s" : ""}',
                    style: const TextStyle(fontSize: 11)),
                const SizedBox(width: 8),
                if (diff!.totalAdded > 0)
                  Text('+${diff!.totalAdded}',
                      style: const TextStyle(fontSize: 11, color: GolemTheme.green)),
                if (diff!.totalAdded > 0 && diff!.totalRemoved > 0)
                  const SizedBox(width: 4),
                if (diff!.totalRemoved > 0)
                  Text('-${diff!.totalRemoved}',
                      style: const TextStyle(fontSize: 11, color: GolemTheme.red)),
              ],
            ),
            const SizedBox(height: 4),
            ...diff!.files.map((f) => Padding(
                  padding: const EdgeInsets.symmetric(vertical: 1),
                  child: Text(f.path,
                      style: GolemTheme.monoStyle(fontSize: 10)),
                )),
            const SizedBox(height: 16),
          ],
          // Project context (decisions + pitfalls)
          if ((decisions != null && decisions!.isNotEmpty) ||
              (pitfalls != null && pitfalls!.isNotEmpty)) ...[
            const Text('Project Context',
                style: TextStyle(
                    fontSize: 12, fontWeight: FontWeight.w600)),
            const SizedBox(height: 4),
            if (decisions != null && decisions!.isNotEmpty)
              Text('${decisions!.length} decision${decisions!.length != 1 ? "s" : ""}',
                  style: GolemTheme.metaStyle(fontSize: 11)),
            if (pitfalls != null && pitfalls!.isNotEmpty)
              Text('${pitfalls!.length} pitfall${pitfalls!.length != 1 ? "s" : ""}',
                  style: GolemTheme.metaStyle(fontSize: 11)),
          ],
        ],
      ),
    );
  }
}
```

- [ ] **Step 2: Create Terminal tab**

```dart
// lib/views/detail_terminal.dart
import 'package:flutter/material.dart';

import '../theme.dart';

/// Terminal tab in detail panel.
/// For running steps: read-only stream output.
/// For interactive sessions (golem plan): full xterm.
class DetailTerminal extends StatelessWidget {
  final String? processId;

  const DetailTerminal({super.key, this.processId});

  @override
  Widget build(BuildContext context) {
    if (processId == null) {
      return const Center(
        child: Text(
          'No active session',
          style: TextStyle(fontSize: 13, color: GolemTheme.textSecondary),
        ),
      );
    }

    // TODO: Wire xterm.dart terminal using existing ProcessTerminalNotifier
    // from providers/processes.dart. For now, show placeholder.
    return Container(
      color: GolemTheme.bgPrimary,
      padding: const EdgeInsets.all(16),
      child: Text(
        'Terminal attached to process $processId',
        style: GolemTheme.monoStyle(fontSize: 12),
      ),
    );
  }
}
```

- [ ] **Step 3: Create Timeline tab**

```dart
// lib/views/detail_timeline.dart
import 'package:flutter/material.dart';

import '../models/run.dart';
import '../theme.dart';

/// Timeline tab — event log rendered step-by-step.
class DetailTimeline extends StatelessWidget {
  final List<EngineEvent> events;

  const DetailTimeline({super.key, required this.events});

  @override
  Widget build(BuildContext context) {
    if (events.isEmpty) {
      return const Center(
        child: Text(
          'No events yet',
          style: TextStyle(fontSize: 13, color: GolemTheme.textSecondary),
        ),
      );
    }

    return ListView.builder(
      padding: const EdgeInsets.all(16),
      itemCount: events.length,
      itemBuilder: (context, index) {
        final ev = events[index];
        return _EventRow(event: ev);
      },
    );
  }
}

class _EventRow extends StatelessWidget {
  final EngineEvent event;
  const _EventRow({required this.event});

  IconData _icon() {
    return switch (event.type) {
      'pipeline-start' => Icons.play_arrow,
      'pipeline-end' => Icons.stop,
      'step-start' => Icons.arrow_forward,
      'step-end' => Icons.check_circle_outline,
      'loop-enter' => Icons.loop,
      'loop-exit' => Icons.exit_to_app,
      'conditional-skip' => Icons.skip_next,
      'error-retry' => Icons.replay,
      _ => Icons.info_outline,
    };
  }

  Color _color() {
    if (event.status == 'error') return GolemTheme.red;
    if (event.status == 'success') return GolemTheme.green;
    if (event.type.contains('loop')) return GolemTheme.purple;
    if (event.type.contains('error')) return GolemTheme.red;
    return GolemTheme.textSecondary;
  }

  String _label() {
    return switch (event.type) {
      'pipeline-start' =>
        'Pipeline started: ${event.agent} — ${event.goal}',
      'pipeline-end' =>
        'Pipeline ${event.status} (${_formatMs(event.durationMs)})',
      'step-start' => '[${event.stepType}] ${event.step} starting...',
      'step-end' =>
        '[${event.stepType}] ${event.step} ${event.status} (${_formatMs(event.durationMs)})',
      'loop-enter' =>
        'Loop ${event.predicate} iteration ${event.iteration}/${event.max}',
      'loop-exit' =>
        'Loop ${event.predicate} exited (${event.reason})',
      'conditional-skip' =>
        'Skipped (${event.predicate} = false)',
      'error-retry' =>
        '${event.errorType} ${event.step} attempt ${event.attempt} (${event.action})',
      _ => event.type,
    };
  }

  String _formatMs(int? ms) {
    if (ms == null) return '?';
    if (ms < 1000) return '${ms}ms';
    return '${(ms / 1000).toStringAsFixed(1)}s';
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(_icon(), size: 14, color: _color()),
          const SizedBox(width: 8),
          Text(
            '${event.timestamp.hour.toString().padLeft(2, '0')}:${event.timestamp.minute.toString().padLeft(2, '0')}:${event.timestamp.second.toString().padLeft(2, '0')}',
            style: GolemTheme.monoStyle(fontSize: 10),
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              _label(),
              style: const TextStyle(fontSize: 12),
            ),
          ),
        ],
      ),
    );
  }
}
```

- [ ] **Step 4: Create detail panel with tabs**

```dart
// lib/views/detail_panel.dart
import 'package:flutter/material.dart';

import '../models/run.dart';
import '../theme.dart';
import 'detail_state.dart';
import 'detail_terminal.dart';
import 'detail_timeline.dart';

/// Right-side detail panel with State/Terminal/Timeline tabs.
class DetailPanel extends StatefulWidget {
  final RunInfo? selectedRun;
  final Map<String, dynamic>? pipelineState;
  final List<EngineEvent> events;
  final String? processId;

  const DetailPanel({
    super.key,
    this.selectedRun,
    this.pipelineState,
    this.events = const [],
    this.processId,
  });

  @override
  State<DetailPanel> createState() => _DetailPanelState();
}

class _DetailPanelState extends State<DetailPanel>
    with SingleTickerProviderStateMixin {
  late final TabController _tabController;

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 3, vsync: this);
  }

  @override
  void dispose() {
    _tabController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Container(
          decoration: const BoxDecoration(
            border: Border(bottom: BorderSide(color: GolemTheme.border)),
          ),
          child: TabBar(
            controller: _tabController,
            tabs: const [
              Tab(text: 'State'),
              Tab(text: 'Terminal'),
              Tab(text: 'Timeline'),
            ],
            labelStyle: const TextStyle(fontSize: 12),
            labelColor: GolemTheme.accent,
            unselectedLabelColor: GolemTheme.textSecondary,
            indicatorColor: GolemTheme.accent,
            indicatorSize: TabBarIndicatorSize.label,
          ),
        ),
        Expanded(
          child: TabBarView(
            controller: _tabController,
            children: [
              DetailState(
                run: widget.selectedRun,
                pipelineState: widget.pipelineState,
              ),
              DetailTerminal(processId: widget.processId),
              DetailTimeline(events: widget.events),
            ],
          ),
        ),
      ],
    );
  }
}
```

- [ ] **Step 5: Verify all detail panel files compile**

Run: `cd ui/flutter && dart analyze lib/views/detail_state.dart lib/views/detail_terminal.dart lib/views/detail_timeline.dart lib/views/detail_panel.dart`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add ui/flutter/lib/views/detail_state.dart ui/flutter/lib/views/detail_terminal.dart ui/flutter/lib/views/detail_timeline.dart ui/flutter/lib/views/detail_panel.dart
git commit -m "feat(ui): add detail panel with State, Terminal, and Timeline tabs"
```

---

### Task 12: Create run feed

**Files:**
- Create: `ui/flutter/lib/views/run_feed.dart`

- [ ] **Step 1: Implement run feed with active/recent sections**

```dart
// lib/views/run_feed.dart
import 'package:flutter/material.dart';

import '../models/run.dart';
import '../theme.dart';
import 'run_card.dart';

/// Left panel in project workspace: active + recent runs.
class RunFeed extends StatelessWidget {
  final List<RunInfo> runs;
  final String? selectedRunId;
  final ValueChanged<RunInfo> onSelect;

  const RunFeed({
    super.key,
    required this.runs,
    this.selectedRunId,
    required this.onSelect,
  });

  @override
  Widget build(BuildContext context) {
    final active = runs.where((r) => r.status == 'running').toList();
    final recent =
        runs.where((r) => r.status != 'running').toList();

    return SingleChildScrollView(
      padding: const EdgeInsets.all(12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (active.isNotEmpty) ...[
            const Text(
              'ACTIVE',
              style: TextStyle(
                fontSize: 10,
                fontWeight: FontWeight.w700,
                color: GolemTheme.textSecondary,
                letterSpacing: 1.2,
              ),
            ),
            const SizedBox(height: 4),
            ...active.map((r) => RunCard(
                  run: r,
                  onTap: () => onSelect(r),
                )),
            const SizedBox(height: 16),
          ],
          if (recent.isNotEmpty) ...[
            const Text(
              'RECENT',
              style: TextStyle(
                fontSize: 10,
                fontWeight: FontWeight.w700,
                color: GolemTheme.textSecondary,
                letterSpacing: 1.2,
              ),
            ),
            const SizedBox(height: 4),
            ...recent.map((r) => RunCard(
                  run: r,
                  onTap: () => onSelect(r),
                )),
          ],
          if (runs.isEmpty)
            const Padding(
              padding: EdgeInsets.only(top: 24),
              child: Center(
                child: Text(
                  'No runs yet',
                  style: TextStyle(
                    fontSize: 13,
                    color: GolemTheme.textSecondary,
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd ui/flutter && dart analyze lib/views/run_feed.dart`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add ui/flutter/lib/views/run_feed.dart
git commit -m "feat(ui): add run feed with active/recent sections"
```

---

### Task 13: Create project workspace

**Files:**
- Create: `ui/flutter/lib/views/project_workspace.dart`

- [ ] **Step 1: Implement three-zone project workspace**

```dart
// lib/views/project_workspace.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/process.dart';
import '../models/run.dart';
import '../providers/connection.dart';
import '../providers/processes.dart';
import '../providers/project.dart';
import '../providers/runs.dart';
import '../theme.dart';
import 'command_bar.dart';
import 'detail_panel.dart';
import 'run_feed.dart';

/// Project workspace: command bar (top) + run feed (left) + detail panel (right).
class ProjectWorkspace extends ConsumerStatefulWidget {
  final String projectId;

  const ProjectWorkspace({super.key, required this.projectId});

  @override
  ConsumerState<ProjectWorkspace> createState() => _ProjectWorkspaceState();
}

class _ProjectWorkspaceState extends ConsumerState<ProjectWorkspace> {
  String? _selectedRunId;

  // Built-in agents + custom agents from .ctx/agents/.
  // TODO: Fetch via `golem agents --json` API endpoint when available.
  // For now, hardcode built-ins — custom agent discovery is future work
  // that requires adding an /agents endpoint to golem serve.
  static const _defaultAgents = ['build-feature', 'one-shot', 'fix-bug'];

  Future<void> _launchRun(String agent, String goal) async {
    try {
      final api = ref.read(apiClientProvider);
      final projectInfo = ref.read(projectInfoProvider);
      if (projectInfo == null) return;
      await api.launchProcess(
        projectInfo.id,
        LaunchRequest(
          command: 'run $agent',
          config: LaunchConfig(),
          agentName: agent,
          goal: goal,
        ),
      );
      ref.read(processesProvider.notifier).refresh();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Failed to launch: $e')),
        );
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final allRuns = ref.watch(runsProvider);
    final projectRuns =
        allRuns.where((r) => r.projectId == widget.projectId).toList();
    final selectedRun = _selectedRunId != null
        ? projectRuns.where((r) => r.runId == _selectedRunId).firstOrNull
        : projectRuns.isNotEmpty
            ? projectRuns.first
            : null;

    return Column(
      children: [
        // Command bar
        CommandBar(
          agents: _defaultAgents,
          onLaunch: _launchRun,
        ),
        // Run feed + detail panel
        Expanded(
          child: Row(
            children: [
              // Run feed (left, ~55% of workspace)
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
                    onSelect: (run) =>
                        setState(() => _selectedRunId = run.runId),
                  ),
                ),
              ),
              // Detail panel (right, ~45% of workspace)
              Expanded(
                flex: 45,
                child: DetailPanel(
                  selectedRun: selectedRun,
                  events: const [], // TODO: wire from event stream
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

- [ ] **Step 2: Verify it compiles**

Run: `cd ui/flutter && dart analyze lib/views/project_workspace.dart`
Expected: no errors.

- [ ] **Step 3: Wire into AppShell**

In `lib/views/app_shell.dart`, replace the placeholder `Center` widget with `ProjectWorkspace`:

Add import:
```dart
import 'project_workspace.dart';
```

Replace the TODO line:
```dart
// Was:
//   : Center(child: Text('Project workspace: $selectedId', ...)),
// Now:
                            : ProjectWorkspace(
                                projectId: selectedId!,
                              ),
```

- [ ] **Step 4: Verify full app compiles**

Run: `cd ui/flutter && dart analyze lib/`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add ui/flutter/lib/views/project_workspace.dart ui/flutter/lib/views/app_shell.dart
git commit -m "feat(ui): add project workspace with command bar, run feed, and detail panel"
```

---

## Chunk 4: Cleanup and Final Verification

### Task 14: Remove old views

**Files:**
- Remove: `ui/flutter/lib/views/shell.dart`
- Remove: `ui/flutter/lib/views/dashboard.dart`

- [ ] **Step 1: Check for remaining imports of old views**

Run: `cd ui/flutter && grep -r 'shell.dart\|dashboard.dart' lib/ --include='*.dart'`

Remove any remaining imports. The key imports to remove:
- `main.dart` should already import `app_shell.dart` instead of `shell.dart`
- `app_shell.dart` should not import `dashboard.dart` (it uses `activity_feed.dart`)

- [ ] **Step 2: Delete old files**

```bash
rm ui/flutter/lib/views/shell.dart ui/flutter/lib/views/dashboard.dart
```

- [ ] **Step 3: Verify app still compiles**

Run: `cd ui/flutter && dart analyze lib/`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add -A ui/flutter/lib/views/
git commit -m "refactor(ui): remove old ShellView and DashboardView, replaced by AppShell"
```

---

### Task 15: Final verification

- [ ] **Step 1: Full analysis**

Run: `cd ui/flutter && dart analyze lib/`
Expected: no errors.

- [ ] **Step 2: Verify build (if Flutter SDK available)**

Run: `cd ui/flutter && flutter build linux --debug 2>&1 | tail -5` (or `flutter build web`)
Expected: build succeeds.

- [ ] **Step 3: Review diff**

Run: `git diff main --stat -- ui/flutter/`

Verify changes are scoped to:
- New files: `models/run.dart`, `providers/runs.dart`, `providers/projects.dart`, `views/app_shell.dart`, `views/status_bar.dart`, `views/activity_feed.dart`, `views/run_card.dart`, `views/pipeline_progress.dart`, `views/agent_picker.dart`, `views/command_bar.dart`, `views/project_workspace.dart`, `views/run_feed.dart`, `views/detail_panel.dart`, `views/detail_state.dart`, `views/detail_terminal.dart`, `views/detail_timeline.dart`
- Modified: `models/process.dart`, `main.dart`
- Deleted: `views/shell.dart`, `views/dashboard.dart`
