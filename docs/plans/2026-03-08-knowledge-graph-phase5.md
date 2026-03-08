# Phase 5: Execution Graph Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Capture shell commands, test results, and errors from Claude agent sessions and link them to the existing static knowledge graph.

**Architecture:** Parse stream-json output from Claude CLI to extract bash tool-use events. Store execution data in dedicated SQLite tables alongside the existing graph. Extract file references and test results from command output to create cross-reference edges to existing graph nodes.

**Tech Stack:** Go, SQLite, stream-json parsing, regex-based reference extraction

---

### Task 1: Execution Data Model

Add execution-related types to the graph model package.

**Files:**
- Modify: `internal/graph/model/model.go`
- Test: `internal/graph/model/model_test.go`

**Step 1: Write the test**

Create `internal/graph/model/model_test.go`:

```go
package model

import "testing"

func TestExecutionModel(t *testing.T) {
	exec := Execution{
		SessionID: "session-001",
		StartedAt: 1709900000,
		Status:    "running",
	}
	if exec.SessionID != "session-001" {
		t.Fatalf("unexpected session ID: %s", exec.SessionID)
	}

	cmd := Command{
		ID:        "cmd:session-001:1",
		SessionID: "session-001",
		Seq:       1,
		Command:   "go test ./...",
		ExitCode:  0,
	}
	if cmd.Seq != 1 {
		t.Fatalf("unexpected seq: %d", cmd.Seq)
	}

	tr := TestResult{
		ID:        "test:session-001:TestFoo",
		SessionID: "session-001",
		Name:      "TestFoo",
		Passed:    true,
		DurationMs: 42,
	}
	if !tr.Passed {
		t.Fatal("expected test to pass")
	}

	er := Error{
		ID:         "err:session-001:1",
		CommandID:  "cmd:session-001:1",
		Message:    "exit status 1",
		StackTrace: "goroutine 1 [running]:\nmain.go:42",
	}
	if er.Message != "exit status 1" {
		t.Fatalf("unexpected message: %s", er.Message)
	}

	out := Output{
		CommandID: "cmd:session-001:1",
		Stdout:    "PASS",
		Stderr:    "",
		Truncated: false,
	}
	if out.Truncated {
		t.Fatal("expected not truncated")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/model/ -run TestExecutionModel -v`
Expected: FAIL — `Execution`, `Command`, etc. are undefined.

**Step 3: Add the types to model.go**

Append to `internal/graph/model/model.go`:

```go
// Execution represents an agent session (golem code/review/qa run).
type Execution struct {
	SessionID string `json:"sessionId"`
	StartedAt int64  `json:"startedAt"`
	EndedAt   int64  `json:"endedAt,omitempty"`
	Status    string `json:"status"` // running, completed, failed
}

// Command represents a shell command executed during a session.
type Command struct {
	ID         string `json:"id"`        // cmd:{sessionId}:{seq}
	SessionID  string `json:"sessionId"`
	Seq        int    `json:"seq"`
	Command    string `json:"command"`
	ExitCode   int    `json:"exitCode"`
	WorkingDir string `json:"workingDir,omitempty"`
}

// Output holds stdout/stderr for a command.
type Output struct {
	CommandID string `json:"commandId"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
	Truncated bool   `json:"truncated"`
}

// TestResult represents a parsed test outcome.
type TestResult struct {
	ID         string `json:"id"`        // test:{sessionId}:{name}
	SessionID  string `json:"sessionId"`
	Name       string `json:"name"`
	Passed     bool   `json:"passed"`
	DurationMs int    `json:"durationMs,omitempty"`
	Output     string `json:"output,omitempty"`
}

// Error represents a runtime error extracted from command output.
type Error struct {
	ID         string `json:"id"`        // err:{sessionId}:{seq}
	CommandID  string `json:"commandId"`
	Message    string `json:"message"`
	StackTrace string `json:"stackTrace,omitempty"`
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/graph/model/ -run TestExecutionModel -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/graph/model/model.go internal/graph/model/model_test.go
git commit -m "feat(graph): add execution data model types"
```

---

### Task 2: Execution Store Schema & CRUD

Add execution tables to the SQLite store and CRUD methods.

**Files:**
- Modify: `internal/graph/store.go`
- Create: `internal/graph/store_execution.go`
- Create: `internal/graph/store_execution_test.go`

**Step 1: Write the test**

Create `internal/graph/store_execution_test.go`:

```go
package graph

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lofari/golem/internal/graph/model"
)

func TestExecutionStore_InsertAndQuery(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Insert execution
	exec := model.Execution{
		SessionID: "s1",
		StartedAt: 1000,
		Status:    "running",
	}
	if err := s.InsertExecution(exec); err != nil {
		t.Fatal(err)
	}

	// Query executions
	execs, err := s.QueryExecutions(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(execs) != 1 || execs[0].SessionID != "s1" {
		t.Fatalf("expected 1 execution, got %d", len(execs))
	}

	// Insert command
	cmd := model.Command{
		ID:        "cmd:s1:1",
		SessionID: "s1",
		Seq:       1,
		Command:   "go test ./...",
		ExitCode:  0,
	}
	if err := s.InsertCommand(cmd); err != nil {
		t.Fatal(err)
	}

	// Insert output
	out := model.Output{
		CommandID: "cmd:s1:1",
		Stdout:    "PASS",
		Stderr:    "",
		Truncated: false,
	}
	if err := s.InsertOutput(out); err != nil {
		t.Fatal(err)
	}

	// Insert test result
	tr := model.TestResult{
		ID:         "test:s1:TestFoo",
		SessionID:  "s1",
		Name:       "TestFoo",
		Passed:     true,
		DurationMs: 42,
	}
	if err := s.InsertTestResult(tr); err != nil {
		t.Fatal(err)
	}

	// Insert error
	er := model.Error{
		ID:        "err:s1:1",
		CommandID: "cmd:s1:1",
		Message:   "exit 1",
	}
	if err := s.InsertError(er); err != nil {
		t.Fatal(err)
	}

	// Query commands by session
	cmds, err := s.QueryCommandsBySession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}

	// Query failures
	failures, err := s.QueryFailedCommands("s1")
	if err != nil {
		t.Fatal(err)
	}
	// cmd has exit_code 0, so no failures
	if len(failures) != 0 {
		t.Fatalf("expected 0 failures, got %d", len(failures))
	}

	// Query test results
	tests, err := s.QueryTestResults("s1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tests) != 1 || tests[0].Name != "TestFoo" {
		t.Fatalf("expected TestFoo, got %v", tests)
	}

	// Finalize execution
	if err := s.FinalizeExecution("s1", 2000, "completed"); err != nil {
		t.Fatal(err)
	}
	execs, _ = s.QueryExecutions(10)
	if execs[0].Status != "completed" {
		t.Fatalf("expected completed, got %s", execs[0].Status)
	}
}

func TestExecutionStore_LatestSession(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// No sessions yet
	latest, err := s.LatestExecution()
	if err != nil {
		t.Fatal(err)
	}
	if latest != nil {
		t.Fatal("expected nil for empty db")
	}

	// Add two sessions
	s.InsertExecution(model.Execution{SessionID: "s1", StartedAt: 1000, Status: "completed"})
	s.InsertExecution(model.Execution{SessionID: "s2", StartedAt: 2000, Status: "completed"})

	latest, err = s.LatestExecution()
	if err != nil {
		t.Fatal(err)
	}
	if latest.SessionID != "s2" {
		t.Fatalf("expected s2, got %s", latest.SessionID)
	}
}

