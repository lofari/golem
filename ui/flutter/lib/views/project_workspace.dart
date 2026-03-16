import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../providers/connection.dart';
import '../providers/processes.dart';
import '../providers/runs.dart';
import '../models/process.dart';
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

  static const _defaultAgents = ['build-feature', 'one-shot', 'fix-bug'];

  Future<void> _launchRun(String agent, String goal) async {
    try {
      final api = ref.read(apiClientProvider);
      await api.launchProcess(
        widget.projectId,
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
