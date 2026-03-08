package graph

import (
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
