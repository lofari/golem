package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
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
	output *ringBuffer
	mu     sync.Mutex
	subs   map[chan string]struct{} // output subscribers
}

// ringBuffer is a simple circular buffer for output lines.
type ringBuffer struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func newRingBuffer(max int) *ringBuffer {
	return &ringBuffer{lines: make([]string, 0, max), max: max}
}

func (rb *ringBuffer) Write(line string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if len(rb.lines) >= rb.max {
		rb.lines = rb.lines[1:]
	}
	rb.lines = append(rb.lines, line)
}

func (rb *ringBuffer) Lines() []string {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	out := make([]string, len(rb.lines))
	copy(out, rb.lines)
	return out
}

// Subscribe returns a channel that receives output lines.
func (mp *managedProcess) Subscribe() chan string {
	ch := make(chan string, 256)
	mp.mu.Lock()
	mp.subs[ch] = struct{}{}
	mp.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (mp *managedProcess) Unsubscribe(ch chan string) {
	mp.mu.Lock()
	delete(mp.subs, ch)
	mp.mu.Unlock()
	close(ch)
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

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, golemBin, args...)
	cmd.Dir = proj.path

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
		output: newRingBuffer(10000),
		subs:   make(map[chan string]struct{}),
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	cmd.Stderr = cmd.Stdout // merge stderr into stdout

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("starting process: %w", err)
	}
	mp.info.PID = cmd.Process.Pid

	// Read output in background
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				line := string(buf[:n])
				mp.output.Write(line)
				mp.mu.Lock()
				for ch := range mp.subs {
					select {
					case ch <- line:
					default: // don't block
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
		mp.mu.Lock()
		if err != nil {
			mp.info.Status = "failed"
		} else {
			mp.info.Status = "stopped"
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
