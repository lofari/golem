import 'package:flutter/material.dart';

import '../models/run.dart';
import '../theme.dart';

/// Timeline tab — event log rendered step-by-step.
class DetailTimeline extends StatelessWidget {
  final List<EngineEvent> events;

  const DetailTimeline({super.key, required this.events});

  @override
  Widget build(BuildContext context) {
    if (events.isEmpty) {
      return const Center(
        child: Text(
          'No events yet',
          style: TextStyle(fontSize: 13, color: GolemTheme.textSecondary),
        ),
      );
    }

    return ListView.builder(
      padding: const EdgeInsets.all(16),
      itemCount: events.length,
      itemBuilder: (context, index) {
        final ev = events[index];
        return _EventRow(event: ev);
      },
    );
  }
}

class _EventRow extends StatelessWidget {
  final EngineEvent event;
  const _EventRow({required this.event});

  IconData _icon() {
    return switch (event.type) {
      'pipeline-start' => Icons.play_arrow,
      'pipeline-end' => Icons.stop,
      'step-start' => Icons.arrow_forward,
      'step-end' => Icons.check_circle_outline,
      'loop-enter' => Icons.loop,
      'loop-exit' => Icons.exit_to_app,
      'conditional-skip' => Icons.skip_next,
      'error-retry' => Icons.replay,
      _ => Icons.info_outline,
    };
  }

  Color _color() {
    if (event.status == 'error') return GolemTheme.red;
    if (event.status == 'success') return GolemTheme.green;
    if (event.type.contains('loop')) return GolemTheme.purple;
    if (event.type.contains('error')) return GolemTheme.red;
    return GolemTheme.textSecondary;
  }

  String _label() {
    return switch (event.type) {
      'pipeline-start' =>
        'Pipeline started: ${event.agent} — ${event.goal}',
      'pipeline-end' =>
        'Pipeline ${event.status} (${_formatMs(event.durationMs)})',
      'step-start' => '[${event.stepType}] ${event.step} starting...',
      'step-end' =>
        '[${event.stepType}] ${event.step} ${event.status} (${_formatMs(event.durationMs)})',
      'loop-enter' =>
        'Loop ${event.predicate} iteration ${event.iteration}/${event.max}',
      'loop-exit' =>
        'Loop ${event.predicate} exited (${event.reason})',
      'conditional-skip' =>
        'Skipped (${event.predicate} = false)',
      'error-retry' =>
        '${event.errorType} ${event.step} attempt ${event.attempt} (${event.action})',
      _ => event.type,
    };
  }

  String _formatMs(int? ms) {
    if (ms == null) return '?';
    if (ms < 1000) return '${ms}ms';
    return '${(ms / 1000).toStringAsFixed(1)}s';
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(_icon(), size: 14, color: _color()),
          const SizedBox(width: 8),
          Text(
            '${event.timestamp.hour.toString().padLeft(2, '0')}:${event.timestamp.minute.toString().padLeft(2, '0')}:${event.timestamp.second.toString().padLeft(2, '0')}',
            style: GolemTheme.monoStyle(fontSize: 10),
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              _label(),
              style: const TextStyle(fontSize: 12),
            ),
          ),
        ],
      ),
    );
  }
}
