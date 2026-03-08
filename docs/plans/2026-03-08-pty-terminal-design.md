# PTY Terminal Emulator Design

**Goal:** Replace line-based process output with full PTY terminal emulation. All processes (code, review, qa, plan) spawn via PTY for proper ANSI color support. Interactive commands (plan) accept keyboard input through the same WebSocket.

## Architecture

```
Flutter xterm widget  <──WebSocket──>  Go server  <──PTY──>  golem cmd  ──>  claude
     (render+input)                   (relay)              (subprocess)
```

All processes get a PTY. The server streams raw PTY output as base64-encoded JSON over WebSocket. For interactive commands, the same WebSocket accepts input and resize messages from the Flutter client.

## Server-side changes

### PTY spawning (`internal/server/process.go`)

- Replace `cmd.StdoutPipe()` with `creack/pty` — `pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})`
- Store the `*os.File` PTY handle on `managedProcess`
- Read loop reads raw bytes from PTY (not line-based), writes to a `rawBuffer` (circular byte buffer, ~1MB) instead of the current `ringBuffer` of strings
- For interactive commands, write incoming bytes to the PTY file

### WebSocket protocol (`internal/server/websocket.go`)

Messages (server→client):
- `{"type": "output", "data": "<base64>"}` — chunks of PTY output
- `{"type": "exit", "code": N}` — process exited

Messages (client→server):
- `{"type": "input", "data": "<base64>"}` — keystrokes, written to PTY
- `{"type": "resize", "cols": N, "rows": N}` — calls `pty.Setsize()`

On connect, send buffered backlog as one large base64 chunk so reconnecting clients see history.

### Dependencies

Add `github.com/creack/pty` to go.mod.

## Flutter-side changes

### New dependency

`xterm: ^4.0.0` — Flutter terminal emulator with ANSI support, mouse events, selection, resize.

### Process view (`views/process_view.dart`)

- Replace `ListView.builder` output pane with `xterm.TerminalView` widget
- Create a `Terminal` instance per process, feed incoming base64-decoded bytes via `terminal.write()`
- On keystroke, `TerminalView`'s `onInput` callback sends `{"type": "input", "data": "<base64>"}` over WebSocket
- On widget resize, calculate cols/rows from pixel dimensions and cell size, send `{"type": "resize", "cols": N, "rows": N}`
- Task panel on the right stays as-is

### Providers (`providers/processes.dart`)

- `ProcessOutputNotifier` changes from `List<String>` state to holding a `Terminal` object
- WebSocket `onMessage` decodes base64 and writes to terminal
- Add `sendInput(String processId, String data)` and `sendResize(String processId, int cols, int rows)` methods

### WebSocket manager (`api/websocket.dart`)

- Add `send(Map<String, dynamic> message)` method for upstream messages (currently receive-only)

## Robustness & concurrency

### Non-blocking I/O

- Set PTY fd non-blocking with `syscall.SetNonblock(fd)` after PTY creation
- Use buffered reads (fixed 4096-byte buffer with deadline-based reads) to prevent goroutine hangs when processes spam output

### Timeouts

- Read deadlines on PTY reads so goroutine can check context cancellation
- WebSocket write deadlines to prevent slow clients from blocking the output pipeline

### Lifecycle

- On process exit: close PTY fd, drain remaining buffered output, send `{"type": "exit", "code": N}`, close WebSocket
- On WebSocket disconnect: stop sending output (don't block on dead subscribers), keep process running — client can reconnect
- On server shutdown: SIGTERM to all managed processes, close all PTYs, drain gracefully with timeout

### Signals

- `SIGWINCH` propagated via `pty.Setsize()` on resize messages
- `SIGINT`/`SIGTERM` forwarded to PTY process group on stop

### Buffering

- Ring buffer stores raw bytes (~1MB) not lines — handles binary/ANSI output correctly
- Subscriber channels remain non-blocking (`select` with `default`) so slow consumers never block the read loop

## Error handling

- **Process exits:** Server closes PTY, sends exit message. Flutter shows exit status in terminal, disables input.
- **WebSocket reconnect:** Server sends buffered backlog. Terminal clears and replays — cursor state lost but output preserved.
- **Non-interactive commands:** code/review/qa get PTY for ANSI colors but server ignores WebSocket input.
- **Terminal size mismatch:** Initial size from Flutter widget's first layout. If server starts PTY before client connects, default 120x40. Client sends resize on connect.
