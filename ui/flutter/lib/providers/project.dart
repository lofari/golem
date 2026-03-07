import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/client.dart';
import '../api/websocket.dart';
import '../models/project.dart';
import 'connection.dart';

// The single project this instance manages
final projectInfoProvider = StateNotifierProvider<ProjectInfoNotifier, ProjectInfo?>((ref) {
  return ProjectInfoNotifier(ref.read(apiClientProvider));
});

class ProjectInfoNotifier extends StateNotifier<ProjectInfo?> {
  final GolemApiClient _api;

  ProjectInfoNotifier(this._api) : super(null) {
    _fetch();
  }

  Future<void> _fetch() async {
    try {
      final projects = await _api.listProjects();
      if (projects.isNotEmpty) {
        state = projects.first;
      }
    } catch (_) {}
  }

  void refresh() => _fetch();
}

// Project state (tasks, decisions, etc.)
final projectStateProvider = StateNotifierProvider<ProjectStateNotifier, ProjectState?>((ref) {
  final projectInfo = ref.watch(projectInfoProvider);
  final api = ref.read(apiClientProvider);
  return ProjectStateNotifier(api, projectInfo?.id);
});

class ProjectStateNotifier extends StateNotifier<ProjectState?> {
  final GolemApiClient _api;
  final String? _projectId;
  GolemWebSocket? _ws;

  ProjectStateNotifier(this._api, this._projectId) : super(null) {
    if (_projectId != null) {
      _fetch();
      _connectWs();
    }
  }

  Future<void> _fetch() async {
    try {
      state = await _api.getState(_projectId!);
    } catch (_) {}
  }

  void _connectWs() {
    _ws = GolemWebSocket(
      url: _api.stateWatchUrl(_projectId!),
      onMessage: (data) {
        if (data['type'] == 'state_changed' && data['state'] != null) {
          state = ProjectState.fromJson(data['state'] as Map<String, dynamic>);
        }
      },
    );
    _ws!.connect();
  }

  @override
  void dispose() {
    _ws?.dispose();
    super.dispose();
  }
}

// Sessions from log
final sessionsProvider = StateNotifierProvider<SessionsNotifier, List<Session>>((ref) {
  final projectInfo = ref.watch(projectInfoProvider);
  final api = ref.read(apiClientProvider);
  return SessionsNotifier(api, projectInfo?.id);
});

class SessionsNotifier extends StateNotifier<List<Session>> {
  final GolemApiClient _api;
  final String? _projectId;

  SessionsNotifier(this._api, this._projectId) : super([]) {
    if (_projectId != null) _fetch();
  }

  Future<void> _fetch() async {
    try {
      state = await _api.getLog(_projectId!);
    } catch (_) {}
  }

  void addSession(Session session) {
    state = [...state, session];
  }
}
