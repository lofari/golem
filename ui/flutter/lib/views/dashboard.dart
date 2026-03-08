import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/graph.dart';
import '../models/project.dart';
import '../providers/graph.dart';
import '../providers/project.dart';
import '../theme.dart';
import 'diff_viewer.dart';

class DashboardView extends ConsumerWidget {
  const DashboardView({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(projectStateProvider);
    final sessions = ref.watch(sessionsProvider);
    final graphStats = ref.watch(graphStatsProvider);

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
          constraints: const BoxConstraints(maxWidth: 960),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // Project header
              Text(
                state.project.name,
                style: const TextStyle(
                    fontSize: 20, fontWeight: FontWeight.w600),
              ),
              if (state.project.summary.isNotEmpty) ...[
                const SizedBox(height: 4),
                Text(
                  state.project.summary,
                  style: const TextStyle(
                      fontSize: 13, color: GolemTheme.textSecondary),
                ),
              ],
              const SizedBox(height: 4),
              Row(
                children: [
                  Text(
                    'Stack: ${state.project.stack}',
                    style: const TextStyle(
                        fontSize: 11, color: GolemTheme.textSecondary),
                  ),
                  const SizedBox(width: 16),
                  Container(
                    padding: const EdgeInsets.symmetric(
                        horizontal: 8, vertical: 2),
                    decoration: BoxDecoration(
                      color: GolemTheme.phaseColor(state.status.phase)
                          .withValues(alpha: 0.15),
                      borderRadius: BorderRadius.circular(4),
                    ),
                    child: Text(
                      state.status.phase,
                      style: TextStyle(
                        fontSize: 11,
                        color:
                            GolemTheme.phaseColor(state.status.phase),
                        fontWeight: FontWeight.w500,
                      ),
                    ),
                  ),
                ],
              ),

              const SizedBox(height: 24),

              // Three-card row
              LayoutBuilder(
                builder: (context, constraints) {
                  if (constraints.maxWidth < 600) {
                    // Stack vertically on narrow screens
                    return Column(
                      children: [
                        _TasksCard(
                            tasks: tasks, done: done),
                        const SizedBox(height: 12),
                        _GraphSummaryCard(stats: graphStats),
                        const SizedBox(height: 12),
                        if (recentSessions.isNotEmpty)
                          _RecentSessionsCard(
                              sessions: recentSessions),
                      ],
                    );
                  }
                  return Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Expanded(
                        flex: 2,
                        child: _TasksCard(
                            tasks: tasks, done: done),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child:
                            _GraphSummaryCard(stats: graphStats),
                      ),
                      if (recentSessions.isNotEmpty) ...[
                        const SizedBox(width: 12),
                        Expanded(
                          child: _RecentSessionsCard(
                              sessions: recentSessions),
                        ),
                      ],
                    ],
                  );
                },
              ),

              // Diff card (full width below the grid)
              const SizedBox(height: 12),
              const _DiffCard(),

              // Decisions & Pitfalls
              if (state.decisions.isNotEmpty) ...[
                const SizedBox(height: 24),
                _SectionHeader('Decisions',
                    trailing: '${state.decisions.length}'),
                const SizedBox(height: 8),
                ...state.decisions
                    .map((d) => _DecisionRow(decision: d)),
              ],

              if (state.pitfalls.isNotEmpty) ...[
                const SizedBox(height: 24),
                _SectionHeader('Pitfalls',
                    trailing: '${state.pitfalls.length}'),
                const SizedBox(height: 8),
                ...state.pitfalls
                    .map((p) => _PitfallRow(pitfall: p)),
              ],

              const SizedBox(height: 24),
            ],
          ),
        ),
      ),
    );
  }
}

class _TasksCard extends StatefulWidget {
  final List<Task> tasks;
  final int done;

  const _TasksCard({required this.tasks, required this.done});

  @override
  State<_TasksCard> createState() => _TasksCardState();
}

class _TasksCardState extends State<_TasksCard> {
  String _filter = 'all';

