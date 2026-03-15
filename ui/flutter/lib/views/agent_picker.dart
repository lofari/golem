import 'package:flutter/material.dart';

import '../theme.dart';

/// Horizontal chip row for selecting an agent.
class AgentPicker extends StatelessWidget {
  final List<String> agents;
  final String selected;
  final ValueChanged<String> onChanged;

  const AgentPicker({
    super.key,
    required this.agents,
    required this.selected,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: 6,
      children: agents.map((name) {
        final isSelected = name == selected;
        return ChoiceChip(
          label: Text(name, style: const TextStyle(fontSize: 11)),
          selected: isSelected,
          onSelected: (_) => onChanged(name),
          selectedColor: GolemTheme.green.withValues(alpha: 0.2),
          backgroundColor: GolemTheme.bgPrimary,
          side: BorderSide(
            color: isSelected ? GolemTheme.green : GolemTheme.border,
          ),
          padding: EdgeInsets.zero,
          labelPadding: const EdgeInsets.symmetric(horizontal: 8),
          visualDensity: VisualDensity.compact,
        );
      }).toList(),
    );
  }
}
