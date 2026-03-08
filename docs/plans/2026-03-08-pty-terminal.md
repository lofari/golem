# PTY Terminal Emulator Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace line-based process output with full PTY terminal emulation across all commands, enabling interactive `plan` sessions and ANSI color output for all processes.

**Architecture:** All processes spawn via `creack/pty` instead of `cmd.StdoutPipe()`. Raw PTY bytes stream as base64 JSON over bidirectional WebSocket. Flutter renders output in an `xterm` terminal emulator widget. Input and resize messages flow client→server on the same WebSocket.

**Tech Stack:** Go (creack/pty, syscall), Flutter (xterm ^4.0.0), WebSocket (nhooyr.io/websocket, web_socket_channel)

**Design doc:** `docs/plans/2026-03-08-pty-terminal-design.md`

---

### Task 0: Add `creack/pty` dependency

**Files:**
- Modify: `go.mod`

**Step 1: Add the dependency**

```bash
go get github.com/creack/pty/v2
```

**Step 2: Verify**

```bash
go build ./...
```

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "feat(server): add creack/pty dependency"
```

---

### Task 1: Replace `ringBuffer` with `rawBuffer` (byte-based)

The current `ringBuffer` stores `[]string` lines. PTY output is raw bytes. Replace it with a circular byte buffer.

**Files:**
- Modify: `internal/server/process.go`

**Step 1: Replace ringBuffer with rawBuffer**

Replace the `ringBuffer` type and its methods with a byte-based circular buffer:

```go
// rawBuffer is a circular byte buffer for PTY output.
type rawBuffer struct {
	mu   sync.Mutex
	buf  []byte
	size int
	w    int    // write position
	full bool   // whether buffer has wrapped
}

func newRawBuffer(size int) *rawBuffer {
	return &rawBuffer{buf: make([]byte, size), size: size}
}

func (rb *rawBuffer) Write(data []byte) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	for len(data) > 0 {
		n := copy(rb.buf[rb.w:], data)
		rb.w += n
		data = data[n:]
		if rb.w >= rb.size {
			rb.w = 0
			rb.full = true
		}
	}
}

func (rb *rawBuffer) Bytes() []byte {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if !rb.full {
		out := make([]byte, rb.w)
		copy(out, rb.buf[:rb.w])
		return out
	}
	out := make([]byte, rb.size)
	n := copy(out, rb.buf[rb.w:])
	copy(out[n:], rb.buf[:rb.w])
	return out
}
```

**Step 2: Update managedProcess to use rawBuffer**

Change the `output` field type from `*ringBuffer` to `*rawBuffer`, and change subscriber channels from `chan string` to `chan []byte`:

```go
type managedProcess struct {
	info   ProcessInfo
	cmd    *exec.Cmd
	cancel context.CancelFunc
	ptyF   *os.File       // PTY file handle
	output *rawBuffer
	mu     sync.Mutex
	subs   map[chan []byte]struct{}
}
```

Update `Subscribe` and `Unsubscribe` to use `chan []byte`:

```go
func (mp *managedProcess) Subscribe() chan []byte {
	ch := make(chan []byte, 256)
	mp.mu.Lock()
	mp.subs[ch] = struct{}{}
	mp.mu.Unlock()
	return ch
}

func (mp *managedProcess) Unsubscribe(ch chan []byte) {
	mp.mu.Lock()
	delete(mp.subs, ch)
	mp.mu.Unlock()
	close(ch)
}
```

**Step 3: Verify build**

```bash
go build ./...
```

Note: Tests will fail because `launchProcess` and WebSocket code still reference the old types. That's expected — we fix those in the next tasks.

**Step 4: Commit**

```bash
git add internal/server/process.go
git commit -m "refactor(server): replace ringBuffer with rawBuffer for byte-based PTY output"
```

---

### Task 2: PTY-based process spawning

Replace `cmd.StdoutPipe()` with `creack/pty` in `launchProcess`.

**Files:**
- Modify: `internal/server/process.go`

**Step 1: Update imports**

Add to the import block in `process.go`:

```go
"encoding/base64"
"syscall"

