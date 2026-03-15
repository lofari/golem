# Blueprint Authoring Guide

Blueprints are the core authoring surface of golem. Each blueprint is a YAML file that defines a named pipeline of steps, control flow, and error recovery rules. The engine executes the pipeline, manages state, and orchestrates Claude Code sessions — authors focus on the what, not the plumbing.

---

## Table of Contents

1. [What are blueprints](#1-what-are-blueprints)
2. [Quick example: build-feature](#2-quick-example-build-feature)
3. [Blueprint structure](#3-blueprint-structure)
4. [Step types](#4-step-types)
5. [State contracts](#5-state-contracts)
6. [Control flow](#6-control-flow)
7. [Predicates](#7-predicates)
8. [Error handling](#8-error-handling)
9. [Prompt templates](#9-prompt-templates)
10. [Creating your own agent](#10-creating-your-own-agent)
11. [Complete examples](#11-complete-examples)

---

## 1. What are blueprints

A blueprint describes a multi-step, iterative workflow. Each **step** is either:

- An **agentic** step — a full Claude Code session that reads state keys, runs tools, and writes structured output back to state.
- A **builtin** step — a built-in primitive (git, lint, tests, CI, PRs) executed by the engine itself.
- A **shell** step — an arbitrary shell command.

Steps are linked by a **state contract**: each step declares what keys it reads from prior steps and what keys it writes for later steps. The engine validates these contracts at parse time, preventing silent data-flow bugs.

Control flow constructs (`while`, `when`, `if`) make pipelines adaptive — re-running steps until quality gates pass, skipping phases based on configuration, or branching on outcomes.

---

## 2. Quick example: build-feature

The built-in `build-feature` agent (`templates/agents/build-feature.yaml`) is the canonical full-cycle blueprint. Annotated:

```yaml
name: build-feature
description: "Plan, implement with lint/test feedback loops, review, ship."

# Keys that the engine will populate from user input before the pipeline starts.
# 'goal' is always injected automatically; listing it here declares it explicitly.
initial-state: [goal]

# Config keys and their defaults. Users override these in .ctx/config.yaml.
config:
  lint-cmd: null        # e.g. "golangci-lint run ./..."
  lint-fix-cmd: null    # auto-fix command run before lint check
  test-cmd: null        # e.g. "go test ./..."
  ci-enabled: false     # set true to trigger GitHub Actions checks

steps:
  # Create a golem/<agent>-<timestamp> branch. Writes: branch, base.
  - git-setup:
      type: builtin

  # Claude session: reads goal, produces a structured plan.
  - plan:
      type: agentic
      reads: [goal]
      optional-reads: [_error_context]   # injected on retry
      writes: [plan]
      tools: [semantic_search, find_callers, find_dependencies, find_co_changed]

  # Claude session: reads goal + plan, writes code changes and test-results.
  # optional-reads let previous loop iterations feed back context.
  - implement:
      type: agentic
      reads: [goal, plan]
      optional-reads: [_error_context, reflection, review-feedback, lint-results, test-results]
      writes: [code, test-results]
      tools: [semantic_search, find_callers, find_dependencies, find_dependents,
              find_co_changed, find_execution_failures,
              lsp_definition, lsp_references, lsp_hover, lsp_diagnostics]

  - lint:
      type: builtin
      reads: [code]
      writes: [lint-results]

  - run-tests:
      type: builtin
      reads: [code]
      writes: [test-results]

  - review:
      type: agentic
      reads: [code, test-results]
      optional-reads: [_error_context, lint-results]
      writes: [review-feedback]
      tools: [semantic_search, find_callers, find_dependencies]

  # Loop: if review says "needs-work", re-implement up to 3 times.
  - while:
      predicate: needs-work
      max: 3
      steps:
        - implement     # step reference by name
        - lint
        - run-tests
        - review

  # Only run CI steps if ci-enabled=true in config.
  - when:
      predicate: ci-enabled
      steps:
        - ci-tests:     # inline step definition
            type: builtin
            reads: [code]
            writes: [ci-results]
        - while:        # nested control flow
            predicate: ci-failed
            max: 2
            steps:
              - implement
              - lint
              - run-tests
              - ci-tests

  - create-pr:
      type: builtin
      reads: [code, goal]
      optional-reads: [plan, test-results, ci-results, lint-results]
      writes: [pr-result]

# Blueprint-level error recovery.
errors:
  transient: { action: retry, max: 3 }
  malformed-output: { action: re-run, max: 2, hint: "Write session-output.json with required keys." }
  contract-violation: { action: halt }
```

---

## 3. Blueprint structure

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Unique agent name. Used in branch names and logs. |
| `description` | string | no | Human-readable summary shown by `golem agents`. |
| `initial-state` | list of strings | no | Keys available at pipeline start. `goal` is always injected. |
| `config` | map | no | Config keys with default values. Overridden by `.ctx/config.yaml`. |
| `predicates` | map | no | Named custom predicate expressions (see [section 7](#7-predicates)). |
| `steps` | list | yes | Ordered pipeline of steps and control flow nodes. |
| `errors` | map | no | Blueprint-level error handlers (see [section 8](#8-error-handling)). |

The parser rejects unknown top-level fields and suggests corrections for common typos (`intial-state` → `initial-state`, `step` → `steps`, etc.).

---

## 4. Step types

### 4.1 Agentic steps

An agentic step launches a full Claude Code session (`claude -p`) with a rendered prompt, a tool allowlist, and turn/timeout limits. Claude writes output to `session-output.json` in the project directory; the engine reads that file and merges the declared `writes` keys into pipeline state.

**Fields:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | `"agentic"` | required | Identifies the step as an agentic session. |
| `reads` | list | `[]` | State keys that must exist and are injected into the prompt. |
| `writes` | list | `[]` | Keys that Claude must write to `session-output.json`. |
| `optional-reads` | list | `[]` | State keys injected if present; prompt lines are removed if absent. |
| `tools` | list | see defaults | MCP/graph tools available to Claude. |
| `prompt` | string | see below | Inline prompt template. If omitted, `templates/prompts/<step-name>.md` is used. |
| `max-turns` | int | see defaults | Maximum Claude turns before timeout. |
| `timeout` | duration string | see defaults | Wall-clock timeout, e.g. `"45m"`. |
| `model` | string | engine default | Override the Claude model for this step. |

**Defaults by step name** (applied when `max-turns`/`timeout` are not set):

| Step name | `max-turns` | `timeout` |
|-----------|-------------|-----------|
| `plan` | 50 | 20m |
| `implement` | 200 | 30m |
| `review` | 50 | 20m |
| `reflect` | 30 | 10m |
| `research` | 75 | 20m |
| *(any other name)* | 75 | 20m |

**Default tools by step name** (applied when `tools` is omitted):

| Step name | Default tools |
|-----------|---------------|
| `plan` | `semantic_search`, `find_callers`, `find_dependencies`, `find_co_changed` |
| `implement` | `semantic_search`, `find_callers`, `find_dependencies`, `find_dependents`, `find_co_changed`, `find_execution_failures`, `lsp_definition`, `lsp_references`, `lsp_hover`, `lsp_diagnostics` |
| `review` | `semantic_search`, `find_callers`, `find_dependencies` |
| `reflect` | `semantic_search` |
| `research` | `semantic_search`, `find_callers`, `find_dependencies`, `find_co_changed`, `find_execution_failures`, `get_runtime_trace` |

The special write key `code` is engine-managed: instead of reading it from `session-output.json`, the engine runs `git diff --name-only HEAD` and `git diff --stat HEAD` to populate `state["code"]` automatically.

### 4.2 Builtin steps

Builtin steps invoke named primitives implemented in the engine. The `type: builtin` field is required; the step name determines which primitive runs.

| Primitive | What it does | Config used | State written |
|-----------|-------------|-------------|---------------|
| `git-setup` | Creates a `golem/<agent>-<timestamp>` branch from the current HEAD. | — | `branch`, `base` (engine-managed) |
| `lint` | Runs `lint-fix-cmd` (if set), then `lint-cmd`. Skips if `lint-cmd` is null. | `lint-cmd`, `lint-fix-cmd` | declared `writes` key: `{status, output, autofix-applied?}` |
| `run-tests` | Runs `test-cmd`. Skips if null. Configurable timeout. | `test-cmd`, `test-timeout` | declared `writes` key: `{status, output, duration-ms}` |
| `ci-tests` | Pushes the branch and polls `gh run list` for workflow status. Requires `gh` CLI. | — | declared `writes` key: `{status, conclusion, output}` |
| `create-pr` | Pushes branch and calls `gh pr create`. Skips if `code.files` is empty. | — | declared `writes` key: `{status, url, title}` |

For `lint` and `run-tests`, status values are `"pass"`, `"fail"`, or `"skipped"`.

Example with explicit reads/writes (as used in `build-feature.yaml`):

```yaml
- lint:
    type: builtin
    reads: [code]       # declares dependency, not consumed by the primitive itself
    writes: [lint-results]
```

### 4.3 Shell steps

Shell steps run an arbitrary command via `sh -c`. The step name is free-form.

```yaml
- generate-schema:
    type: shell
    command: "go generate ./internal/schema/..."
    timeout: "2m"
    writes: [schema-output]
    errors:
      non-zero: halt    # stop the pipeline if the command exits non-zero
```

**Fields specific to shell steps:**

| Field | Default | Description |
|-------|---------|-------------|
| `command` | required | Shell command string, executed via `sh -c`. |
| `timeout` | 5m | Wall-clock timeout. |
| `errors.non-zero` | (transient retry) | Set to `"halt"` to treat non-zero exit as unrecoverable. |

The engine writes `{status: "pass"|"fail", output: "..."}` to each declared `writes` key.

---

## 5. State contracts

The state contract system prevents pipelines from breaking silently when a step cannot find its inputs.

### Validation rules

At parse time, `ValidateContracts` walks the pipeline and maintains a set of **available keys**. Each step's `reads` must be satisfiable from that set:

- Keys in `initial-state` are available from the start.
- `branch` and `base` are engine-managed and always available after `git-setup`.
- A step's `writes` become available to all subsequent steps.
- Keys written **inside control flow** (`while`/`when`/`if`) are marked **conditional** — they may not be written if the predicate is never true. A downstream step that puts such a key in `reads` gets a validation error; it must use `optional-reads` instead.

### Error messages

```
contract violation: step "review" reads "lint-results" which is not produced
by any prior step or initial-state

contract violation: step "implement" reads "ci-results" which is only
conditionally written; use optional-reads instead
```

### The `_error_context` key

`_error_context` is injected by the engine into state when a step is retried or re-run. Steps that should adapt their behavior on failure should list it in `optional-reads`. The corresponding `${_error_context}` token in a prompt template is removed entirely when no error is active.

---

## 6. Control flow

All three constructs share the same step reference syntax: entries are either step names (strings) that reference previously-defined steps, or inline step definitions.

### `while`

Repeats the body until the predicate is false or `max` iterations are reached.

```yaml
- while:
    predicate: needs-work
    max: 3
    steps:
      - implement
      - lint
      - run-tests
      - review
```

The predicate is evaluated **before** each iteration. If it is false on the first check, the body never runs.

### `when`

Executes the body once if the predicate is true; skips otherwise.

```yaml
- when:
    predicate: ci-enabled
    steps:
      - ci-tests:
          type: builtin
          reads: [code]
          writes: [ci-results]
```

### `if`

Branches on the predicate. `then` is required; `else` is optional.

```yaml
- if:
    predicate: lint-failed
    then:
      - implement
      - lint
    else:
      - review
```

### Nesting

Control flow nodes can be nested to arbitrary depth:

```yaml
- when:
    predicate: ci-enabled
    steps:
      - ci-tests:
          type: builtin
          reads: [code]
          writes: [ci-results]
      - while:
          predicate: ci-failed
          max: 2
          steps:
            - implement
            - ci-tests
```

---

## 7. Predicates

A predicate is a boolean expression evaluated against the current pipeline state and config. Predicates appear in `while`, `when`, and `if` nodes.

### Built-in predicates

Five built-in predicates are always available by name:

| Name | True when |
|------|-----------|
| `needs-work` | `state["review-feedback"]["verdict"] == "needs-work"` |
| `failed` | `state["test-results"]["status"] == "fail"` |
| `lint-failed` | `state["lint-results"]["status"] == "fail"` |
| `ci-enabled` | `config["ci-enabled"] == true` |
| `ci-failed` | `state["ci-results"]["status"] == "fail"` |

### Custom predicates

Define named predicates in the `predicates` map at the blueprint root. Each value is an **expression** parsed at load time:

```yaml
predicates:
  coverage-low: "test-results.coverage < 80"
  strict-mode: "config.strict == true"
  approved: "review-feedback.verdict == \"approved\""
```

Expression syntax:

```
<dotted.path> <operator> <value>
```

- **Path**: dot-separated key segments into state. Use `config.` prefix to resolve against config instead of state.
- **Operators**: `==`, `!=`, `>`, `<`, `>=`, `<=`
- **Values**: quoted strings (`"approved"`), numbers (`80`), booleans (`true`, `false`)

Paths that do not exist evaluate to `false`. Type mismatches also evaluate to `false`.

Use a custom predicate in control flow just like a built-in:

```yaml
- while:
    predicate: coverage-low
    max: 2
    steps:
      - implement
      - run-tests
```

---

## 8. Error handling

### Error types

| Type | When raised |
|------|-------------|
| `TransientError` | Temporary failures: network issues, timeouts, transient subprocess exits. |
| `MalformedOutputError` | Claude did not write `session-output.json`, or it contained invalid JSON, or required `writes` keys are missing. |
| `UnrecoverableError` | Permanent failures: command not found, missing branch state, unknown builtin name. Always halts. |

### Handler configuration

```yaml
errors:
  transient:        { action: retry,  max: 3 }
  malformed-output: { action: re-run, max: 2, hint: "Include all required keys." }
  contract-violation: { action: halt }
```

| Field | Values | Description |
|-------|--------|-------------|
| `action` | `retry`, `re-run`, `halt` | `retry` re-runs the same step. `re-run` does the same but injects a hint into `_error_context`. `halt` stops the pipeline immediately. |
| `max` | int | Maximum recovery attempts before giving up. |
| `hint` | string | Appended to `_error_context` on `re-run`, steering Claude toward the correct output format. |

### Priority chain

The engine resolves the handler using a three-level priority chain:

```
step-level errors > blueprint-level errors > built-in defaults
```

Built-in defaults (when no handler is configured):

| Error type | Default action | Default max |
|------------|---------------|-------------|
| `transient` | `retry` | 3 |
| `malformed-output` | `re-run` | 2 |
| `unrecoverable` | `halt` | — |
| `contract-violation` | `halt` | — |

### Per-step override

```yaml
- implement:
    type: agentic
    reads: [goal, plan]
    writes: [code, test-results]
    errors:
      transient: { action: retry, max: 5 }
      malformed-output: { action: re-run, max: 3, hint: "Write session-output.json with 'test-results' key." }
```

### `_error_context` injection

On both `retry` and `re-run`, the engine writes a message into `state["_error_context"]`:

```
Previous error (attempt 1/3): step "implement" did not write session-output.json
Hint: Write session-output.json with required keys.
```

Steps receive this via `optional-reads: [_error_context]` and the `${_error_context}` prompt token.

---

## 9. Prompt templates

### Lookup order

For agentic steps, the engine loads the prompt as follows:

1. If the step has an inline `prompt:` field, use it.
2. Otherwise, look for `templates/prompts/<step-name>.md` in the embedded filesystem.
3. If neither exists, the pipeline errors immediately.

Custom step names that are not `plan`, `implement`, `review`, `reflect`, or `research` must supply an inline `prompt:` or a file at `.ctx/prompts/<step-name>.md` (project-local templates are not yet auto-loaded — use inline prompts for custom steps).

### Token interpolation

Tokens use the syntax `${key}`. All values are JSON-encoded before substitution.

| Token | Source | Behavior if absent |
|-------|--------|--------------------|
| `${<key>}` (from `reads`) | `state[key]` | Error — reads keys are required. |
| `${<key>}` (from `optional-reads`) | `state[key]` | The entire line containing the token is removed from the prompt. If the preceding line begins with `#`, that header line is also removed. |
| `${config.<key>}` | `config[key]` | Left unresolved (produces template error). |
| `${agent.name}` | Engine config | Always available. |
| `${run.id}` | Engine config | Always available. |

Any unresolved `${...}` token after substitution is a template error that halts the pipeline.

### Example prompt

```markdown
You are implementing a code change.

# Goal
${goal}

# Plan
${plan}

# Prior Lint Results
${lint-results}

# Previous Error Context
${_error_context}

When finished, write session-output.json:
{"test-results": {"status": "pass|fail", "summary": "..."}}
```

With `reads: [goal, plan]`, `optional-reads: [lint-results, _error_context]`: if `lint-results` and `_error_context` are absent from state, those two lines (and their `#` headers) are silently dropped from the rendered prompt.

### `session-output.json` contract

The Claude session must write a file named `session-output.json` in the project root before exiting. The engine reads it, extracts the keys listed in `writes`, and merges them into state. The file is deleted after reading.

```json
{
  "plan": [{"step": 1, "desc": "Add the new endpoint"}],
  "test-results": {"status": "pass", "summary": "12/12 passed"}
}
```

The `code` key is special: never write it from Claude. The engine detects file changes via `git diff` automatically.

---

## 10. Creating your own agent

1. Create the file `.ctx/agents/my-agent.yaml` in your project directory.
2. Run it with:

```sh
golem run my-agent --goal "Add rate limiting to the API"
```

3. List available agents (built-in + project-local) with:

```sh
golem agents
```

The engine searches for agents in this order:
- Built-in templates (embedded in the binary)
- `.ctx/agents/` in the current project directory

Project-local agents take precedence over built-ins when names collide.

**Minimal agent skeleton:**

```yaml
name: my-agent
description: "What this agent does."
initial-state: [goal]

config:
  test-cmd: null

steps:
  - git-setup:
      type: builtin

  - implement:
      type: agentic
      reads: [goal]
      optional-reads: [_error_context, test-results]
      writes: [code, test-results]
      prompt: |
        You are implementing a change.
        Goal: ${goal}
        Write session-output.json: {"test-results": {"status": "pass|fail"}}
        ## Previous Error Context
        ${_error_context}

  - run-tests:
      type: builtin
      reads: [code]
      writes: [test-results]

errors:
  transient: { action: retry, max: 2 }
  malformed-output: { action: re-run, max: 2, hint: "Write session-output.json." }
```

---

## 11. Complete examples

### 11.1 Documentation generator

Generates API documentation from source, validates it, opens a PR.

```yaml
name: gen-docs
description: "Generate and validate API documentation, then open a PR."
initial-state: [goal]

config:
  lint-cmd: "markdownlint docs/"
  test-cmd: null

steps:
  - git-setup:
      type: builtin

  - research:
      type: agentic
      reads: [goal]
      optional-reads: [_error_context]
      writes: [research-context]

  - implement:
      type: agentic
      reads: [goal, research-context]
      optional-reads: [_error_context, lint-results]
      writes: [code]
      prompt: |
        You are generating API documentation for a software project.

        # Goal
        ${goal}

        # Research
        ${research-context}

        # Prior Lint Results
        ${lint-results}

        Generate Markdown documentation files in the docs/ directory.
        Do NOT write session-output.json for the "code" key — the engine detects changes.
        Write session-output.json: {}

        ## Previous Error Context
        ${_error_context}

  - lint:
      type: builtin
      reads: [code]
      writes: [lint-results]

  - when:
      predicate: lint-failed
      steps:
        - implement
        - lint

  - create-pr:
      type: builtin
      reads: [code, goal]
      optional-reads: [research-context, lint-results]
      writes: [pr-result]

errors:
  transient: { action: retry, max: 2 }
  malformed-output: { action: re-run, max: 2, hint: "Write session-output.json: {}" }
```

### 11.2 Database migration agent

Generates a schema migration, runs it, validates with tests.

```yaml
name: db-migrate
description: "Generate a database migration and validate it."
initial-state: [goal]

config:
  test-cmd: "go test ./internal/db/..."
  test-timeout: "3m"
  migrate-cmd: "go run ./cmd/migrate up"

steps:
  - git-setup:
      type: builtin

  - plan:
      type: agentic
      reads: [goal]
      optional-reads: [_error_context]
      writes: [plan]

  - implement:
      type: agentic
      reads: [goal, plan]
      optional-reads: [_error_context, test-results]
      writes: [code, test-results]
      prompt: |
        You are writing a database schema migration.

        # Goal
        ${goal}

        # Plan
        ${plan}

        Create a migration file in db/migrations/ and update any affected models.
        Write session-output.json: {"test-results": {"status": "pass|fail", "summary": "..."}}

        ## Previous Error Context
        ${_error_context}

  - apply-migration:
      type: shell
      command: "${config.migrate-cmd}"
      timeout: "1m"
      errors:
        non-zero: halt

  - run-tests:
      type: builtin
      reads: [code]
      writes: [test-results]

  - when:
      predicate: failed
      steps:
        - implement
        - apply-migration
        - run-tests

  - create-pr:
      type: builtin
      reads: [code, goal]
      optional-reads: [plan, test-results]
      writes: [pr-result]

errors:
  transient: { action: retry, max: 2 }
  malformed-output: { action: re-run, max: 2 }

predicates:
  migration-tested: "test-results.status == \"pass\""
```

### 11.3 Security audit

Research-heavy read-only analysis that writes a report rather than opening a PR.

```yaml
name: security-audit
description: "Audit the codebase for security issues and produce a report."
initial-state: [goal]

steps:
  - research:
      type: agentic
      reads: [goal]
      optional-reads: [_error_context]
      writes: [research-context]
      max-turns: 100
      timeout: "30m"

  - analyze:
      type: agentic
      reads: [goal, research-context]
      optional-reads: [_error_context]
      writes: [audit-report]
      max-turns: 50
      prompt: |
        You are a security auditor reviewing a codebase.

        # Goal
        ${goal}

        # Research Findings
        ${research-context}

        Produce a structured security report covering:
        - Authentication and authorization issues
        - Input validation gaps
        - Dependency vulnerabilities
        - Secrets exposure risk

        Write session-output.json:
        {"audit-report": {"summary": "...", "findings": [...], "severity": "low|medium|high|critical"}}

        ## Previous Error Context
        ${_error_context}

  - write-report:
      type: shell
      command: "echo 'Audit complete. See .ctx/runs/current/state.json for findings.'"

errors:
  transient: { action: retry, max: 2 }
  malformed-output: { action: re-run, max: 3, hint: "Write audit-report with summary, findings, and severity fields." }
```

---

## Reference: valid step fields

| Field | Applies to | Notes |
|-------|-----------|-------|
| `type` | all | `agentic`, `builtin`, or `shell` |
| `reads` | agentic, builtin | Must be available at execution time |
| `writes` | agentic, builtin, shell | Keys merged into state after the step |
| `optional-reads` | agentic | Present in state: injected. Absent: prompt line removed. |
| `tools` | agentic | Overrides default tool list |
| `prompt` | agentic | Inline prompt template |
| `max-turns` | agentic | Overrides step-name defaults |
| `timeout` | agentic, shell | Go duration string, e.g. `"30m"`, `"90s"` |
| `model` | agentic | Overrides engine-level model |
| `command` | shell | Shell command string |
| `errors` | all | Per-step error handler overrides |

## Reference: valid control flow fields

| Field | Applies to | Notes |
|-------|-----------|-------|
| `predicate` | while, when, if | Built-in name or custom predicate from `predicates` map |
| `max` | while | Maximum iterations |
| `steps` | while, when | Ordered list of step refs or inline steps |
| `then` | if | Steps to run when predicate is true |
| `else` | if | Steps to run when predicate is false (optional) |
