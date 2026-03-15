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
