package server

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/creack/pty/v2"
	"github.com/fsnotify/fsnotify"
	"nhooyr.io/websocket"

	golemctx "github.com/lofari/golem/internal/ctx"
)

// WSMessage is a WebSocket message sent to clients.
type WSMessage struct {
	Type    string      `json:"type"`
	Data    string      `json:"data,omitempty"`    // base64-encoded PTY output
	Line    string      `json:"line,omitempty"`    // deprecated, kept for compat
	Cols    int         `json:"cols,omitempty"`
	Rows    int         `json:"rows,omitempty"`
	Code    *int        `json:"code,omitempty"`    // exit code
	State   interface{} `json:"state,omitempty"`
	Session interface{} `json:"session,omitempty"`
	Event   interface{} `json:"event,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func (s *Server) handleProcessStream(w http.ResponseWriter, r *http.Request) {
	procID := r.PathValue("procId")
	s.mu.RLock()
	mp, ok := s.processes[procID]
	s.mu.RUnlock()

	if !ok {
		writeError(w, http.StatusNotFound, "process not found")
		return
	}

	conn, err := websocket.Accept(w, r, nil)
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

// tailLogJSON reads new NDJSON lines from path starting at offset.
// Only advances offset past lines that are valid JSON; a partial
// trailing line is left for the next call.
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
	if _, err := f.Seek(offset, 0); err != nil {
		return nil, offset
	}
	var events []json.RawMessage
	var committed int64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		lineLen := int64(len(line)) + 1 // +1 for newline
		if len(line) == 0 {
			committed += lineLen
			continue
		}
		raw := make(json.RawMessage, len(line))
		copy(raw, line)
		if json.Valid(raw) {
			events = append(events, raw)
			committed += lineLen
		}
		// Invalid JSON (partial line) — don't advance offset
	}
	return events, offset + committed
}

func (s *Server) handleStateWatch(w http.ResponseWriter, r *http.Request) {
	proj, ok := s.getProject(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := r.Context()

	// Set up file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		msg, _ := json.Marshal(WSMessage{Type: "error", Error: "could not start watcher"})
		conn.Write(ctx, websocket.MessageText, msg)
		return
	}
	defer watcher.Close()

	statePath := filepath.Join(proj.path, ".ctx", "state.yaml")
	logPath := filepath.Join(proj.path, ".ctx", "log.yaml")
	ctxDir := filepath.Join(proj.path, ".ctx")

	// Watch the directory (fsnotify needs directory for rename-based writes)
	watcher.Add(ctxDir)

	// Runs directory watching
	runsDir, _ := filepath.Abs(filepath.Join(proj.path, ".ctx", "runs"))
	runsWatched := false
	logOffsets := make(map[string]int64)

	watchRunDirs := func() {
		info, err := os.Stat(runsDir)
		if err != nil || !info.IsDir() {
			return
		}
		if !runsWatched {
			watcher.Add(runsDir)
			runsWatched = true
		}
		entries, err := os.ReadDir(runsDir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				subDir := filepath.Join(runsDir, e.Name())
				logPath := filepath.Join(subDir, "log.json")
				if _, known := logOffsets[logPath]; !known {
					watcher.Add(subDir)
					logOffsets[logPath] = 0
				}
			}
		}
	}
	watchRunDirs()

	// Send initial state
	if state, err := golemctx.ReadState(proj.path); err == nil {
		msg, _ := json.Marshal(WSMessage{Type: "state_changed", State: state})
		conn.Write(ctx, websocket.MessageText, msg)
	}

	// Debounce timer to avoid rapid-fire updates
	var debounce *time.Timer
	var pendingState, pendingLog bool

	for {
		select {
		case <-ctx.Done():
			return
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

			isState := absPath == absState
			isLog := absPath == absLog

			// Check if this is a run log.json change
			if strings.HasPrefix(absPath, runsDir+string(os.PathSeparator)) &&
				filepath.Base(absPath) == "log.json" {
				// Immediate delivery — no debounce
				offset := logOffsets[absPath]
				events, newOffset := tailLogJSON(absPath, offset)
				logOffsets[absPath] = newOffset
				for _, ev := range events {
					msg, _ := json.Marshal(WSMessage{Type: "engine_event", Event: ev})
					conn.Write(ctx, websocket.MessageText, msg)
				}
				continue
			}

			// Check if this is a new subdirectory in runs/
			if strings.HasPrefix(absPath, runsDir+string(os.PathSeparator)) {
				watchRunDirs()
				continue
			}

			if !isState && !isLog {
				continue
			}

			// Accumulate pending flags
			if isState {
				pendingState = true
			}
			if isLog {
				pendingLog = true
			}

			// Debounce: wait 200ms for writes to settle.
			// Capture and clear flags before spawning the goroutine
			// to avoid a data race between the event loop and AfterFunc.
			if debounce != nil {
				debounce.Stop()
			}
			captureState := pendingState
			captureLog := pendingLog
			pendingState = false
			pendingLog = false
			debounce = time.AfterFunc(200*time.Millisecond, func() {
				if captureState {
					if state, err := golemctx.ReadState(proj.path); err == nil {
						msg, _ := json.Marshal(WSMessage{Type: "state_changed", State: state})
						conn.Write(ctx, websocket.MessageText, msg)
					}
				}
				if captureLog {
					if log, err := golemctx.ReadLog(proj.path); err == nil {
						sessions := log.Sessions
						if len(sessions) > 0 {
							msg, _ := json.Marshal(WSMessage{Type: "log_appended", Session: sessions[len(sessions)-1]})
							conn.Write(ctx, websocket.MessageText, msg)
						}
					}
				}
			})
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			msg, _ := json.Marshal(WSMessage{Type: "error", Error: fmt.Sprintf("watcher error: %v", err)})
			conn.Write(ctx, websocket.MessageText, msg)
		}
	}
}
