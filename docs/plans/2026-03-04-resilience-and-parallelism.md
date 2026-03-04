# Resilience & Parallelism Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add TUI halt reasons, state snapshots with auto-rollback, an MCP server for structured state updates, and parallel task execution via git worktrees.

**Architecture:** Four incremental features. Each builds on the prior: TUI observability first, then snapshots for resilience, then MCP server to eliminate state corruption, then parallelism using MCP's flock for concurrent state access.

**Tech Stack:** Go 1.24, Bubbletea (TUI), mark3labs/mcp-go (MCP server), gopkg.in/yaml.v3

---

### Task 1: TUI halt reason display

**Files:**
- Modify: `internal/tui/run.go:160-168` (EventLoopDone handler)
- Modify: `internal/tui/run.go:220-237` (View footer)
- Modify: `internal/tui/run.go:133-168` (EventIterEnd handler)
- Modify: `internal/tui/styles.go` (add error/warning styles)

**Step 1: Add haltReason field to RunModel**

In `internal/tui/run.go`, add to the RunModel struct (after `finalErr error` on line 33):

```go
haltReason string
```

**Step 2: Store halt reason on EventLoopDone**

In `internal/tui/run.go`, update the `EventLoopDone` case (line 160-167):

```go
case runner.EventLoopDone:
    m.done = true
    m.finalResult = msg.Result
    m.finalErr = msg.Err
    if msg.Result != nil && msg.Result.Halted {
        m.haltReason = msg.Result.HaltReason
    }
    // Refresh state one last time
    if s, err := golemctx.ReadState(m.dir); err == nil {
        m.state = s
    }
```

**Step 3: Append iteration errors to output**

In the `EventIterEnd` case (line 145), after refreshing state, add:

```go
if msg.Err != nil {
    m.outputLines = append(m.outputLines, fmt.Sprintf("⚠ iteration %d failed: %v", msg.Iter, msg.Err))
    if m.ready {
        m.viewport.SetContent(strings.Join(m.outputLines, "\n"))
        m.viewport.GotoBottom()
    }
}
```

**Step 4: Add error styles**

In `internal/tui/styles.go`, add:

```go
haltStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true) // red bold
warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))           // yellow
```

**Step 5: Update footer to show halt reason**

In `internal/tui/run.go`, update the footer section (lines 221-236):

```go
footerLeft := " q quit"
footerRight := ""
if m.done {
    if m.finalResult != nil && m.finalResult.Completed {
        footerRight = doneStyle.Render("all tasks done!")
    } else if m.haltReason != "" {
        footerRight = haltStyle.Render("HALTED: " + m.haltReason)
    } else {
        footerRight = "loop finished"
    }
} else if m.iter > 0 {
    footerRight = fmt.Sprintf("iter %d/%d", m.iter, m.maxIter)
}
```

**Step 6: Also append halt reason to output panel for scrollback**

In the `EventLoopDone` case, after setting haltReason:

```go
if m.haltReason != "" {
    m.outputLines = append(m.outputLines, "")
    m.outputLines = append(m.outputLines, haltStyle.Render("HALTED: "+m.haltReason))
    if m.ready {
        m.viewport.SetContent(strings.Join(m.outputLines, "\n"))
        m.viewport.GotoBottom()
    }
}
```

**Step 7: Run tests and verify build**

Run: `go build ./... && go test ./...`
Expected: All pass.

**Step 8: Commit**

```bash
git add internal/tui/run.go internal/tui/styles.go
git commit -m "feat(tui): surface halt reasons and iteration errors in TUI"
```

---

### Task 2: State snapshots with auto-rollback

**Files:**
- Create: `internal/runner/snapshot.go`
- Create: `internal/runner/snapshot_test.go`
- Modify: `internal/runner/builder.go:73-79` (snapshot before iteration)
- Modify: `internal/runner/validate.go:41-46` (restore on fatal failure)

**Step 1: Write snapshot_test.go (failing tests)**

Create `internal/runner/snapshot_test.go`:

```go
package runner

import (
    "os"
    "path/filepath"
    "testing"
)

func TestSaveSnapshot(t *testing.T) {
    dir := t.TempDir()
    ctxDir := filepath.Join(dir, ".ctx")
    os.MkdirAll(ctxDir, 0755)
    os.WriteFile(filepath.Join(ctxDir, "state.yaml"), []byte("project:\n  name: test\n"), 0644)

    if err := SaveSnapshot(dir, 1); err != nil {
        t.Fatalf("SaveSnapshot: %v", err)
    }

    snapPath := filepath.Join(ctxDir, "snapshots", "state-001.yaml")
    data, err := os.ReadFile(snapPath)
    if err != nil {
        t.Fatalf("snapshot not created: %v", err)
    }
    if string(data) != "project:\n  name: test\n" {
        t.Errorf("snapshot content = %q", string(data))
    }
}

func TestRestoreSnapshot(t *testing.T) {
    dir := t.TempDir()
    ctxDir := filepath.Join(dir, ".ctx")
    snapDir := filepath.Join(ctxDir, "snapshots")
    os.MkdirAll(snapDir, 0755)

    // Create snapshot
    os.WriteFile(filepath.Join(snapDir, "state-001.yaml"), []byte("good state"), 0644)
    // Corrupt current state
    os.WriteFile(filepath.Join(ctxDir, "state.yaml"), []byte("corrupted"), 0644)

    restored, err := RestoreLatestSnapshot(dir)
    if err != nil {
        t.Fatalf("RestoreLatestSnapshot: %v", err)
    }
    if !restored {
        t.Fatal("expected restore to succeed")
    }

    data, _ := os.ReadFile(filepath.Join(ctxDir, "state.yaml"))
    if string(data) != "good state" {
        t.Errorf("restored content = %q, want %q", string(data), "good state")
    }
}

func TestRestoreSnapshot_NoSnapshots(t *testing.T) {
    dir := t.TempDir()
    os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

    restored, err := RestoreLatestSnapshot(dir)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if restored {
        t.Fatal("expected no restore when no snapshots exist")
    }
}

func TestPruneSnapshots(t *testing.T) {
    dir := t.TempDir()
    snapDir := filepath.Join(dir, ".ctx", "snapshots")
    os.MkdirAll(snapDir, 0755)

    // Create 12 snapshots
    for i := 1; i <= 12; i++ {
        os.WriteFile(filepath.Join(snapDir, fmt.Sprintf("state-%03d.yaml", i)), []byte("data"), 0644)
    }

    PruneSnapshots(dir, 10)

    entries, _ := filepath.Glob(filepath.Join(snapDir, "state-*.yaml"))
    if len(entries) != 10 {
        t.Errorf("after prune: %d snapshots, want 10", len(entries))
    }
    // Oldest (001, 002) should be removed
    if _, err := os.Stat(filepath.Join(snapDir, "state-001.yaml")); !os.IsNotExist(err) {
        t.Error("state-001.yaml should have been pruned")
    }
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/runner/ -run TestSave -v`
Expected: FAIL (SaveSnapshot not defined)

