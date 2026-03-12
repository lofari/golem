# Stripe Minions: Deep Research & Golem Improvement Roadmap

## Part 1: How Stripe's Minions Work

### 1.1 Overview

Stripe's Minions are fully autonomous, one-shot coding agents that produce production-ready pull requests without any human-written code. Over 1,300 PRs merge weekly at Stripe that are completely minion-produced (human-reviewed, zero human code). They operate on a codebase of hundreds of millions of lines, primarily Ruby with Sorbet typing — a stack natively unfamiliar to LLMs.

The key insight: **"What's good for humans is good for agents."** Stripe invested in developer tooling (devboxes, linting, CI, documentation) and discovered those same investments compound into agent reliability.

### 1.2 Infrastructure: Devboxes

Minions run on **devboxes** — AWS EC2 instances that serve as standardized developer environments. These provide three critical properties:

| Property | What it means |
|---|---|
| **Parallelizability** | Multiple agents work on logically separate tasks simultaneously |
| **Predictability** | Standardized environments reduce interference and token waste |
| **Isolation** | Confined blast radius — agents can't reach production or real user data |

Devboxes are **pre-warmed** and spin up in ~10 seconds with:
- Pre-cloned repositories
- Warmed Bazel and type-checking caches
- Running code generation services
- QA environment access only (no production, no real user data, no arbitrary egress)

This is equivalent to golem's **warden** sandbox but at infrastructure scale. Devboxes are the same machines human engineers use — the agent tooling is identical to human tooling.

### 1.3 Agent Foundation: Customized Goose

Stripe forked **Block's goose** (one of the first widely used coding agents) in late 2024. They customized the orchestration to interleave agent loops with deterministic operations. Key distinction: Minions are **unattended** — no human-facing features like interruptibility or human-triggered commands.

### 1.4 The Blueprint: Hybrid Orchestration

This is the core architectural innovation. Rather than pure workflow DAGs or simple agent loops, Minions use **"blueprints"** — a hybrid state machine that intermixes:

- **Deterministic nodes** (rectangles): Git operations, linting, code pushing — guaranteed to complete
- **Agentic nodes** (clouds): "Implement task", "Fix CI failures" — LLM exercises judgment

```
┌──────────────┐     ┌ ─ ─ ─ ─ ─ ─ ┐     ┌──────────────┐
│ Git checkout  │────▶  Implement    ────▶│  Run linter   │
│ + setup       │     │   task       │     │  (< 5 sec)    │
└──────────────┘     └ ─ ─ ─ ─ ─ ─ ┘     └──────┬───────┘
                                                  │
                                          ┌───────▼───────┐
                                          │  Git push      │
                                          └───────┬───────┘
                                                  │
                                          ┌───────▼───────┐     ┌ ─ ─ ─ ─ ─ ─ ┐
                                          │  CI tests      │────▶  Fix CI
                                          │  (selective)   │     │  failures    │
                                          └───────┬───────┘     └ ─ ─ ─ ┬ ─ ─ ┘
                                                  │                     │
                                          ┌───────▼───────┐     ┌──────▼───────┐
                                          │  Apply CI      │     │  2nd CI push  │
                                          │  autofixes     │     │  (FINAL)      │
                                          └───────┬───────┘     └──────────────┘
                                                  │
                                          ┌───────▼───────┐
                                          │  Create PR     │
                                          └──────────────┘
```

**Why blueprints beat pure agent loops:**
1. Guaranteed completion of anticipated subtasks (linting always runs)
2. Reduced token consumption and CI costs at scale
3. Fewer failure opportunities — "putting LLMs into contained boxes"
4. Easy context engineering through tool constraints and system prompt per node
5. Teams can create specialized blueprints for unique workflows (e.g., LLM-assisted migrations)

### 1.5 Context Gathering

#### MCP Integration: Toolshed
Stripe built **Toolshed**, a centralized internal MCP server hosting ~500 tools spanning internal systems and SaaS platforms. This provides:
- Internal documentation lookup
- Ticket details
- Build statuses
- Code intelligence via Sourcegraph search
- Feature flag management

Tools are **intentionally curated per agent type** — minions receive a small default subset with optional expansion. This prevents context window pollution.

#### Rule Files (Scoped, Not Global)
Given the massive codebase, global rules would consume the entire context window. Instead:
- Directory-specific rule files
- File-pattern-scoped guidance
- Automatic attachment as agents traverse the filesystem
- Standardized on Cursor's rule format
- Synchronized across minions, Cursor, and Claude Code

### 1.6 Testing & Validation Pipeline

**Layer 1: Local (shift-left)**
- Automated lint on each git push
- Heuristic-based test selection
- Executes in under 5 seconds
- Catches 80% of issues before CI

