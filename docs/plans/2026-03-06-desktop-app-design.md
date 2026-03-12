# Golem Desktop App Design

## Overview

A desktop application for golem that provides a polished, developer-dark UI for managing and monitoring multiple golem processes. The app wraps the existing CLI — golem's core logic remains in Go. The UI is a pure display and orchestration layer.

**Architecture:** Go server (`golem serve`) + Tauri 2 desktop shell + React frontend.

## Motivation

Running golem today means multiple terminals — one per `golem code`, `golem review`, `golem qa`. There's no unified view of project state across processes, no visual feedback beyond streaming text, and settings require CLI flags or manual YAML editing.

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│  Tauri 2 Desktop Shell                                   │
│  ┌────────────────────────────────────────────────────┐  │
│  │  React Frontend (TypeScript + Tailwind)            │  │
│  │  - Project sidebar                                 │  │
│  │  - Process tabs (code/review/qa)                   │  │
│  │  - Output pane (streaming)                         │  │
│  │  - Task panel                                      │  │
│  │  - Settings/Launch dialogs                         │  │
│  └──────────────────┬─────────────────────────────────┘  │
│                     │ HTTP + WebSocket                    │
└─────────────────────┼────────────────────────────────────┘
                      │
┌─────────────────────┼────────────────────────────────────┐
│  golem serve        │                                     │
│  (Go HTTP server)   │                                     │
│  - REST API for projects, config, process management     │
│  - WebSocket for output streaming + state watching       │
│  - Spawns golem CLI commands as child processes          │
│  - Watches .ctx/ files via fsnotify                      │
└──────────┬───────────────────────┬───────────────────────┘
           │ spawns                │ watches
┌──────────┴──────┐    ┌──────────┴──────┐
│ golem code      │    │ .ctx/           │
│ golem review    │    │  state.yaml     │
│ golem qa        │    │  log.yaml       │
│ (existing CLI)  │    │  config.yaml    │
└─────────────────┘    └─────────────────┘
```

The desktop app never modifies state directly. It launches golem processes and observes what they do.

## UI Layout

```
┌─────────────────────────────────────────────────────────────────┐
│ ■ Golem                                            ⚙  _ □ ✕   │
├────────┬────────────────────────────────────────────────────────┤
│        │  ┌──────┐ ┌──────┐ ┌──────┐  ┌───┐                   │
│ myapp  │  │ code │ │review│ │  qa  │  │ + │  ← start new run  │
│ ●●○    │  └──┬───┘ └──────┘ └──────┘  └───┘                   │
│        │     │                                                  │
│ other  │  ┌──┴──────────────────────────┬──────────────────┐   │
│ ○○○    │  │                             │                  │   │
│        │  │   Stream Output             │   Tasks     3/6  │   │
│        │  │                             │                  │   │
│        │  │   ▎ Reading state.yaml      │   ✓ auth module  │   │
│        │  │   ▎ Picking: price charts   │   ✓ user model   │   │
│        │  │   ▎ Writing component...    │   ✓ price fetch  │   │
│        │  │   ▎ Running tests...        │   ◐ price chart  │   │
│        │  │   ▎ PASS                    │   ○ notifs       │   │
│        │  │   ▎                         │   ✗ shipping     │   │
│        │  │                             │                  │   │
│        │  │                             ├──────────────────┤   │
│        │  │                             │ Iter 4/20  6:12  │   │
│        │  │                             │ Phase: building  │   │
│        │  │                             │ Strategy: cont.  │   │
│        │  ├─────────────────────────────┴──────────────────┤   │
│        │  │ ● code running · iter 4/20 · price charts      │   │
│        │  └────────────────────────────────────────────────┘   │
├────────┴────────────────────────────────────────────────────────┤
│ golem serve :8314 · 3 processes · myapp                        │
└─────────────────────────────────────────────────────────────────┘
```

### Panel Descriptions

- **Left sidebar:** Project list. Each entry shows project name and activity dots (filled = active process, empty = idle). Click to switch projects.
- **Process tabs:** One tab per active golem process (code, review, qa). "+" button opens launch dialog.
- **Output pane:** Scrolling terminal-style display of the selected process's output. Auto-follows bottom. Tool calls get structured formatting (icons, collapsible bash output).
- **Task panel:** Live task list from state.yaml. Refreshed on each iteration end. Icons: ✓ done (green), ◐ in-progress (yellow), ○ todo (grey), ✗ blocked (red).
- **Stats section:** Iteration counter, elapsed timer, current phase, strategy decision.
- **Status bar:** Server connection info, total active processes, current project.

### Project Dashboard (no process selected)

When a project is selected but no process tab is active, show a dashboard:
- Task overview with progress bar
- Recent sessions from log.yaml
- Decisions list
- Pitfalls list

### Output Rendering

Tool calls from Claude are rendered with structure:

```
▎ 📁 Read  src/components/Chart.tsx
▎ ✏️  Edit  src/components/Chart.tsx  +24 -3
▎ 📁 Write src/components/PriceChart.tsx
▎ 🔨 Bash  go test ./...
▎   ├─ PASS  TestChart (0.02s)
▎   ├─ PASS  TestPriceData (0.01s)
▎   └─ ok    pkg/charts  0.05s
```

## Launch Dialog

Opened via the "+" button. Pre-fills from project config.

Fields:
- **Command:** dropdown (code, review, qa, plan)
- **Model:** dropdown (sonnet, opus, haiku)
- **Max Iterations:** number input (default: from config)
- **Max Tool Calls:** number input (default: from config)
- **Sandbox:** checkbox
- **MCP:** checkbox (default: true)
- **Parallel:** number input (default: 1)
- **Task Override:** optional text input (forces specific task)

## Settings

Accessible via ⚙ gear icon. Two tabs: Project and Global.

Shows resolved config values (defaults < global < project). Editing at a level overrides that level's config file. Matches the full Config struct:
- max-iterations, max-tool-calls, verbose, model
- sandbox, sandbox-tools, sandbox-timeout, sandbox-memory
- mcp, parallel, plugin-dir

## Go Server API (`golem serve`)

### REST Endpoints

```
GET    /api/projects                       List known projects
POST   /api/projects                       Register a project directory
GET    /api/projects/:id/state             Current state.yaml parsed
GET    /api/projects/:id/log               Session log
GET    /api/projects/:id/config            Resolved config
PUT    /api/projects/:id/config            Update project config

