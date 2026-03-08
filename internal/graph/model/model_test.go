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
		ID:         "test:session-001:TestFoo",
		SessionID:  "session-001",
		Name:       "TestFoo",
		Passed:     true,
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
