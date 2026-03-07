# Flutter Desktop UI for Golem

## Summary

Replace the Tauri+React desktop app with a Flutter desktop app. The Go API server stays unchanged. Flutter talks to it via HTTP + WebSocket at `localhost:8314`.

**Why:** The Tauri+React setup has webview rendering issues (blank screens from JSON serialization mismatches, Gtk crashes on WSL2, stale embedded builds). Flutter renders natively without a webview, has hot reload, and Material 3 built-in.

## Architecture

- **Flutter desktop app** (Linux target) at `ui/flutter/`
- **Go server** at `:8314` unchanged — REST + WebSocket
- **State management:** Riverpod
- **UI framework:** Material 3 with dark theme
- **Single-project UI** — `golem ui` registers the cwd, no project picker needed

## Project Structure

```
ui/flutter/
├── lib/
│   ├── main.dart
│   ├── api/
│   │   ├── client.dart          # REST client (http package)
│   │   └── websocket.dart       # WebSocket manager with reconnect
│   ├── models/
│   │   ├── project.dart         # ProjectInfo, State, Session, Task, etc.
│   │   └── process.dart         # ProcessInfo, LaunchConfig, GolemConfig
│   ├── providers/
│   │   ├── connection.dart      # Server health polling
│   │   ├── projects.dart        # Project info (single project)
│   │   ├── project_state.dart   # State/log for the project
│   │   └── processes.dart       # Process list + output streams
│   └── views/
│       ├── shell.dart           # Main scaffold: top bar + content
│       ├── dashboard.dart       # Project overview (tasks, sessions, decisions, pitfalls)
│       ├── process_view.dart    # Output pane + task panel
│       ├── launch_dialog.dart   # Launch process config form
│       └── settings_dialog.dart # Config editor (project + global)
├── linux/                       # Flutter linux runner (auto-generated)
├── pubspec.yaml
└── analysis_options.yaml
```

## Layout

### No processes running — Dashboard view:

```
┌─────────────────────────────────────────────────┐
│  TROGUE · building    [+ Launch] [▶ Plan] [⚙]  │
├─────────────────────────────────────────────────┤
│                                                 │
│              Dashboard View                     │
│     (tasks, sessions, decisions, pitfalls)       │
│                                                 │
├─────────────────────────────────────────────────┤
│ ● connected · 0 processes                       │
└─────────────────────────────────────────────────┘
```

### Processes running — Tabbed view:

```
┌─────────────────────────────────────────────────┐
│  TROGUE · building    [+ Launch] [▶ Plan] [⚙]  │
├─────────────────────────────────────────────────┤
│  ● code  │  ● plan  │  📊 Dashboard             │
├───────────────────────────────┬─────────────────┤
│                               │  Tasks  12/40   │
│  > Iteration 3/20            │  ✓ Task 1       │
│  > Working on Task 4...      │  ◑ Task 2       │
│  > Modified src/foo.kt       │  ○ Task 3       │
│                               ├─────────────────┤
│                               │ Phase: build    │
│                               │ Focus: ...      │
│                               │ Decisions: 6    │
├───────────────────────────────┴─────────────────┤
│ ● connected · 2 processes                       │
└─────────────────────────────────────────────────┘
```

- **Top bar:** Project name + phase, action buttons (Launch, Plan, Settings)
- **Process tabs** appear when processes exist; Dashboard becomes a tab
- **Task panel** on the right shows task list + project status, always visible when a process tab is selected
- **Output pane** streams process output via WebSocket, monospace font, auto-scroll
- **Status bar** at bottom shows connection state and process count

## Actions

- **Launch button** — opens dialog: pick command (code/review/qa/plan), model, max iterations, max tool calls, sandbox, MCP, parallel, task override. Starts process, opens output tab.
- **Plan button** — immediately launches a `plan` process, opens its output tab. No dialog.
- **Settings button** — opens dialog with project/global config tabs.
- **Stop button** — on each process tab, kills the process.

## Data Flow

| Provider | Source | Refresh |
|---|---|---|
| `connectionProvider` | `GET /api/health` | Poll 5s |
| `projectProvider` | `GET /api/projects` (pick first) | Once at startup |
| `projectStateProvider` | `GET /api/projects/{id}/state` | WebSocket `state_changed` |
| `sessionsProvider` | `GET /api/projects/{id}/log` | WebSocket `log_appended` |
| `processesProvider` | `GET /api/projects/{id}/processes` | Poll 5s |
| `processOutputProvider(id)` | WebSocket `/api/.../stream` | Real-time stream |

WebSocket reconnection: retry after 2s, exponential backoff to 30s.

## Theme

Material 3 dark theme with custom `ColorScheme`:
- Background: `#0d1117` (primary), `#161b22` (surface), `#1c2128` (elevated)
- Accent: `#58a6ff`
- Green/yellow/red for status indicators
- Monospace font (JetBrains Mono via google_fonts) for output pane

## Dependencies

```yaml
dependencies:
  flutter:
    sdk: flutter
  http: ^1.2.0
  web_socket_channel: ^3.0.0
  flutter_riverpod: ^2.6.0
  google_fonts: ^6.2.0
```

## Build & Integration

- `flutter build linux --release` produces binary at `build/linux/x64/release/bundle/golem-ui`
- `cmd/ui.go` `findAppBinary()` already searches for `golem-ui` — no Go changes needed
- Makefile target: `make ui` builds and copies to `~/go/bin/golem-ui`
- Old Tauri+React code in `ui/src-tauri/` and `ui/src/` can be deleted after Flutter UI is working
