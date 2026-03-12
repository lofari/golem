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
- **Simple `${key}` prompt interpolation** — Prompt templates use `${key}` syntax with no conditionals or loops. The engine handles optional section inclusion. Keeps templates readable for prompt engineering.
- **Reserved engine-managed keys** — `code`, `branch`, `base` are populated by the engine, not by Claude sessions. `session-output.json` validation skips reserved keys.
- **Single-level error propagation** — Errors bubble directly to the pipeline-level error handler. No per-control-flow-node error config. One handler, one place to reason about.
- **Missing predicate keys return false** — When a predicate checks a key that doesn't exist in state, it returns false. Optimistic model — assume things are fine until proven otherwise.
- **Strict contract validation for conditional writes** — If a step writes a key inside a `when`/`if` block, downstream steps must use `optional-reads` for that key. Enforced at load time.
- **Strict YAML parsing** — Unknown fields in agent YAML produce a parse error. Catches typos like `tool` instead of `tools`.
- **Timestamp-based run IDs** — Format: `run-YYYYMMDD-HHMMSS`. No sequential counters, no race conditions.
- **NDJSON event log** — Engine events are written to `log.json` as newline-delimited JSON for CLI, Flutter GUI, and post-mortem consumption.

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
       ├── Agentic step       → ClaudeRunner.RunWithTools() with scoped tools + rendered prompt
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
- `templates/prompts/` — embedded default prompt templates
- MCP tool filtering via environment variable

### What Stays Unchanged

- `ClaudeRunner` — still spawns `claude -p`
- MCP server — still provides graph/state tools (reads/writes `.ctx/state.yaml`)
- Knowledge graph — still syncs and embeds
- Warden sandbox — still wraps sessions (minor change: `GOLEM_TOOLS` env var must be passed through)
- `golem status`, `golem config`, `golem init`, `golem plan`, `golem review`
- Config system (two-layer YAML, `.ctx/config.yaml`)
- `.ctx/state.yaml` — project dashboard, unchanged

---

## Agent YAML Format

