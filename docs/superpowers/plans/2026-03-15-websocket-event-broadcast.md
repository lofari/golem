# WebSocket Engine Event Broadcast Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the Go server to broadcast engine events over WebSocket so the Flutter UI can consume them in real time.

**Architecture:** Extend `handleStateWatch` to watch `.ctx/runs/` for `log.json` files, tail new NDJSON lines, and broadcast each as an `engine_event` WebSocket message. Engine events bypass the debounce timer (they need low-latency delivery). On the Flutter side, handle `engine_event` in `ProjectStateNotifier`, route to `RunsNotifier`, and expose events reactively via a family provider for the timeline view.

**Tech Stack:** Go (fsnotify, nhooyr.io/websocket), Flutter/Dart (Riverpod, web_socket_channel)

**Note:** The current Flutter provider layer (`projectStateProvider`, `projectInfoProvider`) is single-project scoped. This plan wires events through that existing path. Multi-project event routing (one WebSocket per open project tab) is a follow-up.

---

## Chunk 1: Go Server — Event Broadcast

### Task 1: Add event broadcast to handleStateWatch

**Files:**
- Modify: `internal/server/websocket.go:18-29` (WSMessage — add Event field)
- Modify: `internal/server/websocket.go:129-224` (handleStateWatch — add runs dir watcher + log.json tailer)

- [ ] **Step 1: Write the failing test**

In `internal/server/websocket_test.go`, add imports (`"os"`, `"path/filepath"`) and a test that connects to `/watch`, writes NDJSON to `.ctx/runs/{runId}/log.json`, and expects `engine_event` messages:

```go
func TestWebSocketEngineEvents(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	srv.RegisterProject(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	pid := srv.projectID(dir)

	// Create runs directory before connecting so watcher picks it up
	runsDir := filepath.Join(dir, ".ctx", "runs")
	os.MkdirAll(runsDir, 0755)

	// Connect WebSocket
	wsURL := "ws" + ts.URL[4:] + "/api/projects/" + pid + "/watch"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Read initial state_changed
	_, _, err = conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Write a run's log.json with an engine event
	runDir := filepath.Join(runsDir, "run-test-001")
	os.MkdirAll(runDir, 0755)
	time.Sleep(100 * time.Millisecond) // let watcher pick up directory

	logFile, err := os.Create(filepath.Join(runDir, "log.json"))
	if err != nil {
		t.Fatal(err)
	}
	event := map[string]interface{}{
		"type":      "pipeline-start",
		"timestamp": time.Now().Format(time.RFC3339Nano),
		"agent":     "build-feature",
		"goal":      "add auth",
		"run-id":    "run-test-001",
	}
	data, _ := json.Marshal(event)
	logFile.Write(data)
	logFile.Write([]byte("\n"))
	logFile.Close()

	// Read engine_event from WebSocket
	_, msg, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("expected engine_event message: %v", err)
	}

	var wsMsg WSMessage
	if err := json.Unmarshal(msg, &wsMsg); err != nil {
		t.Fatal(err)
	}
	if wsMsg.Type != "engine_event" {
		t.Fatalf("expected type 'engine_event', got %q", wsMsg.Type)
	}
	if wsMsg.Event == nil {
		t.Fatal("expected event data, got nil")
	}
	eventMap := wsMsg.Event.(map[string]interface{})
	if eventMap["type"] != "pipeline-start" {
		t.Fatalf("expected event type 'pipeline-start', got %v", eventMap["type"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestWebSocketEngineEvents -v`
Expected: FAIL — `WSMessage` has no `Event` field.

- [ ] **Step 3: Add Event field to WSMessage**

In `internal/server/websocket.go`, add the `Event` field to `WSMessage`:

```go
type WSMessage struct {
	Type    string      `json:"type"`
	Data    string      `json:"data,omitempty"`
	Line    string      `json:"line,omitempty"`
	Cols    int         `json:"cols,omitempty"`
	Rows    int         `json:"rows,omitempty"`
	Code    *int        `json:"code,omitempty"`
	State   interface{} `json:"state,omitempty"`
	Session interface{} `json:"session,omitempty"`
	Event   interface{} `json:"event,omitempty"`
	Error   string      `json:"error,omitempty"`
}
```

- [ ] **Step 4: Add tailLogJSON helper function**

