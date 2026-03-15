import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/project.dart';
import '../models/run.dart';
import '../providers/projects.dart';
import '../providers/runs.dart';
import '../theme.dart';
import 'activity_feed.dart';
import 'graph_explorer.dart';
import 'project_workspace.dart';
import 'settings_dialog.dart';
import 'status_bar.dart';

class AppShell extends ConsumerStatefulWidget {
  const AppShell({super.key});

  @override
  ConsumerState<AppShell> createState() => _AppShellState();
}

class _AppShellState extends ConsumerState<AppShell> {
  bool _railExpanded = false;

  Color _projectStatusColor(String projectId) {
    final runs = ref.read(runsProvider);
    final projectRuns = runs.where((r) => r.projectId == projectId);
    if (projectRuns.any((r) => r.status == 'error')) return GolemTheme.red;
    if (projectRuns.any((r) => r.status == 'running')) return GolemTheme.yellow;
    return GolemTheme.green;
  }

  void _openProject(String projectId) {
    final openTabs = ref.read(openProjectTabsProvider);
    ref.read(openProjectTabsProvider.notifier).state = {...openTabs, projectId};
    ref.read(selectedProjectIdProvider.notifier).state = projectId;
  }

  void _closeProject(String projectId) {
    final openTabs = ref.read(openProjectTabsProvider);
    final newTabs = {...openTabs}..remove(projectId);
    ref.read(openProjectTabsProvider.notifier).state = newTabs;
    final selected = ref.read(selectedProjectIdProvider);
    if (selected == projectId) {
      ref.read(selectedProjectIdProvider.notifier).state =
          newTabs.isEmpty ? null : newTabs.last;
    }
  }

  void _onRunTap(RunInfo run) {
    _openProject(run.projectId);
  }

