# Flutter Desktop UI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace Tauri+React UI with a Flutter desktop app that connects to the existing Go API server.

**Architecture:** Flutter desktop app (Linux) communicates with Go server at `localhost:8314` via REST + WebSocket. Riverpod for state management. Material 3 dark theme. Single-project UI (no project picker).

**Tech Stack:** Flutter 3.38, Dart, Riverpod, http, web_socket_channel, google_fonts

**Design doc:** `docs/plans/2026-03-07-flutter-desktop-ui-design.md`

---

### Task 0: Add json tags to Config struct

The config API endpoint returns PascalCase field names. Add json tags to match the kebab-case yaml tags used by the frontend.

**Files:**
- Modify: `internal/config/config.go`

**Step 1: Add json tags to Config struct**

In `internal/config/config.go`, update the `Config` struct:

```go
type Config struct {
	MaxIterations  int      `yaml:"max-iterations" json:"max-iterations"`
	MaxToolCalls   int      `yaml:"max-tool-calls" json:"max-tool-calls"`
	Verbose        bool     `yaml:"verbose" json:"verbose"`
	Sandbox        bool     `yaml:"sandbox" json:"sandbox"`
	SandboxTools   []string `yaml:"sandbox-tools" json:"sandbox-tools,omitempty"`
	SandboxTimeout string   `yaml:"sandbox-timeout" json:"sandbox-timeout,omitempty"`
	SandboxMemory  string   `yaml:"sandbox-memory" json:"sandbox-memory,omitempty"`
	MCP            bool     `yaml:"mcp" json:"mcp"`
	Parallel       int      `yaml:"parallel" json:"parallel"`
	PluginDir      []string `yaml:"plugin-dir" json:"plugin-dir,omitempty"`
	Model          string   `yaml:"model" json:"model"`
}
```

**Step 2: Verify**

Run: `go build ./... && go test ./...`
Expected: All pass.

**Step 3: Verify API returns lowercase**

Run: `curl -s http://localhost:8314/api/projects/<id>/config | python3 -m json.tool | head -5`
Expected: `"max-iterations": 20` (lowercase with hyphens)

**Step 4: Commit**

```bash
git add internal/config/config.go
git commit -m "fix(config): add json tags for API serialization"
```

---

### Task 1: Scaffold Flutter project

**Files:**
- Create: `ui/flutter/` (entire Flutter project scaffold)
- Create: `ui/flutter/pubspec.yaml`

**Step 1: Create Flutter project**

```bash
cd ui && flutter create --org com.golem --project-name golem_ui --platforms linux flutter
```

**Step 2: Update pubspec.yaml dependencies**

Replace `ui/flutter/pubspec.yaml` with:

```yaml
name: golem_ui
description: Golem desktop UI
publish_to: 'none'
version: 0.1.0

environment:
  sdk: ^3.10.0

dependencies:
  flutter:
    sdk: flutter
  http: ^1.3.0
  web_socket_channel: ^3.0.0
  flutter_riverpod: ^2.6.0
  google_fonts: ^6.2.0

dev_dependencies:
  flutter_test:
    sdk: flutter
  flutter_lints: ^5.0.0
```

**Step 3: Get dependencies**

```bash
cd ui/flutter && flutter pub get
```

**Step 4: Verify it builds**

```bash
cd ui/flutter && flutter build linux --debug
```
Expected: Build succeeds, binary at `build/linux/x64/debug/bundle/golem_ui`

**Step 5: Commit**

```bash
git add ui/flutter/
git commit -m "feat(ui): scaffold Flutter desktop project"
```

---

### Task 2: Data models

**Files:**
- Create: `ui/flutter/lib/models/project.dart`
- Create: `ui/flutter/lib/models/process.dart`

**Step 1: Create project models**

`ui/flutter/lib/models/project.dart`:

```dart
class ProjectInfo {
  final String id;
  final String path;
  final String name;
  final String phase;

  ProjectInfo({
    required this.id,
    required this.path,
    required this.name,
    required this.phase,
  });

  factory ProjectInfo.fromJson(Map<String, dynamic> json) => ProjectInfo(
        id: json['id'] as String? ?? '',
        path: json['path'] as String? ?? '',
        name: json['name'] as String? ?? '',
        phase: json['phase'] as String? ?? '',
      );
}

class ProjectState {
  final ProjectMeta project;
  final ProjectStatus status;
  final List<Decision> decisions;
  final List<Lock> locked;
  final List<Task> tasks;
  final List<Pitfall> pitfalls;

  ProjectState({
    required this.project,
    required this.status,
    required this.decisions,
    required this.locked,
    required this.tasks,
    required this.pitfalls,
  });

  factory ProjectState.fromJson(Map<String, dynamic> json) => ProjectState(
        project: ProjectMeta.fromJson(json['project'] as Map<String, dynamic>? ?? {}),
        status: ProjectStatus.fromJson(json['status'] as Map<String, dynamic>? ?? {}),
        decisions: (json['decisions'] as List<dynamic>?)
                ?.map((e) => Decision.fromJson(e as Map<String, dynamic>))
                .toList() ??
            [],
        locked: (json['locked'] as List<dynamic>?)
                ?.map((e) => Lock.fromJson(e as Map<String, dynamic>))
                .toList() ??
            [],
        tasks: (json['tasks'] as List<dynamic>?)
                ?.map((e) => Task.fromJson(e as Map<String, dynamic>))
                .toList() ??
            [],
        pitfalls: (json['pitfalls'] as List<dynamic>?)
                ?.map((e) => Pitfall.fromJson(e as Map<String, dynamic>))
                .toList() ??
            [],
      );
}

class ProjectMeta {
  final String name;
  final String summary;
  final String stack;
  final String docsPath;

  ProjectMeta({
    required this.name,
    required this.summary,
    required this.stack,
    required this.docsPath,
  });

  factory ProjectMeta.fromJson(Map<String, dynamic> json) => ProjectMeta(
        name: json['name'] as String? ?? '',
        summary: json['summary'] as String? ?? '',
        stack: json['stack'] as String? ?? '',
        docsPath: json['docs_path'] as String? ?? '',
      );
}

class ProjectStatus {
  final String currentFocus;
  final String phase;
  final String lastSession;

  ProjectStatus({
    required this.currentFocus,
    required this.phase,
    required this.lastSession,
  });

  factory ProjectStatus.fromJson(Map<String, dynamic> json) => ProjectStatus(
        currentFocus: json['current_focus'] as String? ?? '',
        phase: json['phase'] as String? ?? '',
        lastSession: json['last_session'] as String? ?? '',
      );
}

class Decision {
  final String what;
  final String why;
  final String when;

  Decision({required this.what, required this.why, required this.when});

  factory Decision.fromJson(Map<String, dynamic> json) => Decision(
        what: json['what'] as String? ?? '',
        why: json['why'] as String? ?? '',
        when: json['when'] as String? ?? '',
      );
}

class Lock {
  final String path;
  final String note;

  Lock({required this.path, required this.note});

  factory Lock.fromJson(Map<String, dynamic> json) => Lock(
        path: json['path'] as String? ?? '',
        note: json['note'] as String? ?? '',
      );
}

class Task {
  final String name;
  final String status;
  final String? notes;
  final List<String>? dependsOn;
  final String? blockedReason;

  Task({
    required this.name,
    required this.status,
    this.notes,
    this.dependsOn,
    this.blockedReason,
  });

  factory Task.fromJson(Map<String, dynamic> json) => Task(
        name: json['name'] as String? ?? '',
        status: json['status'] as String? ?? 'todo',
        notes: json['notes'] as String?,
        dependsOn: (json['depends_on'] as List<dynamic>?)?.cast<String>(),
        blockedReason: json['blocked_reason'] as String?,
      );
}

class Pitfall {
  final String what;
  final String fix;

  Pitfall({required this.what, required this.fix});

  factory Pitfall.fromJson(Map<String, dynamic> json) => Pitfall(
        what: json['what'] as String? ?? '',
        fix: json['fix'] as String? ?? '',
      );
}

class Session {
  final int iteration;
  final String timestamp;
  final String task;
  final String outcome;
  final String summary;
  final String? handoff;
  final List<String>? filesChanged;
  final List<String>? decisionsMade;

  Session({
    required this.iteration,
    required this.timestamp,
    required this.task,
    required this.outcome,
    required this.summary,
    this.handoff,
    this.filesChanged,
    this.decisionsMade,
  });

  factory Session.fromJson(Map<String, dynamic> json) => Session(
        iteration: json['iteration'] as int? ?? 0,
        timestamp: json['timestamp'] as String? ?? '',
        task: json['task'] as String? ?? '',
        outcome: json['outcome'] as String? ?? '',
        summary: json['summary'] as String? ?? '',
        handoff: json['handoff'] as String?,
        filesChanged: (json['files_changed'] as List<dynamic>?)?.cast<String>(),
        decisionsMade: (json['decisions_made'] as List<dynamic>?)?.cast<String>(),
      );
}
```

