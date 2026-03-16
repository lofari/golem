import 'package:file_picker/file_picker.dart';
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
  String _pluginDir = '';
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
          if (cfg.pluginDir.isNotEmpty) _pluginDir = cfg.pluginDir;
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
            pluginDir: _pluginDir.isNotEmpty ? _pluginDir : null,
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
                  _dropdown('Command', _command, ['code', 'review', 'qa', 'plan', 'setup'],
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
                  const SizedBox(height: 12),
                  Row(
                    children: [
                      Expanded(
                        child: Text(
                          _pluginDir.isEmpty ? 'No plugin directory' : _pluginDir,
                          style: TextStyle(
                            fontSize: 12,
                            color: _pluginDir.isEmpty
                                ? GolemTheme.textSecondary
                                : GolemTheme.textPrimary,
                            fontFamily: 'JetBrains Mono',
                          ),
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                      const SizedBox(width: 8),
                      if (_pluginDir.isNotEmpty)
                        IconButton(
                          icon: const Icon(Icons.clear, size: 16),
                          color: GolemTheme.textSecondary,
                          onPressed: () => setState(() => _pluginDir = ''),
                          tooltip: 'Clear',
                          splashRadius: 14,
                          padding: EdgeInsets.zero,
                          constraints: const BoxConstraints(minWidth: 28, minHeight: 28),
                        ),
                      IconButton(
                        icon: const Icon(Icons.folder_open, size: 16),
                        color: GolemTheme.accent,
                        onPressed: () async {
                          final result = await FilePicker.platform.getDirectoryPath(
                            dialogTitle: 'Select Plugin Directory',
                          );
                          if (result != null) setState(() => _pluginDir = result);
                        },
                        tooltip: 'Browse',
                        splashRadius: 14,
                        padding: EdgeInsets.zero,
                        constraints: const BoxConstraints(minWidth: 28, minHeight: 28),
                      ),
                    ],
                  ),
                  Padding(
                    padding: const EdgeInsets.only(top: 4),
                    child: Text(
                      'plugin-dir',
                      style: TextStyle(fontSize: 10, color: GolemTheme.textSecondary),
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
      initialValue: value,
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
