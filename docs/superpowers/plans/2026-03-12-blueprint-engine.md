# Blueprint Engine Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Go builder loop and Clojure DSL with a YAML-driven blueprint engine that walks agent pipelines with deterministic primitives, tool scoping, and structured contract enforcement.

**Architecture:** The engine parses agent YAML into a Go pipeline structure, validates contracts at load time, then walks the pipeline sequentially — dispatching agentic steps to `ClaudeRunner`, builtin steps to Go functions, and shell steps to `exec.Command`. Pipeline state flows between steps as versioned JSON in `.ctx/runs/`. Events are emitted as NDJSON for CLI TUI, Flutter GUI, and post-mortem consumption.

**Tech Stack:** Go stdlib, `gopkg.in/yaml.v3`, `embed`, existing `ClaudeRunner` / MCP infrastructure.

**Spec:** `docs/plans/2026-03-12-minions-alignment-design.md`

---

## File Structure

### New Files

| File | Responsibility |
|---|---|
| `internal/runner/blueprint.go` | Agent YAML parsing, pipeline construction, contract validation |
| `internal/runner/blueprint_test.go` | Tests for parsing, validation, contract rules |
| `internal/runner/predicates.go` | Built-in predicate registry and evaluation |
| `internal/runner/predicates_test.go` | Tests for each predicate with present/missing/nil keys |
| `internal/runner/primitives.go` | Built-in primitive implementations (git-setup, lint, run-tests, ci-tests, create-pr) |
| `internal/runner/primitives_test.go` | Tests for each primitive with mock exec |
| `internal/runner/engine.go` | Blueprint executor — walks pipeline, dispatches steps, manages state |
| `internal/runner/engine_test.go` | Unit and integration tests for engine |
| `internal/runner/testhelpers_test.go` | Shared test helpers: `setupGitRepo`, `smartMockRunner` |
| `templates/agents/build-feature.yaml` | Embedded default agent |
| `templates/agents/fix-bug.yaml` | Embedded default agent |
| `templates/agents/one-shot.yaml` | Embedded default agent |
| `templates/prompts/plan.md` | Prompt template for plan step |
| `templates/prompts/implement.md` | Prompt template for implement step |
| `templates/prompts/review.md` | Prompt template for review step |
| `templates/prompts/research.md` | Prompt template for research step |
| `templates/prompts/reflect.md` | Prompt template for reflect step |
| `internal/runner/testdata/valid-agent.yaml` | Test fixture: valid agent |
| `internal/runner/testdata/invalid-contract.yaml` | Test fixture: contract violation |
| `internal/runner/testdata/unknown-fields.yaml` | Test fixture: unknown YAML fields |
| `internal/runner/testdata/shell-steps.yaml` | Test fixture: shell step agent |
| `cmd/runs.go` | `golem runs list/inspect/clean/watch` commands |
| `cmd/agents.go` | `golem agents list` command (replaces existing DSL-era agents.go) |

### Modified Files

| File | Change |
|---|---|
| `internal/runner/command.go` | Add `RunWithTools` method to interface + `ClaudeRunner` |
| `internal/mcp/server.go` | Read `GOLEM_TOOLS` env var, filter tool registration |
| `cmd/code.go` | Add `engine: blueprint` path, `--goal` flag |
| `cmd/helpers.go` | Add `--goal` flag registration |
| `templates/embed.go` | Embed `agents/*.yaml` and `prompts/*.md` |

---

## Chunk 1: Foundation — Blueprint Parsing & Contract Validation

### Task 1: Blueprint YAML Types and Parsing

**Files:**
- Create: `internal/runner/blueprint.go`
- Create: `internal/runner/blueprint_test.go`
- Create: `internal/runner/testdata/valid-agent.yaml`
- Create: `internal/runner/testdata/invalid-contract.yaml`
- Create: `internal/runner/testdata/unknown-fields.yaml`
- Create: `internal/runner/testdata/shell-steps.yaml`

- [ ] **Step 1: Write test fixtures**

Create `internal/runner/testdata/valid-agent.yaml`:
```yaml
name: test-agent
description: "Test agent"
initial-state: [goal]

config:
  lint-cmd: null
  test-cmd: null
  ci-enabled: false

steps:
  - git-setup:
      type: builtin

  - plan:
      type: agentic
      reads: [goal]
      writes: [plan]
      tools: [semantic_search]

  - implement:
      type: agentic
      reads: [goal, plan]
      optional-reads: [lint-results, test-results]
      writes: [code, test-results]

  - lint:
      type: builtin
      reads: [code]
      writes: [lint-results]

  - run-tests:
      type: builtin
      reads: [code]
      writes: [test-results]

  - create-pr:
      type: builtin
      reads: [code, goal]
      optional-reads: [plan, test-results, lint-results]
      writes: [pr-result]

errors:
  transient: { action: retry, max: 3 }
  malformed-output: { action: re-run, max: 2, hint: "Write session-output.json with required keys." }
  contract-violation: { action: halt }
```

