import 'package:flutter_riverpod/flutter_riverpod.dart';

// Re-export projectListProvider from graph.dart as the canonical projects provider.
// The existing ProjectListNotifier already calls api.listProjects().
export 'graph.dart' show projectListProvider;

/// Currently selected project tab ID (null = Activity feed).
final selectedProjectIdProvider = StateProvider<String?>((ref) => null);

/// Set of open project tab IDs (persists during session).
final openProjectTabsProvider = StateProvider<Set<String>>((ref) => {});