**Step 3: Implement snapshot.go**

Create `internal/runner/snapshot.go`:

```go
package runner

import (
    "fmt"
    "os"
    "path/filepath"
    "sort"
)

const maxSnapshots = 10

// SaveSnapshot copies state.yaml to .ctx/snapshots/state-<iteration>.yaml.
func SaveSnapshot(dir string, iteration int) error {
    src := filepath.Join(dir, ".ctx", "state.yaml")
    data, err := os.ReadFile(src)
    if err != nil {
        return fmt.Errorf("reading state for snapshot: %w", err)
    }

    snapDir := filepath.Join(dir, ".ctx", "snapshots")
    if err := os.MkdirAll(snapDir, 0755); err != nil {
        return fmt.Errorf("creating snapshots dir: %w", err)
    }

    dst := filepath.Join(snapDir, fmt.Sprintf("state-%03d.yaml", iteration))
    return os.WriteFile(dst, data, 0644)
}

// RestoreLatestSnapshot copies the most recent snapshot back to state.yaml.
// Returns (true, nil) if restored, (false, nil) if no snapshots exist.
func RestoreLatestSnapshot(dir string) (bool, error) {
    snapDir := filepath.Join(dir, ".ctx", "snapshots")
    matches, _ := filepath.Glob(filepath.Join(snapDir, "state-*.yaml"))
    if len(matches) == 0 {
        return false, nil
    }

    sort.Strings(matches)
    latest := matches[len(matches)-1]

    data, err := os.ReadFile(latest)
    if err != nil {
        return false, fmt.Errorf("reading snapshot %s: %w", latest, err)
    }

    dst := filepath.Join(dir, ".ctx", "state.yaml")
    if err := os.WriteFile(dst, data, 0644); err != nil {
        return false, fmt.Errorf("restoring snapshot: %w", err)
    }

    return true, nil
}

// PruneSnapshots keeps only the most recent `keep` snapshots, deleting older ones.
func PruneSnapshots(dir string, keep int) {
    snapDir := filepath.Join(dir, ".ctx", "snapshots")
    matches, _ := filepath.Glob(filepath.Join(snapDir, "state-*.yaml"))
    if len(matches) <= keep {
        return
    }
    sort.Strings(matches)
    for _, f := range matches[:len(matches)-keep] {
        os.Remove(f)
    }
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/runner/ -run "TestSave|TestRestore|TestPrune" -v`
Expected: All PASS.

**Step 5: Integrate into builder loop**

In `internal/runner/builder.go`, after reading state at line 74 and before `remaining = state.RemainingTasks()` at line 81, add:

```go
// Snapshot state before agent touches it
if err := SaveSnapshot(cfg.Dir, i); err != nil {
    fmt.Fprintf(os.Stderr, "golem: warning: could not save snapshot: %v\n", err)
}
PruneSnapshots(cfg.Dir, maxSnapshots)
```

**Step 6: Integrate into validation — restore on fatal failure**

In `internal/runner/validate.go`, replace the fatal halt block (lines 41-46) with:

```go
if err2 := ctx.ValidateState(repaired); err2 != nil {
    // Try snapshot restore before giving up
    restored, restoreErr := RestoreLatestSnapshot(dir)
    if restoreErr != nil {
        result.Warnings = append(result.Warnings, fmt.Sprintf("WARNING — snapshot restore failed: %v", restoreErr))
    }
    if restored {
        result.Warnings = append(result.Warnings, "WARNING — state corrupted beyond repair, restored from snapshot")
        // Don't halt — next iteration will re-read restored state
        return result
    }
    // No snapshot available — halt
    result.Halted = true
    result.Warnings = append(result.Warnings, fmt.Sprintf("FATAL: state.yaml validation failed (no snapshot to restore): %v", err2))
    return result
}
```

**Step 7: Run all tests**

Run: `go build ./... && go test ./...`
Expected: All pass.

**Step 8: Commit**

```bash
git add internal/runner/snapshot.go internal/runner/snapshot_test.go internal/runner/builder.go internal/runner/validate.go
git commit -m "feat(runner): state snapshots with auto-rollback on corruption"
```

---

### Task 3: MCP server — scaffolding and protocol

**Files:**
- Create: `internal/mcp/server.go`
- Create: `internal/mcp/server_test.go`
- Modify: `go.mod` (add `github.com/mark3labs/mcp-go`)

**Step 1: Add mcp-go dependency**

Run: `go get github.com/mark3labs/mcp-go`

**Step 2: Write server_test.go (failing test for server creation)**

Create `internal/mcp/server_test.go`:

```go
package mcp

import (
    "testing"
)

func TestNewServer(t *testing.T) {
    s := NewServer(t.TempDir())
    if s == nil {
        t.Fatal("NewServer returned nil")
    }
    tools := s.ListTools()
    expected := []string{"mark_task", "set_phase", "add_decision", "add_pitfall", "add_locked", "log_session"}
    if len(tools) != len(expected) {
        t.Errorf("got %d tools, want %d", len(tools), len(expected))
    }
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/mcp/ -run TestNewServer -v`
Expected: FAIL (package doesn't exist)

**Step 4: Implement server.go — server scaffold with tool registration**

Create `internal/mcp/server.go`:

```go
package mcp

import (
    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
)

// GolemServer wraps an MCP server that exposes state update tools.
type GolemServer struct {
    mcpServer *server.MCPServer
    dir       string
}

// NewServer creates a new MCP server with all golem tools registered.
func NewServer(dir string) *GolemServer {
    s := server.NewMCPServer("golem", "1.0.0",
        server.WithToolCapabilities(true),
    )

    gs := &GolemServer{mcpServer: s, dir: dir}
    gs.registerTools()
    return gs
}

// ListTools returns the names of all registered tools.
func (gs *GolemServer) ListTools() []string {
    // Tool names are registered in registerTools
    return []string{"mark_task", "set_phase", "add_decision", "add_pitfall", "add_locked", "log_session"}
}

// ServeStdio runs the MCP server over stdin/stdout.
func (gs *GolemServer) ServeStdio() error {
    return server.ServeStdio(gs.mcpServer)
}

func (gs *GolemServer) registerTools() {
    gs.mcpServer.AddTool(markTaskTool(), gs.handleMarkTask)
    gs.mcpServer.AddTool(setPhaseTool(), gs.handleSetPhase)
    gs.mcpServer.AddTool(addDecisionTool(), gs.handleAddDecision)
    gs.mcpServer.AddTool(addPitfallTool(), gs.handleAddPitfall)
    gs.mcpServer.AddTool(addLockedTool(), gs.handleAddLocked)
    gs.mcpServer.AddTool(logSessionTool(), gs.handleLogSession)
}
```

**Step 5: Run test — will still fail because tool definitions don't exist yet**

Expected: FAIL (markTaskTool not defined). That's fine — Task 4 implements the tools.

**Step 6: Commit scaffold**

```bash
git add internal/mcp/server.go internal/mcp/server_test.go go.mod go.sum
git commit -m "feat(mcp): scaffold MCP server with tool registration"
```

---

### Task 4: MCP server — tool implementations

**Files:**
- Create: `internal/mcp/tools.go`
- Create: `internal/mcp/tools_test.go`

**Step 1: Write tools_test.go**

Create `internal/mcp/tools_test.go`:

```go
package mcp

import (
    "context"
    "os"
    "path/filepath"
    "testing"

    "github.com/mark3labs/mcp-go/mcp"
    golemctx "github.com/lofari/golem/internal/ctx"
)

func setupTestDir(t *testing.T) string {
    t.Helper()
    dir := t.TempDir()
    os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)
    state := golemctx.State{
        Project: golemctx.Project{Name: "test"},
        Status:  golemctx.Status{Phase: "building"},
        Tasks: []golemctx.Task{
            {Name: "auth", Status: "todo"},
            {Name: "api", Status: "in-progress"},
        },
    }
    golemctx.WriteState(dir, state)
    log := golemctx.Log{Sessions: []golemctx.Session{}}
    golemctx.WriteLog(dir, log)
    return dir
}

func TestHandleMarkTask(t *testing.T) {
    dir := setupTestDir(t)
    gs := NewServer(dir)

    result, err := gs.handleMarkTask(context.Background(), mcp.CallToolRequest{
        Params: mcp.CallToolParams{
            Arguments: map[string]interface{}{
                "name":   "auth",
                "status": "done",
                "notes":  "implemented OAuth2",
            },
        },
    })
    if err != nil {
        t.Fatalf("handleMarkTask: %v", err)
    }
    if result.IsError {
        t.Fatalf("tool returned error: %v", result.Content)
    }

    state, _ := golemctx.ReadState(dir)
    task := state.FindTask("auth")
    if task.Status != "done" {
        t.Errorf("task status = %q, want %q", task.Status, "done")
    }
    if task.Notes != "implemented OAuth2" {
        t.Errorf("task notes = %q", task.Notes)
    }
}

func TestHandleMarkTask_InvalidStatus(t *testing.T) {
    dir := setupTestDir(t)
    gs := NewServer(dir)

    result, err := gs.handleMarkTask(context.Background(), mcp.CallToolRequest{
        Params: mcp.CallToolParams{
            Arguments: map[string]interface{}{
                "name":   "auth",
                "status": "completed",
            },
        },
    })
    if err != nil {
        t.Fatalf("handleMarkTask: %v", err)
    }
    if !result.IsError {
        t.Fatal("expected error for invalid status")
    }
}

func TestHandleMarkTask_BlockedRequiresReason(t *testing.T) {
    dir := setupTestDir(t)
    gs := NewServer(dir)

    result, _ := gs.handleMarkTask(context.Background(), mcp.CallToolRequest{
        Params: mcp.CallToolParams{
            Arguments: map[string]interface{}{
                "name":   "auth",
                "status": "blocked",
            },
        },
    })
    if !result.IsError {
        t.Fatal("expected error when blocking without reason")
    }
}

func TestHandleSetPhase(t *testing.T) {
    dir := setupTestDir(t)
    gs := NewServer(dir)

    result, err := gs.handleSetPhase(context.Background(), mcp.CallToolRequest{
        Params: mcp.CallToolParams{
            Arguments: map[string]interface{}{
                "phase": "polishing",
            },
        },
    })
    if err != nil {
        t.Fatalf("handleSetPhase: %v", err)
    }
    if result.IsError {
        t.Fatalf("tool returned error: %v", result.Content)
    }

    state, _ := golemctx.ReadState(dir)
    if state.Status.Phase != "polishing" {
        t.Errorf("phase = %q, want %q", state.Status.Phase, "polishing")
    }
}

func TestHandleSetPhase_Invalid(t *testing.T) {
    dir := setupTestDir(t)
    gs := NewServer(dir)

    result, _ := gs.handleSetPhase(context.Background(), mcp.CallToolRequest{
        Params: mcp.CallToolParams{
            Arguments: map[string]interface{}{
                "phase": "reviewing",
            },
        },
    })
    if !result.IsError {
        t.Fatal("expected error for invalid phase")
    }
}

func TestHandleLogSession(t *testing.T) {
    dir := setupTestDir(t)
    gs := NewServer(dir)

    result, err := gs.handleLogSession(context.Background(), mcp.CallToolRequest{
        Params: mcp.CallToolParams{
            Arguments: map[string]interface{}{
                "task":          "auth",
                "outcome":       "done",
                "summary":       "implemented OAuth2 flow",
                "files_changed": []interface{}{"auth.go", "auth_test.go"},
            },
        },
    })
    if err != nil {
        t.Fatalf("handleLogSession: %v", err)
    }
    if result.IsError {
        t.Fatalf("tool returned error: %v", result.Content)
    }

    log, _ := golemctx.ReadLog(dir)
    if len(log.Sessions) != 1 {
        t.Fatalf("sessions = %d, want 1", len(log.Sessions))
    }
    if log.Sessions[0].Task != "auth" {
        t.Errorf("session task = %q", log.Sessions[0].Task)
    }
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/mcp/ -v`
Expected: FAIL (tool functions not defined)

**Step 3: Implement tools.go**

Create `internal/mcp/tools.go`:

```go
package mcp

import (
    "context"
    "fmt"
    "os"
    "syscall"
    "time"

    "github.com/mark3labs/mcp-go/mcp"
    golemctx "github.com/lofari/golem/internal/ctx"
)

// flock acquires an exclusive file lock. Returns unlock function.
func flock(path string) (func(), error) {
    f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0644)
    if err != nil {
        return nil, err
    }
    if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
        f.Close()
        return nil, err
    }
    return func() {
        syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
        f.Close()
    }, nil
}

func (gs *GolemServer) withStateLock(fn func(golemctx.State) (golemctx.State, error)) error {
    unlock, err := flock(golemctx.StatePath(gs.dir))
    if err != nil {
        return fmt.Errorf("acquiring state lock: %w", err)
    }
    defer unlock()

    state, err := golemctx.ReadState(gs.dir)
    if err != nil {
        return err
    }
    updated, err := fn(state)
    if err != nil {
        return err
    }
    return golemctx.WriteState(gs.dir, updated)
}

func (gs *GolemServer) withLogLock(fn func(golemctx.Log) (golemctx.Log, error)) error {
    unlock, err := flock(golemctx.LogPath(gs.dir))
    if err != nil {
        return fmt.Errorf("acquiring log lock: %w", err)
    }
    defer unlock()

    log, err := golemctx.ReadLog(gs.dir)
    if err != nil {
        return err
    }
    updated, err := fn(log)
    if err != nil {
        return err
    }
    return golemctx.WriteLog(gs.dir, updated)
}

func textResult(msg string) *mcp.CallToolResult {
    return &mcp.CallToolResult{
        Content: []mcp.Content{mcp.TextContent{Type: "text", Text: msg}},
    }
}

func errorResult(msg string) *mcp.CallToolResult {
    return &mcp.CallToolResult{
        IsError: true,
        Content: []mcp.Content{mcp.TextContent{Type: "text", Text: msg}},
    }
}

func getStr(args map[string]interface{}, key string) string {
    v, _ := args[key].(string)
    return v
}

func getStrSlice(args map[string]interface{}, key string) []string {
    raw, ok := args[key].([]interface{})
    if !ok {
        return nil
    }
    out := make([]string, 0, len(raw))
    for _, v := range raw {
        if s, ok := v.(string); ok {
            out = append(out, s)
        }
    }
    return out
}

// --- Tool Definitions ---

func markTaskTool() mcp.Tool {
    return mcp.Tool{
        Name:        "mark_task",
        Description: "Update a task's status and notes in state.yaml. Valid statuses: todo, in-progress, done, blocked. If status is 'blocked', blocked_reason is required.",
        InputSchema: mcp.ToolInputSchema{
            Type: "object",
            Properties: map[string]interface{}{
                "name":           map[string]string{"type": "string", "description": "Task name (must match exactly)"},
                "status":         map[string]string{"type": "string", "description": "New status: todo, in-progress, done, or blocked"},
                "notes":          map[string]string{"type": "string", "description": "Optional notes to set on the task"},
                "blocked_reason": map[string]string{"type": "string", "description": "Required when status is 'blocked'"},
            },
            Required: []string{"name", "status"},
        },
    }
}

func (gs *GolemServer) handleMarkTask(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    args := req.Params.Arguments
    name := getStr(args, "name")
    status := getStr(args, "status")
    notes := getStr(args, "notes")
    blockedReason := getStr(args, "blocked_reason")

    if !golemctx.ValidTaskStatuses()[status] {
        return errorResult(fmt.Sprintf("invalid status %q — must be one of: todo, in-progress, done, blocked", status)), nil
    }
    if status == "blocked" && blockedReason == "" {
        return errorResult("blocked_reason is required when status is 'blocked'"), nil
    }

    err := gs.withStateLock(func(s golemctx.State) (golemctx.State, error) {
        task := s.FindTask(name)
        if task == nil {
            return s, fmt.Errorf("task %q not found", name)
        }
        task.Status = status
        if notes != "" {
            task.Notes = notes
        }
        if blockedReason != "" {
            task.BlockedReason = blockedReason
        }
        return s, nil
    })
    if err != nil {
        return errorResult(err.Error()), nil
    }

    return textResult(fmt.Sprintf("task %q marked as %s", name, status)), nil
}

func setPhaseTool() mcp.Tool {
    return mcp.Tool{
        Name:        "set_phase",
        Description: "Set the project phase in state.yaml. Valid phases: planning, building, fixing, polishing.",
        InputSchema: mcp.ToolInputSchema{
            Type: "object",
            Properties: map[string]interface{}{
                "phase": map[string]string{"type": "string", "description": "New phase: planning, building, fixing, or polishing"},
            },
            Required: []string{"phase"},
        },
    }
}

func (gs *GolemServer) handleSetPhase(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    phase := getStr(req.Params.Arguments, "phase")

    if !golemctx.ValidPhases()[phase] {
        return errorResult(fmt.Sprintf("invalid phase %q — must be one of: planning, building, fixing, polishing", phase)), nil
    }

    err := gs.withStateLock(func(s golemctx.State) (golemctx.State, error) {
        s.Status.Phase = phase
        return s, nil
    })
    if err != nil {
        return errorResult(err.Error()), nil
    }

    return textResult(fmt.Sprintf("phase set to %q", phase)), nil
}

func addDecisionTool() mcp.Tool {
    return mcp.Tool{
        Name:        "add_decision",
        Description: "Append an architectural decision to state.yaml.",
        InputSchema: mcp.ToolInputSchema{
            Type: "object",
            Properties: map[string]interface{}{
                "what": map[string]string{"type": "string", "description": "What was decided"},
                "why":  map[string]string{"type": "string", "description": "Why this decision was made"},
            },
            Required: []string{"what", "why"},
        },
    }
}

func (gs *GolemServer) handleAddDecision(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    args := req.Params.Arguments
    what := getStr(args, "what")
    why := getStr(args, "why")

    err := gs.withStateLock(func(s golemctx.State) (golemctx.State, error) {
        s.Decisions = append(s.Decisions, golemctx.Decision{
            What: what,
            Why:  why,
            When: time.Now().Format("2006-01-02"),
        })
        return s, nil
    })
    if err != nil {
        return errorResult(err.Error()), nil
    }

    return textResult(fmt.Sprintf("decision added: %s", what)), nil
}

func addPitfallTool() mcp.Tool {
    return mcp.Tool{
        Name:        "add_pitfall",
        Description: "Record a pitfall/lesson learned in state.yaml.",
        InputSchema: mcp.ToolInputSchema{
            Type: "object",
            Properties: map[string]interface{}{
                "what": map[string]string{"type": "string", "description": "What went wrong or could go wrong"},
                "fix":  map[string]string{"type": "string", "description": "How to fix or avoid it"},
            },
            Required: []string{"what"},
        },
    }
}

func (gs *GolemServer) handleAddPitfall(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    args := req.Params.Arguments
    what := getStr(args, "what")
    fix := getStr(args, "fix")

    err := gs.withStateLock(func(s golemctx.State) (golemctx.State, error) {
        s.Pitfalls = append(s.Pitfalls, golemctx.Pitfall{What: what, Fix: fix})
        return s, nil
    })
    if err != nil {
        return errorResult(err.Error()), nil
    }

    return textResult(fmt.Sprintf("pitfall added: %s", what)), nil
}

func addLockedTool() mcp.Tool {
    return mcp.Tool{
        Name:        "add_locked",
        Description: "Lock a file path so it won't be modified by future iterations.",
        InputSchema: mcp.ToolInputSchema{
            Type: "object",
            Properties: map[string]interface{}{
                "path": map[string]string{"type": "string", "description": "File or directory path to lock"},
                "note": map[string]string{"type": "string", "description": "Why this path is locked"},
            },
            Required: []string{"path"},
        },
    }
}

func (gs *GolemServer) handleAddLocked(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    args := req.Params.Arguments
    path := getStr(args, "path")
    note := getStr(args, "note")

    err := gs.withStateLock(func(s golemctx.State) (golemctx.State, error) {
        s.Locked = append(s.Locked, golemctx.Lock{Path: path, Note: note})
        return s, nil
    })
    if err != nil {
        return errorResult(err.Error()), nil
    }

    return textResult(fmt.Sprintf("locked: %s", path)), nil
}

func logSessionTool() mcp.Tool {
    return mcp.Tool{
        Name:        "log_session",
        Description: "Append a session entry to log.yaml. Call this at the end of each iteration.",
        InputSchema: mcp.ToolInputSchema{
            Type: "object",
            Properties: map[string]interface{}{
                "task":          map[string]string{"type": "string", "description": "Task name worked on"},
                "outcome":       map[string]string{"type": "string", "description": "Outcome: done, partial, blocked, or unproductive"},
                "summary":       map[string]string{"type": "string", "description": "Brief summary of work done"},
                "files_changed": map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}, "description": "List of files modified"},
            },
            Required: []string{"task", "outcome", "summary"},
        },
    }
}

func (gs *GolemServer) handleLogSession(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    args := req.Params.Arguments
    task := getStr(args, "task")
    outcome := getStr(args, "outcome")
    summary := getStr(args, "summary")
    filesChanged := getStrSlice(args, "files_changed")

    validOutcomes := map[string]bool{"done": true, "partial": true, "blocked": true, "unproductive": true}
    if !validOutcomes[outcome] {
        return errorResult(fmt.Sprintf("invalid outcome %q — must be one of: done, partial, blocked, unproductive", outcome)), nil
    }

    err := gs.withLogLock(func(l golemctx.Log) (golemctx.Log, error) {
        iteration := len(l.Sessions) + 1
        l.Sessions = append(l.Sessions, golemctx.Session{
            Iteration:    iteration,
            Timestamp:    time.Now().Format(time.RFC3339),
            Task:         task,
            Outcome:      outcome,
            Summary:      summary,
            FilesChanged: filesChanged,
        })
        return l, nil
    })
    if err != nil {
        return errorResult(err.Error()), nil
    }

    return textResult(fmt.Sprintf("session logged: %s — %s", task, outcome)), nil
}
```

**Step 4: Run tests**

Run: `go test ./internal/mcp/ -v`
Expected: All PASS.

**Step 5: Commit**

```bash
git add internal/mcp/tools.go internal/mcp/tools_test.go
git commit -m "feat(mcp): implement mark_task, set_phase, add_decision, add_pitfall, add_locked, log_session tools"
```

---

### Task 5: MCP server — CLI subcommand and runner integration

**Files:**
- Create: `cmd/mcp_serve.go`
- Modify: `internal/runner/command.go` (write mcp config, pass `--mcp-config`)
- Modify: `internal/runner/command_test.go` (update tests)

**Step 1: Create `cmd/mcp_serve.go`**

```go
package cmd

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"
    golemmcp "github.com/lofari/golem/internal/mcp"
)

var mcpServeCmd = &cobra.Command{
    Use:    "mcp-serve",
    Short:  "Run the golem MCP server (stdio)",
    Hidden: true, // internal — spawned by runner, not user-facing
    RunE: func(cmd *cobra.Command, args []string) error {
        dir, _ := cmd.Flags().GetString("dir")
        if dir == "" {
            var err error
            dir, err = os.Getwd()
            if err != nil {
                return err
            }
        }

        s := golemmcp.NewServer(dir)
        return s.ServeStdio()
    },
}

func init() {
    rootCmd.AddCommand(mcpServeCmd)
    mcpServeCmd.Flags().String("dir", "", "project directory")
}
```

**Step 2: Add MCPConfig field to ClaudeRunner**

In `internal/runner/command.go`, add to `ClaudeRunner` struct:

```go
MCPConfig string // path to mcp_servers.json (if set, passes --mcp-config)
```

**Step 3: Pass --mcp-config to claude if set**

In `ClaudeRunner.Run()`, after the `--plugin-dir` loop (line 43), add:

```go
if c.MCPConfig != "" {
    args = append(args, "--mcp-config", c.MCPConfig)
}
```

**Step 4: Add WriteMCPConfig helper**

In `internal/runner/command.go`, add:

```go
// WriteMCPConfig writes a temporary mcp_servers.json for this session.
// Returns the path to the config file.
func WriteMCPConfig(dir string) (string, error) {
    golemBin, err := os.Executable()
    if err != nil {
        return "", fmt.Errorf("finding golem binary: %w", err)
    }

    config := fmt.Sprintf(`{
  "mcpServers": {
    "golem": {
      "command": %q,
      "args": ["mcp-serve", "--dir", %q]
    }
  }
}`, golemBin, dir)

    configPath := filepath.Join(dir, ".ctx", "mcp_servers.json")
    if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
        return "", fmt.Errorf("writing mcp config: %w", err)
    }
    return configPath, nil
}
```

**Step 5: Integrate MCP config into builder loop**

In `internal/runner/builder.go`, before the `cfg.Runner.Run()` call (line 116), add MCP config setup. This requires adding `MCPConfig` to `BuilderConfig`:

```go
// In BuilderConfig struct:
MCPEnabled bool

// Before cfg.Runner.Run() in the loop:
if cfg.MCPEnabled {
    if claudeRunner, ok := cfg.Runner.(*ClaudeRunner); ok {
        mcpPath, mcpErr := WriteMCPConfig(cfg.Dir)
        if mcpErr != nil {
            fmt.Fprintf(os.Stderr, "golem: warning: could not write MCP config: %v\n", mcpErr)
        } else {
            claudeRunner.MCPConfig = mcpPath
        }
    }
}
```

**Step 6: Wire --mcp flag in cmd/run.go**

In `cmd/run.go`, add flag and pass through:

```go
// In init():
runCmd.Flags().Bool("mcp", true, "enable golem MCP server for structured state updates")

// In RunE, read the flag:
mcpEnabled, _ := cmd.Flags().GetBool("mcp")

// Pass to BuilderConfig:
MCPEnabled: mcpEnabled,
```

**Step 7: Run all tests**

Run: `go build ./... && go test ./...`
Expected: All pass.

**Step 8: Commit**

```bash
git add cmd/mcp_serve.go internal/runner/command.go internal/runner/builder.go cmd/run.go
git commit -m "feat(mcp): add mcp-serve subcommand and --mcp-config integration"
```

---

### Task 6: Update prompt template for MCP tools

**Files:**
- Modify: `templates/prompt.md`

**Step 1: Update end-of-session instructions**

Replace the "End of Session" section in `templates/prompt.md` with:

```markdown
## End of Session
Before exiting, use the golem MCP tools to update state:
1. Call `mark_task` to update the task you worked on (set status and notes).
2. Call `set_phase` if the project phase has changed.
3. Call `add_decision` for any new architectural decisions.
4. Call `add_pitfall` for any lessons learned.
5. Call `add_locked` for any completed, tested modules that should not be modified.
6. Call `log_session` with task name, outcome (done|partial|blocked|unproductive), summary, and files_changed.

If the golem MCP tools are not available, fall back to editing `.ctx/state.yaml` and `.ctx/log.yaml` directly.
Valid task statuses: `todo`, `in-progress`, `done`, `blocked`.
Valid phases: `planning`, `building`, `fixing`, `polishing`.
```

**Step 2: Run tests**

Run: `go build ./... && go test ./...`
Expected: All pass.

**Step 3: Commit**

```bash
git add templates/prompt.md
git commit -m "docs(prompt): update end-of-session to use golem MCP tools"
```

---

### Task 7: Parallel tasks — eligible task selection

**Files:**
- Create: `internal/runner/parallel.go`
- Create: `internal/runner/parallel_test.go`

**Step 1: Write parallel_test.go — eligible task selection tests**

Create `internal/runner/parallel_test.go`:

```go
package runner

import (
    "testing"

    golemctx "github.com/lofari/golem/internal/ctx"
)

func TestEligibleTasks(t *testing.T) {
    tasks := []golemctx.Task{
        {Name: "auth", Status: "done"},
        {Name: "api", Status: "todo"},
        {Name: "ui", Status: "todo", DependsOn: golemctx.FlexString{"api"}},
        {Name: "tests", Status: "todo"},
        {Name: "deploy", Status: "blocked", BlockedReason: "waiting"},
        {Name: "docs", Status: "in-progress"},
    }

    eligible := EligibleTasks(tasks)

    // api and tests are eligible (todo, no unresolved deps)
    // ui depends on api which isn't done — not eligible
    // deploy is blocked — not eligible
    // docs is in-progress — eligible (should be picked up)
    // auth is done — not eligible
    names := make(map[string]bool)
    for _, t := range eligible {
        names[t.Name] = true
    }

    if !names["api"] {
        t.Error("api should be eligible")
    }
    if !names["tests"] {
        t.Error("tests should be eligible")
    }
    if !names["docs"] {
        t.Error("docs (in-progress) should be eligible")
    }
    if names["ui"] {
        t.Error("ui should NOT be eligible (depends on api)")
    }
    if names["deploy"] {
        t.Error("deploy should NOT be eligible (blocked)")
    }
    if names["auth"] {
        t.Error("auth should NOT be eligible (done)")
    }
    if len(eligible) != 3 {
        t.Errorf("got %d eligible, want 3", len(eligible))
    }
}

func TestSanitizeTaskName(t *testing.T) {
    tests := []struct {
        input string
        want  string
    }{
        {"Simple Task", "simple-task"},
        {"[review] Fix the bug!", "review-fix-the-bug"},
        {"Task with    spaces", "task-with-spaces"},
        {"A very long task name that exceeds the maximum length allowed for worktree directory names and should be truncated", "a-very-long-task-name-that-exceeds-the-maximum-length-allowed-for-workt"},
    }
    for _, tt := range tests {
        got := SanitizeTaskName(tt.input)
        if got != tt.want {
            t.Errorf("SanitizeTaskName(%q) = %q, want %q", tt.input, got, tt.want)
        }
    }
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/runner/ -run "TestEligible|TestSanitize" -v`
Expected: FAIL

**Step 3: Implement parallel.go — task selection and name sanitization**

Create `internal/runner/parallel.go`:

```go
package runner

import (
    "regexp"
    "strings"

    golemctx "github.com/lofari/golem/internal/ctx"
)

const maxWorktreeNameLen = 70

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9-]+`)
var multiDash = regexp.MustCompile(`-{2,}`)

