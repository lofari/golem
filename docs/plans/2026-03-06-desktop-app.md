# Desktop App Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a desktop application for golem with a Go API server (`golem serve`) and a Tauri 2 + React frontend that monitors and manages multiple golem processes.

**Architecture:** Go HTTP/WebSocket server exposes process management and state watching. Tauri 2 desktop shell wraps a React frontend that connects to the server. The CLI remains unchanged — the app wraps it.

**Tech Stack:** Go (net/http, nhooyr.io/websocket, fsnotify), Tauri 2, React 19, TypeScript, Tailwind CSS, Zustand, Vite

**Design doc:** `docs/plans/2026-03-06-desktop-app-design.md`

---

## Task 1: Go Server — HTTP Foundation

Create the `internal/server` package with a basic HTTP server, CORS middleware, and health endpoint.

**Files:**
- Create: `internal/server/server.go`
- Create: `internal/server/server_test.go`

**Step 1: Write the failing test**

```go
// internal/server/server_test.go
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	srv := New(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", body["status"])
	}
}

func TestCORSHeaders(t *testing.T) {
	srv := New(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("missing CORS header")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -v`
Expected: FAIL — package doesn't exist

**Step 3: Write the implementation**

```go
// internal/server/server.go
package server

import (
	"encoding/json"
	"net"
	"net/http"
)

// Config holds server configuration.
type Config struct {
	Addr string // listen address, default ":8314"
}

// Server is the golem API server.
type Server struct {
	cfg    Config
	mux    *http.ServeMux
}

// New creates a new Server.
func New(cfg Config) *Server {
	if cfg.Addr == "" {
		cfg.Addr = ":8314"
	}
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Handler returns the HTTP handler with CORS middleware.
func (s *Server) Handler() http.Handler {
	return cors(s.mux)
}

// ListenAndServe starts the server.
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	return http.Serve(ln, s.Handler())
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/server/server.go internal/server/server_test.go
git commit -m "feat(server): add HTTP server foundation with health endpoint and CORS"
```

---

## Task 2: Go Server — Project Registry

Add project registration, listing, and state/log/config reading via REST endpoints.

**Files:**
- Create: `internal/server/projects.go`
- Create: `internal/server/projects_test.go`
- Modify: `internal/server/server.go` (add routes)

**Step 1: Write the failing tests**

```go
// internal/server/projects_test.go
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func setupTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ctxDir := filepath.Join(dir, ".ctx")
	os.MkdirAll(ctxDir, 0755)
	os.WriteFile(filepath.Join(ctxDir, "state.yaml"), []byte(`
project:
  name: test-project
  summary: test
  stack: Go
  docs_path: docs/
status:
  phase: building
  current_focus: auth
tasks:
  - name: auth
    status: in-progress
  - name: api
    status: todo
`), 0644)
	os.WriteFile(filepath.Join(ctxDir, "log.yaml"), []byte(`
sessions:
  - iteration: 1
    timestamp: "2026-03-06T10:00:00Z"
    task: auth
    outcome: partial
    summary: started auth
`), 0644)
	return dir
}

func TestRegisterAndListProjects(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Register project
	body, _ := json.Marshal(map[string]string{"path": dir})
	resp, err := http.Post(ts.URL+"/api/projects", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d", resp.StatusCode)
	}

	// List projects
	resp, err = http.Get(ts.URL + "/api/projects")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var projects []ProjectInfo
	json.NewDecoder(resp.Body).Decode(&projects)
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Name != "test-project" {
		t.Fatalf("expected name test-project, got %q", projects[0].Name)
	}
}

func TestGetProjectState(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	srv.RegisterProject(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/projects/" + srv.projectID(dir) + "/state")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestGetProjectLog(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	srv.RegisterProject(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/projects/" + srv.projectID(dir) + "/log")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRegisterInvalidPath(t *testing.T) {
	srv := New(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body, _ := json.Marshal(map[string]string{"path": "/nonexistent"})
	resp, _ := http.Post(ts.URL+"/api/projects", "application/json", bytes.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -v -run TestRegister`
Expected: FAIL

**Step 3: Write the implementation**

```go
// internal/server/projects.go
package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	golemctx "github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/config"
)

// ProjectInfo is the API representation of a registered project.
type ProjectInfo struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Name  string `json:"name"`
	Phase string `json:"phase"`
}

// project holds internal state for a registered project.
type project struct {
	id   string
	path string
}

// RegisterProject adds a project directory to the registry.
func (s *Server) RegisterProject(dir string) error {
	ctxDir := filepath.Join(dir, ".ctx")
	if _, err := os.Stat(ctxDir); err != nil {
		return fmt.Errorf("no .ctx/ directory at %s", dir)
	}
	id := s.projectID(dir)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects[id] = &project{id: id, path: dir}
	return nil
}

func (s *Server) projectID(dir string) string {
	h := sha256.Sum256([]byte(dir))
	return hex.EncodeToString(h[:8])
}

func (s *Server) getProject(id string) (*project, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.projects[id]
	return p, ok
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var infos []ProjectInfo
	for _, p := range s.projects {
		state, err := golemctx.ReadState(p.path)
		info := ProjectInfo{ID: p.id, Path: p.path}
		if err == nil {
			info.Name = state.Project.Name
			info.Phase = state.Status.Phase
		}
		infos = append(infos, info)
	}
	writeJSON(w, http.StatusOK, infos)
}

func (s *Server) handleRegisterProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.RegisterProject(body.Path); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id := s.projectID(body.Path)
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) handleGetState(w http.ResponseWriter, r *http.Request) {
	p, ok := s.getProject(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	state, err := golemctx.ReadState(p.path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleGetLog(w http.ResponseWriter, r *http.Request) {
	p, ok := s.getProject(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	log, err := golemctx.ReadLog(p.path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, log)
}

func (s *Server) handleGetProjectConfig(w http.ResponseWriter, r *http.Request) {
	p, ok := s.getProject(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	cfg := config.Load(config.GlobalPath(), config.ProjectPath(p.path))
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleUpdateProjectConfig(w http.ResponseWriter, r *http.Request) {
	p, ok := s.getProject(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	var cfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := config.WriteFile(config.ProjectPath(p.path), cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleGetGlobalConfig(w http.ResponseWriter, r *http.Request) {
	cfg := config.Load(config.GlobalPath(), "")
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleUpdateGlobalConfig(w http.ResponseWriter, r *http.Request) {
	var cfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := config.WriteFile(config.GlobalPath(), cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}
```

Then update `server.go` to add the `mu` and `projects` fields and new routes:

```go
// In server.go — update Server struct and New():
type Server struct {
	cfg      Config
	mux      *http.ServeMux
	mu       sync.RWMutex
	projects map[string]*project
}

func New(cfg Config) *Server {
	if cfg.Addr == "" {
		cfg.Addr = ":8314"
	}
	s := &Server{
		cfg:      cfg,
		mux:      http.NewServeMux(),
		projects: make(map[string]*project),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)

	// Projects
	s.mux.HandleFunc("GET /api/projects", s.handleListProjects)
	s.mux.HandleFunc("POST /api/projects", s.handleRegisterProject)
	s.mux.HandleFunc("GET /api/projects/{id}/state", s.handleGetState)
	s.mux.HandleFunc("GET /api/projects/{id}/log", s.handleGetLog)
	s.mux.HandleFunc("GET /api/projects/{id}/config", s.handleGetProjectConfig)
	s.mux.HandleFunc("PUT /api/projects/{id}/config", s.handleUpdateProjectConfig)

	// Global config
	s.mux.HandleFunc("GET /api/config", s.handleGetGlobalConfig)
	s.mux.HandleFunc("PUT /api/config", s.handleUpdateGlobalConfig)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/server/projects.go internal/server/projects_test.go internal/server/server.go
git commit -m "feat(server): add project registry with state, log, and config endpoints"
```

