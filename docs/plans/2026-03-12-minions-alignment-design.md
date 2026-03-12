# Minions Alignment: YAML Blueprint Engine with Deterministic Primitives

> Date: 2026-03-12
> Status: Draft

## Summary

Replace both the Go builder loop and the Clojure DSL with a single Go-native blueprint engine driven by YAML agent definitions. Add deterministic pipeline primitives (`lint`, `run-tests`, `ci-tests`, `create-pr`), tool scoping per step, and a `one-shot` agent. These changes close the five largest gaps between golem and Stripe's Minions architecture while simplifying the stack from two languages/runtimes to one.

## Decisions

- **YAML blueprints replace Clojure DSL** — The DSL validated the blueprint pattern; YAML agent definitions with a Go engine deliver the same semantics without a second language, sidecar binary, or GraalVM dependency.
- **Explicit config over auto-detection** — Lint/test commands are configured per-project via `agent-opts`, not auto-detected from project stack. Predictable and debuggable for autonomous agents.
- **Convention-based with config override for lint/test** — Auto-detect project stack for common cases, explicit config overrides when needed.
- **GitHub Actions via `gh` CLI** — CI integration uses `gh run` commands. Lowest friction, no custom API client needed.
- **Full PR workflow** — `create-pr` handles branch push, structured body generation, CODEOWNERS reviewer detection.
- **Primitive defaults with agent-level override** — Agentic steps declare default `tools`, agent definitions can narrow or widen per step. Omitted = all tools (backwards-compatible).
- **`one-shot` is just another agent** — No special CLI treatment. Invoked via `golem code --agent one-shot --goal "..."`.
- **Built-in + project-local agents** — Default agents embedded via Go `embed`. Project-local overrides in `.ctx/agents/`. Resolution: project-local wins.
- **Built-in primitives + generic `shell` type** — Complex primitives (`ci-tests`, `create-pr`) are Go functions. Simple user-defined steps use `shell` type.
- **JSON for state, YAML for config and agents** — State is machine-managed (JSON avoids YAML coercion traps). Config and agent definitions are human-authored (YAML is friendlier to write).

---

## Architecture

```
golem code --goal "Add auth"
  │
  ├── Load agent: .ctx/agents/build-feature.yaml (or embedded default)
  ├── Parse blueprint → Go DAG structure
  ├── Validate contracts (reads/writes chain)
  │
  └── Engine walks DAG:
       ├── Agentic node       → ClaudeRunner.Run() with scoped tools + rendered prompt
       ├── Builtin node       → Go function (lint, run-tests, ci-tests, create-pr)
       ├── Shell node         → exec.Command() with output capture
       └── Control flow node  → engine evaluates predicate on state
```

**No sidecar binary. No subprocess protocol.** The engine is Go code in `internal/runner/` that calls `ClaudeRunner` directly — same path that works today.

### What Gets Removed

- `golem-dsl/` directory (entire Clojure codebase)
- `internal/runner/dsl_runner.go`, `dsl_events.go` (sidecar protocol)
- `internal/runner/builder.go` (old Go loop — phased)
- `internal/runner/strategy.go`, `validate.go` (logic moves into engine — phased)
- `engine: dsl` config option
- GraalVM build infrastructure

### What Gets Added

- `internal/runner/engine.go` — blueprint executor
- `internal/runner/blueprint.go` — YAML parsing, DAG construction, contract validation
- `internal/runner/primitives.go` — built-in deterministic primitive implementations
- `templates/agents/` — embedded default agent YAML files
- MCP tool filtering via `--tools` flag

### What Stays Unchanged

- `ClaudeRunner` — still spawns `claude -p`
- MCP server — still provides graph/state tools
- Knowledge graph — still syncs and embeds
- Warden sandbox — still wraps sessions
- Event system — engine emits same `Event` types
- `golem status`, `golem config`, `golem init`, `golem plan`, `golem review`
- Config system (two-layer YAML, `.ctx/config.yaml`)

---

## Agent YAML Format

