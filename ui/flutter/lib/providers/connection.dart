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
