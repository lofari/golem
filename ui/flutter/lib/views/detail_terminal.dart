import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:xterm/xterm.dart';

import '../providers/processes.dart';
import '../theme.dart';

/// Terminal tab in detail panel.
/// For running steps: read-only stream output.
/// For interactive sessions (golem plan): full xterm.
class DetailTerminal extends ConsumerStatefulWidget {
  final String? processId;

  const DetailTerminal({super.key, this.processId});

  @override
  ConsumerState<DetailTerminal> createState() => _DetailTerminalState();
}

class _DetailTerminalState extends ConsumerState<DetailTerminal> {
  final _controller = TerminalController();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (widget.processId == null) {
      return const Center(
        child: Text(
          'No active session',
          style: TextStyle(fontSize: 13, color: GolemTheme.textSecondary),
        ),
      );
    }

    final terminal = ref.watch(processTerminalProvider(widget.processId!));

    return Container(
      color: GolemTheme.bgPrimary,
      child: LayoutBuilder(
        builder: (context, constraints) {
          WidgetsBinding.instance.addPostFrameCallback((_) {
            if (!mounted) return;
            final notifier = ref.read(
              processTerminalProvider(widget.processId!).notifier,
            );
            notifier.sendResize(terminal.viewWidth, terminal.viewHeight);
          });

          return TerminalView(
            terminal,
            controller: _controller,
            autofocus: false,
            hardwareKeyboardOnly: true,
          );
        },
      ),
    );
  }
}
