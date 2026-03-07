import 'package:flutter/material.dart';

class LaunchDialog extends StatelessWidget {
  const LaunchDialog({super.key});

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Launch Process'),
      content: const Text('Coming soon'),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Close')),
      ],
    );
  }
}
