import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/project.dart';
import '../providers/project.dart';
import '../theme.dart';

class DashboardView extends ConsumerWidget {
  const DashboardView({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(projectStateProvider);
    final sessions = ref.watch(sessionsProvider);

    if (state == null) {
      return const Center(
        child: CircularProgressIndicator(color: GolemTheme.accent),
      );
    }

    final tasks = state.tasks;
    final done = tasks.where((t) => t.status == 'done').length;
    final recentSessions = sessions.length > 5
        ? sessions.sublist(sessions.length - 5).reversed.toList()
        : sessions.reversed.toList();

    return SingleChildScrollView(
      padding: const EdgeInsets.all(24),
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 720),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // Project header
              Text(
                state.project.name,
                style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w600),
              ),
              if (state.project.summary.isNotEmpty) ...[
                const SizedBox(height: 4),
                Text(
                  state.project.summary,
                  style: const TextStyle(fontSize: 13, color: GolemTheme.textSecondary),
                ),
              ],
              const SizedBox(height: 4),
              Row(
                children: [
                  Text(
                    'Stack: ${state.project.stack}',
                    style: const TextStyle(fontSize: 11, color: GolemTheme.textSecondary),
                  ),
                  const SizedBox(width: 16),
                  Text(
                    'Phase: ${state.status.phase}',
                    style: const TextStyle(fontSize: 11, color: GolemTheme.textSecondary),
                  ),
                ],
              ),

              const SizedBox(height: 24),

              // Tasks
              _SectionHeader('Tasks', trailing: '$done/${tasks.length}'),
              const SizedBox(height: 8),
              ClipRRect(
                borderRadius: BorderRadius.circular(4),
                child: LinearProgressIndicator(
                  value: tasks.isEmpty ? 0 : done / tasks.length,
                  backgroundColor: GolemTheme.bgElevated,
                  color: GolemTheme.green,
                  minHeight: 6,
                ),
              ),
              const SizedBox(height: 12),
              ...tasks.map((t) => _TaskRow(task: t)),

              if (recentSessions.isNotEmpty) ...[
                const SizedBox(height: 24),
                _SectionHeader('Recent Sessions'),
                const SizedBox(height: 8),
                ...recentSessions.map((s) => _SessionCard(session: s)),
              ],

              if (state.decisions.isNotEmpty) ...[
                const SizedBox(height: 24),
                _SectionHeader('Decisions', trailing: '${state.decisions.length}'),
                const SizedBox(height: 8),
                ...state.decisions.map((d) => _DecisionRow(decision: d)),
              ],

              if (state.pitfalls.isNotEmpty) ...[
                const SizedBox(height: 24),
                _SectionHeader('Pitfalls', trailing: '${state.pitfalls.length}'),
                const SizedBox(height: 8),
                ...state.pitfalls.map((p) => _PitfallRow(pitfall: p)),
              ],

              const SizedBox(height: 24),
            ],
          ),
        ),
      ),
    );
  }
}

class _SectionHeader extends StatelessWidget {
  final String title;
  final String? trailing;

  const _SectionHeader(this.title, {this.trailing});

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Text(
          title,
          style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600),
        ),
        if (trailing != null) ...[
          const Spacer(),
          Text(
            trailing!,
            style: const TextStyle(fontSize: 12, color: GolemTheme.textSecondary),
          ),
        ],
      ],
    );
  }
}

class _TaskRow extends StatelessWidget {
  final Task task;
  const _TaskRow({required this.task});

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
            width: 20,
            child: Text(icon, style: TextStyle(color: color, fontSize: 13)),
          ),
          Expanded(
            child: Text(
              task.name,
              style: TextStyle(
                fontSize: 13,
                color: task.status == 'done' ? GolemTheme.textSecondary : GolemTheme.textPrimary,
                decoration: task.status == 'done' ? TextDecoration.lineThrough : null,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _SessionCard extends StatelessWidget {
  final Session session;
  const _SessionCard({required this.session});

  @override
  Widget build(BuildContext context) {
    final outcomeColor = switch (session.outcome) {
      'done' => GolemTheme.green,
      'partial' => GolemTheme.yellow,
      _ => GolemTheme.red,
    };

    return Card(
      margin: const EdgeInsets.only(bottom: 8),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                Expanded(
                  child: Text(
                    '#${session.iteration} \u2014 ${session.task}',
                    style: const TextStyle(fontSize: 13),
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
                Text(
                  session.outcome,
                  style: TextStyle(fontSize: 12, color: outcomeColor),
                ),
              ],
            ),
            if (session.summary.isNotEmpty) ...[
              const SizedBox(height: 4),
              Text(
                session.summary,
                style: const TextStyle(fontSize: 11, color: GolemTheme.textSecondary),
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _DecisionRow extends StatelessWidget {
  final Decision decision;
  const _DecisionRow({required this.decision});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: Text.rich(
        TextSpan(children: [
          TextSpan(
            text: '${decision.when} ',
            style: const TextStyle(fontSize: 12, color: GolemTheme.textSecondary),
          ),
          TextSpan(
            text: decision.what,
            style: const TextStyle(fontSize: 12),
          ),
          TextSpan(
            text: ' \u2014 ${decision.why}',
            style: const TextStyle(fontSize: 12, color: GolemTheme.textSecondary),
          ),
        ]),
      ),
    );
  }
}

class _PitfallRow extends StatelessWidget {
  final Pitfall pitfall;
  const _PitfallRow({required this.pitfall});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: Text.rich(
        TextSpan(children: [
          TextSpan(text: pitfall.what, style: const TextStyle(fontSize: 12)),
          if (pitfall.fix.isNotEmpty)
            TextSpan(
              text: ' \u2014 Fix: ${pitfall.fix}',
              style: const TextStyle(fontSize: 12, color: GolemTheme.textSecondary),
            ),
        ]),
      ),
    );
  }
}