```yaml
name: build-feature
description: "Plan, implement with lint/test feedback loops, review, ship."
initial-state: [goal]

config:
  lint-cmd: null
  lint-fix-cmd: null
  test-cmd: null
  ci-enabled: false

steps:
  - plan:
      type: agentic
      reads: [goal]
      writes: [plan]
      tools: [semantic_search, find_callers, find_dependencies, find_co_changed]

  - implement:
      type: agentic
      reads: [goal, plan]
      optional-reads: [reflection, review-feedback, lint-results, test-results]
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
      reads: [code, test-results, lint-results]
      writes: [review-feedback]
      tools: [semantic_search, find_callers, find_dependencies]

  - while:
      predicate: needs-work
      max: 3
      steps:
        - implement
        - lint
        - run-tests
        - review

  - when:
      predicate: ci-enabled
      steps:
        - ci-tests:
            type: builtin
            reads: [code]
            writes: [ci-results]
        - when:
            predicate: ci-failed
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

errors:
  transient: { action: retry, max: 3 }
  malformed-output: { action: re-run, hint: "Check contract schema." }
  contract-violation: { action: halt }
```

### Step Types

| Type | Execution | Use Case |
|---|---|---|
| `agentic` | Spawns Claude session with scoped tools and rendered prompt | plan, implement, review, reflect, research |
| `builtin` | Calls a registered Go function | lint, run-tests, ci-tests, create-pr |
| `shell` | Runs user-defined command, captures output | Custom deterministic steps |

### Control Flow Nodes

| Node | Behavior |
|---|---|
| `while` | Loop steps while predicate is true, up to `max` iterations |
| `when` | Execute steps only if predicate is true |
| `if` | Execute `then` steps if true, `else` steps if false |

Control flow nodes reference steps by name. Steps defined earlier in the file can be reused in control flow blocks without re-declaring their full config.

### Prompt Templates

Agentic steps use templates from `resources/prompts/<step-name>.md`. Templates receive the step's `reads` keys rendered from state. Custom template per step:

```yaml
- plan:
    type: agentic
    prompt: custom-plan.md    # looks in .ctx/prompts/ then embedded templates
    reads: [goal]
    writes: [plan]
```

---

## Deterministic Primitives

All four primitives use `type: builtin` — they execute locally without spawning a Claude session.

### `lint`

**Inputs from config:**
- `lint-cmd` — lint command (e.g., `"golangci-lint run"`)
- `lint-fix-cmd` — autofix command (e.g., `"golangci-lint run --fix"`)

**Behavior:**
1. If `lint-fix-cmd` is set, run it first (applies mechanical fixes)
2. Run `lint-cmd` to verify
3. Parse stdout into structured issues: `[{"file": "f.go", "line": 12, "message": "..."}]`
4. Timeout: 30s default

**Output:**

| Scenario | Result |
|---|---|
| Not configured | `{"status": "skipped", "reason": "no lint-cmd"}` |
| Passes | `{"status": "pass", "issues": []}` |
| Fails after autofix | `{"status": "fail", "issues": [...], "fixed": [...]}` |
| Fails, no fix cmd | `{"status": "fail", "issues": [...]}` |
| Timeout | `transient` error → retry handler |
| Command not found | `unrecoverable` → halt |

### `run-tests`

**Inputs from config:**
- `test-cmd` — test command (e.g., `"go test ./..."`)
- `test-timeout` — timeout duration (default: 5 minutes)

**Behavior:**
1. Run `test-cmd` in project directory
2. Parse output for failures: test name, file, message
3. Capture total duration

**Output:**

| Scenario | Result |
|---|---|
| Not configured | `{"status": "skipped"}` |
| All pass | `{"status": "pass", "duration-ms": N}` |
| Failures | `{"status": "fail", "failures": [...], "duration-ms": N}` |
| Timeout | `transient` error → retry |
| Command not found | `unrecoverable` → halt |

### `ci-tests`

**Behavior:**
1. `git push origin <branch>` (branch already exists from agent commits)
2. `gh run list --branch <branch> --limit 1 --json databaseId,status` — find triggered workflow
3. `gh run watch <run-id>` — poll until completion (timeout: 15min default)
4. On completion: `gh run view <run-id> --json conclusion,jobs` for results
5. On failure: `gh api repos/{owner}/{repo}/actions/jobs/{job-id}/logs` to fetch failed job logs
6. Parse logs into failure structs

**Output:**

| Scenario | Result |
|---|---|
| `gh` not on PATH | `unrecoverable` → halt with "Install GitHub CLI" |
| No git remote | `unrecoverable` → halt |
| Push fails | `unrecoverable` → halt |
| No workflow triggered | `{"status": "skipped", "reason": "no workflow triggered"}` |
| CI passes | `{"status": "pass", "run-url": "..."}` |
| CI fails | `{"status": "fail", "run-url": "...", "failures": [...]}` |
| CI timeout (>15min) | `{"status": "fail", "reason": "timeout", "run-url": "..."}` |

### `create-pr`

