# Plugin Directory UI Configuration — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add plugin directory configuration to the Flutter UI so users can select a local plugin folder that gets passed to all Claude sessions.

**Architecture:** The Go config already supports `plugin-dir` as `[]string`. We add it to the server's `LaunchConfig`, the Flutter `GolemConfig` and `LaunchConfig` models, and the Settings/Launch dialogs with a native directory picker via `file_picker` package.

**Tech Stack:** Go (server), Flutter/Dart (UI), file_picker (native directory browser)

---

### Task 1: Add file_picker dependency to Flutter

**Files:**
- Modify: `ui/flutter/pubspec.yaml`

**Step 1: Add dependency**

Add `file_picker: ^8.0.0` under `dependencies:` in pubspec.yaml (after `google_fonts`).

**Step 2: Install**

Run: `cd ui/flutter && flutter pub get`
Expected: Resolving dependencies... Got dependencies!

**Step 3: Commit**

```bash
git add ui/flutter/pubspec.yaml ui/flutter/pubspec.lock
git commit -m "feat(ui): add file_picker dependency"
```

---

### Task 2: Add pluginDir to Flutter models

**Files:**
- Modify: `ui/flutter/lib/models/process.dart`

**Step 1: Add pluginDir to GolemConfig**

Add `String pluginDir;` field to `GolemConfig` class (after `model`):

```dart
class GolemConfig {
  int maxIterations;
  int maxToolCalls;
  bool verbose;
  bool sandbox;
  bool mcp;
  int parallel;
  String model;
  String pluginDir;  // <-- add this

  GolemConfig({
    this.maxIterations = 20,
    this.maxToolCalls = 200,
    this.verbose = false,
    this.sandbox = false,
    this.mcp = true,
    this.parallel = 1,
    this.model = '',
    this.pluginDir = '',  // <-- add this
  });

  factory GolemConfig.fromJson(Map<String, dynamic> json) => GolemConfig(
        maxIterations: json['max-iterations'] as int? ?? 20,
        maxToolCalls: json['max-tool-calls'] as int? ?? 200,
        verbose: json['verbose'] as bool? ?? false,
        sandbox: json['sandbox'] as bool? ?? false,
        mcp: json['mcp'] as bool? ?? true,
        parallel: json['parallel'] as int? ?? 1,
        model: json['model'] as String? ?? '',
        pluginDir: _firstPluginDir(json['plugin-dir']),  // <-- add this
      );

  // Extract first plugin dir from the config's string list
  static String _firstPluginDir(dynamic v) {
    if (v is List && v.isNotEmpty) return v.first.toString();
    if (v is String) return v;
    return '';
  }

  Map<String, dynamic> toJson() => {
        'max-iterations': maxIterations,
        'max-tool-calls': maxToolCalls,
        'verbose': verbose,
        'sandbox': sandbox,
        'mcp': mcp,
        'parallel': parallel,
        'model': model,
        if (pluginDir.isNotEmpty) 'plugin-dir': [pluginDir],  // <-- add this
      };
}
```

**Step 2: Add pluginDir to LaunchConfig**

Add `String? pluginDir;` field to `LaunchConfig`:

```dart
class LaunchConfig {
  final int? maxIterations;
  final int? maxToolCalls;
  final String? model;
  final String? task;
  final bool sandbox;
  final bool mcp;
  final int? parallel;
  final String? pluginDir;  // <-- add this

  LaunchConfig({
    this.maxIterations,
    this.maxToolCalls,
    this.model,
    this.task,
    this.sandbox = false,
    this.mcp = true,
    this.parallel,
    this.pluginDir,  // <-- add this
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
    if (pluginDir != null && pluginDir!.isNotEmpty) map['pluginDir'] = pluginDir;  // <-- add this
    return map;
  }
}
```

**Step 3: Verify build**

Run: `cd ui/flutter && flutter analyze`
Expected: No issues found

