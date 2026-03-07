import 'package:flutter/material.dart';
import '../theme.dart';

class DashboardView extends StatelessWidget {
  const DashboardView({super.key});

  @override
  Widget build(BuildContext context) {
    return const Center(
      child: Text('Dashboard', style: TextStyle(color: GolemTheme.textSecondary)),
    );
  }
}