  List<Task> get _filteredTasks {
    return switch (_filter) {
      'active' => widget.tasks
          .where((t) => t.status == 'in-progress' || t.status == 'todo')
          .toList(),
      'done' => widget.tasks.where((t) => t.status == 'done').toList(),
      'blocked' =>
        widget.tasks.where((t) => t.status == 'blocked').toList(),
      _ => widget.tasks,
    };
  }

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                const Text('Tasks',
                    style: TextStyle(
                        fontSize: 13, fontWeight: FontWeight.w600)),
                const Spacer(),
                Text('${widget.done}/${widget.tasks.length}',
                    style: const TextStyle(
                        fontSize: 12,
                        color: GolemTheme.textSecondary)),
              ],
            ),
            const SizedBox(height: 8),
            ClipRRect(
              borderRadius: BorderRadius.circular(4),
              child: LinearProgressIndicator(
                value: widget.tasks.isEmpty
                    ? 0
                    : widget.done / widget.tasks.length,
                backgroundColor: GolemTheme.bgElevated,
                color: GolemTheme.green,
                minHeight: 6,
              ),
            ),
            const SizedBox(height: 8),
            Wrap(
              spacing: 4,
              children: ['all', 'active', 'done', 'blocked']
                  .map((f) => ChoiceChip(
                        label: Text(
                            f[0].toUpperCase() + f.substring(1),
                            style: const TextStyle(fontSize: 11)),
                        selected: _filter == f,
                        onSelected: (_) =>
                            setState(() => _filter = f),
                        selectedColor:
                            GolemTheme.accent.withValues(alpha: 0.2),
                        backgroundColor: GolemTheme.bgPrimary,
                        side: const BorderSide(
                            color: GolemTheme.border),
                        padding: EdgeInsets.zero,
                        labelPadding: const EdgeInsets.symmetric(
                            horizontal: 6),
                        visualDensity: VisualDensity.compact,
                      ))
                  .toList(),
            ),
            const SizedBox(height: 8),
            ..._filteredTasks.map((t) => _TaskRow(task: t)),
          ],
        ),
      ),
    );
  }
}

class _GraphSummaryCard extends StatelessWidget {
  final GraphStats? stats;
  const _GraphSummaryCard({this.stats});

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                const Icon(Icons.account_tree,
                    size: 14, color: GolemTheme.accent),
                const SizedBox(width: 6),
                const Text('Knowledge Graph',
                    style: TextStyle(
                        fontSize: 13, fontWeight: FontWeight.w600)),
              ],
            ),
            const SizedBox(height: 12),
            if (stats == null) ...[
              Text(
                'No graph yet',
                style: TextStyle(
                    fontSize: 12, color: GolemTheme.textSecondary),
              ),
              const SizedBox(height: 8),
              Container(
                padding: const EdgeInsets.all(8),
                decoration: BoxDecoration(
                  color: GolemTheme.bgPrimary,
                  borderRadius: BorderRadius.circular(4),
                ),
                child: Text(
                  'golem graph build',
                  style: GolemTheme.monoStyle(fontSize: 11),
                ),
              ),
            ] else ...[
              Text(
                '${stats!.totalNodes} nodes \u00B7 ${stats!.totalEdges} edges',
                style: const TextStyle(fontSize: 12),
              ),
              const SizedBox(height: 4),
              if (stats!.embeddingCount > 0) ...[
                Text(
                  '${stats!.embeddingCount} embeddings',
                  style: GolemTheme.metaStyle(fontSize: 10),
                ),
                if (stats!.embedModel.isNotEmpty)
                  Text(
                    stats!.embedModel,
                    style: GolemTheme.metaStyle(fontSize: 10),
                  ),
              ],
              if (stats!.lastIndexed.isNotEmpty) ...[
                const SizedBox(height: 4),
                Text(
                  'Indexed: ${stats!.lastIndexed}',
                  style: GolemTheme.metaStyle(fontSize: 10),
                ),
              ],
              if (stats!.commitCount > 0) ...[
                const SizedBox(height: 4),
                Text(
                  '${stats!.commitCount} commits \u00B7 ${stats!.authorCount} authors',
                  style: GolemTheme.metaStyle(fontSize: 10),
                ),
              ],
            ],
          ],
        ),
      ),
    );
  }
}

class _RecentSessionsCard extends StatelessWidget {
  final List<Session> sessions;
  const _RecentSessionsCard({required this.sessions});

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text('Recent Sessions',
                style: TextStyle(
                    fontSize: 13, fontWeight: FontWeight.w600)),
            const SizedBox(height: 8),
            ...sessions.map((s) => _CompactSessionRow(session: s)),
          ],
        ),
      ),
    );
  }
}

