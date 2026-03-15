package runner

import (
	"os"
	"strings"
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
	if !strings.Contains(result, "Add auth") {
		t.Errorf("result should contain goal, got: %s", result)
	}
}

func TestRenderTemplate_OptionalReadsOmitted(t *testing.T) {
	tmpl := "Goal: ${goal}"
	state := map[string]any{"goal": "Add auth"}
	result, err := RenderStepPrompt(tmpl, []string{"goal"}, []string{"lint-results"}, state, nil, "test-agent", "run-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result, "lint-results") {
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
	if !strings.Contains(result, "golangci-lint run") {
		t.Errorf("config var not replaced, got: %s", result)
	}
}

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
