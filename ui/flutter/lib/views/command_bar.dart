import 'package:flutter/material.dart';

import '../theme.dart';
import 'agent_picker.dart';
import 'launch_dialog.dart';

/// Top bar in project workspace: Plan + Review buttons, goal input + agent picker + Go button + menu.
class CommandBar extends StatefulWidget {
  final List<String> agents;
  final void Function(String agent, String goal) onLaunch;
  final VoidCallback? onPlan;
  final VoidCallback? onReview;

  const CommandBar({
    super.key,
    required this.agents,
    required this.onLaunch,
    this.onPlan,
    this.onReview,
  });

  @override
  State<CommandBar> createState() => _CommandBarState();
}

class _CommandBarState extends State<CommandBar> {
  final _goalController = TextEditingController();
  late String _selectedAgent;

  @override
  void initState() {
    super.initState();
    _selectedAgent =
        widget.agents.isNotEmpty ? widget.agents.first : 'implementer';
  }

  @override
  void dispose() {
    _goalController.dispose();
    super.dispose();
  }

  void _launch() {
    final goal = _goalController.text.trim();
    if (goal.isEmpty) return;
    widget.onLaunch(_selectedAgent, goal);
    _goalController.clear();
  }

  Widget _actionButton(String label, IconData icon, VoidCallback? onPressed) {
    return OutlinedButton.icon(
      onPressed: onPressed,
      icon: Icon(icon, size: 14),
      label: Text(label, style: const TextStyle(fontSize: 12)),
      style: OutlinedButton.styleFrom(
        foregroundColor: GolemTheme.textPrimary,
        side: const BorderSide(color: GolemTheme.border),
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
        minimumSize: Size.zero,
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: const BoxDecoration(
        color: GolemTheme.bgSurface,
        border: Border(bottom: BorderSide(color: GolemTheme.border)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              _actionButton('Plan', Icons.architecture, widget.onPlan),
              const SizedBox(width: 6),
              _actionButton('Review', Icons.rate_review, widget.onReview),
              const SizedBox(width: 8),
              const SizedBox(
                height: 24,
                child: VerticalDivider(
                  color: GolemTheme.border,
                  width: 16,
                  thickness: 1,
                ),
              ),
              Expanded(
                child: TextField(
                  controller: _goalController,
                  style: const TextStyle(fontSize: 13),
                  decoration: InputDecoration(
                    hintText: 'Describe what you want to build...',
                    hintStyle: TextStyle(
                      fontSize: 13,
                      color: GolemTheme.textSecondary.withValues(alpha: 0.5),
                    ),
                    isDense: true,
                    contentPadding: const EdgeInsets.symmetric(
                        horizontal: 12, vertical: 10),
                    border: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(6),
                      borderSide:
                          const BorderSide(color: GolemTheme.border),
                    ),
                    enabledBorder: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(6),
                      borderSide:
                          const BorderSide(color: GolemTheme.border),
                    ),
                    focusedBorder: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(6),
                      borderSide:
                          const BorderSide(color: GolemTheme.accent),
                    ),
                  ),
                  onSubmitted: (_) => _launch(),
                ),
              ),
              const SizedBox(width: 8),
              ElevatedButton.icon(
                onPressed: _launch,
                icon: const Icon(Icons.play_arrow, size: 16),
                label: const Text('Go', style: TextStyle(fontSize: 12)),
                style: ElevatedButton.styleFrom(
                  backgroundColor: GolemTheme.green,
                  foregroundColor: Colors.white,
                  padding: const EdgeInsets.symmetric(
                      horizontal: 16, vertical: 10),
                  minimumSize: Size.zero,
                ),
              ),
              const SizedBox(width: 4),
              IconButton(
                icon: const Icon(Icons.more_vert, size: 18),
                color: GolemTheme.textSecondary,
                tooltip: 'Advanced launch options',
                splashRadius: 16,
                padding: EdgeInsets.zero,
                constraints: const BoxConstraints(minWidth: 32, minHeight: 32),
                onPressed: () {
                  showDialog(
                    context: context,
                    builder: (_) => const LaunchDialog(),
                  );
                },
              ),
            ],
          ),
          const SizedBox(height: 8),
          AgentPicker(
            agents: widget.agents,
            selected: _selectedAgent,
            onChanged: (agent) =>
                setState(() => _selectedAgent = agent),
          ),
        ],
      ),
    );
  }
}
