import 'package:flutter/material.dart';
import '../theme.dart';

enum ProjectPhase { needsSetup, readyToPlan, readyToBuild, active }

class WelcomeView extends StatelessWidget {
  final ProjectPhase phase;
  final int taskCount;
  final VoidCallback onSetup;
  final VoidCallback onPlan;
  final VoidCallback onBuild;
  final VoidCallback onSkip;

  const WelcomeView({
    super.key,
    required this.phase,
    this.taskCount = 0,
    required this.onSetup,
    required this.onPlan,
    required this.onBuild,
    required this.onSkip,
  });

  @override
  Widget build(BuildContext context) {
    return Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 400),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(_icon, size: 48, color: GolemTheme.accent),
            const SizedBox(height: 16),
            Text(_title, style: const TextStyle(fontSize: 18, fontWeight: FontWeight.bold, color: GolemTheme.textPrimary)),
            const SizedBox(height: 8),
            Text(_subtitle, style: const TextStyle(fontSize: 13, color: GolemTheme.textSecondary), textAlign: TextAlign.center),
            const SizedBox(height: 24),
            ..._actions,
          ],
        ),
      ),
    );
  }

  IconData get _icon => switch (phase) {
    ProjectPhase.needsSetup => Icons.settings_suggest,
    ProjectPhase.readyToPlan => Icons.architecture,
    ProjectPhase.readyToBuild => Icons.rocket_launch,
    ProjectPhase.active => Icons.play_arrow,
  };

  String get _title => switch (phase) {
    ProjectPhase.needsSetup => 'Welcome to Golem',
    ProjectPhase.readyToPlan => 'Project configured',
    ProjectPhase.readyToBuild => 'Plan ready',
    ProjectPhase.active => '',
  };

  String get _subtitle => switch (phase) {
    ProjectPhase.needsSetup => 'This project needs configuration. Set up your stack, test commands, and preferences.',
    ProjectPhase.readyToPlan => 'Plan a feature to create tasks, or jump straight into a quick build.',
    ProjectPhase.readyToBuild => '$taskCount tasks ready from your plan. Launch the implementer to start building.',
    ProjectPhase.active => '',
  };

  List<Widget> get _actions => switch (phase) {
    ProjectPhase.needsSetup => [
      _primaryButton('Set up project', onSetup),
      const SizedBox(height: 8),
      _secondaryButton('Skip', onSkip),
    ],
    ProjectPhase.readyToPlan => [
      _primaryButton('Plan a feature', onPlan),
      const SizedBox(height: 8),
      _secondaryButton('Quick build', onSkip),
    ],
    ProjectPhase.readyToBuild => [
      _primaryButton('Launch implementer', onBuild),
      const SizedBox(height: 8),
      _secondaryButton('Review plan first', onPlan),
    ],
    ProjectPhase.active => [],
  };

  Widget _primaryButton(String label, VoidCallback onPressed) {
    return SizedBox(
      width: double.infinity,
      child: ElevatedButton(
        onPressed: onPressed,
        style: ElevatedButton.styleFrom(
          backgroundColor: GolemTheme.green,
          foregroundColor: Colors.white,
          padding: const EdgeInsets.symmetric(vertical: 12),
        ),
        child: Text(label),
      ),
    );
  }

  Widget _secondaryButton(String label, VoidCallback onPressed) {
    return SizedBox(
      width: double.infinity,
      child: OutlinedButton(
        onPressed: onPressed,
        style: OutlinedButton.styleFrom(
          foregroundColor: GolemTheme.textSecondary,
          side: const BorderSide(color: GolemTheme.border),
          padding: const EdgeInsets.symmetric(vertical: 12),
        ),
        child: Text(label),
      ),
    );
  }
}
