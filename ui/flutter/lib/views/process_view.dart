import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:xterm/xterm.dart';
import '../models/project.dart' as models;
import '../providers/project.dart';
import '../providers/processes.dart';
import '../theme.dart';

class ProcessView extends ConsumerWidget {
  final String processId;
  const ProcessView({super.key, required this.processId});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Row(
      children: [
        Expanded(child: _TerminalPane(processId: processId)),
        const _TaskPanel(),
      ],
    );
  }
}

class _TerminalPane extends ConsumerStatefulWidget {
  final String processId;
  const _TerminalPane({required this.processId});

  @override
  ConsumerState<_TerminalPane> createState() => _TerminalPaneState();
}

class _TerminalPaneState extends ConsumerState<_TerminalPane> {
  @override
  Widget build(BuildContext context) {
    final terminal = ref.watch(processTerminalProvider(widget.processId));

    return Container(
      color: GolemTheme.bgPrimary,
      child: LayoutBuilder(
        builder: (context, constraints) {
          // Send resize when layout changes
          WidgetsBinding.instance.addPostFrameCallback((_) {
            final notifier = ref.read(
              processTerminalProvider(widget.processId).notifier,
            );
            notifier.sendResize(terminal.viewWidth, terminal.viewHeight);
          });

          return TerminalView(
            terminal,
            autofocus: true,
          );
        },
      ),
    );
  }
}

class _TaskPanel extends ConsumerWidget {
  const _TaskPanel();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(projectStateProvider);
    final sessions = ref.watch(sessionsProvider);

    if (state == null) return const SizedBox.shrink();

    final tasks = state.tasks;
    final done = tasks.where((t) => t.status == 'done').length;
    final lastSession = sessions.isNotEmpty ? sessions.last : null;

    return Container(
      width: 220,
      decoration: const BoxDecoration(
        color: GolemTheme.bgSurface,
        border: Border(left: BorderSide(color: GolemTheme.border)),
      ),
      child: Column(
        children: [
          // Header
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
            decoration: const BoxDecoration(
              border: Border(bottom: BorderSide(color: GolemTheme.border)),
            ),
            child: Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                const Text(
                  'TASKS',
                  style: TextStyle(
                    fontSize: 10,
                    fontWeight: FontWeight.w600,
                    letterSpacing: 1,
                    color: GolemTheme.textSecondary,
                  ),
                ),
                Text(
                  '$done/${tasks.length}',
                  style: const TextStyle(fontSize: 11, color: GolemTheme.textSecondary),
                ),
              ],
            ),
          ),
          // Task list
          Expanded(
            child: ListView.builder(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
              itemCount: tasks.length,
              itemBuilder: (context, i) => _TaskItem(task: tasks[i]),
            ),
          ),
          // Stats footer
          Container(
            padding: const EdgeInsets.all(12),
            decoration: const BoxDecoration(
              border: Border(top: BorderSide(color: GolemTheme.border)),
            ),
            child: Column(
              children: [
                _StatRow('Phase', state.status.phase.isNotEmpty ? state.status.phase : '\u2014'),
                _StatRow(
                  'Focus',
                  state.status.currentFocus.isNotEmpty
                      ? state.status.currentFocus
                      : '\u2014',
                ),
                if (lastSession != null) ...[
                  _StatRow('Last iter', '#${lastSession.iteration}'),
                  _StatRow(
                    'Outcome',
                    lastSession.outcome,
                    valueColor: switch (lastSession.outcome) {
                      'done' => GolemTheme.green,
                      'blocked' || 'unproductive' => GolemTheme.red,
                      _ => GolemTheme.yellow,
                    },
                  ),
                ],
                _StatRow('Decisions', '${state.decisions.length}'),
                _StatRow('Pitfalls', '${state.pitfalls.length}'),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _TaskItem extends StatelessWidget {
  final models.Task task;
  const _TaskItem({required this.task});

  @override
  Widget build(BuildContext context) {
    final (icon, color) = switch (task.status) {
      'done' => ('\u2713', GolemTheme.green),
      'in-progress' => ('\u25D0', GolemTheme.yellow),
      'blocked' => ('\u2717', GolemTheme.red),
      _ => ('\u25CB', GolemTheme.textSecondary),
    };

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 16,
            child: Text(icon, style: TextStyle(fontSize: 11, color: color, fontFamily: 'monospace')),
          ),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  task.name,
                  style: const TextStyle(fontSize: 12),
                  overflow: TextOverflow.ellipsis,
                ),
                if (task.blockedReason != null)
                  Text(
                    task.blockedReason!,
                    style: const TextStyle(fontSize: 10, color: GolemTheme.red),
                    overflow: TextOverflow.ellipsis,
                  ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _StatRow extends StatelessWidget {
  final String label;
  final String value;
  final Color? valueColor;

  const _StatRow(this.label, this.value, {this.valueColor});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 1),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Text(label, style: const TextStyle(fontSize: 11, color: GolemTheme.textSecondary)),
          Flexible(
            child: Text(
              value,
              style: TextStyle(fontSize: 11, color: valueColor ?? GolemTheme.textPrimary),
              overflow: TextOverflow.ellipsis,
              textAlign: TextAlign.end,
            ),
          ),
        ],
      ),
    );
  }
}
