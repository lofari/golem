# Minions Alignment: YAML Blueprint Engine with Deterministic Primitives

> Date: 2026-03-12
> Status: Draft

## Summary

Replace both the Go builder loop and the Clojure DSL with a single Go-native blueprint engine driven by YAML agent definitions. Add deterministic pipeline primitives (`git-setup`, `lint`, `run-tests`, `ci-tests`, `create-pr`), tool scoping per step, and a `one-shot` agent. These changes close the five largest gaps between golem and Stripe's Minions architecture while simplifying the stack from two languages/runtimes to one.

## Decisions

- **YAML blueprints replace Clojure DSL** — The DSL validated the blueprint pattern; YAML agent definitions with a Go engine deliver the same semantics without a second language, sidecar binary, or GraalVM dependency.
- **Explicit config over auto-detection** — Lint/test commands are configured per-project via `agent-opts`, not auto-detected from project stack. Predictable and debuggable for autonomous agents.
- **GitHub Actions via `gh` CLI** — CI integration uses `gh run` commands. Lowest friction, no custom API client needed.
- **Full PR workflow** — `create-pr` handles branch push, structured body generation, CODEOWNERS reviewer detection.
- **Primitive defaults with agent-level override** — Agentic steps declare default `tools`, agent definitions can narrow or widen per step. Omitted = all tools (backwards-compatible).
- **`one-shot` is just another agent** — No special CLI treatment. Invoked via `golem code --agent one-shot --goal "..."`.
- **Built-in + project-local agents** — Default agents embedded via Go `embed`. Project-local overrides in `.ctx/agents/`. Resolution: project-local wins.
- **Built-in primitives + generic `shell` type** — Complex primitives (`ci-tests`, `create-pr`) are Go functions. Simple user-defined steps use `shell` type.
- **JSON for pipeline state, YAML for config and agents** — Pipeline state is machine-managed (JSON avoids YAML coercion traps). Config and agent definitions are human-authored (YAML is friendlier to write).
- **Pipeline state is separate from project state** — The blueprint engine manages its own pipeline data flow in `.ctx/runs/`. The existing `.ctx/state.yaml` continues as the human-facing project dashboard. MCP tools (`mark_task`, `set_phase`) still write to `state.yaml`. The two state systems coexist.
- **Last-writer-wins for multiple writers** — When multiple steps write the same key (e.g., `implement` and `run-tests` both write `test-results`), the later step's value overwrites. This is intentional: deterministic results from `run-tests` are more reliable than agent-reported results.
- **Raw output over parsed output for lint/tests** — Lint and test primitives capture raw stdout/stderr rather than trying to parse language-specific output formats. Claude handles interpretation in the next agentic step.
- **Pipeline, not DAG** — The execution model is sequential with control flow (while/when/if). No parallel steps within a single agent run. The term "pipeline" is used throughout, not "DAG."

---

## Architecture

```
golem code --goal "Add auth"
  │
  ├── Load agent: .ctx/agents/build-feature.yaml (or embedded default)
  ├── Parse blueprint → Go pipeline structure
  ├── Validate contracts (reads/writes chain)
  │
  └── Engine walks pipeline:
       ├── Agentic step       → ClaudeRunner.Run() with scoped tools + rendered prompt
       ├── Builtin step       → Go function (git-setup, lint, run-tests, ci-tests, create-pr)
       ├── Shell step         → exec.Command() with output capture
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
- `internal/runner/blueprint.go` — YAML parsing, pipeline construction, contract validation
- `internal/runner/primitives.go` — built-in deterministic primitive implementations
- `internal/runner/predicates.go` — built-in predicate registry and evaluation
- `templates/agents/` — embedded default agent YAML files
- MCP tool filtering via environment variable

### What Stays Unchanged

- `ClaudeRunner` — still spawns `claude -p`
- MCP server — still provides graph/state tools (reads/writes `.ctx/state.yaml`)
- Knowledge graph — still syncs and embeds
- Warden sandbox — still wraps sessions
- Event system — engine emits same `Event` types
- `golem status`, `golem config`, `golem init`, `golem plan`, `golem review`
- Config system (two-layer YAML, `.ctx/config.yaml`)
- `.ctx/state.yaml` — project dashboard, unchanged

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
  - git-setup:
      type: builtin

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
        - while:
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

errors:
  transient: { action: retry, max: 3 }
  malformed-output: { action: re-run, hint: "Write session-output.json with required keys." }
  contract-violation: { action: halt }
```