**Step 2: Create process models**

`ui/flutter/lib/models/process.dart`:

```dart
class ProcessInfo {
  final String id;
  final String command;
  final String status;
  final String startedAt;
  final int? pid;

  ProcessInfo({
    required this.id,
    required this.command,
    required this.status,
    required this.startedAt,
    this.pid,
  });

  factory ProcessInfo.fromJson(Map<String, dynamic> json) => ProcessInfo(
        id: json['id'] as String? ?? '',
        command: json['command'] as String? ?? '',
        status: json['status'] as String? ?? '',
        startedAt: json['startedAt'] as String? ?? '',
        pid: json['pid'] as int?,
      );
}

class LaunchRequest {
  final String command;
  final LaunchConfig config;

  LaunchRequest({required this.command, required this.config});

  Map<String, dynamic> toJson() => {
        'command': command,
        'config': config.toJson(),
      };
}

class LaunchConfig {
  final int? maxIterations;
  final int? maxToolCalls;
  final String? model;
  final String? task;
  final bool sandbox;
  final bool mcp;
  final int? parallel;

  LaunchConfig({
    this.maxIterations,
    this.maxToolCalls,
    this.model,
    this.task,
    this.sandbox = false,
    this.mcp = true,
    this.parallel,
  });

  Map<String, dynamic> toJson() {
    final map = <String, dynamic>{};
    if (maxIterations != null) map['maxIterations'] = maxIterations;
    if (maxToolCalls != null) map['maxToolCalls'] = maxToolCalls;
    if (model != null && model!.isNotEmpty) map['model'] = model;
    if (task != null && task!.isNotEmpty) map['task'] = task;
    if (sandbox) map['sandbox'] = true;
    if (mcp) map['mcp'] = true;
    if (parallel != null && parallel! > 1) map['parallel'] = parallel;
    return map;
  }
}

class GolemConfig {
  int maxIterations;
  int maxToolCalls;
  bool verbose;
  bool sandbox;
  bool mcp;
  int parallel;
  String model;

  GolemConfig({
    this.maxIterations = 20,
    this.maxToolCalls = 200,
    this.verbose = false,
    this.sandbox = false,
    this.mcp = true,
    this.parallel = 1,
    this.model = '',
  });

  factory GolemConfig.fromJson(Map<String, dynamic> json) => GolemConfig(
        maxIterations: json['max-iterations'] as int? ?? 20,
        maxToolCalls: json['max-tool-calls'] as int? ?? 200,
        verbose: json['verbose'] as bool? ?? false,
        sandbox: json['sandbox'] as bool? ?? false,
        mcp: json['mcp'] as bool? ?? true,
        parallel: json['parallel'] as int? ?? 1,
        model: json['model'] as String? ?? '',
      );

  Map<String, dynamic> toJson() => {
        'max-iterations': maxIterations,
        'max-tool-calls': maxToolCalls,
        'verbose': verbose,
        'sandbox': sandbox,
        'mcp': mcp,
        'parallel': parallel,
        'model': model,
      };
}
```

**Step 3: Verify compilation**

```bash
cd ui/flutter && flutter analyze
```
Expected: No errors.

**Step 4: Commit**

```bash
git add ui/flutter/lib/models/
git commit -m "feat(ui): add data models for project, process, and config"
```

---

### Task 3: API client

**Files:**
- Create: `ui/flutter/lib/api/client.dart`

**Step 1: Create the REST client**

`ui/flutter/lib/api/client.dart`:

```dart
import 'dart:convert';
import 'package:http/http.dart' as http;
import '../models/project.dart';
import '../models/process.dart';

class GolemApiClient {
  final String baseUrl;
  final http.Client _http;

  GolemApiClient({this.baseUrl = 'http://localhost:8314'})
      : _http = http.Client();

  void dispose() => _http.close();

  Future<Map<String, dynamic>> _getJson(String path) async {
    final resp = await _http.get(Uri.parse('$baseUrl$path'),
        headers: {'Content-Type': 'application/json'});
    if (resp.statusCode >= 400) {
      final body = jsonDecode(resp.body) as Map<String, dynamic>;
      throw ApiException(body['error'] as String? ?? resp.reasonPhrase ?? 'Unknown error');
    }
    return jsonDecode(resp.body) as Map<String, dynamic>;
  }

  Future<List<dynamic>> _getJsonList(String path) async {
    final resp = await _http.get(Uri.parse('$baseUrl$path'),
        headers: {'Content-Type': 'application/json'});
    if (resp.statusCode >= 400) {
      final body = jsonDecode(resp.body) as Map<String, dynamic>;
      throw ApiException(body['error'] as String? ?? resp.reasonPhrase ?? 'Unknown error');
    }
    return jsonDecode(resp.body) as List<dynamic>;
  }

  Future<Map<String, dynamic>> _postJson(String path, Map<String, dynamic> body) async {
    final resp = await _http.post(
      Uri.parse('$baseUrl$path'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode(body),
    );
    if (resp.statusCode >= 400) {
      final respBody = jsonDecode(resp.body) as Map<String, dynamic>;
      throw ApiException(respBody['error'] as String? ?? resp.reasonPhrase ?? 'Unknown error');
    }
    return jsonDecode(resp.body) as Map<String, dynamic>;
  }

  Future<Map<String, dynamic>> _putJson(String path, Map<String, dynamic> body) async {
    final resp = await _http.put(
      Uri.parse('$baseUrl$path'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode(body),
    );
    if (resp.statusCode >= 400) {
      final respBody = jsonDecode(resp.body) as Map<String, dynamic>;
      throw ApiException(respBody['error'] as String? ?? resp.reasonPhrase ?? 'Unknown error');
    }
    return jsonDecode(resp.body) as Map<String, dynamic>;
  }

  Future<Map<String, dynamic>> _delete(String path) async {
    final resp = await _http.delete(Uri.parse('$baseUrl$path'),
        headers: {'Content-Type': 'application/json'});
    if (resp.statusCode >= 400) {
      final body = jsonDecode(resp.body) as Map<String, dynamic>;
      throw ApiException(body['error'] as String? ?? resp.reasonPhrase ?? 'Unknown error');
    }
    return jsonDecode(resp.body) as Map<String, dynamic>;
  }

  // Health
  Future<bool> health() async {
    try {
      await _getJson('/api/health');
      return true;
    } catch (_) {
      return false;
    }
  }

  // Projects
  Future<List<ProjectInfo>> listProjects() async {
    final list = await _getJsonList('/api/projects');
    return list.map((e) => ProjectInfo.fromJson(e as Map<String, dynamic>)).toList();
  }

  // State
  Future<ProjectState> getState(String projectId) async {
    final json = await _getJson('/api/projects/$projectId/state');
    return ProjectState.fromJson(json);
  }

  // Log
  Future<List<Session>> getLog(String projectId) async {
    final json = await _getJson('/api/projects/$projectId/log');
    final sessions = json['sessions'] as List<dynamic>? ?? [];
    return sessions.map((e) => Session.fromJson(e as Map<String, dynamic>)).toList();
  }

  // Processes
  Future<List<ProcessInfo>> listProcesses(String projectId) async {
    final list = await _getJsonList('/api/projects/$projectId/processes');
    return list.map((e) => ProcessInfo.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<String> launchProcess(String projectId, LaunchRequest req) async {
    final json = await _postJson('/api/projects/$projectId/processes', req.toJson());
    return json['id'] as String;
  }

  Future<void> stopProcess(String projectId, String processId) async {
    await _delete('/api/projects/$projectId/processes/$processId');
  }

  // Config
  Future<GolemConfig> getProjectConfig(String projectId) async {
    final json = await _getJson('/api/projects/$projectId/config');
    return GolemConfig.fromJson(json);
  }

  Future<void> updateProjectConfig(String projectId, GolemConfig config) async {
    await _putJson('/api/projects/$projectId/config', config.toJson());
  }

  Future<GolemConfig> getGlobalConfig() async {
    final json = await _getJson('/api/config');
    return GolemConfig.fromJson(json);
  }

  Future<void> updateGlobalConfig(GolemConfig config) async {
    await _putJson('/api/config', config.toJson());
  }

  // WebSocket URLs
  String processStreamUrl(String projectId, String processId) =>
      'ws://localhost:8314/api/projects/$projectId/processes/$processId/stream';

  String stateWatchUrl(String projectId) =>
      'ws://localhost:8314/api/projects/$projectId/watch';
}

class ApiException implements Exception {
  final String message;
  ApiException(this.message);

  @override
  String toString() => message;
}
```

