# Blueprint Builder Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Express the legacy builder loop as a blueprint agent with explicit context assembly, deterministic task selection, and strategy evaluation.

**Architecture:** Four new builtin steps (`init-state`, `sync-state`, `pick-task`, `build-context`, `strategy-eval`) and one predicate (`tasks-remaining`) are added to the blueprint engine. A `builder.yaml` agent YAML wires them into a `while` loop. The engine's `execBuiltinStep` result storage is fixed to support multi-key writes.

**Tech Stack:** Go, stdlib `testing`, existing `internal/ctx`, `internal/runner`, `internal/graph/context` packages.

**Spec:** `docs/superpowers/specs/2026-03-15-blueprint-builder-design.md`

---

## Chunk 1: Engine Fix + Predicate + State Builtins

### Task 1: Fix `execBuiltinStep` Multi-Key Result Storage

**Files:**
- Modify: `internal/runner/engine.go:380-396`
- Test: `internal/runner/engine_test.go`

- [ ] **Step 1: Write the failing test**

```go
// In engine_test.go — add test for per-key result extraction
func TestExecBuiltinStep_MultiKeyResult(t *testing.T) {
	// Create a minimal engine with a step that writes multiple keys
	bp := &Blueprint{
		pipeline: &Pipeline{
			StepDefs: map[string]*Step{},
			Nodes:    nil,
		},
	}
	e := &Engine{
		cfg:   EngineConfig{Blueprint: bp},
		state: map[string]any{},
	}

	// Simulate a PrimitiveResult with multiple keys
	result := PrimitiveResult{
		"project-context": map[string]any{"phase": "building"},
		"tasks":           []any{map[string]any{"name": "task1", "status": "todo"}},
		"log-context":     map[string]any{"iteration": 1},
	}

	step := &Step{
		Name:   "test-multi-key",
		Type:   StepTypeBuiltin,
		Writes: []string{"project-context", "tasks", "log-context"},
	}

	// Call storeBuiltinResult directly
	e.storeBuiltinResult(step, result)

	// Each key should have its own distinct value
	pc, ok := e.state["project-context"].(map[string]any)
	if !ok {
		t.Fatal("project-context should be a map")
	}
	if pc["phase"] != "building" {
		t.Errorf("project-context.phase = %v, want building", pc["phase"])
	}

	tasks, ok := e.state["tasks"].([]any)
	if !ok {
		t.Fatal("tasks should be a slice")
	}
	if len(tasks) != 1 {
		t.Errorf("tasks len = %d, want 1", len(tasks))
	}

	lc, ok := e.state["log-context"].(map[string]any)
	if !ok {
		t.Fatal("log-context should be a map")
	}
	if lc["iteration"] != 1 {
		t.Errorf("log-context.iteration = %v, want 1", lc["iteration"])
	}
}

// Verify backward compatibility: single-key builtins still get full result
func TestExecBuiltinStep_SingleKeyFallback(t *testing.T) {
	bp := &Blueprint{
		pipeline: &Pipeline{
			StepDefs: map[string]*Step{},
			Nodes:    nil,
		},
	}
	e := &Engine{
		cfg:   EngineConfig{Blueprint: bp},
		state: map[string]any{},
	}

	result := PrimitiveResult{
		"status": "pass",
		"output": "all tests passed",
	}

	step := &Step{
		Name:   "run-tests",
		Type:   StepTypeBuiltin,
		Writes: []string{"test-results"},
	}

	e.storeBuiltinResult(step, result)

	// "test-results" key not in result, so should get full map
	tr, ok := e.state["test-results"].(map[string]any)
	if !ok {
		t.Fatal("test-results should be a map")
	}
	if tr["status"] != "pass" {
		t.Errorf("test-results.status = %v, want pass", tr["status"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runner/ -run TestExecBuiltinStep -v`
Expected: Compile error — `storeBuiltinResult` not defined yet

- [ ] **Step 3: Extract result storage into `storeBuiltinResult` and fix the logic**

In `internal/runner/engine.go`, replace lines 378-396 with:

```go
	if err != nil {
		return err
	}

	e.storeBuiltinResult(step, result)
	return nil
}

// storeBuiltinResult writes a PrimitiveResult into pipeline state based on the step's Writes.
// If the result contains a key matching a write key name, that specific value is stored.
// Otherwise, the full result map is stored (backward-compatible with single-key builtins).
func (e *Engine) storeBuiltinResult(step *Step, result PrimitiveResult) {
	if len(step.Writes) > 0 {
		for _, key := range step.Writes {
			if reservedKeys[key] {
				if val, ok := result[key]; ok {
					e.state[key] = val
				}
			} else if val, ok := result[key]; ok {
				e.state[key] = val
			} else {
				e.state[key] = map[string]any(result)
			}
		}
	} else {
		for k, v := range result {
			e.state[k] = v
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/runner/ -run TestExecBuiltinStep -v`
Expected: PASS

- [ ] **Step 5: Run all existing tests to verify no regressions**

Run: `go test ./internal/runner/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/runner/engine.go internal/runner/engine_test.go
git commit -m "fix(runner): extract storeBuiltinResult with per-key result mapping"
```

---

### Task 2: Add `tasks-remaining` Predicate

**Files:**
- Modify: `internal/runner/predicates.go:5-28`
- Test: `internal/runner/predicates_test.go`

- [ ] **Step 1: Write the failing test**

```go
// In predicates_test.go
func TestTasksRemainingPredicate(t *testing.T) {
	tests := []struct {
		name  string
		state map[string]any
		want  bool
	}{
		{
			name:  "no tasks key",
			state: map[string]any{},
			want:  false,
		},
		{
			name: "all done",
			state: map[string]any{
				"tasks": []any{
					map[string]any{"name": "t1", "status": "done"},
					map[string]any{"name": "t2", "status": "done"},
				},
			},
			want: false,
		},
		{
			name: "has todo",
			state: map[string]any{
				"tasks": []any{
					map[string]any{"name": "t1", "status": "done"},
					map[string]any{"name": "t2", "status": "todo"},
				},
			},
			want: true,
		},
		{
			name: "has in-progress",
			state: map[string]any{
				"tasks": []any{
					map[string]any{"name": "t1", "status": "in-progress"},
				},
			},
			want: true,
		},
		{
			name: "only blocked tasks",
			state: map[string]any{
				"tasks": []any{
					map[string]any{"name": "t1", "status": "blocked"},
				},
			},
			want: false,
		},
		{
			name: "halt flag set",
			state: map[string]any{
				"tasks": []any{
					map[string]any{"name": "t1", "status": "todo"},
				},
				"_halt": true,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := evalBuiltinPredicate("tasks-remaining", tt.state, nil)
			if !found {
				t.Fatal("predicate not recognized")
			}
			if got != tt.want {
				t.Errorf("tasks-remaining = %v, want %v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runner/ -run TestTasksRemainingPredicate -v`
Expected: FAIL — `tasks-remaining` returns `(false, false)` from default case

- [ ] **Step 3: Implement the predicate**

In `internal/runner/predicates.go`, add a case before `default:`:

```go
	case "tasks-remaining":
		if halt, _ := state["_halt"].(bool); halt {
			return false, true
		}
		tasks, ok := state["tasks"].([]any)
		if !ok {
			return false, true
		}
		for _, t := range tasks {
			tm, ok := t.(map[string]any)
			if !ok {
				continue
			}
			status, _ := tm["status"].(string)
			if status == "todo" || status == "in-progress" {
				return true, true
			}
		}
		return false, true
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/runner/ -run TestTasksRemainingPredicate -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runner/predicates.go internal/runner/predicates_test.go
git commit -m "feat(runner): add tasks-remaining predicate for builder blueprint"
```

---

### Task 3: Add `init-state` and `sync-state` Builtins

**Files:**
- Create: `internal/runner/builder_primitives.go`
- Create: `internal/runner/builder_primitives_test.go`
- Modify: `internal/runner/engine.go` (add cases to `execBuiltinStep`)

- [ ] **Step 1: Write the failing tests**

```go
// internal/runner/builder_primitives_test.go
package runner

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	golemctx "github.com/lofari/golem/internal/ctx"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ctxDir := filepath.Join(dir, ".ctx")
	os.MkdirAll(ctxDir, 0755)

	state := golemctx.State{
		Project: golemctx.Project{
			Name:     "test-project",
			DocsPath: "docs/",
		},
		Status: golemctx.Status{Phase: "building", CurrentFocus: "testing"},
		Decisions: []golemctx.Decision{
			{What: "use sqlite", Why: "simple", When: "2026-01-01"},
		},
		Tasks: []golemctx.Task{
			{Name: "task-1", Status: "done"},
			{Name: "task-2", Status: "todo", Notes: "implement feature"},
			{Name: "task-3", Status: "todo", DependsOn: golemctx.FlexString{"task-2"}},
		},
		Pitfalls: []golemctx.Pitfall{
			{What: "watch out for nulls", Fix: "add nil checks"},
		},
	}
	golemctx.WriteState(dir, state)

	log := golemctx.Log{
		Sessions: []golemctx.Session{
			{
				Iteration: 1, Task: "task-1", Outcome: "done",
				Handoff: "task-1 complete, move to task-2",
				Timestamp: "2026-01-01T00:00:00Z",
			},
		},
	}
	golemctx.WriteLog(dir, log)

	return dir
}

func TestPrimitiveInitState(t *testing.T) {
	dir := setupTestDir(t)

	// Initialize git repo for HEAD recording
	gitInit(t, dir)

	pipelineState := map[string]any{}
	result, err := primitiveInitState(nil, dir, nil, pipelineState)
	if err != nil {
		t.Fatalf("primitiveInitState: %v", err)
	}

	// Check project-context
	pc, ok := result["project-context"].(map[string]any)
	if !ok {
		t.Fatal("project-context should be a map")
	}
	decisions, ok := pc["decisions"].([]golemctx.Decision)
	if !ok || len(decisions) != 1 {
		t.Errorf("expected 1 decision, got %v", pc["decisions"])
	}

	// Check tasks
	tasks, ok := result["tasks"].([]any)
	if !ok {
		t.Fatal("tasks should be a slice")
	}
	if len(tasks) != 3 {
		t.Errorf("tasks len = %d, want 3", len(tasks))
	}

	// Check log-context
	lc, ok := result["log-context"].(map[string]any)
	if !ok {
		t.Fatal("log-context should be a map")
	}
	if lc["last_task"] != "task-1" {
		t.Errorf("last_task = %v, want task-1", lc["last_task"])
	}
	if lc["last_handoff"] != "task-1 complete, move to task-2" {
		t.Errorf("last_handoff = %v", lc["last_handoff"])
	}

	// Check snapshot was saved
	snapshots, _ := filepath.Glob(filepath.Join(dir, ".ctx", "snapshots", "state-*.yaml"))
	if len(snapshots) == 0 {
		t.Error("expected a snapshot to be saved")
	}

	// Check _head_before was set
	if pipelineState["_head_before"] == nil {
		t.Error("_head_before should be set in pipeline state")
	}
	if pipelineState["_sync_initialized"] != true {
		t.Error("_sync_initialized should be true")
	}
}

func TestPrimitiveSyncState(t *testing.T) {
	dir := setupTestDir(t)
	gitInit(t, dir)

	pipelineState := map[string]any{
		"_sync_initialized":   true,
		"_head_before":        "abc123",
		"_last_log_iteration": 1, // matches current log
	}

	result, err := primitiveSyncState(nil, dir, nil, pipelineState)
	if err != nil {
		t.Fatalf("primitiveSyncState: %v", err)
	}

	// Check log-context has agent_logged
	lc, ok := result["log-context"].(map[string]any)
	if !ok {
		t.Fatal("log-context should be a map")
	}
	// iteration count is still 1 (same as _last_log_iteration), so agent_logged = false
	if lc["agent_logged"] != false {
		t.Errorf("agent_logged = %v, want false (iteration didn't increment)", lc["agent_logged"])
	}
}

func TestPrimitiveSyncState_AgentLogged(t *testing.T) {
	dir := setupTestDir(t)
	gitInit(t, dir)

	// Simulate agent writing a log session
	golemctx.AppendSession(dir, golemctx.Session{
		Iteration: 2, Task: "task-2", Outcome: "done",
		Timestamp: time.Now().Format(time.RFC3339),
	})

	pipelineState := map[string]any{
		"_sync_initialized":   true,
		"_head_before":        "abc123",
		"_last_log_iteration": 1, // was 1 before agent ran
	}

	result, err := primitiveSyncState(nil, dir, nil, pipelineState)
	if err != nil {
		t.Fatalf("primitiveSyncState: %v", err)
	}

	lc := result["log-context"].(map[string]any)
	if lc["agent_logged"] != true {
		t.Errorf("agent_logged = %v, want true (iteration incremented)", lc["agent_logged"])
	}
}

// gitInit creates a minimal git repo for HEAD recording
func gitInit(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "test"},
		{"git", "add", "."},
		{"git", "commit", "-m", "init", "--allow-empty"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
}
```

Note: add `"os/exec"` and `"time"` to the test file's import block.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/runner/ -run TestPrimitiveInitState -v`
Expected: Compile error — `primitiveInitState` not defined

- [ ] **Step 3: Implement `builder_primitives.go`**

Create `internal/runner/builder_primitives.go`:

```go
package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	golemctx "github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/graph"
	graphctx "github.com/lofari/golem/internal/graph/context"
	"github.com/lofari/golem/internal/graph/embed"
)

// readProjectState reads state.yaml and log.yaml and returns a PrimitiveResult
// with project-context, tasks, and log-context keys.
// pipelineState is used to read/write internal keys (_head_before, etc.).
func readProjectState(dir string, pipelineState map[string]any) (PrimitiveResult, error) {
	state, err := golemctx.ReadState(dir)
	if err != nil {
		return nil, &UnrecoverableError{Msg: fmt.Sprintf("init-state: %v", err)}
	}

	log, err := golemctx.ReadLog(dir)
	if err != nil {
		return nil, &UnrecoverableError{Msg: fmt.Sprintf("init-state: reading log: %v", err)}
	}

	// Build project-context
	projectContext := map[string]any{
		"decisions":     state.Decisions,
		"pitfalls":      state.Pitfalls,
		"phase":         state.Status.Phase,
		"current_focus": state.Status.CurrentFocus,
		"docs_path":     state.Project.DocsPath,
	}

	// Build tasks as []any for pipeline state compatibility
	tasks := make([]any, len(state.Tasks))
	for i, t := range state.Tasks {
		tm := map[string]any{
			"name":   t.Name,
			"status": t.Status,
		}
		if t.Notes != "" {
			tm["notes"] = t.Notes
		}
		if !t.DependsOn.IsEmpty() {
			tm["depends_on"] = []string(t.DependsOn)
		}
		if t.BlockedReason != "" {
			tm["blocked_reason"] = t.BlockedReason
		}
		tasks[i] = tm
	}

	// Build log-context
	logContext := map[string]any{
		"iteration": len(log.Sessions),
	}
	if len(log.Sessions) > 0 {
		last := log.Sessions[len(log.Sessions)-1]
		logContext["last_task"] = last.Task
		logContext["last_outcome"] = last.Outcome
		logContext["last_handoff"] = last.Handoff
	}

	// Compute diff stat from _head_before if available
	if headBefore, ok := pipelineState["_head_before"].(string); ok && headBefore != "" {
		diffStat := gitDiffStat(dir, headBefore)
		if diffStat != "" {
			logContext["last_diff_stat"] = diffStat
		}
	}

	return PrimitiveResult{
		"project-context": projectContext,
		"tasks":           tasks,
		"log-context":     logContext,
	}, nil
}

// primitiveInitState runs once before the loop. Saves snapshot, records HEAD.
func primitiveInitState(_ context.Context, dir string, config map[string]any, pipelineState map[string]any) (PrimitiveResult, error) {
	// Save snapshot
	if err := SaveSnapshot(dir, 0); err != nil {
		fmt.Fprintf(os.Stderr, "golem: warning: could not save snapshot: %v\n", err)
	}

	// Record HEAD
	head := gitHead(dir)
	pipelineState["_head_before"] = head
	pipelineState["_sync_initialized"] = true

	result, err := readProjectState(dir, pipelineState)
	if err != nil {
		return nil, err
	}

	// Record initial log iteration count for agent_logged detection
	if lc, ok := result["log-context"].(map[string]any); ok {
		pipelineState["_last_log_iteration"] = lc["iteration"]
	}

	return result, nil
}

// primitiveSyncState runs inside the loop. Validates state, detects agent logging.
func primitiveSyncState(_ context.Context, dir string, config map[string]any, pipelineState map[string]any) (PrimitiveResult, error) {
	// Validate and auto-repair state
	state, err := golemctx.ReadState(dir)
	if err != nil {
		// Try snapshot restore
		restored, restoreErr := RestoreLatestSnapshot(dir)
		if !restored || restoreErr != nil {
			return nil, &UnrecoverableError{Msg: fmt.Sprintf("sync-state: state unreadable, no snapshot: %v", err)}
		}
		state, err = golemctx.ReadState(dir)
		if err != nil {
			return nil, &UnrecoverableError{Msg: fmt.Sprintf("sync-state: state still unreadable after restore: %v", err)}
		}
	}

	// Auto-repair invalid phases/statuses
	repaired := false
	if state.Status.Phase != "" {
		if _, ok := golemctx.ValidPhases()[state.Status.Phase]; !ok {
			state.Status.Phase = "building"
			repaired = true
		}
	}
	for i := range state.Tasks {
		if _, ok := golemctx.ValidTaskStatuses()[state.Tasks[i].Status]; !ok {
			state.Tasks[i].Status = "todo"
			repaired = true
		}
		if state.Tasks[i].Status == "blocked" && state.Tasks[i].BlockedReason == "" {
			state.Tasks[i].BlockedReason = "no reason provided by agent"
			repaired = true
		}
	}
	if repaired {
		golemctx.WriteState(dir, state)
	}

	result, err := readProjectState(dir, pipelineState)
	if err != nil {
		return nil, err
	}

	// Detect if agent logged (iteration count incremented)
	lc, _ := result["log-context"].(map[string]any)
	prevIteration, _ := pipelineState["_last_log_iteration"].(int)
	currentIteration, _ := lc["iteration"].(int)
	lc["agent_logged"] = currentIteration > prevIteration

	// Update tracking state
	pipelineState["_last_log_iteration"] = currentIteration
	pipelineState["_head_before"] = gitHead(dir)

	return result, nil
}

// gitHead returns the current HEAD commit hash.
func gitHead(dir string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitDiffStat returns `git diff --stat` between two commits.
func gitDiffStat(dir, from string) string {
	cmd := exec.Command("git", "diff", "--stat", from+"..HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
```

Note: graph-related imports (`graph`, `graphctx`, `embed`) are included in the initial import block but won't be used until Task 6. The Go compiler will complain about unused imports — either add the graph functions in Task 6 to this same file, or temporarily comment out the unused imports and uncomment them in Task 6.

- [ ] **Step 4: Wire into `execBuiltinStep` in `engine.go`**

In `internal/runner/engine.go`, in the `execBuiltinStep` switch statement, add after the `create-pr` case:

```go
	case "init-state":
		result, err = primitiveInitState(ctx, e.cfg.Dir, e.cfg.Config, e.state)
	case "sync-state":
		result, err = primitiveSyncState(ctx, e.cfg.Dir, e.cfg.Config, e.state)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/runner/ -run "TestPrimitiveInitState|TestPrimitiveSyncState" -v`
Expected: PASS

- [ ] **Step 6: Run all tests**

Run: `go test ./internal/runner/ -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/runner/builder_primitives.go internal/runner/builder_primitives_test.go internal/runner/engine.go
git commit -m "feat(runner): add init-state and sync-state builtins for builder blueprint"
```

---

### Task 4: Add `pick-task` Builtin

**Files:**
- Modify: `internal/runner/builder_primitives.go`
- Modify: `internal/runner/builder_primitives_test.go`
- Modify: `internal/runner/engine.go` (add case)

- [ ] **Step 1: Write the failing tests**

```go
// In builder_primitives_test.go
func TestPrimitivePickTask(t *testing.T) {
	tests := []struct {
		name     string
		tasks    []any
		config   map[string]any
		wantName string
		wantErr  bool
	}{
		{
			name: "prefers in-progress over todo",
			tasks: []any{
				map[string]any{"name": "t1", "status": "todo"},
				map[string]any{"name": "t2", "status": "in-progress"},
			},
			wantName: "t2",
		},
		{
			name: "picks first todo when no in-progress",
			tasks: []any{
				map[string]any{"name": "t1", "status": "done"},
				map[string]any{"name": "t2", "status": "todo"},
				map[string]any{"name": "t3", "status": "todo"},
			},
			wantName: "t2",
		},
		{
			name: "respects dependencies",
			tasks: []any{
				map[string]any{"name": "t1", "status": "todo", "depends_on": []string{"t0"}},
				map[string]any{"name": "t2", "status": "todo"},
			},
			wantName: "t2", // t1 blocked by unfinished t0
		},
		{
			name: "dependency satisfied",
			tasks: []any{
				map[string]any{"name": "t0", "status": "done"},
				map[string]any{"name": "t1", "status": "todo", "depends_on": []string{"t0"}},
			},
			wantName: "t1",
		},
		{
			name: "task override from config",
			tasks: []any{
				map[string]any{"name": "t1", "status": "todo"},
				map[string]any{"name": "t2", "status": "todo"},
			},
			config:   map[string]any{"task": "t2"},
			wantName: "t2",
		},
		{
			name: "skips blocked and done",
			tasks: []any{
				map[string]any{"name": "t1", "status": "done"},
				map[string]any{"name": "t2", "status": "blocked"},
				map[string]any{"name": "t3", "status": "todo"},
			},
			wantName: "t3",
		},
		{
			name: "no eligible tasks",
			tasks: []any{
				map[string]any{"name": "t1", "status": "done"},
				map[string]any{"name": "t2", "status": "blocked"},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := map[string]any{"tasks": tt.tasks}
			result, err := primitivePickTask(nil, "", tt.config, state)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			ct, ok := result["current-task"].(map[string]any)
			if !ok {
				t.Fatal("current-task should be a map")
			}
			if ct["name"] != tt.wantName {
				t.Errorf("picked %v, want %v", ct["name"], tt.wantName)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runner/ -run TestPrimitivePickTask -v`
