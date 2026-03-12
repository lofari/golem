# Adaptive Iteration Engine Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a reactive strategy layer to the builder loop that retries stuck tasks with context injection, auto-skips after repeated failures, detects dependency deadlocks, and halts on sustained no-progress.

**Architecture:** A `Strategy` struct in `internal/runner/strategy.go` holds per-task failure counters and unproductive-iteration counters. Its `Evaluate()` method is called between iterations in `RunBuilderLoop`, returning a `Decision` that the loop applies (continue, retry with context, skip task, or halt). Prompt rendering gains an `InjectedContext` field for the strategy to prepend failure context.

**Tech Stack:** Go stdlib, existing `internal/ctx` types

---

### Task 1: Add InjectedContext to prompt rendering

**Files:**
- Modify: `internal/runner/prompt.go:22-39`
- Modify: `internal/runner/prompt_test.go`
- Modify: `templates/prompt.md:5`

**Step 1: Write the failing test**

Add to `internal/runner/prompt_test.go`:

```go
func TestRenderPrompt_InjectedContext(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	tmpl := "{{INJECTED_CONTEXT}}{{ITERATION_CONTEXT}}\n{{TASK_OVERRIDE}}{{DOCS_PATH}}"
	os.WriteFile(filepath.Join(dir, ".ctx", "prompt.md"), []byte(tmpl), 0644)

	result, err := RenderPrompt(dir, "prompt.md", PromptVars{
		DocsPath:         "docs/",
		IterationContext: "Iter 1 of 5.",
		InjectedContext:  "## Previous Iteration Context\nTask X failed.\n\n",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "## Previous Iteration Context") {
		t.Error("InjectedContext not rendered")
	}
	if !strings.Contains(result, "Iter 1 of 5") {
		t.Error("IterationContext missing after injection")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/runner/ -run TestRenderPrompt_InjectedContext -v`
Expected: FAIL — `InjectedContext` field does not exist on `PromptVars`

**Step 3: Add InjectedContext field and template replacement**

In `internal/runner/prompt.go`, add field to `PromptVars`:

```go
type PromptVars struct {
	DocsPath         string
	IterationContext string
	TaskOverride     string
	ReviewContext    string
	InjectedContext  string
}
```

In `RenderPrompt`, add replacement after the existing ones:

```go
content = strings.ReplaceAll(content, "{{INJECTED_CONTEXT}}", vars.InjectedContext)
```

**Step 4: Update the embedded prompt template**

In `templates/prompt.md`, add `{{INJECTED_CONTEXT}}` on line 5 (after the iteration context line):

```
{{ITERATION_CONTEXT}}

{{INJECTED_CONTEXT}}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/runner/ -run TestRenderPrompt -v`
Expected: PASS (both old and new tests)

**Step 6: Commit**

```bash
git add internal/runner/prompt.go internal/runner/prompt_test.go templates/prompt.md
git commit -m "feat(runner): add InjectedContext to prompt rendering"
```

---

### Task 2: Create Strategy struct with Action types

**Files:**
- Create: `internal/runner/strategy.go`
- Create: `internal/runner/strategy_test.go`

**Step 1: Write the failing test for NewStrategy and basic state**

```go
// internal/runner/strategy_test.go
package runner

import (
	"testing"

	"github.com/lofari/golem/internal/ctx"
)

func TestNewStrategy(t *testing.T) {
	s := NewStrategy()
	if s == nil {
		t.Fatal("NewStrategy returned nil")
	}
	d := s.Evaluate(ctx.State{}, ctx.Log{}, "")
	if d.Action != ActionContinue {
		t.Errorf("empty state should return Continue, got %v", d.Action)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/runner/ -run TestNewStrategy -v`
Expected: FAIL — `NewStrategy` undefined

**Step 3: Write minimal implementation**

```go
// internal/runner/strategy.go
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
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/runner/ -run TestNewStrategy -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/runner/strategy.go internal/runner/strategy_test.go
git commit -m "feat(runner): add Strategy struct with Action types"
```

---