**Step 2: Verify**

```bash
cd ui/flutter && flutter analyze
```

**Step 3: Commit**

```bash
git add ui/flutter/lib/api/client.dart
git commit -m "feat(ui): add REST API client"
```

---

### Task 4: WebSocket manager

**Files:**
- Create: `ui/flutter/lib/api/websocket.dart`

**Step 1: Create the WebSocket manager**

`ui/flutter/lib/api/websocket.dart`:

```dart
import 'dart:async';
import 'dart:convert';
import 'dart:math';
import 'package:web_socket_channel/web_socket_channel.dart';

class GolemWebSocket {
  final String url;
  final void Function(Map<String, dynamic> data) onMessage;
  final void Function()? onConnected;
  final void Function()? onDisconnected;

  WebSocketChannel? _channel;
  StreamSubscription? _subscription;
  Timer? _reconnectTimer;
  int _retryDelay = 2;
  bool _disposed = false;

  GolemWebSocket({
    required this.url,
    required this.onMessage,
    this.onConnected,
    this.onDisconnected,
  });

  void connect() {
    if (_disposed) return;

    try {
      _channel = WebSocketChannel.connect(Uri.parse(url));
      onConnected?.call();
      _retryDelay = 2;

      _subscription = _channel!.stream.listen(
        (data) {
          try {
            final json = jsonDecode(data as String) as Map<String, dynamic>;
            onMessage(json);
          } catch (_) {}
        },
        onDone: () {
          onDisconnected?.call();
          _scheduleReconnect();
        },
        onError: (_) {
          onDisconnected?.call();
          _scheduleReconnect();
        },
      );
    } catch (_) {
      _scheduleReconnect();
    }
  }

  void _scheduleReconnect() {
    if (_disposed) return;
    _reconnectTimer?.cancel();
    _reconnectTimer = Timer(Duration(seconds: _retryDelay), () {
      _retryDelay = min(_retryDelay * 2, 30);
      connect();
    });
  }

  void dispose() {
    _disposed = true;
    _reconnectTimer?.cancel();
    _subscription?.cancel();
    _channel?.sink.close();
  }
}
```

**Step 2: Verify**

```bash
cd ui/flutter && flutter analyze
```

**Step 3: Commit**

```bash
git add ui/flutter/lib/api/websocket.dart
git commit -m "feat(ui): add WebSocket manager with reconnection"
```

---

### Task 5: Riverpod providers

**Files:**
- Create: `ui/flutter/lib/providers/connection.dart`
- Create: `ui/flutter/lib/providers/project.dart`
- Create: `ui/flutter/lib/providers/processes.dart`

**Step 1: Create API client provider and connection provider**

`ui/flutter/lib/providers/connection.dart`:

```dart
import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/client.dart';

final apiClientProvider = Provider<GolemApiClient>((ref) {
  final client = GolemApiClient();
  ref.onDispose(client.dispose);
  return client;
});

final connectionProvider = StateNotifierProvider<ConnectionNotifier, bool>((ref) {
  return ConnectionNotifier(ref.read(apiClientProvider));
});

class ConnectionNotifier extends StateNotifier<bool> {
  final GolemApiClient _api;
  Timer? _timer;

  ConnectionNotifier(this._api) : super(false) {
    _poll();
    _timer = Timer.periodic(const Duration(seconds: 5), (_) => _poll());
  }

  Future<void> _poll() async {
    state = await _api.health();
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }
}
```

**Step 2: Create project providers**

`ui/flutter/lib/providers/project.dart`:

```dart
import 'dart:async';
import 'dart:convert';
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
```

**Step 3: Create process providers**

`ui/flutter/lib/providers/processes.dart`:

```dart
import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/client.dart';
import '../api/websocket.dart';
import '../models/process.dart';
import 'connection.dart';
import 'project.dart';

final processesProvider = StateNotifierProvider<ProcessesNotifier, List<ProcessInfo>>((ref) {
  final projectInfo = ref.watch(projectInfoProvider);
  final api = ref.read(apiClientProvider);
  return ProcessesNotifier(api, projectInfo?.id);
});

class ProcessesNotifier extends StateNotifier<List<ProcessInfo>> {
  final GolemApiClient _api;
  final String? _projectId;
  Timer? _timer;

  ProcessesNotifier(this._api, this._projectId) : super([]) {
    if (_projectId != null) {
      _fetch();
      _timer = Timer.periodic(const Duration(seconds: 5), (_) => _fetch());
    }
  }

  Future<void> _fetch() async {
    try {
      state = await _api.listProcesses(_projectId!);
    } catch (_) {}
  }

  void refresh() => _fetch();

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }
}

// Selected process tab
final selectedProcessIdProvider = StateProvider<String?>((ref) => null);

// Output lines per process
final processOutputProvider =
    StateNotifierProvider.family<ProcessOutputNotifier, List<String>, String>((ref, processId) {
  final projectInfo = ref.read(projectInfoProvider);
  final api = ref.read(apiClientProvider);
  return ProcessOutputNotifier(api, projectInfo?.id, processId);
});

class ProcessOutputNotifier extends StateNotifier<List<String>> {
  final GolemApiClient _api;
  GolemWebSocket? _ws;
  static const _maxLines = 5000;

  ProcessOutputNotifier(this._api, String? projectId, String processId) : super([]) {
    if (projectId != null) {
      _ws = GolemWebSocket(
        url: _api.processStreamUrl(projectId, processId),
        onMessage: (data) {
          if (data['type'] == 'output' && data['line'] != null) {
            final newLines = [...state, data['line'] as String];
            if (newLines.length > _maxLines) {
              state = newLines.sublist(newLines.length - _maxLines);
            } else {
              state = newLines;
            }
          }
        },
      );
      _ws!.connect();
    }
  }

  @override
  void dispose() {
    _ws?.dispose();
    super.dispose();
  }
}
```

**Step 4: Verify**

```bash
cd ui/flutter && flutter analyze
```

**Step 5: Commit**

```bash
git add ui/flutter/lib/providers/
git commit -m "feat(ui): add Riverpod providers for connection, project, and processes"
```

---

### Task 6: Theme and main.dart

**Files:**
- Create: `ui/flutter/lib/theme.dart`
- Modify: `ui/flutter/lib/main.dart`

**Step 1: Create theme**

`ui/flutter/lib/theme.dart`:

```dart
import 'package:flutter/material.dart';
import 'package:google_fonts/google_fonts.dart';

class GolemTheme {
  static const bgPrimary = Color(0xFF0d1117);
  static const bgSurface = Color(0xFF161b22);
  static const bgElevated = Color(0xFF1c2128);
  static const border = Color(0xFF30363d);
  static const textPrimary = Color(0xFFe6edf3);
  static const textSecondary = Color(0xFF8b949e);
  static const accent = Color(0xFF58a6ff);
  static const green = Color(0xFF3fb950);
  static const yellow = Color(0xFFd29922);
  static const red = Color(0xFFf85149);

  static ThemeData dark() {
    return ThemeData(
      useMaterial3: true,
      brightness: Brightness.dark,
      scaffoldBackgroundColor: bgPrimary,
      colorScheme: const ColorScheme.dark(
        primary: accent,
        surface: bgSurface,
        onSurface: textPrimary,
        onPrimary: Colors.white,
        outline: border,
      ),
      cardTheme: const CardThemeData(
        color: bgSurface,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.all(Radius.circular(8)),
          side: BorderSide(color: border),
        ),
      ),
      dividerColor: border,
      textTheme: GoogleFonts.interTextTheme(ThemeData.dark().textTheme).apply(
        bodyColor: textPrimary,
        displayColor: textPrimary,
      ),
      appBarTheme: const AppBarTheme(
        backgroundColor: bgSurface,
        foregroundColor: textPrimary,
        elevation: 0,
        surfaceTintColor: Colors.transparent,
      ),
      tabBarTheme: const TabBarThemeData(
        labelColor: textPrimary,
        unselectedLabelColor: textSecondary,
        indicatorColor: accent,
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: bgPrimary,
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: border),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: border),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: accent),
        ),
        contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
        isDense: true,
      ),
      dialogTheme: const DialogThemeData(
        backgroundColor: bgSurface,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.all(Radius.circular(12)),
          side: BorderSide(color: border),
        ),
      ),
      filledButtonTheme: FilledButtonThemeData(
        style: FilledButton.styleFrom(
          backgroundColor: accent,
          foregroundColor: Colors.white,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(6)),
        ),
      ),
      textButtonTheme: TextButtonThemeData(
        style: TextButton.styleFrom(foregroundColor: textSecondary),
      ),
    );
  }

  static TextStyle monoStyle({double fontSize = 13}) {
    return GoogleFonts.jetBrainsMono(
      fontSize: fontSize,
      color: textPrimary,
      height: 1.5,
    );
  }
}
```

**Step 2: Update main.dart**

`ui/flutter/lib/main.dart`:

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'theme.dart';
import 'views/shell.dart';

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
      home: const ShellView(),
    );
  }
}
```

**Step 3: Verify**

```bash
cd ui/flutter && flutter analyze
```
Expected: Only warnings about missing ShellView (created in next task).

**Step 4: Commit**

```bash
git add ui/flutter/lib/theme.dart ui/flutter/lib/main.dart
git commit -m "feat(ui): add Material 3 dark theme and app entry point"
```

---

### Task 7: Shell view (top bar + status bar + content area)

**Files:**
- Create: `ui/flutter/lib/views/shell.dart`

**Step 1: Create shell view**

`ui/flutter/lib/views/shell.dart`:

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/process.dart';
import '../providers/connection.dart';
import '../providers/project.dart';
import '../providers/processes.dart';
import '../theme.dart';
import 'dashboard.dart';
import 'process_view.dart';
import 'launch_dialog.dart';
import 'settings_dialog.dart';

class ShellView extends ConsumerWidget {
  const ShellView({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final projectInfo = ref.watch(projectInfoProvider);
    final connected = ref.watch(connectionProvider);
    final processes = ref.watch(processesProvider);
    final selectedProcessId = ref.watch(selectedProcessIdProvider);

    return Scaffold(
      body: Column(
        children: [
          // Top bar
          _TopBar(
            projectName: projectInfo?.name ?? 'Golem',
            phase: projectInfo?.phase ?? '',
            onLaunch: () => _showLaunchDialog(context, ref),
            onPlan: () => _launchPlan(context, ref),
            onSettings: () => _showSettingsDialog(context, ref),
          ),
          // Process tabs (only if processes exist)
          if (processes.isNotEmpty)
            _ProcessTabs(
              processes: processes,
              selectedId: selectedProcessId,
              onSelect: (id) =>
                  ref.read(selectedProcessIdProvider.notifier).state = id,
              onDashboard: () =>
                  ref.read(selectedProcessIdProvider.notifier).state = null,
              showDashboardTab: true,
            ),
          // Content
          Expanded(
            child: selectedProcessId != null
                ? ProcessView(processId: selectedProcessId)
                : const DashboardView(),
          ),
          // Status bar
          _StatusBar(connected: connected, processCount: processes.length),
        ],
      ),
    );
  }

  void _showLaunchDialog(BuildContext context, WidgetRef ref) {
    showDialog(
      context: context,
      builder: (_) => const LaunchDialog(),
    );
  }

  Future<void> _launchPlan(BuildContext context, WidgetRef ref) async {
    final projectInfo = ref.read(projectInfoProvider);
    if (projectInfo == null) return;

    try {
      final api = ref.read(apiClientProvider);
      final id = await api.launchProcess(
        projectInfo.id,
        LaunchRequest(command: 'plan', config: LaunchConfig()),
      );
      ref.read(processesProvider.notifier).refresh();
      ref.read(selectedProcessIdProvider.notifier).state = id;
    } catch (e) {
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Failed to launch plan: $e')),
        );
      }
    }
  }

  void _showSettingsDialog(BuildContext context, WidgetRef ref) {
    showDialog(
      context: context,
      builder: (_) => const SettingsDialog(),
    );
  }
}

class _TopBar extends StatelessWidget {
  final String projectName;
  final String phase;
  final VoidCallback onLaunch;
  final VoidCallback onPlan;
  final VoidCallback onSettings;

  const _TopBar({
    required this.projectName,
    required this.phase,
    required this.onLaunch,
    required this.onPlan,
    required this.onSettings,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 48,
      padding: const EdgeInsets.symmetric(horizontal: 16),
      decoration: const BoxDecoration(
        color: GolemTheme.bgSurface,
        border: Border(bottom: BorderSide(color: GolemTheme.border)),
      ),
      child: Row(
        children: [
          Text(
            projectName,
            style: const TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w600,
              color: GolemTheme.textPrimary,
            ),
          ),
          if (phase.isNotEmpty) ...[
            const SizedBox(width: 8),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
              decoration: BoxDecoration(
                color: GolemTheme.bgElevated,
                borderRadius: BorderRadius.circular(4),
              ),
              child: Text(
                phase,
                style: const TextStyle(
                  fontSize: 11,
                  color: GolemTheme.textSecondary,
                ),
              ),
            ),
          ],
          const Spacer(),
          _ActionButton(
            icon: Icons.add,
            label: 'Launch',
            onPressed: onLaunch,
          ),
          const SizedBox(width: 4),
          _ActionButton(
            icon: Icons.play_arrow,
            label: 'Plan',
            onPressed: onPlan,
            color: GolemTheme.green,
          ),
          const SizedBox(width: 4),
          IconButton(
            icon: const Icon(Icons.settings, size: 18),
            color: GolemTheme.textSecondary,
            onPressed: onSettings,
            tooltip: 'Settings',
            splashRadius: 18,
          ),
        ],
      ),
    );
  }
}

class _ActionButton extends StatelessWidget {
  final IconData icon;
  final String label;
  final VoidCallback onPressed;
  final Color? color;

  const _ActionButton({
    required this.icon,
    required this.label,
    required this.onPressed,
    this.color,
  });

  @override
  Widget build(BuildContext context) {
    return TextButton.icon(
      onPressed: onPressed,
      icon: Icon(icon, size: 16, color: color ?? GolemTheme.textSecondary),
      label: Text(
        label,
        style: TextStyle(
          fontSize: 12,
          color: color ?? GolemTheme.textSecondary,
        ),
      ),
      style: TextButton.styleFrom(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
        minimumSize: Size.zero,
      ),
    );
  }
}

class _ProcessTabs extends StatelessWidget {
  final List<ProcessInfo> processes;
  final String? selectedId;
  final ValueChanged<String> onSelect;
  final VoidCallback onDashboard;
  final bool showDashboardTab;

  const _ProcessTabs({
    required this.processes,
    required this.selectedId,
    required this.onSelect,
    required this.onDashboard,
    required this.showDashboardTab,
  });

  Color _statusColor(String status) {
    switch (status) {
      case 'running':
        return GolemTheme.green;
      case 'failed':
        return GolemTheme.red;
      default:
        return GolemTheme.textSecondary;
    }
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 36,
      decoration: const BoxDecoration(
        border: Border(bottom: BorderSide(color: GolemTheme.border)),
      ),
      child: Row(
        children: [
          ...processes.map((p) {
            final isSelected = p.id == selectedId;
            return GestureDetector(
              onTap: () => onSelect(p.id),
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 12),
                decoration: BoxDecoration(
                  border: Border(
                    bottom: BorderSide(
                      color: isSelected ? GolemTheme.accent : Colors.transparent,
                      width: 2,
                    ),
                  ),
                ),
                child: Row(
                  children: [
                    Container(
                      width: 7,
                      height: 7,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        color: _statusColor(p.status),
                      ),
                    ),
                    const SizedBox(width: 6),
                    Text(
                      p.command,
                      style: TextStyle(
                        fontSize: 12,
                        color: isSelected
                            ? GolemTheme.textPrimary
                            : GolemTheme.textSecondary,
                      ),
                    ),
                  ],
                ),
              ),
            );
          }),
          if (showDashboardTab)
            GestureDetector(
              onTap: onDashboard,
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 12),
                decoration: BoxDecoration(
                  border: Border(
                    bottom: BorderSide(
                      color: selectedId == null
                          ? GolemTheme.accent
                          : Colors.transparent,
                      width: 2,
                    ),
                  ),
                ),
                child: Row(
                  children: [
                    Icon(
                      Icons.dashboard_outlined,
                      size: 14,
                      color: selectedId == null
                          ? GolemTheme.textPrimary
                          : GolemTheme.textSecondary,
                    ),
                    const SizedBox(width: 4),
                    Text(
                      'Dashboard',
                      style: TextStyle(
                        fontSize: 12,
                        color: selectedId == null
                            ? GolemTheme.textPrimary
                            : GolemTheme.textSecondary,
                      ),
                    ),
                  ],
                ),
              ),
            ),
        ],
      ),
    );
  }
}

class _StatusBar extends StatelessWidget {
  final bool connected;
  final int processCount;

  const _StatusBar({required this.connected, required this.processCount});

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 28,
      padding: const EdgeInsets.symmetric(horizontal: 12),
      decoration: const BoxDecoration(
        color: GolemTheme.bgSurface,
        border: Border(top: BorderSide(color: GolemTheme.border)),
      ),
      child: Row(
        children: [
          Container(
            width: 8,
            height: 8,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: connected ? GolemTheme.green : GolemTheme.red,
            ),
          ),
          const SizedBox(width: 8),
          Text(
            connected
                ? 'golem serve \u00B7 $processCount process${processCount != 1 ? "es" : ""}'
                : 'Disconnected \u2014 start golem serve',
            style: const TextStyle(
              fontSize: 11,
              color: GolemTheme.textSecondary,
            ),
          ),
        ],
      ),
    );
  }
}
```