Expected: Compile error — `primitivePickTask` not defined

- [ ] **Step 3: Implement `primitivePickTask`**

Add to `internal/runner/builder_primitives.go`:

```go
// primitivePickTask selects the next task to work on.
// Priority: config override > in-progress > todo (with deps satisfied).
func primitivePickTask(_ context.Context, dir string, config map[string]any, pipelineState map[string]any) (PrimitiveResult, error) {
	tasksRaw, ok := pipelineState["tasks"].([]any)
	if !ok || len(tasksRaw) == 0 {
		return nil, &UnrecoverableError{Msg: "pick-task: no tasks in state"}
	}

	// Check for task override
	if config != nil {
		if override, ok := config["task"].(string); ok && override != "" {
			for _, t := range tasksRaw {
				tm, ok := t.(map[string]any)
				if !ok {
					continue
				}
				if tm["name"] == override {
					return PrimitiveResult{
						"current-task": buildCurrentTask(tm, dir, pipelineState),
					}, nil
				}
			}
			return nil, &UnrecoverableError{Msg: fmt.Sprintf("pick-task: override task %q not found", override)}
		}
	}

	// Build done set for dependency checking
	doneSet := make(map[string]bool)
	for _, t := range tasksRaw {
		tm, _ := t.(map[string]any)
		if tm["status"] == "done" {
			name, _ := tm["name"].(string)
			doneSet[name] = true
		}
	}

	// First pass: prefer in-progress
	for _, t := range tasksRaw {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		if tm["status"] == "in-progress" {
			return PrimitiveResult{
				"current-task": buildCurrentTask(tm, dir, pipelineState),
			}, nil
		}
	}

	// Second pass: first todo with deps satisfied
	for _, t := range tasksRaw {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		if tm["status"] != "todo" {
			continue
		}
		if depsOK(tm, doneSet) {
			return PrimitiveResult{
				"current-task": buildCurrentTask(tm, dir, pipelineState),
			}, nil
		}
	}

	return nil, &UnrecoverableError{Msg: "pick-task: no eligible tasks"}
}

// depsOK checks if all depends_on entries are in the done set.
func depsOK(taskMap map[string]any, doneSet map[string]bool) bool {
	deps, ok := taskMap["depends_on"]
	if !ok {
		return true
	}
	switch d := deps.(type) {
	case []string:
		for _, dep := range d {
			if !doneSet[dep] {
				return false
			}
		}
	case []any:
		for _, dep := range d {
			if s, ok := dep.(string); ok && !doneSet[s] {
				return false
			}
		}
	case string:
		if !doneSet[d] {
			return false
		}
	}
	return true
}

// buildCurrentTask assembles the current-task map with optional doc_hint.
func buildCurrentTask(tm map[string]any, dir string, pipelineState map[string]any) map[string]any {
	ct := map[string]any{
		"name":   tm["name"],
		"status": tm["status"],
	}
	if notes, ok := tm["notes"].(string); ok && notes != "" {
		ct["notes"] = notes
	}

	// Try to find doc_hint
	var docsPath string
	if pc, ok := pipelineState["project-context"].(map[string]any); ok {
		docsPath, _ = pc["docs_path"].(string)
	}
	if docsPath != "" && dir != "" {
		name, _ := tm["name"].(string)
		if hint := findDocSection(dir, docsPath, name); hint != "" {
			ct["doc_hint"] = hint
		}
	}

	return ct
}
```

- [ ] **Step 4: Wire into `execBuiltinStep`**

In `internal/runner/engine.go`, add:

```go
	case "pick-task":
		result, err = primitivePickTask(ctx, e.cfg.Dir, e.cfg.Config, e.state)
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/runner/ -run TestPrimitivePickTask -v`
Expected: PASS (the `findDocSection` function doesn't exist yet, but `buildCurrentTask` handles empty docsPath gracefully — it just won't add `doc_hint`)

- [ ] **Step 6: Commit**

```bash
git add internal/runner/builder_primitives.go internal/runner/builder_primitives_test.go internal/runner/engine.go
git commit -m "feat(runner): add pick-task builtin with dependency-aware selection"
```

---

## Chunk 2: Doc Scanner + build-context + strategy-eval

### Task 5: Add Doc Section Scanner

**Files:**
- Create: `internal/runner/doc_scanner.go`
- Create: `internal/runner/doc_scanner_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/runner/doc_scanner_test.go
package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindDocSection(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0755)

	// Create a doc file with various heading formats
	doc := `# Implementation Plan

## Task 1: Setup Database
Steps for setting up the database...

## Task 2: Add Authentication
Steps for auth...

## 3. Build API Endpoints
Steps for API...

## Task 4: Write Tests
Steps for tests...
`
	os.WriteFile(filepath.Join(docsDir, "impl.md"), []byte(doc), 0644)

	tests := []struct {
		taskName string
		wantHint string
		wantOK   bool
	}{
		{"Setup Database", "docs/impl.md", true},
		{"Add Authentication", "docs/impl.md", true},
		{"Build API Endpoints", "docs/impl.md", true},
		{"Write Tests", "docs/impl.md", true},
		{"Nonexistent Task", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.taskName, func(t *testing.T) {
			hint := findDocSection(dir, "docs/", tt.taskName)
			if tt.wantOK {
				if hint == "" {
					t.Errorf("expected hint for %q, got empty", tt.taskName)
				}
				if !strings.Contains(hint, tt.wantHint) {
					t.Errorf("hint %q should reference %q", hint, tt.wantHint)
				}
			} else {
				if hint != "" {
					t.Errorf("expected no hint for %q, got %q", tt.taskName, hint)
				}
			}
		})
	}
}

func TestFindDocSection_PrefersRecentFile(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0755)

	// Old file
	os.WriteFile(filepath.Join(docsDir, "old.md"), []byte("## Task: Setup\nold content\n"), 0644)

	// Touch to make it older
	// New file with same task name
	os.WriteFile(filepath.Join(docsDir, "new.md"), []byte("## Task: Setup\nnew content\n"), 0644)

	hint := findDocSection(dir, "docs/", "Setup")
	if hint == "" {
		t.Fatal("expected a hint")
	}
	// Should prefer new.md (more recently modified)
	if !strings.Contains(hint, "new.md") {
		t.Errorf("hint = %q, expected reference to new.md", hint)
	}
}

// Note: use strings.Contains in place of a custom helper. Add "strings" to the import block.
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runner/ -run TestFindDocSection -v`
Expected: Compile error — `findDocSection` not defined

- [ ] **Step 3: Implement `doc_scanner.go`**

Create `internal/runner/doc_scanner.go`:

```go
package runner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// findDocSection scans markdown files in docsPath for a section header matching taskName.
// Returns a hint string like "docs/impl.md section '## Task 4: Write Tests'" or "" if not found.
func findDocSection(projectDir, docsPath, taskName string) string {
	absDocsPath := filepath.Join(projectDir, docsPath)
	info, err := os.Stat(absDocsPath)
	if err != nil || !info.IsDir() {
		return ""
	}

	type match struct {
		file    string
		heading string
		modTime int64
	}
	var matches []match

	filepath.Walk(absDocsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "#") {
				continue
			}
			if matchesTaskHeading(line, taskName) {
				relPath, _ := filepath.Rel(projectDir, path)
				matches = append(matches, match{
					file:    relPath,
					heading: strings.TrimSpace(line),
					modTime: info.ModTime().UnixNano(),
				})
				break // one match per file is enough
			}
		}
		return nil
	})

	if len(matches) == 0 {
		return ""
	}

	// Prefer most recently modified file
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].modTime > matches[j].modTime
	})

	return fmt.Sprintf("%s section '%s'", matches[0].file, matches[0].heading)
}

// headingPatterns for matching task names in markdown headings
var (
	// ## Task <N>: <name> or ## Task: <name> or ## Task <name>
	taskPrefixRe = regexp.MustCompile(`(?i)^#{2,3}\s+Task\s*\d*[.:]*\s*`)
	// ## <N>. <name>
	numberedRe = regexp.MustCompile(`(?i)^#{2,3}\s+\d+\.\s*`)
)

