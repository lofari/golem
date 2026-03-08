import 'package:file_picker/file_picker.dart';
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
        pluginDir: config.pluginDir,
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
            initialValue: config.model,
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
          const SizedBox(height: 12),
          Row(
            children: [
              Expanded(
                child: Text(
                  config.pluginDir.isEmpty ? 'No plugin directory' : config.pluginDir,
                  style: TextStyle(
                    fontSize: 12,
                    color: config.pluginDir.isEmpty
                        ? GolemTheme.textSecondary
                        : GolemTheme.textPrimary,
                    fontFamily: 'JetBrains Mono',
                  ),
                  overflow: TextOverflow.ellipsis,
                ),
              ),
              const SizedBox(width: 8),
              if (config.pluginDir.isNotEmpty)
                IconButton(
                  icon: const Icon(Icons.clear, size: 16),
                  color: GolemTheme.textSecondary,
                  onPressed: () {
                    final c = _copy();
                    c.pluginDir = '';
                    onChanged(c);
                  },
                  tooltip: 'Clear plugin directory',
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
                  if (result != null) {
                    final c = _copy();
                    c.pluginDir = result;
                    onChanged(c);
                  }
                },
                tooltip: 'Browse for plugin directory',
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
