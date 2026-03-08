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

  void set(ProjectInfo info) {
    state = info;
  }
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
  void Function(Map<String, dynamic>)? _onLogAppended;

  ProjectStateNotifier(this._api, this._projectId) : super(null) {
    if (_projectId != null) {
      _fetch();
      _connectWs();
    }
  }

  set onLogAppended(void Function(Map<String, dynamic>)? cb) =>
      _onLogAppended = cb;

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
        if (data['type'] == 'log_appended') {
          _onLogAppended?.call(data);
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
  final projectState = ref.watch(projectStateProvider.notifier);
  final notifier = SessionsNotifier(api, projectInfo?.id);

  // Register callback so WebSocket log_appended events update sessions.
  projectState.onLogAppended = (data) {
    if (data['session'] != null) {
      final session =
          Session.fromJson(data['session'] as Map<String, dynamic>);
      notifier.addSession(session);
    }
  };

  ref.onDispose(() {
    projectState.onLogAppended = null;
  });

  return notifier;
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
