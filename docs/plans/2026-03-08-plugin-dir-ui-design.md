# Plugin Directory UI Configuration — Design

**Goal:** Let users configure a plugin directory from the Flutter UI, applied to all Claude sessions (code, plan, qa, review).

## Architecture

### Server side

Add `PluginDir string` to the server's `LaunchConfig` struct in `internal/server/process.go`. When building the Claude command, pass `--plugin-dir <path>` if set. The config API already supports reading/writing `plugin-dir` via `golem config set/get`.

### Flutter UI — Settings dialog

Add a "Plugin directory" row with the current path (or "None") and a folder picker button. Use the `file_picker` package for native directory browsing. Save to project config via the existing config API (`plugin-dir` key). Clearing the field removes the config entry.

### Flutter UI — Launch dialog

Add a "Plugin directory" row matching the settings pattern (path display + folder picker). Pre-populated from project config, overridable per launch. Passed in the launch request JSON as `"plugin_dir": "/path/to/plugin"`.

### Data flow

```
Settings dialog → config API → .ctx/config.yaml (default for all launches)
Launch dialog → launch request JSON → server → ClaudeRunner → claude --plugin-dir /path
```

## Constraints

- Single plugin directory (not multiple)
- CLI `--plugin-dir` flag behavior unchanged
- Config merge logic (project overrides global) unchanged
- Sandbox read-only mounting unchanged
