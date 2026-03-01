// internal/ctx/log_test.go
package ctx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadWriteLog(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	original := Log{
		Sessions: []Session{
			{Iteration: 1, Task: "setup", Outcome: "done", Summary: "did setup"},
		},
	}

	if err := WriteLog(dir, original); err != nil {
		t.Fatalf("WriteLog: %v", err)
	}

	got, err := ReadLog(dir)
	if err != nil {
		t.Fatalf("ReadLog: %v", err)
	}

	if len(got.Sessions) != 1 {
		t.Fatalf("len(Sessions) = %d, want 1", len(got.Sessions))
	}
	if got.Sessions[0].Task != "setup" {
		t.Errorf("Sessions[0].Task = %q, want %q", got.Sessions[0].Task, "setup")
	}
}

func TestAppendSession(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	// Start with empty log
	if err := WriteLog(dir, Log{Sessions: []Session{}}); err != nil {
		t.Fatal(err)
	}

	// Append two sessions
	if err := AppendSession(dir, Session{Iteration: 1, Task: "first", Outcome: "done"}); err != nil {
		t.Fatal(err)
	}
	if err := AppendSession(dir, Session{Iteration: 2, Task: "second", Outcome: "partial"}); err != nil {
		t.Fatal(err)
	}

	l, err := ReadLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Sessions) != 2 {
		t.Fatalf("len(Sessions) = %d, want 2", len(l.Sessions))
	}
	if l.Sessions[1].Task != "second" {
		t.Errorf("Sessions[1].Task = %q, want %q", l.Sessions[1].Task, "second")
	}
}

func TestLastNSessions(t *testing.T) {
	l := Log{Sessions: []Session{
		{Iteration: 1}, {Iteration: 2}, {Iteration: 3}, {Iteration: 4}, {Iteration: 5},
	}}

	got := l.LastNSessions(3)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Iteration != 3 {
		t.Errorf("first.Iteration = %d, want 3", got[0].Iteration)
	}

	// n=0 returns all
	if len(l.LastNSessions(0)) != 5 {
		t.Error("LastNSessions(0) should return all")
	}

	// n > len returns all
	if len(l.LastNSessions(100)) != 5 {
		t.Error("LastNSessions(100) should return all")
	}
}

func TestFailedSessions(t *testing.T) {
	l := Log{Sessions: []Session{
		{Iteration: 1, Outcome: "done"},
		{Iteration: 2, Outcome: "blocked"},
		{Iteration: 3, Outcome: "partial"},
		{Iteration: 4, Outcome: "unproductive"},
	}}

	got := l.FailedSessions()
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Iteration != 2 {
		t.Errorf("got[0].Iteration = %d, want 2", got[0].Iteration)
	}
	if got[1].Iteration != 4 {
		t.Errorf("got[1].Iteration = %d, want 4", got[1].Iteration)
	}
}