---

## Task 3: Go Server — Process Management

Add the ability to launch, list, and stop golem processes. Each process captures stdout/stderr into a ring buffer for streaming.

**Files:**
- Create: `internal/server/process.go`
- Create: `internal/server/process_test.go`
- Modify: `internal/server/server.go` (add routes, process map)

**Step 1: Write the failing tests**

```go
// internal/server/process_test.go
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLaunchAndListProcesses(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	srv.RegisterProject(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	pid := srv.projectID(dir)

	// Launch a process (use "echo" as a test command instead of real golem)
	body, _ := json.Marshal(LaunchRequest{
		Command: "code",
		Config:  LaunchConfig{MaxIterations: 5},
	})
	resp, err := http.Post(ts.URL+"/api/projects/"+pid+"/processes", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["id"] == "" {
		t.Fatal("expected process ID")
	}

	// List processes
	resp2, err := http.Get(ts.URL + "/api/projects/" + pid + "/processes")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	var procs []ProcessInfo
	json.NewDecoder(resp2.Body).Decode(&procs)
	if len(procs) != 1 {
		t.Fatalf("expected 1 process, got %d", len(procs))
	}
	if procs[0].Command != "code" {
		t.Fatalf("expected command 'code', got %q", procs[0].Command)
	}
}

func TestStopProcess(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	srv.RegisterProject(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	pid := srv.projectID(dir)

	// Launch
	body, _ := json.Marshal(LaunchRequest{Command: "code"})
	resp, _ := http.Post(ts.URL+"/api/projects/"+pid+"/processes", "application/json", bytes.NewReader(body))
	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	procID := result["id"]

	// Stop
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/projects/"+pid+"/processes/"+procID, nil)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}

	// Wait a moment and verify it's gone
	time.Sleep(100 * time.Millisecond)
	resp3, _ := http.Get(ts.URL + "/api/projects/" + pid + "/processes")
	var procs []ProcessInfo
	json.NewDecoder(resp3.Body).Decode(&procs)
	resp3.Body.Close()

	for _, p := range procs {
		if p.ID == procID && p.Status == "running" {
			t.Fatal("process should not be running after stop")
		}
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -v -run TestLaunch`
Expected: FAIL

**Step 3: Write the implementation**

```go
// internal/server/process.go
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
	if s.processes == nil {
		s.processes = make(map[string]*managedProcess)
	}
	s.processes[id] = mp
	s.mu.Unlock()

	return mp, nil
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
	for _, mp := range s.processes {
		// Filter by project (process ID starts with project ID prefix)
		if len(mp.info.ID) > 8 && mp.info.ID[:len(projID)] == projID[:min(len(projID), 8)] {
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
```

Update `server.go` to add the `processes` field and routes:

```go
// In Server struct:
type Server struct {
	cfg       Config
	mux       *http.ServeMux
	mu        sync.RWMutex
	projects  map[string]*project
	processes map[string]*managedProcess
}

// In New():
s := &Server{
	cfg:       cfg,
	mux:       http.NewServeMux(),
	projects:  make(map[string]*project),
	processes: make(map[string]*managedProcess),
}

// In routes():
s.mux.HandleFunc("POST /api/projects/{id}/processes", s.handleLaunchProcess)
s.mux.HandleFunc("GET /api/projects/{id}/processes", s.handleListProcesses)
s.mux.HandleFunc("DELETE /api/projects/{id}/processes/{procId}", s.handleStopProcess)
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/server/process.go internal/server/process_test.go internal/server/server.go
git commit -m "feat(server): add process management — launch, list, and stop golem processes"
```

---

## Task 4: Go Server — WebSocket Output Streaming

Add WebSocket endpoints for real-time process output streaming and state change watching via fsnotify.

**Files:**
- Create: `internal/server/websocket.go`
- Create: `internal/server/websocket_test.go`
- Modify: `internal/server/server.go` (add routes)
- Modify: `go.mod` (add nhooyr.io/websocket, fsnotify)

**Step 1: Add dependencies**

Run: `go get nhooyr.io/websocket github.com/fsnotify/fsnotify`

**Step 2: Write the failing tests**

```go
// internal/server/websocket_test.go
package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"nhooyr.io/websocket"
)

func TestWebSocketProcessStream(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	srv.RegisterProject(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	pid := srv.projectID(dir)

	// Launch a dummy process that produces output
	mp := &managedProcess{
		info: ProcessInfo{
			ID:      "test-proc",
			Command: "code",
			Status:  "running",
		},
		output: newRingBuffer(100),
		subs:   make(map[chan string]struct{}),
	}
	srv.mu.Lock()
	srv.processes["test-proc"] = mp
	srv.mu.Unlock()

	// Connect WebSocket
	wsURL := "ws" + ts.URL[4:] + "/api/projects/" + pid + "/processes/test-proc/stream"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Write output to the process
	mp.output.Write("hello world\n")
	ch := mp.Subscribe()
	go func() {
		time.Sleep(50 * time.Millisecond)
		mp.mu.Lock()
		for sub := range mp.subs {
			select {
			case sub <- "test line\n":
			default:
			}
		}
		mp.mu.Unlock()
	}()
	mp.Unsubscribe(ch)

	// Read from WebSocket — should get backlog first
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
}

type WSMessage struct {
	Type string `json:"type"`
	Line string `json:"line,omitempty"`
}
```

**Step 3: Write the implementation**

```go
// internal/server/websocket.go
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"nhooyr.io/websocket"

	golemctx "github.com/lofari/golem/internal/ctx"
)

// WSMessage is a WebSocket message sent to clients.
type WSMessage struct {
	Type    string      `json:"type"`
	Line    string      `json:"line,omitempty"`
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
		InsecureSkipVerify: true, // Allow any origin for local dev
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := r.Context()

	// Send backlog
	for _, line := range mp.output.Lines() {
		msg, _ := json.Marshal(WSMessage{Type: "output", Line: line})
		if err := conn.Write(ctx, websocket.MessageText, msg); err != nil {
			return
		}
	}

	// Subscribe to new output
	ch := mp.Subscribe()
	defer mp.Unsubscribe(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-ch:
			if !ok {
				return
			}
			msg, _ := json.Marshal(WSMessage{Type: "output", Line: line})
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
```

Update `server.go` routes:

```go
// In routes():
s.mux.HandleFunc("GET /api/projects/{id}/processes/{procId}/stream", s.handleProcessStream)
s.mux.HandleFunc("GET /api/projects/{id}/watch", s.handleStateWatch)
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/server/websocket.go internal/server/websocket_test.go internal/server/server.go go.mod go.sum
git commit -m "feat(server): add WebSocket streaming for process output and state watching"
```

---

## Task 5: Go Server — `golem serve` Command

Wire the server into the CLI as a new cobra command.

**Files:**
- Create: `cmd/serve.go`
- Modify: `internal/server/server.go` (add Shutdown support)

**Step 1: Write the command**

```go
// cmd/serve.go
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/lofari/golem/internal/server"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the golem API server for the desktop app",
	Long:  "Starts an HTTP/WebSocket server that manages golem processes and exposes project state.",
	RunE: func(cmd *cobra.Command, args []string) error {
		addr, _ := cmd.Flags().GetString("addr")

		srv := server.New(server.Config{Addr: addr})

		// Auto-register current directory if it has .ctx/
		dir, _ := os.Getwd()
		if _, err := os.Stat(dir + "/.ctx"); err == nil {
			srv.RegisterProject(dir)
			fmt.Fprintf(os.Stderr, "golem serve: registered project at %s\n", dir)
		}

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		fmt.Fprintf(os.Stderr, "golem serve: listening on %s\n", addr)

		errCh := make(chan error, 1)
		go func() {
			errCh <- srv.ListenAndServe()
		}()

		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "\ngolem serve: shutting down\n")
			return nil
		case err := <-errCh:
			return err
		}
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().String("addr", ":8314", "listen address")
}
```

