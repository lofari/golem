import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../providers/connection.dart';
import '../providers/processes.dart';
import '../providers/project.dart';
import '../providers/runs.dart';
import '../models/process.dart';
import '../models/project.dart';
import '../theme.dart';
import 'command_bar.dart';
import 'detail_panel.dart';
import 'run_feed.dart';
import 'welcome_view.dart';

/// Project workspace: command bar (top) + run feed (left) + detail panel (right).
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
      setState(() => _skipWelcome = true);
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
    if (state == null || state.project.stack.isEmpty) return ProjectPhase.needsSetup;
    if (state.tasks.any((t) => t.status == 'todo')) return ProjectPhase.readyToBuild;
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

    // Check if a standalone process was launched via the LaunchDialog.
    final standaloneProcessId = ref.watch(selectedProcessIdProvider);

    // Find the process associated with the selected run, or use standalone.
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
                      // Clear standalone process when selecting a run
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
