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

  // Graph
  Future<Map<String, dynamic>> getGraphStats(String projectId) async {
    return _getJson('/api/projects/$projectId/graph/stats');
  }

  Future<List<dynamic>> graphSearch(
      String projectId, String query,
      {int limit = 10, List<String>? types}) async {
    final body = <String, dynamic>{'query': query, 'limit': limit};
    if (types != null && types.isNotEmpty) body['types'] = types;
    final resp = await _http.post(
      Uri.parse('$baseUrl/api/projects/$projectId/graph/search'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode(body),
    );
    if (resp.statusCode >= 400) {
      final b = jsonDecode(resp.body) as Map<String, dynamic>;
      throw ApiException(b['error'] as String? ?? 'Search failed');
    }
    return jsonDecode(resp.body) as List<dynamic>;
  }

  Future<Map<String, dynamic>> graphRelated(
      String projectId, String name,
      {String direction = 'all', int depth = 1}) async {
    return _getJson(
        '/api/projects/$projectId/graph/related?name=${Uri.encodeComponent(name)}&direction=$direction&depth=$depth');
  }

  Future<Map<String, dynamic>> getContextMap(
      String projectId, String task,
      {int limit = 15}) async {
    return _getJson(
        '/api/projects/$projectId/graph/context-map?task=${Uri.encodeComponent(task)}&limit=$limit');
  }

  // Diff
  Future<Map<String, dynamic>> getDiff(String projectId, {String? ref}) async {
    final query = ref != null ? '?ref=${Uri.encodeComponent(ref)}' : '';
    return _getJson('/api/projects/$projectId/diff$query');
  }

  Future<String> getFilePatch(String projectId, String filePath,
      {String? ref}) async {
    final refQuery = ref != null ? '&ref=${Uri.encodeComponent(ref)}' : '';
    final json = await _getJson(
        '/api/projects/$projectId/diff?file=${Uri.encodeComponent(filePath)}$refQuery');
    return json['patch'] as String? ?? '';
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