**Step 2: Build and test manually**

Run: `go build ./... && go run . serve --help`
Expected: Shows serve command help

**Step 3: Commit**

```bash
git add cmd/serve.go
git commit -m "feat(cmd): add golem serve command for desktop app API server"
```

---

## Task 6: Tauri + React — Project Scaffolding

Set up the `ui/` directory with Tauri 2, React 19, TypeScript, Vite, and Tailwind CSS.

**Prerequisites:** `npm`, `cargo`, and Tauri CLI (`cargo install tauri-cli@^2`) must be installed.

**Step 1: Create the Tauri + React project**

```bash
cd /home/winler/projects/golem
npm create tauri-app@latest ui -- --template react-ts --manager npm
```

If the interactive setup asks questions:
- Project name: `golem-ui`
- Package manager: `npm`
- UI template: `React`
- UI flavor: `TypeScript`

**Step 2: Install additional dependencies**

```bash
cd ui
npm install zustand tailwindcss @tailwindcss/vite
```

**Step 3: Configure Tailwind**

Update `ui/vite.config.ts`:

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

const host = process.env.TAURI_DEV_HOST;

export default defineConfig(async () => ({
  plugins: [react(), tailwindcss()],
  clearScreen: false,
  server: {
    port: 1420,
    strictPort: true,
    host: host || false,
    hmr: host ? { protocol: "ws", host, port: 1421 } : undefined,
    watch: { ignored: ["**/src-tauri/**"] },
  },
}));
```

Replace `ui/src/styles.css` (or equivalent main CSS file) with:

```css
@import "tailwindcss";

:root {
  --bg-primary: #0d1117;
  --bg-surface: #161b22;
  --bg-elevated: #1c2128;
  --border: #30363d;
  --text-primary: #e6edf3;
  --text-secondary: #8b949e;
  --accent: #58a6ff;
  --green: #3fb950;
  --yellow: #d29922;
  --red: #f85149;
}

body {
  margin: 0;
  background: var(--bg-primary);
  color: var(--text-primary);
  font-family: "Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}

/* Scrollbar styling */
::-webkit-scrollbar {
  width: 6px;
}
::-webkit-scrollbar-track {
  background: var(--bg-primary);
}
::-webkit-scrollbar-thumb {
  background: var(--border);
  border-radius: 3px;
}
```

**Step 4: Configure Tauri window**

Update `ui/src-tauri/tauri.conf.json` — set the window title, dimensions, and dark mode:

```json
{
  "productName": "Golem",
  "version": "0.1.0",
  "identifier": "com.golem.desktop",
  "build": {
    "frontendDist": "../dist",
    "devUrl": "http://localhost:1420",
    "beforeDevCommand": "npm run dev",
    "beforeBuildCommand": "npm run build"
  },
  "app": {
    "windows": [
      {
        "title": "Golem",
        "width": 1200,
        "height": 800,
        "minWidth": 900,
        "minHeight": 600,
        "decorations": true,
        "transparent": false
      }
    ]
  }
}
```

**Step 5: Verify the setup works**

```bash
cd ui && npm run dev
# In another terminal:
cd ui && cargo tauri dev
```

Expected: A desktop window opens showing the default React app with dark background.

**Step 6: Commit**

```bash
cd /home/winler/projects/golem
echo "ui/node_modules/" >> .gitignore
echo "ui/dist/" >> .gitignore
echo "ui/src-tauri/target/" >> .gitignore
git add ui/ .gitignore
git commit -m "feat(ui): scaffold Tauri 2 + React + TypeScript + Tailwind project"
```

---

## Task 7: React — API Client and Store

Create the Zustand store and API client hooks that connect to `golem serve`.

**Files:**
- Create: `ui/src/lib/api.ts`
- Create: `ui/src/stores/appStore.ts`
- Create: `ui/src/hooks/useWebSocket.ts`

**Step 1: Create the API client**

```ts
// ui/src/lib/api.ts
const BASE_URL = "http://localhost:8314";

export interface ProjectInfo {
  id: string;
  path: string;
  name: string;
  phase: string;
}

export interface ProcessInfo {
  id: string;
  command: string;
  status: string;
  startedAt: string;
  pid?: number;
}

export interface Task {
  name: string;
  status: string;
  notes?: string;
  depends_on?: string[];
  blocked_reason?: string;
}

export interface State {
  project: { name: string; summary: string; stack: string; docs_path: string };
  status: { current_focus: string; phase: string; last_session: string };
  decisions: { what: string; why: string; when: string }[];
  locked: { path: string; note: string }[];
  tasks: Task[];
  pitfalls: { what: string; fix: string }[];
}

export interface Session {
  iteration: number;
  timestamp: string;
  task: string;
  outcome: string;
  summary: string;
  files_changed?: string[];
}

export interface LaunchConfig {
  command: string;
  config: {
    maxIterations?: number;
    maxToolCalls?: number;
    model?: string;
    task?: string;
    sandbox?: boolean;
    mcp?: boolean;
    parallel?: number;
  };
}

export interface GolemConfig {
  "max-iterations": number;
  "max-tool-calls": number;
  verbose: boolean;
  sandbox: boolean;
  "sandbox-tools"?: string[];
  "sandbox-timeout"?: string;
  "sandbox-memory"?: string;
  mcp: boolean;
  parallel: number;
  "plugin-dir"?: string[];
  model: string;
}

async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(`${BASE_URL}${path}`, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: resp.statusText }));
    throw new Error(err.error || resp.statusText);
  }
  return resp.json();
}

export const api = {
  health: () => fetchJSON<{ status: string }>("/api/health"),

  listProjects: () => fetchJSON<ProjectInfo[]>("/api/projects"),
  registerProject: (path: string) =>
    fetchJSON<{ id: string }>("/api/projects", {
      method: "POST",
      body: JSON.stringify({ path }),
    }),
  getState: (projectId: string) =>
    fetchJSON<State>(`/api/projects/${projectId}/state`),
  getLog: (projectId: string) =>
    fetchJSON<{ sessions: Session[] }>(`/api/projects/${projectId}/log`),
  getProjectConfig: (projectId: string) =>
    fetchJSON<GolemConfig>(`/api/projects/${projectId}/config`),
  updateProjectConfig: (projectId: string, config: Partial<GolemConfig>) =>
    fetchJSON<{ status: string }>(`/api/projects/${projectId}/config`, {
      method: "PUT",
      body: JSON.stringify(config),
    }),

  listProcesses: (projectId: string) =>
    fetchJSON<ProcessInfo[]>(`/api/projects/${projectId}/processes`),
  launchProcess: (projectId: string, config: LaunchConfig) =>
    fetchJSON<{ id: string }>(`/api/projects/${projectId}/processes`, {
      method: "POST",
      body: JSON.stringify(config),
    }),
  stopProcess: (projectId: string, processId: string) =>
    fetchJSON<{ status: string }>(
      `/api/projects/${projectId}/processes/${processId}`,
      { method: "DELETE" }
    ),

  getGlobalConfig: () => fetchJSON<GolemConfig>("/api/config"),
  updateGlobalConfig: (config: Partial<GolemConfig>) =>
    fetchJSON<{ status: string }>("/api/config", {
      method: "PUT",
      body: JSON.stringify(config),
    }),
};

export function processStreamURL(projectId: string, processId: string): string {
  return `ws://localhost:8314/api/projects/${projectId}/processes/${processId}/stream`;
}

export function stateWatchURL(projectId: string): string {
  return `ws://localhost:8314/api/projects/${projectId}/watch`;
}
```

**Step 2: Create the Zustand store**

```ts
// ui/src/stores/appStore.ts
import { create } from "zustand";
import type { ProjectInfo, ProcessInfo, State, Session } from "../lib/api";

