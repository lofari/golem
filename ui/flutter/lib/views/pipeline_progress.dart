import 'package:flutter/material.dart';

import '../models/run.dart';
import '../theme.dart';

/// Segmented pipeline progress bar. Each segment = one step.
/// Colors: green (success), yellow pulsing (running), red (error), gray (pending).
class PipelineProgressBar extends StatelessWidget {
  final List<StepProgress> steps;
  final double height;

  const PipelineProgressBar({
    super.key,
    required this.steps,
    this.height = 6,
  });

  Color _stepColor(String status) {
    return switch (status) {
      'success' => GolemTheme.green,
      'running' => GolemTheme.yellow,
      'error' => GolemTheme.red,
      'skipped' => GolemTheme.textSecondary,
      _ => GolemTheme.bgElevated, // pending
    };
  }

  @override
  Widget build(BuildContext context) {
    if (steps.isEmpty) return const SizedBox.shrink();

    return ClipRRect(
      borderRadius: BorderRadius.circular(3),
      child: SizedBox(
        height: height,
        child: Row(
          children: steps.map((s) {
            return Expanded(
              child: Container(
                margin: const EdgeInsets.symmetric(horizontal: 0.5),
                color: _stepColor(s.status),
              ),
            );
          }).toList(),
        ),
      ),
    );
  }
}
