import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:xterm/xterm.dart';
import '../models/graph.dart';
import '../models/project.dart' as models;
import '../providers/graph.dart';
import '../providers/project.dart';
import '../providers/processes.dart';
import '../theme.dart';
import 'diff_viewer.dart';

class ProcessView extends ConsumerStatefulWidget {
  final String processId;
  const ProcessView({super.key, required this.processId});

  @override
  ConsumerState<ProcessView> createState() => _ProcessViewState();
}

class _ProcessViewState extends ConsumerState<ProcessView> {
  double _sidePanelWidth = 320;

  static const double _minWidth = 200;
  static const double _maxWidth = 500;

  @override
  Widget build(BuildContext context) {
    final processes = ref.watch(processesProvider);
    final processInfo = processes.where((p) => p.id == widget.processId).firstOrNull;
    final commandLabel = processInfo?.command ?? widget.processId;

    return Row(
      children: [
        // Terminal pane with header
        Expanded(
          child: Column(
            children: [
              // Terminal header bar
              Container(
                height: 32,
                padding: const EdgeInsets.symmetric(horizontal: 12),
                decoration: const BoxDecoration(
                  color: GolemTheme.bgElevated,
                  border: Border(
                    bottom: BorderSide(color: GolemTheme.border),
                  ),
                ),
                alignment: Alignment.centerLeft,
                child: Text(
                  commandLabel,
                  style: GolemTheme.monoStyle(fontSize: 11).copyWith(
                    color: GolemTheme.textSecondary,
                  ),
                  overflow: TextOverflow.ellipsis,
                ),
              ),
              // Terminal
              Expanded(child: _TerminalPane(processId: widget.processId)),
            ],
          ),
        ),
        // Draggable divider
        MouseRegion(
          cursor: SystemMouseCursors.resizeColumn,
          child: GestureDetector(
            onHorizontalDragUpdate: (details) {
              setState(() {
                _sidePanelWidth =
                    (_sidePanelWidth - details.delta.dx).clamp(_minWidth, _maxWidth);
              });
            },
            child: Container(
              width: 2,
              color: GolemTheme.border,
            ),
          ),
        ),
        // Side panel
        SizedBox(
          width: _sidePanelWidth,
          child: const _SidePanel(),
        ),
      ],
    );
  }
}

class _TerminalPane extends ConsumerStatefulWidget {
  final String processId;
  const _TerminalPane({required this.processId});

  @override
  ConsumerState<_TerminalPane> createState() => _TerminalPaneState();
}

class _TerminalPaneState extends ConsumerState<_TerminalPane> {
  final _controller = TerminalController();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final terminal = ref.watch(processTerminalProvider(widget.processId));

    return Container(
      color: GolemTheme.bgPrimary,
      child: LayoutBuilder(
        builder: (context, constraints) {
          // Send resize when layout changes
          WidgetsBinding.instance.addPostFrameCallback((_) {
            final notifier = ref.read(
              processTerminalProvider(widget.processId).notifier,
            );
            notifier.sendResize(terminal.viewWidth, terminal.viewHeight);
          });

          return TerminalView(
            terminal,
            controller: _controller,
            autofocus: true,
            hardwareKeyboardOnly: true,
          );
        },
      ),
    );
  }
}

class _SidePanel extends ConsumerStatefulWidget {
  const _SidePanel();

  @override
  ConsumerState<_SidePanel> createState() => _SidePanelState();
}