interface AppState {
  // Connection
  connected: boolean;
  setConnected: (connected: boolean) => void;

  // Projects
  projects: ProjectInfo[];
  setProjects: (projects: ProjectInfo[]) => void;
  selectedProjectId: string | null;
  selectProject: (id: string) => void;

  // Processes
  processes: ProcessInfo[];
  setProcesses: (processes: ProcessInfo[]) => void;
  selectedProcessId: string | null;
  selectProcess: (id: string | null) => void;

  // State
  projectState: State | null;
  setProjectState: (state: State) => void;

  // Log
  sessions: Session[];
  setSessions: (sessions: Session[]) => void;
  addSession: (session: Session) => void;

  // Output
  outputLines: Map<string, string[]>;
  appendOutput: (processId: string, line: string) => void;
  clearOutput: (processId: string) => void;
}

export const useAppStore = create<AppState>((set) => ({
  connected: false,
  setConnected: (connected) => set({ connected }),

  projects: [],
  setProjects: (projects) => set({ projects }),
  selectedProjectId: null,
  selectProject: (id) => set({ selectedProjectId: id, selectedProcessId: null }),

  processes: [],
  setProcesses: (processes) => set({ processes }),
  selectedProcessId: null,
  selectProcess: (id) => set({ selectedProcessId: id }),

  projectState: null,
  setProjectState: (projectState) => set({ projectState }),

  sessions: [],
  setSessions: (sessions) => set({ sessions }),
  addSession: (session) =>
    set((s) => ({ sessions: [...s.sessions, session] })),

  outputLines: new Map(),
  appendOutput: (processId, line) =>
    set((s) => {
      const lines = new Map(s.outputLines);
      const existing = lines.get(processId) || [];
      // Keep last 5000 lines
      const updated = [...existing, line].slice(-5000);
      lines.set(processId, updated);
      return { outputLines: lines };
    }),
  clearOutput: (processId) =>
    set((s) => {
      const lines = new Map(s.outputLines);
      lines.delete(processId);
      return { outputLines: lines };
    }),
}));
```

**Step 3: Create the WebSocket hook**

```ts
// ui/src/hooks/useWebSocket.ts
import { useEffect, useRef, useCallback } from "react";

interface UseWebSocketOptions {
  url: string | null;
  onMessage: (data: unknown) => void;
  onOpen?: () => void;
  onClose?: () => void;
}

export function useWebSocket({ url, onMessage, onOpen, onClose }: UseWebSocketOptions) {
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeout = useRef<ReturnType<typeof setTimeout>>();

  const connect = useCallback(() => {
    if (!url) return;

    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      onOpen?.();
    };

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        onMessage(data);
      } catch {
        // ignore non-JSON messages
      }
    };

    ws.onclose = () => {
      onClose?.();
      // Reconnect after 2 seconds
      reconnectTimeout.current = setTimeout(connect, 2000);
    };

    ws.onerror = () => {
      ws.close();
    };
  }, [url, onMessage, onOpen, onClose]);

  useEffect(() => {
    connect();
    return () => {
      clearTimeout(reconnectTimeout.current);
      wsRef.current?.close();
    };
  }, [connect]);

  return wsRef;
}
```

**Step 4: Commit**

```bash
cd /home/winler/projects/golem
git add ui/src/lib/api.ts ui/src/stores/appStore.ts ui/src/hooks/useWebSocket.ts
git commit -m "feat(ui): add API client, Zustand store, and WebSocket hook"
```

---

## Task 8: React — App Shell and Sidebar

Build the main app layout with the project sidebar.

**Files:**
- Create: `ui/src/components/Sidebar.tsx`
- Create: `ui/src/components/ConnectionStatus.tsx`
- Modify: `ui/src/App.tsx`

**Step 1: Create the Sidebar component**

```tsx
// ui/src/components/Sidebar.tsx
import { useAppStore } from "../stores/appStore";
import type { ProjectInfo } from "../lib/api";

function ProjectItem({ project }: { project: ProjectInfo }) {
  const { selectedProjectId, selectProject } = useAppStore();
  const isSelected = selectedProjectId === project.id;

  return (
    <button
      onClick={() => selectProject(project.id)}
      className={`w-full text-left px-3 py-2 rounded text-sm transition-colors ${
        isSelected
          ? "bg-[var(--bg-elevated)] text-[var(--text-primary)]"
          : "text-[var(--text-secondary)] hover:bg-[var(--bg-elevated)] hover:text-[var(--text-primary)]"
      }`}
    >
      <div className="font-medium truncate">{project.name || project.path.split("/").pop()}</div>
      <div className="text-xs text-[var(--text-secondary)] truncate">{project.phase || "—"}</div>
    </button>
  );
}

export function Sidebar() {
  const { projects } = useAppStore();

  return (
    <div className="w-48 min-w-48 border-r border-[var(--border)] bg-[var(--bg-surface)] flex flex-col">
      <div className="px-3 py-3 text-xs font-semibold uppercase tracking-wider text-[var(--text-secondary)] border-b border-[var(--border)]">
        Projects
      </div>
      <div className="flex-1 overflow-y-auto p-2 space-y-1">
        {projects.length === 0 ? (
          <div className="text-xs text-[var(--text-secondary)] px-3 py-4 text-center">
            No projects registered
          </div>
        ) : (
          projects.map((p) => <ProjectItem key={p.id} project={p} />)
        )}
      </div>
    </div>
  );
}
```

**Step 2: Create the ConnectionStatus component**

```tsx
// ui/src/components/ConnectionStatus.tsx
import { useAppStore } from "../stores/appStore";

export function ConnectionStatus() {
  const { connected, projects, processes } = useAppStore();

  return (
    <div className="h-7 border-t border-[var(--border)] bg-[var(--bg-surface)] flex items-center px-3 text-xs text-[var(--text-secondary)]">
      <span
        className={`inline-block w-2 h-2 rounded-full mr-2 ${
          connected ? "bg-[var(--green)]" : "bg-[var(--red)]"
        }`}
      />
      {connected
        ? `golem serve · ${projects.length} project${projects.length !== 1 ? "s" : ""} · ${processes.length} process${processes.length !== 1 ? "es" : ""}`
        : "Disconnected — start golem serve"}
    </div>
  );
}
```

**Step 3: Update App.tsx**

```tsx
// ui/src/App.tsx
import { useEffect } from "react";
import { Sidebar } from "./components/Sidebar";
import { ConnectionStatus } from "./components/ConnectionStatus";
import { useAppStore } from "./stores/appStore";
import { api } from "./lib/api";

function App() {
  const { setConnected, setProjects, selectedProjectId } = useAppStore();

  useEffect(() => {
    let mounted = true;
    let interval: ReturnType<typeof setInterval>;

    async function poll() {
      try {
        await api.health();
        if (!mounted) return;
        setConnected(true);
        const projects = await api.listProjects();
        if (mounted) setProjects(projects);
      } catch {
        if (mounted) setConnected(false);
      }
    }

    poll();
    interval = setInterval(poll, 5000);

    return () => {
      mounted = false;
      clearInterval(interval);
    };
  }, [setConnected, setProjects]);

  return (
    <div className="h-screen flex flex-col bg-[var(--bg-primary)]">
      <div className="flex-1 flex overflow-hidden">
        <Sidebar />
        <main className="flex-1 flex items-center justify-center">
          {selectedProjectId ? (
            <div className="text-[var(--text-secondary)]">Project view coming soon</div>
          ) : (
            <div className="text-center text-[var(--text-secondary)]">
              <div className="text-lg mb-2">Select a project</div>
              <div className="text-sm">or register one via golem serve</div>
            </div>
          )}
        </main>
      </div>
      <ConnectionStatus />
    </div>
  );
}

