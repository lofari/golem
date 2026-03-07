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