**Step 2: Create placeholder views so it compiles**

Create minimal placeholders for `DashboardView`, `ProcessView`, `LaunchDialog`, `SettingsDialog` (they'll be fully implemented in subsequent tasks).

`ui/flutter/lib/views/dashboard.dart`:

```dart
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
```

`ui/flutter/lib/views/process_view.dart`:

```dart
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
```

`ui/flutter/lib/views/launch_dialog.dart`:

```dart
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
```

`ui/flutter/lib/views/settings_dialog.dart`:

```dart
import 'package:flutter/material.dart';

class SettingsDialog extends StatelessWidget {
  const SettingsDialog({super.key});

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Settings'),
      content: const Text('Coming soon'),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Close')),
      ],
    );
  }
}
```

**Step 3: Verify it builds and runs**

```bash
cd ui/flutter && flutter analyze && flutter build linux --debug
```

**Step 4: Commit**

```bash
git add ui/flutter/lib/views/
git commit -m "feat(ui): add shell view with top bar, process tabs, and status bar"
```

---

### Task 8: Dashboard view

**Files:**
- Modify: `ui/flutter/lib/views/dashboard.dart`

**Step 1: Implement the full dashboard**

Replace `ui/flutter/lib/views/dashboard.dart` with the full implementation. It shows: project summary, task progress bar, task list, recent sessions, decisions, and pitfalls.

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/project.dart';
import '../providers/project.dart';
import '../theme.dart';

class DashboardView extends ConsumerWidget {
  const DashboardView({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(projectStateProvider);
    final sessions = ref.watch(sessionsProvider);

    if (state == null) {
      return const Center(
        child: CircularProgressIndicator(color: GolemTheme.accent),
      );
    }

    final tasks = state.tasks;
    final done = tasks.where((t) => t.status == 'done').length;
    final recentSessions = sessions.length > 5
        ? sessions.sublist(sessions.length - 5).reversed.toList()
        : sessions.reversed.toList();

    return SingleChildScrollView(
      padding: const EdgeInsets.all(24),
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 720),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // Project header
              Text(
                state.project.name,
                style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w600),
              ),
              if (state.project.summary.isNotEmpty) ...[
                const SizedBox(height: 4),
                Text(
                  state.project.summary,
                  style: const TextStyle(fontSize: 13, color: GolemTheme.textSecondary),
                ),
              ],
              const SizedBox(height: 4),
              Row(
                children: [
                  Text(
                    'Stack: ${state.project.stack}',
                    style: const TextStyle(fontSize: 11, color: GolemTheme.textSecondary),
                  ),
                  const SizedBox(width: 16),
                  Text(
                    'Phase: ${state.status.phase}',
                    style: const TextStyle(fontSize: 11, color: GolemTheme.textSecondary),
                  ),
                ],
              ),

              const SizedBox(height: 24),

              // Tasks
              _SectionHeader('Tasks', trailing: '$done/${tasks.length}'),
              const SizedBox(height: 8),
              ClipRRect(
                borderRadius: BorderRadius.circular(4),
                child: LinearProgressIndicator(
                  value: tasks.isEmpty ? 0 : done / tasks.length,
                  backgroundColor: GolemTheme.bgElevated,
                  color: GolemTheme.green,
                  minHeight: 6,
                ),
              ),
              const SizedBox(height: 12),
              ...tasks.map((t) => _TaskRow(task: t)),

              if (recentSessions.isNotEmpty) ...[
                const SizedBox(height: 24),
                _SectionHeader('Recent Sessions'),
                const SizedBox(height: 8),
                ...recentSessions.map((s) => _SessionCard(session: s)),
              ],

              if (state.decisions.isNotEmpty) ...[
                const SizedBox(height: 24),
                _SectionHeader('Decisions', trailing: '${state.decisions.length}'),
                const SizedBox(height: 8),
                ...state.decisions.map((d) => _DecisionRow(decision: d)),
              ],

              if (state.pitfalls.isNotEmpty) ...[
                const SizedBox(height: 24),
                _SectionHeader('Pitfalls', trailing: '${state.pitfalls.length}'),
                const SizedBox(height: 8),
                ...state.pitfalls.map((p) => _PitfallRow(pitfall: p)),
              ],

              const SizedBox(height: 24),
            ],
          ),
        ),
      ),
    );
  }
}

class _SectionHeader extends StatelessWidget {
  final String title;
  final String? trailing;

  const _SectionHeader(this.title, {this.trailing});

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Text(
          title,
          style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600),
        ),
        if (trailing != null) ...[
          const Spacer(),
          Text(
            trailing!,
            style: const TextStyle(fontSize: 12, color: GolemTheme.textSecondary),
          ),
        ],
      ],
    );
  }
}

class _TaskRow extends StatelessWidget {
  final Task task;
  const _TaskRow({required this.task});

