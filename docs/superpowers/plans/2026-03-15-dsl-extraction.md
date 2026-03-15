# DSL Extraction & Removal Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract custom predicates and error classification from the Clojure DSL into the Go blueprint engine, then remove the DSL runtime.

**Architecture:** Three independent workstreams: (1) expression-based custom predicates parsed from blueprint YAML, (2) error handler priority chain with prompt amendment on retry, (3) removal of all Clojure DSL code and Go integration. Workstreams 1 and 2 can run in parallel. Workstream 3 depends on both completing.

**Tech Stack:** Go stdlib (testing, strings, strconv, fmt), YAML parsing (gopkg.in/yaml.v3)

**Spec:** `docs/superpowers/specs/2026-03-15-dsl-extraction-design.md`

---

## Chunk 1: Custom Predicates

### Task 1: Predicate Expression Parser

**Files:**
- Create: `internal/runner/predicate_expr.go`
- Create: `internal/runner/predicate_expr_test.go`

- [ ] **Step 1: Write failing tests for expression parsing**

```go
// internal/runner/predicate_expr_test.go
package runner

import "testing"

func TestParsePredicateExpr_Valid(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"string equality", `review-feedback.verdict == "needs-work"`},
		{"numeric greater", `test-results.coverage > 80`},
		{"boolean equality", `config.ci-enabled == true`},
		{"not equal string", `test-results.status != "fail"`},
		{"numeric less-equal", `test-results.coverage <= 100`},
		{"float comparison", `metrics.score >= 3.14`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := ParsePredicateExpr(tt.expr)
			if err != nil {
				t.Fatalf("ParsePredicateExpr(%q) error: %v", tt.expr, err)
			}
			if expr == nil {
				t.Fatal("expected non-nil expr")
			}
		})
	}
}

func TestParsePredicateExpr_Invalid(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"empty", ""},
		{"no operator", "review-feedback.verdict"},
		{"bad operator", `review-feedback.verdict ~~ "x"`},
		{"missing value", `review-feedback.verdict ==`},
		{"missing path", `== "value"`},
		{"unquoted string", `review-feedback.verdict == needs-work`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePredicateExpr(tt.expr)
			if err == nil {
				t.Fatalf("ParsePredicateExpr(%q) should error", tt.expr)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestParsePredicateExpr -v`
Expected: FAIL — `ParsePredicateExpr` undefined

- [ ] **Step 3: Implement the expression parser**