  @override
  Widget build(BuildContext context) {
    final projects = ref.watch(projectListProvider);
    final selectedId = ref.watch(selectedProjectIdProvider);
    final openTabs = ref.watch(openProjectTabsProvider);

    return Scaffold(
      body: Column(
        children: [
          Expanded(
            child: Row(
              children: [
                // Left rail
                MouseRegion(
                  onEnter: (_) => setState(() => _railExpanded = true),
                  onExit: (_) => setState(() => _railExpanded = false),
                  child: AnimatedContainer(
                    duration: const Duration(milliseconds: 150),
                    width: _railExpanded ? 160 : 40,
                    decoration: const BoxDecoration(
                      color: GolemTheme.bgSurface,
                      border: Border(
                          right: BorderSide(color: GolemTheme.border)),
                    ),
                    child: Column(
                      children: [
                        _RailItem(
                          icon: Icons.bolt,
                          label: 'Activity',
                          expanded: _railExpanded,
                          selected: selectedId == null,
                          onTap: () => ref
                              .read(selectedProjectIdProvider.notifier)
                              .state = null,
                        ),
                        const Divider(
                            height: 1, color: GolemTheme.border),
                        Expanded(
                          child: ListView(
                            children: projects.map((p) {
                              return _RailItem(
                                letter: p.name.isNotEmpty
                                    ? p.name[0].toUpperCase()
                                    : '?',
                                label: p.name,
                                expanded: _railExpanded,
                                selected: selectedId == p.id,
                                statusColor: _projectStatusColor(p.id),
                                onTap: () => _openProject(p.id),
                              );
                            }).toList(),
                          ),
                        ),
                        const Divider(
                            height: 1, color: GolemTheme.border),
                        _RailItem(
                          icon: Icons.account_tree,
                          label: 'Graph',
                          expanded: _railExpanded,
                          selected: false,
                          onTap: () => showDialog(
                            context: context,
                            builder: (_) => const GraphExplorer(),
                          ),
                        ),
                        _RailItem(
                          icon: Icons.settings,
                          label: 'Settings',
                          expanded: _railExpanded,
                          selected: false,
                          onTap: () => showDialog(
                            context: context,
                            builder: (_) => const SettingsDialog(),
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
                // Main content area
                Expanded(
                  child: Column(
                    children: [
                      if (openTabs.isNotEmpty)
                        _TabBar(
                          projects: projects,
                          openTabs: openTabs,
                          selectedId: selectedId,
                          onSelect: (id) => ref
                              .read(selectedProjectIdProvider.notifier)
                              .state = id,
                          onClose: _closeProject,
                          onActivity: () => ref
                              .read(selectedProjectIdProvider.notifier)
                              .state = null,
                        ),
                      Expanded(
                        child: selectedId == null
                            ? ActivityFeed(onRunTap: _onRunTap)
                            : ProjectWorkspace(
                                projectId: selectedId,
                              ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),
          const StatusBar(),
        ],
      ),
    );
  }
}

class _RailItem extends StatelessWidget {
  final IconData? icon;
  final String? letter;
  final String label;
  final bool expanded;
  final bool selected;
  final Color? statusColor;
  final VoidCallback onTap;

  const _RailItem({
    this.icon,
    this.letter,
    required this.label,
    required this.expanded,
    required this.selected,
    this.statusColor,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      child: Container(
        height: 40,
        padding: const EdgeInsets.symmetric(horizontal: 8),
        color: selected ? GolemTheme.accent.withValues(alpha: 0.1) : null,
        child: Row(
          children: [
            SizedBox(
              width: 24,
              child: Stack(
                children: [
                  Center(
                    child: icon != null
                        ? Icon(icon, size: 18,
                            color: selected
                                ? GolemTheme.accent
                                : GolemTheme.textSecondary)
                        : Container(
                            width: 24,
                            height: 24,
                            decoration: BoxDecoration(
                              color: GolemTheme.bgElevated,
                              borderRadius: BorderRadius.circular(4),
                            ),
                            alignment: Alignment.center,
                            child: Text(
                              letter ?? '?',
                              style: TextStyle(
                                fontSize: 12,
                                fontWeight: FontWeight.w600,
                                color: selected
                                    ? GolemTheme.accent
                                    : GolemTheme.textPrimary,
                              ),
                            ),
                          ),
                  ),
                  if (statusColor != null)
                    Positioned(
                      right: 0,
                      bottom: 6,
                      child: Container(
                        width: 6,
                        height: 6,
                        decoration: BoxDecoration(
                          shape: BoxShape.circle,
                          color: statusColor,
                        ),
                      ),
                    ),
                ],
              ),
            ),
            if (expanded) ...[
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  label,
                  style: TextStyle(
                    fontSize: 12,
                    color: selected
                        ? GolemTheme.accent
                        : GolemTheme.textPrimary,
                  ),
                  overflow: TextOverflow.ellipsis,
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _TabBar extends StatelessWidget {
  final List<ProjectInfo> projects;
  final Set<String> openTabs;
  final String? selectedId;
  final ValueChanged<String> onSelect;
  final ValueChanged<String> onClose;
  final VoidCallback onActivity;

  const _TabBar({
    required this.projects,
    required this.openTabs,
    required this.selectedId,
    required this.onSelect,
    required this.onClose,
    required this.onActivity,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 36,
      padding: const EdgeInsets.symmetric(horizontal: 8),
      decoration: const BoxDecoration(
        border: Border(bottom: BorderSide(color: GolemTheme.border)),
      ),
      child: Row(
        children: [
          _Tab(
            label: 'Activity',
            icon: Icons.bolt,
            selected: selectedId == null,
            onTap: onActivity,
          ),
          ...openTabs.map((tabId) {
            final project =
                projects.where((p) => p.id == tabId).firstOrNull;
            return _Tab(
              label: project?.name ?? tabId,
              selected: selectedId == tabId,
              onTap: () => onSelect(tabId),
              onClose: () => onClose(tabId),
            );
          }),
        ],
      ),
    );
  }
}

class _Tab extends StatelessWidget {
  final String label;
  final IconData? icon;
  final bool selected;
  final VoidCallback onTap;
  final VoidCallback? onClose;

  const _Tab({
    required this.label,
    this.icon,
    required this.selected,
    required this.onTap,
    this.onClose,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 2),
      child: GestureDetector(
        onTap: onTap,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
          decoration: BoxDecoration(
            color: selected
                ? GolemTheme.accent.withValues(alpha: 0.15)
                : Colors.transparent,
            borderRadius: BorderRadius.circular(6),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (icon != null) ...[
                Icon(icon, size: 14,
                    color: selected
                        ? GolemTheme.accent
                        : GolemTheme.textSecondary),
                const SizedBox(width: 4),
              ],
              Text(
                label,
                style: TextStyle(
                  fontSize: 12,
                  color: selected
                      ? GolemTheme.accent
                      : GolemTheme.textSecondary,
                ),
              ),
              if (onClose != null) ...[
                const SizedBox(width: 4),
                GestureDetector(
                  onTap: onClose,
                  child: const Icon(Icons.close, size: 12,
                      color: GolemTheme.textSecondary),
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}