**Behavior:**
1. Determine base branch: `gh repo view --json defaultBranchRef`
2. `git push -u origin <branch>`
3. Build PR body from state:
   - **Summary** from `goal` and `plan` (if present)
   - **Test results** section: local pass/fail + CI pass/fail with run URL
   - **Lint status**: pass/fail with issue count
   - **Files changed**: from `git diff --stat <base>..HEAD`
4. Detect CODEOWNERS: parse `CODEOWNERS` file, match changed files, pass `--reviewer`
5. `gh pr create --title "<title>" --body "<body>"` with reviewers if found
6. Return PR URL and number

**Output:**

| Scenario | Result |
|---|---|
| `gh` not on PATH | `unrecoverable` → halt |
| PR already exists | Return existing: `{"url": "...", "number": N, "existing": true}` |
| No commits vs base | `{"status": "skipped", "reason": "no changes"}` |
| No CODEOWNERS | Create PR without reviewers (not an error) |
| API error | `transient` → retry |

**PR body template:**

```markdown
## Summary

{goal}

{plan summary if available}

## Changes

{git diff --stat output}

## Validation

- Lint: {pass/fail} ({N issues})
- Local tests: {pass/fail} ({duration})
- CI: {pass/fail/skipped} ({run URL if available})

---
Generated by [golem](https://github.com/lofari/golem)
```

---

## Tool Scoping

### Default Tools Per Agentic Step

| Step | Default Tools |
|---|---|
| `plan` | `semantic_search`, `find_callers`, `find_dependencies`, `find_co_changed` |
| `implement` | `semantic_search`, `find_callers`, `find_dependencies`, `find_dependents`, `find_co_changed`, `find_execution_failures`, `lsp_definition`, `lsp_references`, `lsp_hover`, `lsp_diagnostics` |
| `review` | `semantic_search`, `find_callers`, `find_dependencies` |
| `reflect` | `semantic_search` |
| `research` | `semantic_search`, `find_callers`, `find_dependencies`, `find_co_changed`, `find_execution_failures`, `get_runtime_trace` |

### Agent-Level Override

Agent definitions override tools per step in the YAML:

```yaml
- plan:
    type: agentic
    tools: [semantic_search]           # narrower than default
- implement:
    type: agentic
    tools: [semantic_search, find_callers, graph_query]  # add graph_query
```

### Resolution Order

```
1. Agent step override   tools: [...]   ← wins if present
2. Built-in default for step name       ← fallback
3. omitted               → all tools    ← backwards-compatible
```

### Mechanism

The engine resolves the tool list and passes it to `ClaudeRunner`. The runner generates a filtered MCP config exposing only those tools.

**Go-side change:** `WriteMCPConfig` accepts an optional `tools []string` filter. If nil, all tools registered. If non-nil, only matching tools.

```go
// internal/mcp/server.go
func (s *GolemServer) FilteredTools(allowed []string) []mcp.Tool
```

---

## Agent Definitions

### `one-shot` (new)

```yaml
name: one-shot
description: "One task, one PR. Implement, validate, ship."
initial-state: [goal]

config:
  lint-cmd: null
  test-cmd: null
  ci-enabled: false

steps:
  - implement:
      type: agentic
      reads: [goal]
      writes: [code, test-results]

  - lint:
      type: builtin
      reads: [code]
      writes: [lint-results]

  - run-tests:
      type: builtin
      reads: [code]
      writes: [test-results]

  - when:
      predicate: ci-enabled
      steps:
        - ci-tests:
            type: builtin
            reads: [code]
            writes: [ci-results]

  - when:
      predicate: ci-failed
      steps:
        - implement
        - ci-tests

  - create-pr:
      type: builtin
      reads: [code, goal]
      optional-reads: [test-results, ci-results, lint-results]
      writes: [pr-result]

errors:
  transient: { action: retry, max: 2 }
  contract-violation: { action: halt }
```

### `build-feature` (updated)

```yaml
name: build-feature
description: "Plan, implement with lint/test feedback loops, review, ship."
initial-state: [goal]

config:
  lint-cmd: null
  lint-fix-cmd: null
  test-cmd: null
  ci-enabled: false

steps:
  - plan:
      type: agentic
      reads: [goal]
      writes: [plan]
      tools: [semantic_search, find_callers, find_dependencies, find_co_changed]

  - implement:
      type: agentic
      reads: [goal, plan]
      optional-reads: [reflection, review-feedback, lint-results, test-results]
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
      reads: [code, test-results, lint-results]
      writes: [review-feedback]
      tools: [semantic_search, find_callers, find_dependencies]

  - while:
      predicate: needs-work
      max: 3
      steps:
        - implement
        - lint
        - run-tests
        - review

  - when:
      predicate: ci-enabled
      steps:
        - ci-tests:
            type: builtin
            reads: [code]
            writes: [ci-results]
        - when:
            predicate: ci-failed
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

errors:
  transient: { action: retry, max: 3 }
  malformed-output: { action: re-run, hint: "Check contract schema." }
  contract-violation: { action: halt }
```