export default App;
```

**Step 4: Verify visually**

Run: `cd ui && npm run dev` (with `golem serve` running in another terminal)
Expected: Dark UI with sidebar showing projects and a status bar at the bottom.

**Step 5: Commit**

```bash
cd /home/winler/projects/golem
git add ui/src/components/Sidebar.tsx ui/src/components/ConnectionStatus.tsx ui/src/App.tsx
git commit -m "feat(ui): add app shell with project sidebar and connection status"
```

---

## Task 9: React — Process Tabs and Output Pane

Build the process tabs and streaming output pane for the selected project.

**Files:**
- Create: `ui/src/components/ProcessTabs.tsx`
- Create: `ui/src/components/OutputPane.tsx`
- Create: `ui/src/components/ProjectView.tsx`
- Modify: `ui/src/App.tsx` (use ProjectView)

**Step 1: Create ProcessTabs**

```tsx
// ui/src/components/ProcessTabs.tsx
import { useAppStore } from "../stores/appStore";
import type { ProcessInfo } from "../lib/api";

const statusColors: Record<string, string> = {
  running: "bg-[var(--green)]",
  stopped: "bg-[var(--text-secondary)]",
  failed: "bg-[var(--red)]",
};

function ProcessTab({ process }: { process: ProcessInfo }) {
  const { selectedProcessId, selectProcess } = useAppStore();
  const isSelected = selectedProcessId === process.id;

  return (
    <button
      onClick={() => selectProcess(process.id)}
      className={`flex items-center gap-2 px-3 py-1.5 text-sm rounded-t border-b-2 transition-colors ${
        isSelected
          ? "bg-[var(--bg-surface)] text-[var(--text-primary)] border-[var(--accent)]"
          : "text-[var(--text-secondary)] border-transparent hover:text-[var(--text-primary)]"
      }`}
    >
      <span className={`w-2 h-2 rounded-full ${statusColors[process.status] || ""}`} />
      {process.command}
    </button>
  );
}

interface ProcessTabsProps {
  onLaunch: () => void;
}

export function ProcessTabs({ onLaunch }: ProcessTabsProps) {
  const { processes } = useAppStore();

  return (
    <div className="flex items-center gap-1 px-2 border-b border-[var(--border)]">
      {processes.map((p) => (
        <ProcessTab key={p.id} process={p} />
      ))}
      <button
        onClick={onLaunch}
        className="px-2 py-1.5 text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors"
        title="Launch new process"
      >
        +
      </button>
    </div>
  );
}
```

**Step 2: Create OutputPane**

```tsx
// ui/src/components/OutputPane.tsx
import { useEffect, useRef } from "react";
import { useAppStore } from "../stores/appStore";
import { useWebSocket } from "../hooks/useWebSocket";
import { processStreamURL } from "../lib/api";

export function OutputPane() {
  const { selectedProjectId, selectedProcessId, outputLines, appendOutput } = useAppStore();
  const bottomRef = useRef<HTMLDivElement>(null);

  const wsUrl =
    selectedProjectId && selectedProcessId
      ? processStreamURL(selectedProjectId, selectedProcessId)
      : null;

  useWebSocket({
    url: wsUrl,
    onMessage: (data: any) => {
      if (data.type === "output" && selectedProcessId) {
        appendOutput(selectedProcessId, data.line);
      }
    },
  });

  const lines = selectedProcessId ? outputLines.get(selectedProcessId) || [] : [];

  // Auto-scroll to bottom
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [lines.length]);

  if (!selectedProcessId) {
    return (
      <div className="flex-1 flex items-center justify-center text-[var(--text-secondary)] text-sm">
        Select a process or launch a new one
      </div>
    );
  }

  return (
    <div className="flex-1 overflow-y-auto bg-[var(--bg-primary)] font-mono text-sm p-3">
      {lines.map((line, i) => (
        <div key={i} className="whitespace-pre-wrap break-all leading-relaxed text-[var(--text-primary)]">
          <span className="text-[var(--border)] mr-2 select-none">▎</span>
          {line}
        </div>
      ))}
      <div ref={bottomRef} />
    </div>
  );
}
```

**Step 3: Create ProjectView**

```tsx
// ui/src/components/ProjectView.tsx
import { useEffect, useState } from "react";
import { ProcessTabs } from "./ProcessTabs";
import { OutputPane } from "./OutputPane";
import { TaskPanel } from "./TaskPanel";
import { useAppStore } from "../stores/appStore";
import { api, stateWatchURL } from "../lib/api";
import { useWebSocket } from "../hooks/useWebSocket";

export function ProjectView() {
  const {
    selectedProjectId,
    setProcesses,
    setProjectState,
    setSessions,
  } = useAppStore();
  const [showLaunchDialog, setShowLaunchDialog] = useState(false);

  // Fetch processes on mount and periodically
  useEffect(() => {
    if (!selectedProjectId) return;
    let mounted = true;

    async function fetch() {
      try {
        const [procs, state, log] = await Promise.all([
          api.listProcesses(selectedProjectId!),
          api.getState(selectedProjectId!),
          api.getLog(selectedProjectId!),
        ]);
        if (mounted) {
          setProcesses(procs);
          setProjectState(state);
          setSessions(log.sessions);
        }
      } catch {
        // ignore
      }
    }

    fetch();
    const interval = setInterval(fetch, 5000);
    return () => {
      mounted = false;
      clearInterval(interval);
    };
  }, [selectedProjectId, setProcesses, setProjectState, setSessions]);

  // Watch state changes via WebSocket
  useWebSocket({
    url: selectedProjectId ? stateWatchURL(selectedProjectId) : null,
    onMessage: (data: any) => {
      if (data.type === "state_changed") {
        setProjectState(data.state);
      }
      if (data.type === "log_appended") {
        useAppStore.getState().addSession(data.session);
      }
    },
  });

  if (!selectedProjectId) return null;

  return (
    <div className="flex-1 flex flex-col">
      <ProcessTabs onLaunch={() => setShowLaunchDialog(true)} />
      <div className="flex-1 flex overflow-hidden">
        <OutputPane />
        <TaskPanel />
      </div>
    </div>
  );
}
```

**Step 4: Update App.tsx to use ProjectView**

```tsx
// In App.tsx, replace the main area:
import { ProjectView } from "./components/ProjectView";

// In the main section:
<main className="flex-1 flex overflow-hidden">
  {selectedProjectId ? (
    <ProjectView />
  ) : (
    <div className="flex-1 flex items-center justify-center text-center text-[var(--text-secondary)]">
      <div>
        <div className="text-lg mb-2">Select a project</div>
        <div className="text-sm">or register one via golem serve</div>
      </div>
    </div>
  )}
</main>
```

**Step 5: Commit**

```bash
cd /home/winler/projects/golem
git add ui/src/components/ProcessTabs.tsx ui/src/components/OutputPane.tsx ui/src/components/ProjectView.tsx ui/src/App.tsx
git commit -m "feat(ui): add process tabs, streaming output pane, and project view"
```

---

## Task 10: React — Task Panel

Build the right-side task panel showing live task status, iteration info, and phase.

**Files:**
- Create: `ui/src/components/TaskPanel.tsx`

**Step 1: Create TaskPanel**

```tsx
// ui/src/components/TaskPanel.tsx
import { useAppStore } from "../stores/appStore";
import type { Task } from "../lib/api";

const statusIcons: Record<string, { icon: string; color: string }> = {
  done: { icon: "✓", color: "text-[var(--green)]" },
  "in-progress": { icon: "◐", color: "text-[var(--yellow)]" },
  todo: { icon: "○", color: "text-[var(--text-secondary)]" },
  blocked: { icon: "✗", color: "text-[var(--red)]" },
};

