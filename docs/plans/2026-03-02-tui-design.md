# Golem TUI Design

## Overview

Add a terminal UI to `golem run` and `golem status` using Bubbletea + Lipgloss. The TUI is a display layer that wraps the existing builder loop — no loop logic changes. A `--no-tui` flag (and non-terminal detection) preserves current plain-text behavior.

## Architecture

```
cmd/run.go
  ├── --no-tui or non-terminal → existing RunBuilderLoop (unchanged)
  └── default (terminal)       → bubbletea program
                                  ├── spawns RunBuilderLoop in goroutine
                                  ├── loop emits events via channels
                                  └── TUI renders: output pane + sidebar
```

Key principle: the builder loop and reviewer work exactly as today. The TUI observes output and state changes. New package `internal/tui/` contains all bubbletea code.

## Output Streaming

New `StreamingCommandRunner` interface alongside existing `CommandRunner`:

```go
CommandRunner interface (unchanged)
  Run(ctx, dir, prompt, maxTurns, model) → (string, error)

StreamingCommandRunner interface (new)
  RunStreaming(ctx, dir, prompt, maxTurns, model, outputCh chan<- string) → (string, error)
```

`ClaudeRunner` implements both. `RunStreaming` pipes stdout through a scanner that sends each line to `outputCh`, while collecting full output in a buffer for promise detection and session saving. Existing `Run()` stays untouched.

`BuilderConfig` gets an optional `OutputCh chan<- string` field. When set, uses `RunStreaming`. When nil, calls `Run()` as before.

## Event System

```go
type Event struct {
    Type    EventType
    Iter    int            // IterStart, IterEnd
    MaxIter int            // IterStart
    Task    string         // IterEnd
    Outcome string         // IterEnd
    Line    string         // OutputLine
    Err     error          // IterEnd (if failed), LoopDone
    Result  *BuilderResult // LoopDone
}

type EventType int
const (
    EventIterStart   // iteration N beginning
    EventOutputLine  // a line of claude output
    EventIterEnd     // iteration N finished
    EventLoopDone    // loop finished
)
```

`BuilderConfig` gets an optional `Events chan<- Event` field. When non-nil, the loop sends events at natural points (alongside existing `fmt.Fprintf` calls). When nil, zero overhead.

The TUI re-reads `state.yaml` on each `EventIterEnd` to refresh the task list.

## `golem run` TUI Layout

```
┌─ golem run ──────────────────────────────────────────┐
│                                    │ Tasks        3/6 │
│  Claude Output                     │ ✓ auth module    │
│  ─────────────────────────         │ ✓ user model     │
│  > Reading state.yaml...           │ ✓ price fetcher  │
│  > Picking task: price charts      │ ◐ price charts   │
│  > Creating chart component...     │ ○ notifications  │
│  > Writing tests...                │ ✗ shipping       │
│  > Running go test ./...           │                  │
│  > PASS                            │─────────────────│
│  >                                 │ Iteration  4/20  │
│                                    │ Elapsed  6m12s   │
│                                    │ Tasks    3/6     │
│                                    │─────────────────│
│                                    │ ◐ price charts   │
│                                    │   running 1m30s  │
├────────────────────────────────────┴─────────────────┤
│ q quit                                    iter 4/20  │
└──────────────────────────────────────────────────────┘
```

Components:
- **Output pane**: scrolling list of output lines, auto-follows bottom
- **Sidebar tasks**: rendered from State.Tasks, refreshed on EventIterEnd
- **Sidebar stats**: iteration counter, elapsed timer (1s tick), task counts
- **Sidebar current**: current task name + running duration
- **Footer**: keybindings hint + iteration indicator

Styling: sidebar fixed 20-char width with border-left separator. Task icons colored (green done, yellow in-progress, dim todo, red blocked). Display-only — `q`/Ctrl+C to quit gracefully.

## `golem status` TUI

Live-watching dashboard that polls `state.yaml` and `log.yaml` every 2 seconds. Updates in place when data changes.

```
┌─ golem status ───────────────────────────────────────┐
│ Project: MyProject          Phase: building          │
│ Focus: competitor price tracking                     │
│                                                      │
│ Tasks                                           3/6  │
│  ✓ auth module                                       │
│  ✓ user model                                        │
│  ✓ price fetcher                                     │
│  ◐ price charts — "working on chart component"       │
│  ○ notifications (depends on: price charts)           │
│  ✗ shipping — blocked: "external API pending"        │
│                                                      │
│ Decisions: 4    Pitfalls: 3    Locked: 2             │
│ Sessions: 7 logged                                   │
├──────────────────────────────────────────────────────┤
│ q quit                          watching state.yaml  │
└──────────────────────────────────────────────────────┘
```

Polling chosen over fsnotify to avoid adding a dependency for files that change at most once per minute.

## Integration

`cmd/run.go`: New `--no-tui` flag. Default is TUI when stdout is a terminal. Non-terminal auto-falls back to plain text.

`cmd/status.go`: Same `--no-tui` pattern.

`ClaudeRunner`: In TUI mode, stdout/stderr from Claude process go through channels to the TUI instead of directly to os.Stdout/os.Stderr.

## New Files

```
internal/tui/
├── run.go         // bubbletea model for golem run
├── status.go      // bubbletea model for golem status
├── styles.go      // shared lipgloss styles
├── events.go      // Event types and EventType constants
└── components.go  // shared renderers (task list, stats panel)
```

## Dependencies

```
github.com/charmbracelet/bubbletea
github.com/charmbracelet/lipgloss
github.com/charmbracelet/bubbles   // viewport for scrolling output
```