func TestExecutionStore_ExecutionCount(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	count, _ := s.ExecutionCount()
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}

	s.InsertExecution(model.Execution{SessionID: "s1", StartedAt: 1000, Status: "completed"})
	count, _ = s.ExecutionCount()
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/ -run TestExecutionStore -v`
Expected: FAIL — methods not defined.

**Step 3: Add schema migration to store.go**

In `internal/graph/store.go`, add to the `init()` method's schema string, after the `vec_embeddings` table:

```sql
CREATE TABLE IF NOT EXISTS executions (
    session_id TEXT PRIMARY KEY,
    started_at INTEGER NOT NULL,
    ended_at   INTEGER,
    status     TEXT NOT NULL DEFAULT 'running'
);
CREATE TABLE IF NOT EXISTS commands (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL,
    seq         INTEGER NOT NULL,
    command     TEXT NOT NULL,
    exit_code   INTEGER,
    working_dir TEXT,
    FOREIGN KEY (session_id) REFERENCES executions(session_id)
);
CREATE TABLE IF NOT EXISTS outputs (
    command_id TEXT PRIMARY KEY,
    stdout     TEXT,
    stderr     TEXT,
    truncated  BOOLEAN DEFAULT FALSE,
    FOREIGN KEY (command_id) REFERENCES commands(id)
);
CREATE TABLE IF NOT EXISTS test_results (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL,
    name        TEXT NOT NULL,
    passed      BOOLEAN NOT NULL,
    duration_ms INTEGER,
    output      TEXT,
    FOREIGN KEY (session_id) REFERENCES executions(session_id)
);
CREATE TABLE IF NOT EXISTS errors (
    id          TEXT PRIMARY KEY,
    command_id  TEXT NOT NULL,
    message     TEXT NOT NULL,
    stack_trace TEXT,
    FOREIGN KEY (command_id) REFERENCES commands(id)
);
CREATE INDEX IF NOT EXISTS idx_commands_session ON commands(session_id);
CREATE INDEX IF NOT EXISTS idx_test_results_session ON test_results(session_id);
CREATE INDEX IF NOT EXISTS idx_errors_command ON errors(command_id);
```

**Step 4: Create store_execution.go with CRUD methods**

Create `internal/graph/store_execution.go`:

```go
package graph

import (
	"database/sql"

	"github.com/lofari/golem/internal/graph/model"
)

// InsertExecution inserts an execution record.
func (s *Store) InsertExecution(exec model.Execution) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO executions (session_id, started_at, ended_at, status) VALUES (?, ?, ?, ?)",
		exec.SessionID, exec.StartedAt, exec.EndedAt, exec.Status,
	)
	return err
}

// FinalizeExecution updates an execution's end time and status.
func (s *Store) FinalizeExecution(sessionID string, endedAt int64, status string) error {
	_, err := s.db.Exec(
		"UPDATE executions SET ended_at = ?, status = ? WHERE session_id = ?",
		endedAt, status, sessionID,
	)
	return err
}

// QueryExecutions returns the most recent executions, ordered by start time descending.
func (s *Store) QueryExecutions(limit int) ([]model.Execution, error) {
	rows, err := s.db.Query(
		"SELECT session_id, started_at, COALESCE(ended_at, 0), status FROM executions ORDER BY started_at DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var execs []model.Execution
	for rows.Next() {
		var e model.Execution
		if err := rows.Scan(&e.SessionID, &e.StartedAt, &e.EndedAt, &e.Status); err != nil {
			return nil, err
		}
		execs = append(execs, e)
	}
	return execs, rows.Err()
}

// LatestExecution returns the most recent execution, or nil if none exist.
func (s *Store) LatestExecution() (*model.Execution, error) {
	execs, err := s.QueryExecutions(1)
	if err != nil {
		return nil, err
	}
	if len(execs) == 0 {
		return nil, nil
	}
	return &execs[0], nil
}

// ExecutionCount returns the number of stored executions.
func (s *Store) ExecutionCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM executions").Scan(&count)
	return count, err
}

// InsertCommand inserts a command record.
func (s *Store) InsertCommand(cmd model.Command) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO commands (id, session_id, seq, command, exit_code, working_dir) VALUES (?, ?, ?, ?, ?, ?)",
		cmd.ID, cmd.SessionID, cmd.Seq, cmd.Command, cmd.ExitCode, cmd.WorkingDir,
	)
	return err
}

// QueryCommandsBySession returns all commands for a session, ordered by sequence.
func (s *Store) QueryCommandsBySession(sessionID string) ([]model.Command, error) {
	rows, err := s.db.Query(
		"SELECT id, session_id, seq, command, COALESCE(exit_code, 0), COALESCE(working_dir, '') FROM commands WHERE session_id = ? ORDER BY seq",
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cmds []model.Command
	for rows.Next() {
		var c model.Command
		if err := rows.Scan(&c.ID, &c.SessionID, &c.Seq, &c.Command, &c.ExitCode, &c.WorkingDir); err != nil {
			return nil, err
		}
		cmds = append(cmds, c)
	}
	return cmds, rows.Err()
}

// QueryFailedCommands returns commands with non-zero exit codes for a session.
func (s *Store) QueryFailedCommands(sessionID string) ([]model.Command, error) {
	rows, err := s.db.Query(
		"SELECT id, session_id, seq, command, exit_code, COALESCE(working_dir, '') FROM commands WHERE session_id = ? AND exit_code != 0 ORDER BY seq",
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cmds []model.Command
	for rows.Next() {
		var c model.Command
		if err := rows.Scan(&c.ID, &c.SessionID, &c.Seq, &c.Command, &c.ExitCode, &c.WorkingDir); err != nil {
			return nil, err
		}
		cmds = append(cmds, c)
	}
	return cmds, rows.Err()
}

// InsertOutput inserts a command output record.
func (s *Store) InsertOutput(out model.Output) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO outputs (command_id, stdout, stderr, truncated) VALUES (?, ?, ?, ?)",
		out.CommandID, out.Stdout, out.Stderr, out.Truncated,
	)
	return err
}

// QueryOutput returns the output for a given command ID.
func (s *Store) QueryOutput(commandID string) (*model.Output, error) {
	var o model.Output
	err := s.db.QueryRow(
		"SELECT command_id, COALESCE(stdout, ''), COALESCE(stderr, ''), truncated FROM outputs WHERE command_id = ?",
		commandID,
	).Scan(&o.CommandID, &o.Stdout, &o.Stderr, &o.Truncated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// InsertTestResult inserts a test result record.
func (s *Store) InsertTestResult(tr model.TestResult) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO test_results (id, session_id, name, passed, duration_ms, output) VALUES (?, ?, ?, ?, ?, ?)",
		tr.ID, tr.SessionID, tr.Name, tr.Passed, tr.DurationMs, tr.Output,
	)
	return err
}

// QueryTestResults returns test results for a session, optionally filtered by status.
func (s *Store) QueryTestResults(sessionID string, status string) ([]model.TestResult, error) {
	var rows *sql.Rows
	var err error
	switch status {
	case "passed":
		rows, err = s.db.Query(
			"SELECT id, session_id, name, passed, COALESCE(duration_ms, 0), COALESCE(output, '') FROM test_results WHERE session_id = ? AND passed = TRUE ORDER BY name",
			sessionID,
		)
	case "failed":
		rows, err = s.db.Query(
			"SELECT id, session_id, name, passed, COALESCE(duration_ms, 0), COALESCE(output, '') FROM test_results WHERE session_id = ? AND passed = FALSE ORDER BY name",
			sessionID,
		)
	default:
		rows, err = s.db.Query(
			"SELECT id, session_id, name, passed, COALESCE(duration_ms, 0), COALESCE(output, '') FROM test_results WHERE session_id = ? ORDER BY name",
			sessionID,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.TestResult
	for rows.Next() {
		var tr model.TestResult
		if err := rows.Scan(&tr.ID, &tr.SessionID, &tr.Name, &tr.Passed, &tr.DurationMs, &tr.Output); err != nil {
			return nil, err
		}
		results = append(results, tr)
	}
	return results, rows.Err()
}

// InsertError inserts an error record.
func (s *Store) InsertError(er model.Error) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO errors (id, command_id, message, stack_trace) VALUES (?, ?, ?, ?)",
		er.ID, er.CommandID, er.Message, er.StackTrace,
	)
	return err
}