"github.com/creack/pty/v2"
```

Remove the `"time"` import only if it becomes unused (it's still used for `ProcessInfo.StartedAt` and the process ID generation — keep it).

**Step 2: Replace process spawning in launchProcess**

Replace everything from `ctx, cancel := context.WithCancel(...)` through the end of the output read goroutine and wait goroutine (lines ~126-196) with:

```go
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, golemBin, args...)
	cmd.Dir = proj.path

	// Start with PTY
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("starting process with pty: %w", err)
	}

	// Set non-blocking to prevent deadlocks when processes spam output
	if err := syscall.SetNonblock(int(ptmx.Fd()), true); err != nil {
		ptmx.Close()
		cancel()
		return nil, fmt.Errorf("set nonblock: %w", err)
	}

	id := fmt.Sprintf("%s-%s-%d", proj.id[:8], req.Command, time.Now().UnixMilli())
	mp := &managedProcess{
		info: ProcessInfo{
			ID:        id,
			Command:   req.Command,
			Status:    "running",
			StartedAt: time.Now(),
		},
		cmd:    cmd,
		cancel: cancel,
		ptyF:   ptmx,
		output: newRawBuffer(1024 * 1024), // 1MB
		subs:   make(map[chan []byte]struct{}),
	}
	mp.info.PID = cmd.Process.Pid

	// Read PTY output in background
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				mp.output.Write(chunk)
				mp.mu.Lock()
				for ch := range mp.subs {
					select {
					case ch <- chunk:
					default: // don't block on slow consumers
					}
				}
				mp.mu.Unlock()
			}
			if err != nil {
				break
			}
		}
	}()

	// Wait for process to finish in background
	go func() {
		err := cmd.Wait()
		ptmx.Close()
		mp.mu.Lock()
		if err != nil {
			mp.info.Status = "failed"
		} else {
			mp.info.Status = "stopped"
		}
		// Notify subscribers that process exited
		for ch := range mp.subs {
			select {
			case ch <- nil: // nil signals exit
			default:
			}
		}
		mp.mu.Unlock()
	}()

	s.mu.Lock()
	s.processes[id] = mp
	s.mu.Unlock()

	return mp, nil
```

**Step 3: Remove old StdoutPipe code**

The old code that used `cmd.StdoutPipe()` and `cmd.Stderr = cmd.Stdout` should be fully replaced by step 2.

**Step 4: Verify build**

```bash
go build ./...
```

**Step 5: Commit**

```bash
git add internal/server/process.go
git commit -m "feat(server): spawn processes with PTY via creack/pty"
```

---

### Task 3: Update WebSocket to stream base64 + accept input/resize

**Files:**
- Modify: `internal/server/websocket.go`

**Step 1: Update WSMessage struct**

Add `Data` and `Code` fields, keep `Line` for backward compat during transition:

```go
type WSMessage struct {
	Type    string      `json:"type"`
	Data    string      `json:"data,omitempty"`    // base64-encoded PTY output
	Line    string      `json:"line,omitempty"`    // deprecated, kept for compat
	Cols    int         `json:"cols,omitempty"`
	Rows    int         `json:"rows,omitempty"`
	Code    *int        `json:"code,omitempty"`    // exit code
	State   interface{} `json:"state,omitempty"`
	Session interface{} `json:"session,omitempty"`
	Error   string      `json:"error,omitempty"`
}
```

**Step 2: Replace handleProcessStream**

Replace the entire `handleProcessStream` function:

```go
func (s *Server) handleProcessStream(w http.ResponseWriter, r *http.Request) {
	procID := r.PathValue("procId")
	s.mu.RLock()
	mp, ok := s.processes[procID]
	s.mu.RUnlock()

	if !ok {
		writeError(w, http.StatusNotFound, "process not found")
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := r.Context()

	// Send backlog as one base64 chunk
	backlog := mp.output.Bytes()
	if len(backlog) > 0 {
		msg, _ := json.Marshal(WSMessage{
			Type: "output",
			Data: base64.StdEncoding.EncodeToString(backlog),
		})
		conn.Write(ctx, websocket.MessageText, msg)
	}

	// Subscribe to new output
	ch := mp.Subscribe()
	defer mp.Unsubscribe(ch)

	// Read input from client in background
	go func() {
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var inMsg WSMessage
			if err := json.Unmarshal(data, &inMsg); err != nil {
				continue
			}
			switch inMsg.Type {
			case "input":
				if mp.ptyF != nil {
					decoded, err := base64.StdEncoding.DecodeString(inMsg.Data)
					if err == nil {
						mp.ptyF.Write(decoded)
					}
				}
			case "resize":
				if mp.ptyF != nil && inMsg.Cols > 0 && inMsg.Rows > 0 {
					pty.Setsize(mp.ptyF, &pty.Winsize{
						Rows: uint16(inMsg.Rows),
						Cols: uint16(inMsg.Cols),
					})
				}
			}
		}
	}()

	// Stream output to client
	for {
		select {
		case <-ctx.Done():
			return
		case chunk, ok := <-ch:
			if !ok {
				return
			}
			if chunk == nil {
				// Process exited
				mp.mu.Lock()
				status := mp.info.Status
				mp.mu.Unlock()
				code := 0
				if status == "failed" {
					code = 1
				}
				msg, _ := json.Marshal(WSMessage{Type: "exit", Code: &code})
				conn.Write(ctx, websocket.MessageText, msg)
				return
			}
			msg, _ := json.Marshal(WSMessage{
				Type: "output",
				Data: base64.StdEncoding.EncodeToString(chunk),
			})
			if err := conn.Write(ctx, websocket.MessageText, msg); err != nil {
				return
			}
		}
	}
}
```

**Step 3: Add imports**

Add to the import block in `websocket.go`:

```go
"encoding/base64"

