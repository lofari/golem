import 'package:flutter/material.dart';

import '../models/run.dart';
import '../theme.dart';
import 'detail_state.dart';
import 'detail_terminal.dart';
import 'detail_timeline.dart';

/// Right-side detail panel with State/Terminal/Timeline tabs.
class DetailPanel extends StatefulWidget {
  final RunInfo? selectedRun;
  final Map<String, dynamic>? pipelineState;
  final List<EngineEvent> events;
  final String? processId;

  const DetailPanel({
    super.key,
    this.selectedRun,
    this.pipelineState,
    this.events = const [],
    this.processId,
  });

  @override
  State<DetailPanel> createState() => _DetailPanelState();
}

class _DetailPanelState extends State<DetailPanel>
    with SingleTickerProviderStateMixin {
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
    return Column(
      children: [
        Container(
          decoration: const BoxDecoration(
            border: Border(bottom: BorderSide(color: GolemTheme.border)),
          ),
          child: TabBar(
            controller: _tabController,
            tabs: const [
              Tab(text: 'State'),
              Tab(text: 'Terminal'),
              Tab(text: 'Timeline'),
            ],
            labelStyle: const TextStyle(fontSize: 12),
            labelColor: GolemTheme.accent,
            unselectedLabelColor: GolemTheme.textSecondary,
            indicatorColor: GolemTheme.accent,
            indicatorSize: TabBarIndicatorSize.label,
          ),
        ),
        Expanded(
          child: TabBarView(
            controller: _tabController,
            children: [
              DetailState(
                run: widget.selectedRun,
                pipelineState: widget.pipelineState,
              ),
              DetailTerminal(processId: widget.processId),
              DetailTimeline(events: widget.events),
            ],
          ),
        ),
      ],
    );
  }
}
