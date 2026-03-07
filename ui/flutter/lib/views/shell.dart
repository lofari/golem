import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/process.dart';
import '../providers/connection.dart';
import '../providers/project.dart';
import '../providers/processes.dart';
import '../theme.dart';
import 'dashboard.dart';
import 'process_view.dart';
import 'launch_dialog.dart';
import 'settings_dialog.dart';

class ShellView extends ConsumerWidget {
  const ShellView({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final projectInfo = ref.watch(projectInfoProvider);
    final connected = ref.watch(connectionProvider);
    final processes = ref.watch(processesProvider);
    final selectedProcessId = ref.watch(selectedProcessIdProvider);

    return Scaffold(
      body: Column(
        children: [
          // Top bar
          _TopBar(
            projectName: projectInfo?.name ?? 'Golem',
            phase: projectInfo?.phase ?? '',
            onLaunch: () => _showLaunchDialog(context, ref),
            onPlan: () => _launchPlan(context, ref),
            onSettings: () => _showSettingsDialog(context, ref),
          ),
          // Process tabs (only if processes exist)
          if (processes.isNotEmpty)
            _ProcessTabs(
              processes: processes,
              selectedId: selectedProcessId,
              onSelect: (id) =>
                  ref.read(selectedProcessIdProvider.notifier).state = id,
              onDashboard: () =>
                  ref.read(selectedProcessIdProvider.notifier).state = null,
              showDashboardTab: true,
            ),
          // Content
          Expanded(
            child: selectedProcessId != null
                ? ProcessView(processId: selectedProcessId)
                : const DashboardView(),
          ),
          // Status bar
          _StatusBar(connected: connected, processCount: processes.length),
        ],
      ),
    );
  }

  void _showLaunchDialog(BuildContext context, WidgetRef ref) {
    showDialog(
      context: context,
      builder: (_) => const LaunchDialog(),
    );
  }

  Future<void> _launchPlan(BuildContext context, WidgetRef ref) async {
    final projectInfo = ref.read(projectInfoProvider);
    if (projectInfo == null) return;

    try {
      final api = ref.read(apiClientProvider);
      final id = await api.launchProcess(
        projectInfo.id,
        LaunchRequest(command: 'plan', config: LaunchConfig()),
      );
      ref.read(processesProvider.notifier).refresh();
      ref.read(selectedProcessIdProvider.notifier).state = id;
    } catch (e) {
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Failed to launch plan: $e')),
        );
      }
    }
  }

  void _showSettingsDialog(BuildContext context, WidgetRef ref) {
    showDialog(
      context: context,
      builder: (_) => const SettingsDialog(),
    );
  }
}

class _TopBar extends StatelessWidget {
  final String projectName;
  final String phase;
  final VoidCallback onLaunch;
  final VoidCallback onPlan;
  final VoidCallback onSettings;

  const _TopBar({
    required this.projectName,
    required this.phase,
    required this.onLaunch,
    required this.onPlan,
    required this.onSettings,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 48,
      padding: const EdgeInsets.symmetric(horizontal: 16),
      decoration: const BoxDecoration(
        color: GolemTheme.bgSurface,
        border: Border(bottom: BorderSide(color: GolemTheme.border)),
      ),
      child: Row(
        children: [
          Text(
            projectName,
            style: const TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w600,
              color: GolemTheme.textPrimary,
            ),
          ),
          if (phase.isNotEmpty) ...[
            const SizedBox(width: 8),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
              decoration: BoxDecoration(
                color: GolemTheme.bgElevated,
                borderRadius: BorderRadius.circular(4),
              ),
              child: Text(
                phase,
                style: const TextStyle(
                  fontSize: 11,
                  color: GolemTheme.textSecondary,
                ),
              ),
            ),
          ],
          const Spacer(),
          _ActionButton(
            icon: Icons.add,
            label: 'Launch',
            onPressed: onLaunch,
          ),
          const SizedBox(width: 4),
          _ActionButton(
            icon: Icons.play_arrow,
            label: 'Plan',
            onPressed: onPlan,
            color: GolemTheme.green,
          ),
          const SizedBox(width: 4),
          IconButton(
            icon: const Icon(Icons.settings, size: 18),
            color: GolemTheme.textSecondary,
            onPressed: onSettings,
            tooltip: 'Settings',
            splashRadius: 18,
          ),
        ],
      ),
    );
  }
}

