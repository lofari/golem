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

func (s *Strategy) evaluateFailure(last golemctx.Session, log golemctx.Log) Decision {
	return Decision{Action: ActionContinue}
}

func (s *Strategy) evaluateThrashing(log golemctx.Log) Decision {
	return Decision{Action: ActionContinue}
}

func (s *Strategy) evaluateDeadlock(state golemctx.State) Decision {
	return Decision{Action: ActionContinue}
}

// Suppress unused import warnings — these will be used by rule implementations.
var _ = fmt.Sprintf
var _ = strings.Contains