### Task 3: Implement Rule 1 — consecutive failure with context injection

**Files:**
- Modify: `internal/runner/strategy.go`
- Modify: `internal/runner/strategy_test.go`

**Step 1: Write failing tests**

Add to `strategy_test.go`:

```go
func TestStrategy_FirstFailureRetries(t *testing.T) {
	s := NewStrategy()
	log := ctx.Log{Sessions: []ctx.Session{
		{Task: "auth", Outcome: "blocked", Summary: "jwt library not found"},
	}}
	state := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks:   []ctx.Task{{Name: "auth", Status: "todo"}},
	}

	d := s.Evaluate(state, log, "error: could not resolve jwt")
	if d.Action != ActionRetry {
		t.Errorf("first failure should retry, got %v", d.Action)
	}
	if !strings.Contains(d.InjectContext, "auth") {
		t.Error("inject context should mention the failed task")
	}
	if !strings.Contains(d.InjectContext, "jwt library not found") {
		t.Error("inject context should include the summary")
	}
}

func TestStrategy_SecondFailureSkips(t *testing.T) {
	s := NewStrategy()
	state := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks:   []ctx.Task{{Name: "auth", Status: "todo"}},
	}

	// First failure
	log1 := ctx.Log{Sessions: []ctx.Session{
		{Task: "auth", Outcome: "blocked", Summary: "jwt not found"},
	}}
	s.Evaluate(state, log1, "")

	// Second failure
	log2 := ctx.Log{Sessions: []ctx.Session{
		{Task: "auth", Outcome: "blocked", Summary: "jwt not found"},
		{Task: "auth", Outcome: "blocked", Summary: "still can't find jwt"},
	}}
	d := s.Evaluate(state, log2, "")
	if d.Action != ActionSkip {
		t.Errorf("second failure should skip, got %v", d.Action)
	}
	if len(d.SkipTasks) != 1 || d.SkipTasks[0] != "auth" {
		t.Errorf("should skip 'auth', got %v", d.SkipTasks)
	}
}

func TestStrategy_SuccessResetsFailureCount(t *testing.T) {
	s := NewStrategy()
	state := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks:   []ctx.Task{{Name: "auth", Status: "todo"}},
	}

	// One failure
	log1 := ctx.Log{Sessions: []ctx.Session{
		{Task: "auth", Outcome: "blocked"},
	}}
	s.Evaluate(state, log1, "")

	// Then success on same task
	log2 := ctx.Log{Sessions: []ctx.Session{
		{Task: "auth", Outcome: "blocked"},
		{Task: "auth", Outcome: "done"},
	}}
	s.Evaluate(state, log2, "")

	// Another failure should be treated as first
	log3 := ctx.Log{Sessions: []ctx.Session{
		{Task: "auth", Outcome: "blocked"},
		{Task: "auth", Outcome: "done"},
		{Task: "auth", Outcome: "blocked"},
	}}
	d := s.Evaluate(state, log3, "")
	if d.Action != ActionRetry {
		t.Errorf("after success reset, next failure should retry, got %v", d.Action)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/runner/ -run TestStrategy_ -v`
Expected: FAIL — `evaluateFailure` method not implemented

**Step 3: Implement evaluateFailure**

Add to `strategy.go`:

```go
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
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/runner/ -run TestStrategy_ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/runner/strategy.go internal/runner/strategy_test.go
git commit -m "feat(runner): implement failure detection with retry and skip"
```

---

### Task 4: Implement Rule 2 — dependency deadlock detection

**Files:**
- Modify: `internal/runner/strategy.go`
- Modify: `internal/runner/strategy_test.go`

**Step 1: Write failing tests**