POST   /api/projects/:id/processes         Launch a golem process
GET    /api/projects/:id/processes         List active processes
DELETE /api/projects/:id/processes/:pid    Stop a process

GET    /api/config                         Global config
PUT    /api/config                         Update global config
```

### WebSocket Endpoints

```
WS /api/projects/:id/processes/:pid/stream    Real-time output + events
WS /api/projects/:id/watch                    State change notifications
```

### WebSocket Message Types

**Process stream:**
```json
{ "type": "output",     "line": "Reading state.yaml..." }
{ "type": "iter_start", "iter": 4, "maxIter": 20 }
{ "type": "iter_end",   "iter": 4, "task": "price charts", "outcome": "done" }
{ "type": "loop_done",  "result": { "iterations": 4, "outcome": "complete" } }
{ "type": "error",      "message": "process exited unexpectedly" }
```

**State watch:**
```json
{ "type": "state_changed", "state": { ... } }
{ "type": "log_appended",  "session": { ... } }
```

### Server Implementation

- Default port: `:8314`
- Process management via `os/exec` — spawns `golem code`, `golem review`, etc.
- Output streaming: pipe stdout/stderr through a fan-out buffer for multiple WebSocket clients
- State watching: `fsnotify` on `.ctx/state.yaml` and `.ctx/log.yaml`
- Project discovery: scan for `.ctx/` directories in configurable search paths

## Technology Stack

| Component | Technology | Role |
|-----------|-----------|------|
| CLI | Go (existing, unchanged) | Builder loop, review, QA, all business logic |
| Server | Go (`golem serve`, new) | Process management, state watching, WebSocket API |
| Desktop shell | Tauri 2 (Rust) | Window mgmt, system tray, native integration |
| Frontend | React 19 + TypeScript | All UI rendering and interaction |
| Styling | Tailwind CSS | Dark theme, terminal-inspired aesthetic |
| State mgmt | Zustand | Client-side state |
| Realtime | WebSocket (reconnecting) | Output streaming, state change notifications |
| Build | Vite | Frontend bundling |

## Visual Style

Developer-dark, terminal-inspired (Linear/Warp/Ghostty aesthetic):

- Background: `#0d1117` or `#1a1b26`
- Surface: `#161b22` for panels
- Borders: `#30363d`, subtle 1px
- Text: `#e6edf3` primary, `#8b949e` secondary
- Accent: `#58a6ff` (blue) for active/selected
- Done: `#3fb950` (green), Progress: `#d29922` (yellow), Todo: `#8b949e` (grey), Blocked: `#f85149` (red)
- Monospace: JetBrains Mono / Fira Code / system monospace
- Sans: Inter / system sans

## Project Structure

```
golem/
├── cmd/                    # existing CLI commands
├── internal/               # existing Go packages
│   └── server/             # NEW — golem serve implementation
│       ├── server.go       # HTTP server, routing
│       ├── processes.go    # process spawning, fan-out
│       ├── projects.go     # project discovery, state watching
│       └── websocket.go    # WebSocket handlers
├── templates/              # existing templates
├── ui/                     # NEW — desktop app
│   ├── src-tauri/          # Tauri Rust backend (minimal glue)
│   │   ├── src/main.rs
│   │   ├── Cargo.toml
│   │   └── tauri.conf.json
│   ├── src/                # React frontend
│   │   ├── App.tsx
│   │   ├── components/
│   │   │   ├── Sidebar.tsx
│   │   │   ├── ProcessView.tsx
│   │   │   ├── OutputPane.tsx
│   │   │   ├── TaskPanel.tsx
│   │   │   ├── LaunchDialog.tsx
│   │   │   └── SettingsDialog.tsx
│   │   ├── hooks/
│   │   │   ├── useWebSocket.ts
│   │   │   └── useGolemApi.ts
│   │   ├── stores/
│   │   │   └── appStore.ts
│   │   └── styles/
│   │       └── theme.ts
│   ├── package.json
│   ├── tailwind.config.ts
│   ├── vite.config.ts
│   └── tsconfig.json
├── go.mod
└── go.sum
```

## Error Handling

- **Server not running:** App shows connection screen with "Start Server" button
- **Process crashes:** WebSocket error event, UI shows error state with retry + session log link
- **Strategy halts:** Prominent halt reason display, affected tasks highlighted
- **Multiple clients:** WebSocket fan-out supports multiple windows
- **Disconnected:** Reconnecting WebSocket with exponential backoff, connection status indicator

## New Dependencies

**Go:**
- `nhooyr.io/websocket` or `github.com/gorilla/websocket` — WebSocket server
- `github.com/fsnotify/fsnotify` — file watching

**Frontend (ui/package.json):**
- `react`, `react-dom` — UI framework
- `@tauri-apps/api` — Tauri bridge
- `tailwindcss` — styling
- `zustand` — state management
- `vite` — build tool

**Tauri (ui/src-tauri/Cargo.toml):**
- `tauri` — desktop shell