### `fix-bug` (updated)

```yaml
name: fix-bug
description: "Research, fix, validate, ship."
initial-state: [goal]

config:
  lint-cmd: null
  test-cmd: null
  ci-enabled: false

steps:
  - research:
      type: agentic
      reads: [goal]
      writes: [research-context]
      tools: [semantic_search, find_callers, find_dependencies,
              find_co_changed, find_execution_failures, get_runtime_trace]

  - implement:
      type: agentic
      reads: [goal, research-context]
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

  - when:
      predicate: failed
      steps:
        - implement
        - lint
        - run-tests

  - when:
      predicate: ci-enabled
      steps:
        - ci-tests:
            type: builtin
            reads: [code]
            writes: [ci-results]

  - create-pr:
      type: builtin
      reads: [code, goal]
      optional-reads: [research-context, test-results, ci-results, lint-results]
      writes: [pr-result]

errors:
  transient: { action: retry, max: 2 }
```

---

## Built-in Predicates

Predicates are Go functions registered by name. The engine evaluates them against current state and config.

| Predicate | Logic |
|---|---|
| `needs-work` | `state["review-feedback"]["verdict"] == "needs-work"` |
| `failed` | `state["test-results"]["status"] == "fail"` |
| `lint-failed` | `state["lint-results"]["status"] == "fail"` |
| `ci-enabled` | `config["ci-enabled"] == true` |
| `ci-failed` | `state["ci-results"]["status"] == "fail"` |

No custom predicates in YAML. The built-in set covers common cases. The `shell` step type with exit-code-based branching handles the long tail.

---

## State Model

State is a JSON map, versioned per step.

### Directory Structure

```
.ctx/
  state.json              # current aggregate state (replaces state.yaml)
  config.yaml             # project config (stays YAML)
  agents/                 # project-local agent overrides (YAML)
  runs/
    run-001/
      state-v0.json       # initial: {"goal": "Add user auth"}
      state-v1.json       # after plan
      state-v2.json       # after implement
      state-v3.json       # after lint
      log.json            # execution log (append-only)
      sessions/           # raw Claude session outputs
  prompts/                # project-local prompt template overrides
  snapshots/              # state backups (existing, migrated to JSON)
```

### Example State After Full Run

```json
{
  "goal": "Add rate limiting to /api/users",
  "plan": [
    {"step": 1, "desc": "Add middleware struct"},
    {"step": 2, "desc": "Wire into router"}
  ],
  "code": {
    "files": ["middleware/ratelimit.go", "middleware/ratelimit_test.go"],
    "language": "go"
  },
  "test-results": {"status": "pass", "duration-ms": 1200},
  "lint-results": {"status": "pass", "issues": []},
  "review-feedback": {"verdict": "approved"},
  "pr-result": {"url": "https://github.com/org/repo/pull/42", "number": 42}
}
```

### Contract Validation

At load-time, the engine walks the step DAG and verifies:
- Every `reads` key is written by a prior step or declared in `initial-state`
- `optional-reads` keys are allowed but not required
- Steps inside `while`/`when`/`if` can read keys written by steps before the control flow node
- Re-referenced steps (by name in control flow) inherit their original contract

Fails fast with a clear error before any execution starts.

### Execution Log

```json
[
  {
    "node": "plan",
    "type": "agentic",
    "timestamp": "2026-03-12T14:22:45Z",
    "duration-ms": 12400,
    "status": "success",
    "state-version": 1,
    "contract": {"reads": ["goal"], "writes": ["plan"]}
  },
  {
    "node": "lint",
    "type": "builtin",
    "timestamp": "2026-03-12T14:24:00Z",
    "duration-ms": 2100,
    "status": "success",
    "state-version": 3
  }
]
```

---

## Config Surface

### `.ctx/config.yaml`

```yaml
agent: build-feature
agent-opts:
  lint-cmd: "golangci-lint run"
  lint-fix-cmd: "golangci-lint run --fix"
  test-cmd: "go test ./..."
  ci-enabled: true
```

### Flow