This is the full `build-feature` agent (also appears in [Agent Definitions](#agent-definitions)):

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
  malformed-output: { action: re-run, max: 2, hint: "Write session-output.json with required keys." }
  contract-violation: { action: halt }
```

### Step Types

| Type | Execution | Use Case |
|---|---|---|
| `agentic` | Spawns Claude session with scoped tools and rendered prompt | plan, implement, review, reflect, research |
| `builtin` | Calls a registered Go function | git-setup, lint, run-tests, ci-tests, create-pr |
| `shell` | Runs user-defined command, captures output | Custom deterministic steps |

### Agentic Step Fields

| Field | Required | Description |
|---|---|---|
| `type` | Yes | Must be `agentic` |
| `reads` | Yes | Keys required from pipeline state |
| `writes` | Yes | Keys this step must produce |
| `optional-reads` | No | Keys included in prompt if present in state |
| `tools` | No | MCP tools available to this step (overrides defaults) |
| `prompt` | No | Custom prompt template filename (overrides step-name-based lookup) |
| `max-turns` | No | Maximum Claude conversation turns (overrides default for step name) |
| `timeout` | No | Maximum wall-clock duration (overrides default for step name) |
| `model` | No | Claude model override (e.g., `claude-sonnet-4-6`) |

### Agentic Step Defaults

Per-step defaults for `max-turns` and `timeout`, applied when the agent YAML does not override:

| Step name | Default max-turns | Default timeout |
|---|---|---|
| `plan` | 50 | 20m |
| `implement` | 200 | 30m |
| `review` | 50 | 20m |
| `reflect` | 30 | 10m |
| `research` | 75 | 20m |
| Custom/unnamed | 75 | 20m |

Resolution order: step-level override in YAML → built-in default for step name → engine hardcoded defaults.

**Duration format:** All timeout values use Go's `time.ParseDuration` syntax (e.g., `30s`, `5m`, `1h30m`). This applies to agentic step `timeout`, shell step `timeout`, and builtin primitive timeouts.

**Model per step**: If omitted, uses the model from config (CLI flag → project config → global config → default). Useful for cost optimization — Haiku for simple review, Opus for complex implementation.

Example with overrides:

```yaml
- implement:
    type: agentic
    max-turns: 300
    timeout: 45m
    model: claude-sonnet-4-6
    reads: [goal, plan]
    writes: [code, test-results]
```

### Shell Step

```yaml
- db-migrate:
    type: shell
    command: "make db-migrate"
    timeout: 60s
    reads: [code]
    writes: [migration-results]
    errors:
      non-zero: halt    # default: transient
```

Shell steps capture stdout+stderr as raw text in the `output` field. Exit code determines `status`:
- Exit code 0 → `{"status": "pass", "output": "..."}`
- Exit code non-zero → `{"status": "fail", "output": "...", "exit-code": N}`

Shell `command` fields are static strings from YAML — no variable interpolation from pipeline state. The engine passes the command to `exec.Command("sh", "-c", command)` as-is.

### Control Flow Nodes

| Node | Behavior |
|---|---|
| `while` | Loop steps while predicate is true, up to `max` iterations |
| `when` | Execute steps only if predicate is true |
| `if` | Execute `then` steps if true, `else` steps if false |

Control flow nodes reference steps by name. When a step is referenced by name inside a control flow block, it uses the same config (type, tools, reads/writes) as its original definition earlier in the file, but operates on the current pipeline state. This means `implement` inside a `while` loop sees updated `lint-results` and `test-results` from the previous iteration via its `optional-reads`.

---

## Prompt Templates

### Syntax

Simple `${key}` interpolation. No conditionals, no loops, no pipes. The engine replaces `${key}` with the JSON-serialized value of that key from pipeline state.

### Available Variables

| Variable | Source | Description |
|---|---|---|
| `${<reads-key>}` | Pipeline state | Required — guaranteed present |
| `${<optional-reads-key>}` | Pipeline state | Included if present, section omitted if absent |
| `${config.<key>}` | Agent config | Config values (e.g., `${config.lint-cmd}`) |
| `${agent.name}` | Agent definition | The agent name |
| `${run.id}` | Engine | Current run ID |

### Optional Reads Handling

The engine wraps each optional-read in a conditional section. If the key is absent from state, the entire section (header + value) is omitted from the rendered prompt. This is engine logic, not template logic.

### Template Resolution Order

1. `.ctx/prompts/<step-name>.md` — project-local override
2. `templates/prompts/<step-name>.md` — embedded default

Custom template per step:

```yaml
- plan:
    type: agentic
    prompt: custom-plan.md    # resolved via same order above
    reads: [goal]
    writes: [plan]
```

### Rendering Pipeline

1. Engine loads template file (project-local → embedded)
2. Injects all `reads` keys via `${key}` replacement
3. For each `optional-reads` key present in state, injects its section
4. For absent `optional-reads`, omits the section entirely
5. Replaces `${config.*}`, `${agent.name}`, `${run.id}`
6. Any remaining `${...}` tokens → engine error (catches typos in templates)

### Example Template (`templates/prompts/implement.md`)

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

**Reserved keys in templates:** Prompt templates should instruct Claude to write only non-reserved keys in `session-output.json`. The `code` key (and `branch`, `base`) are populated by the engine — Claude should never write them. The step's `writes: [code, test-results]` declaration tells the engine to run git diff detection; the template only asks Claude to write `test-results`.

With optional reads rendered by the engine (only if present in state):

```markdown
# Previous Test Results
${test-results}

# Previous Lint Results
${lint-results}

# Review Feedback
${review-feedback}
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
2. Validates that all declared `writes` keys are present (excluding reserved keys)
3. Extracts **only** keys declared in the step's `writes` list — extra keys are silently dropped
4. If missing keys → `malformed-output` error → re-run with hint
5. If valid → merges into pipeline state
6. Deletes `session-output.json` (ephemeral, per-step)

### Reserved Engine-Managed Keys

The following keys are populated by the engine, never by Claude sessions:

| Key | Source | When |
|---|---|---|
| `code` | `git diff --name-only` against pre-step snapshot | After any agentic step that declares `writes: [code]` |
| `branch` | Branch name created by `git-setup` | After `git-setup` runs |
| `base` | Base branch at time of `git-setup` | After `git-setup` runs |

When a step declares `writes: [code]`, this signals the engine to run git diff detection after the step exits. The `session-output.json` validation skips reserved keys — it only validates non-reserved declared writes.

**`code` detection:**
1. After the agentic step exits, engine runs `git diff --name-only` against the pre-step commit
2. Changed files are captured as `{"files": ["path/to/file.go", ...], "language": "go"}`
3. Language is inferred from file extensions (majority wins)
4. This is merged into state alongside the `session-output.json` contents

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
| Custom/unnamed | All tools (backwards-compatible) |

State management tools (`mark_task`, `set_phase`, `set_status`, `add_decision`, `add_pitfall`, `log_session`) and the `find_test_results` graph query tool are always available to all agentic steps — they write to the project dashboard or read from the knowledge graph, not the pipeline state.

### Recognized Step Names

| Step | Purpose | Notes |
|---|---|---|
| `plan` | Analyze goal, produce implementation plan | Used in `build-feature` |
| `implement` | Write code changes and tests | Used in all agents |
| `review` | Evaluate code quality, set verdict | Used in `build-feature` |
| `reflect` | Holistic self-evaluation after iterative loops | Available but not in default agents |
| `research` | Investigate codebase, trace bugs, examine runtime data | Used in `fix-bug` |

`reflect` writes `reflection` which feeds into `implement` via `optional-reads`. It catches architectural issues, naming inconsistencies, and forgotten edge cases that granular review might miss.

`get_runtime_trace` is an existing MCP graph query tool that retrieves runtime execution traces (stack traces, error logs, performance data) collected from previous runs and stored in the knowledge graph.

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

**Warden sandbox integration:** When running in sandbox mode, `ClaudeRunner.buildCommand` constructs a `warden run --env ...` invocation. `GOLEM_TOOLS` must be explicitly passed through as `--env GOLEM_TOOLS=<value>` in the warden arguments — setting it on `cmd.Env` alone is not sufficient, as warden does not inherit the parent process environment.

---

## Deterministic Primitives

All builtin primitives execute locally without spawning a Claude session.

### `git-setup`

**Behavior:**
1. Create branch `golem/<agent-name>-<YYYYMMDD-HHMMSS>` from current HEAD
2. `git checkout` the new branch
3. Write branch name to pipeline state

**Output:** `{"branch": "golem/build-feature-20260312-142200", "base": "main"}`

**Branch name collision:** If the branch already exists (e.g., from a cancelled run in the same second), append `-1`, `-2`, etc. — same collision rule as run IDs.

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
2. Poll for workflow trigger: `gh run list --branch <branch> --limit 1 --json databaseId,status` — retry every 5s for up to 30s. GitHub Actions workflows are not instantaneous after a push; this window avoids false "no workflow triggered" results.
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
2. `git push -u origin <branch> --force-with-lease` (safe push — handles the case where `ci-tests` already pushed the branch earlier in the pipeline; `--force-with-lease` ensures we don't overwrite unexpected remote changes)
3. Generate PR title from `goal` — truncated to 70 chars at last word boundary, appended with `...` if truncated
4. Build PR body from pipeline state (see template below)
5. Detect CODEOWNERS: parse `CODEOWNERS` file, match changed files, pass `--reviewer`
6. If `draft: true` in config, add `--draft` flag
7. If `pr-labels` in config, add `--label` flags
8. `gh pr create --title "<title>" --body "<body>"` with reviewers/labels if configured
9. Return PR URL and number

**Config options:**

| Key | Default | Description |
|---|---|---|
| `draft` | `false` | Create PR as draft |
| `pr-labels` | `[]` | Labels to apply (e.g., `["golem", "automated"]`) |

**Output:**

| Scenario | Result |
|---|---|
| `gh` not on PATH | `unrecoverable` → halt |
| PR already exists | Return existing: `{"url": "...", "number": N, "existing": true}` |
| No commits vs base | `{"status": "skipped", "reason": "no changes"}` |
| No CODEOWNERS | Create PR without reviewers (not an error) |
| API error | `transient` → retry |

**CODEOWNERS parsing:** Best-effort, line-by-line. Supports exact paths, directory globs (`/docs/`), wildcard globs (`*.go`, `src/api/**`), and team handles (`@org/team-name`). Complex negation and inline comments are silently skipped. If the file doesn't exist or parsing finds no matches, the PR is created without reviewers.

**PR body template:**

```markdown
## Summary

{goal}

{plan summary if available}

## Changes

{git diff --stat output}

## Validation

- Lint: {pass/fail/skipped} {issue details if failed}
- Local tests: {pass/fail/skipped} {duration if ran}
- CI: {pass/fail/skipped} {run URL if available}
```

Sections with "skipped" status show the label only, no details.

---

## Agent Definitions

### `one-shot` (new)

```yaml
name: one-shot
description: "One task, one PR. Implement, validate, ship."
initial-state: [goal]

config:
  lint-cmd: null
  lint-fix-cmd: null
  test-cmd: null
  ci-enabled: false

steps:
  - git-setup:
      type: builtin

  - implement:
      type: agentic
      reads: [goal]
      optional-reads: [lint-results, test-results, ci-results]
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
              - lint
              - run-tests
              - ci-tests

  - create-pr:
      type: builtin
      reads: [code, goal]
      optional-reads: [test-results, ci-results, lint-results]
      writes: [pr-result]

errors:
  transient: { action: retry, max: 2 }
  malformed-output: { action: re-run, max: 2, hint: "Write session-output.json with required keys." }
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
  malformed-output: { action: re-run, max: 2, hint: "Write session-output.json with required keys." }
  contract-violation: { action: halt }
```

### `fix-bug` (updated)

```yaml
name: fix-bug
description: "Research, fix, validate, ship."
initial-state: [goal]

config:
  lint-cmd: null
  lint-fix-cmd: null    # supported — add to enable autofix during lint step
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
  malformed-output: { action: re-run, max: 2, hint: "Write session-output.json with required keys." }
  contract-violation: { action: halt }
```

---

## Built-in Predicates

Predicates are Go functions registered by name. The engine evaluates them against current pipeline state and config.

| Predicate | Logic | Missing key → |
|---|---|---|
| `needs-work` | `state["review-feedback"]["verdict"] == "needs-work"` | `false` |
| `failed` | `state["test-results"]["status"] == "fail"` | `false` |
| `lint-failed` | `state["lint-results"]["status"] == "fail"` | `false` |
| `ci-enabled` | `config["ci-enabled"] == true` | `false` |
| `ci-failed` | `state["ci-results"]["status"] == "fail"` | `false` |

**Missing key behavior:** When a predicate checks a key that doesn't exist in state, it returns `false`. This is the optimistic model — assume things are fine until proven otherwise. This matches the natural pipeline flow: the first time through, `review-feedback` doesn't exist, so `needs-work` is false and the `while` loop is skipped (correct — the first pass runs the linear pipeline above).

No compound predicates. No custom predicates in YAML. The built-in set covers common cases. The `shell` step type with exit-code-based branching handles the long tail.

---

## Error Handling

### Error Classification

| Condition | Classification |
|---|---|
| Agentic step: Claude exits normally | Check `session-output.json` → success or `malformed-output` |
| Agentic step: process crash/killed | `transient` |
| Agentic step: timeout exceeded | `transient` |
| Builtin step: command not found | `unrecoverable` → always halt |
| Builtin step: timeout | `transient` |
| Builtin step: non-zero exit | Step-specific (lint/test failures are normal results, not errors) |
| Shell step: non-zero exit | `transient` (default), overridable per step via `errors.non-zero` |
| Shell step: command not found | `unrecoverable` → always halt |
| Contract violation | Always halt (non-negotiable) |
| `session-output.json` missing/malformed | `malformed-output` |

### Error Propagation

Single level. Errors bubble directly to the pipeline-level `errors` block. No per-control-flow-node error config. If a step inside a `while` loop fails and exhausts retries, the pipeline halts. The loop breaks on any unhandled error.

**Default for unhandled error types:** If an error type is not declared in the agent's `errors` block, the engine halts. This means agents that omit `malformed-output` will halt on bad session output rather than silently ignoring it. The `errors` block is opt-in per type — omission is equivalent to `{ action: halt }`.

### `malformed-output` Re-run

The engine re-runs the agentic step with the original prompt plus an appended hint:

```markdown
## IMPORTANT: Previous attempt failed

Your previous session did not produce a valid session-output.json.
You MUST write a session-output.json file containing these keys: ${writes}

Hint: ${error-hint}
```

The re-run gets a fresh Claude session (no state carried from the failed attempt). The `max` field limits re-runs (same semantics as `retry`'s `max`). The `hint` field is appended to the re-run prompt.

### Error Logging

Every error is recorded in the execution log with:
- Step name
- Error type (`transient`, `malformed-output`, `unrecoverable`, `contract-violation`)
- Action taken (`retry`, `re-run`, `halt`)
- Attempt number
- Error message/details

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
    .lock                 # flock for single-run enforcement
    current -> run-20260312-142200/   # symlink to active run, removed on completion
    run-20260312-142200/
      state-v0.json       # initial: {"goal": "Add user auth"}
      state-v1.json       # after plan
      state-v2.json       # after implement
      state-v3.json       # after lint
      log.json            # execution event log (NDJSON, append-only)
      sessions/           # raw Claude session outputs
  prompts/                # project-local prompt template overrides
  snapshots/              # state backups (existing)
```

### Run Management

**Run IDs:** Timestamp-based. Format: `run-YYYYMMDD-HHMMSS`. If a collision occurs (same second), append `-1`, `-2`.

**Concurrency:** Single run per project. Enforced via `flock` on `.ctx/runs/.lock`. Second invocation fails immediately:

```
error: another golem run is in progress (PID 12345, started 2m ago)
```

No queuing, no waiting.

**New commands:**

| Command | Behavior |
|---|---|
| `golem runs list` | Shows all runs: ID, agent, status, duration, outcome. Most recent first. |
| `golem runs inspect <run-id>` | Shows execution log, final state, step-by-step timeline with durations and statuses |
| `golem runs clean --keep N` | Deletes all but the N most recent runs. Default keep: 10. |
| `golem runs watch <run-id>` | Streams events as NDJSON to stdout. For active runs: blocks and streams live events as they are appended to `log.json`. For completed runs: replays all events from `log.json` then exits. The Flutter GUI spawns this as a subprocess for live updates. |

No automatic cleanup. Runs are small (JSON files). Users clean up when they want.

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

1. Every `reads` key is written by a prior step or declared in `initial-state`
2. `optional-reads` keys are allowed but not required
3. Writes inside `when`/`if` blocks are conditional — downstream steps must use `optional-reads` for those keys (strict enforcement)
4. Steps inside `while`/`when`/`if` can read keys written by steps before the control flow node
5. Re-referenced steps (by name in control flow) inherit their original contract
6. Step names must be unique within an agent
7. Reserved keys (`code`, `branch`, `base`) are skipped during `session-output.json` validation
8. Multiple steps can write the same key (last-writer-wins, explicitly allowed)
9. `git-setup` implicitly writes `branch` and `base` (no contract declaration needed)
10. Builtin steps access pipeline state internally (e.g., `ci-tests` reads `branch` from state at runtime) — they are exempt from `reads` validation for keys they consume internally. Their declared `reads` are for documentation and state-flow tracking, not strict enforcement.
11. Unknown fields in agent YAML → parse error with field name and location

Fails fast with a clear error before any execution starts. Example:

```
contract error: step "create-pr" reads "ci-results" but it is only written
inside a conditional block (when: ci-enabled); use optional-reads instead
```

### Cancellation

When the engine receives SIGINT:
1. If an agentic step is running: forward SIGINT to the Claude process, wait for graceful exit
2. Save current pipeline state as the latest version (partial state is valid)
3. Write `{"status": "cancelled"}` to the execution log
4. Remove `.ctx/runs/current` symlink
5. Exit with non-zero code

The run can be inspected via `golem runs inspect <run-id>` to see partial state. Resumption is not supported in v1 — the user re-runs from scratch.

---

## Observability

### Event Types

The engine emits structured events for three consumers: CLI TUI, Flutter GUI, and post-mortem analysis.

| Event | Payload | When |
|---|---|---|
| `pipeline-start` | agent, goal, run ID | Engine begins |
| `step-start` | step name, type | Any step begins |
| `step-output` | step name, line of text | Streaming stdout from builtin/shell/agentic steps |
| `step-end` | step name, status, duration-ms | Any step completes |
| `loop-enter` | predicate, iteration N, max | `while` loop starts an iteration |
| `loop-exit` | predicate, reason (false/max) | `while` loop ends |
| `conditional-skip` | predicate, block name | `when`/`if` evaluated false |
| `pipeline-end` | status, duration-ms, run ID | Engine finishes |
| `error-occurred` | step, error type, action, attempt | Error handler fires |

### Event Delivery

| Consumer | Mechanism |
|---|---|
| CLI TUI | Go channel (in-process, same pattern as current builder loop) |
| Flutter GUI | Tail `log.json` via file watcher, or `golem runs watch <run-id>` which streams NDJSON to stdout |
| Post-mortem | Read `log.json` directly via `golem runs inspect` |

### Event Format in `log.json`

Events are written as newline-delimited JSON (NDJSON), appended in real time:

```json
{"type": "pipeline-start", "timestamp": "2026-03-12T14:22:00Z", "agent": "build-feature", "goal": "Add auth", "run-id": "run-20260312-142200"}
{"type": "step-start", "timestamp": "2026-03-12T14:22:01Z", "step": "git-setup", "step-type": "builtin"}
{"type": "step-end", "timestamp": "2026-03-12T14:22:01Z", "step": "git-setup", "status": "success", "duration-ms": 200}
{"type": "step-start", "timestamp": "2026-03-12T14:22:02Z", "step": "plan", "step-type": "agentic"}
{"type": "step-output", "timestamp": "2026-03-12T14:22:05Z", "step": "plan", "line": "Analyzing codebase..."}
{"type": "loop-enter", "timestamp": "2026-03-12T14:23:00Z", "predicate": "needs-work", "iteration": 1, "max": 3}
{"type": "error-occurred", "timestamp": "2026-03-12T14:24:00Z", "step": "implement", "error-type": "transient", "action": "retry", "attempt": 2}
{"type": "pipeline-end", "timestamp": "2026-03-12T14:26:00Z", "status": "success", "duration-ms": 240000, "run-id": "run-20260312-142200"}
```

### Display Modes

| Mode | Behavior |
|---|---|
| Default | Compact one-line-per-step: step name with spinner while running, then status + duration on completion |
| `--verbose` | Raw step output streamed inline. Agentic steps show Claude's full output via `StreamParser`. Builtin/shell steps show stdout/stderr. |

### `golem status --watch` Integration

Reads the `.ctx/runs/current` symlink to find the active run. Tails `log.json` for live step-by-step updates. When the run ends and the symlink is removed, shows final status.

---

## Agent Resolution

### Agent Name Resolution

Which agent to run:

```
1. --agent CLI flag                      → wins if present
2. .ctx/config.yaml agent:               → project default
3. ~/.config/golem/config.yaml agent:    → global default
4. "build-feature"                       → hardcoded fallback
```

### Agent File Resolution

Loading the YAML:

```
1. .ctx/agents/<name>.yaml               → project-local override
2. templates/agents/<name>.yaml           → embedded default
3. not found → error with available agents list
```

Error message:

```
agent "foo" not found. Available: build-feature, fix-bug, one-shot
Searched: .ctx/agents/, built-in templates
```

No personal agents directory (`~/.config/golem/agents/`) in v1. Easy to add as a middle resolution step later.

### YAML Parsing

Strict. Unknown fields produce a parse error with the field name and line number:

```
parse error in .ctx/agents/my-agent.yaml: unknown field "tool" at line 12
  (did you mean "tools"?)
```

Common typo suggestions: `tool`→`tools`, `write`→`writes`, `read`→`reads`.

### Agent Listing

`golem agents list` shows all available agents — built-in and project-local — with name, description, and source (built-in or project path).

---

## Security

### Shell Step Injection

Shell `command` fields are static strings from agent YAML. No variable interpolation from pipeline state into shell commands. The engine passes the command to `exec.Command("sh", "-c", command)` as-is. If dynamic commands are needed in the future, that would be a new step type with explicit escaping — not an extension of `shell`.

### GOLEM_TOOLS Trust

The engine sets `GOLEM_TOOLS` on the subprocess environment before spawning `claude -p`. The MCP server reads it at startup and filters tool registration. Claude cannot modify its own process environment. Even if Claude spawns child processes, those are not the MCP server — the server is already running with tools filtered. Not an attack surface.

### Session Output Sanitization

The engine reads `session-output.json` and extracts **only** keys declared in the step's `writes` list. Extra keys are silently dropped. Combined with reserved keys being engine-managed (`code`, `branch`, `base`), a rogue session cannot pollute pipeline state beyond its declared contract.

### No Eval Path

`session-output.json` values are treated as opaque JSON. They are serialized into prompt templates as text for downstream agentic steps. No code execution path from session output to engine behavior. Predicates check specific hardcoded fields (`status`, `verdict`) — not arbitrary expressions.

### Summary of Guarantees

- No pipeline state → shell command injection path
- No session output → arbitrary state pollution path
- No session output → code execution path
- Tool scoping is enforced at MCP server startup, not runtime

---

## Config Surface

### `.ctx/config.yaml`

```yaml
agent: build-feature
agent-opts:
  lint-cmd: "golangci-lint run"
  lint-fix-cmd: "golangci-lint run --fix"
  test-cmd: "go test ./..."
  test-timeout: "5m"
  ci-enabled: true
  draft: false
  pr-labels: ["golem"]
```

### Flow

1. `golem code` reads `agent-opts` from config
2. Merges into agent's `config` map (config values override agent defaults)
3. Engine passes merged config to primitives
4. Primitives read their commands from config

### Skip Behavior

When a required config value is nil (e.g., `lint-cmd`), the primitive writes `{"status": "skipped"}` rather than failing. Deterministic primitives are opt-in per project.

---

## Testing Strategy

### Unit Tests

| File | Test file | What's tested |
|---|---|---|
| `blueprint.go` | `blueprint_test.go` | Valid YAML parsing, invalid YAML errors, contract validation (missing reads, conditional writes requiring optional-reads, duplicate step names, unknown fields with "did you mean?" suggestions, reserved key handling) |
| `predicates.go` | `predicates_test.go` | Each predicate with present key, missing key (→ false), nil value, wrong type |
| `primitives.go` | `primitives_test.go` | Each primitive with mock `exec.Command`: pass, fail, skipped (no config), timeout, command-not-found. Verify output JSON shape for every scenario in the spec. |
| `engine.go` | `engine_test.go` | Full pipeline execution with `MockRunner`: state flow between steps, `while` loop termination (predicate false, max iterations), `when`/`if` branching, error retry/halt, cancellation via context, reserved key population, `session-output.json` reading and validation |

### MockRunner

```go
type MockRunner struct {
    Calls     []MockCall                // records prompt, tools, dir for each invocation
    Responses map[string]MockResponse   // keyed by step name
}
```

`MockResponse` contains: return string, error, and `session-output.json` content to write to the working directory before returning. This simulates the full agentic step contract. Implements both `Run` and `RunWithTools`.

### Integration Tests

`engine_integration_test.go`:
- Run full `one-shot` agent YAML against `MockRunner` — verify final state has all expected keys
- Run `build-feature` with `needs-work` loop — verify loop executes correct number of iterations
- Run agent with all primitives skipped (no config) — verify graceful completion
- Run agent with contract violation — verify load-time error

### Test Fixtures

Located in `internal/runner/testdata/`:
- `valid-agent.yaml`, `invalid-contract.yaml`, `unknown-fields.yaml`, `shell-steps.yaml`
- `session-output-valid.json`, `session-output-missing-keys.json`

Stdlib `testing` only. No external test frameworks.

---

## Go-Side Changes

### New Files

| File | Purpose |
|---|---|
| `internal/runner/engine.go` | Blueprint executor — walks pipeline, dispatches to step handlers, manages state, emits events, handles cancellation |
| `internal/runner/blueprint.go` | YAML parsing, pipeline construction, contract validation, "did you mean?" suggestions |
| `internal/runner/primitives.go` | Built-in primitive implementations: git-setup, lint, run-tests, ci-tests, create-pr |
| `internal/runner/predicates.go` | Built-in predicate registry and evaluation (missing key → false) |
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
| `cmd/code.go` | Add `engine: blueprint` path, load agent YAML, run engine. Add `--goal` flag (replaces `--task` for blueprint engine — `--task` is retained as alias for backwards compatibility). The `--goal` value populates the `goal` key in initial pipeline state. |
| `cmd/helpers.go` | Populate agent-opts from config into engine |
| `templates/embed.go` | Embed `agents/*.yaml` and `prompts/*.md` files |

### New Commands

| File | Command |
|---|---|
| `cmd/runs.go` | `golem runs list`, `golem runs inspect`, `golem runs clean`, `golem runs watch` |
| `cmd/agents.go` | `golem agents list` |

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

**Note on last-writer-wins with skipped primitives:** When `test-cmd` is not configured, `run-tests` writes `{"status": "skipped"}` which overwrites any `test-results` that the `implement` step reported in its `session-output.json`. This is intentional — the deterministic primitive result always takes precedence, even when that result is "skipped". The `review` step will see `{"status": "skipped"}` for test results in this case, not the agent's self-reported results.

---

## Migration Path

### Phase 1: Build Engine Alongside Existing Code

- Add `engine.go`, `blueprint.go`, `primitives.go`, `predicates.go`
- Add embedded default agents in `templates/agents/`
- Add prompt templates in `templates/prompts/`
- Add MCP tool filtering via `GOLEM_TOOLS` env var
- Add `RunWithTools` to `CommandRunner` interface
- Add `golem runs` and `golem agents` commands
- New config option: `engine: blueprint` (opt-in)
- `golem code` with `engine: blueprint` uses new engine
- Existing `engine: go` and `engine: dsl` continue working
- Pipeline state in `.ctx/runs/` coexists with `.ctx/state.yaml`

### Phase 2: Blueprint Becomes Default

- `golem code` uses blueprint engine by default
- `engine: legacy` keeps old Go loop as fallback
- `golem init` creates `.ctx/agents/` directory (empty — agents resolve to embedded defaults). Only generates agent YAML files if the user runs `golem agents init` to create project-local overrides for customization.
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
