import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'theme.dart';
import 'views/app_shell.dart';

void main() {
  runApp(const ProviderScope(child: GolemApp()));
}

class GolemApp extends StatelessWidget {
  const GolemApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Golem',
      debugShowCheckedModeBanner: false,
      theme: GolemTheme.dark(),
      home: const AppShell(),
    );
  }
}