class _CompactSessionRow extends StatelessWidget {
  final Session session;
  const _CompactSessionRow({required this.session});

  @override
  Widget build(BuildContext context) {
    final outcomeColor = switch (session.outcome) {
      'done' => GolemTheme.green,
      'partial' => GolemTheme.yellow,
      _ => GolemTheme.red,
    };

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: Row(
        children: [
          Container(
            width: 6,
            height: 6,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: outcomeColor,
            ),
          ),
          const SizedBox(width: 6),
          Expanded(
            child: Text(
              '#${session.iteration} ${session.task}',
              style: const TextStyle(fontSize: 12),
              overflow: TextOverflow.ellipsis,
            ),
          ),
          if (session.filesChanged != null &&
              session.filesChanged!.isNotEmpty) ...[
            const SizedBox(width: 4),
            Container(
              padding: const EdgeInsets.symmetric(
                  horizontal: 4, vertical: 1),
              decoration: BoxDecoration(
                color: GolemTheme.bgElevated,
                borderRadius: BorderRadius.circular(3),
              ),
              child: Text(
                '${session.filesChanged!.length}',
                style: GolemTheme.metaStyle(fontSize: 9),
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class _DiffCard extends ConsumerWidget {
  const _DiffCard();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final diff = ref.watch(diffProvider);
    final fileCount = diff?.files.length ?? 0;
    final added = diff?.totalAdded ?? 0;
    final removed = diff?.totalRemoved ?? 0;

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                const Icon(Icons.difference_outlined,
                    size: 14, color: GolemTheme.accent),
                const SizedBox(width: 6),
                const Text('Changes',
                    style: TextStyle(
                        fontSize: 13, fontWeight: FontWeight.w600)),
                const Spacer(),
                if (fileCount > 0) ...[
                  Text(
                    '$fileCount ${fileCount == 1 ? 'file' : 'files'}',
                    style: const TextStyle(
                        fontSize: 12, color: GolemTheme.textSecondary),
                  ),
                  if (added > 0 || removed > 0) ...[
                    const SizedBox(width: 8),
                    if (added > 0)
                      Text('+$added',
                          style: const TextStyle(
                              fontSize: 12, color: GolemTheme.green)),
                    if (added > 0 && removed > 0)
                      const SizedBox(width: 4),
                    if (removed > 0)
                      Text('-$removed',
                          style: const TextStyle(
                              fontSize: 12, color: GolemTheme.red)),
                  ],
                ],
              ],
            ),
            const SizedBox(height: 8),
            const DiffViewer(),
          ],
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
          style: const TextStyle(
              fontSize: 13, fontWeight: FontWeight.w600),
        ),
        if (trailing != null) ...[
          const Spacer(),
          Text(
            trailing!,
            style: const TextStyle(
                fontSize: 12, color: GolemTheme.textSecondary),
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
            child:
                Text(icon, style: TextStyle(color: color, fontSize: 13)),
          ),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  task.name,
                  style: TextStyle(
                    fontSize: 13,
                    color: task.status == 'done'
                        ? GolemTheme.textSecondary
                        : GolemTheme.textPrimary,
                    decoration: task.status == 'done'
                        ? TextDecoration.lineThrough
                        : null,
                  ),
                ),
                if (task.notes != null && task.notes!.isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.only(top: 2),
                    child: Text(
                      task.notes!,
                      style: GolemTheme.metaStyle(fontSize: 10),
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ),
              ],
            ),
          ),
        ],
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
            style: const TextStyle(
                fontSize: 12, color: GolemTheme.textSecondary),
          ),
          TextSpan(
            text: decision.what,
            style: const TextStyle(fontSize: 12),
          ),
          TextSpan(
            text: ' \u2014 ${decision.why}',
            style: const TextStyle(
                fontSize: 12, color: GolemTheme.textSecondary),
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
          TextSpan(
              text: pitfall.what,
              style: const TextStyle(fontSize: 12)),
          if (pitfall.fix.isNotEmpty)
            TextSpan(
              text: ' \u2014 Fix: ${pitfall.fix}',
              style: const TextStyle(
                  fontSize: 12, color: GolemTheme.textSecondary),
            ),
        ]),
      ),
    );
  }
}