### Step Types

| Type | Execution | Use Case |
|---|---|---|
| `agentic` | Spawns Claude session with scoped tools and rendered prompt | plan, implement, review, reflect, research |
| `builtin` | Calls a registered Go function | git-setup, lint, run-tests, ci-tests, create-pr |
| `shell` | Runs user-defined command, captures output | Custom deterministic steps |

### Shell Step Example

```yaml
- db-migrate:
    type: shell
    command: "make db-migrate"
    timeout: 60s
    reads: [code]
    writes: [migration-results]
    # Exit code 0 → {"status": "pass", "output": "..."}
    # Exit code non-zero → {"status": "fail", "output": "...", "exit-code": N}
```

Shell steps capture stdout+stderr as raw text in the `output` field. Exit code determines `status`. The output is available to downstream agentic steps for interpretation.

### Control Flow Nodes

| Node | Behavior |
|---|---|
| `while` | Loop steps while predicate is true, up to `max` iterations |
| `when` | Execute steps only if predicate is true |
| `if` | Execute `then` steps if true, `else` steps if false |

Control flow nodes reference steps by name. When a step is referenced by name inside a control flow block, it uses the same config (type, tools, reads/writes) as its original definition earlier in the file, but operates on the current pipeline state. This means `implement` inside a `while` loop sees updated `lint-results` and `test-results` from the previous iteration via its `optional-reads`.

### Prompt Templates

Agentic steps use prompt templates. Resolution order:
1. `.ctx/prompts/<step-name>.md` — project-local override
2. `templates/prompts/<step-name>.md` — embedded default

Templates receive the step's `reads` and `optional-reads` keys rendered from current pipeline state. Custom template per step:

```yaml
- plan:
    type: agentic
    prompt: custom-plan.md    # resolved via same order above
    reads: [goal]
    writes: [plan]
```

---

## Agentic Step Contract Enforcement

Agentic steps (Claude sessions) must produce structured output so the engine can validate contracts and merge results into pipeline state.

### Mechanism: `session-output.json`

The prompt template instructs Claude to write a `session-output.json` file in the working directory containing the step's declared `writes` keys:

```json
{
  "plan": [
    {"step": 1, "desc": "Add middleware struct"},
    {"step": 2, "desc": "Wire into router"}
  ]
}
```

After the session exits, the engine:
1. Reads `session-output.json` from the working directory
2. Validates that all declared `writes` keys are present
3. If missing keys → `malformed-output` error → re-run with hint
4. If valid → merges into pipeline state
5. Deletes `session-output.json` (ephemeral, per-step)

### Special Case: `code` Key

The `code` key is populated automatically by the engine, not by Claude:
1. After an agentic step exits, the engine runs `git diff --name-only` against the pre-step snapshot
2. Changed files are captured as `{"files": ["path/to/file.go", ...], "language": "go"}`
3. This is merged into state alongside the `session-output.json` contents

This means Claude doesn't need to track which files it changed — the engine detects it deterministically.

### Relationship to Project State

Agentic steps can still use MCP tools (`mark_task`, `set_phase`, `add_decision`) to update `.ctx/state.yaml` during their session. This is orthogonal to the pipeline state — the project dashboard is updated for human visibility, while `session-output.json` drives the pipeline data flow.

---

## Tool Scoping

### Mechanism