1. `golem code` reads `agent-opts` from config
2. Merges into agent's `config` map (config values override agent defaults)
3. Engine passes merged config to primitives
4. Primitives read their commands from config

### Skip Behavior

When a required config value is nil (e.g., `lint-cmd`), the primitive writes `{"status": "skipped"}` rather than failing. Deterministic primitives are opt-in per project.

---

## Go-Side Changes

### New Files

| File | Purpose |
|---|---|
| `internal/runner/engine.go` | Blueprint executor — walks DAG, dispatches to node handlers, manages state |
| `internal/runner/blueprint.go` | YAML parsing, DAG construction, contract validation |
| `internal/runner/primitives.go` | Built-in primitive implementations: lint, run-tests, ci-tests, create-pr |
| `internal/runner/predicates.go` | Built-in predicate registry and evaluation |
| `templates/agents/build-feature.yaml` | Embedded default agent |
| `templates/agents/fix-bug.yaml` | Embedded default agent |
| `templates/agents/one-shot.yaml` | Embedded default agent |

### Modified Files

| File | Change |
|---|---|
| `internal/mcp/server.go` | Add `FilteredTools(allowed []string)` method |
| `internal/runner/command.go` | Accept `tools []string` parameter, generate filtered MCP config |
| `cmd/code.go` | Add `engine: blueprint` path, load agent YAML, run engine |
| `cmd/helpers.go` | Populate agent-opts from config into engine |
| `internal/ctx/state.go` | Add JSON read/write alongside YAML (migration support) |
| `templates/embed.go` | Embed `agents/*.yaml` files |

### Removed Files (Phase 3)

| File | Reason |
|---|---|
| `internal/runner/builder.go` | Replaced by engine.go |
| `internal/runner/strategy.go` | Logic moves into engine error handling |
| `internal/runner/validate.go` | Logic moves into blueprint contract validation |
| `internal/runner/dsl_runner.go` | Clojure sidecar no longer needed |
| `internal/runner/dsl_events.go` | Clojure sidecar no longer needed |
| `golem-dsl/` (entire directory) | Clojure DSL replaced |

---

## Interaction Between Primitives

### Lint/Test Failures Drive Re-Implementation

When lint or tests fail mid-pipeline, the `review` step acts as the quality gate. It sees `lint-results` and `test-results` in its context and sets `verdict: needs-work` if either failed. This keeps the `while needs-work` loop going.

The next `implement` node receives all prior results in its prompt via `optional-reads`, so the agent knows exactly what to fix.

### State Flow

```
plan       → writes: plan
implement  → reads: plan, writes: code, test-results
lint       → reads: code, writes: lint-results
run-tests  → reads: code, writes: test-results (overwrites implement's)
review     → reads: code, test-results, lint-results, writes: review-feedback
...loop...
ci-tests   → reads: code, writes: ci-results
create-pr  → reads: code, goal; optional: plan, test-results, ci-results, lint-results
```

### Skipped Primitives

When `lint-cmd` or `test-cmd` is not configured, the primitive writes `{"status": "skipped"}`. Downstream steps handle this gracefully:
- `review` treats skipped lint/tests as neutral
- `create-pr` omits skipped sections from PR body
- Predicates (`failed`, `lint-failed`) return false for skipped status

---

## Migration Path

### Phase 1: Build Engine Alongside Existing Code

- Add `engine.go`, `blueprint.go`, `primitives.go`, `predicates.go`
- Add embedded default agents in `templates/agents/`
- Add MCP tool filtering
- Add JSON state management alongside existing YAML state
- New config option: `engine: blueprint` (opt-in)
- `golem code` with `engine: blueprint` uses new engine
- Existing `engine: go` and `engine: dsl` continue working

### Phase 2: Blueprint Becomes Default

- `golem code` uses blueprint engine by default
- `engine: legacy` keeps old Go loop as fallback
- `golem init` generates `.ctx/agents/` with default agents
- State files migrate from `.ctx/state.yaml` to `.ctx/state.json`
- Deprecation warning on legacy engine

### Phase 3: Remove Old Engines

- Delete `builder.go`, `strategy.go`, `validate.go`
- Delete `dsl_runner.go`, `dsl_events.go`
- Delete `golem-dsl/` directory
- Remove `engine` config option
- Single engine, single format

### Backwards Compatibility

- `.ctx/state.yaml` auto-migrated to `.ctx/state.json` on first blueprint engine run
- Old snapshot format readable for restore
- `golem status` reads whichever format exists
- Config stays YAML throughout
