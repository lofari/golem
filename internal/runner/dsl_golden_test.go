package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGolden_CompleteRun(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "golden_events.ndjson"))
	if err != nil {
		t.Fatal(err)
	}

	events, err := ParseDSLEventStream(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}

	if len(events) != 7 {
		t.Fatalf("expected 7 events, got %d", len(events))
	}

	// Verify step-start events
	for _, idx := range []int{0, 2, 4} {
		if events[idx].Type != "step-start" {
			t.Errorf("event[%d]: expected type step-start, got %s", idx, events[idx].Type)
		}
		if events[idx].Agent != "build-feature" {
			t.Errorf("event[%d]: expected agent build-feature, got %s", idx, events[idx].Agent)
		}
	}

	// Verify step names in order
	steps := []string{"plan", "implement", "review"}
	for i, step := range steps {
		if events[i*2].Step != step {
			t.Errorf("event[%d]: expected step %s, got %s", i*2, step, events[i*2].Step)
		}
	}

	// Verify iterations increment
	for i := 0; i < 3; i++ {
		if events[i*2].Iteration != i+1 {
			t.Errorf("event[%d]: expected iteration %d, got %d", i*2, i+1, events[i*2].Iteration)
		}
	}

	// Verify step-end events have state versions
	for i := 0; i < 3; i++ {
		idx := i*2 + 1
		if events[idx].Type != "step-end" {
			t.Errorf("event[%d]: expected type step-end, got %s", idx, events[idx].Type)
		}
		if events[idx].StateVer != i+1 {
			t.Errorf("event[%d]: expected state-version %d, got %d", idx, i+1, events[idx].StateVer)
		}
	}

	// Verify agent-done
	last := events[6]
	if last.Type != "agent-done" {
		t.Fatalf("last event: expected agent-done, got %s", last.Type)
	}
	if last.Outcome != "complete" {
		t.Errorf("expected outcome complete, got %s", last.Outcome)
	}
	if last.TotalSteps != 3 {
		t.Errorf("expected total-steps 3, got %d", last.TotalSteps)
	}

	// Verify MapDSLEvent produces correct event types
	mapped := MapDSLEvent(events[0], 10)
	if mapped.Type != EventIterStart {
		t.Errorf("mapped step-start: expected %v, got %v", EventIterStart, mapped.Type)
	}

	mapped = MapDSLEvent(events[1], 10)
	if mapped.Type != EventIterEnd {
		t.Errorf("mapped step-end: expected %v, got %v", EventIterEnd, mapped.Type)
	}

	mapped = MapDSLEvent(last, 10)
	if mapped.Type != EventLoopDone {
		t.Errorf("mapped agent-done: expected %v, got %v", EventLoopDone, mapped.Type)
	}
	if mapped.Result == nil {
		t.Fatal("mapped agent-done: expected non-nil Result")
	}
	if !mapped.Result.Completed {
		t.Error("mapped agent-done: expected Completed=true")
	}
	if mapped.Result.Iterations != 3 {
		t.Errorf("mapped agent-done: expected Iterations=3, got %d", mapped.Result.Iterations)
	}
}

func TestGolden_HaltedRun(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "golden_events_halted.ndjson"))
	if err != nil {
		t.Fatal(err)
	}

	events, err := ParseDSLEventStream(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}

	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	// Verify error event
	errEvt := events[3]
	if errEvt.Type != "error" {
		t.Fatalf("event[3]: expected error, got %s", errEvt.Type)
	}
	if errEvt.ErrorType != "unrecoverable" {
		t.Errorf("expected error-type unrecoverable, got %s", errEvt.ErrorType)
	}

	// Verify halted outcome
	last := events[4]
	if last.Outcome != "halted" {
		t.Errorf("expected outcome halted, got %s", last.Outcome)
	}

	// Verify MapDSLEvent for error
	mapped := MapDSLEvent(errEvt, 10)
	if mapped.Type != EventIterEnd {
		t.Errorf("mapped error: expected %v, got %v", EventIterEnd, mapped.Type)
	}

	// Verify MapDSLEvent for halted agent-done
	mapped = MapDSLEvent(last, 10)
	if mapped.Result == nil {
		t.Fatal("mapped halted: expected non-nil Result")
	}
	if mapped.Result.Completed {
		t.Error("mapped halted: expected Completed=false")
	}
	if !mapped.Result.Halted {
		t.Error("mapped halted: expected Halted=true")
	}
}

func TestGolden_RetryRun(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "golden_events_with_retry.ndjson"))
	if err != nil {
		t.Fatal(err)
	}

	events, err := ParseDSLEventStream(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}

	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	// Verify error with retry action
	errEvt := events[1]
	if errEvt.Action != "retry" {
		t.Errorf("expected action retry, got %s", errEvt.Action)
	}
	if errEvt.Attempt != 1 {
		t.Errorf("expected attempt 1, got %d", errEvt.Attempt)
	}

	// Verify retry attempt in next step-start
	retry := events[2]
	if retry.Attempt != 2 {
		t.Errorf("expected attempt 2 on retry, got %d", retry.Attempt)
	}

	// Still completes successfully
	last := events[4]
	if last.Outcome != "complete" {
		t.Errorf("expected outcome complete after retry, got %s", last.Outcome)
	}
}