// EligibleTasks returns tasks that can be worked on: todo or in-progress,
// not blocked, and all dependencies are done.
func EligibleTasks(tasks []golemctx.Task) []golemctx.Task {
    doneSet := make(map[string]bool)
    for _, t := range tasks {
        if t.Status == "done" {
            doneSet[t.Name] = true
        }
    }

    var eligible []golemctx.Task
    for _, t := range tasks {
        if t.Status != "todo" && t.Status != "in-progress" {
            continue
        }
        // Check all dependencies are done
        allDepsDone := true
        for _, dep := range t.DependsOn {
            if !doneSet[dep] {
                allDepsDone = false
                break
            }
        }
        if allDepsDone {
            eligible = append(eligible, t)
        }
    }
    return eligible
}

// SanitizeTaskName converts a task name into a safe directory name for worktrees.
func SanitizeTaskName(name string) string {
    s := strings.ToLower(name)
    s = nonAlphaNum.ReplaceAllString(s, "-")
    s = multiDash.ReplaceAllString(s, "-")
    s = strings.Trim(s, "-")
    if len(s) > maxWorktreeNameLen {
        s = s[:maxWorktreeNameLen]
        s = strings.TrimRight(s, "-")
    }
    return s
}
```

**Step 4: Run tests**

Run: `go test ./internal/runner/ -run "TestEligible|TestSanitize" -v`
Expected: All PASS.

**Step 5: Commit**

```bash
git add internal/runner/parallel.go internal/runner/parallel_test.go
git commit -m "feat(runner): eligible task selection and name sanitization for parallel execution"
```

---

### Task 8: Parallel tasks — worktree management and concurrent execution

**Files:**
- Modify: `internal/runner/parallel.go` (add worktree and merge logic)
- Modify: `internal/runner/parallel_test.go` (add worktree tests)
- Modify: `internal/runner/builder.go` (parallel dispatch)
- Modify: `cmd/run.go` (add --parallel flag)

**Step 1: Add worktree and merge functions to parallel.go**

Append to `internal/runner/parallel.go`:

```go
import (
    "context"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "sort"
    "sync"
)

