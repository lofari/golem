import 'package:flutter/material.dart';

import '../models/graph.dart';
import '../models/project.dart';
import '../models/run.dart';
import '../theme.dart';

/// State tab in detail panel — shows current step, pipeline state KV, changes, project context.
class DetailState extends StatelessWidget {
  final RunInfo? run;
  final Map<String, dynamic>? pipelineState;
  final DiffSummary? diff;
  final List<Decision>? decisions;
  final List<Pitfall>? pitfalls;

  const DetailState({
    super.key,
    this.run,
    this.pipelineState,
    this.diff,
    this.decisions,
    this.pitfalls,
  });

  @override
  Widget build(BuildContext context) {
    if (run == null) {
      return const Center(
        child: Text(
          'Select a run to view state',
          style: TextStyle(fontSize: 13, color: GolemTheme.textSecondary),
        ),
      );
    }

    final currentStep =
        run!.steps.where((s) => s.status == 'running').firstOrNull;

    return SingleChildScrollView(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (currentStep != null) ...[
            const Text('Current Step',
                style: TextStyle(
                    fontSize: 12, fontWeight: FontWeight.w600)),
            const SizedBox(height: 4),
            Container(
              padding: const EdgeInsets.all(8),
              decoration: BoxDecoration(
                color: GolemTheme.bgElevated,
                borderRadius: BorderRadius.circular(4),
              ),
              child: Row(
                children: [
                  Container(
                    width: 6,
                    height: 6,
                    decoration: const BoxDecoration(
                      shape: BoxShape.circle,
                      color: GolemTheme.yellow,
                    ),
                  ),
                  const SizedBox(width: 8),
                  Text(currentStep.name,
                      style: const TextStyle(
                          fontSize: 12, fontWeight: FontWeight.w500)),
                  const SizedBox(width: 8),
                  Text('[${currentStep.type}]',
                      style: GolemTheme.metaStyle(fontSize: 10)),
                  if (currentStep.startedAt != null) ...[
                    const SizedBox(width: 8),
                    Text(
                      '${DateTime.now().difference(currentStep.startedAt!).inSeconds}s',
                      style: GolemTheme.metaStyle(fontSize: 10),
                    ),
                  ],
                  if (currentStep.toolCallCount > 0) ...[
                    const SizedBox(width: 8),
                    Text(
                      '${currentStep.toolCallCount} tools',
                      style: GolemTheme.metaStyle(fontSize: 10),
                    ),
                  ],
                ],
              ),
            ),
            const SizedBox(height: 16),
          ],
          if (pipelineState != null && pipelineState!.isNotEmpty) ...[
            const Text('Pipeline State',
                style: TextStyle(
                    fontSize: 12, fontWeight: FontWeight.w600)),
            const SizedBox(height: 4),
            ...pipelineState!.entries.map((e) => Padding(
                  padding: const EdgeInsets.symmetric(vertical: 2),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      SizedBox(
                        width: 120,
                        child: Text(e.key,
                            style: GolemTheme.monoStyle(fontSize: 11)),
                      ),
                      Expanded(
                        child: Text(
                          '${e.value}',
                          style: const TextStyle(fontSize: 11),
                          maxLines: 3,
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                    ],
                  ),
                )),
            const SizedBox(height: 16),
          ],
          if (diff != null && diff!.files.isNotEmpty) ...[
            const Text('Changes',
                style: TextStyle(
                    fontSize: 12, fontWeight: FontWeight.w600)),
            const SizedBox(height: 4),
            Row(
              children: [
                Text(
                    '${diff!.files.length} file${diff!.files.length != 1 ? "s" : ""}',
                    style: const TextStyle(fontSize: 11)),
                const SizedBox(width: 8),
                if (diff!.totalAdded > 0)
                  Text('+${diff!.totalAdded}',
                      style: const TextStyle(
                          fontSize: 11, color: GolemTheme.green)),
                if (diff!.totalAdded > 0 && diff!.totalRemoved > 0)
                  const SizedBox(width: 4),
                if (diff!.totalRemoved > 0)
                  Text('-${diff!.totalRemoved}',
                      style: const TextStyle(
                          fontSize: 11, color: GolemTheme.red)),
              ],
            ),
            const SizedBox(height: 4),
            ...diff!.files.map((f) => Padding(
                  padding: const EdgeInsets.symmetric(vertical: 1),
                  child: Text(f.path,
                      style: GolemTheme.monoStyle(fontSize: 10)),
                )),
            const SizedBox(height: 16),
          ],
          if ((decisions != null && decisions!.isNotEmpty) ||
              (pitfalls != null && pitfalls!.isNotEmpty)) ...[
            const Text('Project Context',
                style: TextStyle(
                    fontSize: 12, fontWeight: FontWeight.w600)),
            const SizedBox(height: 4),
            if (decisions != null && decisions!.isNotEmpty)
              Text(
                  '${decisions!.length} decision${decisions!.length != 1 ? "s" : ""}',
                  style: GolemTheme.metaStyle(fontSize: 11)),
            if (pitfalls != null && pitfalls!.isNotEmpty)
              Text(
                  '${pitfalls!.length} pitfall${pitfalls!.length != 1 ? "s" : ""}',
                  style: GolemTheme.metaStyle(fontSize: 11)),
          ],
        ],
      ),
    );
  }
}
