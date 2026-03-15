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
        Expanded(
          child: filtered.isEmpty
              ? Center(
                  child: Text(
                    _filter == 'all'
                        ? 'No runs yet. Launch an agent to get started.'
                        : 'No $_filter runs.',
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