**Layer 2: CI (selective, max 2 rounds)**
- Selects from 3+ million tests using heuristics
- Auto-applies CI autofixes for known failure patterns
- Failures without autofixes go back to agent for one remediation attempt
- **Hard limit: 2 CI rounds maximum** — diminishing returns after that

### 1.7 Entry Points

Engineers invoke minions through:
- **Slack** (primary — just message a bot)
- CLI and web interfaces
- Internal applications (docs platform, feature flags, ticketing)
- Automated ticket creation for detected flaky tests

Context is gathered from Slack threads, links, and deterministic MCP tool execution before the agent loop starts.

### 1.8 One-Shot Philosophy

The North Star: **produce a PR without any human code**. But pragmatically:
- A minion run that's not entirely correct is still an excellent starting point
- Engineers frequently spin up multiple minions in parallel
- Particularly useful during on-call to resolve many small issues simultaneously

---

## Part 2: Golem vs. Minions — Gap Analysis

### 2.1 Architecture Comparison

| Aspect | Stripe Minions | Golem |
|---|---|---|
| **Orchestration** | Blueprint (hybrid deterministic + agentic state machine) | Sequential iteration loop with strategy |
| **Agent foundation** | Forked goose | Wraps `claude -p` |
| **Isolation** | Pre-warmed devboxes (EC2) | Warden containers (Docker/Firecracker) |
| **Context** | Toolshed (~500 MCP tools) + scoped rules | MCP server (~20 tools) + graph + embeddings |
| **Testing** | Local lint + selective CI (3M tests) | Delegated to agent (no structured pipeline) |
| **Entry points** | Slack, CLI, web, ticketing, automated | CLI only |
| **Iteration limit** | 2 CI rounds, single blueprint execution | Up to 20 iterations |
| **Parallelism** | Multiple devboxes per engineer | Git worktrees (same machine) |
| **State** | Ephemeral (one-shot, no persistent state) | Persistent YAML state across iterations |
| **Review** | Human review of final PR | Automated review loop (`golem review`) |
| **Feedback loop** | <5s lint, then selective CI | Agent runs tests itself |
| **Scope per run** | One task → one PR | Multi-task project with dependencies |

### 2.2 What Golem Already Has That Aligns

1. **MCP server with graph tools** — Golem's semantic search, co-change analysis, and execution history are more sophisticated than what Stripe describes
2. **Warden sandbox** — Equivalent to devboxes for isolation
3. **Strategy system** — Adaptive failure detection (thrashing, deadlocks, consecutive failures)
4. **Parallel execution** — Git worktrees for concurrent tasks
5. **Superpowers plugin** — Structured skill-based guidance (TDD, debugging, verification)
6. **Snapshot/restore** — State safety net
7. **Execution history** — Tracking bash commands, test results, errors in graph DB

### 2.3 Critical Gaps

#### Gap 1: No Blueprint/Pipeline Architecture
**Minions:** Deterministic nodes guarantee linting, testing, git operations always happen in the right order.
**Golem:** Everything is delegated to the agent within a single prompt. The agent decides when/whether to lint, test, commit. This means:
- Tests might not run
- Linting might be skipped
- Git operations might fail silently
- No structured feedback loop

#### Gap 2: No Structured Testing Pipeline
**Minions:** Local lint (<5s) + selective CI (2 rounds max) with autofixes.
**Golem:** Agent is told to test but there's no enforcement. No heuristic test selection. No autofix application. No CI integration.

#### Gap 3: No Task-to-PR Mapping
**Minions:** One task → one PR. Clean, reviewable, atomic.
**Golem:** Multi-task state file with dependencies. PRs are not automatically created. Work accumulates across iterations without clean boundaries.

#### Gap 4: No Scoped Rule System
**Minions:** Directory-specific rules attached as agent traverses filesystem.
**Golem:** Single prompt template with fixed context. No conditional rule injection based on which files the agent is touching.

#### Gap 5: No Entry Point Diversity
**Minions:** Slack, web, CLI, ticketing, automated triggers.
**Golem:** CLI only. No webhook/API for external triggers.

