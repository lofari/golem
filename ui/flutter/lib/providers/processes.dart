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
