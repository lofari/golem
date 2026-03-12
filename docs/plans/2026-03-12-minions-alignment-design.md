# Minions Alignment: Deterministic Primitives, Tool Scoping & One-Shot Agents

> Date: 2026-03-12
> Status: Draft

## Summary

Add deterministic pipeline primitives (`lint`, `run-tests`, `ci-tests`, `create-pr`), tool scoping per primitive, and a `one-shot` agent to golem-dsl. These changes close the five largest gaps between golem and Stripe's Minions architecture while preserving golem's existing strengths (graph intelligence, persistent state, skill-based guidance).

## Decisions

- **Explicit config over auto-detection** — Lint/test commands are configured per-project via `agent-opts`, not auto-detected from project stack. Predictable and debuggable for autonomous agents.
- **GitHub Actions via `gh` CLI** — CI integration uses `gh run` commands. Lowest friction, no custom API client needed.
- **Full PR workflow** — `create-pr` handles branch push, structured body generation, CODEOWNERS reviewer detection. Not minimal.
- **Primitive defaults with agent-level override** — Primitives declare default `:tools`, agent definitions can narrow or widen per step. `nil` means all tools (backwards-compatible).
- **`one-shot` is just another agent** — No special CLI treatment. Invoked via `golem code --agent one-shot --goal "..."`.

---

## New Deterministic Primitives

All four primitives use `:session false` — they execute locally without spawning a Claude session.

### `lint`

```clojure
(defprimitive lint
  "Run project linter with optional autofix."
  {:reads  [:code]
   :writes [:lint-results]
   :session false}
  ...)
```

**Inputs from config:**
- `lint-cmd` — lint command (e.g., `"golangci-lint run"`)
- `lint-fix-cmd` — autofix command (e.g., `"golangci-lint run --fix"`)

**Behavior:**
1. If `lint-fix-cmd` is set, run it first (applies mechanical fixes)
2. Run `lint-cmd` to verify
3. Parse stdout into structured issues: `[{:file "f.go" :line 12 :message "..."}]`
4. Timeout: 30s default

**Output:**

| Scenario | Result |
|---|---|
| Not configured | `{:status :skipped :reason "no lint-cmd"}` |
| Passes | `{:status :pass :issues []}` |
| Fails after autofix | `{:status :fail :issues [...] :fixed [...]}` |
| Fails, no fix cmd | `{:status :fail :issues [...]}` |
| Timeout | `:transient` error → retry handler |
| Command not found | `:unrecoverable` → halt |

### `run-tests` (enhanced)

```clojure
(defprimitive run-tests
  "Run local test suite."
  {:reads  [:code]
   :optional-reads [:lint-results]
   :writes [:test-results]
   :session false}
  ...)
```

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
| Not configured | `{:status :skipped}` |
| All pass | `{:status :pass :duration-ms N}` |
| Failures | `{:status :fail :failures [{:test "..." :file "..." :message "..."}] :duration-ms N}` |
| Timeout | `:transient` error → retry |
| Command not found | `:unrecoverable` → halt |

### `ci-tests`

```clojure
(defprimitive ci-tests
  "Trigger CI via GitHub Actions and collect results."
  {:reads  [:code]
   :writes [:ci-results]
   :session false}
  ...)
```

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
| `gh` not on PATH | `:unrecoverable` → halt with "Install GitHub CLI" |
| No git remote | `:unrecoverable` → halt |
| Push fails | `:unrecoverable` → halt |
| No workflow triggered | `{:status :skipped :reason "no workflow triggered"}` |
| CI passes | `{:status :pass :run-url "..."}` |
| CI fails | `{:status :fail :run-url "..." :failures [...]}` |
| CI timeout (>15min) | `{:status :fail :reason "timeout" :run-url "..."}` |
| PR already exists | Detect via `gh pr list --head <branch>`, return existing PR |

### `create-pr`

```clojure
(defprimitive create-pr
  "Create a pull request with structured body."
  {:reads  [:code :goal]
   :optional-reads [:plan :test-results :ci-results :lint-results]
   :writes [:pr-result]
   :session false}
  ...)
```

**Behavior:**
1. Determine base branch: `gh repo view --json defaultBranchRef`
2. `git push -u origin <branch>`
3. Build PR body from state:
   - **Summary** from `:goal` and `:plan` (if present)
   - **Test results** section: local pass/fail + CI pass/fail with run URL
   - **Lint status**: pass/fail with issue count
   - **Files changed**: from `git diff --stat <base>..HEAD`
