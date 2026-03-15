package runner

import (
	"strings"
	"testing"
)

func TestParseDSLEvent_StepStart(t *testing.T) {
	line := `{"type":"step-start","step":"plan","iteration":1,"agent":"build-feature"}`
	evt, err := ParseDSLEvent(line)
	if err != nil {
		t.Fatal(err)
	}
	if evt.Type != "step-start" {
		t.Fatalf("expected step-start, got %s", evt.Type)
	}
	if evt.Step != "plan" {
		t.Fatalf("expected plan, got %s", evt.Step)
	}
}

func TestParseDSLEvent_AgentDone(t *testing.T) {
	line := `{"type":"agent-done","agent":"build-feature","outcome":"complete","total-steps":5}`
	evt, err := ParseDSLEvent(line)
	if err != nil {
		t.Fatal(err)
	}
	if evt.Outcome != "complete" {
		t.Fatalf("expected complete, got %s", evt.Outcome)
	}
	if evt.TotalSteps != 5 {
		t.Fatalf("expected total-steps=5, got %d", evt.TotalSteps)
	}
}

func TestParseDSLEvents_Stream(t *testing.T) {
	input := strings.NewReader(
		`{"type":"step-start","step":"plan","iteration":1,"agent":"build-feature"}
{"type":"step-end","step":"plan","state-version":1}
{"type":"agent-done","agent":"build-feature","outcome":"complete","total-steps":2}
`)
	events, err := ParseDSLEventStream(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
}

func TestMapDSLEvent_StepStart(t *testing.T) {
	dsl := DSLEvent{Type: "step-start", Step: "plan", Iteration: 1, Agent: "build-feature"}
	evt := MapDSLEvent(dsl, 5)
	if evt.Type != EventIterStart {
		t.Fatalf("expected EventIterStart, got %v", evt.Type)
	}
	if evt.Iter != 1 {
		t.Fatalf("expected iter=1, got %d", evt.Iter)
	}
	if evt.MaxIter != 5 {
		t.Fatalf("expected maxIter=5, got %d", evt.MaxIter)
	}
}

func TestMapDSLEvent_AgentDone(t *testing.T) {
	dsl := DSLEvent{Type: "agent-done", Outcome: "complete", TotalSteps: 5}
	evt := MapDSLEvent(dsl, 5)
	if evt.Type != EventLoopDone {
		t.Fatalf("expected EventLoopDone, got %v", evt.Type)
	}
	if !evt.Result.Completed {
		t.Fatal("expected Result.Completed=true for outcome=complete")
	}
}

func TestMapDSLEvent_AgentDoneHalted(t *testing.T) {
	dsl := DSLEvent{Type: "agent-done", Outcome: "halted", TotalSteps: 3}
	evt := MapDSLEvent(dsl, 5)
	if evt.Type != EventLoopDone {
		t.Fatalf("expected EventLoopDone, got %v", evt.Type)
	}
	if !evt.Result.Halted {
		t.Fatal("expected Result.Halted=true for outcome=halted")
	}
}

func TestMapDSLEvent_Error(t *testing.T) {
	dsl := DSLEvent{Type: "error", Step: "build", ErrorType: "timeout", Iteration: 2}
	evt := MapDSLEvent(dsl, 5)
	if evt.Type != EventIterEnd {
		t.Fatalf("expected EventIterEnd, got %v", evt.Type)
	}
	if evt.Outcome != "timeout" {
		t.Fatalf("expected outcome=timeout, got %s", evt.Outcome)
	}
}