// ParallelResult holds results from one parallel task execution.
type ParallelResult struct {
    Task    string
    Output  string
    Err     error
    Merged  bool
}

// CreateWorktree creates a git worktree for a task.
// Returns the worktree path and branch name.
func CreateWorktree(projectDir string, taskName string) (string, string, error) {
    dirName := SanitizeTaskName(taskName)
    wtDir := filepath.Join(projectDir, ".ctx", "worktrees", dirName)
    branch := "golem/" + dirName

    // Create worktree with new branch
    cmd := exec.Command("git", "worktree", "add", "-b", branch, wtDir)
    cmd.Dir = projectDir
    if out, err := cmd.CombinedOutput(); err != nil {
        return "", "", fmt.Errorf("creating worktree: %s: %w", string(out), err)
    }

    return wtDir, branch, nil
}

// MergeWorktree merges a worktree branch back into the current branch.
// Returns true if merged successfully, false if conflict.
func MergeWorktree(projectDir string, branch string) (bool, error) {
    cmd := exec.Command("git", "merge", "--no-edit", branch)
    cmd.Dir = projectDir
    if out, err := cmd.CombinedOutput(); err != nil {
        // Check if it's a merge conflict
        abortCmd := exec.Command("git", "merge", "--abort")
        abortCmd.Dir = projectDir
        abortCmd.Run()
        return false, fmt.Errorf("merge conflict: %s", string(out))
    }
    return true, nil
}