**Step 4: Commit**

```bash
git add ui/flutter/lib/models/process.dart
git commit -m "feat(ui): add pluginDir field to GolemConfig and LaunchConfig models"
```

---

### Task 3: Add pluginDir to server LaunchConfig

**Files:**
- Modify: `internal/server/process.go:24-32`

**Step 1: Add field to LaunchConfig struct**

Add `PluginDir string` to the server's `LaunchConfig` struct:

```go
type LaunchConfig struct {
	MaxIterations int    `json:"maxIterations,omitempty"`
	MaxToolCalls  int    `json:"maxToolCalls,omitempty"`
	Model         string `json:"model,omitempty"`
	Task          string `json:"task,omitempty"`
	Sandbox       bool   `json:"sandbox,omitempty"`
	MCP           bool   `json:"mcp,omitempty"`
	Parallel      int    `json:"parallel,omitempty"`
	PluginDir     string `json:"pluginDir,omitempty"`
}
```

**Step 2: Pass plugin-dir flag in launchProcess**

In `launchProcess()`, after the `Parallel` block (after line 141), add:

```go
if req.Config.PluginDir != "" {
	args = append(args, "--plugin-dir", req.Config.PluginDir)
}
```

**Step 3: Run Go tests**

Run: `go test ./internal/server/ -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/server/process.go
git commit -m "feat(server): pass plugin-dir from launch config to golem subprocess"
```

---

### Task 4: Add plugin directory picker to Settings dialog

**Files:**
- Modify: `ui/flutter/lib/views/settings_dialog.dart`

**Step 1: Add file_picker import**

Add at top of file:

```dart
import 'package:file_picker/file_picker.dart';
```

**Step 2: Add pluginDir to _ConfigForm._copy()**

Update the `_copy()` method to include `pluginDir`:

```dart
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
```

**Step 3: Add plugin directory row to _ConfigForm.build()**

After the `parallel` TextFormField (after line 218), add:

```dart
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
```

**Step 4: Verify build**

Run: `cd ui/flutter && flutter analyze`
Expected: No issues found

**Step 5: Commit**

```bash
git add ui/flutter/lib/views/settings_dialog.dart
git commit -m "feat(ui): add plugin directory picker to settings dialog"
```

---

### Task 5: Add plugin directory picker to Launch dialog

**Files:**
- Modify: `ui/flutter/lib/views/launch_dialog.dart`

**Step 1: Add file_picker import**

Add at top of file:

```dart
import 'package:file_picker/file_picker.dart';
```

**Step 2: Add state variable**

In `_LaunchDialogState`, add after `_task`:

```dart
String _pluginDir = '';
```

**Step 3: Load from config**

In `_loadConfig()`, inside the `setState` block (after line 48), add:

```dart
if (cfg.pluginDir.isNotEmpty) _pluginDir = cfg.pluginDir;
```

**Step 4: Pass in launch request**

In `_launch()`, update the `LaunchConfig` constructor (around line 69) to include:

```dart
pluginDir: _pluginDir.isNotEmpty ? _pluginDir : null,
```

**Step 5: Add UI row**

After the Task Override TextField (after line 143), add:

```dart
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
```

**Step 6: Verify build**

Run: `cd ui/flutter && flutter analyze`
Expected: No issues found

**Step 7: Commit**

```bash
git add ui/flutter/lib/views/launch_dialog.dart
git commit -m "feat(ui): add plugin directory picker to launch dialog"
```

---

### Task 6: Integration testing

**Step 1: Run all Go tests**

Run: `go test ./... -v`
Expected: PASS

**Step 2: Run Flutter analysis**

Run: `cd ui/flutter && flutter analyze`
Expected: No issues found

**Step 3: Build Go binary**

Run: `go build ./...`
Expected: Clean build

**Step 4: Commit any fixes**

```bash
git add -A
git commit -m "test: integration verification for plugin-dir UI"
```
