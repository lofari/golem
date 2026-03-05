package runner

import (
	"fmt"
	"strings"

	golemctx "github.com/lofari/golem/internal/ctx"
)

// Action represents what the builder loop should do next.
type Action int

const (
	ActionContinue Action = iota // Proceed normally
	ActionRetry                  // Retry current task with injected context
	ActionSkip                   // Skip task, mark as blocked
	ActionHalt                   // Stop the loop
)

// Decision is returned by Strategy.Evaluate.
type Decision struct {
	Action        Action
	SkipTasks     []string // Tasks to mark as blocked
	InjectContext string   // Extra context to prepend to prompt
	HaltReason    string   // Why the loop should stop
}

// Strategy tracks iteration outcomes and decides how to adapt.
type Strategy struct {
	taskFailures      map[string]int // per-task failure count
	unproductiveCount int            // consecutive unproductive iterations
}

// NewStrategy creates a Strategy with initialized state.
func NewStrategy() *Strategy {
	return &Strategy{
		taskFailures: make(map[string]int),
	}
}

// Evaluate inspects state and log after an iteration and returns a Decision.
func (s *Strategy) Evaluate(state golemctx.State, log golemctx.Log, sessionOutput string) Decision {
	if len(log.Sessions) == 0 {
		return Decision{Action: ActionContinue}
	}

	last := log.Sessions[len(log.Sessions)-1]

	// Rule 4: No-progress detection
	d := s.evaluateProgress(last, sessionOutput)
	if d.Action != ActionContinue {
		return d
	}

	// Rule 1: Consecutive failure detection
	d = s.evaluateFailure(last, log)
	if d.Action != ActionContinue {
		return d
	}

	// Rule 3: Thrashing detection (3 consecutive same task)
	d = s.evaluateThrashing(log)
	if d.Action != ActionContinue {
		return d
	}

	// Rule 2: Dependency deadlock
	d = s.evaluateDeadlock(state)
	if d.Action != ActionContinue {
		return d
	}

	return Decision{Action: ActionContinue}
}

func (s *Strategy) evaluateProgress(last golemctx.Session, sessionOutput string) Decision {
	return Decision{Action: ActionContinue}
}

const maxTaskFailures = 2

func isFailedOutcome(outcome string) bool {
	return outcome == "blocked" || outcome == "unproductive"
}

func (s *Strategy) evaluateFailure(last golemctx.Session, log golemctx.Log) Decision {
	if last.Task == "" {
		return Decision{Action: ActionContinue}
	}

	// Reset on success
	if !isFailedOutcome(last.Outcome) {
		s.taskFailures[last.Task] = 0
		return Decision{Action: ActionContinue}
	}

	s.taskFailures[last.Task]++
	count := s.taskFailures[last.Task]

	if count >= maxTaskFailures {
		return Decision{
			Action:    ActionSkip,
			SkipTasks: []string{last.Task},
			InjectContext: fmt.Sprintf(
				"## Strategy Override\nTask %q has failed %d times and will be skipped. Work on a different task.\n",
				last.Task, count,
			),
		}
	}

	// First failure — retry with context
	summary := last.Summary
	if summary == "" {
		summary = last.Outcome
	}
	return Decision{
		Action: ActionRetry,
		InjectContext: fmt.Sprintf(
			"## Previous Iteration Context\nThe previous iteration attempted task %q but did not complete it. Outcome: %s.\n\nSummary: %s\n\nTry a different approach.\n",
			last.Task, last.Outcome, summary,
		),
	}
}

func (s *Strategy) evaluateThrashing(log golemctx.Log) Decision {
	return Decision{Action: ActionContinue}
}

func (s *Strategy) evaluateDeadlock(state golemctx.State) Decision {
	doneSet := make(map[string]bool)
	var remaining []golemctx.Task
	for _, t := range state.Tasks {
		if t.Status == "done" {
			doneSet[t.Name] = true
		} else {
			remaining = append(remaining, t)
		}
	}

	if len(remaining) == 0 {
		return Decision{Action: ActionContinue}
	}

	for _, t := range remaining {
		if t.Status == "blocked" {
			continue
		}
		if t.DependsOn.IsEmpty() {
			return Decision{Action: ActionContinue} // actionable
		}
		allDepsDone := true
		for _, dep := range t.DependsOn {
			if !doneSet[dep] {
				allDepsDone = false
				break
			}
		}
		if allDepsDone {
			return Decision{Action: ActionContinue} // actionable
		}
	}

	return Decision{
		Action:     ActionHalt,
		HaltReason: "all remaining tasks are blocked or depend on blocked tasks",
	}
}

// Suppress unused import warnings — these will be used by rule implementations.
var _ = fmt.Sprintf
var _ = strings.Contains