Each `claude -p` session spawns its own MCP server process via `mcp_servers.json`. The engine controls which tools are available by setting the `GOLEM_TOOLS` environment variable before spawning the session. The MCP server reads this at startup and only registers the listed tools.

```go
// internal/mcp/server.go — at startup
func (s *GolemServer) registerTools() {
    allowed := os.Getenv("GOLEM_TOOLS") // comma-separated, empty = all
    // Only register tools in the allowed list
    // If empty/unset, register all tools (backwards-compatible)
}
```

This works because each session gets its own MCP server process — no shared state, no need to dynamically add/remove tools.

### Default Tools Per Agentic Step

| Step | Default Tools |
|---|---|
| `plan` | `semantic_search`, `find_callers`, `find_dependencies`, `find_co_changed` |
| `implement` | `semantic_search`, `find_callers`, `find_dependencies`, `find_dependents`, `find_co_changed`, `find_execution_failures`, `lsp_definition`, `lsp_references`, `lsp_hover`, `lsp_diagnostics` |
| `review` | `semantic_search`, `find_callers`, `find_dependencies` |
| `reflect` | `semantic_search` |
| `research` | `semantic_search`, `find_callers`, `find_dependencies`, `find_co_changed`, `find_execution_failures`, `get_runtime_trace` |

State management tools (`mark_task`, `set_phase`, `set_status`, `add_decision`, `add_pitfall`, `log_session`) are always available to all agentic steps — they write to the project dashboard, not the pipeline state.

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

### CommandRunner Interface Change

```go
// internal/runner/command.go

type CommandRunner interface {
    Run(ctx context.Context, dir, prompt string, maxTurns int, model string) (string, error)
    // New method for tool-scoped sessions:
    RunWithTools(ctx context.Context, dir, prompt string, maxTurns int, model string, tools []string) (string, error)
}
```

`RunWithTools` sets `GOLEM_TOOLS` in the subprocess environment before spawning `claude -p`. `Run` continues to work unchanged (all tools). `ClaudeRunner` implements both; test mocks implement both.

---

## Deterministic Primitives

All builtin primitives execute locally without spawning a Claude session.

### `git-setup`

**Behavior:**
1. Create branch `golem/<agent-name>-<YYYYMMDD-HHMMSS>` from current HEAD
2. `git checkout` the new branch
3. Write branch name to pipeline state

**Output:** `{"branch": "golem/build-feature-20260312-142200", "base": "main"}`

This runs as the first step in every agent, ensuring all work happens on an isolated branch. `ci-tests` and `create-pr` use the branch name from state.

### `lint`

**Inputs from config:**
- `lint-cmd` — lint command (e.g., `"golangci-lint run"`)
- `lint-fix-cmd` — autofix command (e.g., `"golangci-lint run --fix"`)

**Behavior:**
1. If `lint-fix-cmd` is set, run it first (applies mechanical fixes)
2. Run `lint-cmd` to verify
3. Capture raw stdout+stderr (no language-specific parsing)
4. Timeout: 30s default

**Output:**

| Scenario | Result |
|---|---|
| Not configured | `{"status": "skipped", "reason": "no lint-cmd"}` |
| Passes | `{"status": "pass", "output": ""}` |
| Fails after autofix | `{"status": "fail", "output": "...", "autofix-applied": true}` |
| Fails, no fix cmd | `{"status": "fail", "output": "..."}` |
| Timeout | `transient` error → retry handler |
| Command not found | `unrecoverable` → halt |

### `run-tests`

**Inputs from config:**
- `test-cmd` — test command (e.g., `"go test ./..."`)
- `test-timeout` — timeout duration (default: 5 minutes)

**Behavior:**
1. Run `test-cmd` in project directory
2. Capture raw stdout+stderr
3. Capture exit code and duration

**Output:**

| Scenario | Result |
|---|---|
| Not configured | `{"status": "skipped"}` |
| All pass | `{"status": "pass", "output": "...", "duration-ms": N}` |
| Failures | `{"status": "fail", "output": "...", "duration-ms": N}` |
| Timeout | `transient` error → retry |
| Command not found | `unrecoverable` → halt |

