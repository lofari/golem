import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../providers/graph.dart';
import '../providers/project.dart';
import '../theme.dart';

class ProjectSwitcher extends ConsumerWidget {
  const ProjectSwitcher({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final current = ref.watch(projectInfoProvider);
    final projects = ref.watch(projectListProvider);

    return PopupMenuButton<String>(
      offset: const Offset(0, 36),
      color: GolemTheme.bgElevated,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(8),
        side: const BorderSide(color: GolemTheme.border),
      ),
      onSelected: (id) {
        if (id == '__add__') return;
        final project = projects.firstWhere((p) => p.id == id);
        ref.read(projectInfoProvider.notifier).set(project);
      },
      itemBuilder: (_) => [
        ...projects.map((p) => PopupMenuItem<String>(
              value: p.id,
              child: Row(
                children: [
                  if (p.id == current?.id)
                    const Icon(Icons.check, size: 14, color: GolemTheme.accent)
                  else
                    const SizedBox(width: 14),
                  const SizedBox(width: 8),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Text(
                          p.name.isNotEmpty ? p.name : p.path.split('/').last,
                          style: const TextStyle(fontSize: 13),
                        ),
                        Text(
                          p.path,
                          style: GolemTheme.metaStyle(fontSize: 10),
                          overflow: TextOverflow.ellipsis,
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(width: 8),
                  _PhaseBadge(phase: p.phase),
                ],
              ),
            )),
        const PopupMenuDivider(),
        const PopupMenuItem<String>(
          value: '__add__',
          child: Row(
            children: [
              SizedBox(width: 14),
              SizedBox(width: 8),
              Icon(Icons.add, size: 14, color: GolemTheme.textSecondary),
              SizedBox(width: 6),
              Text('Add project...',
                  style: TextStyle(
                      fontSize: 13, color: GolemTheme.textSecondary)),
            ],
          ),
        ),
      ],
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            current?.name ?? 'Golem',
            style: const TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w600,
              color: GolemTheme.textPrimary,
            ),
          ),
          const SizedBox(width: 4),
          const Icon(Icons.expand_more,
              size: 16, color: GolemTheme.textSecondary),
        ],
      ),
    );
  }
}

class _PhaseBadge extends StatelessWidget {
  final String phase;
  const _PhaseBadge({required this.phase});

  @override
  Widget build(BuildContext context) {
    if (phase.isEmpty) return const SizedBox.shrink();
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: GolemTheme.phaseColor(phase).withValues(alpha: 0.15),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        phase,
        style: TextStyle(
          fontSize: 10,
          color: GolemTheme.phaseColor(phase),
          fontWeight: FontWeight.w500,
        ),
      ),
    );
  }
}
