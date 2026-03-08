import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/client.dart';
import '../models/graph.dart';
import '../models/project.dart';
import 'connection.dart';
import 'project.dart';

// Graph stats — fetched on demand, refreshable
final graphStatsProvider =
    StateNotifierProvider<GraphStatsNotifier, GraphStats?>((ref) {
  final projectInfo = ref.watch(projectInfoProvider);
  final api = ref.read(apiClientProvider);
  return GraphStatsNotifier(api, projectInfo?.id);
});

class GraphStatsNotifier extends StateNotifier<GraphStats?> {
  final GolemApiClient _api;
  final String? _projectId;

  GraphStatsNotifier(this._api, this._projectId) : super(null) {
    if (_projectId != null) _fetch();
  }

  Future<void> _fetch() async {
    try {
      final json = await _api.getGraphStats(_projectId!);
      state = GraphStats.fromJson(json);
    } catch (_) {
      // Graph doesn't exist — leave null
    }
  }

  void refresh() => _fetch();
}

// Diff summary — fetched on demand
final diffProvider =
    StateNotifierProvider<DiffNotifier, DiffSummary?>((ref) {
  final projectInfo = ref.watch(projectInfoProvider);
  final api = ref.read(apiClientProvider);
  return DiffNotifier(api, projectInfo?.id);
});

class DiffNotifier extends StateNotifier<DiffSummary?> {
  final GolemApiClient _api;
  final String? _projectId;

  DiffNotifier(this._api, this._projectId) : super(null) {
    if (_projectId != null) _fetch();
  }

  Future<void> _fetch() async {
    try {
      final json = await _api.getDiff(_projectId!);
      state = DiffSummary.fromJson(json);
    } catch (_) {}
  }

  void refresh() => _fetch();

  Future<String> loadPatch(String filePath) async {
    final pid = _projectId;
    if (pid == null) return '';
    return _api.getFilePatch(pid, filePath);
  }
}

// Graph search results
final graphSearchProvider = StateNotifierProvider.family<
    GraphSearchNotifier, List<GraphSearchResult>, String>((ref, query) {
  final projectInfo = ref.read(projectInfoProvider);
  final api = ref.read(apiClientProvider);
  return GraphSearchNotifier(api, projectInfo?.id, query);
});

class GraphSearchNotifier extends StateNotifier<List<GraphSearchResult>> {
  final GolemApiClient _api;
  final String? _projectId;

  GraphSearchNotifier(this._api, this._projectId, String query) : super([]) {
    if (_projectId != null && query.isNotEmpty) _search(query);
  }

  Future<void> _search(String query, {List<String>? types}) async {
    try {
      final results = await _api.graphSearch(_projectId!, query, types: types);
      state = results
          .map((e) => GraphSearchResult.fromJson(e as Map<String, dynamic>))
          .toList();
    } catch (_) {}
  }
}

// Graph related (for explorer)
final graphRelatedProvider = FutureProvider.family<GraphRelatedResult, String>(
    (ref, name) async {
  final projectInfo = ref.read(projectInfoProvider);
  final api = ref.read(apiClientProvider);
  if (projectInfo == null) return GraphRelatedResult(nodes: [], edges: []);
  final json = await api.graphRelated(projectInfo.id, name);
  return GraphRelatedResult.fromJson(json);
});

// Project list for switcher
final projectListProvider =
    StateNotifierProvider<ProjectListNotifier, List<ProjectInfo>>((ref) {
  final api = ref.read(apiClientProvider);
  return ProjectListNotifier(api);
});

class ProjectListNotifier extends StateNotifier<List<ProjectInfo>> {
  final GolemApiClient _api;

  ProjectListNotifier(this._api) : super([]) {
    _fetch();
  }

  Future<void> _fetch() async {
    try {
      final projects = await _api.listProjects();
      state = projects;
    } catch (_) {}
  }

  void refresh() => _fetch();
}
