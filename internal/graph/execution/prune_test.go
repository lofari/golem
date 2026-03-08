package execution

import (
	"fmt"
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