// QueryErrorsBySession returns all errors for a session.
func (s *Store) QueryErrorsBySession(sessionID string) ([]model.Error, error) {
	rows, err := s.db.Query(
		`SELECT e.id, e.command_id, e.message, COALESCE(e.stack_trace, '')
		 FROM errors e
		 JOIN commands c ON e.command_id = c.id
		 WHERE c.session_id = ?
		 ORDER BY c.seq`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var errors []model.Error
	for rows.Next() {
		var er model.Error
		if err := rows.Scan(&er.ID, &er.CommandID, &er.Message, &er.StackTrace); err != nil {
			return nil, err
		}
		errors = append(errors, er)
	}
	return errors, rows.Err()
}

// CommandCount returns the total number of stored commands.
func (s *Store) CommandCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM commands").Scan(&count)
	return count, err
}

// FailedCommandCount returns the number of commands with non-zero exit codes.
func (s *Store) FailedCommandCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM commands WHERE exit_code != 0").Scan(&count)
	return count, err
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/graph/ -run TestExecutionStore -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/graph/store.go internal/graph/store_execution.go internal/graph/store_execution_test.go
git commit -m "feat(graph): add execution store schema and CRUD methods"
```

---

### Task 3: Session Pruning

Implement pruning of old execution sessions to enforce the retention limit.

**Files:**
- Create: `internal/graph/execution/prune.go`
- Create: `internal/graph/execution/prune_test.go`

**Step 1: Write the test**

Create `internal/graph/execution/prune_test.go`:

```go
package execution

import (
	"path/filepath"
	"testing"

	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/graph/model"
)