#### Gap 6: No Shift-Left Feedback
**Minions:** Deterministic lint runs in <5s between agentic nodes.
**Golem:** Agent runs linting itself (or doesn't). No fast-feedback deterministic steps between iterations.

#### Gap 7: Limited Tool Curation
**Minions:** Intentionally small default MCP tool subset per agent type, expandable.
**Golem:** All MCP tools always available. No per-task tool scoping.

---

## Part 3: Improvement Roadmap

### Priority 1: Blueprint Architecture (High Impact, Medium Effort)

**What:** Replace the flat iteration loop with a blueprint-style state machine that intermixes deterministic and agentic nodes.

**Implementation sketch:**

```go
// internal/runner/blueprint.go

type NodeType int
const (
    NodeDeterministic NodeType = iota
    NodeAgentic
)

type BlueprintNode struct {
    Name     string
    Type     NodeType
    // For deterministic nodes:
    Command  func(ctx context.Context, dir string) error
    // For agentic nodes:
    Prompt   string
    MaxTurns int
    Tools    []string  // scoped MCP tools for this node
    // Transitions:
    OnSuccess string   // next node name
    OnFailure string   // fallback node name
    MaxRetries int
}

type Blueprint struct {
    Name  string
    Nodes map[string]*BlueprintNode
    Start string
}
```

**Default "build-feature" blueprint:**
```
1. [deterministic] git-setup: checkout branch, sync state
2. [agentic]       implement: write code (scoped tools: graph, state)
3. [deterministic] lint: run project linter (<5s feedback)
4. [agentic]       fix-lint: fix any lint errors (if lint failed)
5. [deterministic] test-local: run relevant tests
6. [agentic]       fix-tests: fix test failures (if tests failed, max 2 retries)
7. [deterministic] commit: git add + commit with conventional message
8. [deterministic] update-state: mark task done in state.yaml
```

**Why this matters:**
- Linting and testing ALWAYS happen (not optional)
- Agent gets fast feedback between nodes
- Each agentic node can have scoped tools and tailored prompts
- Failures are contained — lint failure doesn't waste a full iteration
- Teams can define custom blueprints (migrations, refactors, etc.)

### Priority 2: Shift-Left Testing Pipeline (High Impact, Medium Effort)

**What:** Add deterministic testing nodes that run between agentic iterations.

**Components:**

```go
// internal/runner/feedback.go

type FeedbackPipeline struct {
    Steps []FeedbackStep
}

type FeedbackStep struct {
    Name    string
    Command string            // e.g., "go vet ./..."
    Timeout time.Duration     // e.g., 10s
    Parse   func(output string) []Issue
}

type Issue struct {
    File    string
    Line    int
    Message string
    AutoFix string  // optional: command to auto-fix
}
```

**Default steps:**
1. **Lint** — `golangci-lint run` / language-appropriate linter (< 10s)
2. **Type check** — `go build ./...` / `tsc --noEmit` (< 30s)
3. **Relevant tests** — run tests for changed files only (< 2min)

**Autofix support:**
- If a lint step has known autofixes (`gofmt -w`, `eslint --fix`), apply them deterministically
- Don't waste agent turns on mechanical fixes

### Priority 3: One-Task-One-PR Mode (Medium Impact, Low Effort)

**What:** Add a `--one-shot` mode that mirrors Minions' task-to-PR mapping.

```bash
golem code --one-shot --task "Add rate limiting to /api/users endpoint"
```

**Behavior:**
1. Create feature branch: `golem/<sanitized-task-name>`
2. Run single blueprint execution for that one task
3. Create PR automatically via `gh pr create`
4. Exit

**This enables:**
- Clean, atomic PRs (easy to review)
- Parallel execution: run multiple `golem code --one-shot` in separate worktrees
- Slack/webhook integration: trigger one-shot runs from external systems

### Priority 4: Scoped Rule System (Medium Impact, Medium Effort)

**What:** Support directory-scoped rule files that are injected into context based on which files the agent is working on.

**Implementation:**

```go
// internal/runner/rules.go

type RuleFile struct {
    Path      string   // e.g., "internal/api/.rules.md"
    Glob      string   // e.g., "internal/api/**/*.go"
    Content   string
}

func GatherRules(dir string, changedFiles []string) []RuleFile {
    // Walk up from each changed file, collecting .rules.md files
    // Also check .cursorrules format for compatibility
    // Deduplicate and return
}
```

**Rule file format (compatible with Cursor):**
```markdown
---
globs: ["internal/api/**/*.go"]
---
# API Layer Rules
- All endpoints must validate input using the `validate` package
- Use `errors.Wrap` not `fmt.Errorf` for error wrapping
- Every handler must log request ID from context
```

Rules would be injected into the prompt for the relevant blueprint node, keeping context focused.

### Priority 5: External Trigger API (Medium Impact, Medium Effort)

**What:** Add a lightweight HTTP/webhook server that accepts task submissions.

```bash
golem serve --port 8080
```

**Endpoints:**
- `POST /tasks` — submit a new task (returns task ID)
- `GET /tasks/:id` — check status
- `GET /tasks/:id/pr` — get PR URL when done
- `POST /webhooks/slack` — Slack bot integration

**This enables:**
- Slack-driven agent invocation (like Minions)
- CI-triggered runs (flaky test detected → auto-fix agent)
- Dashboard/web UI integration
- Parallel task submission from external systems

### Priority 6: Tool Scoping Per Node (Low Effort, Medium Impact)

**What:** Allow blueprints to specify which MCP tools are available at each agentic node.

**Why:** The "implement" node needs graph search, semantic search, and code analysis tools. The "fix-lint" node only needs the lint output and file editing. Reducing available tools reduces confusion and token waste.

```go
// In blueprint node definition:
BlueprintNode{
    Name:  "implement",
    Tools: []string{"semantic_search", "find_callers", "find_dependencies", "find_co_changed"},
}

BlueprintNode{
    Name:  "fix-lint",
    Tools: []string{"mark_task"},  // minimal tools, lint output injected as context
}
```

### Priority 7: Devbox-Style Pre-Warming (Low Impact Short-Term, High Impact at Scale)

**What:** Pre-warm warden containers with project dependencies cached.

**Implementation:**
```bash
golem warmup  # builds warden image with project deps pre-installed
```

- Cache `go mod download`, `npm install`, `pip install` results in a project-specific warden image
- Tag: `warden:golem-<project-hash>`
- Cuts cold-start from minutes to seconds
- Mirrors Stripe's pre-warmed devboxes

### Priority 8: PR Auto-Creation (Low Effort, High Impact)

**What:** Automatically create PRs at the end of successful runs.

Currently golem produces code but doesn't create PRs. Adding `--create-pr` flag:

```go
// At end of successful blueprint:
if cfg.CreatePR {
    title := fmt.Sprintf("[golem] %s", taskName)
    body := buildPRBody(sessionLog, changedFiles, testResults)
    exec.Command("gh", "pr", "create", "--title", title, "--body", body).Run()
}
```

### Priority 9: Execution Metrics & Observability (Medium Effort, Medium Impact)

**What:** Track success rates, token usage, time-to-PR, failure modes.

**Metrics to track:**
- Tasks attempted vs completed
- Tokens consumed per task
- Time from start to PR
- Lint pass rate (before/after agent)
- Test pass rate (before/after agent)
- CI rounds needed
- Most common failure modes

Store in graph.db or a separate metrics table. Surface via `golem status --metrics`.

---

## Part 4: Implementation Order

Based on impact and effort, recommended order:

| Phase | Improvement | Impact | Effort |
|---|---|---|---|
| **Phase 1** | Blueprint architecture | High | Medium |
| **Phase 1** | Shift-left testing pipeline | High | Medium |
| **Phase 1** | PR auto-creation | High | Low |
| **Phase 2** | One-task-one-PR mode | Medium | Low |
| **Phase 2** | Tool scoping per node | Medium | Low |
| **Phase 2** | Scoped rule system | Medium | Medium |
| **Phase 3** | External trigger API | Medium | Medium |
| **Phase 3** | Devbox pre-warming | Medium | Medium |
| **Phase 3** | Execution metrics | Medium | Medium |

**Phase 1** transforms golem from "agent loop with state" to "structured pipeline with agent nodes" — the single biggest architectural shift that Minions demonstrates. Phases 2 and 3 add the ecosystem features that make Minions practical at scale.

---

## Part 5: What Golem Has That Minions Don't

It's worth noting areas where golem is already more sophisticated:

1. **Semantic code graph with embeddings** — Minions use Sourcegraph for code search; golem has a local graph DB with ONNX embeddings, co-change analysis, and structural expansion. This is a significant advantage for context relevance.

2. **Persistent state across iterations** — Minions are one-shot; golem can handle multi-task projects with dependencies, decisions, and pitfalls tracked across sessions. This enables more complex, multi-day work.

3. **Adaptive strategy** — Golem's strategy system detects thrashing, deadlocks, and unproductive streaks. Minions don't appear to have equivalent adaptive behavior.

4. **Superpowers skill system** — Structured guidance for TDD, debugging, and verification that shapes agent behavior through prompt engineering. Minions use rule files but don't appear to have the same depth of workflow enforcement.

5. **Review loop** — Golem has a built-in `golem review` that creates [review] tasks. Minions rely entirely on human review after PR creation.

6. **Snapshot/restore** — State safety net that Minions don't need (one-shot, no persistent state).

The optimal path is to adopt Minions' **blueprint architecture and shift-left testing** while preserving golem's **graph intelligence, persistent state, and skill-based guidance**. This combination would be more capable than either system alone.

---

## Appendix: Key Terminology

| Term | Stripe | Golem Equivalent |
|---|---|---|
| Devbox | Pre-warmed EC2 instance | Warden container |
| Blueprint | Hybrid state machine | Builder loop (to be replaced) |
| Toolshed | Centralized MCP server (~500 tools) | Golem MCP server (~20 tools) |
| Rule files | Directory-scoped `.cursorrules` | Prompt template (single) |
| Minion | One-shot autonomous agent | `golem code` session |
| Autofix | Deterministic fix for known CI failure | Not implemented |
| Shift-left | Fast local feedback before CI | Not implemented |