class _SidePanelState extends ConsumerState<_SidePanel>
    with TickerProviderStateMixin {
  late final TabController _tabController;

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 3, vsync: this);
  }

  @override
  void dispose() {
    _tabController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(
        color: GolemTheme.bgSurface,
      ),
      child: Column(
        children: [
          // Tab bar
          Container(
            decoration: const BoxDecoration(
              border: Border(bottom: BorderSide(color: GolemTheme.border)),
            ),
            child: TabBar(
              controller: _tabController,
              tabs: const [
                Tab(text: 'Tasks'),
                Tab(text: 'Context'),
                Tab(text: 'Diff'),
              ],
              labelStyle: const TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w600,
                letterSpacing: 0.5,
              ),
              unselectedLabelStyle: const TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w400,
                letterSpacing: 0.5,
              ),
              labelColor: GolemTheme.textPrimary,
              unselectedLabelColor: GolemTheme.textSecondary,
              indicatorColor: GolemTheme.accent,
              indicatorSize: TabBarIndicatorSize.tab,
              indicatorWeight: 2,
              dividerHeight: 0,
              labelPadding: EdgeInsets.zero,
            ),
          ),
          // Tab content
          Expanded(
            child: TabBarView(
              controller: _tabController,
              children: const [
                _TasksTabContent(),
                _ContextTabContent(),
                _DiffTabContent(),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

// ─── Tasks Tab ───────────────────────────────────────────────────────────────

class _TasksTabContent extends ConsumerWidget {
  const _TasksTabContent();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(projectStateProvider);
    final sessions = ref.watch(sessionsProvider);

    if (state == null) return const SizedBox.shrink();

    final tasks = state.tasks;
    final done = tasks.where((t) => t.status == 'done').length;
    final lastSession = sessions.isNotEmpty ? sessions.last : null;

    return Column(
      children: [
        // Header
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
          decoration: const BoxDecoration(
            border: Border(bottom: BorderSide(color: GolemTheme.border)),
          ),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              const Text(
                'TASKS',
                style: TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.w600,
                  letterSpacing: 1,
                  color: GolemTheme.textSecondary,
                ),
              ),
              Text(
                '$done/${tasks.length}',
                style: const TextStyle(
                    fontSize: 11, color: GolemTheme.textSecondary),
              ),
            ],
          ),
        ),
        // Task list
        Expanded(
          child: ListView.builder(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
            itemCount: tasks.length,
            itemBuilder: (context, i) => _TaskItem(task: tasks[i]),
          ),
        ),
        // Stats footer
        Container(
          padding: const EdgeInsets.all(12),
          decoration: const BoxDecoration(
            border: Border(top: BorderSide(color: GolemTheme.border)),
          ),
          child: Column(
            children: [
              _StatRow(
                  'Phase',
                  state.status.phase.isNotEmpty
                      ? state.status.phase
                      : '\u2014'),
              _StatRow(
                'Focus',
                state.status.currentFocus.isNotEmpty
                    ? state.status.currentFocus
                    : '\u2014',
              ),
              if (lastSession != null) ...[
                _StatRow('Last iter', '#${lastSession.iteration}'),
                _StatRow(
                  'Outcome',
                  lastSession.outcome,
                  valueColor: switch (lastSession.outcome) {
                    'done' => GolemTheme.green,
                    'blocked' || 'unproductive' => GolemTheme.red,
                    _ => GolemTheme.yellow,
                  },
                ),
              ],
              _StatRow('Decisions', '${state.decisions.length}'),
              _StatRow('Pitfalls', '${state.pitfalls.length}'),
            ],
          ),
        ),
      ],
    );
  }
}

// ─── Context Tab ─────────────────────────────────────────────────────────────

class _ContextTabContent extends ConsumerWidget {
  const _ContextTabContent();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final contextMap = ref.watch(contextMapProvider);

    if (contextMap == null || contextMap.symbols.isEmpty) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Text(
            'No context map available',
            style: GolemTheme.metaStyle(),
          ),
        ),
      );
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Task header
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
          decoration: const BoxDecoration(
            border: Border(bottom: BorderSide(color: GolemTheme.border)),
          ),
          child: Row(
            children: [
              const Text(
                'RELEVANT SYMBOLS',
                style: TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.w600,
                  letterSpacing: 1,
                  color: GolemTheme.textSecondary,
                ),
              ),
              const Spacer(),
              Text(
                '${contextMap.symbols.length}',
                style: const TextStyle(
                    fontSize: 11, color: GolemTheme.textSecondary),
              ),
            ],
          ),
        ),
        // Symbol list
        Expanded(
          child: ListView.builder(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
            itemCount: contextMap.symbols.length,
            itemBuilder: (context, i) =>
                _ContextSymbolItem(symbol: contextMap.symbols[i]),
          ),
        ),
      ],
    );
  }
}

