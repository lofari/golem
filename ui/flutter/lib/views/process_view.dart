import 'package:flutter/material.dart';
import '../theme.dart';

class ProcessView extends StatelessWidget {
  final String processId;
  const ProcessView({super.key, required this.processId});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Text('Process: $processId', style: const TextStyle(color: GolemTheme.textSecondary)),
    );
  }
}