function TaskItem({ task }: { task: Task }) {
  const { icon, color } = statusIcons[task.status] || statusIcons.todo;

  return (
    <div className="flex items-start gap-2 py-1">
      <span className={`${color} font-mono text-xs mt-0.5`}>{icon}</span>
      <div className="min-w-0">
        <div className="text-sm truncate">{task.name}</div>
        {task.notes && (
          <div className="text-xs text-[var(--text-secondary)] truncate">{task.notes}</div>
        )}
        {task.blocked_reason && (
          <div className="text-xs text-[var(--red)] truncate">{task.blocked_reason}</div>
        )}
      </div>
    </div>
  );
}

export function TaskPanel() {
  const { projectState, sessions } = useAppStore();

  if (!projectState) return null;

  const tasks = projectState.tasks || [];
  const doneTasks = tasks.filter((t) => t.status === "done").length;
  const totalTasks = tasks.length;
  const lastSession = sessions.length > 0 ? sessions[sessions.length - 1] : null;

  return (
    <div className="w-56 min-w-56 border-l border-[var(--border)] bg-[var(--bg-surface)] flex flex-col">
      {/* Task list */}
      <div className="px-3 py-2 border-b border-[var(--border)] flex items-center justify-between">
        <span className="text-xs font-semibold uppercase tracking-wider text-[var(--text-secondary)]">
          Tasks
        </span>
        <span className="text-xs text-[var(--text-secondary)]">
          {doneTasks}/{totalTasks}
        </span>
      </div>
      <div className="flex-1 overflow-y-auto px-3 py-2 space-y-0.5">
        {tasks.map((t) => (
          <TaskItem key={t.name} task={t} />
        ))}
      </div>

      {/* Stats */}
      <div className="border-t border-[var(--border)] px-3 py-2 space-y-1 text-xs text-[var(--text-secondary)]">
        <div className="flex justify-between">
          <span>Phase</span>
          <span className="text-[var(--text-primary)]">{projectState.status.phase || "—"}</span>
        </div>
        <div className="flex justify-between">
          <span>Focus</span>
          <span className="text-[var(--text-primary)] truncate ml-2">
            {projectState.status.current_focus || "—"}
          </span>
        </div>
        {lastSession && (
          <>
            <div className="flex justify-between">
              <span>Last iter</span>
              <span className="text-[var(--text-primary)]">#{lastSession.iteration}</span>
            </div>
            <div className="flex justify-between">
              <span>Outcome</span>
              <span
                className={
                  lastSession.outcome === "done"
                    ? "text-[var(--green)]"
                    : lastSession.outcome === "blocked" || lastSession.outcome === "unproductive"
                    ? "text-[var(--red)]"
                    : "text-[var(--yellow)]"
                }
              >
                {lastSession.outcome}
              </span>
            </div>
          </>
        )}
        <div className="flex justify-between">
          <span>Decisions</span>
          <span className="text-[var(--text-primary)]">{projectState.decisions?.length || 0}</span>
        </div>
        <div className="flex justify-between">
          <span>Pitfalls</span>
          <span className="text-[var(--text-primary)]">{projectState.pitfalls?.length || 0}</span>
        </div>
      </div>
    </div>
  );
}
```

**Step 2: Verify visually**

Run dev server and check that the task panel renders correctly with task icons and colors.

**Step 3: Commit**

```bash
cd /home/winler/projects/golem
git add ui/src/components/TaskPanel.tsx
git commit -m "feat(ui): add task panel with live status, stats, and session info"
```

---

## Task 11: React — Launch Dialog

Build the modal dialog for launching new golem processes.

**Files:**
- Create: `ui/src/components/LaunchDialog.tsx`
- Modify: `ui/src/components/ProjectView.tsx` (wire dialog)

**Step 1: Create LaunchDialog**

```tsx
// ui/src/components/LaunchDialog.tsx
import { useState, useEffect } from "react";
import { useAppStore } from "../stores/appStore";
import { api } from "../lib/api";
import type { GolemConfig } from "../lib/api";

interface LaunchDialogProps {
  open: boolean;
  onClose: () => void;
}

