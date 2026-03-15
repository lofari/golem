package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		Project: golemctx.Project{Name: "test-project", DocsPath: "docs/"},
		Status:  golemctx.Status{Phase: "building", CurrentFocus: "testing"},
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
				Handoff:   "task-1 complete, move to task-2",
				Timestamp: "2026-01-01T00:00:00Z",
			},
		},
	}
	golemctx.WriteLog(dir, log)

	return dir
}

func gitInitTestRepo(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "test"},
		{"git", "add", "."},
		{"git", "commit", "-m", "init"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
}

func TestPrimitiveInitState(t *testing.T) {
	dir := setupTestDir(t)
	gitInitTestRepo(t, dir)

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
	if pc["phase"] != "building" {
		t.Errorf("phase = %v, want building", pc["phase"])
	}
	if pc["docs_path"] != "docs/" {
		t.Errorf("docs_path = %v, want docs/", pc["docs_path"])
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
	if lc["iteration"] != 1 {
		t.Errorf("iteration = %v, want 1", lc["iteration"])
	}

	// Check snapshot was saved
	snapshots, _ := filepath.Glob(filepath.Join(dir, ".ctx", "snapshots", "state-*.yaml"))
	if len(snapshots) == 0 {
		t.Error("expected a snapshot to be saved")
	}

	// Check internal state
	if pipelineState["_head_before"] == nil || pipelineState["_head_before"] == "" {
		t.Error("_head_before should be set")
	}
	if pipelineState["_sync_initialized"] != true {
		t.Error("_sync_initialized should be true")
	}
	if pipelineState["_last_log_iteration"] != 1 {
		t.Errorf("_last_log_iteration = %v, want 1", pipelineState["_last_log_iteration"])
	}
}

func TestPrimitiveSyncState_AgentNotLogged(t *testing.T) {
	dir := setupTestDir(t)
	gitInitTestRepo(t, dir)

	pipelineState := map[string]any{
		"_sync_initialized":   true,
		"_head_before":        "abc123",
		"_last_log_iteration": 1,
	}

	result, err := primitiveSyncState(nil, dir, nil, pipelineState)
	if err != nil {
		t.Fatalf("primitiveSyncState: %v", err)
	}

	lc, ok := result["log-context"].(map[string]any)
	if !ok {
		t.Fatal("log-context should be a map")
	}
	// iteration count is still 1 (same as _last_log_iteration), so agent_logged = false
	if lc["agent_logged"] != false {
		t.Errorf("agent_logged = %v, want false", lc["agent_logged"])
	}
}

func TestPrimitiveSyncState_AgentLogged(t *testing.T) {
	dir := setupTestDir(t)
	gitInitTestRepo(t, dir)

	// Simulate agent writing a log session
	golemctx.AppendSession(dir, golemctx.Session{
		Iteration: 2, Task: "task-2", Outcome: "done",
		Timestamp: time.Now().Format(time.RFC3339),
	})

	pipelineState := map[string]any{
		"_sync_initialized":   true,
		"_head_before":        "abc123",
		"_last_log_iteration": 1,
	}

	result, err := primitiveSyncState(nil, dir, nil, pipelineState)
	if err != nil {
		t.Fatalf("primitiveSyncState: %v", err)
	}

	lc := result["log-context"].(map[string]any)
	if lc["agent_logged"] != true {
		t.Errorf("agent_logged = %v, want true", lc["agent_logged"])
	}
	// _last_log_iteration should be updated to 2
	if pipelineState["_last_log_iteration"] != 2 {
		t.Errorf("_last_log_iteration = %v, want 2", pipelineState["_last_log_iteration"])
	}
}

func TestPrimitiveSyncState_AutoRepairInvalidPhase(t *testing.T) {
	dir := setupTestDir(t)

	// Write state with invalid phase
	state, _ := golemctx.ReadState(dir)
	state.Status.Phase = "invalid-phase"
	golemctx.WriteState(dir, state)

	gitInitTestRepo(t, dir)

	pipelineState := map[string]any{
		"_sync_initialized":   true,
		"_head_before":        "",
		"_last_log_iteration": 1,
	}

	result, err := primitiveSyncState(nil, dir, nil, pipelineState)
	if err != nil {
		t.Fatalf("primitiveSyncState: %v", err)
	}

	pc := result["project-context"].(map[string]any)
	if pc["phase"] != "building" {
		t.Errorf("phase should be auto-repaired to 'building', got %v", pc["phase"])
	}
}

func TestPrimitiveSyncState_AutoRepairInvalidTaskStatus(t *testing.T) {
	dir := setupTestDir(t)

	// Write state with invalid task status
	state, _ := golemctx.ReadState(dir)
	state.Tasks[1].Status = "bogus"
	golemctx.WriteState(dir, state)

	gitInitTestRepo(t, dir)

	pipelineState := map[string]any{
		"_sync_initialized":   true,
		"_head_before":        "",
		"_last_log_iteration": 1,
	}

	result, err := primitiveSyncState(nil, dir, nil, pipelineState)
	if err != nil {
		t.Fatalf("primitiveSyncState: %v", err)
	}

	tasks := result["tasks"].([]any)
	task2 := tasks[1].(map[string]any)
	if task2["status"] != "todo" {
		t.Errorf("task-2 status should be auto-repaired to 'todo', got %v", task2["status"])
	}
}

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
			wantName: "t2",
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

func TestPrimitiveSyncState_AutoRepairBlockedWithoutReason(t *testing.T) {
	dir := setupTestDir(t)

	// Write state with blocked task but no reason
	state, _ := golemctx.ReadState(dir)
	state.Tasks[1].Status = "blocked"
	state.Tasks[1].BlockedReason = ""
	golemctx.WriteState(dir, state)

	gitInitTestRepo(t, dir)

	pipelineState := map[string]any{
		"_sync_initialized":   true,
		"_head_before":        "",
		"_last_log_iteration": 1,
	}

	result, err := primitiveSyncState(nil, dir, nil, pipelineState)
	if err != nil {
		t.Fatalf("primitiveSyncState: %v", err)
	}

	tasks := result["tasks"].([]any)
	task2 := tasks[1].(map[string]any)
	if task2["blocked_reason"] != "no reason provided by agent" {
		t.Errorf("blocked_reason should be auto-filled, got %v", task2["blocked_reason"])
	}
}

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
			"last_task":      "Setup DB",
			"last_outcome":   "done",
			"last_handoff":   "DB ready, move to auth",
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

	checks := []string{
		"## Your Task",
		`Name: "Add Auth"`,
		"implement JWT auth",
		"## Documentation",
		"docs/impl.md section '## Task 2'",
		"## Handoff from Previous Iteration",
		"DB ready, move to auth",
		"## Recent Changes",
		"3 files changed",
		"## Project Decisions",
		"use JWT",
		"## Known Pitfalls",
		"token expiry",
	}
	for _, check := range checks {
		if !strings.Contains(tc, check) {
			t.Errorf("task-context missing %q", check)
		}
	}
}

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
	ec, _ := result["_error_context"].(string)
	if ec != "" {
		t.Errorf("_error_context should be empty on continue, got %q", ec)
	}
	if state["_halt"] != nil {
		t.Error("_halt should not be set on continue")
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

	_, err := primitiveStrategyEval(nil, dir, map[string]any{"max-iterations": 20}, state)
	if err != nil {
		t.Fatalf("strategy-eval: %v", err)
	}
	if state["_halt"] != true {
		t.Error("expected _halt to be set when max iterations reached")
	}
}

func TestPrimitiveStrategyEval_SyntheticLog(t *testing.T) {
	dir := setupTestDir(t)
	gitInitTestRepo(t, dir)
	state := map[string]any{
		"current-task": map[string]any{"name": "t2"},
		"tasks": []any{
			map[string]any{"name": "t2", "status": "todo"},
		},
		"log-context": map[string]any{
			"last_task":    "t1",
			"last_outcome": "done",
			"iteration":    1,
			"agent_logged": false,
		},
	}

	_, err := primitiveStrategyEval(nil, dir, map[string]any{"max-iterations": 20}, state)
	if err != nil {
		t.Fatalf("strategy-eval: %v", err)
	}

	log, _ := golemctx.ReadLog(dir)
	if len(log.Sessions) != 2 {
		t.Errorf("expected 2 sessions (1 original + 1 synthetic), got %d", len(log.Sessions))
	}
	if len(log.Sessions) >= 2 {
		last := log.Sessions[len(log.Sessions)-1]
		if last.Outcome != "error" {
			t.Errorf("synthetic session outcome = %q, want error", last.Outcome)
		}
	}
}

func TestPrimitiveBuildContext_MinimalState(t *testing.T) {
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
