package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty/v2"
)

// LaunchRequest is the body for POST /api/projects/:id/processes.
type LaunchRequest struct {
	Command string       `json:"command"` // code, review, qa, plan
	Config  LaunchConfig `json:"config"`
}

// LaunchConfig overrides for the launched process.
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

// ProcessInfo is the API representation of a managed process.
type ProcessInfo struct {
	ID        string    `json:"id"`
	Command   string    `json:"command"`
	Status    string    `json:"status"` // running, stopped, failed
	StartedAt time.Time `json:"startedAt"`
	PID       int       `json:"pid,omitempty"`
}

// managedProcess holds a running golem process.
type managedProcess struct {
	info   ProcessInfo
	cmd    *exec.Cmd
	cancel context.CancelFunc
	ptyF   *os.File // PTY file handle
	output *rawBuffer
	mu     sync.Mutex
	subs   map[chan []byte]struct{}
}

// rawBuffer is a circular byte buffer for PTY output.
type rawBuffer struct {
	mu   sync.Mutex
	buf  []byte
	size int
	w    int  // write position
	full bool // whether buffer has wrapped
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

// Subscribe returns a channel that receives output chunks.
func (mp *managedProcess) Subscribe() chan []byte {
	ch := make(chan []byte, 256)
	mp.mu.Lock()
	mp.subs[ch] = struct{}{}
	mp.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (mp *managedProcess) Unsubscribe(ch chan []byte) {
	mp.mu.Lock()
	delete(mp.subs, ch)
	mp.mu.Unlock()
	close(ch)
}

// reapProcesses removes finished processes older than the retention period.
func (s *Server) reapProcesses() {
	retention := s.cfg.ProcessRetention
	if retention == 0 {
		retention = 1 * time.Hour
	}
	cutoff := time.Now().Add(-retention)
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, mp := range s.processes {
		mp.mu.Lock()
		done := mp.info.Status != "running" && mp.info.StartedAt.Before(cutoff)
		mp.mu.Unlock()
		if done {
			delete(s.processes, id)
		}
	}
}

// StopAll cancels all running processes.
func (s *Server) StopAll() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, mp := range s.processes {
		mp.mu.Lock()
		if mp.info.Status == "running" {
			mp.cancel()
		}
		mp.mu.Unlock()
	}
}

func (s *Server) launchProcess(proj *project, req LaunchRequest) (*managedProcess, error) {
	validCommands := map[string]bool{"code": true, "review": true, "qa": true, "plan": true}
	if !validCommands[req.Command] {
		return nil, fmt.Errorf("invalid command: %q", req.Command)
	}

	golemBin, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("finding golem binary: %w", err)
	}

	args := []string{req.Command}
	if req.Config.MaxIterations > 0 {
		args = append(args, "--max-iterations", fmt.Sprintf("%d", req.Config.MaxIterations))
	}
	if req.Config.MaxToolCalls > 0 {
		args = append(args, "--max-tool-calls", fmt.Sprintf("%d", req.Config.MaxToolCalls))
	}
	if req.Config.Model != "" {
		args = append(args, "--model", req.Config.Model)
	}
	if req.Config.Task != "" {
		args = append(args, "--task", req.Config.Task)
	}
	if req.Config.Sandbox {
		args = append(args, "--sandbox")
	}
	if req.Config.Parallel > 1 {
		args = append(args, "--parallel", fmt.Sprintf("%d", req.Config.Parallel))
	}
	if req.Config.PluginDir != "" {
		args = append(args, "--plugin-dir", req.Config.PluginDir)
	}

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
}

func (s *Server) handleLaunchProcess(w http.ResponseWriter, r *http.Request) {
	proj, ok := s.getProject(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	var req LaunchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	mp, err := s.launchProcess(proj, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": mp.info.ID})
}

func (s *Server) handleListProcesses(w http.ResponseWriter, r *http.Request) {
	s.reapProcesses()
	projID := r.PathValue("id")
	s.mu.RLock()
	defer s.mu.RUnlock()

	var infos []ProcessInfo
	prefix := projID
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	for _, mp := range s.processes {
		// Filter by project (process ID starts with project ID prefix)
		if len(mp.info.ID) >= len(prefix) && mp.info.ID[:len(prefix)] == prefix {
			mp.mu.Lock()
			infos = append(infos, mp.info)
			mp.mu.Unlock()
		}
	}
	if infos == nil {
		infos = []ProcessInfo{}
	}
	writeJSON(w, http.StatusOK, infos)
}

func (s *Server) handleStopProcess(w http.ResponseWriter, r *http.Request) {
	procID := r.PathValue("procId")
	s.mu.RLock()
	mp, ok := s.processes[procID]
	s.mu.RUnlock()

	if !ok {
		writeError(w, http.StatusNotFound, "process not found")
		return
	}

	mp.cancel()
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopping"})
}