export function LaunchDialog({ open, onClose }: LaunchDialogProps) {
  const { selectedProjectId, setProcesses } = useAppStore();
  const [command, setCommand] = useState("code");
  const [model, setModel] = useState("");
  const [maxIterations, setMaxIterations] = useState(20);
  const [maxToolCalls, setMaxToolCalls] = useState(200);
  const [sandbox, setSandbox] = useState(false);
  const [mcp, setMcp] = useState(true);
  const [parallel, setParallel] = useState(1);
  const [task, setTask] = useState("");
  const [launching, setLaunching] = useState(false);

  // Load defaults from config
  useEffect(() => {
    if (!selectedProjectId || !open) return;
    api.getProjectConfig(selectedProjectId).then((cfg: GolemConfig) => {
      setMaxIterations(cfg["max-iterations"]);
      setMaxToolCalls(cfg["max-tool-calls"]);
      setSandbox(cfg.sandbox);
      setMcp(cfg.mcp);
      setParallel(cfg.parallel);
      if (cfg.model) setModel(cfg.model);
    }).catch(() => {});
  }, [selectedProjectId, open]);

  if (!open) return null;

  async function handleLaunch() {
    if (!selectedProjectId) return;
    setLaunching(true);
    try {
      await api.launchProcess(selectedProjectId, {
        command,
        config: {
          maxIterations,
          maxToolCalls,
          model: model || undefined,
          task: task || undefined,
          sandbox,
          mcp,
          parallel,
        },
      });
      const procs = await api.listProcesses(selectedProjectId);
      setProcesses(procs);
      onClose();
    } catch (err) {
      alert(`Launch failed: ${err}`);
    } finally {
      setLaunching(false);
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
      <div
        className="bg-[var(--bg-surface)] border border-[var(--border)] rounded-lg w-[420px] p-6 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-lg font-semibold mb-4">Launch Process</h2>

        <div className="space-y-4">
          {/* Command */}
          <label className="block">
            <span className="text-xs text-[var(--text-secondary)] uppercase tracking-wider">Command</span>
            <select
              value={command}
              onChange={(e) => setCommand(e.target.value)}
              className="mt-1 block w-full bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-2 text-sm"
            >
              <option value="code">code</option>
              <option value="review">review</option>
              <option value="qa">qa</option>
              <option value="plan">plan</option>
            </select>
          </label>

          {/* Model */}
          <label className="block">
            <span className="text-xs text-[var(--text-secondary)] uppercase tracking-wider">Model</span>
            <select
              value={model}
              onChange={(e) => setModel(e.target.value)}
              className="mt-1 block w-full bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-2 text-sm"
            >
              <option value="">default</option>
              <option value="sonnet">sonnet</option>
              <option value="opus">opus</option>
              <option value="haiku">haiku</option>
            </select>
          </label>

          {/* Max Iterations + Max Tool Calls */}
          <div className="grid grid-cols-2 gap-3">
            <label className="block">
              <span className="text-xs text-[var(--text-secondary)] uppercase tracking-wider">Max Iterations</span>
              <input
                type="number"
                value={maxIterations}
                onChange={(e) => setMaxIterations(parseInt(e.target.value) || 20)}
                className="mt-1 block w-full bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-2 text-sm"
              />
            </label>
            <label className="block">
              <span className="text-xs text-[var(--text-secondary)] uppercase tracking-wider">Max Tool Calls</span>
              <input
                type="number"
                value={maxToolCalls}
                onChange={(e) => setMaxToolCalls(parseInt(e.target.value) || 200)}
                className="mt-1 block w-full bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-2 text-sm"
              />
            </label>
          </div>

          {/* Checkboxes */}
          <div className="flex gap-6">
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={sandbox}
                onChange={(e) => setSandbox(e.target.checked)}
                className="rounded"
              />
              Sandbox
            </label>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={mcp}
                onChange={(e) => setMcp(e.target.checked)}
                className="rounded"
              />
              MCP
            </label>
          </div>

          {/* Parallel */}
          <label className="block">
            <span className="text-xs text-[var(--text-secondary)] uppercase tracking-wider">Parallel</span>
            <input
              type="number"
              value={parallel}
              onChange={(e) => setParallel(parseInt(e.target.value) || 1)}
              min={1}
              className="mt-1 block w-20 bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-2 text-sm"
            />
          </label>

          {/* Task Override */}
          <label className="block">
            <span className="text-xs text-[var(--text-secondary)] uppercase tracking-wider">Task Override</span>
            <input
              type="text"
              value={task}
              onChange={(e) => setTask(e.target.value)}
              placeholder="(optional)"
              className="mt-1 block w-full bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-2 text-sm placeholder:text-[var(--text-secondary)]"
            />
          </label>
        </div>

        {/* Actions */}
        <div className="flex justify-end gap-3 mt-6">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={handleLaunch}
            disabled={launching}
            className="px-4 py-2 text-sm bg-[var(--accent)] text-white rounded hover:opacity-90 transition-opacity disabled:opacity-50"
          >
            {launching ? "Launching..." : "Launch"}
          </button>
        </div>
      </div>
    </div>
  );
}
```

**Step 2: Wire dialog into ProjectView**

Update `ProjectView.tsx` to import and render `LaunchDialog`:

```tsx
import { LaunchDialog } from "./LaunchDialog";

// In the JSX, add before the closing tag:
<LaunchDialog open={showLaunchDialog} onClose={() => setShowLaunchDialog(false)} />
```

**Step 3: Commit**

```bash
cd /home/winler/projects/golem
git add ui/src/components/LaunchDialog.tsx ui/src/components/ProjectView.tsx
git commit -m "feat(ui): add launch dialog with all config options"
```

---

## Task 12: React — Settings Dialog

Build the settings modal with project and global config tabs.

**Files:**
- Create: `ui/src/components/SettingsDialog.tsx`
- Modify: `ui/src/App.tsx` (add settings button)

**Step 1: Create SettingsDialog**

```tsx
// ui/src/components/SettingsDialog.tsx
import { useState, useEffect } from "react";
import { useAppStore } from "../stores/appStore";
import { api } from "../lib/api";
import type { GolemConfig } from "../lib/api";

interface SettingsDialogProps {
  open: boolean;
  onClose: () => void;
}

type Tab = "project" | "global";

function ConfigForm({
  config,
  onChange,
}: {
  config: GolemConfig;
  onChange: (config: GolemConfig) => void;
}) {
  function set<K extends keyof GolemConfig>(key: K, value: GolemConfig[K]) {
    onChange({ ...config, [key]: value });
  }

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3">
        <label className="block">
          <span className="text-xs text-[var(--text-secondary)]">max-iterations</span>
          <input
            type="number"
            value={config["max-iterations"]}
            onChange={(e) => set("max-iterations", parseInt(e.target.value) || 20)}
            className="mt-1 block w-full bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-1.5 text-sm"
          />
        </label>
        <label className="block">
          <span className="text-xs text-[var(--text-secondary)]">max-tool-calls</span>
          <input
            type="number"
            value={config["max-tool-calls"]}
            onChange={(e) => set("max-tool-calls", parseInt(e.target.value) || 200)}
            className="mt-1 block w-full bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-1.5 text-sm"
          />
        </label>
      </div>
      <label className="block">
        <span className="text-xs text-[var(--text-secondary)]">model</span>
        <select
          value={config.model}
          onChange={(e) => set("model", e.target.value)}
          className="mt-1 block w-full bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-1.5 text-sm"
        >
          <option value="">default</option>
          <option value="sonnet">sonnet</option>
          <option value="opus">opus</option>
          <option value="haiku">haiku</option>
        </select>
      </label>
      <div className="flex gap-6">
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={config.verbose} onChange={(e) => set("verbose", e.target.checked)} />
          verbose
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={config.sandbox} onChange={(e) => set("sandbox", e.target.checked)} />
          sandbox
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={config.mcp} onChange={(e) => set("mcp", e.target.checked)} />
          mcp
        </label>
      </div>
      <label className="block">
        <span className="text-xs text-[var(--text-secondary)]">parallel</span>
        <input
          type="number"
          value={config.parallel}
          onChange={(e) => set("parallel", parseInt(e.target.value) || 1)}
          min={1}
          className="mt-1 block w-20 bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-1.5 text-sm"
        />
      </label>
    </div>
  );
}