func TestPruneSessions(t *testing.T) {
	dir := t.TempDir()
	s, err := graph.OpenStore(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Insert 7 sessions with commands, outputs, errors, test results, and edges
	for i := 1; i <= 7; i++ {
		sid := fmt.Sprintf("s%d", i)
		s.InsertExecution(model.Execution{SessionID: sid, StartedAt: int64(i * 1000), Status: "completed"})
		cmdID := fmt.Sprintf("cmd:%s:1", sid)
		s.InsertCommand(model.Command{ID: cmdID, SessionID: sid, Seq: 1, Command: "echo hi", ExitCode: 0})
		s.InsertOutput(model.Output{CommandID: cmdID, Stdout: "hi"})
		s.InsertTestResult(model.TestResult{ID: fmt.Sprintf("test:%s:T1", sid), SessionID: sid, Name: "T1", Passed: true})
		s.InsertError(model.Error{ID: fmt.Sprintf("err:%s:1", sid), CommandID: cmdID, Message: "oops"})
		// Add an edge referencing this command
		s.InsertBatch(nil, []graph.Edge{{From: cmdID, To: "file:main.go", Type: "ACCESSES"}})
	}

	// Prune to keep 5
	pruned, err := PruneSessions(s, 5)
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 2 {
		t.Fatalf("expected 2 pruned, got %d", pruned)
	}

	// Verify 5 sessions remain
	count, _ := s.ExecutionCount()
	if count != 5 {
		t.Fatalf("expected 5 sessions, got %d", count)
	}

	// Verify oldest sessions were pruned (s1 and s2)
	execs, _ := s.QueryExecutions(10)
	for _, e := range execs {
		if e.SessionID == "s1" || e.SessionID == "s2" {
			t.Fatalf("session %s should have been pruned", e.SessionID)
		}
	}

	// Verify cascaded deletes
	cmds, _ := s.QueryCommandsBySession("s1")
	if len(cmds) != 0 {
		t.Fatal("expected commands for s1 to be deleted")
	}
	tests, _ := s.QueryTestResults("s1", "")
	if len(tests) != 0 {
		t.Fatal("expected test results for s1 to be deleted")
	}
}

func TestPruneSessions_NoPruneNeeded(t *testing.T) {
	dir := t.TempDir()
	s, err := graph.OpenStore(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.InsertExecution(model.Execution{SessionID: "s1", StartedAt: 1000, Status: "completed"})

	pruned, err := PruneSessions(s, 5)
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 0 {
		t.Fatalf("expected 0 pruned, got %d", pruned)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/execution/ -run TestPruneSessions -v`
Expected: FAIL — package and function don't exist.

**Step 3: Implement pruning**

Create `internal/graph/execution/prune.go`:

```go
package execution

import (
	"fmt"

	"github.com/lofari/golem/internal/graph"
)

// PruneSessions removes the oldest execution sessions beyond the retention limit.
// Returns the number of sessions pruned.
func PruneSessions(store *graph.Store, keep int) (int, error) {
	execs, err := store.QueryExecutions(1000) // ordered newest-first
	if err != nil {
		return 0, fmt.Errorf("querying executions: %w", err)
	}

	if len(execs) <= keep {
		return 0, nil
	}

	// Sessions to prune are those beyond the keep limit (oldest ones)
	toPrune := execs[keep:]
	for _, e := range toPrune {
		if err := store.DeleteExecution(e.SessionID); err != nil {
			return 0, fmt.Errorf("deleting session %s: %w", e.SessionID, err)
		}
	}

	return len(toPrune), nil
}
```

**Step 4: Add DeleteExecution to store**

Add to `internal/graph/store_execution.go`:

```go
// DeleteExecution removes an execution and all its related data (commands, outputs, errors, test results, edges).
func (s *Store) DeleteExecution(sessionID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete edges referencing execution nodes
	_, err = tx.Exec(`DELETE FROM edges WHERE from_node LIKE ? OR to_node LIKE ?`,
		"cmd:"+sessionID+":%", "cmd:"+sessionID+":%")
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM edges WHERE from_node LIKE ? OR to_node LIKE ?`,
		"err:"+sessionID+":%", "err:"+sessionID+":%")
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM edges WHERE from_node LIKE ? OR to_node LIKE ?`,
		"test:"+sessionID+":%", "test:"+sessionID+":%")
	if err != nil {
		return err
	}

	// Delete outputs (via command IDs)
	_, err = tx.Exec(`DELETE FROM outputs WHERE command_id IN (SELECT id FROM commands WHERE session_id = ?)`, sessionID)
	if err != nil {
		return err
	}

	// Delete errors (via command IDs)
	_, err = tx.Exec(`DELETE FROM errors WHERE command_id IN (SELECT id FROM commands WHERE session_id = ?)`, sessionID)
	if err != nil {
		return err
	}

	// Delete test results
	_, err = tx.Exec(`DELETE FROM test_results WHERE session_id = ?`, sessionID)
	if err != nil {
		return err
	}

	// Delete commands
	_, err = tx.Exec(`DELETE FROM commands WHERE session_id = ?`, sessionID)
	if err != nil {
		return err
	}

	// Delete execution
	_, err = tx.Exec(`DELETE FROM executions WHERE session_id = ?`, sessionID)
	if err != nil {
		return err
	}

	return tx.Commit()
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/graph/execution/ -run TestPruneSessions -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/graph/execution/prune.go internal/graph/execution/prune_test.go internal/graph/store_execution.go
git commit -m "feat(graph): add execution session pruning"
```

---

### Task 4: Reference Extractor

Extract file paths, Go stack traces, and Go test results from command text and output.

**Files:**
- Create: `internal/graph/execution/refs.go`
- Create: `internal/graph/execution/refs_test.go`

**Step 1: Write the test**

Create `internal/graph/execution/refs_test.go`:

```go
package execution

import (
	"testing"
)

func TestExtractFilePaths(t *testing.T) {
	// Known project files
	known := map[string]bool{
		"internal/runner/builder.go": true,
		"cmd/graph.go":              true,
		"main.go":                   true,
	}

	text := `go test ./internal/runner/
--- FAIL: TestBuilder (0.01s)
    builder_test.go:42: assertion failed
FAIL	internal/runner	0.015s
Error in cmd/graph.go:15`

	paths := ExtractFilePaths(text, known)
	if len(paths) == 0 {
		t.Fatal("expected file paths")
	}

	found := make(map[string]bool)
	for _, p := range paths {
		found[p] = true
	}
	if !found["cmd/graph.go"] {
		t.Error("expected cmd/graph.go")
	}
}

func TestExtractGoTestResults(t *testing.T) {
	output := `=== RUN   TestFoo
--- PASS: TestFoo (0.01s)
=== RUN   TestBar
--- FAIL: TestBar (0.05s)
    bar_test.go:10: expected 1, got 2
=== RUN   TestBaz/subtest
--- PASS: TestBaz/subtest (0.00s)
FAIL
exit status 1`

	results := ExtractGoTestResults(output)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	byName := make(map[string]bool)
	for _, r := range results {
		byName[r.Name] = r.Passed
	}
	if !byName["TestFoo"] {
		t.Error("TestFoo should pass")
	}
	if byName["TestBar"] {
		t.Error("TestBar should fail")
	}
	if !byName["TestBaz/subtest"] {
		t.Error("TestBaz/subtest should pass")
	}
}

func TestExtractGoStackTrace(t *testing.T) {
	output := `goroutine 1 [running]:
main.main()
	/home/user/project/main.go:42 +0x1a4
github.com/foo/bar.Init()
	/home/user/project/internal/runner/builder.go:100 +0x84`

	files := ExtractGoStackFiles(output)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	found := make(map[string]bool)
	for _, f := range files {
		found[f.Path] = true
	}
	if !found["main.go"] {
		t.Error("expected main.go")
	}
	if !found["internal/runner/builder.go"] {
		t.Error("expected internal/runner/builder.go")
	}
}

func TestExtractPythonTracebackFiles(t *testing.T) {
	output := `Traceback (most recent call last):
  File "src/app.py", line 42, in main
    result = process()
  File "src/utils/helper.py", line 10, in process
    raise ValueError("bad")`

	files := ExtractPythonTracebackFiles(output)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/execution/ -run TestExtract -v`
Expected: FAIL — functions not defined.

**Step 3: Implement the extractors**

Create `internal/graph/execution/refs.go`:

```go
package execution

import (
	"regexp"
	"strconv"
	"strings"
)

// FileRef represents a file reference found in output.
type FileRef struct {
	Path string
	Line int // 0 if unknown
}

// ParsedTestResult represents a test result parsed from output.
type ParsedTestResult struct {
	Name       string
	Passed     bool
	DurationMs int
}

// ExtractFilePaths finds project file paths in text.
// knownFiles is a set of known project-relative file paths.
func ExtractFilePaths(text string, knownFiles map[string]bool) []string {
	// Match patterns like path/to/file.ext or path/to/file.ext:123
	re := regexp.MustCompile(`(?:^|\s|["'(])([a-zA-Z0-9_./\-]+\.\w+)(?::\d+)?`)
	matches := re.FindAllStringSubmatch(text, -1)

	seen := make(map[string]bool)
	var result []string
	for _, m := range matches {
		path := m[1]
		if knownFiles[path] && !seen[path] {
			seen[path] = true
			result = append(result, path)
		}
	}
	return result
}

// ExtractGoTestResults parses Go test output for pass/fail results.
func ExtractGoTestResults(output string) []ParsedTestResult {
	// Match: --- PASS: TestName (0.01s) or --- FAIL: TestName (0.05s)
	re := regexp.MustCompile(`--- (PASS|FAIL): (\S+) \((\d+\.\d+)s\)`)
	matches := re.FindAllStringSubmatch(output, -1)

	var results []ParsedTestResult
	for _, m := range matches {
		duration, _ := strconv.ParseFloat(m[3], 64)
		results = append(results, ParsedTestResult{
			Name:       m[2],
			Passed:     m[1] == "PASS",
			DurationMs: int(duration * 1000),
		})
	}
	return results
}

// ExtractGoStackFiles extracts file paths from Go stack traces.
func ExtractGoStackFiles(output string) []FileRef {
	// Match: \t/absolute/path/to/file.go:42 +0x...
	re := regexp.MustCompile(`\t(/[^\s]+\.go):(\d+)`)
	matches := re.FindAllStringSubmatch(output, -1)

	seen := make(map[string]bool)
	var refs []FileRef
	for _, m := range matches {
		absPath := m[1]
		line, _ := strconv.Atoi(m[2])

		// Try to extract project-relative path
		// Look for common project directory markers
		relPath := toProjectRelative(absPath)
		if !seen[relPath] {
			seen[relPath] = true
			refs = append(refs, FileRef{Path: relPath, Line: line})
		}
	}
	return refs
}

// ExtractPythonTracebackFiles extracts file paths from Python tracebacks.
func ExtractPythonTracebackFiles(output string) []FileRef {
	// Match: File "path.py", line 42
	re := regexp.MustCompile(`File "([^"]+\.py)", line (\d+)`)
	matches := re.FindAllStringSubmatch(output, -1)

	seen := make(map[string]bool)
	var refs []FileRef
	for _, m := range matches {
		path := m[1]
		line, _ := strconv.Atoi(m[2])
		if !seen[path] {
			seen[path] = true
			refs = append(refs, FileRef{Path: path, Line: line})
		}
	}
	return refs
}

// toProjectRelative attempts to convert an absolute path to a project-relative one.
// It strips everything up to and including common project directory patterns.
func toProjectRelative(absPath string) string {
	// Try to find a recognizable project root indicator
	markers := []string{"/internal/", "/cmd/", "/pkg/", "/src/", "/lib/", "/test/", "/tests/"}
	for _, m := range markers {
		if idx := strings.Index(absPath, m); idx >= 0 {
			return absPath[idx+1:] // strip leading slash
		}
	}
	// Fallback: return basename parts after last /home/ or /root/ or /tmp/
	for _, prefix := range []string{"/home/", "/root/", "/tmp/"} {
		if idx := strings.Index(absPath, prefix); idx >= 0 {
			rest := absPath[idx+len(prefix):]
			// Skip user dir: user/project/...
			parts := strings.SplitN(rest, "/", 3)
			if len(parts) >= 3 {
				return parts[2]
			}
		}
	}
	// Last resort: just the filename
	if idx := strings.LastIndex(absPath, "/"); idx >= 0 {
		return absPath[idx+1:]
	}
	return absPath
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/graph/execution/ -run TestExtract -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/graph/execution/refs.go internal/graph/execution/refs_test.go
git commit -m "feat(graph): add reference extractor for file paths, stack traces, and test results"
```

---

### Task 5: Output Truncation

Implement output truncation logic (first 50 + last 50 lines for success, full for failures).

**Files:**
- Create: `internal/graph/execution/truncate.go`
- Create: `internal/graph/execution/truncate_test.go`

**Step 1: Write the test**

Create `internal/graph/execution/truncate_test.go`:

```go
package execution

import (
	"fmt"
	"strings"
	"testing"
)

func TestTruncateOutput_Short(t *testing.T) {
	text := "line1\nline2\nline3"
	result, truncated := TruncateOutput(text, 50)
	if truncated {
		t.Fatal("should not truncate short output")
	}
	if result != text {
		t.Fatalf("expected original text, got %q", result)
	}
}

func TestTruncateOutput_Long(t *testing.T) {
	var lines []string
	for i := 1; i <= 200; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	text := strings.Join(lines, "\n")

	result, truncated := TruncateOutput(text, 50)
	if !truncated {
		t.Fatal("should truncate long output")
	}

	resultLines := strings.Split(result, "\n")
	// Should have 50 + 1 (separator) + 50 = 101 lines
	if len(resultLines) != 101 {
		t.Fatalf("expected 101 lines, got %d", len(resultLines))
	}
	if resultLines[0] != "line 1" {
		t.Fatalf("first line should be 'line 1', got %q", resultLines[0])
	}
	if resultLines[100] != "line 200" {
		t.Fatalf("last line should be 'line 200', got %q", resultLines[100])
	}
	// Check separator
	if !strings.Contains(resultLines[50], "truncated") {
		t.Fatalf("expected truncation marker, got %q", resultLines[50])
	}
}

func TestTruncateOutput_ExactBoundary(t *testing.T) {
	var lines []string
	for i := 1; i <= 100; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	text := strings.Join(lines, "\n")

	_, truncated := TruncateOutput(text, 50)
	if truncated {
		t.Fatal("100 lines with limit 50 should not truncate (50+50 = 100)")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/execution/ -run TestTruncateOutput -v`
Expected: FAIL

**Step 3: Implement truncation**

Create `internal/graph/execution/truncate.go`:

```go
package execution

import (
	"fmt"
	"strings"
)

// TruncateOutput keeps the first N and last N lines of text.
// Returns the truncated text and whether truncation occurred.
func TruncateOutput(text string, keepLines int) (string, bool) {
	lines := strings.Split(text, "\n")
	total := len(lines)

	if total <= keepLines*2 {
		return text, false
	}

	head := lines[:keepLines]
	tail := lines[total-keepLines:]
	omitted := total - keepLines*2

	result := strings.Join(head, "\n") +
		fmt.Sprintf("\n... [%d lines truncated] ...\n", omitted) +
		strings.Join(tail, "\n")

	return result, true
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/graph/execution/ -run TestTruncateOutput -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/graph/execution/truncate.go internal/graph/execution/truncate_test.go
git commit -m "feat(graph): add output truncation for execution capture"
```

---

### Task 6: Execution Collector

The core component that receives stream-json events and populates the execution graph.

**Files:**
- Create: `internal/graph/execution/collector.go`
- Create: `internal/graph/execution/collector_test.go`

**Step 1: Write the test**

Create `internal/graph/execution/collector_test.go`:

```go
package execution

import (
	"path/filepath"
	"testing"

	"github.com/lofari/golem/internal/graph"
)

func TestCollector_BasicFlow(t *testing.T) {
	dir := t.TempDir()
	store, err := graph.OpenStore(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Insert a known file node so ACCESSES edges can be created
	store.InsertBatch(
		[]graph.Node{{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go"}},
		nil,
	)

	c := NewCollector(store, "test-session")
	c.Start()

	// Simulate a bash command
	c.OnBashCommand("go build ./...", "")
	c.OnBashResult(0, "build successful\n", "")

	// Simulate a failing command referencing main.go
	c.OnBashCommand("go test ./...", "")
	c.OnBashResult(1, "--- FAIL: TestMain (0.01s)\n    main.go:42: error\nFAIL", "exit status 1")

	c.Finish("completed")

	// Verify execution was created
	exec, err := store.LatestExecution()
	if err != nil {
		t.Fatal(err)
	}
	if exec.SessionID != "test-session" {
		t.Fatalf("expected test-session, got %s", exec.SessionID)
	}
	if exec.Status != "completed" {
		t.Fatalf("expected completed, got %s", exec.Status)
	}

	// Verify commands
	cmds, _ := store.QueryCommandsBySession("test-session")
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
	if cmds[0].Command != "go build ./..." {
		t.Fatalf("unexpected command: %s", cmds[0].Command)
	}
	if cmds[1].ExitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", cmds[1].ExitCode)
	}

	// Verify test results were extracted
	tests, _ := store.QueryTestResults("test-session", "")
	if len(tests) != 1 {
		t.Fatalf("expected 1 test result, got %d", len(tests))
	}
	if tests[0].Name != "TestMain" {
		t.Fatalf("expected TestMain, got %s", tests[0].Name)
	}
	if tests[0].Passed {
		t.Fatal("expected TestMain to fail")
	}

	// Verify error was created for failing command
	errors, _ := store.QueryErrorsBySession("test-session")
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}

	// Verify ACCESSES edge was created (main.go was referenced in output)
	edges, _ := store.EdgesFrom(cmds[1].ID)
	foundAccess := false
	for _, e := range edges {
		if e.Type == "ACCESSES" && e.To == "file:main.go" {
			foundAccess = true
		}
	}
	if !foundAccess {
		t.Fatal("expected ACCESSES edge from command to file:main.go")
	}
}

func TestCollector_OutputTruncation(t *testing.T) {
	dir := t.TempDir()
	store, err := graph.OpenStore(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	c := NewCollector(store, "trunc-session")
	c.Start()

	// Generate long output (200 lines)
	var longOutput string
	for i := 0; i < 200; i++ {
		longOutput += "output line\n"
	}

	c.OnBashCommand("echo lots", "")
	c.OnBashResult(0, longOutput, "")
	c.Finish("completed")

	// Verify output was truncated
	cmds, _ := store.QueryCommandsBySession("trunc-session")
	out, _ := store.QueryOutput(cmds[0].ID)
	if !out.Truncated {
		t.Fatal("expected output to be truncated")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/execution/ -run TestCollector -v`
Expected: FAIL

**Step 3: Implement the collector**

Create `internal/graph/execution/collector.go`:

```go
package execution

import (
	"fmt"
	"strings"
	"time"

	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/graph/model"
)

const defaultKeepLines = 50

// Collector receives bash tool-use events and populates the execution graph.
type Collector struct {
	store     *graph.Store
	sessionID string
	seq       int
	pendingCmd string
	pendingDir string
	knownFiles map[string]bool // cache of known file paths in graph
}

// NewCollector creates a new execution collector for the given session.
func NewCollector(store *graph.Store, sessionID string) *Collector {
	return &Collector{
		store:     store,
		sessionID: sessionID,
	}
}

// Start initializes the execution session and prunes old sessions.
func (c *Collector) Start() {
	c.loadKnownFiles()
	c.store.InsertExecution(model.Execution{
		SessionID: c.sessionID,
		StartedAt: time.Now().Unix(),
		Status:    "running",
	})
}

// OnBashCommand records a new bash command being executed.
func (c *Collector) OnBashCommand(command, workingDir string) {
	c.seq++
	c.pendingCmd = command
	c.pendingDir = workingDir
}

// OnBashResult records the result of the pending bash command.
func (c *Collector) OnBashResult(exitCode int, stdout, stderr string) {
	if c.pendingCmd == "" {
		return
	}

	cmdID := fmt.Sprintf("cmd:%s:%d", c.sessionID, c.seq)

	// Store command
	c.store.InsertCommand(model.Command{
		ID:         cmdID,
		SessionID:  c.sessionID,
		Seq:        c.seq,
		Command:    c.pendingCmd,
		ExitCode:   exitCode,
		WorkingDir: c.pendingDir,
	})

	// Store output (truncated for success, full for failure)
	truncatedStdout := stdout
	truncated := false
	if exitCode == 0 {
		truncatedStdout, truncated = TruncateOutput(stdout, defaultKeepLines)
	}
	c.store.InsertOutput(model.Output{
		CommandID: cmdID,
		Stdout:    truncatedStdout,
		Stderr:    stderr,
		Truncated: truncated,
	})

	// Extract references from combined command + output
	combined := c.pendingCmd + "\n" + stdout + "\n" + stderr
	c.extractReferences(cmdID, combined, exitCode)

	// Create error node for failures
	if exitCode != 0 {
		errMsg := extractErrorMessage(stderr, stdout)
		stackTrace := ""
		stackFiles := ExtractGoStackFiles(combined)
		if len(stackFiles) > 0 {
			stackTrace = extractStackTraceText(combined)
		}

		errID := fmt.Sprintf("err:%s:%d", c.sessionID, c.seq)
		c.store.InsertError(model.Error{
			ID:         errID,
			CommandID:  cmdID,
			Message:    errMsg,
			StackTrace: stackTrace,
		})

		// Create FAILS_IN edges for stack trace files
		for _, f := range stackFiles {
			fileNodeID := "file:" + f.Path
			if c.knownFiles[f.Path] {
				c.store.InsertBatch(nil, []graph.Edge{{From: errID, To: fileNodeID, Type: "FAILS_IN"}})
			}
		}
	}

	c.pendingCmd = ""
	c.pendingDir = ""
}

// Finish finalizes the execution session.
func (c *Collector) Finish(status string) {
	c.store.FinalizeExecution(c.sessionID, time.Now().Unix(), status)
}

func (c *Collector) extractReferences(cmdID, text string, exitCode int) {
	// Extract file path references -> ACCESSES edges
	filePaths := ExtractFilePaths(text, c.knownFiles)
	var edges []graph.Edge
	for _, p := range filePaths {
		edges = append(edges, graph.Edge{From: cmdID, To: "file:" + p, Type: "ACCESSES"})
	}

	// Extract Go test results
	testResults := ExtractGoTestResults(text)
	for _, tr := range testResults {
		testID := fmt.Sprintf("test:%s:%s", c.sessionID, tr.Name)
		c.store.InsertTestResult(model.TestResult{
			ID:         testID,
			SessionID:  c.sessionID,
			Name:       tr.Name,
			Passed:     tr.Passed,
			DurationMs: tr.DurationMs,
		})

		// Try to link test to function node (best-effort)
		// Convention: TestFoo tests function Foo
		fnName := strings.TrimPrefix(tr.Name, "Test")
		if idx := strings.Index(fnName, "/"); idx > 0 {
			fnName = fnName[:idx] // strip subtests
		}
		if fnName != "" {
			fnNodes, _ := c.store.FindNodesByName(fnName)
			for _, n := range fnNodes {
				if n.Type == "function" || n.Type == "method" {
					edges = append(edges, graph.Edge{From: testID, To: n.ID, Type: "TESTS"})
				}
			}
		}

		// PRODUCES edge from command to test result
		edges = append(edges, graph.Edge{From: cmdID, To: testID, Type: "PRODUCES"})
	}

	// Extract Python traceback files -> FAILS_IN edges
	if exitCode != 0 {
		pyFiles := ExtractPythonTracebackFiles(text)
		for _, f := range pyFiles {
			if c.knownFiles[f.Path] {
				errID := fmt.Sprintf("err:%s:%d", c.sessionID, c.seq)
				edges = append(edges, graph.Edge{From: errID, To: "file:" + f.Path, Type: "FAILS_IN"})
			}
		}
	}

	if len(edges) > 0 {
		c.store.InsertBatch(nil, edges)
	}
}

func (c *Collector) loadKnownFiles() {
	c.knownFiles = make(map[string]bool)
	fileNodes, err := c.store.NodesByType("file")
	if err != nil {
		return
	}
	for _, n := range fileNodes {
		c.knownFiles[n.Path] = true
	}
}

// extractErrorMessage builds an error message from stderr (or stdout if stderr is empty).
func extractErrorMessage(stderr, stdout string) string {
	source := stderr
	if strings.TrimSpace(source) == "" {
		source = stdout
	}
	lines := strings.Split(strings.TrimSpace(source), "\n")
	// Take last 10 lines
	if len(lines) > 10 {
		lines = lines[len(lines)-10:]
	}
	return strings.Join(lines, "\n")
}

// extractStackTraceText extracts the raw stack trace portion from output.
func extractStackTraceText(text string) string {
	idx := strings.Index(text, "goroutine ")
	if idx < 0 {
		return ""
	}
	// Take up to 50 lines from the goroutine marker
	rest := text[idx:]
	lines := strings.Split(rest, "\n")
	if len(lines) > 50 {
		lines = lines[:50]
	}
	return strings.Join(lines, "\n")
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/graph/execution/ -run TestCollector -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/graph/execution/collector.go internal/graph/execution/collector_test.go
git commit -m "feat(graph): add execution collector for stream event processing"
```

---

### Task 7: Wire Collector into StreamParser

Modify the `StreamParser` to emit bash events that the `Collector` can consume, and wire it up in the builder loop.

**Files:**
- Modify: `internal/runner/stream.go`
- Modify: `internal/runner/command.go`
- Modify: `internal/runner/builder.go`
- Create: `internal/runner/stream_test.go` (extend existing tests if they exist)

**Step 1: Write the test**

Create or add to `internal/runner/stream_test.go`:

```go
package runner

import (
	"strings"
	"testing"
)

func TestStreamParser_BashCallback(t *testing.T) {
	var commands []string
	var results []struct {
		exitCode int
		stdout   string
	}

	parser := NewStreamParser(&strings.Builder{})
	parser.OnBashCommand = func(command, workingDir string) {
		commands = append(commands, command)
	}
	parser.OnBashResult = func(exitCode int, stdout, stderr string) {
		results = append(results, struct {
			exitCode int
			stdout   string
		}{exitCode, stdout})
	}

	// Simulate stream-json with a bash tool use and result
	// CLI format: tool_use then tool_result
	input := `{"type":"tool_use","tool":{"name":"Bash","input":{"command":"echo hello"}}}
{"type":"tool_result","tool":{"name":"Bash"},"content":"hello\n","exit_code":0}
`
	parser.Parse(strings.NewReader(input))

	if len(commands) != 1 || commands[0] != "echo hello" {
		t.Fatalf("expected 'echo hello', got %v", commands)
	}
	if len(results) != 1 || results[0].exitCode != 0 {
		t.Fatalf("unexpected result: %v", results)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/runner/ -run TestStreamParser_BashCallback -v`
Expected: FAIL — `OnBashCommand` field doesn't exist.

**Step 3: Add callback fields to StreamParser**

In `internal/runner/stream.go`, add callback fields to the `StreamParser` struct and invoke them from the relevant handlers. Add these fields:

```go
type StreamParser struct {
	display       io.Writer
	text          strings.Builder
	debugLog      *os.File
	OnBashCommand func(command, workingDir string) // called when a Bash tool is invoked
	OnBashResult  func(exitCode int, stdout, stderr string) // called when Bash result arrives
	lastBashCmd   string // track last bash command for result matching
}
```

Update `handleToolUse` to detect Bash calls:

```go
func (sp *StreamParser) handleToolUse(raw map[string]json.RawMessage) {
	if toolRaw, ok := raw["tool"]; ok {
		var tool struct {
			Name  string                 `json:"name"`
			Input map[string]interface{} `json:"input"`
		}
		if err := json.Unmarshal(toolRaw, &tool); err == nil && tool.Name != "" {
			fmt.Fprintln(sp.display, formatToolCall(tool.Name, tool.Input))
			if tool.Name == "Bash" && sp.OnBashCommand != nil {
				cmd := strVal(tool.Input, "command")
				sp.lastBashCmd = cmd
				sp.OnBashCommand(cmd, "")
			}
		}
	}
}
```

Add a new handler for `tool_result` events. Add `"tool_result"` to the switch in `Parse` (remove it from the skip list) and handle it:

```go
case "tool_result":
	sp.handleToolResult(raw)
```

```go
func (sp *StreamParser) handleToolResult(raw map[string]json.RawMessage) {
	if sp.OnBashResult == nil || sp.lastBashCmd == "" {
		return
	}

	// Parse tool_result for Bash
	toolRaw, ok := raw["tool"]
	if !ok {
		// Try content/exit_code at top level
		sp.parseToolResultFields(raw)
		return
	}

	var tool struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(toolRaw, &tool); err != nil || tool.Name != "Bash" {
		return
	}

	sp.parseToolResultFields(raw)
}

func (sp *StreamParser) parseToolResultFields(raw map[string]json.RawMessage) {
	var content string
	if c, ok := raw["content"]; ok {
		json.Unmarshal(c, &content)
	}

	exitCode := 0
	if ec, ok := raw["exit_code"]; ok {
		json.Unmarshal(ec, &exitCode)
	}

	// Split stdout/stderr (stream-json typically combines them in content)
	sp.OnBashResult(exitCode, content, "")
	sp.lastBashCmd = ""
}
```

Also update `handleAssistantMsg` to detect Bash tool uses in the assistant message format:

```go
case "tool_use":
	fmt.Fprintln(sp.display, formatToolCall(block.Name, block.Input))
	if block.Name == "Bash" && sp.OnBashCommand != nil {
		cmd := strVal(block.Input, "command")
		sp.lastBashCmd = cmd
		sp.OnBashCommand(cmd, "")
	}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/runner/ -run TestStreamParser_BashCallback -v`
Expected: PASS

**Step 5: Wire collector into builder loop**

In `internal/runner/command.go`, modify `runStreamJSON` to accept an optional collector callback setup. Add a field to `ClaudeRunner`:

```go
type ClaudeRunner struct {
	// ... existing fields ...
	SetupStreamCallbacks func(parser *StreamParser) // optional: hook execution collector into parser
}
```

In `runStreamJSON`, after creating the parser but before calling `parser.Parse`:

```go
if c.SetupStreamCallbacks != nil {
	c.SetupStreamCallbacks(parser)
}
```

In `internal/runner/builder.go`, in the graph sync section (around line 70), after syncing the graph, set up the execution collector. Add before the main loop (after graph store is opened):

```go
// Set up execution collector if graph exists and runner supports stream callbacks
if _, statErr := os.Stat(graphPath); statErr == nil {
	if claudeRunner, ok := cfg.Runner.(*ClaudeRunner); ok && claudeRunner.StreamJSON {
		if gStore, gErr := graph.OpenStore(graphPath); gErr == nil {
			sessionID := fmt.Sprintf("build-%d", time.Now().Unix())
			collector := execution.NewCollector(gStore, sessionID)

			// Prune old sessions
			execution.PruneSessions(gStore, 5)

			collector.Start()
			defer func() {
				status := "completed"
				if result.Halted {
					status = "failed"
				}
				collector.Finish(status)
				gStore.Close()
			}()

			claudeRunner.SetupStreamCallbacks = func(parser *StreamParser) {
				parser.OnBashCommand = collector.OnBashCommand
				parser.OnBashResult = collector.OnBashResult
			}
		}
	}
}
```

**Step 6: Run all tests**

Run: `go test ./internal/runner/ -v && go test ./internal/graph/execution/ -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/runner/stream.go internal/runner/command.go internal/runner/builder.go internal/runner/stream_test.go
git commit -m "feat(graph): wire execution collector into stream parser and builder loop"
```

---

### Task 8: MCP Tools — find_execution_failures

**Files:**
- Modify: `internal/mcp/graph_tools.go`
- Modify: `internal/mcp/server.go`
- Modify: `internal/mcp/server_test.go`

**Step 1: Write the test**

Add to `internal/mcp/server_test.go` — update the expected tool list to include `find_execution_failures`.

**Step 2: Implement the tool**

Add to `internal/mcp/graph_tools.go`:

```go
// --- find_execution_failures ---

func findExecutionFailuresTool() mcp.Tool {
	return mcp.Tool{
		Name:        "find_execution_failures",
		Description: "Find commands that failed during agent execution. Returns error details, stack traces, and affected files.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"session": map[string]string{"type": "string", "description": "Session ID (default: latest session)"},
				"file":    map[string]string{"type": "string", "description": "Filter failures by file path"},
			},
		},
	}
}

func (gs *GolemServer) handleFindExecutionFailures(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	sessionID := getStr(args, "session")
	fileFilter := getStr(args, "file")

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("opening graph: %v", err)), nil
	}
	defer store.Close()

	// Resolve session
	if sessionID == "" {
		latest, err := store.LatestExecution()
		if err != nil || latest == nil {
			return mcp.NewToolResultText("no execution sessions found"), nil
		}
		sessionID = latest.SessionID
	}

	// Get failed commands
	failedCmds, err := store.QueryFailedCommands(sessionID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("querying failures: %v", err)), nil
	}

	type failureResult struct {
		Command       string   `json:"command"`
		ExitCode      int      `json:"exitCode"`
		ErrorMessage  string   `json:"errorMessage,omitempty"`
		StackTrace    string   `json:"stackTrace,omitempty"`
		FilesInvolved []string `json:"filesInvolved,omitempty"`
	}

	var results []failureResult
	for _, cmd := range failedCmds {
		// Get errors for this command
		errors, _ := store.QueryErrorsBySession(sessionID)
		var errMsg, stackTrace string
		for _, e := range errors {
			if e.CommandID == cmd.ID {
				errMsg = e.Message
				stackTrace = e.StackTrace
				break
			}
		}

		// Get files accessed by this command
		edges, _ := store.EdgesFrom(cmd.ID)
		var files []string
		for _, e := range edges {
			if e.Type == "ACCESSES" {
				files = append(files, strings.TrimPrefix(e.To, "file:"))
			}
		}

		// Apply file filter
		if fileFilter != "" {
			matched := false
			for _, f := range files {
				if strings.Contains(f, fileFilter) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		results = append(results, failureResult{
			Command:       cmd.Command,
			ExitCode:      cmd.ExitCode,
			ErrorMessage:  errMsg,
			StackTrace:    stackTrace,
			FilesInvolved: files,
		})
	}

	if len(results) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no failures found in session %q", sessionID)), nil
	}

	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}
```

Register in `server.go`:

```go
gs.mcpServer.AddTool(findExecutionFailuresTool(), gs.handleFindExecutionFailures)
```

Update `ToolNames()` in `server.go` to include `"find_execution_failures"`.

**Step 3: Run tests**

Run: `go test ./internal/mcp/ -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/mcp/graph_tools.go internal/mcp/server.go internal/mcp/server_test.go
git commit -m "feat(mcp): add find_execution_failures tool"
```

---

### Task 9: MCP Tools — get_runtime_trace

**Files:**
- Modify: `internal/mcp/graph_tools.go`
- Modify: `internal/mcp/server.go`
- Modify: `internal/mcp/server_test.go`

**Step 1: Implement the tool**

Add to `internal/mcp/graph_tools.go`:

```go
// --- get_runtime_trace ---

func getRuntimeTraceTool() mcp.Tool {
	return mcp.Tool{
		Name:        "get_runtime_trace",
		Description: "Get a trace of commands executed during an agent session. Shows what happened and in what order.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"session":        map[string]string{"type": "string", "description": "Session ID (default: latest session)"},
				"command_filter": map[string]string{"type": "string", "description": "Filter commands by substring match"},
			},
		},
	}
}

func (gs *GolemServer) handleGetRuntimeTrace(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	sessionID := getStr(args, "session")
	cmdFilter := getStr(args, "command_filter")

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("opening graph: %v", err)), nil
	}
	defer store.Close()

	if sessionID == "" {
		latest, err := store.LatestExecution()
		if err != nil || latest == nil {
			return mcp.NewToolResultText("no execution sessions found"), nil
		}
		sessionID = latest.SessionID
	}

	cmds, err := store.QueryCommandsBySession(sessionID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("querying commands: %v", err)), nil
	}

	exec, _ := store.LatestExecution()

	type cmdEntry struct {
		Command       string   `json:"command"`
		ExitCode      int      `json:"exitCode"`
		FilesAccessed []string `json:"filesAccessed,omitempty"`
		OutputPreview string   `json:"outputPreview,omitempty"`
	}

	type traceResult struct {
		SessionID string     `json:"sessionId"`
		Status    string     `json:"status"`
		Commands  []cmdEntry `json:"commands"`
	}

	result := traceResult{SessionID: sessionID}
	if exec != nil {
		result.Status = exec.Status
	}

	for _, cmd := range cmds {
		if cmdFilter != "" && !strings.Contains(cmd.Command, cmdFilter) {
			continue
		}

		entry := cmdEntry{
			Command:  cmd.Command,
			ExitCode: cmd.ExitCode,
		}

		// Get files accessed
		edges, _ := store.EdgesFrom(cmd.ID)
		for _, e := range edges {
			if e.Type == "ACCESSES" {
				entry.FilesAccessed = append(entry.FilesAccessed, strings.TrimPrefix(e.To, "file:"))
			}
		}

		// Output preview (first 5 lines)
		if out, _ := store.QueryOutput(cmd.ID); out != nil && out.Stdout != "" {
			lines := strings.SplitN(out.Stdout, "\n", 6)
			if len(lines) > 5 {
				lines = lines[:5]
			}
			entry.OutputPreview = strings.Join(lines, "\n")
		}

		result.Commands = append(result.Commands, entry)
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}
```

Register in `server.go` and update `ToolNames()`.

**Step 2: Run tests**

Run: `go test ./internal/mcp/ -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/mcp/graph_tools.go internal/mcp/server.go internal/mcp/server_test.go
git commit -m "feat(mcp): add get_runtime_trace tool"
```

---

### Task 10: MCP Tools — find_test_results

**Files:**
- Modify: `internal/mcp/graph_tools.go`
- Modify: `internal/mcp/server.go`
- Modify: `internal/mcp/server_test.go`

**Step 1: Implement the tool**

Add to `internal/mcp/graph_tools.go`:

```go
// --- find_test_results ---

func findTestResultsTool() mcp.Tool {
	return mcp.Tool{
		Name:        "find_test_results",
		Description: "Find test results from agent execution. Shows which tests passed/failed and what functions they exercise.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"session": map[string]string{"type": "string", "description": "Session ID (default: latest session)"},
				"status":  map[string]string{"type": "string", "description": "Filter by status: passed, failed, or all (default: all)"},
				"name":    map[string]string{"type": "string", "description": "Filter by test name substring"},
			},
		},
	}
}

func (gs *GolemServer) handleFindTestResults(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	sessionID := getStr(args, "session")
	status := getStr(args, "status")
	nameFilter := getStr(args, "name")

	store, err := gs.openGraph()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("opening graph: %v", err)), nil
	}
	defer store.Close()

	if sessionID == "" {
		latest, err := store.LatestExecution()
		if err != nil || latest == nil {
			return mcp.NewToolResultText("no execution sessions found"), nil
		}
		sessionID = latest.SessionID
	}

	tests, err := store.QueryTestResults(sessionID, status)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("querying test results: %v", err)), nil
	}

	type exercisedFn struct {
		Function string `json:"function"`
		Path     string `json:"path"`
		Line     int    `json:"line,omitempty"`
	}

	type testEntry struct {
		Name       string        `json:"name"`
		Passed     bool          `json:"passed"`
		DurationMs int           `json:"durationMs,omitempty"`
		Output     string        `json:"output,omitempty"`
		Exercises  []exercisedFn `json:"exercises,omitempty"`
	}

	var results []testEntry
	for _, tr := range tests {
		if nameFilter != "" && !strings.Contains(tr.Name, nameFilter) {
			continue
		}

		entry := testEntry{
			Name:       tr.Name,
			Passed:     tr.Passed,
			DurationMs: tr.DurationMs,
			Output:     tr.Output,
		}

		// Find TESTS edges from this test result
		edges, _ := store.EdgesFrom(tr.ID)
		for _, e := range edges {
			if e.Type == "TESTS" {
				if n, _ := store.NodeByID(e.To); n != nil {
					entry.Exercises = append(entry.Exercises, exercisedFn{
						Function: n.Name,
						Path:     n.Path,
						Line:     n.Line,
					})
				}
			}
		}

		results = append(results, entry)
	}

	if len(results) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no test results found in session %q", sessionID)), nil
	}

	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}
```

Register in `server.go` and update `ToolNames()`.

**Step 2: Run tests**

Run: `go test ./internal/mcp/ -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/mcp/graph_tools.go internal/mcp/server.go internal/mcp/server_test.go
git commit -m "feat(mcp): add find_test_results tool"
```

---

### Task 11: Config — execution-history

Add the `execution-history` config key for controlling session retention.

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go` (if exists)
- Modify: `internal/runner/builder.go` (use config value)

**Step 1: Add to config struct**

In `internal/config/config.go`, add to `Config`:

```go
ExecutionHistory int `yaml:"execution-history" json:"execution-history"`
```

Add to `Defaults()`:

```go
ExecutionHistory: 5,
```

Add to `configLayer`:

```go
ExecutionHistory *int `yaml:"execution-history"`
```

Add to `merge()`:

```go
if layer.ExecutionHistory != nil {
	base.ExecutionHistory = *layer.ExecutionHistory
}
```

Add to `GetValue()`:

```go
case "execution-history":
	return strconv.Itoa(cfg.ExecutionHistory), nil
```

Add to `PrintConfig()`:

```go
fmt.Fprintf(w, "execution-history: %d\n", cfg.ExecutionHistory)
```

Add to `Keys()`:

```go
{"execution-history", "number of execution sessions to retain (default 5)"},
```

**Step 2: Update builder.go to read config**

In the execution collector setup in `builder.go`, replace the hardcoded `5` with the config value. This requires passing config into the builder or reading it there.

**Step 3: Run tests**

Run: `go test ./internal/config/ -v && go build ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/config/config.go internal/runner/builder.go
git commit -m "feat(config): add execution-history config key"
```

---

### Task 12: Graph Status — Execution Stats

Update `golem graph status` to show execution statistics.

**Files:**
- Modify: `cmd/graph.go`

**Step 1: Add execution stats to graphStatusCmd**

After the history stats section in `graphStatusCmd`, add:

```go
// Execution stats
execCount, _ := store.ExecutionCount()
if execCount > 0 {
	cmdCount, _ := store.CommandCount()
	failCount, _ := store.FailedCommandCount()
	fmt.Printf("\nExecution: %d sessions, %d commands (%d failed)\n", execCount, cmdCount, failCount)
	if latest, _ := store.LatestExecution(); latest != nil {
		fmt.Printf("Latest session: %s (status: %s)\n", latest.SessionID, latest.Status)
	}
}
```

**Step 2: Run build to verify**

Run: `go build ./...`
Expected: Compiles successfully.

**Step 3: Commit**

```bash
git add cmd/graph.go
git commit -m "feat(cli): show execution stats in golem graph status"
```

---

### Task 13: Integration Test

End-to-end test validating the full pipeline: stream parsing -> collector -> store -> MCP query.

**Files:**
- Create: `internal/graph/execution/integration_test.go`

**Step 1: Write the integration test**

```go
package execution

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/graph/model"
	"github.com/lofari/golem/internal/runner"
)

func TestIntegration_StreamToGraph(t *testing.T) {
	dir := t.TempDir()
	store, err := graph.OpenStore(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Seed with file nodes
	store.InsertBatch(
		[]graph.Node{
			{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go"},
			{ID: "fn:main.go:Foo", Type: "function", Name: "Foo", Path: "main.go", Line: 10},
		},
		nil,
	)

	// Set up collector
	collector := NewCollector(store, "integration-test")
	collector.Start()

	// Set up stream parser with callbacks
	parser := runner.NewStreamParser(&strings.Builder{})
	parser.OnBashCommand = collector.OnBashCommand
	parser.OnBashResult = collector.OnBashResult

	// Feed simulated stream-json
	input := `{"type":"tool_use","tool":{"name":"Bash","input":{"command":"go test ./..."}}}
{"type":"tool_result","tool":{"name":"Bash"},"content":"--- FAIL: TestFoo (0.01s)\n    main.go:10: assertion failed\nFAIL","exit_code":1}
`
	parser.Parse(strings.NewReader(input))
	collector.Finish("completed")

	// Verify execution
	exec, _ := store.LatestExecution()
	if exec == nil || exec.Status != "completed" {
		t.Fatal("expected completed execution")
	}

	// Verify command captured
	cmds, _ := store.QueryCommandsBySession("integration-test")
	if len(cmds) != 1 || cmds[0].Command != "go test ./..." {
		t.Fatalf("unexpected commands: %v", cmds)
	}

	// Verify test result extracted
	tests, _ := store.QueryTestResults("integration-test", "")
	if len(tests) != 1 || tests[0].Name != "TestFoo" || tests[0].Passed {
		t.Fatalf("unexpected test results: %v", tests)
	}

	// Verify TESTS edge to Foo function
	edges, _ := store.EdgesFrom(tests[0].ID)
	foundTests := false
	for _, e := range edges {
		if e.Type == "TESTS" && e.To == "fn:main.go:Foo" {
			foundTests = true
		}
	}
	if !foundTests {
		t.Fatal("expected TESTS edge from test result to Foo function")
	}

	// Verify ACCESSES edge from command to file
	cmdEdges, _ := store.EdgesFrom(cmds[0].ID)
	foundAccess := false
	for _, e := range cmdEdges {
		if e.Type == "ACCESSES" && e.To == "file:main.go" {
			foundAccess = true
		}
	}
	if !foundAccess {
		t.Fatal("expected ACCESSES edge from command to file:main.go")
	}
}
```

**Step 2: Run the test**

Run: `go test ./internal/graph/execution/ -run TestIntegration -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/graph/execution/integration_test.go
git commit -m "test(graph): add execution graph integration test"
```

---

### Task 14: Final Verification

**Step 1: Run all tests**

Run: `go test ./... -v`
Expected: All pass.

**Step 2: Build**

Run: `go build ./...`
Expected: Compiles.

**Step 3: Verify graph status works**

Run: `go run . graph status`
Expected: Shows execution section (0 sessions if no runs yet).

**Step 4: Commit any remaining fixes**

```bash
git add -A
git commit -m "chore: final cleanup for Phase 5 execution graph"
```
