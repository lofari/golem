package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
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

func (s *Server) handleStateWatch(w http.ResponseWriter, r *http.Request) {
	proj, ok := s.getProject(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
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

	// Send initial state
	if state, err := golemctx.ReadState(proj.path); err == nil {
		msg, _ := json.Marshal(WSMessage{Type: "state_changed", State: state})
		conn.Write(ctx, websocket.MessageText, msg)
	}

	// Debounce timer to avoid rapid-fire updates
	var debounce *time.Timer

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

			// Check if it's a file we care about
			absPath, _ := filepath.Abs(event.Name)
			absState, _ := filepath.Abs(statePath)
			absLog, _ := filepath.Abs(logPath)

			isState := absPath == absState
			isLog := absPath == absLog

			if !isState && !isLog {
				continue
			}

			// Debounce: wait 200ms for writes to settle
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(200*time.Millisecond, func() {
				if isState {
					if state, err := golemctx.ReadState(proj.path); err == nil {
						msg, _ := json.Marshal(WSMessage{Type: "state_changed", State: state})
						conn.Write(ctx, websocket.MessageText, msg)
					}
				}
				if isLog {
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
