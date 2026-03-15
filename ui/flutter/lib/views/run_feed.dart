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