```go
func TestStrategy_DeadlockHalts(t *testing.T) {
	s := NewStrategy()
	state := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks: []ctx.Task{
			{Name: "auth", Status: "blocked", BlockedReason: "stuck"},
			{Name: "api", Status: "todo", DependsOn: ctx.FlexString{"auth"}},
			{Name: "ui", Status: "todo", DependsOn: ctx.FlexString{"api"}},
		},
	}
	log := ctx.Log{Sessions: []ctx.Session{{Task: "auth", Outcome: "done"}}}

	d := s.Evaluate(state, log, "")
	if d.Action != ActionHalt {
		t.Errorf("all tasks blocked by deps should halt, got %v", d.Action)
	}
}

func TestStrategy_NoDeadlockWhenActionable(t *testing.T) {
	s := NewStrategy()
	state := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks: []ctx.Task{
			{Name: "auth", Status: "blocked", BlockedReason: "stuck"},
			{Name: "api", Status: "todo", DependsOn: ctx.FlexString{"auth"}},
			{Name: "docs", Status: "todo"}, // no dependency — actionable
		},
	}
	log := ctx.Log{Sessions: []ctx.Session{{Task: "auth", Outcome: "done"}}}

	d := s.Evaluate(state, log, "")
	if d.Action == ActionHalt {
		t.Error("should not halt when an actionable task exists")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/runner/ -run TestStrategy_Deadlock -v`
Expected: FAIL — `evaluateDeadlock` not implemented

**Step 3: Implement evaluateDeadlock**

```go
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
```

**Step 4: Run tests**

Run: `go test ./internal/runner/ -run TestStrategy_Deadlock -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/runner/strategy.go internal/runner/strategy_test.go
git commit -m "feat(runner): implement dependency deadlock detection"
```

---

### Task 5: Implement Rule 3 — thrashing detection (replaces existing)

**Files:**
- Modify: `internal/runner/strategy.go`
- Modify: `internal/runner/strategy_test.go`

**Step 1: Write failing tests**

```go
func TestStrategy_ThrashingSkips(t *testing.T) {
	s := NewStrategy()
	state := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks:   []ctx.Task{{Name: "payment", Status: "in-progress"}},
	}
	log := ctx.Log{Sessions: []ctx.Session{
		{Task: "payment", Outcome: "partial"},
		{Task: "payment", Outcome: "partial"},
		{Task: "payment", Outcome: "partial"},
	}}

	d := s.Evaluate(state, log, "")
	if d.Action != ActionSkip {
		t.Errorf("3 consecutive same task should skip, got %v", d.Action)
	}
	if len(d.SkipTasks) != 1 || d.SkipTasks[0] != "payment" {
		t.Errorf("should skip 'payment', got %v", d.SkipTasks)
	}
}

func TestStrategy_NoThrashingDifferentTasks(t *testing.T) {
	s := NewStrategy()
	state := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks:   []ctx.Task{{Name: "a", Status: "todo"}, {Name: "b", Status: "todo"}},
	}
	log := ctx.Log{Sessions: []ctx.Session{
		{Task: "a", Outcome: "partial"},
		{Task: "b", Outcome: "partial"},
		{Task: "a", Outcome: "partial"},
	}}

	d := s.Evaluate(state, log, "")
	if d.Action == ActionSkip {
		t.Error("different tasks should not trigger thrashing")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/runner/ -run TestStrategy_Thrashing -v`
Expected: FAIL — `evaluateThrashing` not implemented

**Step 3: Implement evaluateThrashing**

```go
func (s *Strategy) evaluateThrashing(log golemctx.Log) Decision {
	if len(log.Sessions) < 3 {
		return Decision{Action: ActionContinue}
	}

	last3 := log.Sessions[len(log.Sessions)-3:]
	task := last3[0].Task
	if task == "" {
		return Decision{Action: ActionContinue}
	}
	if last3[1].Task != task || last3[2].Task != task {
		return Decision{Action: ActionContinue}
	}

	return Decision{
		Action:    ActionSkip,
		SkipTasks: []string{task},
		InjectContext: fmt.Sprintf(
			"## Strategy Override\nTask %q has been attempted for 3 consecutive iterations without completion. It will be skipped. Work on a different task.\n",
			task,
		),
	}
}
```

**Step 4: Run tests**

Run: `go test ./internal/runner/ -run TestStrategy_Thrashing -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/runner/strategy.go internal/runner/strategy_test.go
git commit -m "feat(runner): implement thrashing detection in strategy"
```

