import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/process.dart';
import '../providers/connection.dart';
import '../providers/project.dart';
import '../providers/processes.dart';
import '../theme.dart';
import 'dashboard.dart';
import 'graph_explorer.dart';
import 'process_view.dart';
import 'launch_dialog.dart';
import 'project_switcher.dart';
import 'settings_dialog.dart';

class ShellView extends ConsumerWidget {
  const ShellView({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final connected = ref.watch(connectionProvider);
    final processes = ref.watch(processesProvider);
    final selectedProcessId = ref.watch(selectedProcessIdProvider);

    return Scaffold(
      body: Column(
        children: [
          // Top bar
          _TopBar(
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

class _TopBar extends ConsumerWidget {
  final VoidCallback onLaunch;
  final VoidCallback onPlan;
  final VoidCallback onSettings;

  const _TopBar({
    required this.onLaunch,
    required this.onPlan,
    required this.onSettings,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final projectState = ref.watch(projectStateProvider);
    final phase = projectState?.status.phase ?? '';

    return Container(
      height: 48,
      padding: const EdgeInsets.symmetric(horizontal: 16),
      decoration: const BoxDecoration(
        color: GolemTheme.bgSurface,
        border: Border(bottom: BorderSide(color: GolemTheme.border)),
      ),
      child: Row(
        children: [
          const ProjectSwitcher(),
          if (phase.isNotEmpty) ...[
            const SizedBox(width: 12),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
              decoration: BoxDecoration(
                color: GolemTheme.phaseColor(phase).withValues(alpha: 0.15),
                borderRadius: BorderRadius.circular(4),
              ),
              child: Text(
                phase,
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w600,
                  color: GolemTheme.phaseColor(phase),
                ),
              ),
            ),
          ],
          const Spacer(),
          IconButton(
            icon: const Icon(Icons.account_tree, size: 18),
            color: GolemTheme.textSecondary,
            onPressed: () => showDialog(
              context: context,
              builder: (_) => const GraphExplorer(),
            ),
            tooltip: 'Graph Explorer',
            splashRadius: 18,
          ),
          const SizedBox(width: 4),
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

class _ProcessTabs extends StatefulWidget {
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

  @override
  State<_ProcessTabs> createState() => _ProcessTabsState();
}

class _ProcessTabsState extends State<_ProcessTabs>
    with SingleTickerProviderStateMixin {
  late final AnimationController _pulseController;
  late final Animation<double> _pulseAnimation;

  @override
  void initState() {
    super.initState();
    _pulseController = AnimationController(
      vsync: this,
      duration: const Duration(seconds: 1),
    )..repeat(reverse: true);
    _pulseAnimation = Tween<double>(begin: 0.3, end: 1.0).animate(
      CurvedAnimation(parent: _pulseController, curve: Curves.easeInOut),
    );
  }

  @override
  void dispose() {
    _pulseController.dispose();
    super.dispose();
  }

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

  Widget _buildStatusDot(String status) {
    final dot = Container(
      width: 7,
      height: 7,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: _statusColor(status),
      ),
    );

    if (status == 'running') {
      return FadeTransition(
        opacity: _pulseAnimation,
        child: dot,
      );
    }
    return dot;
  }

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
          ...widget.processes.map((p) {
            final isSelected = p.id == widget.selectedId;
            return Padding(
              padding: const EdgeInsets.symmetric(horizontal: 2),
              child: GestureDetector(
                onTap: () => widget.onSelect(p.id),
                child: Container(
                  padding:
                      const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                  decoration: BoxDecoration(
                    color: isSelected
                        ? GolemTheme.accent.withValues(alpha: 0.15)
                        : Colors.transparent,
                    borderRadius: BorderRadius.circular(6),
                  ),
                  child: Row(
                    children: [
                      _buildStatusDot(p.status),
                      const SizedBox(width: 6),
                      Text(
                        p.command,
                        style: TextStyle(
                          fontSize: 12,
                          color: isSelected
                              ? GolemTheme.accent
                              : GolemTheme.textSecondary,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            );
          }),
          if (widget.showDashboardTab)
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 2),
              child: GestureDetector(
                onTap: widget.onDashboard,
                child: Container(
                  padding:
                      const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                  decoration: BoxDecoration(
                    color: widget.selectedId == null
                        ? GolemTheme.accent.withValues(alpha: 0.15)
                        : Colors.transparent,
                    borderRadius: BorderRadius.circular(6),
                  ),
                  child: Row(
                    children: [
                      Icon(
                        Icons.dashboard_outlined,
                        size: 14,
                        color: widget.selectedId == null
                            ? GolemTheme.accent
                            : GolemTheme.textSecondary,
                      ),
                      const SizedBox(width: 4),
                      Text(
                        'Dashboard',
                        style: TextStyle(
                          fontSize: 12,
                          color: widget.selectedId == null
                              ? GolemTheme.accent
                              : GolemTheme.textSecondary,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}

class _StatusBar extends ConsumerWidget {
  final bool connected;
  final int processCount;

  const _StatusBar({required this.connected, required this.processCount});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final projectState = ref.watch(projectStateProvider);
    final hasRunning = processCount > 0;
    final phase = projectState?.status.phase ?? '';
    final focus = projectState?.status.currentFocus ?? '';

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
          const Spacer(),
          if (hasRunning && phase.isNotEmpty)
            _buildIterationInfo(phase, focus),
        ],
      ),
    );
  }

  Widget _buildIterationInfo(String phase, String focus) {
    final phaseColor = GolemTheme.phaseColor(phase);
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          'Phase: ',
          style: TextStyle(
            fontSize: 11,
            color: GolemTheme.textSecondary,
          ),
        ),
        Text(
          phase,
          style: TextStyle(
            fontSize: 11,
            fontWeight: FontWeight.w600,
            color: phaseColor,
          ),
        ),
        if (focus.isNotEmpty) ...[
          Text(
            ' \u00B7 Focus: ',
            style: TextStyle(
              fontSize: 11,
              color: GolemTheme.textSecondary,
            ),
          ),
          Text(
            focus,
            style: const TextStyle(
              fontSize: 11,
              color: GolemTheme.textPrimary,
            ),
          ),
        ],
      ],
    );
  }
}