class _ActionButton extends StatelessWidget {
  final IconData icon;
  final String label;
  final VoidCallback onPressed;
  final Color? color;

  const _ActionButton({
    required this.icon,
    required this.label,
    required this.onPressed,
    this.color,
  });

  @override
  Widget build(BuildContext context) {
    return TextButton.icon(
      onPressed: onPressed,
      icon: Icon(icon, size: 16, color: color ?? GolemTheme.textSecondary),
      label: Text(
        label,
        style: TextStyle(
          fontSize: 12,
          color: color ?? GolemTheme.textSecondary,
        ),
      ),
      style: TextButton.styleFrom(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
        minimumSize: Size.zero,
      ),
    );
  }
}

class _ProcessTabs extends StatelessWidget {
  final List<ProcessInfo> processes;
  final String? selectedId;
  final ValueChanged<String> onSelect;
  final VoidCallback onDashboard;
  final bool showDashboardTab;

  const _ProcessTabs({
    required this.processes,
    required this.selectedId,
    required this.onSelect,
    required this.onDashboard,
    required this.showDashboardTab,
  });

  Color _statusColor(String status) {
    switch (status) {
      case 'running':
        return GolemTheme.green;
      case 'failed':
        return GolemTheme.red;
      default:
        return GolemTheme.textSecondary;
    }
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 36,
      decoration: const BoxDecoration(
        border: Border(bottom: BorderSide(color: GolemTheme.border)),
      ),
      child: Row(
        children: [
          ...processes.map((p) {
            final isSelected = p.id == selectedId;
            return GestureDetector(
              onTap: () => onSelect(p.id),
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 12),
                decoration: BoxDecoration(
                  border: Border(
                    bottom: BorderSide(
                      color: isSelected ? GolemTheme.accent : Colors.transparent,
                      width: 2,
                    ),
                  ),
                ),
                child: Row(
                  children: [
                    Container(
                      width: 7,
                      height: 7,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        color: _statusColor(p.status),
                      ),
                    ),
                    const SizedBox(width: 6),
                    Text(
                      p.command,
                      style: TextStyle(
                        fontSize: 12,
                        color: isSelected
                            ? GolemTheme.textPrimary
                            : GolemTheme.textSecondary,
                      ),
                    ),
                  ],
                ),
              ),
            );
          }),
          if (showDashboardTab)
            GestureDetector(
              onTap: onDashboard,
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 12),
                decoration: BoxDecoration(
                  border: Border(
                    bottom: BorderSide(
                      color: selectedId == null
                          ? GolemTheme.accent
                          : Colors.transparent,
                      width: 2,
                    ),
                  ),
                ),
                child: Row(
                  children: [
                    Icon(
                      Icons.dashboard_outlined,
                      size: 14,
                      color: selectedId == null
                          ? GolemTheme.textPrimary
                          : GolemTheme.textSecondary,
                    ),
                    const SizedBox(width: 4),
                    Text(
                      'Dashboard',
                      style: TextStyle(
                        fontSize: 12,
                        color: selectedId == null
                            ? GolemTheme.textPrimary
                            : GolemTheme.textSecondary,
                      ),
                    ),
                  ],
                ),
              ),
            ),
        ],
      ),
    );
  }
}

class _StatusBar extends StatelessWidget {
  final bool connected;
  final int processCount;

  const _StatusBar({required this.connected, required this.processCount});

  @override
  Widget build(BuildContext context) {
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
            connected
                ? 'golem serve \u00B7 $processCount process${processCount != 1 ? "es" : ""}'
                : 'Disconnected \u2014 start golem serve',
            style: const TextStyle(
              fontSize: 11,
              color: GolemTheme.textSecondary,
            ),
          ),
        ],
      ),
    );
  }
}