```go
// internal/runner/predicate_expr.go
package runner

import (
	"fmt"
	"strconv"
	"strings"
)

// PredicateExpr represents a parsed predicate expression: path op value.
type PredicateExpr struct {
	Path     string   // dotted path, e.g. "review-feedback.verdict"
	Op       string   // ==, !=, >, <, >=, <=
	Value    any      // string, float64, or bool
	IsConfig bool     // true if path starts with "config."
	Segments []string // path split by "."
}

var validOps = map[string]bool{
	"==": true, "!=": true,
	">": true, "<": true,
	">=": true, "<=": true,
}

// ParsePredicateExpr parses an expression like: path.to.key == "value"
func ParsePredicateExpr(expr string) (*PredicateExpr, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("empty predicate expression")
	}

	// Find the operator
	var op string
	var opIdx int
	for _, candidate := range []string{">=", "<=", "!=", "==", ">", "<"} {
		idx := strings.Index(expr, candidate)
		if idx > 0 {
			op = candidate
			opIdx = idx
			break
		}
	}
	if op == "" {
		return nil, fmt.Errorf("no operator found in %q (expected ==, !=, >, <, >=, <=)", expr)
	}

	path := strings.TrimSpace(expr[:opIdx])
	rawVal := strings.TrimSpace(expr[opIdx+len(op):])

	if path == "" {
		return nil, fmt.Errorf("missing path in %q", expr)
	}
	if rawVal == "" {
		return nil, fmt.Errorf("missing value in %q", expr)
	}

	// Parse the right-hand value
	val, err := parseValue(rawVal)
	if err != nil {
		return nil, fmt.Errorf("invalid value in %q: %w", expr, err)
	}

	p := &PredicateExpr{
		Path:  path,
		Op:    op,
		Value: val,
	}

	if strings.HasPrefix(path, "config.") {
		p.IsConfig = true
		p.Segments = strings.Split(path[len("config."):], ".")
	} else {
		p.Segments = strings.Split(path, ".")
	}

	return p, nil
}

func parseValue(raw string) (any, error) {
	if raw == "true" {
		return true, nil
	}
	if raw == "false" {
		return false, nil
	}
	if strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`) && len(raw) >= 2 {
		return raw[1 : len(raw)-1], nil
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return f, nil
	}
	return nil, fmt.Errorf("unrecognized value %q (use quoted strings, numbers, or true/false)", raw)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestParsePredicateExpr -v`
Expected: PASS

- [ ] **Step 5: Write failing tests for expression evaluation**

Add to `internal/runner/predicate_expr_test.go`:

```go
func TestPredicateExpr_Eval(t *testing.T) {
	state := map[string]any{
		"review-feedback": map[string]any{"verdict": "needs-work"},
		"test-results":    map[string]any{"status": "fail", "coverage": float64(85)},
	}
	config := map[string]any{"ci-enabled": true}

	tests := []struct {
		name string
		expr string
		want bool
	}{
		{"string match", `review-feedback.verdict == "needs-work"`, true},
		{"string mismatch", `review-feedback.verdict == "approved"`, false},
		{"string not-equal", `review-feedback.verdict != "approved"`, true},
		{"numeric greater true", `test-results.coverage > 80`, true},
		{"numeric greater false", `test-results.coverage > 90`, false},
		{"numeric less-equal", `test-results.coverage <= 85`, true},
		{"config bool", `config.ci-enabled == true`, true},
		{"config bool false", `config.ci-enabled == false`, false},
		{"missing path", `nonexistent.key == "x"`, false},
		{"missing nested", `review-feedback.nonexistent == "x"`, false},
		{"type mismatch num vs string", `test-results.coverage == "85"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := ParsePredicateExpr(tt.expr)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			got := expr.Eval(state, config)
			if got != tt.want {
				t.Errorf("Eval(%q) = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestPredicateExpr_Eval -v`
Expected: FAIL — `Eval` method undefined

- [ ] **Step 7: Implement the Eval method**

Add to `internal/runner/predicate_expr.go`:

```go
// Eval evaluates the predicate expression against state and config maps.
// Returns false for missing paths or type mismatches.
func (p *PredicateExpr) Eval(state, config map[string]any) bool {
	var resolved any
	if p.IsConfig {
		resolved = resolvePath(config, p.Segments)
	} else {
		resolved = resolvePath(state, p.Segments)
	}
	if resolved == nil {
		return false
	}
	return compare(resolved, p.Op, p.Value)
}

// resolvePath walks a dotted path through nested maps.
func resolvePath(m map[string]any, segments []string) any {
	if m == nil || len(segments) == 0 {
		return nil
	}
	val, ok := m[segments[0]]
	if !ok || val == nil {
		return nil
	}
	if len(segments) == 1 {
		return val
	}
	nested, ok := val.(map[string]any)
	if !ok {
		return nil
	}
	return resolvePath(nested, segments[1:])
}

// compare performs typed comparison. Returns false on type mismatch.
func compare(left any, op string, right any) bool {
	switch rv := right.(type) {
	case string:
		lv, ok := left.(string)
		if !ok {
			return false
		}
		return compareString(lv, op, rv)
	case bool:
		lv, ok := left.(bool)
		if !ok {
			return false
		}
		if op == "==" {
			return lv == rv
		}
		if op == "!=" {
			return lv != rv
		}
		return false
	case float64:
		lf := toFloat64(left)
		if lf == nil {
			return false
		}
		return compareFloat(*lf, op, rv)
	}
	return false
}

func compareString(a, op, b string) bool {
	switch op {
	case "==":
		return a == b
	case "!=":
		return a != b
	case ">":
		return a > b
	case "<":
		return a < b
	case ">=":
		return a >= b
	case "<=":
		return a <= b
	}
	return false
}

func compareFloat(a float64, op string, b float64) bool {
	switch op {
	case "==":
		return a == b
	case "!=":
		return a != b
	case ">":
		return a > b
	case "<":
		return a < b
	case ">=":
		return a >= b
	case "<=":
		return a <= b
	}
	return false
}

func toFloat64(v any) *float64 {
	switch n := v.(type) {
	case float64:
		return &n
	case int:
		f := float64(n)
		return &f
	case int64:
		f := float64(n)
		return &f
	}
	return nil
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestPredicateExpr -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add internal/runner/predicate_expr.go internal/runner/predicate_expr_test.go
git commit -m "feat(runner): add predicate expression parser and evaluator"
```

---

### Task 2: Wire Custom Predicates into Blueprint + Engine

**Files:**
- Modify: `internal/runner/blueprint.go` (add `Predicates` field, parse, validate)
- Modify: `internal/runner/predicates.go` (refactor to Engine method)
- Modify: `internal/runner/predicates_test.go` (update calls)
- Modify: `internal/runner/engine.go` (update 3 call sites)

- [ ] **Step 1: Write failing test for blueprint predicate parsing**

Add to `internal/runner/blueprint_test.go`:

```go
func TestParseBlueprint_CustomPredicates(t *testing.T) {
	data := []byte(`
name: test
initial-state: [goal]
predicates:
  custom-pred: test-results.status == "fail"
  high-coverage: test-results.coverage > 80
steps:
  - plan:
      type: agentic
      reads: [goal]
      writes: [plan]
`)
	bp, err := ParseBlueprint(data)
	if err != nil {
		t.Fatalf("ParseBlueprint error: %v", err)
	}
	if len(bp.Predicates) != 2 {
		t.Fatalf("expected 2 predicates, got %d", len(bp.Predicates))
	}
	if bp.Predicates["custom-pred"] != `test-results.status == "fail"` {
		t.Errorf("unexpected predicate expr: %s", bp.Predicates["custom-pred"])
	}
}

func TestParseBlueprint_InvalidPredicateExpr(t *testing.T) {
	data := []byte(`
name: test
initial-state: [goal]
predicates:
  bad-pred: no-operator-here
steps:
  - plan:
      type: agentic
      reads: [goal]
      writes: [plan]
`)
	_, err := ParseBlueprint(data)
	if err == nil {
		t.Fatal("expected error for invalid predicate expression")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestParseBlueprint_CustomPredicate -v`
Expected: FAIL — `Predicates` field doesn't exist or `predicates` is unknown field

- [ ] **Step 3: Add Predicates field to Blueprint and parse from YAML**

In `internal/runner/blueprint.go`:

Add `Predicates` field to `Blueprint` struct:
```go
type Blueprint struct {
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description"`
	InitialState []string          `yaml:"initial-state"`
	Config       map[string]any    `yaml:"config"`
	Predicates   map[string]string `yaml:"predicates"`
	Steps        []Step            `yaml:"-"`
	Errors       ErrorHandlers     `yaml:"errors"`
	pipeline     *Pipeline
}
```

Add `"predicates"` to `validTopLevelFields`:
```go
var validTopLevelFields = map[string]bool{
	"name":          true,
	"description":   true,
	"initial-state": true,
	"config":        true,
	"predicates":    true,
	"steps":         true,
	"errors":        true,
}
```

Add `parsedPredicates` field to `Blueprint` (unexported, populated at parse time):
```go
parsedPredicates map[string]*PredicateExpr
```

Add predicate validation in `ParseBlueprint` after error handler validation. This parses and caches all expressions to avoid re-parsing during while loop evaluation:
```go
// Validate and cache predicate expressions
parsed, err := parsePredicates(bp.Predicates)
if err != nil {
	return nil, err
}
bp.parsedPredicates = parsed
```

Add the parse+validate function:
```go
func parsePredicates(preds map[string]string) (map[string]*PredicateExpr, error) {
	if len(preds) == 0 {
		return nil, nil
	}
	result := make(map[string]*PredicateExpr, len(preds))
	for name, expr := range preds {
		parsed, err := ParsePredicateExpr(expr)
		if err != nil {
			return nil, fmt.Errorf("blueprint: predicate %q: %w", name, err)
		}
		result[name] = parsed
	}
	return result, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestParseBlueprint_CustomPredicate -v`
Expected: PASS

- [ ] **Step 5: Refactor EvalPredicate to Engine method**

In `internal/runner/predicates.go`, rename `EvalPredicate` to `evalBuiltinPredicate` (unexported) and change return type to `(bool, bool)` where the second bool indicates whether it was a known built-in:

```go
func evalBuiltinPredicate(name string, state map[string]any, config map[string]any) (bool, bool) {
	switch name {
	case "needs-work":
		return getNestedString(state, "review-feedback", "verdict") == "needs-work", true
	case "failed":
		return getNestedString(state, "test-results", "status") == "fail", true
	case "lint-failed":
		return getNestedString(state, "lint-results", "status") == "fail", true
	case "ci-enabled":
		if config == nil {
			return false, true
		}
		v, ok := config["ci-enabled"]
		if !ok {
			return false, true
		}
		b, ok := v.(bool)
		return ok && b, true
	case "ci-failed":
		return getNestedString(state, "ci-results", "status") == "fail", true
	default:
		return false, false
	}
}
```

In `internal/runner/engine.go`, add the Engine method:

```go
// evalPredicate evaluates a predicate by name. Checks custom predicates first, then built-ins.
func (e *Engine) evalPredicate(name string, state map[string]any, config map[string]any) bool {
	// Check custom predicates from blueprint (use cached parsed exprs)
	if e.cfg.Blueprint != nil && e.cfg.Blueprint.parsedPredicates != nil {
		if expr, ok := e.cfg.Blueprint.parsedPredicates[name]; ok {
			return expr.Eval(state, config)
		}
	}
	// Fall back to built-ins
	result, _ := evalBuiltinPredicate(name, state, config)
	return result
}
```

Update the 3 call sites in `engine.go`:
- `execWhile` line 444: `EvalPredicate(cf.Predicate, e.state, e.cfg.Config)` → `e.evalPredicate(cf.Predicate, e.state, e.cfg.Config)`
- `execWhen` line 459: same change
- `execIf` line 467: same change

- [ ] **Step 6: Update predicates_test.go to use new function signature**

Update all `EvalPredicate(...)` calls in `predicates_test.go` to `evalBuiltinPredicate(...)` and check the second return:

```go
func TestPredicate_NeedsWork(t *testing.T) {
	tests := []struct {
		name   string
		state  map[string]any
		config map[string]any
		want   bool
	}{
		{"missing key", map[string]any{}, nil, false},
		{"approved", map[string]any{"review-feedback": map[string]any{"verdict": "approved"}}, nil, false},
		{"needs-work", map[string]any{"review-feedback": map[string]any{"verdict": "needs-work"}}, nil, true},
		{"nil value", map[string]any{"review-feedback": nil}, nil, false},
		{"wrong type", map[string]any{"review-feedback": "string"}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := evalBuiltinPredicate("needs-work", tt.state, tt.config)
			if !found {
				t.Fatal("needs-work should be a known predicate")
			}
			if got != tt.want {
				t.Errorf("evalBuiltinPredicate(needs-work) = %v, want %v", got, tt.want)
			}
		})
	}
}
```

Apply same pattern to `TestPredicate_Failed`, `TestPredicate_CIEnabled`.

Update `TestPredicate_Unknown`:
```go
func TestPredicate_Unknown(t *testing.T) {
	_, found := evalBuiltinPredicate("nonexistent", nil, nil)
	if found {
		t.Error("unknown predicate should return found=false")
	}
}
```

- [ ] **Step 7: Write test for Engine.evalPredicate with custom predicate**

Add to `internal/runner/engine_test.go`:

```go
func TestEngine_EvalPredicate_Custom(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	bp := &Blueprint{
		Predicates: map[string]string{
			"custom-check": `test-results.status == "fail"`,
		},
		pipeline: &Pipeline{StepDefs: map[string]*Step{}},
	}
	e := NewEngine(EngineConfig{Dir: dir, AgentName: "test", Goal: "test", Blueprint: bp})

	state := map[string]any{"test-results": map[string]any{"status": "fail"}}
	if !e.evalPredicate("custom-check", state, nil) {
		t.Error("custom predicate should match")
	}
	if e.evalPredicate("custom-check", map[string]any{}, nil) {
		t.Error("custom predicate should not match on empty state")
	}
	// Built-in still works
	if !e.evalPredicate("needs-work", map[string]any{"review-feedback": map[string]any{"verdict": "needs-work"}}, nil) {
		t.Error("built-in predicate should still work")
	}
}
```

- [ ] **Step 8: Run all predicate and engine tests**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run "TestPredicate|TestEngine_EvalPredicate" -v`
Expected: PASS

- [ ] **Step 9: Run full test suite to check nothing is broken**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -v`
Expected: PASS (all existing tests still pass)

- [ ] **Step 10: Commit**

```bash
git add internal/runner/blueprint.go internal/runner/predicates.go internal/runner/predicates_test.go internal/runner/engine.go internal/runner/engine_test.go
git commit -m "feat(runner): wire custom predicates into blueprint parsing and engine evaluation"
```

---

## Chunk 2: Error Classification with Prompt Amendment

### Task 3: Expand StepErrors and Add Handler Priority Chain

**Files:**
- Modify: `internal/runner/blueprint.go` (expand `StepErrors` struct)
- Modify: `internal/runner/engine.go` (handler priority chain + error context injection)
- Modify: `internal/runner/engine_test.go` (tests)

- [ ] **Step 1: Write failing test for per-step error handler resolution**

Add to `internal/runner/engine_test.go`:

```go
func TestResolveHandler_Priority(t *testing.T) {
	stepHandler := ErrorHandler{Action: "retry", Max: 5}
	bpHandler := ErrorHandler{Action: "retry", Max: 2}

	// Step-level wins over blueprint-level
	got := resolveErrorHandler(
		&StepErrors{Transient: &stepHandler},
		&ErrorHandlers{Transient: bpHandler},
		"transient",
	)
	if got.Max != 5 {
		t.Errorf("expected step handler max=5, got %d", got.Max)
	}

	// Blueprint-level used when step has no handler
	got = resolveErrorHandler(
		nil,
		&ErrorHandlers{Transient: bpHandler},
		"transient",
	)
	if got.Max != 2 {
		t.Errorf("expected blueprint handler max=2, got %d", got.Max)
	}

	// Built-in default when neither defines handler (behavior change: previously halt, now retry)
	got = resolveErrorHandler(nil, &ErrorHandlers{}, "transient")
	if got.Action != "retry" || got.Max != 3 {
		t.Errorf("expected default retry/3, got %s/%d", got.Action, got.Max)
	}

	// Built-in default for malformed-output (behavior change: previously halt, now re-run)
	got = resolveErrorHandler(nil, &ErrorHandlers{}, "malformed-output")
	if got.Action != "re-run" || got.Max != 2 {
		t.Errorf("expected default re-run/2, got %s/%d", got.Action, got.Max)
	}

	// Unrecoverable always halts
	got = resolveErrorHandler(nil, &ErrorHandlers{}, "unrecoverable")
	if got.Action != "halt" {
		t.Errorf("expected default halt, got %s", got.Action)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestResolveHandler_Priority -v`
Expected: FAIL — `resolveErrorHandler` undefined, `StepErrors.Transient` doesn't exist

**Note:** Steps 3-8 modify interconnected function signatures. Apply all of them before running tests — the code won't compile in intermediate states.

- [ ] **Step 3: Expand StepErrors struct**

In `internal/runner/blueprint.go`, replace the `StepErrors` struct:

```go
// StepErrors holds per-step error handling configuration.
type StepErrors struct {
	NonZero         string        `yaml:"non-zero"`
	Transient       *ErrorHandler `yaml:"transient"`
	MalformedOutput *ErrorHandler `yaml:"malformed-output"`
	ContractViolation *ErrorHandler `yaml:"contract-violation"`
}
```

- [ ] **Step 4: Implement resolveErrorHandler**

Add to `internal/runner/engine.go`:

```go
// Built-in error handler defaults.
var defaultErrorHandlers = map[string]ErrorHandler{
	"transient":          {Action: "retry", Max: 3},
	"malformed-output":   {Action: "re-run", Max: 2},
	"unrecoverable":      {Action: "halt"},
	"contract-violation": {Action: "halt"},
}

// resolveErrorHandler returns the handler for a given error type using the priority chain:
// step-level > blueprint-level > built-in defaults.
func resolveErrorHandler(stepErrs *StepErrors, bpErrs *ErrorHandlers, errType string) ErrorHandler {
	// Step-level
	if stepErrs != nil {
		switch errType {
		case "transient":
			if stepErrs.Transient != nil {
				return *stepErrs.Transient
			}
		case "malformed-output":
			if stepErrs.MalformedOutput != nil {
				return *stepErrs.MalformedOutput
			}
		case "contract-violation":
			if stepErrs.ContractViolation != nil {
				return *stepErrs.ContractViolation
			}
		}
	}

	// Blueprint-level
	if bpErrs != nil {
		switch errType {
		case "transient":
			if bpErrs.Transient.Action != "" {
				return bpErrs.Transient
			}
		case "malformed-output":
			if bpErrs.MalformedOutput.Action != "" {
				return bpErrs.MalformedOutput
			}
		case "contract-violation":
			if bpErrs.ContractViolation.Action != "" {
				return bpErrs.ContractViolation
			}
		}
	}

	// Built-in default
	return defaultErrorHandlers[errType]
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestResolveHandler_Priority -v`
Expected: PASS

- [ ] **Step 6: Refactor handleError to use resolveErrorHandler**

In `internal/runner/engine.go`, replace `handleError`:

```go
func (e *Engine) handleError(ctx context.Context, step *Step, err error) error {
	var transient *TransientError
	var unrecoverable *UnrecoverableError
	var malformed *MalformedOutputError

	switch {
	case errors.As(err, &unrecoverable):
		handler := resolveErrorHandler(step.StepErrors, &e.cfg.Blueprint.Errors, "unrecoverable")
		e.emit(EngineEvent{Type: "error-occurred", Step: step.Name, ErrorType: "unrecoverable", Action: handler.Action})
		return err

	case errors.As(err, &malformed):
		handler := resolveErrorHandler(step.StepErrors, &e.cfg.Blueprint.Errors, "malformed-output")
		return e.handleMalformedOutput(ctx, step, malformed, handler)

	case errors.As(err, &transient):
		handler := resolveErrorHandler(step.StepErrors, &e.cfg.Blueprint.Errors, "transient")
		return e.handleTransient(ctx, step, transient, handler)

	default:
		handler := resolveErrorHandler(step.StepErrors, &e.cfg.Blueprint.Errors, "transient")
		return e.handleTransient(ctx, step, &TransientError{Msg: err.Error()}, handler)
	}
}
```

- [ ] **Step 7: Add error context injection to handleTransient**

Replace `handleTransient`:

```go
func (e *Engine) handleTransient(ctx context.Context, step *Step, transientErr *TransientError, handler ErrorHandler) error {
	if handler.Action != "retry" || handler.Max <= 0 {
		return fmt.Errorf("step %q failed (transient, no retry configured)", step.Name)
	}
	for attempt := 1; attempt <= handler.Max; attempt++ {
		e.emit(EngineEvent{Type: "error-retry", Step: step.Name, ErrorType: "transient", Attempt: attempt, Action: "retry"})
		e.state["_error_context"] = fmt.Sprintf("Previous error (attempt %d/%d): %s", attempt, handler.Max, transientErr.Msg)
		if err := e.runStep(ctx, step); err == nil {
			delete(e.state, "_error_context")
			return nil
		}
	}
	delete(e.state, "_error_context")
	return fmt.Errorf("step %q failed after %d retries", step.Name, handler.Max)
}
```

- [ ] **Step 8: Add error context injection to handleMalformedOutput**

Replace `handleMalformedOutput`:

```go
func (e *Engine) handleMalformedOutput(ctx context.Context, step *Step, malformed *MalformedOutputError, handler ErrorHandler) error {
	if handler.Action != "re-run" || handler.Max <= 0 {
		return fmt.Errorf("step %q: malformed output: %s", step.Name, malformed.Msg)
	}
	for attempt := 1; attempt <= handler.Max; attempt++ {
		e.emit(EngineEvent{Type: "error-retry", Step: step.Name, ErrorType: "malformed-output", Attempt: attempt, Action: "re-run"})
		errCtx := fmt.Sprintf("Previous error (attempt %d/%d): %s", attempt, handler.Max, malformed.Msg)
		if handler.Hint != "" {
			errCtx += "\nHint: " + handler.Hint
		}
		e.state["_error_context"] = errCtx
		if err := e.runStep(ctx, step); err == nil {
			delete(e.state, "_error_context")
			return nil
		}
	}
	delete(e.state, "_error_context")
	return fmt.Errorf("step %q: malformed output after %d re-runs: %s", step.Name, handler.Max, malformed.Msg)
}
```

- [ ] **Step 9: Run full engine tests**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -v`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add internal/runner/blueprint.go internal/runner/engine.go internal/runner/engine_test.go
git commit -m "feat(runner): add error handler priority chain with error context injection"
```

---

### Task 4: Add _error_context to Prompt Templates

**Files:**
- Modify: `templates/prompts/implement.md`
- Modify: `templates/prompts/plan.md`
- Modify: `templates/prompts/review.md`
- Modify: `templates/prompts/research.md`
- Modify: `templates/prompts/reflect.md`
- Modify: `templates/agents/build-feature.yaml`
- Modify: `templates/agents/fix-bug.yaml`
- Modify: `templates/agents/one-shot.yaml`

- [ ] **Step 1: Add _error_context section to each prompt template**

Append to each prompt template in `templates/prompts/` a section that will only appear on retries (the line is removed by `RenderStepPrompt` when `_error_context` is absent via optional-reads):

For each of `plan.md`, `implement.md`, `review.md`, `research.md`, `reflect.md`, add at the end:

```markdown

## Previous Error Context
${_error_context}
```

- [ ] **Step 2: Add _error_context to optional-reads in agent YAML templates**

In each agent YAML template, add `_error_context` to the `optional-reads` list of every agentic step. For example in `build-feature.yaml`:

```yaml
  - plan:
      type: agentic
      reads: [goal]
      optional-reads: [_error_context]
      writes: [plan]
```

Apply the same to all agentic steps in `build-feature.yaml`, `fix-bug.yaml`, and `one-shot.yaml`.

- [ ] **Step 3: Run full test suite**

Run: `cd /home/winler/projects/golem && go test ./... -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add templates/prompts/ templates/agents/
git commit -m "feat(templates): add _error_context to prompt templates and agent optional-reads"
```

---

## Chunk 3: Remove Clojure DSL Runtime

**Note:** Tasks 5-7 must be executed in order. Task 7 (CLI removal) must complete before Task 6 (config removal) because `cmd/code.go` references `rc.DSLCommand` which is removed from the Config struct in Task 6.

### Task 5: Delete DSL Go Integration Files

**Files:**
- Delete: `internal/runner/dsl_runner.go`
- Delete: `internal/runner/dsl_events.go`
- Delete: `internal/runner/dsl_runner_test.go`
- Delete: `internal/runner/dsl_events_test.go`
- Delete: `internal/runner/dsl_runner_mock_test.go`
- Delete: `internal/runner/dsl_golden_test.go`
- Delete: `internal/runner/testdata/mock-dsl/main.go`
- Delete: `internal/runner/testdata/golden_events.ndjson`
- Delete: `internal/runner/testdata/golden_events_halted.ndjson`
- Delete: `internal/runner/testdata/golden_events_with_retry.ndjson`
- Delete: `test/integration/dsl_integration_test.go`
- Delete: `test/integration/dsl_state_test.go`

- [ ] **Step 1: Delete DSL runner and event files**

```bash
cd /home/winler/projects/golem
rm internal/runner/dsl_runner.go internal/runner/dsl_events.go
rm internal/runner/dsl_runner_test.go internal/runner/dsl_events_test.go
rm internal/runner/dsl_runner_mock_test.go internal/runner/dsl_golden_test.go
rm internal/runner/testdata/mock-dsl/main.go
rmdir internal/runner/testdata/mock-dsl/
rm internal/runner/testdata/golden_events.ndjson
rm internal/runner/testdata/golden_events_halted.ndjson
rm internal/runner/testdata/golden_events_with_retry.ndjson
```

- [ ] **Step 2: Delete integration test files**

```bash
rm test/integration/dsl_integration_test.go test/integration/dsl_state_test.go
```

- [ ] **Step 3: Verify runner package compiles**

Run: `cd /home/winler/projects/golem && go build ./internal/runner/`
Expected: SUCCESS (DSL types were only referenced from deleted files and cmd/code.go)

- [ ] **Step 4: Commit**

```bash
git add -u internal/runner/ test/integration/
git commit -m "refactor(runner): remove DSL runner and event integration files"
```

---

### Task 6: Remove DSL from CLI Commands

**Files:**
- Modify: `cmd/code.go`
- Modify: `cmd/code_test.go`
- Modify: `cmd/helpers.go`
- Modify: `cmd/session.go`

**Note:** This task MUST run before Task 7 (config removal) because `cmd/code.go` references `rc.DSLCommand` which comes from the Config struct.

- [ ] **Step 1: Remove DSL branch from cmd/code.go**

- Remove the `if shouldUseDSL(rc.Engine)` block (lines 79-99)
- Remove the `shouldUseDSL()` function (lines 138-140)
- Remove any now-unused imports (the `runner.DSLRunner` reference)

- [ ] **Step 2: Remove TestShouldUseDSL from cmd/code_test.go**

Remove the entire `TestShouldUseDSL` function (lines 5-22).

- [ ] **Step 3: Update --engine flag help in cmd/helpers.go**

Change line 45 from:
```go
cmd.Flags().String("engine", "", "execution engine: go (legacy builder), blueprint (YAML pipeline), dsl")
```
To:
```go
cmd.Flags().String("engine", "", "execution engine: go (legacy builder), blueprint (YAML pipeline)")
```

- [ ] **Step 4: Update session.go description**

Change `cmd/session.go` line 13 from:
```go
Long:  "Spawns one Claude session. Used by golem-dsl as a session adapter.",
```
To:
```go
Long:  "Spawns one Claude session.",
```

- [ ] **Step 5: Build and test**

Run: `cd /home/winler/projects/golem && go build ./... && go test ./cmd/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/code.go cmd/code_test.go cmd/helpers.go cmd/session.go
git commit -m "refactor(cmd): remove DSL engine option from CLI"
```

---

### Task 7: Remove DSL from Config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Remove DSLCommand from Config struct**

In `internal/config/config.go`:
- Remove `DSLCommand` field (line 32) from `Config` struct
- Remove `DSLCommand: "golem-dsl",` from `Defaults()` (line 49)
- Remove `DSLCommand *string` from `configLayer` struct (line 107)
- Remove the `if layer.DSLCommand != nil` block from `merge()` (lines 171-173)
- Remove `case "dsl-command"` from `GetValue()` (lines 250-251)
- Remove `if cfg.DSLCommand != "golem-dsl"` block from `PrintConfig()` (lines 289-291)
- Remove `{"dsl-command", ...}` from `Keys()` (line 319)
- Update `Keys()` engine description: change `"orchestration engine: go or dsl (default: go)"` to `"orchestration engine: go (legacy) or blueprint (default: blueprint)"`

- [ ] **Step 2: Update config_test.go**

- In `TestDefaults_Engine`: remove DSLCommand assertion (keep engine default assertion)
- Remove `TestLoad_EngineFromYAML` entirely (tests dsl-command YAML parsing)
- In `TestGetValue_Engine`: remove the `dsl-command` test case

- [ ] **Step 3: Run config tests**

Run: `cd /home/winler/projects/golem && go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "refactor(config): remove DSLCommand config field"
```

---

### Task 8: Delete Clojure DSL Directory

**Files:**
- Delete: `golem-dsl/` (entire directory)

- [ ] **Step 1: Remove the directory**

```bash
cd /home/winler/projects/golem && rm -rf golem-dsl/
```

- [ ] **Step 2: Run full test suite**

Run: `cd /home/winler/projects/golem && go test ./...`
Expected: PASS — no Go code depends on golem-dsl/

- [ ] **Step 3: Build from source**

Run: `cd /home/winler/projects/golem && go build .`
Expected: SUCCESS

- [ ] **Step 4: Commit**

```bash
git add -A golem-dsl/
git commit -m "refactor: remove Clojure DSL runtime

The valuable patterns (custom predicates, error classification with prompt
amendment) have been extracted into the Go blueprint engine. The Clojure
runtime is no longer needed."
```

---

## Chunk 4: Final Verification

### Task 9: Full Build + Test + Smoke Test

- [ ] **Step 1: Full build**

Run: `cd /home/winler/projects/golem && go build ./...`
Expected: SUCCESS

- [ ] **Step 2: Full test suite**

Run: `cd /home/winler/projects/golem && go test ./...`
Expected: ALL PASS

- [ ] **Step 3: Verify CLI help shows no DSL references**

Run: `cd /home/winler/projects/golem && go run . code --help`
Expected: `--engine` flag help does not mention "dsl"

- [ ] **Step 4: Verify golem run --help still works**

Run: `cd /home/winler/projects/golem && go run . run --help`
Expected: SUCCESS — agent/agent-opts still functional
