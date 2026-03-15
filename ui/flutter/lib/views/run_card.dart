import 'package:flutter/material.dart';

import '../models/run.dart';
import '../theme.dart';
import 'pipeline_progress.dart';

/// Card for a single run in activity feed or project run feed.
class RunCard extends StatefulWidget {
  final RunInfo run;
  final VoidCallback? onTap;
  final bool showProjectBadge;

  const RunCard({
    super.key,
    required this.run,
    this.onTap,
    this.showProjectBadge = false,
  });

  @override
  State<RunCard> createState() => _RunCardState();
}

class _RunCardState extends State<RunCard>
    with SingleTickerProviderStateMixin {
  AnimationController? _pulseController;
  Animation<double>? _pulseAnimation;

  RunInfo get run => widget.run;

  @override
  void initState() {
    super.initState();
    if (run.status == 'running') {
      _pulseController = AnimationController(
        vsync: this,
        duration: const Duration(seconds: 1),
      )..repeat(reverse: true);
      _pulseAnimation = Tween<double>(begin: 0.3, end: 1.0).animate(
        CurvedAnimation(parent: _pulseController!, curve: Curves.easeInOut),
      );
    }
  }

  @override
  void dispose() {
    _pulseController?.dispose();
    super.dispose();
  }

  Color _statusColor() {
    return switch (run.status) {
      'running' => GolemTheme.yellow,
      'success' => GolemTheme.green,
      'error' => GolemTheme.red,
      _ => GolemTheme.textSecondary,
    };
  }

  String _relativeTime() {
    final diff = DateTime.now().difference(run.startedAt);
    if (diff.inSeconds < 60) return '${diff.inSeconds}s ago';
    if (diff.inMinutes < 60) return '${diff.inMinutes}m ago';
    if (diff.inHours < 24) return '${diff.inHours}h ago';
    return '${diff.inDays}d ago';
  }

  String _formatDuration(Duration d) {
    if (d.inSeconds < 60) return '${d.inSeconds}s';
    final mins = d.inMinutes;
    final secs = d.inSeconds % 60;
    return '${mins}m${secs.toString().padLeft(2, '0')}s';
  }

  Widget _buildStatusDot() {
    final dot = Container(
      width: 8,
      height: 8,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: _statusColor(),
      ),
    );
    if (run.status == 'running' && _pulseAnimation != null) {
      return FadeTransition(opacity: _pulseAnimation!, child: dot);
    }
    return dot;
  }

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: const EdgeInsets.symmetric(vertical: 4),
      child: InkWell(
        onTap: widget.onTap,
        borderRadius: BorderRadius.circular(8),
        child: Padding(
          padding: const EdgeInsets.all(12),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  _buildStatusDot(),
                  const SizedBox(width: 8),
                  if (widget.showProjectBadge &&
                      run.projectName.isNotEmpty) ...[
                    Container(
                      padding: const EdgeInsets.symmetric(
                          horizontal: 6, vertical: 2),
                      decoration: BoxDecoration(
                        color: GolemTheme.accent.withValues(alpha: 0.15),
                        borderRadius: BorderRadius.circular(4),
                      ),
                      child: Text(
                        run.projectName,
                        style: const TextStyle(
                          fontSize: 10,
                          fontWeight: FontWeight.w600,
                          color: GolemTheme.accent,
                        ),
                      ),
                    ),
                    const SizedBox(width: 8),
                  ],
                  Text(
                    run.agentName,
                    style: const TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                  const Spacer(),
                  Text(
                    run.status == 'running'
                        ? _relativeTime()
                        : run.duration != null
                            ? _formatDuration(run.duration!)
                            : _relativeTime(),
                    style: GolemTheme.metaStyle(fontSize: 11),
                  ),
                ],
              ),
              const SizedBox(height: 4),
              Text(
                run.goal,
                style: const TextStyle(fontSize: 12),
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
              ),
              if (run.status == 'running' && run.steps.isNotEmpty) ...[
                const SizedBox(height: 8),
                PipelineProgressBar(steps: run.steps),
                const SizedBox(height: 4),
                Wrap(
                  spacing: 8,
                  children: run.steps.map((s) {
                    return Text(
                      s.name,
                      style: TextStyle(
                        fontSize: 9,
                        color: s.status == 'running'
                            ? GolemTheme.yellow
                            : s.status == 'success'
                                ? GolemTheme.green
                                : GolemTheme.textSecondary,
                      ),
                    );
                  }).toList(),
                ),
              ],
              if (run.status == 'success' && run.prUrl != null) ...[
                const SizedBox(height: 4),
                Text(
                  run.prUrl!,
                  style: const TextStyle(
                    fontSize: 11,
                    color: GolemTheme.accent,
                    decoration: TextDecoration.underline,
                  ),
                ),
              ],
              if (run.status == 'error' && run.haltReason != null) ...[
                const SizedBox(height: 4),
                Text(
                  run.haltReason!,
                  style: const TextStyle(fontSize: 11, color: GolemTheme.red),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}