// CleanupWorktree removes a worktree and its branch.
func CleanupWorktree(projectDir string, wtDir string, branch string) {
    exec.Command("git", "worktree", "remove", "--force", wtDir).Run()
    exec.Command("git", "branch", "-D", branch).Run()
}

// RunParallel executes N tasks concurrently in separate worktrees.
func RunParallel(ctx context.Context, cfg BuilderConfig, tasks []golemctx.Task, iteration int) []ParallelResult {
    results := make([]ParallelResult, len(tasks))
    var wg sync.WaitGroup

    for i, task := range tasks {
        wg.Add(1)
        go func(idx int, t golemctx.Task) {
            defer wg.Done()
            results[idx] = runInWorktree(ctx, cfg, t, iteration)
        }(i, task)
    }

    wg.Wait()
    return results
}

func runInWorktree(ctx context.Context, cfg BuilderConfig, task golemctx.Task, iteration int) ParallelResult {
    result := ParallelResult{Task: task.Name}

    wtDir, branch, err := CreateWorktree(cfg.Dir, task.Name)
    if err != nil {
        result.Err = err
        return result
    }

    // Render prompt with forced task
    iterCtx := BuildIterationContext(iteration, cfg.MaxIterations, 1)
    taskOverride := BuildTaskOverride(task.Name)
    prompt, err := RenderPrompt(cfg.Dir, "prompt.md", PromptVars{
        IterationContext: iterCtx,
        TaskOverride:     taskOverride,
    })
    if err != nil {
        result.Err = fmt.Errorf("rendering prompt: %w", err)
        CleanupWorktree(cfg.Dir, wtDir, branch)
        return result
    }

    // Run claude in the worktree directory
    output, err := cfg.Runner.Run(ctx, wtDir, prompt, cfg.MaxTurns, cfg.Model)
    result.Output = output
    if err != nil {
        result.Err = err
        CleanupWorktree(cfg.Dir, wtDir, branch)
        return result
    }

    // Save session output
    SaveSessionOutput(cfg.Dir, fmt.Sprintf("parallel-%s", SanitizeTaskName(task.Name)), iteration, output)

    return result
}

