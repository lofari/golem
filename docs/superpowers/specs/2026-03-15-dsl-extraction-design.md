# Extract DSL Ideas & Remove Clojure Runtime

## Summary

Extract two valuable patterns from the Clojure DSL into the Go blueprint engine (custom predicates, error classification with prompt amendment), then remove the entire Clojure DSL runtime and its Go integration code.

## Motivation

The Clojure DSL was a prototype that validated workflow orchestration ideas. The Go blueprint engine now covers ~85% of its features natively. The remaining 15% consists of two genuinely useful patterns (custom predicates, smarter error recovery) and things we don't need (custom primitives — shell steps cover it, predicate composition — YAGNI). The JVM subprocess adds startup latency and a deployment dependency that fights Go's single-binary distribution model.

## Part 1: Custom Predicates

### Problem

`predicates.go` has a hardcoded `switch` with 5 predicates. Users cannot define their own predicates in blueprint YAML, limiting what control flow can express.

### Design

Add an optional `predicates:` section to blueprint YAML. Each predicate is a simple expression:

```yaml
predicates:
  needs-work: review-feedback.verdict == "needs-work"
  tests-failing: test-results.status == "fail"
  high-coverage: test-results.coverage > 80
  ci-enabled: config.ci-enabled == true
```

### Expression Language

Minimal, no parser generator needed:

- **Left side**: Dotted path into state — `key.subkey` resolves to `state["key"]["subkey"]`. Paths starting with `config.` resolve against the config map instead.
- **Operators**: `==`, `!=`, `>`, `<`, `>=`, `<=`
- **Right side**: Quoted strings (`"value"`), numbers (`80`, `3.14`), booleans (`true`, `false`)
- **No boolean combinators** (AND/OR/NOT) — keep it simple, add later if needed
- **Type mismatches**: Comparison between incompatible types (e.g., string vs number) returns `false`
- **Missing paths**: If the dotted path doesn't resolve (key missing, intermediate value is not a map), return `false`

### Implementation

- **New file**: `internal/runner/predicate_expr.go` (~80 lines)
  - `ParsePredicateExpr(expr string) (*PredicateExpr, error)` — tokenize and validate
  - `(p *PredicateExpr) Eval(state, config map[string]any) bool` — evaluate against runtime state
- **Blueprint struct**: Add `Predicates map[string]string` field
- **`EvalPredicate` becomes `Engine` method**: Check custom predicates first, fall back to built-in 5. Three call sites to update: `execWhile` (line 444), `execWhen` (line 459), `execIf` (line 467)
- **Parse-time validation**: Syntax check on all predicate expressions
- **Built-in predicates remain**: The 5 existing predicates work without YAML declaration (backward compatible)

### Files Changed

| File | Change |
|------|--------|
| `internal/runner/predicate_expr.go` | New: expression parser + evaluator |
| `internal/runner/predicate_expr_test.go` | New: tests |
| `internal/runner/predicates.go` | Refactor: `EvalPredicate` becomes `Engine.evalPredicate`, checks custom first |
| `internal/runner/blueprint.go` | Add `Predicates` field to `Blueprint`, parse from YAML, add to `validTopLevelFields` |
| `internal/runner/engine.go` | Update control flow methods to call `e.evalPredicate` instead of `EvalPredicate` |

## Part 2: Error Classification with Prompt Amendment

### Problem

The engine retries steps but doesn't tell the step what went wrong. Re-running the exact same prompt hopes for a different result. The DSL's "amend prompt with error context" pattern is more effective.

### Design

#### Error context injection

On retry/re-run, inject error details into state as `_error_context` before re-executing. The step's prompt can reference this via `${_error_context}` in optional-reads. After successful retry, clean up the key.

```
Previous error: step "implement" failed: session-output.json was missing key "code"
Hint: Output must contain a "code" key with the implementation.
```

This replaces the current `_hint` mechanism. Note: `_hint` was never referenced in any built-in prompt template — it was injected into state but no template consumed it. `_error_context` is the first-time integration of error context into prompts.

#### Per-step error handlers

Extend `StepErrors` to support full error handler configuration per step:

```yaml
steps:
  - implement:
      type: agentic
      reads: [goal, plan]
      writes: [code]
      errors:
        transient: { action: retry, max: 5 }
        malformed-output: { action: re-run, max: 3, hint: "Output must contain 'code' key" }
```