In `internal/server/websocket.go`, add the helper function before `handleStateWatch`. Add `"bufio"`, `"os"`, and `"strings"` to the imports.

```go
// tailLogJSON reads new lines from a log.json file starting at the given offset.
// Returns the parsed events and the new offset (offset + bytes consumed).
func tailLogJSON(path string, offset int64) ([]json.RawMessage, int64) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.Size() <= offset {
		return nil, offset
	}

	f.Seek(offset, 0)
	var events []json.RawMessage
	var bytesRead int64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		bytesRead += int64(len(line)) + 1 // +1 for newline
		if len(line) == 0 {
			continue
		}
		raw := make(json.RawMessage, len(line))
		copy(raw, line)
		if json.Valid(raw) {
			events = append(events, raw)
		}
	}
	return events, offset + bytesRead
}
```

- [ ] **Step 5: Extend handleStateWatch with runs directory watching**

In `handleStateWatch`, after `watcher.Add(ctxDir)` (line 160), add runs directory watching. Do NOT use `os.MkdirAll` — only watch if the directory already exists:

```go
	// Watch runs directory for engine events (if it exists)
	runsDir := filepath.Join(proj.path, ".ctx", "runs")
	runsWatched := false

	// Track known run dirs and their log offsets (keyed by absolute path)
	logOffsets := make(map[string]int64)

	// Watch for new run directories and register them
	watchRunDirs := func() {
		if !runsWatched {
			if _, err := os.Stat(runsDir); err == nil {
				watcher.Add(runsDir)
				runsWatched = true
			}
		}
		if !runsWatched {
			return
		}
		entries, err := os.ReadDir(runsDir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			runPath, _ := filepath.Abs(filepath.Join(runsDir, entry.Name()))
			if _, known := logOffsets[runPath]; !known {
				logOffsets[runPath] = 0
				watcher.Add(runPath)
			}
		}
	}
	watchRunDirs()
```

- [ ] **Step 6: Replace the event loop body in handleStateWatch**

Replace the existing event classification and debounce block (lines 179-215) with logic that separates engine events (immediate delivery) from state/log changes (debounced). The key fix: engine events do NOT share the debounce timer. Declare `pendingState` and `pendingLog` before the `for` loop to accumulate across debounce resets:

```go
	var pendingState, pendingLog bool
```

Then the event loop body becomes:

```go
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}

			absPath, _ := filepath.Abs(event.Name)
			absState, _ := filepath.Abs(statePath)
			absLog, _ := filepath.Abs(logPath)
			absRuns, _ := filepath.Abs(runsDir)

			isState := absPath == absState
			isLog := absPath == absLog
			isRunsDir := absPath == absRuns
			isRunLog := !isState && !isLog && strings.HasSuffix(absPath, "log.json") &&
				strings.HasPrefix(absPath, absRuns+string(filepath.Separator))

			if !isState && !isLog && !isRunsDir && !isRunLog {
				continue
			}

			// New run directory appeared — register it
			if isRunsDir {
				watchRunDirs()
				continue
			}

			// Engine events: deliver immediately, no debounce
			if isRunLog {
				runDir, _ := filepath.Abs(filepath.Dir(absPath))
				events, newOffset := tailLogJSON(absPath, logOffsets[runDir])
				logOffsets[runDir] = newOffset
				for _, ev := range events {
					msg, _ := json.Marshal(WSMessage{Type: "engine_event", Event: ev})
					conn.Write(ctx, websocket.MessageText, msg)
				}
				continue
			}

			// State/log changes: debounce 200ms, accumulate pending flags
			if isState {
				pendingState = true
			}
			if isLog {
				pendingLog = true
			}
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(200*time.Millisecond, func() {
				if pendingState {
					pendingState = false
					if state, err := golemctx.ReadState(proj.path); err == nil {
						msg, _ := json.Marshal(WSMessage{Type: "state_changed", State: state})
						conn.Write(ctx, websocket.MessageText, msg)
					}
				}
				if pendingLog {
					pendingLog = false
					if log, err := golemctx.ReadLog(proj.path); err == nil {
						sessions := log.Sessions
						if len(sessions) > 0 {
							msg, _ := json.Marshal(WSMessage{Type: "log_appended", Session: sessions[len(sessions)-1]})
							conn.Write(ctx, websocket.MessageText, msg)
						}
					}
				}
			})
```