  @override
  Widget build(BuildContext context) {
    final (icon, color) = switch (task.status) {
      'done' => ('\u2713', GolemTheme.green),
      'in-progress' => ('\u25D0', GolemTheme.yellow),
      'blocked' => ('\u2717', GolemTheme.red),
      _ => ('\u25CB', GolemTheme.textSecondary),
    };

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 20,
            child: Text(icon, style: TextStyle(color: color, fontSize: 13)),
          ),
          Expanded(
            child: Text(
              task.name,
              style: TextStyle(
                fontSize: 13,
                color: task.status == 'done' ? GolemTheme.textSecondary : GolemTheme.textPrimary,
                decoration: task.status == 'done' ? TextDecoration.lineThrough : null,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _SessionCard extends StatelessWidget {
  final Session session;
  const _SessionCard({required this.session});

  @override
  Widget build(BuildContext context) {
    final outcomeColor = switch (session.outcome) {
      'done' => GolemTheme.green,
      'partial' => GolemTheme.yellow,
      _ => GolemTheme.red,
    };

    return Card(
      margin: const EdgeInsets.only(bottom: 8),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                Expanded(
                  child: Text(
                    '#${session.iteration} \u2014 ${session.task}',
                    style: const TextStyle(fontSize: 13),
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
                Text(
                  session.outcome,
                  style: TextStyle(fontSize: 12, color: outcomeColor),
                ),
              ],
            ),
            if (session.summary.isNotEmpty) ...[
              const SizedBox(height: 4),
              Text(
                session.summary,
                style: const TextStyle(fontSize: 11, color: GolemTheme.textSecondary),
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _DecisionRow extends StatelessWidget {
  final Decision decision;
  const _DecisionRow({required this.decision});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: Text.rich(
        TextSpan(children: [
          TextSpan(
            text: '${decision.when} ',
            style: const TextStyle(fontSize: 12, color: GolemTheme.textSecondary),
          ),
          TextSpan(
            text: decision.what,
            style: const TextStyle(fontSize: 12),
          ),
          TextSpan(
            text: ' \u2014 ${decision.why}',
            style: const TextStyle(fontSize: 12, color: GolemTheme.textSecondary),
          ),
        ]),
      ),
    );
  }
}

class _PitfallRow extends StatelessWidget {
  final Pitfall pitfall;
  const _PitfallRow({required this.pitfall});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: Text.rich(
        TextSpan(children: [
          TextSpan(text: pitfall.what, style: const TextStyle(fontSize: 12)),
          if (pitfall.fix.isNotEmpty)
            TextSpan(
              text: ' \u2014 Fix: ${pitfall.fix}',
              style: const TextStyle(fontSize: 12, color: GolemTheme.textSecondary),
            ),
        ]),
      ),
    );
  }
}
```

**Step 2: Verify**

```bash
cd ui/flutter && flutter analyze && flutter build linux --debug
```

**Step 3: Commit**

```bash
git add ui/flutter/lib/views/dashboard.dart
git commit -m "feat(ui): implement dashboard view with tasks, sessions, decisions"
```

---

### Task 9: Process view (output pane + task panel)

**Files:**
- Modify: `ui/flutter/lib/views/process_view.dart`

**Step 1: Implement the process view**

Replace `ui/flutter/lib/views/process_view.dart`:

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/project.dart' as models;
import '../providers/connection.dart';
import '../providers/project.dart';
import '../providers/processes.dart';
import '../theme.dart';

class ProcessView extends ConsumerWidget {
  final String processId;
  const ProcessView({super.key, required this.processId});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Row(
      children: [
        Expanded(child: _OutputPane(processId: processId)),
        const _TaskPanel(),
      ],
    );
  }
}

class _OutputPane extends ConsumerStatefulWidget {
  final String processId;
  const _OutputPane({required this.processId});

  @override
  ConsumerState<_OutputPane> createState() => _OutputPaneState();
}

class _OutputPaneState extends ConsumerState<_OutputPane> {
  final _scrollController = ScrollController();
  bool _autoScroll = true;

  @override
  void dispose() {
    _scrollController.dispose();
    super.dispose();
  }

  void _scrollToBottom() {
    if (_autoScroll && _scrollController.hasClients) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (_scrollController.hasClients) {
          _scrollController.jumpTo(_scrollController.position.maxScrollExtent);
        }
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final lines = ref.watch(processOutputProvider(widget.processId));

    // Auto-scroll when new lines arrive
    ref.listen(processOutputProvider(widget.processId), (_, __) {
      _scrollToBottom();
    });

    return Container(
      color: GolemTheme.bgPrimary,
      child: NotificationListener<ScrollNotification>(
        onNotification: (notification) {
          if (notification is UserScrollNotification) {
            final pos = _scrollController.position;
            _autoScroll = pos.pixels >= pos.maxScrollExtent - 50;
          }
          return false;
        },
        child: ListView.builder(
          controller: _scrollController,
          padding: const EdgeInsets.all(12),
          itemCount: lines.length,
          itemBuilder: (context, index) {
            return SelectableText.rich(
              TextSpan(
                children: [
                  TextSpan(
                    text: '\u258E ',
                    style: GolemTheme.monoStyle().copyWith(color: GolemTheme.border),
                  ),
                  TextSpan(
                    text: lines[index],
                    style: GolemTheme.monoStyle(),
                  ),
                ],
              ),
            );
          },
        ),
      ),
    );
  }
}

class _TaskPanel extends ConsumerWidget {
  const _TaskPanel();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(projectStateProvider);
    final sessions = ref.watch(sessionsProvider);

    if (state == null) return const SizedBox.shrink();

    final tasks = state.tasks;
    final done = tasks.where((t) => t.status == 'done').length;
    final lastSession = sessions.isNotEmpty ? sessions.last : null;

    return Container(
      width: 220,
      decoration: const BoxDecoration(
        color: GolemTheme.bgSurface,
        border: Border(left: BorderSide(color: GolemTheme.border)),
      ),
      child: Column(
        children: [
          // Header
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
            decoration: const BoxDecoration(
              border: Border(bottom: BorderSide(color: GolemTheme.border)),
            ),
            child: Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                const Text(
                  'TASKS',
                  style: TextStyle(
                    fontSize: 10,
                    fontWeight: FontWeight.w600,
                    letterSpacing: 1,
                    color: GolemTheme.textSecondary,
                  ),
                ),
                Text(
                  '$done/${tasks.length}',
                  style: const TextStyle(fontSize: 11, color: GolemTheme.textSecondary),
                ),
              ],
            ),
          ),
          // Task list
          Expanded(
            child: ListView.builder(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
              itemCount: tasks.length,
              itemBuilder: (context, i) => _TaskItem(task: tasks[i]),
            ),
          ),
          // Stats footer
          Container(
            padding: const EdgeInsets.all(12),
            decoration: const BoxDecoration(
              border: Border(top: BorderSide(color: GolemTheme.border)),
            ),
            child: Column(
              children: [
                _StatRow('Phase', state.status.phase.isNotEmpty ? state.status.phase : '\u2014'),
                _StatRow(
                  'Focus',
                  state.status.currentFocus.isNotEmpty
                      ? state.status.currentFocus
                      : '\u2014',
                ),
                if (lastSession != null) ...[
                  _StatRow('Last iter', '#${lastSession.iteration}'),
                  _StatRow(
                    'Outcome',
                    lastSession.outcome,
                    valueColor: switch (lastSession.outcome) {
                      'done' => GolemTheme.green,
                      'blocked' || 'unproductive' => GolemTheme.red,
                      _ => GolemTheme.yellow,
                    },
                  ),
                ],
                _StatRow('Decisions', '${state.decisions.length}'),
                _StatRow('Pitfalls', '${state.pitfalls.length}'),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _TaskItem extends StatelessWidget {
  final models.Task task;
  const _TaskItem({required this.task});

  @override
  Widget build(BuildContext context) {
    final (icon, color) = switch (task.status) {
      'done' => ('\u2713', GolemTheme.green),
      'in-progress' => ('\u25D0', GolemTheme.yellow),
      'blocked' => ('\u2717', GolemTheme.red),
      _ => ('\u25CB', GolemTheme.textSecondary),
    };

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 16,
            child: Text(icon, style: TextStyle(fontSize: 11, color: color, fontFamily: 'monospace')),
          ),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  task.name,
                  style: const TextStyle(fontSize: 12),
                  overflow: TextOverflow.ellipsis,
                ),
                if (task.blockedReason != null)
                  Text(
                    task.blockedReason!,
                    style: const TextStyle(fontSize: 10, color: GolemTheme.red),
                    overflow: TextOverflow.ellipsis,
                  ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _StatRow extends StatelessWidget {
  final String label;
  final String value;
  final Color? valueColor;

  const _StatRow(this.label, this.value, {this.valueColor});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 1),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Text(label, style: const TextStyle(fontSize: 11, color: GolemTheme.textSecondary)),
          Flexible(
            child: Text(
              value,
              style: TextStyle(fontSize: 11, color: valueColor ?? GolemTheme.textPrimary),
              overflow: TextOverflow.ellipsis,
              textAlign: TextAlign.end,
            ),
          ),
        ],
      ),
    );
  }
}
```

**Step 2: Verify**

```bash
cd ui/flutter && flutter analyze && flutter build linux --debug
```

**Step 3: Commit**

```bash
git add ui/flutter/lib/views/process_view.dart
git commit -m "feat(ui): implement process view with output pane and task panel"
```

---

### Task 10: Launch dialog

**Files:**
- Modify: `ui/flutter/lib/views/launch_dialog.dart`

**Step 1: Implement the full launch dialog**

Replace `ui/flutter/lib/views/launch_dialog.dart`:

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/process.dart';
import '../providers/connection.dart';
import '../providers/project.dart';
import '../providers/processes.dart';
import '../theme.dart';

class LaunchDialog extends ConsumerStatefulWidget {
  const LaunchDialog({super.key});

  @override
  ConsumerState<LaunchDialog> createState() => _LaunchDialogState();
}

class _LaunchDialogState extends ConsumerState<LaunchDialog> {
  String _command = 'code';
  String _model = '';
  int _maxIterations = 20;
  int _maxToolCalls = 200;
  bool _sandbox = false;
  bool _mcp = true;
  int _parallel = 1;
  String _task = '';
  bool _launching = false;
  bool _loaded = false;

  @override
  void initState() {
    super.initState();
    _loadConfig();
  }

  Future<void> _loadConfig() async {
    final projectInfo = ref.read(projectInfoProvider);
    if (projectInfo == null) return;

    try {
      final api = ref.read(apiClientProvider);
      final cfg = await api.getProjectConfig(projectInfo.id);
      if (mounted) {
        setState(() {
          _maxIterations = cfg.maxIterations;
          _maxToolCalls = cfg.maxToolCalls;
          _sandbox = cfg.sandbox;
          _mcp = cfg.mcp;
          _parallel = cfg.parallel;
          if (cfg.model.isNotEmpty) _model = cfg.model;
          _loaded = true;
        });
      }
    } catch (_) {
      if (mounted) setState(() => _loaded = true);
    }
  }

  Future<void> _launch() async {
    final projectInfo = ref.read(projectInfoProvider);
    if (projectInfo == null) return;

    setState(() => _launching = true);

    try {
      final api = ref.read(apiClientProvider);
      final id = await api.launchProcess(
        projectInfo.id,
        LaunchRequest(
          command: _command,
          config: LaunchConfig(
            maxIterations: _maxIterations,
            maxToolCalls: _maxToolCalls,
            model: _model.isNotEmpty ? _model : null,
            task: _task.isNotEmpty ? _task : null,
            sandbox: _sandbox,
            mcp: _mcp,
            parallel: _parallel > 1 ? _parallel : null,
          ),
        ),
      );
      ref.read(processesProvider.notifier).refresh();
      ref.read(selectedProcessIdProvider.notifier).state = id;
      if (mounted) Navigator.pop(context);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Launch failed: $e')),
        );
        setState(() => _launching = false);
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Launch Process', style: TextStyle(fontSize: 16)),
      content: SizedBox(
        width: 380,
        child: !_loaded
            ? const Center(child: CircularProgressIndicator())
            : Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  _dropdown('Command', _command, ['code', 'review', 'qa', 'plan'],
                      (v) => setState(() => _command = v)),
                  const SizedBox(height: 12),
                  _dropdown('Model', _model, ['', 'sonnet', 'opus', 'haiku'],
                      (v) => setState(() => _model = v),
                      labels: {'': 'default'}),
                  const SizedBox(height: 12),
                  Row(
                    children: [
                      Expanded(
                        child: _numberField('Max Iterations', _maxIterations,
                            (v) => setState(() => _maxIterations = v)),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: _numberField('Max Tool Calls', _maxToolCalls,
                            (v) => setState(() => _maxToolCalls = v)),
                      ),
                    ],
                  ),
                  const SizedBox(height: 12),
                  Row(
                    children: [
                      _checkbox('Sandbox', _sandbox, (v) => setState(() => _sandbox = v)),
                      const SizedBox(width: 24),
                      _checkbox('MCP', _mcp, (v) => setState(() => _mcp = v)),
                    ],
                  ),
                  const SizedBox(height: 12),
                  _numberField('Parallel', _parallel, (v) => setState(() => _parallel = v)),
                  const SizedBox(height: 12),
                  TextField(
                    decoration: const InputDecoration(
                      labelText: 'Task Override',
                      hintText: '(optional)',
                      labelStyle: TextStyle(fontSize: 12, color: GolemTheme.textSecondary),
                    ),
                    style: const TextStyle(fontSize: 13),
                    onChanged: (v) => _task = v,
                  ),
                ],
              ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Cancel'),
        ),
        FilledButton(
          onPressed: _launching ? null : _launch,
          child: Text(_launching ? 'Launching...' : 'Launch'),
        ),
      ],
    );
  }

  Widget _dropdown(String label, String value, List<String> items,
      ValueChanged<String> onChanged,
      {Map<String, String>? labels}) {
    return DropdownButtonFormField<String>(
      value: value,
      decoration: InputDecoration(
        labelText: label,
        labelStyle: const TextStyle(fontSize: 12, color: GolemTheme.textSecondary),
      ),
      dropdownColor: GolemTheme.bgElevated,
      style: const TextStyle(fontSize: 13, color: GolemTheme.textPrimary),
      items: items
          .map((v) => DropdownMenuItem(value: v, child: Text(labels?[v] ?? v)))
          .toList(),
      onChanged: (v) {
        if (v != null) onChanged(v);
      },
    );
  }

  Widget _numberField(String label, int value, ValueChanged<int> onChanged) {
    return TextFormField(
      initialValue: value.toString(),
      decoration: InputDecoration(
        labelText: label,
        labelStyle: const TextStyle(fontSize: 12, color: GolemTheme.textSecondary),
      ),
      style: const TextStyle(fontSize: 13),
      keyboardType: TextInputType.number,
      onChanged: (v) => onChanged(int.tryParse(v) ?? value),
    );
  }

  Widget _checkbox(String label, bool value, ValueChanged<bool> onChanged) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        SizedBox(
          height: 20,
          width: 20,
          child: Checkbox(
            value: value,
            onChanged: (v) => onChanged(v ?? false),
            activeColor: GolemTheme.accent,
          ),
        ),
        const SizedBox(width: 6),
        Text(label, style: const TextStyle(fontSize: 13)),
      ],
    );
  }
}
```

**Step 2: Verify**

```bash
cd ui/flutter && flutter analyze && flutter build linux --debug
```

**Step 3: Commit**

```bash
git add ui/flutter/lib/views/launch_dialog.dart
git commit -m "feat(ui): implement launch dialog with config form"
```

---

### Task 11: Settings dialog

**Files:**
- Modify: `ui/flutter/lib/views/settings_dialog.dart`

**Step 1: Implement the full settings dialog**

Replace `ui/flutter/lib/views/settings_dialog.dart`:

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/process.dart';
import '../providers/connection.dart';
import '../providers/project.dart';
import '../theme.dart';

class SettingsDialog extends ConsumerStatefulWidget {
  const SettingsDialog({super.key});

  @override
  ConsumerState<SettingsDialog> createState() => _SettingsDialogState();
}

class _SettingsDialogState extends ConsumerState<SettingsDialog>
    with SingleTickerProviderStateMixin {
  late TabController _tabController;
  GolemConfig? _config;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 2, vsync: this);
    _tabController.addListener(() {
      if (!_tabController.indexIsChanging) _loadConfig();
    });
    _loadConfig();
  }

  @override
  void dispose() {
    _tabController.dispose();
    super.dispose();
  }

  Future<void> _loadConfig() async {
    setState(() => _config = null);
    final api = ref.read(apiClientProvider);
    try {
      if (_tabController.index == 0) {
        final projectInfo = ref.read(projectInfoProvider);
        if (projectInfo != null) {
          _config = await api.getProjectConfig(projectInfo.id);
        }
      } else {
        _config = await api.getGlobalConfig();
      }
      if (mounted) setState(() {});
    } catch (_) {
      if (mounted) setState(() => _config = GolemConfig());
    }
  }

  Future<void> _save() async {
    if (_config == null) return;
    setState(() => _saving = true);

    try {
      final api = ref.read(apiClientProvider);
      if (_tabController.index == 0) {
        final projectInfo = ref.read(projectInfoProvider);
        if (projectInfo != null) {
          await api.updateProjectConfig(projectInfo.id, _config!);
        }
      } else {
        await api.updateGlobalConfig(_config!);
      }
      if (mounted) Navigator.pop(context);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Save failed: $e')),
        );
        setState(() => _saving = false);
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Settings', style: TextStyle(fontSize: 16)),
      content: SizedBox(
        width: 420,
        height: 360,
        child: Column(
          children: [
            TabBar(
              controller: _tabController,
              tabs: const [Tab(text: 'Project'), Tab(text: 'Global')],
            ),
            const SizedBox(height: 16),
            Expanded(
              child: _config == null
                  ? const Center(child: CircularProgressIndicator())
                  : _ConfigForm(
                      config: _config!,
                      onChanged: (cfg) => setState(() => _config = cfg),
                    ),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Cancel'),
        ),
        FilledButton(
          onPressed: _saving || _config == null ? null : _save,
          child: Text(_saving ? 'Saving...' : 'Save'),
        ),
      ],
    );
  }
}

class _ConfigForm extends StatelessWidget {
  final GolemConfig config;
  final ValueChanged<GolemConfig> onChanged;

  const _ConfigForm({required this.config, required this.onChanged});

  GolemConfig _copy() => GolemConfig(
        maxIterations: config.maxIterations,
        maxToolCalls: config.maxToolCalls,
        verbose: config.verbose,
        sandbox: config.sandbox,
        mcp: config.mcp,
        parallel: config.parallel,
        model: config.model,
      );

  @override
  Widget build(BuildContext context) {
    return SingleChildScrollView(
      child: Column(
        children: [
          Row(
            children: [
              Expanded(
                child: TextFormField(
                  initialValue: config.maxIterations.toString(),
                  decoration: const InputDecoration(labelText: 'max-iterations'),
                  keyboardType: TextInputType.number,
                  onChanged: (v) {
                    final c = _copy();
                    c.maxIterations = int.tryParse(v) ?? 20;
                    onChanged(c);
                  },
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: TextFormField(
                  initialValue: config.maxToolCalls.toString(),
                  decoration: const InputDecoration(labelText: 'max-tool-calls'),
                  keyboardType: TextInputType.number,
                  onChanged: (v) {
                    final c = _copy();
                    c.maxToolCalls = int.tryParse(v) ?? 200;
                    onChanged(c);
                  },
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          DropdownButtonFormField<String>(
            value: config.model,
            decoration: const InputDecoration(labelText: 'model'),
            dropdownColor: GolemTheme.bgElevated,
            items: const [
              DropdownMenuItem(value: '', child: Text('default')),
              DropdownMenuItem(value: 'sonnet', child: Text('sonnet')),
              DropdownMenuItem(value: 'opus', child: Text('opus')),
              DropdownMenuItem(value: 'haiku', child: Text('haiku')),
            ],
            onChanged: (v) {
              final c = _copy();
              c.model = v ?? '';
              onChanged(c);
            },
          ),
          const SizedBox(height: 12),
          Row(
            children: [
              _check('verbose', config.verbose, (v) {
                final c = _copy();
                c.verbose = v;
                onChanged(c);
              }),
              const SizedBox(width: 24),
              _check('sandbox', config.sandbox, (v) {
                final c = _copy();
                c.sandbox = v;
                onChanged(c);
              }),
              const SizedBox(width: 24),
              _check('mcp', config.mcp, (v) {
                final c = _copy();
                c.mcp = v;
                onChanged(c);
              }),
            ],
          ),
          const SizedBox(height: 12),
          TextFormField(
            initialValue: config.parallel.toString(),
            decoration: const InputDecoration(labelText: 'parallel'),
            keyboardType: TextInputType.number,
            onChanged: (v) {
              final c = _copy();
              c.parallel = int.tryParse(v) ?? 1;
              onChanged(c);
            },
          ),
        ],
      ),
    );
  }

  Widget _check(String label, bool value, ValueChanged<bool> onChanged) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        SizedBox(
          height: 20,
          width: 20,
          child: Checkbox(
            value: value,
            onChanged: (v) => onChanged(v ?? false),
            activeColor: GolemTheme.accent,
          ),
        ),
        const SizedBox(width: 6),
        Text(label, style: const TextStyle(fontSize: 13)),
      ],
    );
  }
}
```

