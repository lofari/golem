package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// buildMockDSL compiles the mock-dsl binary into a temp directory and returns its path.
func buildMockDSL(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "mock-dsl")
	src := filepath.Join("testdata", "mock-dsl", "main.go")

	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to build mock-dsl: %v", err)
	}
	return bin
}

func TestDSLRunner_Run_Complete(t *testing.T) {
	bin := buildMockDSL(t)
	dir := t.TempDir()

	events := make(chan Event, 20)
	runner := &DSLRunner{
		DSLCommand: bin,
		Agent:      "mock-agent",
		Goal:       "test goal",
		StateDir:   dir,
		Events:     events,
		MaxIter:    10,
	}

	result, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	close(events)

	if !result.Completed {
		t.Error("expected Completed=true")
	}
	if result.Halted {
		t.Error("expected Halted=false")
	}
	if result.Iterations != 2 {
		t.Errorf("expected 2 iterations, got %d", result.Iterations)
	}

	// Collect all events
	var collected []Event
	for e := range events {
		collected = append(collected, e)
	}

	if len(collected) != 5 {
		t.Fatalf("expected 5 events, got %d", len(collected))
	}

	// Verify event sequence
	expectedTypes := []EventType{EventIterStart, EventIterEnd, EventIterStart, EventIterEnd, EventLoopDone}
	for i, et := range expectedTypes {
		if collected[i].Type != et {
			t.Errorf("event[%d]: expected type %v, got %v", i, et, collected[i].Type)
		}
	}

	// Verify final event has result
	last := collected[len(collected)-1]
	if last.Result == nil {
		t.Fatal("last event should have Result")
	}
	if !last.Result.Completed {
		t.Error("last event Result should be Completed")
	}
}

func TestDSLRunner_Run_Halted(t *testing.T) {
	bin := buildMockDSL(t)
	dir := t.TempDir()

	events := make(chan Event, 20)
	runner := &DSLRunner{
		DSLCommand: bin,
		Agent:      "halt-agent",
		Goal:       "test goal",
		StateDir:   dir,
		Events:     events,
		MaxIter:    10,
	}

	result, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	close(events)

	if result.Completed {
		t.Error("expected Completed=false")
	}
	if !result.Halted {
		t.Error("expected Halted=true")
	}
	if result.HaltReason != "halted" {
		t.Errorf("expected HaltReason=halted, got %s", result.HaltReason)
	}
}

func TestDSLRunner_Run_BinaryFailure(t *testing.T) {
	bin := buildMockDSL(t)
	dir := t.TempDir()

	runner := &DSLRunner{
		DSLCommand: bin,
		Agent:      "fail-agent",
		Goal:       "test goal",
		StateDir:   dir,
		MaxIter:    10,
	}

	result, err := runner.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for fail-agent")
	}

	if result == nil {
		t.Fatal("expected non-nil result even on failure")
	}
	if !result.Halted {
		t.Error("expected Halted=true on binary failure")
	}
}

func TestDSLRunner_Run_NoEvents(t *testing.T) {
	bin := buildMockDSL(t)
	dir := t.TempDir()

	// nil Events channel — should not panic
	runner := &DSLRunner{
		DSLCommand: bin,
		Agent:      "mock-agent",
		Goal:       "test goal",
		StateDir:   dir,
		MaxIter:    10,
	}

	result, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !result.Completed {
		t.Error("expected Completed=true")
	}
}
