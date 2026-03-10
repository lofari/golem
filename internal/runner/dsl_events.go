package runner

import (
	"bufio"
	"encoding/json"
	"io"
)

// DSLEvent represents a JSON event emitted by golem-dsl on stdout.
type DSLEvent struct {
	Type       string `json:"type"`
	Step       string `json:"step,omitempty"`
	Iteration  int    `json:"iteration,omitempty"`
	Agent      string `json:"agent,omitempty"`
	SessionID  string `json:"session-id,omitempty"`
	Outcome    string `json:"outcome,omitempty"`
	DurationMs int    `json:"duration-ms,omitempty"`
	StateVer   int    `json:"state-version,omitempty"`
	ErrorType  string `json:"error-type,omitempty"`
	Action     string `json:"action,omitempty"`
	Attempt    int    `json:"attempt,omitempty"`
	TotalSteps int    `json:"total-steps,omitempty"`
}

// ParseDSLEvent parses a single NDJSON line into a DSLEvent.
func ParseDSLEvent(line string) (DSLEvent, error) {
	var evt DSLEvent
	err := json.Unmarshal([]byte(line), &evt)
	return evt, err
}

// ParseDSLEventStream reads all NDJSON events from a reader.
func ParseDSLEventStream(r io.Reader) ([]DSLEvent, error) {
	var events []DSLEvent
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		evt, err := ParseDSLEvent(scanner.Text())
		if err != nil {
			return events, err
		}
		events = append(events, evt)
	}
	return events, scanner.Err()
}

// MapDSLEvent converts a DSLEvent to the existing Event type for display.
func MapDSLEvent(dsl DSLEvent, maxIter int) Event {
	switch dsl.Type {
	case "step-start":
		return Event{Type: EventIterStart, Iter: dsl.Iteration, MaxIter: maxIter, Task: dsl.Step}
	case "step-end":
		return Event{Type: EventIterEnd, Iter: dsl.Iteration, MaxIter: maxIter, Task: dsl.Step, Outcome: "done"}
	case "session-end":
		return Event{Type: EventIterEnd, Iter: dsl.Iteration, Task: dsl.Step, Outcome: dsl.Outcome}
	case "error":
		return Event{Type: EventIterEnd, Iter: dsl.Iteration, Task: dsl.Step, Outcome: dsl.ErrorType}
	case "agent-done":
		result := &BuilderResult{
			Iterations: dsl.TotalSteps,
			Completed:  dsl.Outcome == "complete",
			Halted:     dsl.Outcome != "complete",
		}
		if result.Halted {
			result.HaltReason = dsl.Outcome
		}
		return Event{Type: EventLoopDone, Outcome: dsl.Outcome, Result: result}
	default:
		return Event{Type: EventOutputLine, Line: dsl.Type}
	}
}