"github.com/creack/pty/v2"
```

**Step 4: Verify build**

```bash
go build ./...
```

**Step 5: Commit**

```bash
git add internal/server/websocket.go
git commit -m "feat(server): bidirectional WebSocket with base64 PTY streaming"
```

---

### Task 4: Update server tests for new PTY/buffer types

The existing tests use `ringBuffer` and `chan string`. Update them to work with the new types.

**Files:**
- Modify: `internal/server/process_test.go`
- Modify: `internal/server/websocket_test.go`

**Step 1: Update process_test.go**

The `TestLaunchAndListProcesses` and `TestStopProcess` tests launch real golem subprocesses. They should still work since `launchProcess` now uses PTY. No changes needed to the test logic — just verify they still pass.

**Step 2: Update websocket_test.go**

The test creates a dummy `managedProcess` with `ringBuffer`. Update to use `rawBuffer` and `chan []byte`:

```go
func TestWebSocketProcessStream(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	srv.RegisterProject(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	pid := srv.projectID(dir)

	// Create a dummy process with output
	mp := &managedProcess{
		info: ProcessInfo{
			ID:      "test-proc",
			Command: "code",
			Status:  "running",
		},
		output: newRawBuffer(1024),
		subs:   make(map[chan []byte]struct{}),
	}
	srv.mu.Lock()
	srv.processes["test-proc"] = mp
	srv.mu.Unlock()

	// Write backlog
	mp.output.Write([]byte("hello world\n"))

	// Connect WebSocket
	wsURL := "ws" + ts.URL[4:] + "/api/projects/" + pid + "/processes/test-proc/stream"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Read from WebSocket — should get backlog as base64
	_, msg, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var wsMsg WSMessage
	if err := json.Unmarshal(msg, &wsMsg); err != nil {
		t.Fatal(err)
	}
	if wsMsg.Type != "output" {
		t.Fatalf("expected type 'output', got %q", wsMsg.Type)
	}
	decoded, err := base64.StdEncoding.DecodeString(wsMsg.Data)
	if err != nil {
		t.Fatalf("failed to decode base64: %v", err)
	}
	if string(decoded) != "hello world\n" {
		t.Fatalf("expected 'hello world\\n', got %q", string(decoded))
	}
}
```

**Step 3: Add import**

Add `"encoding/base64"` to `websocket_test.go` imports.

**Step 4: Run tests**

```bash
go test ./internal/server/ -v -run TestWebSocket
go test ./internal/server/ -v -run TestLaunch
go test ./internal/server/ -v -run TestStop
go test ./internal/server/ -v
```

Expected: All tests pass.

**Step 5: Commit**

```bash
git add internal/server/process_test.go internal/server/websocket_test.go
git commit -m "test(server): update tests for PTY-based process streaming"
```

---

### Task 5: Add `xterm` dependency to Flutter

**Files:**
- Modify: `ui/flutter/pubspec.yaml`

**Step 1: Add xterm dependency**

```bash
cd ui/flutter && flutter pub add xterm
```

**Step 2: Verify**

```bash
cd ui/flutter && flutter analyze
```

**Step 3: Commit**

```bash
git add ui/flutter/pubspec.yaml ui/flutter/pubspec.lock
git commit -m "feat(ui): add xterm terminal emulator dependency"
```

---

### Task 6: Add send method to GolemWebSocket

The WebSocket client is currently receive-only. Add a `send` method for upstream messages.

**Files:**
- Modify: `ui/flutter/lib/api/websocket.dart`

**Step 1: Add send method**

Add this method to the `GolemWebSocket` class, after the `connect()` method:

```dart
  void send(Map<String, dynamic> message) {
    _channel?.sink.add(jsonEncode(message));
  }
```

**Step 2: Verify**

```bash
cd ui/flutter && flutter analyze
```

**Step 3: Commit**

```bash
git add ui/flutter/lib/api/websocket.dart
git commit -m "feat(ui): add send method to GolemWebSocket for bidirectional communication"
```

---

### Task 7: Update providers for terminal-based output

Replace the line-based `ProcessOutputNotifier` with a terminal-based one.

**Files:**
- Modify: `ui/flutter/lib/providers/processes.dart`

**Step 1: Replace ProcessOutputNotifier**

Replace the `processOutputProvider` and `ProcessOutputNotifier` class with a terminal-based provider. The new provider holds a `Terminal` object from the xterm package and feeds decoded bytes into it.

```dart
import 'dart:convert';
import 'package:xterm/xterm.dart';
```

Add these to the existing imports at the top of the file.

Replace the `processOutputProvider` and `ProcessOutputNotifier` with:

```dart
// Terminal instance per process
final processTerminalProvider =
    StateNotifierProvider.family<ProcessTerminalNotifier, Terminal, String>((ref, processId) {
  final projectInfo = ref.read(projectInfoProvider);
  final api = ref.read(apiClientProvider);
  return ProcessTerminalNotifier(api, projectInfo?.id, processId);
});

class ProcessTerminalNotifier extends StateNotifier<Terminal> {
  final GolemApiClient _api;
  GolemWebSocket? _ws;
  bool _exited = false;

  ProcessTerminalNotifier(this._api, String? projectId, String processId)
      : super(Terminal()) {
    if (projectId != null) {
      _ws = GolemWebSocket(
        url: _api.processStreamUrl(projectId, processId),
        onMessage: (data) {
          switch (data['type']) {
            case 'output':
              final bytes = base64Decode(data['data'] as String);
              state.write(String.fromCharCodes(bytes));
            case 'exit':
              _exited = true;
          }
        },
      );
      _ws!.connect();

      // Forward terminal input to server
      state.onOutput = (data) {
        if (!_exited) {
          _ws?.send({
            'type': 'input',
            'data': base64Encode(utf8.encode(data)),
          });
        }
      };
    }
  }

  bool get exited => _exited;

  void sendResize(int cols, int rows) {
    _ws?.send({'type': 'resize', 'cols': cols, 'rows': rows});
  }

  @override
  void dispose() {
    _ws?.dispose();
    super.dispose();
  }
}
```

Remove the old `processOutputProvider` and `ProcessOutputNotifier`.

**Step 2: Verify**

```bash
cd ui/flutter && flutter analyze
```

Note: `process_view.dart` will have errors because it still references `processOutputProvider`. That's fixed in the next task.

**Step 3: Commit**

```bash
git add ui/flutter/lib/providers/processes.dart
git commit -m "feat(ui): replace line-based output provider with terminal-based provider"
```

---

### Task 8: Update ProcessView to use TerminalView

Replace the `ListView.builder` output pane with the `xterm` `TerminalView` widget.

**Files:**
- Modify: `ui/flutter/lib/views/process_view.dart`

**Step 1: Replace process_view.dart**

Replace the entire file:

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:xterm/xterm.dart';
import '../models/project.dart' as models;
import '../providers/project.dart';
import '../providers/processes.dart';
import '../theme.dart';

class ProcessView extends ConsumerWidget {
  final String processId;
  const ProcessView({super.key, required this.processId});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Row(
      children: [
        Expanded(child: _TerminalPane(processId: processId)),
        const _TaskPanel(),
      ],
    );
  }
}

class _TerminalPane extends ConsumerStatefulWidget {
  final String processId;
  const _TerminalPane({required this.processId});

  @override
  ConsumerState<_TerminalPane> createState() => _TerminalPaneState();
}

class _TerminalPaneState extends ConsumerState<_TerminalPane> {
  @override
  Widget build(BuildContext context) {
    final terminal = ref.watch(processTerminalProvider(widget.processId));

    return Container(
      color: GolemTheme.bgPrimary,
      child: LayoutBuilder(
        builder: (context, constraints) {
          // Send resize when layout changes
          WidgetsBinding.instance.addPostFrameCallback((_) {
            final notifier = ref.read(
              processTerminalProvider(widget.processId).notifier,
            );
            notifier.sendResize(terminal.viewWidth, terminal.viewHeight);
          });

          return TerminalView(
            terminal,
            autofocus: true,
          );
        },
      ),
    );
  }
}

class _TaskPanel extends ConsumerWidget {
  const _TaskPanel();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(projectStateProvider);
    final sessions = ref.watch(sessionsProvider);

    if (state == null) return const SizedBox.shrink();

    final tasks = state.tasks;
    final done = tasks.where((t) => t.status == 'done').length;
    final lastSession = sessions.isNotEmpty ? sessions.last : null;

    return Container(
      width: 220,
      decoration: const BoxDecoration(
        color: GolemTheme.bgSurface,
        border: Border(left: BorderSide(color: GolemTheme.border)),
      ),
      child: Column(
        children: [
          // Header
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
            decoration: const BoxDecoration(
              border: Border(bottom: BorderSide(color: GolemTheme.border)),
            ),
            child: Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                const Text(
                  'TASKS',
                  style: TextStyle(
                    fontSize: 10,
                    fontWeight: FontWeight.w600,
                    letterSpacing: 1,
                    color: GolemTheme.textSecondary,
                  ),
                ),
                Text(
                  '$done/${tasks.length}',
                  style: const TextStyle(fontSize: 11, color: GolemTheme.textSecondary),
                ),
              ],
            ),
          ),
          // Task list
          Expanded(
            child: ListView.builder(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
              itemCount: tasks.length,
              itemBuilder: (context, i) => _TaskItem(task: tasks[i]),
            ),
          ),
          // Stats footer
          Container(
            padding: const EdgeInsets.all(12),
            decoration: const BoxDecoration(
              border: Border(top: BorderSide(color: GolemTheme.border)),
            ),
            child: Column(
              children: [
                _StatRow('Phase', state.status.phase.isNotEmpty ? state.status.phase : '\u2014'),
                _StatRow(
                  'Focus',
                  state.status.currentFocus.isNotEmpty
                      ? state.status.currentFocus
                      : '\u2014',
                ),
                if (lastSession != null) ...[
                  _StatRow('Last iter', '#${lastSession.iteration}'),
                  _StatRow(
                    'Outcome',
                    lastSession.outcome,
                    valueColor: switch (lastSession.outcome) {
                      'done' => GolemTheme.green,
                      'blocked' || 'unproductive' => GolemTheme.red,
                      _ => GolemTheme.yellow,
                    },
                  ),
                ],
                _StatRow('Decisions', '${state.decisions.length}'),
                _StatRow('Pitfalls', '${state.pitfalls.length}'),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _TaskItem extends StatelessWidget {
  final models.Task task;
  const _TaskItem({required this.task});

  @override
  Widget build(BuildContext context) {
    final (icon, color) = switch (task.status) {
      'done' => ('\u2713', GolemTheme.green),
      'in-progress' => ('\u25D0', GolemTheme.yellow),
      'blocked' => ('\u2717', GolemTheme.red),
      _ => ('\u25CB', GolemTheme.textSecondary),
    };

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 16,
            child: Text(icon, style: TextStyle(fontSize: 11, color: color, fontFamily: 'monospace')),
          ),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  task.name,
                  style: const TextStyle(fontSize: 12),
                  overflow: TextOverflow.ellipsis,
                ),
                if (task.blockedReason != null)
                  Text(
                    task.blockedReason!,
                    style: const TextStyle(fontSize: 10, color: GolemTheme.red),
                    overflow: TextOverflow.ellipsis,
                  ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _StatRow extends StatelessWidget {
  final String label;
  final String value;
  final Color? valueColor;

  const _StatRow(this.label, this.value, {this.valueColor});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 1),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Text(label, style: const TextStyle(fontSize: 11, color: GolemTheme.textSecondary)),
          Flexible(
            child: Text(
              value,
              style: TextStyle(fontSize: 11, color: valueColor ?? GolemTheme.textPrimary),
              overflow: TextOverflow.ellipsis,
              textAlign: TextAlign.end,
            ),
          ),
        ],
      ),
    );
  }
}
```

**Step 2: Verify**

```bash
cd ui/flutter && flutter analyze && flutter build linux --debug
```

**Step 3: Commit**

```bash
git add ui/flutter/lib/views/process_view.dart
git commit -m "feat(ui): replace output ListView with xterm TerminalView"
```

---

### Task 9: Build, install, and integration test

**Step 1: Build Go server**

```bash
go build ./... && go test ./...
```

Expected: All tests pass.

**Step 2: Build Flutter release**

```bash
make ui
```

Expected: Release binary at `~/go/bin/golem-ui-bundle/golem_ui`.

**Step 3: Install golem**

```bash
go install . && make install-ui
```

**Step 4: Integration test**

```bash
pkill -f "golem ui" || true
cd /home/winler/projects/TROGUE && golem ui
```

Expected:
- Server starts on :8314
- Flutter app launches
- Dashboard shows TROGUE project
- Click "Launch" → select "code" → Launch → terminal shows colored golem output
- Click "Plan" → terminal opens with interactive claude session, accepts keyboard input
- Resize window → terminal reflows correctly

**Step 5: Commit any fixes**

```bash
git add -A
git commit -m "feat(ui): complete PTY terminal integration"
```
