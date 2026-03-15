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