---

### Task 6: Implement Rule 4 — no-progress detection

**Files:**
- Modify: `internal/runner/strategy.go`
- Modify: `internal/runner/strategy_test.go`

**Step 1: Write failing tests**

```go
func TestStrategy_UnproductiveWarns(t *testing.T) {
	s := NewStrategy()
	state := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks:   []ctx.Task{{Name: "task1", Status: "todo"}},
	}
	log := ctx.Log{Sessions: []ctx.Session{
		{Task: "task1", Outcome: "unproductive"},
		{Task: "task1", Outcome: "unproductive"},
	}}

	d := s.Evaluate(state, log, "")
	// 2 unproductive — should inject context but not halt
	if d.Action == ActionHalt {
		t.Error("2 unproductive should not halt yet")
	}
	if d.InjectContext == "" {
		t.Error("2 unproductive should inject warning context")
	}
}

func TestStrategy_ThreeUnproductiveHalts(t *testing.T) {
	s := NewStrategy()
	state := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks:   []ctx.Task{{Name: "task1", Status: "todo"}},
	}

	// Simulate 3 unproductive evaluations
	for i := 0; i < 3; i++ {
		log := ctx.Log{Sessions: []ctx.Session{
			{Task: "task1", Outcome: "unproductive"},
		}}
		d := s.Evaluate(state, log, "")
		if i < 2 && d.Action == ActionHalt {
			t.Errorf("should not halt on unproductive iteration %d", i+1)
		}
		if i == 2 && d.Action != ActionHalt {
			t.Errorf("should halt on 3rd unproductive iteration, got %v", d.Action)
		}
	}
}

func TestStrategy_ProductiveResetsUnproductive(t *testing.T) {
	s := NewStrategy()
	state := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks:   []ctx.Task{{Name: "task1", Status: "todo"}},
	}

	// 2 unproductive
	for i := 0; i < 2; i++ {
		log := ctx.Log{Sessions: []ctx.Session{{Task: "task1", Outcome: "unproductive"}}}
		s.Evaluate(state, log, "")
	}

	// Productive iteration
	log := ctx.Log{Sessions: []ctx.Session{{Task: "task1", Outcome: "done"}}}
	s.Evaluate(state, log, "")

	// Next unproductive should be count 1 again, not halt
	log2 := ctx.Log{Sessions: []ctx.Session{{Task: "task1", Outcome: "unproductive"}}}
	d := s.Evaluate(state, log2, "")
	if d.Action == ActionHalt {
		t.Error("productive iteration should reset unproductive counter")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/runner/ -run TestStrategy_Unproductive -v`
Expected: FAIL — `evaluateProgress` not implemented

**Step 3: Implement evaluateProgress**

```go
const (
	maxUnproductiveWarn = 2
	maxUnproductiveHalt = 3
)

func (s *Strategy) evaluateProgress(last golemctx.Session, sessionOutput string) Decision {
	if last.Outcome == "unproductive" {
		s.unproductiveCount++
	} else {
		s.unproductiveCount = 0
		return Decision{Action: ActionContinue}
	}

	if s.unproductiveCount >= maxUnproductiveHalt {
		return Decision{
			Action:     ActionHalt,
			HaltReason: fmt.Sprintf("%d consecutive unproductive iterations", s.unproductiveCount),
		}
	}

	if s.unproductiveCount >= maxUnproductiveWarn {
		return Decision{
			Action: ActionRetry,
			InjectContext: fmt.Sprintf(
				"## Warning\nThe last %d iterations produced no meaningful progress. Focus on making concrete, testable changes. If you are stuck, consider working on a different task.\n",
				s.unproductiveCount,
			),
		}
	}

	return Decision{Action: ActionContinue}
}
```

**Step 4: Run tests**

Run: `go test ./internal/runner/ -run TestStrategy_Unproductive -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/runner/strategy.go internal/runner/strategy_test.go
git commit -m "feat(runner): implement no-progress detection with halt"
```

---