### `ci-tests`

**Behavior:**
1. `git push origin <branch>` (branch from `git-setup`)
2. `gh run list --branch <branch> --limit 1 --json databaseId,status` — find triggered workflow
3. `gh run watch <run-id>` — poll until completion (timeout: 15min default)
4. On completion: `gh run view <run-id> --json conclusion,jobs` for results
5. On failure: `gh api repos/{owner}/{repo}/actions/jobs/{job-id}/logs` to fetch failed job logs

**Output:**

| Scenario | Result |
|---|---|
| `gh` not on PATH | `unrecoverable` → halt with "Install GitHub CLI" |
| No git remote | `unrecoverable` → halt |
| Push fails | `unrecoverable` → halt |
| No workflow triggered | `{"status": "skipped", "reason": "no workflow triggered"}` |
| CI passes | `{"status": "pass", "run-url": "..."}` |
| CI fails | `{"status": "fail", "run-url": "...", "output": "..."}` |
| CI timeout (>15min) | `{"status": "fail", "reason": "timeout", "run-url": "..."}` |

### `create-pr`

**Behavior:**
1. Determine base branch: `gh repo view --json defaultBranchRef`
2. `git push -u origin <branch>`
3. Build PR body from pipeline state:
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
  - git-setup:
      type: builtin

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
        - while:
            predicate: ci-failed
            max: 2
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
  - git-setup:
      type: builtin

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
        - while:
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

errors:
  transient: { action: retry, max: 3 }
  malformed-output: { action: re-run, hint: "Write session-output.json with required keys." }
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
  - git-setup:
      type: builtin

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
      predicate: lint-failed
      steps:
        - implement
        - lint

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

Predicates are Go functions registered by name. The engine evaluates them against current pipeline state and config.

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

### Two State Systems

| State | Format | Purpose | Managed By |
|---|---|---|---|
| **Pipeline state** | JSON | Data flow between steps (`goal`, `plan`, `code`, `test-results`, etc.) | Blueprint engine |
| **Project state** | YAML | Human-facing dashboard (`tasks`, `decisions`, `pitfalls`, `phase`) | MCP tools during agentic steps |

These coexist. The engine manages pipeline state in `.ctx/runs/`. MCP tools continue to read/write `.ctx/state.yaml` for the project dashboard. `golem status` reads the project state unchanged.

### Pipeline State Directory Structure

```
.ctx/
  state.yaml              # project dashboard (unchanged, managed by MCP tools)
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
  snapshots/              # state backups (existing)
```

### Example Pipeline State After Full Run

```json
{
  "goal": "Add rate limiting to /api/users",
  "branch": "golem/one-shot-20260312-142200",
  "base": "main",
  "plan": [
    {"step": 1, "desc": "Add middleware struct"},
    {"step": 2, "desc": "Wire into router"}
  ],
  "code": {
    "files": ["middleware/ratelimit.go", "middleware/ratelimit_test.go"],
    "language": "go"
  },
  "test-results": {"status": "pass", "output": "ok  ...", "duration-ms": 1200},
  "lint-results": {"status": "pass", "output": ""},
  "review-feedback": {"verdict": "approved"},
  "pr-result": {"url": "https://github.com/org/repo/pull/42", "number": 42}
}
```

### Contract Validation

At load-time, the engine walks the step pipeline and verifies:
- Every `reads` key is written by a prior step or declared in `initial-state`
- `optional-reads` keys are allowed but not required
- Steps inside `while`/`when`/`if` can read keys written by steps before the control flow node
- Re-referenced steps (by name in control flow) inherit their original contract
- Multiple steps can write the same key (last-writer-wins, explicitly allowed)
- `git-setup` implicitly writes `branch` and `base` (no contract declaration needed)

Fails fast with a clear error before any execution starts.

### Cancellation