export function SettingsDialog({ open, onClose }: SettingsDialogProps) {
  const { selectedProjectId } = useAppStore();
  const [tab, setTab] = useState<Tab>("project");
  const [config, setConfig] = useState<GolemConfig | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    if (tab === "project" && selectedProjectId) {
      api.getProjectConfig(selectedProjectId).then(setConfig).catch(() => setConfig(null));
    } else {
      api.getGlobalConfig().then(setConfig).catch(() => setConfig(null));
    }
  }, [open, tab, selectedProjectId]);

  if (!open) return null;

  async function handleSave() {
    if (!config) return;
    setSaving(true);
    try {
      if (tab === "project" && selectedProjectId) {
        await api.updateProjectConfig(selectedProjectId, config);
      } else {
        await api.updateGlobalConfig(config);
      }
      onClose();
    } catch (err) {
      alert(`Save failed: ${err}`);
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
      <div
        className="bg-[var(--bg-surface)] border border-[var(--border)] rounded-lg w-[460px] p-6 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-lg font-semibold mb-4">Settings</h2>

        {/* Tabs */}
        <div className="flex gap-1 mb-4">
          <button
            onClick={() => setTab("project")}
            className={`px-3 py-1.5 text-sm rounded ${
              tab === "project"
                ? "bg-[var(--bg-elevated)] text-[var(--text-primary)]"
                : "text-[var(--text-secondary)]"
            }`}
          >
            Project
          </button>
          <button
            onClick={() => setTab("global")}
            className={`px-3 py-1.5 text-sm rounded ${
              tab === "global"
                ? "bg-[var(--bg-elevated)] text-[var(--text-primary)]"
                : "text-[var(--text-secondary)]"
            }`}
          >
            Global
          </button>
        </div>

        {config ? (
          <ConfigForm config={config} onChange={setConfig} />
        ) : (
          <div className="text-sm text-[var(--text-secondary)] py-8 text-center">Loading...</div>
        )}

        <div className="flex justify-end gap-3 mt-6">
          <button onClick={onClose} className="px-4 py-2 text-sm text-[var(--text-secondary)]">
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={saving || !config}
            className="px-4 py-2 text-sm bg-[var(--accent)] text-white rounded disabled:opacity-50"
          >
            {saving ? "Saving..." : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}
```

**Step 2: Add settings button to App.tsx**

Add a gear icon button in the top-right area of the app bar, wired to open the SettingsDialog.

**Step 3: Commit**

```bash
cd /home/winler/projects/golem
git add ui/src/components/SettingsDialog.tsx ui/src/App.tsx
git commit -m "feat(ui): add settings dialog with project and global config tabs"
```

---

## Task 13: React — Project Dashboard (No Process Selected)

Show a project overview when no process tab is selected — tasks, recent sessions, decisions, pitfalls.

**Files:**
- Create: `ui/src/components/ProjectDashboard.tsx`
- Modify: `ui/src/components/ProjectView.tsx` (render when no process selected)

**Step 1: Create ProjectDashboard**

```tsx
// ui/src/components/ProjectDashboard.tsx
import { useAppStore } from "../stores/appStore";

export function ProjectDashboard() {
  const { projectState, sessions } = useAppStore();

  if (!projectState) return null;

  const tasks = projectState.tasks || [];
  const done = tasks.filter((t) => t.status === "done").length;
  const recentSessions = sessions.slice(-5).reverse();

  return (
    <div className="flex-1 overflow-y-auto p-6 max-w-3xl mx-auto space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-xl font-semibold">{projectState.project.name}</h1>
        <p className="text-sm text-[var(--text-secondary)] mt-1">{projectState.project.summary}</p>
        <div className="flex gap-4 mt-2 text-xs text-[var(--text-secondary)]">
          <span>Stack: {projectState.project.stack}</span>
          <span>Phase: {projectState.status.phase}</span>
        </div>
      </div>

      {/* Task Progress */}
      <div>
        <h2 className="text-sm font-semibold mb-2">Tasks ({done}/{tasks.length})</h2>
        <div className="w-full bg-[var(--bg-elevated)] rounded-full h-2 mb-3">
          <div
            className="bg-[var(--green)] h-2 rounded-full transition-all"
            style={{ width: tasks.length ? `${(done / tasks.length) * 100}%` : "0%" }}
          />
        </div>
        <div className="space-y-1">
          {tasks.map((t) => (
            <div key={t.name} className="flex items-center gap-2 text-sm">
              <span className={t.status === "done" ? "text-[var(--green)]" : t.status === "blocked" ? "text-[var(--red)]" : "text-[var(--text-secondary)]"}>
                {t.status === "done" ? "✓" : t.status === "in-progress" ? "◐" : t.status === "blocked" ? "✗" : "○"}
              </span>
              <span className={t.status === "done" ? "text-[var(--text-secondary)] line-through" : ""}>{t.name}</span>
            </div>
          ))}
        </div>
      </div>

      {/* Recent Sessions */}
      {recentSessions.length > 0 && (
        <div>
          <h2 className="text-sm font-semibold mb-2">Recent Sessions</h2>
          <div className="space-y-2">
            {recentSessions.map((s, i) => (
              <div key={i} className="bg-[var(--bg-surface)] border border-[var(--border)] rounded p-3 text-sm">
                <div className="flex justify-between">
                  <span>#{s.iteration} — {s.task}</span>
                  <span className={s.outcome === "done" ? "text-[var(--green)]" : s.outcome === "partial" ? "text-[var(--yellow)]" : "text-[var(--red)]"}>
                    {s.outcome}
                  </span>
                </div>
                {s.summary && <div className="text-xs text-[var(--text-secondary)] mt-1">{s.summary}</div>}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Decisions */}
      {projectState.decisions?.length > 0 && (
        <div>
          <h2 className="text-sm font-semibold mb-2">Decisions ({projectState.decisions.length})</h2>
          <div className="space-y-1">
            {projectState.decisions.map((d, i) => (
              <div key={i} className="text-sm">
                <span className="text-[var(--text-secondary)]">{d.when}</span>{" "}
                <span>{d.what}</span>{" "}
                <span className="text-[var(--text-secondary)]">— {d.why}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Pitfalls */}
      {projectState.pitfalls?.length > 0 && (
        <div>
          <h2 className="text-sm font-semibold mb-2">Pitfalls ({projectState.pitfalls.length})</h2>
          <div className="space-y-1">
            {projectState.pitfalls.map((p, i) => (
              <div key={i} className="text-sm">
                <span>{p.what}</span>
                {p.fix && <span className="text-[var(--text-secondary)]"> — Fix: {p.fix}</span>}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
```

**Step 2: Wire into ProjectView**

In `ProjectView.tsx`, when `selectedProcessId` is null, render `<ProjectDashboard />` instead of `<OutputPane />` + `<TaskPanel />`:

```tsx
<div className="flex-1 flex overflow-hidden">
  {selectedProcessId ? (
    <>
      <OutputPane />
      <TaskPanel />
    </>
  ) : (
    <ProjectDashboard />
  )}
</div>
```

**Step 3: Commit**

```bash
cd /home/winler/projects/golem
git add ui/src/components/ProjectDashboard.tsx ui/src/components/ProjectView.tsx
git commit -m "feat(ui): add project dashboard with tasks, sessions, decisions, and pitfalls"
```

---

## Task 14: Integration Testing — End-to-End

Test the full flow: start server, register project, launch process, stream output.

**Files:**
- Create: `internal/server/integration_test.go`

**Step 1: Write the integration test**

```go
// internal/server/integration_test.go
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFullWorkflow(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 1. Health check
	resp, _ := http.Get(ts.URL + "/api/health")
	if resp.StatusCode != 200 {
		t.Fatal("health check failed")
	}
	resp.Body.Close()

	// 2. Register project
	body, _ := json.Marshal(map[string]string{"path": dir})
	resp, _ = http.Post(ts.URL+"/api/projects", "application/json", bytes.NewReader(body))
	var regResult map[string]string
	json.NewDecoder(resp.Body).Decode(&regResult)
	resp.Body.Close()
	projectID := regResult["id"]

	// 3. Get state
	resp, _ = http.Get(ts.URL + "/api/projects/" + projectID + "/state")
	var state map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&state)
	resp.Body.Close()
	project := state["project"].(map[string]interface{})
	if project["name"] != "test-project" {
		t.Fatalf("expected name test-project, got %v", project["name"])
	}

	// 4. Get log
	resp, _ = http.Get(ts.URL + "/api/projects/" + projectID + "/log")
	var log map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&log)
	resp.Body.Close()
	sessions := log["sessions"].([]interface{})
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	// 5. Get config
	resp, _ = http.Get(ts.URL + "/api/projects/" + projectID + "/config")
	if resp.StatusCode != 200 {
		t.Fatal("config endpoint failed")
	}
	resp.Body.Close()

	// 6. List processes (empty)
	resp, _ = http.Get(ts.URL + "/api/projects/" + projectID + "/processes")
	var procs []ProcessInfo
	json.NewDecoder(resp.Body).Decode(&procs)
	resp.Body.Close()
	if len(procs) != 0 {
		t.Fatalf("expected 0 processes, got %d", len(procs))
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/server/ -v -run TestFullWorkflow`
Expected: PASS

**Step 3: Run all tests**

Run: `go test ./... -v`
Expected: All PASS

**Step 4: Commit**

```bash
git add internal/server/integration_test.go
git commit -m "test(server): add integration test for full API workflow"
```

---

## Task 15: Polish — Tauri System Tray and Final Wiring

Add system tray support and ensure the Tauri app auto-connects to `golem serve`.

**Files:**
- Modify: `ui/src-tauri/src/main.rs`
- Modify: `ui/src-tauri/tauri.conf.json` (add system tray config)

**Step 1: Update Tauri main.rs for system tray**

Add basic system tray with "Show" and "Quit" options. The exact API depends on the Tauri 2 version installed — consult `@tauri-apps/api` docs when implementing.

**Step 2: Test Tauri build**

```bash
cd ui && cargo tauri build
```

Expected: Produces a desktop app binary in `ui/src-tauri/target/release/`

**Step 3: Commit**

```bash
cd /home/winler/projects/golem
git add ui/src-tauri/
git commit -m "feat(ui): add system tray support for background operation"
```

---

## Summary

| Task | Component | What |
|------|-----------|------|
| 1 | Go Server | HTTP foundation, health, CORS |
| 2 | Go Server | Project registry, state/log/config endpoints |
| 3 | Go Server | Process management — launch/list/stop |
| 4 | Go Server | WebSocket streaming + fsnotify state watching |
| 5 | Go Server | `golem serve` CLI command |
| 6 | UI | Tauri + React + Tailwind scaffolding |
| 7 | UI | API client, Zustand store, WebSocket hook |
| 8 | UI | App shell, sidebar, connection status |
| 9 | UI | Process tabs, output pane, project view |
| 10 | UI | Task panel with live status |
| 11 | UI | Launch dialog with all config options |
| 12 | UI | Settings dialog (project + global) |
| 13 | UI | Project dashboard (no process selected) |
| 14 | Testing | Integration test for full API workflow |
| 15 | Polish | System tray, Tauri build |