### Task 7: Wire Strategy into RunBuilderLoop

**Files:**
- Modify: `internal/runner/builder.go:46-271`
- Modify: `internal/runner/builder_test.go`

**Step 1: Write failing test for strategy integration**

Add to `builder_test.go`:

```go
func TestBuilderLoop_SkipsStuckTask(t *testing.T) {
	dir := setupTestProject(t)

	// Set up state with two tasks
	state := golemctx.State{
		Project: golemctx.Project{Name: "test", DocsPath: "docs/"},
		Status:  golemctx.Status{Phase: "building"},
		Tasks: []golemctx.Task{
			{Name: "stuck-task", Status: "todo"},
			{Name: "good-task", Status: "todo"},
		},
	}
	golemctx.WriteState(dir, state)

	// Pre-seed log with 2 failures on stuck-task to trigger skip
	golemctx.WriteLog(dir, golemctx.Log{Sessions: []golemctx.Session{
		{Iteration: 1, Task: "stuck-task", Outcome: "blocked", Summary: "failed once"},
		{Iteration: 2, Task: "stuck-task", Outcome: "blocked", Summary: "failed twice"},
	}})

	mock := &mockRunner{outputs: []string{"done <promise>COMPLETE</promise>"}}

	result, err := RunBuilderLoop(context.Background(), BuilderConfig{
		Dir:           dir,
		MaxIterations: 5,
		MaxToolCalls:  10,
		Runner:        mock,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify stuck-task was marked blocked
	finalState, _ := golemctx.ReadState(dir)
	for _, task := range finalState.Tasks {
		if task.Name == "stuck-task" && task.Status != "blocked" {
			t.Errorf("stuck-task should be blocked, got %q", task.Status)
		}
	}
	_ = result
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/runner/ -run TestBuilderLoop_SkipsStuckTask -v`
Expected: FAIL — strategy not wired in

**Step 3: Integrate Strategy into RunBuilderLoop**

In `builder.go`, make these changes:

1. Create strategy at the start of the loop:

```go
func RunBuilderLoop(ctx context.Context, cfg BuilderConfig) (BuilderResult, error) {
	startTime := time.Now()
	var result BuilderResult
	strategy := NewStrategy()
	// ... rest of existing init code
```

2. After validation (around line 234), before rendering the next prompt, add strategy evaluation:

```go
		// --- Strategy evaluation (after validation, before next iteration) ---
		decision := strategy.Evaluate(stateAfter, log, output)

		// Apply skip decisions
		for _, taskName := range decision.SkipTasks {
			for j := range stateAfter.Tasks {
				if stateAfter.Tasks[j].Name == taskName && stateAfter.Tasks[j].Status != "done" {
					stateAfter.Tasks[j].Status = "blocked"
					if stateAfter.Tasks[j].BlockedReason == "" {
						stateAfter.Tasks[j].BlockedReason = "auto-skipped by strategy after repeated failures"
					}
					fmt.Fprintf(os.Stderr, "golem: strategy: skipping task %q (marking blocked)\n", taskName)
				}
			}
			golemctx.WriteState(cfg.Dir, stateAfter)
		}

		switch decision.Action {
		case ActionHalt:
			result.Halted = true
			result.HaltReason = "strategy: " + decision.HaltReason
			result.Iterations = i
			fmt.Fprintf(os.Stderr, "golem: strategy: halting — %s\n", decision.HaltReason)
			break Loop
		case ActionRetry, ActionSkip:
			// Store injected context for next iteration
			cfg.injectedContext = decision.InjectContext
		}
```

3. Add `injectedContext` field to BuilderConfig (unexported, internal use only — or use a local var in the loop). A local var is cleaner:

Declare `var injectedContext string` at the top of the function alongside `var result BuilderResult`, and use it in the prompt rendering:

```go
		prompt, err := RenderPrompt(cfg.Dir, templateFile, PromptVars{
			DocsPath:         state.Project.DocsPath,
			IterationContext: iterCtx,
			TaskOverride:     taskOverride,
			ReviewContext:    reviewCtx,
			InjectedContext:  injectedContext,
		})
```

