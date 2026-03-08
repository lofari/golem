import 'dart:async';
import 'dart:convert';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:xterm/xterm.dart';
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

// Terminal instance per process
final processTerminalProvider =
    StateNotifierProvider.family<ProcessTerminalNotifier, Terminal, String>((ref, processId) {
  final projectInfo = ref.read(projectInfoProvider);
  final api = ref.read(apiClientProvider);
  return ProcessTerminalNotifier(api, projectInfo?.id, processId);
});

class ProcessTerminalNotifier extends StateNotifier<Terminal> {
  final GolemApiClient _api;
  GolemWebSocket? _ws;
  bool _exited = false;

  ProcessTerminalNotifier(this._api, String? projectId, String processId)
      : super(Terminal()) {
    if (projectId != null) {
      _ws = GolemWebSocket(
        url: _api.processStreamUrl(projectId, processId),
        onMessage: (data) {
          switch (data['type']) {
            case 'output':
              final bytes = base64Decode(data['data'] as String);
              state.write(utf8.decode(bytes, allowMalformed: true));
            case 'exit':
              _exited = true;
          }
        },
      );
      _ws!.connect();

      // Forward terminal input to server
      state.onOutput = (data) {
        if (!_exited) {
          _ws?.send({
            'type': 'input',
            'data': base64Encode(utf8.encode(data)),
          });
        }
      };
    }
  }

  bool get exited => _exited;

  void sendResize(int cols, int rows) {
    _ws?.send({'type': 'resize', 'cols': cols, 'rows': rows});
  }

  @override
  void dispose() {
    _ws?.dispose();
    super.dispose();
  }
}