- [ ] **Step 7: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestWebSocketEngineEvents -v`
Expected: PASS

- [ ] **Step 8: Run all server tests**

Run: `go test ./internal/server/ -v`
Expected: All tests pass, no regressions.

- [ ] **Step 9: Commit**

```bash
git add internal/server/websocket.go internal/server/websocket_test.go
git commit -m "feat(server): broadcast engine events over existing /watch WebSocket"
```

---

### Task 2: Test multiple sequential events

**Files:**
- Modify: `internal/server/websocket_test.go`

- [ ] **Step 1: Add multi-event test**

This is coverage expansion, not TDD — the implementation from Task 1 should already handle multiple events.

```go
func TestWebSocketEngineEvents_MultipleEvents(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	srv.RegisterProject(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	pid := srv.projectID(dir)
	runsDir := filepath.Join(dir, ".ctx", "runs", "run-multi-001")
	os.MkdirAll(runsDir, 0755)

	wsURL := "ws" + ts.URL[4:] + "/api/projects/" + pid + "/watch"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Read initial state_changed
	_, _, _ = conn.Read(ctx)

	time.Sleep(100 * time.Millisecond)

	// Write multiple events in one batch
	logPath := filepath.Join(runsDir, "log.json")
	events := []map[string]interface{}{
		{"type": "pipeline-start", "timestamp": time.Now().Format(time.RFC3339Nano), "agent": "build-feature", "run-id": "run-multi-001"},
		{"type": "step-start", "timestamp": time.Now().Format(time.RFC3339Nano), "step": "scaffold", "step-type": "builtin", "run-id": "run-multi-001"},
		{"type": "step-end", "timestamp": time.Now().Format(time.RFC3339Nano), "step": "scaffold", "status": "success", "duration-ms": 1200, "run-id": "run-multi-001"},
	}
	f, _ := os.Create(logPath)
	for _, ev := range events {
		data, _ := json.Marshal(ev)
		f.Write(data)
		f.Write([]byte("\n"))
	}
	f.Close()

	// Should receive 3 engine_event messages
	received := 0
	for received < 3 {
		_, msg, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("expected more events, got %d: %v", received, err)
		}
		var wsMsg WSMessage
		json.Unmarshal(msg, &wsMsg)
		if wsMsg.Type == "engine_event" {
			received++
		}
	}
	if received != 3 {
		t.Fatalf("expected 3 engine events, got %d", received)
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestWebSocketEngineEvents -v`
Expected: Both event tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/server/websocket_test.go
git commit -m "test(server): add multi-event WebSocket broadcast test"
```

---

## Chunk 2: Flutter — Event Consumption and Wiring

### Task 3: Handle engine_event in ProjectStateNotifier and wire to RunsNotifier

**Files:**
- Modify: `ui/flutter/lib/providers/project.dart:43-85` (ProjectStateNotifier — add onEngineEvent callback)
- Modify: `ui/flutter/lib/providers/runs.dart` (add per-run events as part of state, add reactive events provider, wire callback)

- [ ] **Step 1: Add onEngineEvent callback to ProjectStateNotifier**

In `ui/flutter/lib/providers/project.dart`, add a `_onEngineEvent` callback field and handle `engine_event` in `_connectWs`. Changes to `ProjectStateNotifier`:

Add field:
```dart
  void Function(Map<String, dynamic>)? _onEngineEvent;
```

Add setter:
```dart
  set onEngineEvent(void Function(Map<String, dynamic>)? cb) =>
      _onEngineEvent = cb;
```

Add handler in `_connectWs` `onMessage` callback, after the `log_appended` check:
```dart
        if (data['type'] == 'engine_event' && data['event'] != null) {
          _onEngineEvent?.call(data['event'] as Map<String, dynamic>);
        }
```

- [ ] **Step 2: Make RunsNotifier events reactive**

In `ui/flutter/lib/providers/runs.dart`, store events per-run in a separate `StateNotifier` so the UI can reactively rebuild when events arrive. Add a `RunEventsNotifier` class and a family provider:

```dart
/// Stores engine events per run, keyed by runId.
class RunEventsNotifier extends StateNotifier<Map<String, List<EngineEvent>>> {
  RunEventsNotifier() : super({});

  void addEvent(EngineEvent event) {
    if (event.runId == null) return;
    final runId = event.runId!;
    final existing = state[runId] ?? [];
    state = {...state, runId: [...existing, event]};
  }

  List<EngineEvent> eventsForRun(String runId) => state[runId] ?? const [];
}

final runEventsProvider =
    StateNotifierProvider<RunEventsNotifier, Map<String, List<EngineEvent>>>((ref) {
  return RunEventsNotifier();
});

/// Reactive events for a specific run.
final runEventsFamily = Provider.family<List<EngineEvent>, String>((ref, runId) {
  final allEvents = ref.watch(runEventsProvider);
  return allEvents[runId] ?? const [];
});
```

- [ ] **Step 3: Update RunsNotifier.processEvent to also notify RunEventsNotifier**

Remove the internal `_events` map from `RunsNotifier` (it won't be used). Instead, event storage will happen in `RunEventsNotifier`. The `processEvent` method stays unchanged for run state tracking — event storage is handled at the wiring layer.

- [ ] **Step 4: Wire onEngineEvent callback**

In `ui/flutter/lib/providers/runs.dart`, add the wiring provider. Import `project.dart`:

```dart
import '../providers/project.dart';

/// Wires engine events from WebSocket into RunsNotifier and RunEventsNotifier.
/// Watch this provider to activate the connection.
final engineEventWiringProvider = Provider<void>((ref) {
  final projectState = ref.watch(projectStateProvider.notifier);
  final runsNotifier = ref.read(runsProvider.notifier);
  final eventsNotifier = ref.read(runEventsProvider.notifier);

  projectState.onEngineEvent = (data) {
    final event = EngineEvent.fromJson(data);
    runsNotifier.processEvent(event);
    eventsNotifier.addEvent(event);
  };

  ref.onDispose(() {
    projectState.onEngineEvent = null;
  });
});
```

- [ ] **Step 5: Activate the wiring in AppShell**

In `ui/flutter/lib/views/app_shell.dart`, add `ref.watch(engineEventWiringProvider)` in the `build()` method, after the existing `ref.watch` calls. The `runs.dart` import is already present on line 7 — no new import needed.

```dart
    // Activate engine event wiring
    ref.watch(engineEventWiringProvider);
```

- [ ] **Step 6: Run dart analyze**

Run: `cd ui/flutter && dart analyze lib/`
Expected: 0 issues.

- [ ] **Step 7: Commit**

```bash
git add ui/flutter/lib/providers/project.dart ui/flutter/lib/providers/runs.dart ui/flutter/lib/views/app_shell.dart
git commit -m "feat(ui): wire engine event WebSocket stream into RunsNotifier"
```

---

### Task 4: Wire events into DetailPanel timeline

**Files:**
- Modify: `ui/flutter/lib/views/project_workspace.dart:88-94` (pass events to DetailPanel reactively)

- [ ] **Step 1: Pass events reactively from runEventsFamily to DetailPanel**

In `ui/flutter/lib/views/project_workspace.dart`, use `ref.watch(runEventsFamily(...))` to get reactive events. Add the import for the family provider (already exported from `runs.dart` which is imported on line 8).

Replace the `DetailPanel` widget (lines 90-93):

```dart
              Expanded(
                flex: 45,
                child: DetailPanel(
                  selectedRun: selectedRun,
                  events: selectedRun != null
                      ? ref.watch(runEventsFamily(selectedRun.runId))
                      : const [],
                ),
              ),
```

- [ ] **Step 2: Run dart analyze**

Run: `cd ui/flutter && dart analyze lib/`
Expected: 0 issues.

- [ ] **Step 3: Commit**

```bash
git add ui/flutter/lib/views/project_workspace.dart
git commit -m "feat(ui): pipe run events into detail panel timeline view"
```

---

## Chunk 3: Build Verification

### Task 5: Full build and test verification

- [ ] **Step 1: Run all Go tests**

Run: `go test ./...`
Expected: All packages pass.

- [ ] **Step 2: Run Go vet**

Run: `go vet ./...`
Expected: No warnings.

- [ ] **Step 3: Run dart analyze**

Run: `cd ui/flutter && dart analyze lib/`
Expected: 0 issues.

- [ ] **Step 4: Build Go binary**

Run: `go build ./...`
Expected: Clean build, no errors.