4. Detect CODEOWNERS: parse `CODEOWNERS` file, match changed files, pass `--reviewer`
5. `gh pr create --title "<title>" --body "<body>"` with reviewers if found
6. Return PR URL and number

**Output:**

| Scenario | Result |
|---|---|
| `gh` not on PATH | `:unrecoverable` → halt |
| PR already exists | Return existing: `{:url "..." :number N :existing true}` |
| No commits vs base | `{:status :skipped :reason "no changes"}` |
| No CODEOWNERS | Create PR without reviewers (not an error) |
| API error | `:transient` → retry |

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

### Primitive Defaults

Agentic primitives (`:session true`) declare a `:tools` key listing MCP tools available during their session:

```clojure
(defprimitive plan
  {:tools [:semantic_search :find_callers :find_dependencies :find_co_changed]}
  ...)

(defprimitive implement
  {:tools [:semantic_search :find_callers :find_dependencies :find_dependents
           :find_co_changed :find_execution_failures
           :lsp_definition :lsp_references :lsp_hover :lsp_diagnostics]}
  ...)

(defprimitive review
  {:tools [:semantic_search :find_callers :find_dependencies]}
  ...)

(defprimitive reflect
  {:tools [:semantic_search]}
  ...)

(defprimitive research
  {:tools [:semantic_search :find_callers :find_dependencies
           :find_co_changed :find_execution_failures :get_runtime_trace]}
  ...)
```

Deterministic primitives (`:session false`) have no `:tools` key — they don't spawn Claude sessions.

### Agent-Level Override

Agent definitions override tools per step:

```clojure
(defagent build-feature
  (plan {:tools [:semantic_search]})           ;; narrower than default
  (implement {:tools [:semantic_search         ;; add graph_query
              :find_callers :graph_query]})
  ...)
```

### Resolution

```
1. Agent step override   {:tools [...]}  ← wins if present
2. Primitive default     {:tools [...]}  ← fallback
3. nil                   → all tools     ← backwards-compatible
```

### Mechanism

The engine resolves the tool list and passes it to the session adapter. The adapter passes `--tools <comma-separated>` to `golem session`. The Go binary generates an MCP config exposing only those tools.

**Go-side change:** Add `--tools` flag to `golem session` (or whatever the session command is named). `WriteMCPConfig` accepts an optional filter slice. If nil, all tools registered. If non-nil, only matching tools registered.

```go
// internal/mcp/server.go
func (s *GolemServer) FilteredTools(allowed []string) []mcp.Tool
```

---

## Agent Definitions

### `one-shot` (new)

```clojure
(defagent one-shot
  "One task, one PR. Implement, validate, ship."
  {:initial-state [:goal]
   :config {:lint-cmd nil
            :test-cmd nil
            :ci-enabled false}}

  (implement)
  (lint)
  (run-tests)
  (when ci-enabled? (ci-tests))
  (when ci-failed?
    (implement {:tools [:semantic_search]})
    (ci-tests))
  (create-pr)

  (on-error :transient (retry {:max 2}))
  (on-error :contract-violation (snapshot-and-halt)))
```

### `build-feature` (updated)

```clojure
(defagent build-feature
  "Plan, implement with lint/test feedback loops, review, ship."
  {:initial-state [:goal]
   :config {:lint-cmd nil
            :test-cmd nil
            :ci-enabled false}}

  (plan)
  (implement)
  (lint)
  (run-tests)
  (review)
  (while needs-work? {:max 3}
    (implement)
    (lint)
    (run-tests)
    (review))
  (when ci-enabled?
    (ci-tests)
    (when ci-failed?
      (implement)
      (lint)
      (run-tests)
      (ci-tests)))
  (create-pr)

  (on-error :transient        (retry {:max 3}))
  (on-error :malformed-output (re-run {:hint "Check contract schema."}))
  (on-error :contract-violation (snapshot-and-halt)))
```

### `fix-bug` (updated)

```clojure
(defagent fix-bug
  "Research, fix, validate, ship."
  {:initial-state [:goal]
   :config {:lint-cmd nil
            :test-cmd nil
            :ci-enabled false}}

  (research)
  (implement)
  (lint)
  (run-tests)
  (when failed?
    (implement)
    (lint)
    (run-tests))
  (when ci-enabled? (ci-tests))
  (create-pr)

  (on-error :transient (retry {:max 2})))
```