// MergeParallelResults merges completed worktrees back, in alphabetical order.
func MergeParallelResults(projectDir string, results []ParallelResult) {
    // Sort by task name for deterministic merge order
    sort.Slice(results, func(i, j int) bool {
        return results[i].Task < results[j].Task
    })

    for i := range results {
        if results[i].Err != nil {
            continue
        }

        branch := "golem/" + SanitizeTaskName(results[i].Task)
        wtDir := filepath.Join(projectDir, ".ctx", "worktrees", SanitizeTaskName(results[i].Task))

        merged, err := MergeWorktree(projectDir, branch)
        if err != nil {
            fmt.Fprintf(os.Stderr, "golem: merge conflict for task %q — will retry next iteration\n", results[i].Task)
            results[i].Merged = false
        } else {
            results[i].Merged = merged
        }

        CleanupWorktree(projectDir, wtDir, branch)
    }
}
```

**Step 2: Add --parallel flag to cmd/run.go**

In `cmd/run.go init()`:

```go
runCmd.Flags().Int("parallel", 1, "max parallel task sessions (1 = sequential)")
```

Read it in RunE and pass to BuilderConfig:

```go
parallel, _ := cmd.Flags().GetInt("parallel")

// Add to BuilderConfig:
Parallel: parallel,
```

**Step 3: Add Parallel field to BuilderConfig**

In `internal/runner/builder.go`:

```go
type BuilderConfig struct {
    // ... existing fields ...
    Parallel int // max parallel sessions (1 = sequential)
}
```

**Step 4: Add parallel dispatch to builder loop**

In `internal/runner/builder.go`, after the prompt render and before `cfg.Runner.Run()`, add parallel path:

```go
// Check for parallel execution
if cfg.Parallel > 1 {
    eligible := EligibleTasks(state.Tasks)
    if len(eligible) >= 2 {
        // Cap at configured parallelism
        n := cfg.Parallel
        if n > len(eligible) {
            n = len(eligible)
        }
        batch := eligible[:n]

        fmt.Fprintf(os.Stderr, "golem: parallel iteration %d — running %d tasks concurrently\n", i, len(batch))
        cfg.emit(Event{Type: EventIterStart, Iter: i, MaxIter: cfg.MaxIterations})

        results := RunParallel(ctx, cfg, batch, i)
        MergeParallelResults(cfg.Dir, results)

        // Count successes
        merged := 0
        for _, r := range results {
            if r.Merged {
                merged++
            }
        }
        fmt.Fprintf(os.Stderr, "golem: parallel iteration %d — %d/%d tasks merged\n", i, merged, len(batch))
        cfg.emit(Event{Type: EventIterEnd, Iter: i, Task: fmt.Sprintf("%d parallel tasks", len(batch))})

        result.Iterations = i
        continue // skip the sequential path
    }
}
```

**Step 5: Run all tests**

Run: `go build ./... && go test ./...`
Expected: All pass.

**Step 6: Commit**

```bash
git add internal/runner/parallel.go internal/runner/parallel_test.go internal/runner/builder.go cmd/run.go
git commit -m "feat(runner): parallel task execution via git worktrees with --parallel flag"
```

---

### Task 9: Integration testing and final verification

**Step 1: Run full test suite**

Run: `go test ./... -v`
Expected: All pass.

**Step 2: Build and verify CLI**

Run: `go build -o golem . && ./golem --help`
Verify: `mcp-serve` appears (hidden), `run --parallel` flag exists.

**Step 3: Verify MCP server starts**

Run: `echo '{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | ./golem mcp-serve --dir /tmp`
Expected: JSON response with server capabilities and tool list.

**Step 4: Commit any fixes**

```bash
git add -A
git commit -m "fix: integration test fixes"
```

**Step 5: Push**

```bash
git push
```

---

Plan complete and saved to `docs/plans/2026-03-04-resilience-and-parallelism.md`. Two execution options:

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

Which approach?