class _ContextSymbolItem extends StatelessWidget {
  final ContextSymbol symbol;
  const _ContextSymbolItem({required this.symbol});

  @override
  Widget build(BuildContext context) {
    final kindColor = switch (symbol.kind) {
      'function' || 'method' => GolemTheme.accent,
      'type' || 'struct' || 'class' => GolemTheme.purple,
      'interface' => GolemTheme.yellow,
      'variable' || 'const' => GolemTheme.green,
      _ => GolemTheme.textSecondary,
    };

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 1),
                decoration: BoxDecoration(
                  color: kindColor.withValues(alpha: 0.15),
                  borderRadius: BorderRadius.circular(3),
                ),
                child: Text(
                  symbol.kind,
                  style: TextStyle(
                    fontSize: 9,
                    color: kindColor,
                    fontWeight: FontWeight.w500,
                  ),
                ),
              ),
              const SizedBox(width: 6),
              Expanded(
                child: Text(
                  symbol.name,
                  style: GolemTheme.monoStyle(fontSize: 11),
                  overflow: TextOverflow.ellipsis,
                ),
              ),
            ],
          ),
          Padding(
            padding: const EdgeInsets.only(top: 1),
            child: Text(
              '${symbol.path}${symbol.line > 0 ? ':${symbol.line}' : ''}',
              style: const TextStyle(
                fontSize: 10,
                color: GolemTheme.textSecondary,
              ),
              overflow: TextOverflow.ellipsis,
            ),
          ),
        ],
      ),
    );
  }
}

// ─── Diff Tab ────────────────────────────────────────────────────────────────

class _DiffTabContent extends StatelessWidget {
  const _DiffTabContent();

  @override
  Widget build(BuildContext context) {
    return const DiffViewer(compact: true);
  }
}

// ─── Shared Widgets ──────────────────────────────────────────────────────────

class _TaskItem extends StatelessWidget {
  final models.Task task;
  const _TaskItem({required this.task});

  @override
  Widget build(BuildContext context) {
    final (icon, color) = switch (task.status) {
      'done' => ('\u2713', GolemTheme.green),
      'in-progress' => ('\u25D0', GolemTheme.yellow),
      'blocked' => ('\u2717', GolemTheme.red),
      _ => ('\u25CB', GolemTheme.textSecondary),
    };

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 16,
            child: Text(icon,
                style: TextStyle(
                    fontSize: 11, color: color, fontFamily: 'monospace')),
          ),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  task.name,
                  style: const TextStyle(fontSize: 12),
                  overflow: TextOverflow.ellipsis,
                ),
                if (task.blockedReason != null)
                  Text(
                    task.blockedReason!,
                    style:
                        const TextStyle(fontSize: 10, color: GolemTheme.red),
                    overflow: TextOverflow.ellipsis,
                  ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _StatRow extends StatelessWidget {
  final String label;
  final String value;
  final Color? valueColor;

  const _StatRow(this.label, this.value, {this.valueColor});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 1),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Text(label,
              style: const TextStyle(
                  fontSize: 11, color: GolemTheme.textSecondary)),
          Flexible(
            child: Text(
              value,
              style: TextStyle(
                  fontSize: 11,
                  color: valueColor ?? GolemTheme.textPrimary),
              overflow: TextOverflow.ellipsis,
              textAlign: TextAlign.end,
            ),
          ),
        ],
      ),
    );
  }
}