// matchesTaskHeading checks if a markdown heading line matches the given task name.
func matchesTaskHeading(line, taskName string) bool {
	taskNameLower := strings.ToLower(taskName)

	// Try: ## Task <name> / ## Task N: <name>
	if loc := taskPrefixRe.FindStringIndex(line); loc != nil {
		remainder := strings.TrimSpace(line[loc[1]:])
		if strings.EqualFold(remainder, taskName) {
			return true
		}
		if strings.Contains(strings.ToLower(remainder), taskNameLower) {
			return true
		}
	}

	// Try: ## N. <name>
	if loc := numberedRe.FindStringIndex(line); loc != nil {
		remainder := strings.TrimSpace(line[loc[1]:])
		if strings.Contains(strings.ToLower(remainder), taskNameLower) {
			return true
		}
	}

	// Fallback: case-insensitive substring match on the heading text (strip ## prefix)
	headingText := strings.TrimLeft(line, "# ")
	headingText = strings.TrimSpace(headingText)
	if strings.Contains(strings.ToLower(headingText), taskNameLower) {
		return true
	}

	return false
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/runner/ -run TestFindDocSection -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runner/doc_scanner.go internal/runner/doc_scanner_test.go
git commit -m "feat(runner): add doc section scanner for pick-task doc_hint"
```

---

### Task 6: Add `build-context` Builtin

**Files:**
- Modify: `internal/runner/builder_primitives.go`
- Modify: `internal/runner/builder_primitives_test.go`
- Modify: `internal/runner/engine.go` (add case)

- [ ] **Step 1: Write the failing tests**

```go
// In builder_primitives_test.go
func TestPrimitiveBuildContext(t *testing.T) {
	state := map[string]any{
		"current-task": map[string]any{
			"name":     "Add Auth",
			"status":   "todo",
			"notes":    "implement JWT auth",
			"doc_hint": "docs/impl.md section '## Task 2'",
		},
		"project-context": map[string]any{
			"decisions": []golemctx.Decision{
				{What: "use JWT", Why: "standard", When: "2026-01-01"},
			},
			"pitfalls": []golemctx.Pitfall{
				{What: "token expiry", Fix: "set 1h TTL"},
			},
			"docs_path": "docs/",
		},
		"log-context": map[string]any{
			"last_task":     "Setup DB",
			"last_outcome":  "done",
			"last_handoff":  "DB ready, move to auth",
			"last_diff_stat": " 3 files changed, 50 insertions(+)",
		},
	}

	result, err := primitiveBuildContext(nil, "", nil, state)
	if err != nil {
		t.Fatalf("primitiveBuildContext: %v", err)
	}

	tc, ok := result["task-context"].(string)
	if !ok || tc == "" {
		t.Fatal("task-context should be a non-empty string")
	}

	// Verify sections present
	checks := []string{
		`## Your Task`,
		`Name: "Add Auth"`,
		`implement JWT auth`,
		`## Documentation`,
		`docs/impl.md section '## Task 2'`,
		`## Handoff from Previous Iteration`,
		`DB ready, move to auth`,
		`## Recent Changes`,
		`3 files changed`,
		`## Project Decisions`,
		`use JWT`,
		`## Known Pitfalls`,
		`token expiry`,
	}
	for _, check := range checks {
		if !strings.Contains(tc, check) {
			t.Errorf("task-context missing %q", check)
		}
	}
}

func TestPrimitiveBuildContext_MinimalState(t *testing.T) {
	// Only required fields, no optional context
	state := map[string]any{
		"current-task": map[string]any{
			"name":   "Do thing",
			"status": "todo",
		},
		"project-context": map[string]any{},
	}

	result, err := primitiveBuildContext(nil, "", nil, state)
	if err != nil {
		t.Fatalf("primitiveBuildContext: %v", err)
	}

	tc := result["task-context"].(string)
	if !strings.Contains(tc, "## Your Task") {
		t.Error("should always contain task section")
	}
	if strings.Contains(tc, "## Documentation") {
		t.Error("should not contain doc section when no doc_hint")
	}
	if strings.Contains(tc, "## Handoff") {
		t.Error("should not contain handoff when no log-context")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runner/ -run TestPrimitiveBuildContext -v`
Expected: Compile error — `primitiveBuildContext` not defined

- [ ] **Step 3: Implement `primitiveBuildContext`**

Add to `internal/runner/builder_primitives.go`:

```go
// primitiveBuildContext assembles the task-context markdown string.
func primitiveBuildContext(_ context.Context, dir string, config map[string]any, pipelineState map[string]any) (PrimitiveResult, error) {
	ct, ok := pipelineState["current-task"].(map[string]any)
	if !ok {
		return nil, &UnrecoverableError{Msg: "build-context: no current-task in state"}
	}
	pc, _ := pipelineState["project-context"].(map[string]any)
	lc, _ := pipelineState["log-context"].(map[string]any)

	var b strings.Builder

	// 1. Task (always)
	name, _ := ct["name"].(string)
	status, _ := ct["status"].(string)
	notes, _ := ct["notes"].(string)
	b.WriteString(fmt.Sprintf("## Your Task\nName: %q\nStatus: %s\n", name, status))
	if notes != "" {
		b.WriteString(fmt.Sprintf("Notes: %s\n", notes))
	}

	// 2. Documentation pointer
	if docHint, ok := ct["doc_hint"].(string); ok && docHint != "" {
		b.WriteString(fmt.Sprintf("\n## Documentation\nRead the implementation details at: %s\nDo NOT read other sections or other doc files — they cover completed work.\n", docHint))
	}

	// 3. Previous iteration handoff
	if lc != nil {
		handoff, _ := lc["last_handoff"].(string)
		if handoff != "" {
			lastTask, _ := lc["last_task"].(string)
			lastOutcome, _ := lc["last_outcome"].(string)
			b.WriteString(fmt.Sprintf("\n## Handoff from Previous Iteration\n%s\n\nLast task: %s — outcome: %s\n", handoff, lastTask, lastOutcome))
		}
	}

	// 4. Recent changes
	if lc != nil {
		diffStat, _ := lc["last_diff_stat"].(string)
		if diffStat != "" {
			b.WriteString(fmt.Sprintf("\n## Recent Changes (last iteration)\n%s\n", diffStat))
		}
	}

	// 5. Decisions & Pitfalls
	if pc != nil {
		if decisions, ok := pc["decisions"].([]golemctx.Decision); ok && len(decisions) > 0 {
			b.WriteString("\n## Project Decisions\n")
			for _, d := range decisions {
				b.WriteString(fmt.Sprintf("- %s — %s\n", d.What, d.Why))
			}
		}
		if pitfalls, ok := pc["pitfalls"].([]golemctx.Pitfall); ok && len(pitfalls) > 0 {
			b.WriteString("\n## Known Pitfalls\n")
			for _, p := range pitfalls {
				b.WriteString(fmt.Sprintf("- %s\n", p.String()))
			}
		}
	}

	// 6. Context map (graph-based) — only if graph exists
	// This will be wired up separately; for now, skip if no graph
	if dir != "" {
		contextMapStr := buildContextMapForTask(dir, name, notes, config)
		if contextMapStr != "" {
			b.WriteString("\n")
			b.WriteString(contextMapStr)
		}
	}

	return PrimitiveResult{
		"task-context": b.String(),
	}, nil
}

// buildContextMapForTask generates a context map string for the given task.
// Returns empty string if graph is not available or has no embeddings.
func buildContextMapForTask(dir, taskName, taskNotes string, config map[string]any) string {
	// Reuse the existing buildContextMapString logic from builder.go
	// but adapted for pipeline state
	graphPath := filepath.Join(dir, ".ctx", "graph.db")
	if _, err := os.Stat(graphPath); err != nil {
		return ""
	}

	taskText := taskName
	if taskNotes != "" {
		taskText += " " + taskNotes
	}
	if taskText == "" {
		return ""
	}

	limit := 15
	if config != nil {
		if l, ok := config["context-map-limit"].(int); ok && l > 0 {
			limit = l
		}
	}

	gStore, err := graph.OpenStore(graphPath)
	if err != nil {
		return ""
	}
	defer gStore.Close()

	eCount, _ := gStore.EmbeddingCount()
	if eCount == 0 {
		return ""
	}

	modelDir, err := embed.EnsureModel(embed.DefaultModelID, embed.DefaultModelDir())
	if err != nil {
		return ""
	}
	embedder, err := embed.NewONNXEmbedder(modelDir)
	if err != nil {
		return ""
	}
	defer embedder.Close()

	cm, err := graphctx.BuildContextMap(gStore, embedder, taskText, limit)
	if err != nil {
		return ""
	}

	formatted := cm.Format()
	if formatted == "" {
		return ""
	}

	// Rewrite the header to match the spec's wording
	formatted = strings.Replace(formatted,
		"## Relevant Context\n\nThe following symbols are relevant to your current task. Start here.\n",
		"## Context Map (pre-loaded)\nThese symbols are relevant to your task — no need to search for them:\n",
		1)
	formatted += "\nGraph tools (find_callers, semantic_search, etc.) are available for deeper exploration.\n"

	return formatted
}
```

Note: all required imports (`graph`, `graphctx`, `embed`, `filepath`, `os`) are already in the import block from Task 3. No new imports needed.

- [ ] **Step 4: Wire into `execBuiltinStep`**

In `internal/runner/engine.go`, add:

```go
	case "build-context":
		result, err = primitiveBuildContext(ctx, e.cfg.Dir, e.cfg.Config, e.state)
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/runner/ -run TestPrimitiveBuildContext -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/runner/builder_primitives.go internal/runner/builder_primitives_test.go internal/runner/engine.go
git commit -m "feat(runner): add build-context builtin for context assembly"
```

---

### Task 7: Add `strategy-eval` Builtin

**Files:**
- Modify: `internal/runner/builder_primitives.go`
- Modify: `internal/runner/builder_primitives_test.go`
- Modify: `internal/runner/engine.go` (add case)

- [ ] **Step 1: Write the failing tests**

```go
// In builder_primitives_test.go
func TestPrimitiveStrategyEval_Continue(t *testing.T) {
	dir := setupTestDir(t)
	state := map[string]any{
		"tasks": []any{
			map[string]any{"name": "t1", "status": "todo"},
		},
		"log-context": map[string]any{
			"last_task":    "t0",
			"last_outcome": "done",
			"iteration":    1,
			"agent_logged": true,
		},
	}

	result, err := primitiveStrategyEval(nil, dir, map[string]any{"max-iterations": 20}, state)
	if err != nil {
		t.Fatalf("strategy-eval: %v", err)
	}
	// On continue, _error_context should be empty
	ec, _ := result["_error_context"].(string)
	if ec != "" {
		t.Errorf("_error_context should be empty on continue, got %q", ec)
	}
}

func TestPrimitiveStrategyEval_MaxIterations(t *testing.T) {
	dir := setupTestDir(t)
	state := map[string]any{
		"tasks": []any{
			map[string]any{"name": "t1", "status": "todo"},
		},
		"log-context": map[string]any{
			"last_task":    "t1",
			"last_outcome": "done",
			"iteration":    20,
			"agent_logged": true,
		},
	}

	result, err := primitiveStrategyEval(nil, dir, map[string]any{"max-iterations": 20}, state)
	if err != nil {
		t.Fatalf("strategy-eval: %v", err)
	}
	// Should set _halt
	if state["_halt"] != true {
		t.Error("expected _halt to be set")
	}
	_ = result
}

func TestPrimitiveStrategyEval_SyntheticLog(t *testing.T) {
	dir := setupTestDir(t)
	gitInit(t, dir)
	state := map[string]any{
		"current-task": map[string]any{"name": "t2"},
		"tasks": []any{
			map[string]any{"name": "t2", "status": "todo"},
		},
		"log-context": map[string]any{
			"last_task":    "t1",
			"last_outcome": "done",
			"iteration":    1,
			"agent_logged": false, // agent didn't log
		},
	}

	_, err := primitiveStrategyEval(nil, dir, map[string]any{"max-iterations": 20}, state)
	if err != nil {
		t.Fatalf("strategy-eval: %v", err)
	}

	// Should have written a synthetic session
	log, _ := golemctx.ReadLog(dir)
	if len(log.Sessions) != 2 { // 1 original + 1 synthetic
		t.Errorf("expected 2 sessions, got %d", len(log.Sessions))
	}
	last := log.Sessions[len(log.Sessions)-1]
	if last.Outcome != "error" {
		t.Errorf("synthetic session outcome = %q, want error", last.Outcome)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runner/ -run TestPrimitiveStrategyEval -v`
Expected: Compile error

- [ ] **Step 3: Implement `primitiveStrategyEval`**

Add to `internal/runner/builder_primitives.go`:

```go
// primitiveStrategyEval evaluates iteration strategy and sets _error_context or _halt.
func primitiveStrategyEval(_ context.Context, dir string, config map[string]any, pipelineState map[string]any) (PrimitiveResult, error) {
	lc, _ := pipelineState["log-context"].(map[string]any)
	agentLogged, _ := lc["agent_logged"].(bool)

	// Write synthetic log session if agent didn't log
	if !agentLogged {
		taskName := "unknown"
		if ct, ok := pipelineState["current-task"].(map[string]any); ok {
			if n, ok := ct["name"].(string); ok {
				taskName = n
			}
		}
		golemctx.AppendSession(dir, golemctx.Session{
			Task:    taskName,
			Outcome: "error",
			Summary: "Agent session completed but did not call log_session MCP tool",
		})
	}

	// Always clear _error_context first
	result := PrimitiveResult{"_error_context": ""}

	// Check max-iterations
	maxIter := 20
	if config != nil {
		if m, ok := config["max-iterations"].(int); ok && m > 0 {
			maxIter = m
		}
	}
	iteration, _ := lc["iteration"].(int)
	if iteration >= maxIter {
		pipelineState["_halt"] = true
		return result, nil
	}

	// Read full log for strategy analysis
	log, err := golemctx.ReadLog(dir)
	if err != nil || len(log.Sessions) == 0 {
		return result, nil
	}

	last := log.Sessions[len(log.Sessions)-1]

	// Thrashing: same task 3+ consecutive times
	if len(log.Sessions) >= 3 {
		last3 := log.Sessions[len(log.Sessions)-3:]
		task := last3[0].Task
		if task != "" && last3[1].Task == task && last3[2].Task == task {
			// Mark task as blocked
			markTaskBlocked(dir, task, "auto-skipped: attempted 3 consecutive iterations without completion")
			result["_error_context"] = fmt.Sprintf("## Strategy Override\nTask %q has been attempted for 3 consecutive iterations without completion. It has been skipped. Work on a different task.\n", task)
			return result, nil
		}
	}

	// Repeated failure: task failed/blocked 2+ times
	if isFailedOutcome(last.Outcome) && last.Task != "" {
		failCount := 0
		for i := len(log.Sessions) - 1; i >= 0 && log.Sessions[i].Task == last.Task; i-- {
			if isFailedOutcome(log.Sessions[i].Outcome) {
				failCount++
			}
		}
		if failCount >= 2 {
			markTaskBlocked(dir, last.Task, fmt.Sprintf("auto-skipped: failed %d times", failCount))
			result["_error_context"] = fmt.Sprintf("## Strategy Override\nTask %q has failed %d times and has been skipped. Work on a different task.\n", last.Task, failCount)
			return result, nil
		}
		if failCount == 1 {
			summary := last.Summary
			if summary == "" {
				summary = last.Outcome
			}
			result["_error_context"] = fmt.Sprintf("## Previous Iteration Context\nThe previous iteration attempted task %q but did not complete it. Outcome: %s.\n\nSummary: %s\n\nTry a different approach.\n", last.Task, last.Outcome, summary)
			return result, nil
		}
	}

	// Unproductive streak
	unproductiveCount := 0
	for i := len(log.Sessions) - 1; i >= 0; i-- {
		if log.Sessions[i].Outcome == "unproductive" {
			unproductiveCount++
		} else {
			break
		}
	}
	if unproductiveCount >= 3 {
		pipelineState["_halt"] = true
		return result, nil
	}
	if unproductiveCount >= 2 {
		result["_error_context"] = fmt.Sprintf("## Warning\nThe last %d iterations produced no meaningful progress. Focus on making concrete, testable changes. If you are stuck, consider working on a different task.\n", unproductiveCount)
		return result, nil
	}

	// Deadlock: all remaining tasks blocked or depend on blocked
	allBlocked := true
	tasksRaw, _ := pipelineState["tasks"].([]any)
	doneSet := make(map[string]bool)
	for _, t := range tasksRaw {
		tm, _ := t.(map[string]any)
		if tm["status"] == "done" {
			name, _ := tm["name"].(string)
			doneSet[name] = true
		}
	}
	for _, t := range tasksRaw {
		tm, _ := t.(map[string]any)
		status, _ := tm["status"].(string)
		if status == "done" || status == "blocked" {
			continue
		}
		if depsOK(tm, doneSet) {
			allBlocked = false
			break
		}
	}
	if allBlocked && len(tasksRaw) > 0 {
		hasRemaining := false
		for _, t := range tasksRaw {
			tm, _ := t.(map[string]any)
			if tm["status"] != "done" {
				hasRemaining = true
				break
			}
		}
		if hasRemaining {
			pipelineState["_halt"] = true
		}
	}

	return result, nil
}

// markTaskBlocked sets a task to blocked status in state.yaml.
func markTaskBlocked(dir, taskName, reason string) {
	state, err := golemctx.ReadState(dir)
	if err != nil {
		return
	}
	for i := range state.Tasks {
		if state.Tasks[i].Name == taskName && state.Tasks[i].Status != "done" {
			state.Tasks[i].Status = "blocked"
			state.Tasks[i].BlockedReason = reason
		}
	}
	golemctx.WriteState(dir, state)
}
```

- [ ] **Step 4: Wire into `execBuiltinStep`**

In `internal/runner/engine.go`, add:

```go
	case "strategy-eval":
		result, err = primitiveStrategyEval(ctx, e.cfg.Dir, e.cfg.Config, e.state)
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/runner/ -run TestPrimitiveStrategyEval -v`
Expected: PASS

- [ ] **Step 6: Run all tests**

Run: `go test ./internal/runner/ -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/runner/builder_primitives.go internal/runner/builder_primitives_test.go internal/runner/engine.go
git commit -m "feat(runner): add strategy-eval builtin with thrashing and deadlock detection"
```

---

## Chunk 3: Builder Agent YAML + Wiring

### Task 8: Create `builder.yaml` Agent

**Files:**
- Create: `templates/agents/builder.yaml`

- [ ] **Step 1: Create the builder agent YAML**

Create `templates/agents/builder.yaml`:

```yaml
name: builder
description: "Multi-task iteration loop with context assembly and strategy evaluation."
initial-state: [goal]

config:
  max-iterations: 20
  lint-cmd: null
  lint-fix-cmd: null
  test-cmd: null

steps:
  - git-setup:
      type: builtin

  - init-state:
      type: builtin
      writes: [project-context, tasks, log-context]

  - while:
      predicate: tasks-remaining
      max: 30
      steps:
        - pick-task:
            type: builtin
            reads: [tasks]
            writes: [current-task]

        - build-context:
            type: builtin
            reads: [current-task, project-context]
            optional-reads: [log-context]
            writes: [task-context]

        - implement:
            type: agentic
            reads: [goal, task-context]
            optional-reads: [_error_context]
            writes: [code]
            prompt: |
              You are working on a software project autonomously. Each iteration you work on ONE task.
              You have no memory of previous iterations — all context is provided below.

              # Goal
              ${goal}

              # Context
              ${task-context}

              # Instructions
              1. If a documentation pointer is provided above, read that section for implementation details.
              2. Use graph tools (find_callers, semantic_search, etc.) if you need to trace code beyond what the context map provides.
              3. Implement the task. Write or update tests. Make sure they pass.
              4. Commit your work with clear commit messages.

              # End of Session
              Use the golem MCP tools to update state:
              1. Call `mark_task` to update your task (set status and notes).
              2. Call `add_decision` for any new architectural decisions.
              3. Call `add_pitfall` for any lessons learned.
              4. Call `set_status` to update current_focus.
              5. Call `log_session` with task name, outcome, summary, files_changed, and a handoff note.
                 The handoff should be specific: what you did, where you left off, what to do next, and any gotchas.
                 Include file paths and line numbers when relevant.

              ## Previous Error Context
              ${_error_context}

        - run-tests:
            type: builtin
            reads: [code]
            writes: [test-results]

        - sync-state:
            type: builtin
            writes: [project-context, tasks, log-context]

        - strategy-eval:
            type: builtin
            reads: [tasks, log-context]
            optional-reads: [current-task]
            writes: [_error_context]

errors:
  transient: { action: retry, max: 3 }
  malformed-output: { action: re-run, max: 2, hint: "Your task updates should be made via the golem MCP tools." }
  contract-violation: { action: halt }
```

- [ ] **Step 2: Verify it parses correctly**

Write a quick parse test (or use an existing test pattern):

Run: `go test ./internal/runner/ -run TestParseBlueprint -v` (if existing parse tests exist, just add builder.yaml to them)

If no existing test covers this, add to an appropriate test file:

```go
func TestParseBuilderBlueprint(t *testing.T) {
	data, err := templates.FS.ReadFile("agents/builder.yaml")
	if err != nil {
		t.Fatalf("reading builder.yaml: %v", err)
	}
	bp, err := ParseBlueprint(data)
	if err != nil {
		t.Fatalf("parsing builder.yaml: %v", err)
	}
	if err := bp.ValidateContracts(); err != nil {
		t.Fatalf("contract validation: %v", err)
	}
	if bp.Name != "builder" {
		t.Errorf("name = %q, want builder", bp.Name)
	}
}
```

- [ ] **Step 3: Run test**

Run: `go test ./internal/runner/ -run TestParseBuilderBlueprint -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add templates/agents/builder.yaml internal/runner/*_test.go
git commit -m "feat(runner): add builder.yaml blueprint agent"
```

---

### Task 9: Wire `builder` as Default Blueprint Agent

**Files:**
- Modify: `cmd/code.go`

- [ ] **Step 1: Read current `cmd/code.go` to understand the wiring**

Already read above. The `rc.Agent` field controls which agent is loaded. The config's `agent` key defaults to `"build-feature"` (check `internal/config/config.go`).

- [ ] **Step 2: No code change needed for wiring**

The `builder` agent is already loadable via `golem run builder --goal "..."`. For `golem code` with `engine: blueprint`, it uses `rc.Agent` which comes from config. Users set `agent: builder` in their config.

No forced default change for now — per the migration path, this is opt-in first.

- [ ] **Step 3: Verify end-to-end with build**

Run: `go build ./...`
Expected: Compiles successfully

- [ ] **Step 4: Commit (if any changes were needed)**

```bash
git commit --allow-empty -m "docs: builder agent is opt-in via config agent: builder"
```

---

### Task 10: Integration Test

**Files:**
- Create: `internal/runner/builder_integration_test.go`

- [ ] **Step 1: Write an integration test with mock runner**

```go
// internal/runner/builder_integration_test.go
package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	golemctx "github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/templates"
)

// mockBuilderRunner simulates a Claude session that calls MCP tools.
// It marks the current task as done and logs a session.
type mockBuilderRunner struct {
	callCount int
}

func (m *mockBuilderRunner) Run(ctx context.Context, dir, prompt string, maxTurns int, model string) (string, error) {
	return m.RunWithTools(ctx, dir, prompt, maxTurns, model, nil)
}

func (m *mockBuilderRunner) RunWithTools(ctx context.Context, dir, prompt string, maxTurns int, model string, tools []string) (string, error) {
	m.callCount++

	// Read state to find in-progress/todo task
	state, _ := golemctx.ReadState(dir)
	for i, t := range state.Tasks {
		if t.Status == "todo" || t.Status == "in-progress" {
			state.Tasks[i].Status = "done"
			state.Tasks[i].Notes = "completed by mock"

			golemctx.WriteState(dir, state)
			golemctx.AppendSession(dir, golemctx.Session{
				Iteration: m.callCount,
				Task:      t.Name,
				Outcome:   "done",
				Summary:   "Mock completed " + t.Name,
				Handoff:   "Move to next task",
			})
			break
		}
	}

	// Write session-output.json (implement step expects this but we write code key)
	os.WriteFile(filepath.Join(dir, "session-output.json"), []byte(`{}`), 0644)

	return "mock output", nil
}

func TestBuilderBlueprintIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	dir := t.TempDir()
	ctxDir := filepath.Join(dir, ".ctx")
	os.MkdirAll(ctxDir, 0755)

	// Set up state with 2 tasks
	state := golemctx.State{
		Project: golemctx.Project{Name: "test", DocsPath: "docs/"},
		Status:  golemctx.Status{Phase: "building"},
		Tasks: []golemctx.Task{
			{Name: "task-1", Status: "todo", Notes: "first task"},
			{Name: "task-2", Status: "todo", Notes: "second task"},
		},
	}
	golemctx.WriteState(dir, state)
	golemctx.WriteLog(dir, golemctx.Log{})

	// Init git
	gitInit(t, dir)

	// Load and parse builder blueprint
	data, err := templates.FS.ReadFile("agents/builder.yaml")
	if err != nil {
		t.Fatalf("reading builder.yaml: %v", err)
	}
	bp, err := ParseBlueprint(data)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	mock := &mockBuilderRunner{}
	e := NewEngine(EngineConfig{
		Dir:       dir,
		AgentName: "builder",
		Goal:      "complete all tasks",
		Blueprint: bp,
		Config:    bp.Config,
		Runner:    mock,
		Model:     "test",
	})

	result, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("engine run: %v", err)
	}

	// Verify both tasks were completed
	finalState, _ := golemctx.ReadState(dir)
	for _, task := range finalState.Tasks {
		if task.Status != "done" {
			t.Errorf("task %q status = %s, want done", task.Name, task.Status)
		}
	}

	// Verify mock was called (at least 2 times for 2 tasks)
	if mock.callCount < 2 {
		t.Errorf("mock called %d times, want >= 2", mock.callCount)
	}

	_ = result
}
```

- [ ] **Step 2: Run integration test**

Run: `go test ./internal/runner/ -run TestBuilderBlueprintIntegration -v`
Expected: PASS

- [ ] **Step 3: Run full test suite**

Run: `go test ./... -v`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add internal/runner/builder_integration_test.go
git commit -m "test(runner): add builder blueprint integration test"
```

---

### Known Gaps (Deferred)

- **Token budget truncation**: The spec calls for estimating token count of `task-context` and truncating lower-priority sections when exceeding 4000 tokens. This is deferred to a follow-up. In practice, the assembled context is small (~1300 tokens for TROGUE-scale projects).
- **`isFailedOutcome` dependency**: `strategy-eval` calls `isFailedOutcome()` from `strategy.go`. If the legacy builder is ever removed, this function must be moved to `builder_primitives.go` or a shared file.

---

### Task 11: Final Verification

- [ ] **Step 1: Build the binary**

Run: `go build ./...`
Expected: Compiles with no errors

- [ ] **Step 2: Run all tests**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 3: Verify `golem agents` lists builder**

Run: `go run . agents`
Expected: Output includes `builder` in the agent list

- [ ] **Step 4: Commit any remaining changes**

```bash
git add -A
git commit -m "feat: blueprint builder agent — complete implementation"
```