Create `internal/runner/testdata/invalid-contract.yaml`:
```yaml
name: bad-contract
description: "Agent with contract violation"
initial-state: [goal]

config: {}

steps:
  - implement:
      type: agentic
      reads: [goal, plan]
      writes: [code]

errors:
  contract-violation: { action: halt }
```
(`implement` reads `plan` but nothing writes it and it's not in `initial-state`)

Create `internal/runner/testdata/unknown-fields.yaml`:
```yaml
name: bad-fields
description: "Agent with unknown field"
initial-state: [goal]

config: {}

steps:
  - plan:
      type: agentic
      reads: [goal]
      write: [plan]
      tool: [semantic_search]

errors: {}
```
(`write` instead of `writes`, `tool` instead of `tools`)

Create `internal/runner/testdata/shell-steps.yaml`:
```yaml
name: shell-agent
description: "Agent with shell step"
initial-state: [goal]

config: {}

steps:
  - git-setup:
      type: builtin

  - implement:
      type: agentic
      reads: [goal]
      writes: [code]

  - db-migrate:
      type: shell
      command: "make db-migrate"
      timeout: 60s
      reads: [code]
      writes: [migration-results]
      errors:
        non-zero: halt

  - create-pr:
      type: builtin
      reads: [code, goal]
      optional-reads: [migration-results]
      writes: [pr-result]

errors:
  transient: { action: retry, max: 2 }
  contract-violation: { action: halt }
```

- [ ] **Step 2: Write failing tests for YAML parsing**

Create `internal/runner/blueprint_test.go`:
```go
package runner

import (
	"os"
	"testing"
)

func TestParseBlueprint_ValidAgent(t *testing.T) {
	data, err := os.ReadFile("testdata/valid-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bp, err := ParseBlueprint(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bp.Name != "test-agent" {
		t.Errorf("name = %q, want %q", bp.Name, "test-agent")
	}
	if bp.Description != "Test agent" {
		t.Errorf("description = %q, want %q", bp.Description, "Test agent")
	}
	if len(bp.InitialState) != 1 || bp.InitialState[0] != "goal" {
		t.Errorf("initial-state = %v, want [goal]", bp.InitialState)
	}
	if len(bp.Steps) != 6 {
		t.Fatalf("steps count = %d, want 6", len(bp.Steps))
	}
	// Check plan step
	plan := bp.Steps[1]
	if plan.Name != "plan" {
		t.Errorf("step[1].Name = %q, want %q", plan.Name, "plan")
	}
	if plan.Type != StepTypeAgentic {
		t.Errorf("step[1].Type = %q, want %q", plan.Type, StepTypeAgentic)
	}
	if len(plan.Tools) != 1 || plan.Tools[0] != "semantic_search" {
		t.Errorf("step[1].Tools = %v, want [semantic_search]", plan.Tools)
	}
}

func TestParseBlueprint_UnknownFields(t *testing.T) {
	data, err := os.ReadFile("testdata/unknown-fields.yaml")
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseBlueprint(data)
	if err == nil {
		t.Fatal("expected error for unknown fields, got nil")
	}
	// Should suggest corrections
	errStr := err.Error()
	if !strings.Contains(errStr, "write") || !strings.Contains(errStr, "writes") {
		t.Errorf("error should mention 'write' -> 'writes', got: %s", errStr)
	}
	if !strings.Contains(errStr, "tool") || !strings.Contains(errStr, "tools") {
		t.Errorf("error should mention 'tool' -> 'tools', got: %s", errStr)
	}
}

func TestParseBlueprint_ShellStep(t *testing.T) {
	data, err := os.ReadFile("testdata/shell-steps.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bp, err := ParseBlueprint(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Find the db-migrate step
	var shellStep *Step
	for i := range bp.Steps {
		if bp.Steps[i].Name == "db-migrate" {
			shellStep = &bp.Steps[i]
			break
		}
	}
	if shellStep == nil {
		t.Fatal("db-migrate step not found")
	}
	if shellStep.Type != StepTypeShell {
		t.Errorf("type = %q, want %q", shellStep.Type, StepTypeShell)
	}
	if shellStep.Command != "make db-migrate" {
		t.Errorf("command = %q, want %q", shellStep.Command, "make db-migrate")
	}
	if shellStep.Timeout != "60s" {
		t.Errorf("timeout = %q, want %q", shellStep.Timeout, "60s")
	}
}

func TestParseBlueprint_ErrorHandlers(t *testing.T) {
	data, err := os.ReadFile("testdata/valid-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bp, err := ParseBlueprint(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bp.Errors.Transient.Action != "retry" || bp.Errors.Transient.Max != 3 {
		t.Errorf("transient = %+v, want action=retry max=3", bp.Errors.Transient)
	}
	if bp.Errors.MalformedOutput.Action != "re-run" || bp.Errors.MalformedOutput.Max != 2 {
		t.Errorf("malformed-output = %+v, want action=re-run max=2", bp.Errors.MalformedOutput)
	}
	if bp.Errors.ContractViolation.Action != "halt" {
		t.Errorf("contract-violation = %+v, want action=halt", bp.Errors.ContractViolation)
	}
}

```

Note: Use `strings.Contains` from stdlib throughout all test files — do not define custom helpers.

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestParseBlueprint -v`
Expected: FAIL — `ParseBlueprint` not defined

- [ ] **Step 4: Implement Blueprint types and ParseBlueprint**

Create `internal/runner/blueprint.go`:
```go
package runner

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Step types
const (
	StepTypeAgentic = "agentic"
	StepTypeBuiltin = "builtin"
	StepTypeShell   = "shell"
)

// Control flow node types
const (
	ControlWhile = "while"
	ControlWhen  = "when"
	ControlIf    = "if"
)

// Reserved engine-managed keys
var reservedKeys = map[string]bool{
	"code":   true,
	"branch": true,
	"base":   true,
}

// Blueprint represents a parsed agent YAML file.
type Blueprint struct {
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description"`
	InitialState []string          `yaml:"initial-state"`
	Config       map[string]any    `yaml:"config"`
	Steps        []Step            `yaml:"-"` // custom parsing
	Errors       ErrorHandlers     `yaml:"errors"`
	pipeline     *Pipeline         // constructed during parsing
}

// Step represents a single step in the pipeline.
type Step struct {
	Name          string   `yaml:"name"`
	Type          string   `yaml:"type"`
	Reads         []string `yaml:"reads"`
	Writes        []string `yaml:"writes"`
	OptionalReads []string `yaml:"optional-reads"`
	Tools         []string `yaml:"tools"`
	Prompt        string   `yaml:"prompt"`
	MaxTurns      int      `yaml:"max-turns"`
	Timeout       string   `yaml:"timeout"`
	Model         string   `yaml:"model"`
	Command       string   `yaml:"command"`       // shell steps only
	StepErrors    *StepErrors `yaml:"errors"`      // shell steps only
}

// StepErrors configures error behavior for shell steps.
type StepErrors struct {
	NonZero string `yaml:"non-zero"` // "transient" (default) or "halt"
}

// ControlFlowNode represents a while/when/if block.
type ControlFlowNode struct {
	Type      string   // "while", "when", "if"
	Predicate string
	Max       int      // while only
	StepRefs  []string // step names to execute
	ThenRefs  []string // if-then only
	ElseRefs  []string // if-else only
	// Inline steps defined within the block (not yet referenced elsewhere)
	InlineSteps []Step
}

// PipelineNode is either a Step or a ControlFlowNode.
type PipelineNode struct {
	Step        *Step
	ControlFlow *ControlFlowNode
}

// Pipeline is the ordered list of nodes to execute.
type Pipeline struct {
	Nodes    []PipelineNode
	StepDefs map[string]*Step // all step definitions by name
}

// ErrorHandlers configures pipeline-level error behavior.
type ErrorHandlers struct {
	Transient         ErrorHandler `yaml:"transient"`
	MalformedOutput   ErrorHandler `yaml:"malformed-output"`
	ContractViolation ErrorHandler `yaml:"contract-violation"`
}

// ErrorHandler specifies action and limits for an error type.
type ErrorHandler struct {
	Action string `yaml:"action"` // "retry", "re-run", "halt"
	Max    int    `yaml:"max"`
	Hint   string `yaml:"hint"`
}

// Default max-turns and timeouts per step name.
var stepDefaults = map[string]struct {
	MaxTurns int
	Timeout  time.Duration
}{
	"plan":      {MaxTurns: 50, Timeout: 20 * time.Minute},
	"implement": {MaxTurns: 200, Timeout: 30 * time.Minute},
	"review":    {MaxTurns: 50, Timeout: 20 * time.Minute},
	"reflect":   {MaxTurns: 30, Timeout: 10 * time.Minute},
	"research":  {MaxTurns: 75, Timeout: 20 * time.Minute},
}

var defaultStepMaxTurns = 75
var defaultStepTimeout = 20 * time.Minute

// Default tools per step name.
var defaultTools = map[string][]string{
	"plan":      {"semantic_search", "find_callers", "find_dependencies", "find_co_changed"},
	"implement": {"semantic_search", "find_callers", "find_dependencies", "find_dependents", "find_co_changed", "find_execution_failures", "lsp_definition", "lsp_references", "lsp_hover", "lsp_diagnostics"},
	"review":    {"semantic_search", "find_callers", "find_dependencies"},
	"reflect":   {"semantic_search"},
	"research":  {"semantic_search", "find_callers", "find_dependencies", "find_co_changed", "find_execution_failures", "get_runtime_trace"},
}

// Known field names for "did you mean?" suggestions.
var knownStepFields = map[string]string{
	"tool":  "tools",
	"write": "writes",
	"read":  "reads",
	"optional-read": "optional-reads",
}

// ParseBlueprint parses agent YAML bytes into a Blueprint.
func ParseBlueprint(data []byte) (*Blueprint, error) {
	// First pass: parse into raw map for strict field checking
	var rawMap map[string]yaml.Node
	if err := yaml.Unmarshal(data, &rawMap); err != nil {
		return nil, fmt.Errorf("YAML parse error: %w", err)
	}

	// Check top-level fields
	knownTopLevel := map[string]bool{
		"name": true, "description": true, "initial-state": true,
		"config": true, "steps": true, "errors": true,
	}
	for key, node := range rawMap {
		if !knownTopLevel[key] {
			return nil, fmt.Errorf("parse error: unknown top-level field %q at line %d", key, node.Line)
		}
	}

	// Second pass: structured parse
	var bp Blueprint
	if err := yaml.Unmarshal(data, &bp); err != nil {
		return nil, fmt.Errorf("YAML parse error: %w", err)
	}

	// Parse steps with strict field checking
	pipeline, err := parseSteps(data)
	if err != nil {
		return nil, err
	}
	bp.Steps = pipeline.steps
	bp.pipeline = pipeline

	return &bp, nil
}
```

This is the skeleton — the full `parseSteps` function handles the step map entries, control flow nodes, and strict field checking. The implementation should use `yaml.Node` for line-number-aware parsing.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestParseBlueprint -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/runner/blueprint.go internal/runner/blueprint_test.go internal/runner/testdata/
git commit -m "feat(runner): add blueprint YAML parsing with strict validation"
```

### Task 2: Contract Validation

**Files:**
- Modify: `internal/runner/blueprint.go`
- Modify: `internal/runner/blueprint_test.go`

- [ ] **Step 1: Write failing tests for contract validation**

Add to `internal/runner/blueprint_test.go`:
```go
func TestValidateContracts_Valid(t *testing.T) {
	data, err := os.ReadFile("testdata/valid-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bp, err := ParseBlueprint(data)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if err := bp.ValidateContracts(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateContracts_MissingReads(t *testing.T) {
	data, err := os.ReadFile("testdata/invalid-contract.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bp, err := ParseBlueprint(data)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	err = bp.ValidateContracts()
	if err == nil {
		t.Fatal("expected contract validation error, got nil")
	}
	if !strings.Contains(err.Error(), "plan") {
		t.Errorf("error should mention missing 'plan' key, got: %s", err.Error())
	}
}

func TestValidateContracts_DuplicateStepNames(t *testing.T) {
	yaml := `
name: dup
description: "duplicate"
initial-state: [goal]
config: {}
steps:
  - plan:
      type: agentic
      reads: [goal]
      writes: [plan]
  - plan:
      type: agentic
      reads: [goal]
      writes: [plan2]
errors: {}
`
	_, err := ParseBlueprint([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for duplicate step names, got nil")
	}
}

func TestValidateContracts_ConditionalWriteRequiresOptionalReads(t *testing.T) {
	yaml := `
name: cond
description: "conditional"
initial-state: [goal]
config:
  ci-enabled: false
steps:
  - implement:
      type: agentic
      reads: [goal]
      writes: [code]
  - when:
      predicate: ci-enabled
      steps:
        - ci-tests:
            type: builtin
            reads: [code]
            writes: [ci-results]
  - review:
      type: agentic
      reads: [code, ci-results]
      writes: [feedback]
errors: {}
`
	bp, err := ParseBlueprint([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	err = bp.ValidateContracts()
	if err == nil {
		t.Fatal("expected contract error for ci-results in reads (conditional write), got nil")
	}
	if !strings.Contains(err.Error(), "optional-reads") {
		t.Errorf("error should suggest optional-reads, got: %s", err.Error())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestValidateContracts -v`
Expected: FAIL — `ValidateContracts` not defined

- [ ] **Step 3: Implement ValidateContracts**

Add to `internal/runner/blueprint.go`:
```go
// ValidateContracts checks that all reads/writes chains are satisfied.
// Returns nil if valid, or a descriptive error.
func (bp *Blueprint) ValidateContracts() error {
	available := make(map[string]bool)
	for _, key := range bp.InitialState {
		available[key] = true
	}
	// git-setup implicitly writes branch and base
	available["branch"] = true
	available["base"] = true

	// Track which keys are written inside conditional blocks
	conditionalWrites := make(map[string]bool)

	seen := make(map[string]bool)

	for _, node := range bp.pipeline.Nodes {
		if node.Step != nil {
			if err := validateStep(node.Step, available, conditionalWrites, seen, false); err != nil {
				return err
			}
		}
		if node.ControlFlow != nil {
			if err := validateControlFlow(node.ControlFlow, available, conditionalWrites, seen, bp.pipeline.StepDefs); err != nil {
				return err
			}
		}
	}
	return nil
}
```

The `validateStep` function checks:
1. All `reads` keys exist in `available` and are not conditional-only (must use `optional-reads`)
2. `optional-reads` keys are allowed but not required
3. Step names are unique (tracked via `seen`)
4. After validation, add `writes` keys to `available`

The `validateControlFlow` function:
1. For `when`/`if`: marks writes from inner steps as conditional
2. For `while`: validates inner step refs exist
3. Recurses into nested control flow

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestValidateContracts -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runner/blueprint.go internal/runner/blueprint_test.go
git commit -m "feat(runner): add contract validation for blueprint pipelines"
```

### Task 3: Predicates

**Files:**
- Create: `internal/runner/predicates.go`
- Create: `internal/runner/predicates_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/runner/predicates_test.go`:
```go
package runner

import "testing"

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
			got := EvalPredicate("needs-work", tt.state, tt.config)
			if got != tt.want {
				t.Errorf("EvalPredicate(needs-work) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPredicate_Failed(t *testing.T) {
	tests := []struct {
		name  string
		state map[string]any
		want  bool
	}{
		{"missing", map[string]any{}, false},
		{"pass", map[string]any{"test-results": map[string]any{"status": "pass"}}, false},
		{"fail", map[string]any{"test-results": map[string]any{"status": "fail"}}, true},
		{"skipped", map[string]any{"test-results": map[string]any{"status": "skipped"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvalPredicate("failed", tt.state, nil)
			if got != tt.want {
				t.Errorf("EvalPredicate(failed) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPredicate_CIEnabled(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
		want   bool
	}{
		{"nil config", nil, false},
		{"not set", map[string]any{}, false},
		{"false", map[string]any{"ci-enabled": false}, false},
		{"true", map[string]any{"ci-enabled": true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvalPredicate("ci-enabled", nil, tt.config)
			if got != tt.want {
				t.Errorf("EvalPredicate(ci-enabled) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPredicate_Unknown(t *testing.T) {
	got := EvalPredicate("nonexistent", nil, nil)
	if got != false {
		t.Error("unknown predicate should return false")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestPredicate -v`
Expected: FAIL — `EvalPredicate` not defined

- [ ] **Step 3: Implement predicates**

Create `internal/runner/predicates.go`:
```go
package runner

// EvalPredicate evaluates a named predicate against pipeline state and config.
// Unknown predicates and missing keys return false.
func EvalPredicate(name string, state map[string]any, config map[string]any) bool {
	switch name {
	case "needs-work":
		return getNestedString(state, "review-feedback", "verdict") == "needs-work"
	case "failed":
		return getNestedString(state, "test-results", "status") == "fail"
	case "lint-failed":
		return getNestedString(state, "lint-results", "status") == "fail"
	case "ci-enabled":
		if config == nil {
			return false
		}
		v, ok := config["ci-enabled"]
		if !ok {
			return false
		}
		b, ok := v.(bool)
		return ok && b
	case "ci-failed":
		return getNestedString(state, "ci-results", "status") == "fail"
	default:
		return false
	}
}

// getNestedString safely extracts state[key1][key2] as a string.
func getNestedString(state map[string]any, key1, key2 string) string {
	if state == nil {
		return ""
	}
	v, ok := state[key1]
	if !ok || v == nil {
		return ""
	}
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	s, ok := m[key2]
	if !ok || s == nil {
		return ""
	}
	str, ok := s.(string)
	if !ok {
		return ""
	}
	return str
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestPredicate -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runner/predicates.go internal/runner/predicates_test.go
git commit -m "feat(runner): add built-in predicate registry with nil-key safety"
```

---

## Chunk 2: CommandRunner Interface Extension & MCP Tool Filtering

### Task 4: Add RunWithTools to CommandRunner

**Files:**
- Modify: `internal/runner/command.go`
- Modify: `internal/runner/command_test.go` (if exists, otherwise create)

- [ ] **Step 1: Write failing test for RunWithTools**

Add to existing test file or create `internal/runner/command_test.go`:
```go
func TestBuildCommand_WithToolsEnv_Sandbox(t *testing.T) {
	cr := &ClaudeRunner{Sandbox: true, SandboxTools: []string{"go"}}
	toolsEnv := "semantic_search,find_callers"
	args := []string{"-p", "--output-format", "stream-json", "--max-turns", "50"}

	name, gotArgs := cr.buildCommandWithTools("/tmp/project", args, toolsEnv)

	if name != "warden" {
		t.Fatalf("expected warden, got %q", name)
	}
	// Check that --env GOLEM_TOOLS=semantic_search,find_callers is in args
	found := false
	for i, arg := range gotArgs {
		if arg == "--env" && i+1 < len(gotArgs) && strings.HasPrefix(gotArgs[i+1], "GOLEM_TOOLS=") {
			found = true
			if gotArgs[i+1] != "GOLEM_TOOLS=semantic_search,find_callers" {
				t.Errorf("GOLEM_TOOLS value = %q, want %q", gotArgs[i+1], "GOLEM_TOOLS=semantic_search,find_callers")
			}
			break
		}
	}
	if !found {
		t.Errorf("--env GOLEM_TOOLS not found in warden args: %v", gotArgs)
	}
}

func TestBuildCommand_WithToolsEnv_NoSandbox(t *testing.T) {
	cr := &ClaudeRunner{}
	toolsEnv := "semantic_search"
	args := []string{"-p"}

	name, _ := cr.buildCommandWithTools("/tmp/project", args, toolsEnv)

	if name != "claude" {
		t.Fatalf("expected claude, got %q", name)
	}
	// Non-sandbox: GOLEM_TOOLS is set on cmd.Env in runInternal, not in args
	// This test just verifies the command name is correct
}
```

- [ ] **Step 2: Implement RunWithTools on CommandRunner interface and ClaudeRunner**

Modify `internal/runner/command.go`:

Add to `CommandRunner` interface:
```go
type CommandRunner interface {
	Run(ctx context.Context, dir string, prompt string, maxTurns int, model string) (string, error)
	RunWithTools(ctx context.Context, dir string, prompt string, maxTurns int, model string, tools []string) (string, error)
}
```

Add implementation on `ClaudeRunner`. **Important:** Pass tools as a local variable to avoid data races — do not store on the struct:
```go
func (c *ClaudeRunner) RunWithTools(ctx context.Context, dir string, prompt string, maxTurns int, model string, tools []string) (string, error) {
	toolsEnv := strings.Join(tools, ",")
	return c.runInternal(ctx, dir, prompt, maxTurns, model, toolsEnv)
}
```

Refactor `Run` to call `runInternal` with empty toolsEnv:
```go
func (c *ClaudeRunner) Run(ctx context.Context, dir string, prompt string, maxTurns int, model string) (string, error) {
	return c.runInternal(ctx, dir, prompt, maxTurns, model, "")
}
```

`runInternal` passes `toolsEnv` to `buildCommand` which:
- In non-sandbox mode: sets `GOLEM_TOOLS=<value>` on `cmd.Env`
- In sandbox mode: adds `--env GOLEM_TOOLS=<value>` to warden args

- [ ] **Step 3: Update mockRunner to implement RunWithTools**

Update `internal/runner/builder_test.go` mockRunner:
```go
func (m *mockRunner) RunWithTools(_ context.Context, _ string, _ string, _ int, _ string, _ []string) (string, error) {
	return m.Run(context.Background(), "", "", 0, "")
}
```

- [ ] **Step 4: Run all tests to verify nothing is broken**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runner/command.go internal/runner/command_test.go internal/runner/builder_test.go
git commit -m "feat(runner): add RunWithTools to CommandRunner interface"
```

### Task 5: MCP Tool Filtering

**Files:**
- Modify: `internal/mcp/server.go`
- Modify: `internal/mcp/server_test.go` (existing file — update `TestNewServer` to use `len(tools) >= 15` instead of `== 15` to accommodate filtered tests)

- [ ] **Step 1: Write failing test**

```go
func TestRegisterTools_WithGOLEM_TOOLS(t *testing.T) {
	// Set GOLEM_TOOLS to filter
	t.Setenv("GOLEM_TOOLS", "mark_task,set_phase")

	gs := NewServer(t.TempDir(), nil)
	tools := gs.ListTools()

	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d: %v", len(tools), tools)
	}
	// Verify only mark_task and set_phase are registered
	toolSet := make(map[string]bool)
	for _, name := range tools {
		toolSet[name] = true
	}
	if !toolSet["mark_task"] {
		t.Error("mark_task should be registered")
	}
	if !toolSet["set_phase"] {
		t.Error("set_phase should be registered")
	}
}

func TestRegisterTools_EmptyGOLEM_TOOLS(t *testing.T) {
	// No filter — all tools registered
	t.Setenv("GOLEM_TOOLS", "")

	gs := NewServer(t.TempDir(), nil)
	tools := gs.ListTools()

	if len(tools) < 5 {
		t.Fatalf("expected all tools (>5), got %d", len(tools))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/winler/projects/golem && go test ./internal/mcp/ -run TestRegisterTools -v`
Expected: FAIL

- [ ] **Step 3: Implement GOLEM_TOOLS filtering**

Modify `internal/mcp/server.go` `registerTools()`:
```go
func (gs *GolemServer) registerTools() {
	allowed := os.Getenv("GOLEM_TOOLS")
	var allowedSet map[string]bool
	if allowed != "" {
		allowedSet = make(map[string]bool)
		for _, name := range strings.Split(allowed, ",") {
			allowedSet[strings.TrimSpace(name)] = true
		}
	}

	// Always-available tools (state management + find_test_results)
	alwaysAvailable := map[string]bool{
		"mark_task": true, "set_phase": true, "set_status": true,
		"add_decision": true, "add_pitfall": true, "log_session": true,
		"find_test_results": true,
	}

	register := func(name string, tool mcp.Tool, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
		if allowedSet == nil || allowedSet[name] || alwaysAvailable[name] {
			gs.mcpServer.AddTool(tool, handler)
		}
	}

	// Use register() for each tool instead of direct AddTool
	register("mark_task", markTaskTool(), gs.handleMarkTask)
	// ... etc for all tools
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/winler/projects/golem && go test ./internal/mcp/ -run TestRegisterTools -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/server.go internal/mcp/server_test.go
git commit -m "feat(mcp): filter tool registration via GOLEM_TOOLS env var"
```

---

## Chunk 3: Shared Test Helpers & Deterministic Primitives

### Task 6: Shared Test Helpers

**Files:**
- Create: `internal/runner/testhelpers_test.go`

- [ ] **Step 1: Create shared test helpers**

Create `internal/runner/testhelpers_test.go`:
```go
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupGitRepo creates a temporary git repo with an initial commit.
func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %s\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-b", "main") // explicit default branch for portability
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0644)
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

// smartMockRunner dispatches responses based on step name and call number.
type smartMockRunner struct {
	responses func(step string, callNum int) MockResponse
	calls     []MockCall
	callCount int
}

func (m *smartMockRunner) Run(ctx context.Context, dir, prompt string, maxTurns int, model string) (string, error) {
	return m.RunWithTools(ctx, dir, prompt, maxTurns, model, nil)
}

func (m *smartMockRunner) RunWithTools(ctx context.Context, dir, prompt string, maxTurns int, model string, tools []string) (string, error) {
	m.callCount++
	call := MockCall{Prompt: prompt, Tools: tools, Dir: dir}
	m.calls = append(m.calls, call)

	// Detect step name from prompt content
	stepName := "unknown"
	for _, name := range []string{"plan", "implement", "review", "research", "reflect"} {
		if strings.Contains(strings.ToLower(prompt), name) {
			stepName = name
			break
		}
	}

	resp := m.responses(stepName, m.callCount)

	if resp.SessionOutput != nil {
		data, _ := json.Marshal(resp.SessionOutput)
		os.WriteFile(filepath.Join(dir, "session-output.json"), data, 0644)
	}

	return resp.Output, resp.Err
}

// MockCall records a single runner invocation.
type MockCall struct {
	Prompt string
	Tools  []string
	Dir    string
}

// MockResponse defines canned behavior for a mock runner call.
type MockResponse struct {
	Output        string
	Err           error
	SessionOutput map[string]any
}
```

- [ ] **Step 2: Verify build succeeds**

Run: `cd /home/winler/projects/golem && go build ./...`
Expected: SUCCESS (test file only, no non-test code yet)

- [ ] **Step 3: Commit**

```bash
git add internal/runner/testhelpers_test.go
git commit -m "test(runner): add shared test helpers for blueprint engine tests"
```

### Task 7: git-setup Primitive

**Files:**
- Create: `internal/runner/primitives.go`
- Create: `internal/runner/primitives_test.go`

- [ ] **Step 1: Write failing tests for git-setup**

Create `internal/runner/primitives_test.go`:
```go
package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// NOTE: setupGitRepo and smartMockRunner are defined in testhelpers_test.go (created before Task 6).
// See the shared test helpers task below.

func TestPrimitiveGitSetup(t *testing.T) {
	dir := setupGitRepo(t)
	result, err := primitiveGitSetup(context.Background(), dir, "test-agent", map[string]any{})
	if err != nil {
		t.Fatalf("git-setup error: %v", err)
	}
	branch, ok := result["branch"].(string)
	if !ok || !strings.HasPrefix(branch, "golem/test-agent-") {
		t.Errorf("branch = %v, want prefix golem/test-agent-", result["branch"])
	}
	base, ok := result["base"].(string)
	if !ok || base == "" {
		t.Errorf("base = %v, want non-empty", result["base"])
	}
	// Verify we're on the new branch
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = dir
	out, _ := cmd.Output()
	if strings.TrimSpace(string(out)) != branch {
		t.Errorf("current branch = %q, want %q", strings.TrimSpace(string(out)), branch)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestPrimitiveGitSetup -v`
Expected: FAIL

- [ ] **Step 3: Implement primitives framework and git-setup**

Create `internal/runner/primitives.go`:
```go
package runner

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// PrimitiveResult is a map of key-value pairs written to pipeline state.
type PrimitiveResult map[string]any

// primitiveGitSetup creates a new branch for the agent run.
func primitiveGitSetup(ctx context.Context, dir string, agentName string, config map[string]any) (PrimitiveResult, error) {
	// Determine base branch
	base, err := gitCurrentBranch(dir)
	if err != nil {
		base = "main"
	}

	// Generate branch name with timestamp
	ts := time.Now().Format("20060102-150405")
	branch := fmt.Sprintf("golem/%s-%s", agentName, ts)

	// Handle collision
	for i := 1; branchExists(dir, branch); i++ {
		branch = fmt.Sprintf("golem/%s-%s-%d", agentName, ts, i)
	}

	// Create and checkout branch
	if err := gitRun(ctx, dir, "checkout", "-b", branch); err != nil {
		return nil, fmt.Errorf("git-setup: create branch: %w", err)
	}

	return PrimitiveResult{
		"branch": branch,
		"base":   base,
	}, nil
}

func gitCurrentBranch(dir string) (string, error) {
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func branchExists(dir, branch string) bool {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = dir
	return cmd.Run() == nil
}

func gitRun(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", err, out)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestPrimitiveGitSetup -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runner/primitives.go internal/runner/primitives_test.go
git commit -m "feat(runner): add primitives framework and git-setup primitive"
```

### Task 8: lint and run-tests Primitives

**Files:**
- Modify: `internal/runner/primitives.go`
- Modify: `internal/runner/primitives_test.go`

- [ ] **Step 1: Write failing tests for lint primitive**

Add to `internal/runner/primitives_test.go`:
```go
func TestPrimitiveLint_NotConfigured(t *testing.T) {
	result, err := primitiveLint(context.Background(), t.TempDir(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	status, _ := result["status"].(string)
	if status != "skipped" {
		t.Errorf("status = %q, want %q", status, "skipped")
	}
}

func TestPrimitiveLint_Pass(t *testing.T) {
	config := map[string]any{"lint-cmd": "true"} // `true` always succeeds
	result, err := primitiveLint(context.Background(), t.TempDir(), config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	status, _ := result["status"].(string)
	if status != "pass" {
		t.Errorf("status = %q, want %q", status, "pass")
	}
}

func TestPrimitiveLint_Fail(t *testing.T) {
	config := map[string]any{"lint-cmd": "false"} // `false` always fails
	result, err := primitiveLint(context.Background(), t.TempDir(), config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	status, _ := result["status"].(string)
	if status != "fail" {
		t.Errorf("status = %q, want %q", status, "fail")
	}
}

func TestPrimitiveRunTests_NotConfigured(t *testing.T) {
	result, err := primitiveRunTests(context.Background(), t.TempDir(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	status, _ := result["status"].(string)
	if status != "skipped" {
		t.Errorf("status = %q, want %q", status, "skipped")
	}
}

func TestPrimitiveRunTests_Pass(t *testing.T) {
	config := map[string]any{"test-cmd": "true"}
	result, err := primitiveRunTests(context.Background(), t.TempDir(), config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	status, _ := result["status"].(string)
	if status != "pass" {
		t.Errorf("status = %q, want %q", status, "pass")
	}
	if _, ok := result["duration-ms"]; !ok {
		t.Error("missing duration-ms")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run "TestPrimitiveLint|TestPrimitiveRunTests" -v`
Expected: FAIL

- [ ] **Step 3: Implement lint and run-tests primitives**

Add to `internal/runner/primitives.go`:
```go
func primitiveLint(ctx context.Context, dir string, config map[string]any) (PrimitiveResult, error) {
	lintCmd, _ := config["lint-cmd"].(string)
	if lintCmd == "" {
		return PrimitiveResult{"status": "skipped", "reason": "no lint-cmd"}, nil
	}

	fixCmd, _ := config["lint-fix-cmd"].(string)
	autofixApplied := false
	if fixCmd != "" {
		runShellCmd(ctx, dir, fixCmd, 30*time.Second) // best-effort
		autofixApplied = true
	}

	out, err := runShellCmd(ctx, dir, lintCmd, 30*time.Second)
	if err != nil {
		if isCommandNotFound(err) {
			return nil, &UnrecoverableError{Msg: fmt.Sprintf("lint command not found: %s", lintCmd)}
		}
		if isTimeout(err) {
			return nil, &TransientError{Msg: "lint timeout"}
		}
		result := PrimitiveResult{"status": "fail", "output": out}
		if autofixApplied {
			result["autofix-applied"] = true
		}
		return result, nil
	}
	return PrimitiveResult{"status": "pass", "output": out}, nil
}

func primitiveRunTests(ctx context.Context, dir string, config map[string]any) (PrimitiveResult, error) {
	testCmd, _ := config["test-cmd"].(string)
	if testCmd == "" {
		return PrimitiveResult{"status": "skipped"}, nil
	}

	timeout := 5 * time.Minute
	if t, ok := config["test-timeout"].(string); ok {
		if d, err := time.ParseDuration(t); err == nil {
			timeout = d
		}
	}

	start := time.Now()
	out, err := runShellCmd(ctx, dir, testCmd, timeout)
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		if isCommandNotFound(err) {
			return nil, &UnrecoverableError{Msg: fmt.Sprintf("test command not found: %s", testCmd)}
		}
		if isTimeout(err) {
			return nil, &TransientError{Msg: "test timeout"}
		}
		return PrimitiveResult{"status": "fail", "output": out, "duration-ms": durationMs}, nil
	}
	return PrimitiveResult{"status": "pass", "output": out, "duration-ms": durationMs}, nil
}

// runShellCmd runs a shell command and returns combined output.
func runShellCmd(ctx context.Context, dir, command string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Error types for classification
type TransientError struct{ Msg string }
func (e *TransientError) Error() string { return e.Msg }

type UnrecoverableError struct{ Msg string }
func (e *UnrecoverableError) Error() string { return e.Msg }

type MalformedOutputError struct{ Msg string }
func (e *MalformedOutputError) Error() string { return e.Msg }

// isCommandNotFound checks if an error indicates the command binary was not found.
func isCommandNotFound(err error) bool {
	return errors.Is(err, exec.ErrNotFound)
}

// isTimeout checks if an error is a context deadline exceeded (timeout).
func isTimeout(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run "TestPrimitiveLint|TestPrimitiveRunTests" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runner/primitives.go internal/runner/primitives_test.go
git commit -m "feat(runner): add lint and run-tests primitives"
```

### Task 9: ci-tests and create-pr Primitives

**Files:**
- Modify: `internal/runner/primitives.go`
- Modify: `internal/runner/primitives_test.go`

- [ ] **Step 1: Write failing tests for ci-tests**

Add to `internal/runner/primitives_test.go`:
```go
func TestPrimitiveCITests_GhNotFound(t *testing.T) {
	// Use a PATH that won't have gh
	config := map[string]any{}
	state := map[string]any{"branch": "golem/test-123", "base": "main"}
	_, err := primitiveCITests(context.Background(), t.TempDir(), config, state)
	if err == nil {
		t.Fatal("expected error when gh not found")
	}
	var unrecov *UnrecoverableError
	if !errors.As(err, &unrecov) {
		t.Errorf("expected UnrecoverableError, got %T", err)
	}
}

func TestPrimitiveCreatePR_NoChanges(t *testing.T) {
	dir := setupGitRepo(t)
	state := map[string]any{
		"branch": "golem/test-123",
		"base":   "main",
		"goal":   "Test goal",
		"code":   map[string]any{"files": []string{}},
	}
	result, err := primitiveCreatePR(context.Background(), dir, map[string]any{}, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	status, _ := result["status"].(string)
	if status != "skipped" {
		t.Errorf("status = %q, want %q", status, "skipped")
	}
}

func TestGeneratePRTitle(t *testing.T) {
	tests := []struct {
		goal string
		want string
	}{
		{"Add auth", "Add auth"},
		{"Short", "Short"},
		// Word boundary truncation: cuts at last space before 70 chars
		{"Refactor the authentication middleware to support OAuth2 and OIDC flows with token refresh", "Refactor the authentication middleware to support OAuth2 and OIDC..."},
		// No word boundary (all one "word"): hard truncate at 67 + "..."
		{strings.Repeat("a", 100), strings.Repeat("a", 67) + "..."},
	}
	for _, tt := range tests {
		got := generatePRTitle(tt.goal)
		if got != tt.want {
			t.Errorf("generatePRTitle(%q) = %q, want %q", tt.goal, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run "TestPrimitiveCITests|TestPrimitiveCreatePR|TestGeneratePRTitle" -v`
Expected: FAIL

- [ ] **Step 3: Implement ci-tests and create-pr primitives**

Add to `internal/runner/primitives.go` the full implementations per spec:

`primitiveCITests`: push branch, poll for workflow (5s intervals, 30s window), watch run, fetch logs on failure.

`primitiveCreatePR`: push with `--force-with-lease`, generate title from goal (truncate at 70 chars word boundary), build PR body from state, parse CODEOWNERS best-effort, `gh pr create`.

`generatePRTitle`: truncate goal at 70 chars on word boundary, append `...` if truncated.

`buildPRBody`: assemble markdown from goal, plan, git diff stat, validation results (lint/test/CI).

`parseCODEOWNERS`: line-by-line, match changed files against patterns, return reviewer list.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run "TestPrimitiveCITests|TestPrimitiveCreatePR|TestGeneratePRTitle" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runner/primitives.go internal/runner/primitives_test.go
git commit -m "feat(runner): add ci-tests and create-pr primitives"
```

---

## Chunk 4: Prompt Templates & Embedded Agents

### Task 10: Prompt Template Rendering

**Files:**
- Modify: `internal/runner/blueprint.go` (add rendering functions)
- Modify: `internal/runner/blueprint_test.go`

- [ ] **Step 1: Write failing tests for template rendering**

Add to `internal/runner/blueprint_test.go`:
```go
func TestRenderTemplate_BasicInterpolation(t *testing.T) {
	tmpl := "Goal: ${goal}\nPlan: ${plan}"
	state := map[string]any{
		"goal": "Add auth",
		"plan": []any{map[string]any{"step": 1, "desc": "Add middleware"}},
	}
	result, err := RenderStepPrompt(tmpl, []string{"goal", "plan"}, nil, state, nil, "test-agent", "run-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(result, "Add auth") {
		t.Errorf("result should contain goal, got: %s", result)
	}
}

func TestRenderTemplate_OptionalReadsOmitted(t *testing.T) {
	tmpl := "Goal: ${goal}"
	state := map[string]any{"goal": "Add auth"}
	// lint-results not in state — optional section should be omitted
	result, err := RenderStepPrompt(tmpl, []string{"goal"}, []string{"lint-results"}, state, nil, "test-agent", "run-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contains(result, "lint-results") {
		t.Errorf("optional read not in state should be omitted, got: %s", result)
	}
}

func TestRenderTemplate_UnknownToken(t *testing.T) {
	tmpl := "Goal: ${goal}\nBad: ${typo}"
	state := map[string]any{"goal": "test"}
	_, err := RenderStepPrompt(tmpl, []string{"goal"}, nil, state, nil, "test-agent", "run-001")
	if err == nil {
		t.Fatal("expected error for unknown ${typo} token")
	}
}

func TestRenderTemplate_ConfigVars(t *testing.T) {
	tmpl := "Cmd: ${config.lint-cmd}"
	config := map[string]any{"lint-cmd": "golangci-lint run"}
	result, err := RenderStepPrompt(tmpl, nil, nil, map[string]any{}, config, "test-agent", "run-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(result, "golangci-lint run") {
		t.Errorf("config var not replaced, got: %s", result)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestRenderTemplate -v`
Expected: FAIL

- [ ] **Step 3: Implement RenderStepPrompt**

Add to `internal/runner/blueprint.go`:
```go
// RenderStepPrompt renders a prompt template with pipeline state.
func RenderStepPrompt(tmpl string, reads, optionalReads []string, state, config map[string]any, agentName, runID string) (string, error) {
	result := tmpl

	// Replace reads keys (guaranteed present)
	for _, key := range reads {
		val, ok := state[key]
		if !ok {
			return "", fmt.Errorf("template error: reads key %q not in state", key)
		}
		jsonVal, _ := json.Marshal(val)
		result = strings.ReplaceAll(result, "${"+key+"}", string(jsonVal))
	}

	// Replace optional-reads (omit entire section if absent).
	// The engine pre-appends optional sections with a "# Section Header\n${key}" block.
	// If the key is absent, remove the entire block (header line + token line).
	for _, key := range optionalReads {
		token := "${" + key + "}"
		val, ok := state[key]
		if ok {
			jsonVal, _ := json.Marshal(val)
			result = strings.ReplaceAll(result, token, string(jsonVal))
		} else {
			// Remove the section: find lines containing the token and the preceding header line
			lines := strings.Split(result, "\n")
			var filtered []string
			skipNext := false
			for i, line := range lines {
				if strings.Contains(line, token) {
					// Also remove preceding header line (e.g., "# Previous Test Results")
					if i > 0 && strings.HasPrefix(strings.TrimSpace(filtered[len(filtered)-1]), "#") {
						filtered = filtered[:len(filtered)-1]
					}
					continue
				}
				if skipNext {
					skipNext = false
					continue
				}
				filtered = append(filtered, line)
			}
			result = strings.Join(filtered, "\n")
		}
	}

	// Replace config vars: ${config.key}
	if config != nil {
		for key, val := range config {
			jsonVal, _ := json.Marshal(val)
			result = strings.ReplaceAll(result, "${config."+key+"}", string(jsonVal))
		}
	}

	// Replace agent.name and run.id
	result = strings.ReplaceAll(result, "${agent.name}", agentName)
	result = strings.ReplaceAll(result, "${run.id}", runID)

	// Check for remaining ${...} tokens — indicates typo
	if idx := strings.Index(result, "${"); idx != -1 {
		end := strings.Index(result[idx:], "}")
		if end != -1 {
			token := result[idx : idx+end+1]
			return "", fmt.Errorf("template error: unresolved token %s (typo in template?)", token)
		}
	}

	return result, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestRenderTemplate -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runner/blueprint.go internal/runner/blueprint_test.go
git commit -m "feat(runner): add prompt template rendering with ${key} interpolation"
```

### Task 11: Embedded Agent YAML and Prompt Templates

**IMPORTANT:** This task MUST be completed before running `go build ./...` on any subsequent task, because `embed.go` references the `agents/*.yaml` and `prompts/*.md` globs. If the files don't exist, the build fails. Tasks 1-10 that modify `internal/runner/` only need `go test ./internal/runner/` (not `go build ./...`) and are unaffected.

**Files:**
- Create: `templates/agents/build-feature.yaml`
- Create: `templates/agents/fix-bug.yaml`
- Create: `templates/agents/one-shot.yaml`
- Create: `templates/prompts/plan.md`
- Create: `templates/prompts/implement.md`
- Create: `templates/prompts/review.md`
- Create: `templates/prompts/research.md`
- Create: `templates/prompts/reflect.md`
- Modify: `templates/embed.go`

- [ ] **Step 1: Create agent YAML files**

Copy the three agent definitions from the spec (`docs/plans/2026-03-12-minions-alignment-design.md`) into:
- `templates/agents/build-feature.yaml`
- `templates/agents/fix-bug.yaml`
- `templates/agents/one-shot.yaml`

- [ ] **Step 2: Create prompt templates**

Create `templates/prompts/implement.md`:
```markdown
You are implementing a code change in a software project.

# Goal
${goal}

# Plan
${plan}

# Instructions
- Write the code changes needed to accomplish the goal
- Write or update tests for your changes
- When finished, write a session-output.json file in the working directory

Write session-output.json containing:
{"test-results": {"status": "pass|fail", "summary": "..."}}

Note: Do NOT write a "code" key — the engine detects changed files automatically via git diff.
```

Create `templates/prompts/plan.md`:
```markdown
You are planning a code change for a software project.

# Goal
${goal}

# Instructions
- Analyze the codebase to understand the current architecture
- Create a step-by-step implementation plan
- Each step should be concrete and actionable

When finished, write a session-output.json file containing:
{"plan": [{"step": 1, "desc": "..."}, {"step": 2, "desc": "..."}, ...]}
```

Create `templates/prompts/review.md`:
```markdown
You are reviewing code changes in a software project.

# Code Changes
${code}

# Test Results
${test-results}

# Lint Results
${lint-results}

# Instructions
- Review the code for correctness, style, and potential issues
- Check if tests adequately cover the changes
- Set verdict to "approved" or "needs-work"

Write session-output.json containing:
{"review-feedback": {"verdict": "approved|needs-work", "comments": "..."}}
```

Create `templates/prompts/research.md`:
```markdown
You are researching a bug in a software project.

# Goal
${goal}

# Instructions
- Investigate the codebase to understand the bug
- Trace call chains and examine related code
- Document your findings

Write session-output.json containing:
{"research-context": {"root-cause": "...", "affected-files": [...], "fix-approach": "..."}}
```

Create `templates/prompts/reflect.md`:
```markdown
You are performing a holistic review of implementation work.

# Goal
${goal}

# Code Changes
${code}

# Instructions
- Look at the overall implementation holistically
- Check for architectural coherence and naming consistency
- Identify edge cases that might have been missed

Write session-output.json containing:
{"reflection": {"issues": [...], "suggestions": [...]}}
```

- [ ] **Step 3: Update embed.go**

Modify `templates/embed.go`:
```go
package templates

import "embed"

//go:embed state.yaml log.yaml prompt.md review-prompt.md qa-prompt.md claude.md agents/*.yaml prompts/*.md
var FS embed.FS
```

- [ ] **Step 4: Verify build succeeds**

Run: `cd /home/winler/projects/golem && go build ./...`
Expected: SUCCESS

- [ ] **Step 5: Commit**

```bash
git add templates/agents/ templates/prompts/ templates/embed.go
git commit -m "feat(templates): add embedded agent YAML and prompt templates"
```

---

## Chunk 5: Blueprint Engine

### Task 12: Engine Core — Pipeline State Management

**Files:**
- Create: `internal/runner/engine.go`
- Create: `internal/runner/engine_test.go`

- [ ] **Step 1: Write failing test for engine state management**

Create `internal/runner/engine_test.go`:
```go
package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// MockRunnerWithTools records calls and returns canned responses.
type MockRunnerWithTools struct {
	Calls     []MockCall
	Responses map[string]MockResponse
	callIndex int
}

type MockCall struct {
	Prompt string
	Tools  []string
	Dir    string
}

type MockResponse struct {
	Output        string
	Err           error
	SessionOutput map[string]any // written as session-output.json
}

func (m *MockRunnerWithTools) Run(ctx context.Context, dir, prompt string, maxTurns int, model string) (string, error) {
	return m.RunWithTools(ctx, dir, prompt, maxTurns, model, nil)
}

func (m *MockRunnerWithTools) RunWithTools(ctx context.Context, dir, prompt string, maxTurns int, model string, tools []string) (string, error) {
	call := MockCall{Prompt: prompt, Tools: tools, Dir: dir}
	m.Calls = append(m.Calls, call)

	// Find response by step name (extracted from prompt or use index)
	var resp MockResponse
	for name, r := range m.Responses {
		if contains(prompt, name) {
			resp = r
			break
		}
	}

	// Write session-output.json if provided
	if resp.SessionOutput != nil {
		data, _ := json.Marshal(resp.SessionOutput)
		os.WriteFile(filepath.Join(dir, "session-output.json"), data, 0644)
	}

	return resp.Output, resp.Err
}

func TestEngine_RunID(t *testing.T) {
	e := NewEngine(EngineConfig{
		Dir:       t.TempDir(),
		AgentName: "test",
		Goal:      "test goal",
	})
	if e.RunID == "" {
		t.Error("RunID should not be empty")
	}
	if !contains(e.RunID, "run-") {
		t.Errorf("RunID should start with run-, got: %s", e.RunID)
	}
}

func TestEngine_InitialState(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	e := NewEngine(EngineConfig{
		Dir:       dir,
		AgentName: "test",
		Goal:      "Add authentication",
	})

	state := e.State()
	goal, ok := state["goal"].(string)
	if !ok || goal != "Add authentication" {
		t.Errorf("state[goal] = %v, want 'Add authentication'", state["goal"])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestEngine -v`
Expected: FAIL

- [ ] **Step 3: Implement engine core with state management**

Create `internal/runner/engine.go`:
```go
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// EngineConfig holds configuration for the blueprint engine.
type EngineConfig struct {
	Dir        string
	AgentName  string
	Goal       string
	Blueprint  *Blueprint
	Config     map[string]any // merged agent config + agent-opts
	Runner     CommandRunner
	Model      string
	Events     chan<- EngineEvent
	Verbose    bool
}

// EngineEvent represents a structured event emitted during pipeline execution.
type EngineEvent struct {
	Type      string         `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Step      string         `json:"step,omitempty"`
	StepType  string         `json:"step-type,omitempty"`
	Status    string         `json:"status,omitempty"`
	Duration  int64          `json:"duration-ms,omitempty"`
	Agent     string         `json:"agent,omitempty"`
	Goal      string         `json:"goal,omitempty"`
	RunID     string         `json:"run-id,omitempty"`
	Line      string         `json:"line,omitempty"`
	Predicate string         `json:"predicate,omitempty"`
	Iteration int            `json:"iteration,omitempty"`
	Max       int            `json:"max,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	ErrorType string         `json:"error-type,omitempty"`
	Action    string         `json:"action,omitempty"`
	Attempt   int            `json:"attempt,omitempty"`
}

// Engine executes a blueprint pipeline.
type Engine struct {
	RunID   string
	cfg     EngineConfig
	state   map[string]any
	runDir  string
	stateVer int
	logFile *os.File
}

// NewEngine creates a new engine instance.
func NewEngine(cfg EngineConfig) *Engine {
	ts := time.Now().Format("20060102-150405")
	runID := "run-" + ts

	e := &Engine{
		RunID: runID,
		cfg:   cfg,
		state: map[string]any{"goal": cfg.Goal},
	}
	return e
}

// State returns the current pipeline state.
func (e *Engine) State() map[string]any {
	return e.state
}

// Run executes the pipeline. Returns final state and error.
func (e *Engine) Run(ctx context.Context) (map[string]any, error) {
	// Create run directory
	e.runDir = filepath.Join(e.cfg.Dir, ".ctx", "runs", e.RunID)
	if err := os.MkdirAll(filepath.Join(e.runDir, "sessions"), 0755); err != nil {
		return nil, fmt.Errorf("create run dir: %w", err)
	}

	// Create current symlink
	currentLink := filepath.Join(e.cfg.Dir, ".ctx", "runs", "current")
	os.Remove(currentLink) // ignore error
	os.Symlink(e.runDir, currentLink)
	defer os.Remove(currentLink)

	// Open log file
	var err error
	e.logFile, err = os.Create(filepath.Join(e.runDir, "log.json"))
	if err != nil {
		return nil, fmt.Errorf("create log: %w", err)
	}
	defer e.logFile.Close()

	// Save initial state
	e.saveState()

	// Emit pipeline-start
	e.emit(EngineEvent{Type: "pipeline-start", Agent: e.cfg.AgentName, Goal: e.cfg.Goal, RunID: e.RunID})

	start := time.Now()

	// Walk pipeline nodes
	for _, node := range e.cfg.Blueprint.pipeline.Nodes {
		if err := e.execNode(ctx, node); err != nil {
			e.emit(EngineEvent{Type: "pipeline-end", Status: "error", Duration: time.Since(start).Milliseconds(), RunID: e.RunID})
			return e.state, err
		}
	}

	e.emit(EngineEvent{Type: "pipeline-end", Status: "success", Duration: time.Since(start).Milliseconds(), RunID: e.RunID})
	return e.state, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestEngine -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runner/engine.go internal/runner/engine_test.go
git commit -m "feat(runner): add blueprint engine core with state management"
```

### Task 13: Engine Pipeline Execution — Step Dispatch

**Files:**
- Modify: `internal/runner/engine.go`
- Modify: `internal/runner/engine_test.go`

- [ ] **Step 1: Write failing test for agentic step execution**

Add to `internal/runner/engine_test.go`:
```go
func TestEngine_AgenticStep(t *testing.T) {
	dir := setupGitRepo(t)
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	bp := &Blueprint{
		Name:         "test",
		InitialState: []string{"goal"},
		Config:       map[string]any{},
		Errors:       ErrorHandlers{ContractViolation: ErrorHandler{Action: "halt"}},
	}
	// Minimal pipeline: just a plan step
	bp.pipeline = &Pipeline{
		Nodes: []PipelineNode{
			{Step: &Step{Name: "plan", Type: StepTypeAgentic, Reads: []string{"goal"}, Writes: []string{"plan"}, Tools: []string{"semantic_search"}}},
		},
		StepDefs: map[string]*Step{},
	}

	mock := &MockRunnerWithTools{
		Responses: map[string]MockResponse{
			"plan": {
				Output:        "planned",
				SessionOutput: map[string]any{"plan": []any{map[string]any{"step": 1, "desc": "do thing"}}},
			},
		},
	}

	e := NewEngine(EngineConfig{
		Dir:       dir,
		AgentName: "test",
		Goal:      "Add auth",
		Blueprint: bp,
		Config:    map[string]any{},
		Runner:    mock,
		Model:     "test-model",
	})

	state, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("engine run error: %v", err)
	}
	if state["plan"] == nil {
		t.Error("plan should be in state after agentic step")
	}
	if len(mock.Calls) != 1 {
		t.Errorf("expected 1 runner call, got %d", len(mock.Calls))
	}
	if len(mock.Calls[0].Tools) != 1 || mock.Calls[0].Tools[0] != "semantic_search" {
		t.Errorf("tools = %v, want [semantic_search]", mock.Calls[0].Tools)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestEngine_AgenticStep -v`
Expected: FAIL

- [ ] **Step 3: Implement execNode, execStep, execAgenticStep**

Add to `internal/runner/engine.go`:
```go
func (e *Engine) execNode(ctx context.Context, node PipelineNode) error {
	if node.Step != nil {
		return e.execStep(ctx, node.Step)
	}
	if node.ControlFlow != nil {
		return e.execControlFlow(ctx, node.ControlFlow)
	}
	return nil
}

func (e *Engine) execStep(ctx context.Context, step *Step) error {
	e.emit(EngineEvent{Type: "step-start", Step: step.Name, StepType: step.Type})
	start := time.Now()

	var err error
	switch step.Type {
	case StepTypeAgentic:
		err = e.execAgenticStep(ctx, step)
	case StepTypeBuiltin:
		err = e.execBuiltinStep(ctx, step)
	case StepTypeShell:
		err = e.execShellStep(ctx, step)
	default:
		err = fmt.Errorf("unknown step type: %s", step.Type)
	}

	status := "success"
	if err != nil {
		status = "error"
	}
	e.emit(EngineEvent{Type: "step-end", Step: step.Name, Status: status, Duration: time.Since(start).Milliseconds()})

	if err != nil {
		return e.handleError(ctx, step, err)
	}
	e.saveState()
	return nil
}

func (e *Engine) execAgenticStep(ctx context.Context, step *Step) error {
	// Load and render prompt template
	tmpl, err := e.loadPromptTemplate(step)
	if err != nil {
		return err
	}
	prompt, err := RenderStepPrompt(tmpl, step.Reads, step.OptionalReads, e.state, e.cfg.Config, e.cfg.AgentName, e.RunID)
	if err != nil {
		return err
	}

	// Resolve tools
	tools := step.Tools
	if len(tools) == 0 {
		tools = defaultTools[step.Name]
	}

	// Resolve max-turns and timeout
	maxTurns := e.resolveMaxTurns(step)
	timeout := e.resolveTimeout(step)

	// Run with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err = e.cfg.Runner.RunWithTools(ctx, e.cfg.Dir, prompt, maxTurns, e.resolveModel(step), tools)
	if err != nil {
		return &TransientError{Msg: fmt.Sprintf("agentic step %s: %v", step.Name, err)}
	}

	// Read session-output.json
	if err := e.readSessionOutput(step); err != nil {
		return err
	}

	// Auto-detect code changes for reserved key
	if containsStr(step.Writes, "code") {
		e.detectCodeChanges()
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestEngine_AgenticStep -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runner/engine.go internal/runner/engine_test.go
git commit -m "feat(runner): add engine step dispatch for agentic, builtin, shell"
```

### Task 14: Engine Control Flow — while/when/if

**Files:**
- Modify: `internal/runner/engine.go`
- Modify: `internal/runner/engine_test.go`

- [ ] **Step 1: Write failing test for while loop**

Add to `internal/runner/engine_test.go`:
```go
func TestEngine_WhileLoop(t *testing.T) {
	dir := setupGitRepo(t)
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	// Build a pipeline with: implement → review → while(needs-work) [implement, review]
	implementStep := &Step{Name: "implement", Type: StepTypeAgentic, Reads: []string{"goal"}, OptionalReads: []string{"review-feedback"}, Writes: []string{"code", "test-results"}}
	reviewStep := &Step{Name: "review", Type: StepTypeAgentic, Reads: []string{"code"}, Writes: []string{"review-feedback"}}

	bp := &Blueprint{
		Name:         "test",
		InitialState: []string{"goal"},
		Config:       map[string]any{},
		Errors:       ErrorHandlers{ContractViolation: ErrorHandler{Action: "halt"}},
	}
	bp.pipeline = &Pipeline{
		Nodes: []PipelineNode{
			{Step: implementStep},
			{Step: reviewStep},
			{ControlFlow: &ControlFlowNode{
				Type: ControlWhile, Predicate: "needs-work", Max: 3,
				StepRefs: []string{"implement", "review"},
			}},
		},
		StepDefs: map[string]*Step{"implement": implementStep, "review": reviewStep},
	}

	// First review: needs-work. Second review: approved.
	reviewCalls := 0
	smartMock := &smartMockRunner{
		responses: func(step string, callNum int) MockResponse {
			if step == "review" {
				reviewCalls++
				if reviewCalls == 1 {
					return MockResponse{SessionOutput: map[string]any{"review-feedback": map[string]any{"verdict": "needs-work"}}}
				}
				return MockResponse{SessionOutput: map[string]any{"review-feedback": map[string]any{"verdict": "approved"}}}
			}
			return MockResponse{SessionOutput: map[string]any{"test-results": map[string]any{"status": "pass"}}}
		},
	}

	e := NewEngine(EngineConfig{
		Dir: dir, AgentName: "test", Goal: "Test",
		Blueprint: bp, Config: map[string]any{}, Runner: smartMock, Model: "test",
	})

	state, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("engine error: %v", err)
	}
	verdict := getNestedString(state, "review-feedback", "verdict")
	if verdict != "approved" {
		t.Errorf("final verdict = %q, want approved", verdict)
	}
	// Should have run: implement, review, while(implement, review) = 4 total calls
	if len(smartMock.calls) != 4 {
		t.Errorf("expected 4 runner calls, got %d", len(smartMock.calls))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestEngine_WhileLoop -v`
Expected: FAIL

- [ ] **Step 2b: Write failing test for if/then/else**

Add to `internal/runner/engine_test.go`:
```go
func TestEngine_IfThenElse(t *testing.T) {
	dir := setupGitRepo(t)
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	stepA := &Step{Name: "fast-path", Type: StepTypeAgentic, Reads: []string{"goal"}, Writes: []string{"result"}}
	stepB := &Step{Name: "slow-path", Type: StepTypeAgentic, Reads: []string{"goal"}, Writes: []string{"result"}}

	bp := &Blueprint{
		Name: "test", InitialState: []string{"goal"}, Config: map[string]any{"ci-enabled": true},
		Errors: ErrorHandlers{ContractViolation: ErrorHandler{Action: "halt"}},
	}
	bp.pipeline = &Pipeline{
		Nodes: []PipelineNode{
			{ControlFlow: &ControlFlowNode{
				Type: ControlIf, Predicate: "ci-enabled",
				ThenRefs: []string{"fast-path"},
				ElseRefs: []string{"slow-path"},
			}},
		},
		StepDefs: map[string]*Step{"fast-path": stepA, "slow-path": stepB},
	}

	mock := &smartMockRunner{
		responses: func(step string, callNum int) MockResponse {
			return MockResponse{SessionOutput: map[string]any{"result": "done"}}
		},
	}

	e := NewEngine(EngineConfig{
		Dir: dir, AgentName: "test", Goal: "Test",
		Blueprint: bp, Config: map[string]any{"ci-enabled": true}, Runner: mock, Model: "test",
	})

	_, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("engine error: %v", err)
	}
	// Should have called fast-path (then branch), not slow-path
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
}
```

- [ ] **Step 3: Implement execControlFlow**

Add to `internal/runner/engine.go`:
```go
func (e *Engine) execControlFlow(ctx context.Context, cf *ControlFlowNode) error {
	switch cf.Type {
	case ControlWhile:
		return e.execWhile(ctx, cf)
	case ControlWhen:
		return e.execWhen(ctx, cf)
	case ControlIf:
		return e.execIf(ctx, cf)
	default:
		return fmt.Errorf("unknown control flow type: %s", cf.Type)
	}
}

func (e *Engine) execWhile(ctx context.Context, cf *ControlFlowNode) error {
	for i := 0; i < cf.Max; i++ {
		if !EvalPredicate(cf.Predicate, e.state, e.cfg.Config) {
			e.emit(EngineEvent{Type: "loop-exit", Predicate: cf.Predicate, Reason: "false"})
			return nil
		}
		e.emit(EngineEvent{Type: "loop-enter", Predicate: cf.Predicate, Iteration: i + 1, Max: cf.Max})

		for _, ref := range cf.StepRefs {
			step := e.cfg.Blueprint.pipeline.StepDefs[ref]
			if step == nil {
				// Check inline steps
				for _, inline := range cf.InlineSteps {
					if inline.Name == ref {
						step = &inline
						break
					}
				}
			}
			if step == nil {
				return fmt.Errorf("while loop: step %q not found", ref)
			}
			if err := e.execStep(ctx, step); err != nil {
				return err
			}
		}
	}
	e.emit(EngineEvent{Type: "loop-exit", Predicate: cf.Predicate, Reason: "max"})
	return nil
}

func (e *Engine) execWhen(ctx context.Context, cf *ControlFlowNode) error {
	if !EvalPredicate(cf.Predicate, e.state, e.cfg.Config) {
		e.emit(EngineEvent{Type: "conditional-skip", Predicate: cf.Predicate})
		return nil
	}
	for _, ref := range cf.StepRefs {
		step := e.cfg.Blueprint.pipeline.StepDefs[ref]
		if step == nil {
			for _, inline := range cf.InlineSteps {
				if inline.Name == ref {
					step = &inline
					break
				}
			}
		}
		if step == nil {
			return fmt.Errorf("when block: step %q not found", ref)
		}
		if err := e.execStep(ctx, step); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestEngine_WhileLoop -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runner/engine.go internal/runner/engine_test.go
git commit -m "feat(runner): add control flow execution (while/when/if)"
```

### Task 15: Engine Error Handling

**Files:**
- Modify: `internal/runner/engine.go`
- Modify: `internal/runner/engine_test.go`

- [ ] **Step 1: Write failing test for error retry**

Add to `internal/runner/engine_test.go`:
```go
func TestEngine_TransientRetry(t *testing.T) {
	dir := setupGitRepo(t)
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	step := &Step{Name: "plan", Type: StepTypeAgentic, Reads: []string{"goal"}, Writes: []string{"plan"}}
	bp := &Blueprint{
		Name: "test", InitialState: []string{"goal"}, Config: map[string]any{},
		Errors: ErrorHandlers{
			Transient:         ErrorHandler{Action: "retry", Max: 2},
			ContractViolation: ErrorHandler{Action: "halt"},
		},
	}
	bp.pipeline = &Pipeline{
		Nodes:    []PipelineNode{{Step: step}},
		StepDefs: map[string]*Step{"plan": step},
	}

	callCount := 0
	failMock := &smartMockRunner{
		responses: func(step string, callNum int) MockResponse {
			callCount++
			if callCount <= 2 {
				return MockResponse{Err: fmt.Errorf("transient failure")}
			}
			return MockResponse{SessionOutput: map[string]any{"plan": "final plan"}}
		},
	}

	e := NewEngine(EngineConfig{
		Dir: dir, AgentName: "test", Goal: "Test",
		Blueprint: bp, Config: map[string]any{}, Runner: failMock, Model: "test",
	})

	_, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("engine should recover after retries, got: %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls (2 fails + 1 success), got %d", callCount)
	}
}

func TestEngine_UnrecoverableHalts(t *testing.T) {
	dir := setupGitRepo(t)
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	step := &Step{Name: "lint", Type: StepTypeBuiltin, Reads: []string{"code"}, Writes: []string{"lint-results"}}
	bp := &Blueprint{
		Name: "test", InitialState: []string{"goal"}, Config: map[string]any{},
		Errors: ErrorHandlers{ContractViolation: ErrorHandler{Action: "halt"}},
	}
	bp.pipeline = &Pipeline{
		Nodes:    []PipelineNode{{Step: step}},
		StepDefs: map[string]*Step{},
	}

	e := NewEngine(EngineConfig{
		Dir: dir, AgentName: "test", Goal: "Test",
		Blueprint: bp, Config: map[string]any{"lint-cmd": "nonexistent-cmd-xyz"}, Runner: nil, Model: "test",
	})

	_, err := e.Run(context.Background())
	if err == nil {
		t.Fatal("expected halt on unrecoverable error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run "TestEngine_Transient|TestEngine_Unrecoverable" -v`
Expected: FAIL

- [ ] **Step 3: Implement handleError**

Add to `internal/runner/engine.go`:
```go
func (e *Engine) handleError(ctx context.Context, step *Step, err error) error {
	// Classify error
	var transient *TransientError
	var unrecoverable *UnrecoverableError
	var malformed *MalformedOutputError

	switch {
	case errors.As(err, &unrecoverable):
		e.emit(EngineEvent{Type: "error-occurred", Step: step.Name, ErrorType: "unrecoverable", Action: "halt"})
		return err // always halt

	case errors.As(err, &malformed):
		handler := e.cfg.Blueprint.Errors.MalformedOutput
		if handler.Action == "" {
			handler.Action = "halt" // default for unhandled
		}
		return e.handleMalformedOutput(ctx, step, malformed, handler)

	case errors.As(err, &transient):
		handler := e.cfg.Blueprint.Errors.Transient
		if handler.Action == "" {
			handler.Action = "halt" // default for unhandled
		}
		return e.handleTransient(ctx, step, handler)

	default:
		// Treat unknown errors as transient
		handler := e.cfg.Blueprint.Errors.Transient
		if handler.Action == "" {
			handler.Action = "halt"
		}
		return e.handleTransient(ctx, step, handler)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run "TestEngine_Transient|TestEngine_Unrecoverable" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runner/engine.go internal/runner/engine_test.go
git commit -m "feat(runner): add engine error handling with retry/re-run/halt"
```

### Task 16: Engine Integration Tests

**Files:**
- Modify: `internal/runner/engine_test.go`

- [ ] **Step 1: Write integration test — full one-shot pipeline**

```go
func TestEngine_Integration_OneShot(t *testing.T) {
	dir := setupGitRepo(t)
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	// Load one-shot agent from embedded templates
	data, err := templates.FS.ReadFile("agents/one-shot.yaml")
	if err != nil {
		t.Fatalf("read one-shot.yaml: %v", err)
	}
	bp, err := ParseBlueprint(data)
	if err != nil {
		t.Fatalf("parse blueprint: %v", err)
	}
	if err := bp.ValidateContracts(); err != nil {
		t.Fatalf("contract validation: %v", err)
	}

	mock := &smartMockRunner{
		responses: func(step string, callNum int) MockResponse {
			return MockResponse{
				SessionOutput: map[string]any{
					"test-results": map[string]any{"status": "pass", "summary": "all green"},
				},
			}
		},
	}

	config := map[string]any{"lint-cmd": "true", "test-cmd": "true", "ci-enabled": false}

	e := NewEngine(EngineConfig{
		Dir: dir, AgentName: "one-shot", Goal: "Add auth",
		Blueprint: bp, Config: config, Runner: mock, Model: "test",
	})

	state, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("engine error: %v", err)
	}

	// Verify final state has expected keys
	for _, key := range []string{"goal", "branch", "base", "test-results", "lint-results"} {
		if state[key] == nil {
			t.Errorf("state[%q] should be present", key)
		}
	}
}
```

- [ ] **Step 2: Run integration test**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestEngine_Integration -v`
Expected: PASS

- [ ] **Step 3: Write integration test — build-feature with loop**

```go
func TestEngine_Integration_BuildFeatureLoop(t *testing.T) {
	dir := setupGitRepo(t)
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	data, err := templates.FS.ReadFile("agents/build-feature.yaml")
	if err != nil {
		t.Fatalf("read build-feature.yaml: %v", err)
	}
	bp, err := ParseBlueprint(data)
	if err != nil {
		t.Fatalf("parse blueprint: %v", err)
	}

	reviewCalls := 0
	mock := &smartMockRunner{
		responses: func(step string, callNum int) MockResponse {
			if step == "review" {
				reviewCalls++
				if reviewCalls == 1 {
					return MockResponse{SessionOutput: map[string]any{"review-feedback": map[string]any{"verdict": "needs-work", "comments": "fix tests"}}}
				}
				return MockResponse{SessionOutput: map[string]any{"review-feedback": map[string]any{"verdict": "approved"}}}
			}
			if step == "plan" {
				return MockResponse{SessionOutput: map[string]any{"plan": []any{map[string]any{"step": 1, "desc": "do it"}}}}
			}
			return MockResponse{SessionOutput: map[string]any{"test-results": map[string]any{"status": "pass"}}}
		},
	}

	config := map[string]any{"lint-cmd": "true", "test-cmd": "true", "ci-enabled": false}
	e := NewEngine(EngineConfig{
		Dir: dir, AgentName: "build-feature", Goal: "Add auth",
		Blueprint: bp, Config: config, Runner: mock, Model: "test",
	})

	state, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("engine error: %v", err)
	}

	// Review should have been called exactly twice (first needs-work, then approved)
	if reviewCalls != 2 {
		t.Errorf("review was called %d times, want 2", reviewCalls)
	}
	verdict := getNestedString(state, "review-feedback", "verdict")
	if verdict != "approved" {
		t.Errorf("final verdict = %q, want approved", verdict)
	}
}
```

- [ ] **Step 4: Run all engine tests**

Run: `cd /home/winler/projects/golem && go test ./internal/runner/ -run TestEngine -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runner/engine_test.go
git commit -m "test(runner): add engine integration tests for one-shot and build-feature"
```

---

## Chunk 6: CLI Integration & New Commands

### Task 17: Wire Engine into cmd/code.go

**Files:**
- Modify: `cmd/code.go`
- Modify: `cmd/helpers.go`

- [ ] **Step 1: Add --goal and --agent flags to helpers.go**

Add to `addAgentFlags` in `cmd/helpers.go`:
```go
cmd.Flags().String("goal", "", "goal for the blueprint engine (populates initial pipeline state)")
cmd.Flags().String("agent", "", "agent to run (e.g., build-feature, fix-bug, one-shot)")
```

Add `Goal` field to `resolvedConfig`:
```go
type resolvedConfig struct {
	config.Config
	Task   string
	Goal   string
	DryRun bool
	Review bool
}
```

Wire it in `resolveConfig`:
```go
rc.Goal, _ = cmd.Flags().GetString("goal")
// --task is retained as alias
if rc.Goal == "" {
	rc.Goal = rc.Task
}
```

- [ ] **Step 2: Add blueprint engine path to cmd/code.go**

In the `RunE` function of `codeCmd`, add before the DSL engine check:
```go
if rc.Engine == "blueprint" {
	// Load agent
	agentName := rc.Agent
	if agentName == "" {
		agentName = "build-feature"
	}
	agentData, err := loadAgent(agentName, dir)
	if err != nil {
		return err
	}
	bp, err := runner.ParseBlueprint(agentData)
	if err != nil {
		return err
	}
	if err := bp.ValidateContracts(); err != nil {
		return err
	}

	// Merge config
	mergedConfig := mergeAgentConfig(bp.Config, rc.AgentOpts)

	cr := newClaudeRunner(rc)
	events := make(chan runner.EngineEvent, 100)

	e := runner.NewEngine(runner.EngineConfig{
		Dir:       dir,
		AgentName: agentName,
		Goal:      rc.Goal,
		Blueprint: bp,
		Config:    mergedConfig,
		Runner:    cr,
		Model:     rc.Model,
		Events:    events,
		Verbose:   rc.Verbose,
	})

	state, err := e.Run(ctx)
	if err != nil {
		return fmt.Errorf("blueprint engine: %w", err)
	}
	_ = state
	return nil
}
```

Add helper functions:
```go
func loadAgent(name, dir string) ([]byte, error) {
	// 1. Check .ctx/agents/<name>.yaml
	projectPath := filepath.Join(dir, ".ctx", "agents", name+".yaml")
	if data, err := os.ReadFile(projectPath); err == nil {
		return data, nil
	}
	// 2. Check embedded templates
	data, err := templates.FS.ReadFile("agents/" + name + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("agent %q not found. Searched: .ctx/agents/, built-in templates", name)
	}
	return data, nil
}

func mergeAgentConfig(agentDefaults map[string]any, agentOpts map[string]interface{}) map[string]any {
	merged := make(map[string]any)
	for k, v := range agentDefaults {
		merged[k] = v
	}
	for k, v := range agentOpts {
		merged[k] = v
	}
	return merged
}
```

- [ ] **Step 3: Verify build succeeds**

Run: `cd /home/winler/projects/golem && go build ./...`
Expected: SUCCESS

- [ ] **Step 4: Commit**

```bash
git add cmd/code.go cmd/helpers.go
git commit -m "feat(cmd): wire blueprint engine into golem code command"
```

### Task 18: golem runs Commands

**Files:**
- Create: `cmd/runs.go`

- [ ] **Step 1: Implement runs subcommands**

Create `cmd/runs.go` with:
- `golem runs list` — reads `.ctx/runs/`, shows ID, agent, status, duration
- `golem runs inspect <run-id>` — reads `log.json` and final state, displays timeline
- `golem runs clean --keep N` — deletes all but N most recent
- `golem runs watch <run-id>` — tails `log.json` as NDJSON (for live runs) or replays (for completed)

- [ ] **Step 2: Verify build succeeds**

Run: `cd /home/winler/projects/golem && go build ./...`
Expected: SUCCESS

- [ ] **Step 3: Commit**

```bash
git add cmd/runs.go
git commit -m "feat(cmd): add golem runs list/inspect/clean/watch commands"
```

### Task 19: golem agents list Command

**Files:**
- Replace: `cmd/agents.go` (existing file has DSL-era hardcoded agent list — replace entirely)

- [ ] **Step 1: Implement agents list**

Replace `cmd/agents.go`:
```go
// Lists all available agents (built-in + project-local)
// Shows: name, description, source
```

- [ ] **Step 2: Verify build succeeds**

Run: `cd /home/winler/projects/golem && go build ./...`
Expected: SUCCESS

- [ ] **Step 3: Commit**

```bash
git add cmd/agents.go
git commit -m "feat(cmd): add golem agents list command"
```

---

## Chunk 7: Run All Tests & Final Verification

### Task 20: Full Test Suite

**Files:** None (verification only)

- [ ] **Step 1: Run all tests**

Run: `cd /home/winler/projects/golem && go test ./... -v`
Expected: All PASS

- [ ] **Step 2: Run build**

Run: `cd /home/winler/projects/golem && go build ./...`
Expected: SUCCESS

- [ ] **Step 3: Verify help text**

Run: `cd /home/winler/projects/golem && go run . code --help`
Expected: Shows `--goal` flag, `--agent` flag

Run: `cd /home/winler/projects/golem && go run . runs --help`
Expected: Shows list/inspect/clean/watch subcommands

Run: `cd /home/winler/projects/golem && go run . agents --help`
Expected: Shows list subcommand

- [ ] **Step 4: Commit any fixes**

```bash
git add -A && git commit -m "fix: address issues from final test suite run"
```

---

## Dependency Graph

```
Group A (sequential — shared blueprint.go):
  Task 1 (Blueprint Parsing) → Task 2 (Contract Validation) → Task 10 (Template Rendering)

Group B (independent):
  Task 3 (Predicates)

Group C (sequential — shared command.go):
  Task 4 (RunWithTools)

Group D (independent):
  Task 5 (MCP Tool Filtering)

Group E (sequential — shared primitives.go):
  Task 6 (Test Helpers) → Task 7 (git-setup) → Task 8 (lint/test) → Task 9 (ci/pr)

Group F (depends on Groups A + E):
  Task 11 (Embedded Templates) — must complete before go build ./...

Engine (sequential, depends on all above):
  Task 12 (Engine Core) → Task 13 (Step Dispatch) → Task 14 (Control Flow) → Task 15 (Error Handling) → Task 16 (Integration Tests)

CLI (depends on Task 16):
  Task 17 (CLI Wiring) → Task 18 (runs commands) + Task 19 (agents command)

Final:
  Task 20 (Full Test Suite) — depends on all above
```

**Parallelism:** Groups A, B, C, D, and E can run in parallel (different files). Within each group, tasks are sequential (shared files). Task 11 is a sync point before engine work begins.