Reset `injectedContext` at the start of each iteration, and set it from the strategy decision:

```go
		// Reset injected context (set by strategy at end of previous iteration)
		// injectedContext is already set from the previous iteration's strategy decision
```

Actually, the cleanest approach: declare `injectedContext` before the loop, set it from `decision.InjectContext` after strategy evaluation, and it naturally gets consumed in the next iteration's `RenderPrompt` call, then cleared:

```go
	var injectedContext string

Loop:
	for i := 1; i <= cfg.MaxIterations; i++ {
		// ... existing code ...

		prompt, err := RenderPrompt(cfg.Dir, templateFile, PromptVars{
			DocsPath:         state.Project.DocsPath,
			IterationContext: iterCtx,
			TaskOverride:     taskOverride,
			ReviewContext:    reviewCtx,
			InjectedContext:  injectedContext,
		})
		// Clear after use — it's consumed
		injectedContext = ""

		// ... existing iteration code ...

		// At end of iteration, after validation:
		decision := strategy.Evaluate(stateAfter, log, output)
		// ... apply decision, set injectedContext = decision.InjectContext ...
	}
```

**Step 4: Run all tests**

Run: `go test ./internal/runner/ -v`
Expected: PASS (all existing + new tests)

**Step 5: Commit**

```bash
git add internal/runner/builder.go internal/runner/builder_test.go
git commit -m "feat(runner): wire strategy into builder loop"
```

---

### Task 8: Remove detectThrashing from validate.go

**Files:**
- Modify: `internal/runner/validate.go:92-97`
- Modify: `internal/runner/validate_test.go:119-150`

**Step 1: Remove thrashing from validation**

In `validate.go`, remove the thrashing detection block (lines 92-97):

```go
	// 4. Thrashing detection  <-- DELETE THIS BLOCK
	thrashing := detectThrashing(log)
	for _, taskName := range thrashing {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("WARNING — task %q has been in-progress for 3+ consecutive iterations", taskName))
	}
```

Also remove the `detectThrashing` function (lines 104-119).

**Step 2: Remove detectThrashing tests**

In `validate_test.go`, remove `TestDetectThrashing` (lines 119-150).

**Step 3: Run all tests**

Run: `go test ./internal/runner/ -v`
Expected: PASS

**Step 4: Run full test suite**

Run: `go test ./...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/runner/validate.go internal/runner/validate_test.go
git commit -m "refactor(runner): remove detectThrashing from validate (moved to strategy)"
```

---

### Task 9: Update embedded prompt template

**Files:**
- Modify: `templates/prompt.md`

**Step 1: Add INJECTED_CONTEXT placeholder**

The template currently starts with:

```
You are working on this project autonomously as part of a loop.
Each iteration you get fresh context — you have no memory of previous iterations.
All persistent state is in `.ctx/`.

{{ITERATION_CONTEXT}}
```

Add `{{INJECTED_CONTEXT}}` after `{{ITERATION_CONTEXT}}`:

```
{{ITERATION_CONTEXT}}

{{INJECTED_CONTEXT}}
```

When `InjectedContext` is empty, this just renders as a blank line (no visible change).

**Step 2: Run scaffold test to make sure template still embeds**

Run: `go build ./...`
Expected: builds clean

**Step 3: Run full tests**

Run: `go test ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add templates/prompt.md
git commit -m "feat(templates): add INJECTED_CONTEXT placeholder to prompt"
```

---

### Task 10: End-to-end verification

**Step 1: Build**

Run: `go build ./...`
Expected: clean build

**Step 2: Run full test suite**

Run: `go test ./... -v`
Expected: all PASS

**Step 3: Dry-run smoke test**

Run from a golem-initialized project:

```bash
go run . code --dry-run --max-iterations 1
```

Expected: renders prompt with empty `{{INJECTED_CONTEXT}}` (no visible change from before)

**Step 4: Commit (if any fixups needed)**

```bash
git add -A
git commit -m "test: end-to-end verification of adaptive iteration engine"
```