When the engine receives SIGINT:
1. If an agentic step is running: forward SIGINT to the Claude process, wait for graceful exit
2. Save current pipeline state as the latest version (partial state is valid)
3. Write `{"status": "cancelled"}` to the execution log
4. Exit with non-zero code

The run can be inspected via `golem inspect run-NNN` to see partial state. Resumption is not supported in v1 — the user re-runs from scratch.

### Execution Log

```json
[
  {
    "node": "git-setup",
    "type": "builtin",
    "timestamp": "2026-03-12T14:22:40Z",
    "duration-ms": 200,
    "status": "success",
    "state-version": 0
  },
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
| `internal/runner/engine.go` | Blueprint executor — walks pipeline, dispatches to step handlers, manages state, handles cancellation |
| `internal/runner/blueprint.go` | YAML parsing, pipeline construction, contract validation |
| `internal/runner/primitives.go` | Built-in primitive implementations: git-setup, lint, run-tests, ci-tests, create-pr |
| `internal/runner/predicates.go` | Built-in predicate registry and evaluation |
| `templates/agents/build-feature.yaml` | Embedded default agent |
| `templates/agents/fix-bug.yaml` | Embedded default agent |
| `templates/agents/one-shot.yaml` | Embedded default agent |
| `templates/prompts/plan.md` | Prompt template for plan step |
| `templates/prompts/implement.md` | Prompt template for implement step |
| `templates/prompts/review.md` | Prompt template for review step |
| `templates/prompts/research.md` | Prompt template for research step |
| `templates/prompts/reflect.md` | Prompt template for reflect step |

### Modified Files

| File | Change |
|---|---|
| `internal/mcp/server.go` | Read `GOLEM_TOOLS` env var at startup, filter tool registration |
| `internal/runner/command.go` | Add `RunWithTools` method to `CommandRunner` interface; `ClaudeRunner` sets `GOLEM_TOOLS` env var on subprocess |
| `cmd/code.go` | Add `engine: blueprint` path, load agent YAML, run engine |
| `cmd/helpers.go` | Populate agent-opts from config into engine |
| `templates/embed.go` | Embed `agents/*.yaml` and `prompts/*.md` files |

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

When lint or tests fail mid-pipeline, the `review` step acts as the quality gate. It sees `lint-results` and `test-results` in its prompt context (raw output included) and sets `verdict: needs-work` if either failed. This keeps the `while needs-work` loop going.

The next `implement` node receives all prior results in its prompt via `optional-reads`, including the raw lint/test output, so the agent knows exactly what to fix.

### State Flow

```
git-setup  → writes: branch, base
plan       → writes: plan
implement  → writes: code, test-results (code auto-detected via git diff)
lint       → reads: code, writes: lint-results (raw output)
run-tests  → reads: code, writes: test-results (overwrites implement's — last-writer-wins)
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
- Add prompt templates in `templates/prompts/`
- Add MCP tool filtering via `GOLEM_TOOLS` env var
- Add `RunWithTools` to `CommandRunner` interface
- New config option: `engine: blueprint` (opt-in)
- `golem code` with `engine: blueprint` uses new engine
- Existing `engine: go` and `engine: dsl` continue working
- Pipeline state in `.ctx/runs/` coexists with `.ctx/state.yaml`

### Phase 2: Blueprint Becomes Default

- `golem code` uses blueprint engine by default
- `engine: legacy` keeps old Go loop as fallback
- `golem init` generates `.ctx/agents/` with default agents
- Deprecation warning on legacy engine

### Phase 3: Remove Old Engines

- Delete `builder.go`, `strategy.go`, `validate.go`
- Delete `dsl_runner.go`, `dsl_events.go`
- Delete `golem-dsl/` directory
- Remove `engine` config option
- Single engine, single format

### Backwards Compatibility

- `.ctx/state.yaml` is never removed — it continues as the project dashboard
- Old snapshot format readable for restore
- `golem status` reads project state unchanged
- Config stays YAML throughout