### New Predicates

```clojure
(defpred ci-enabled?
  "CI integration is configured."
  (get-in config [:ci-enabled]))

(defpred ci-failed?
  "CI tests failed."
  (= :fail (get-in state [:ci-results :status])))

(defpred lint-failed?
  "Lint check failed."
  (= :fail (get-in state [:lint-results :status])))
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
2. Passes as `--opt key=value` flags to `golem-dsl run`
3. DSL merges into agent's `:config` map (config values override agent defaults)
4. Primitives read from merged config

### Agent `:config` Defaults

The `:config` map in `defagent` declares defaults. `agent-opts` overrides them. This keeps agent `.clj` files generic while projects supply specific commands.

When a required config value is nil (e.g., `lint-cmd`), the primitive skips with `{:status :skipped}` rather than failing. This makes the deterministic primitives opt-in per project.

---

## Go-Side Changes

### 1. MCP Tool Filtering

**File:** `internal/mcp/server.go`

Add `FilteredTools(allowed []string) []mcp.Tool` method. If `allowed` is nil, return all tools. Otherwise, return only tools whose names match.

**File:** `internal/mcp/tools.go` (or wherever `WriteMCPConfig` lives)

Accept optional `tools []string` parameter for filtering.

### 2. `golem session` Command

**File:** `cmd/session.go` (new, if not already present)

Thin wrapper exposing `ClaudeRunner.Run()` as a CLI command:

```
golem session --prompt <file> --dir <dir> --max-turns N [--tools tool1,tool2,...] [--sandbox] [--plugin-dir ...]
```

Flags:
- `--prompt` — prompt file path (required)
- `--dir` — working directory (required)
- `--max-turns` — Claude max turns (default from config)
- `--tools` — comma-separated MCP tool filter (optional, nil = all)
- `--sandbox`, `--plugin-dir`, `--model` — passthrough to `ClaudeRunner`

Behavior:
1. Read prompt file
2. Write filtered MCP config via `WriteMCPConfig(dir, tools)`
3. Spawn `claude -p` via `ClaudeRunner.Run()`
4. Exit with Claude's exit code

### 3. Config Passthrough

Already implemented: `DSLRunner.buildArgs()` iterates `AgentOpts` and passes `--opt k=v`. `cmd/helpers.go` populates `AgentOpts` from config's `agent-opts`. No changes needed.

### No Other Go Changes

- Primitive implementations live in Clojure (shell out to lint/test/gh commands)
- State management stays DSL-native (EDN)
- Error handling stays DSL-native
- Event streaming protocol unchanged

---

## Interaction Between Primitives

### Lint/Test Failures Drive Re-Implementation

When lint or tests fail mid-pipeline, the `review` primitive acts as the quality gate. It sees `:lint-results` and `:test-results` in its context and sets `:verdict :needs-work` if either failed. This keeps the `while needs-work?` loop going without special wiring.

The next `implement` node receives all prior results in its prompt, so the agent knows exactly what to fix.

### State Flow Through Pipeline

```
plan       → writes :plan
implement  → reads :plan, writes :code :test-results
lint       → reads :code, writes :lint-results
run-tests  → reads :code, writes :test-results (overwrites implement's)
review     → reads :code :test-results :lint-results, writes :review-feedback
...loop...
ci-tests   → reads :code, writes :ci-results
create-pr  → reads :code :goal, optionally reads :plan :test-results :ci-results :lint-results, writes :pr-result
```

### Skipped Primitives

When `lint-cmd` or `test-cmd` is not configured, the primitive writes `{:status :skipped}`. Downstream primitives handle this gracefully — `review` treats skipped lint/tests as neutral (not failure, not pass). `create-pr` omits skipped sections from the PR body.

---

## What Doesn't Change

- State model (immutable Clojure maps with versioned snapshots)
- Contract system (`:reads`/`:writes` compile-time validation)
- Error handler DSL (`on-error`, retry, re-run, halt)
- Event streaming protocol (NDJSON on stdout)
- CLI interface (`golem-dsl run`, `list`, `inspect`)
- Knowledge graph (Go-side, accessed via MCP tools)
- Warden sandbox (Go-side, transparent to DSL)
- Prompt template system (Stencil templates in `resources/prompts/`)