#### Handler priority chain

step-level > blueprint-level > built-in defaults. Fallback is **per-field**: if a step defines `transient` but not `malformed-output`, the step's transient handler is used but malformed-output falls through to blueprint-level, then built-in defaults.

Built-in defaults (applied only when neither step nor blueprint defines a handler):
- `transient`: `{ action: retry, max: 3 }`
- `malformed-output`: `{ action: re-run, max: 2 }`
- `unrecoverable`: `{ action: halt }`
- `contract-violation`: `{ action: halt }`

**Backward compatibility note:** The current code defaults to `action: "halt"` when the blueprint's handler action is empty. The new built-in defaults change this to retry/re-run for transient/malformed-output. However, the existing built-in blueprint templates (`build-feature.yaml`, `fix-bug.yaml`) already explicitly configure `transient: { action: retry, max: 3 }` and `malformed-output: { action: re-run, max: 2 }`, so this only affects blueprints that omit the `errors:` section entirely — which would get smarter defaults instead of halt-on-everything.

### Implementation

- **`StepErrors` struct**: Expand to include `Transient`, `MalformedOutput`, `ContractViolation` fields (same `ErrorHandler` type). The existing `NonZero string` field is preserved for shell step backward compatibility — it controls whether a non-zero exit becomes `TransientError` (default) or `UnrecoverableError` (`halt`)
- **`handleError`**: Check `step.StepErrors` first, fall back to `bp.Errors`, then built-in defaults
- **`_error_context`**: Injected into state before retry, formatted as readable text with error type + message + hint
- **Prompt templates**: Add `_error_context` to `optional-reads` in built-in prompt templates so retries get context automatically

### Files Changed

| File | Change |
|------|--------|
| `internal/runner/blueprint.go` | Expand `StepErrors` struct, add to `validStepFields` error sub-fields |
| `internal/runner/engine.go` | Refactor `handleError`/`handleTransient`/`handleMalformedOutput`: priority chain + error context injection |
| `internal/runner/engine_test.go` | Tests for priority chain and error context |
| `templates/prompts/*.md` | Add `_error_context` to optional-reads where appropriate |

## Part 3: Remove Clojure DSL Runtime

### Deleted

**Entire directory:**
- `golem-dsl/` — All Clojure source, tests, agents, deps.edn, Makefile, CLOJURE-GUIDE.md

**Go integration files:**
- `internal/runner/dsl_runner.go`
- `internal/runner/dsl_events.go`
- `internal/runner/dsl_runner_test.go`
- `internal/runner/dsl_events_test.go`
- `internal/runner/dsl_runner_mock_test.go`
- `internal/runner/dsl_golden_test.go`
- `internal/runner/testdata/mock-dsl/main.go`
- `internal/runner/testdata/golden_events*.ndjson`
- `test/integration/dsl_integration_test.go`
- `test/integration/dsl_state_test.go`

**Config removals** (`internal/config/config.go`):
- Field: `DSLCommand` only. `Agent` and `AgentOpts` are used by the blueprint engine (`cmd/code.go`, `cmd/run.go`) and must be preserved.
- Remove defaults, getters, config layer field, merge logic, and PrintConfig output for `DSLCommand`
- Remove config key: `dsl-command`
- Update `Keys()` engine description to remove DSL reference

**CLI removals:**
- `cmd/code.go`: DSL engine branch, `shouldUseDSL()` helper
- `cmd/helpers.go`: Remove `dsl` from `--engine` flag help text
- `cmd/session.go`: Update description (remove "Used by golem-dsl" reference)
- `cmd/code_test.go`: `TestShouldUseDSL`

**Config test removals** (`internal/config/config_test.go`):
- `TestDefaults_Engine` (DSL-specific assertions only), `TestLoad_EngineFromYAML`
- DSL-related cases in `TestGetValue_Engine`
- `TestDefaults_Agent` and `TestLoad_AgentOptsFromYAML` are preserved (used by blueprint engine)

### Preserved

- `--engine` flag (still supports `go` and `blueprint`)
- `golem session` command (useful independently)
- All blueprint engine code

## Backward Compatibility

- Existing blueprint YAML files work unchanged (no predicates section needed, built-ins remain)
- Existing error handler config works unchanged (new per-step config is additive)
- `--engine dsl` will error with "unknown engine" instead of silently failing if binary not found
- No user-facing config keys removed that affect blueprint workflows