**Step 2: Verify**

```bash
cd ui/flutter && flutter analyze && flutter build linux --debug
```

**Step 3: Commit**

```bash
git add ui/flutter/lib/views/settings_dialog.dart
git commit -m "feat(ui): implement settings dialog with project/global tabs"
```

---

### Task 12: Build, integrate, and verify

**Files:**
- Modify: `Makefile` (add `ui` target)

**Step 1: Build the release binary**

```bash
cd ui/flutter && flutter build linux --release
```
Expected: Binary at `build/linux/x64/release/bundle/golem_ui`

**Step 2: Create a symlink or copy script**

The Flutter build produces a bundle directory (binary + libraries). We need to handle this differently from a single binary. Create a wrapper script:

Add to `Makefile`:

```makefile
.PHONY: ui
ui:
	cd ui/flutter && flutter build linux --release
	rm -rf $(HOME)/go/bin/golem-ui-bundle
	cp -r ui/flutter/build/linux/x64/release/bundle $(HOME)/go/bin/golem-ui-bundle
	ln -sf $(HOME)/go/bin/golem-ui-bundle/golem_ui $(HOME)/go/bin/golem-ui
```

**Step 3: Update findAppBinary to also check golem_ui (underscore variant)**

In `cmd/ui.go`, update `findAppBinary()` to also check the underscore variant that Flutter produces:

```go
func findAppBinary() string {
	names := []string{"golem-ui", "golem_ui"}

	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, name := range names {
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}

	return ""
}
```

**Step 4: Build and install everything**

```bash
make ui && go install .
```

**Step 5: Kill old server and test**

```bash
pkill -f "golem ui" || true
cd /home/winler/projects/TROGUE && golem ui
```
Expected: Server starts, Flutter desktop app launches, shows TROGUE dashboard.

**Step 6: Commit**

```bash
git add Makefile cmd/ui.go
git commit -m "feat(ui): add Makefile target and update findAppBinary for Flutter"
```

---

### Task 13: Clean up old Tauri+React code

**Files:**
- Delete: `ui/src-tauri/` (entire directory)
- Delete: `ui/src/` (entire directory)
- Delete: `ui/node_modules/` (if present)
- Delete: `ui/package.json`, `ui/package-lock.json`, `ui/tsconfig*.json`, `ui/vite.config.ts`, `ui/index.html`

**Step 1: Remove old UI files**

```bash
rm -rf ui/src-tauri ui/src ui/node_modules ui/dist
rm -f ui/package.json ui/package-lock.json ui/tsconfig.json ui/tsconfig.node.json ui/vite.config.ts ui/index.html ui/.gitignore
```

**Step 2: Commit**

```bash
git add -A ui/
git commit -m "chore(ui): remove old Tauri+React code"
```
