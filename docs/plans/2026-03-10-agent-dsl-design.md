# Golem Agent DSL — Design Document

**Date:** 2026-03-10

---

## Purpose

A Clojure DSL that compiles agent programs into execution graphs. The Golem runtime walks the graph, spawns CLI agent sessions, manages immutable state, and snapshots every step. The DSL makes orchestration logic explicit, composable, and debuggable.

```clojure
(defagent build-feature
  (plan goal)
  (implement)
  (review)
  (while needs-work? {:max 3}
    (implement)
    (review)))
```

---

## Architecture

### Relationship to Existing Golem

The DSL is a **new layer above the Go binary**, not a rewrite. The Go binary becomes a session adapter — the DSL calls `golem session` to spawn Claude with sandbox support, plugins, and MCP config. The two tools coexist: use `golem code` for simple projects, `golem-dsl run` when you need composable orchestration.

The DSL owns its own state. No sync with `.ctx/state.yaml`.

### Compilation Model: Hybrid Macros

No standalone compiler pipeline. `defagent` macros do the work at expand time:

1. Resolve each step to a registered primitive
2. Pull contracts from primitive definitions
3. Verify `:reads` dependencies — every key a step reads must be written by a prior step or declared as initial state
4. Compile control flow (`while`, `if`, `when`) into conditional execution with loop counters
5. Register the graph structure (nodes, edges, contracts) in a global atom for inspection
6. Emit executable Clojure code

This gives compile-time contract verification without a separate compiler pipeline. The macro system *is* the compiler.

---

## DSL Specification

### Agent Definition

```clojure
(defagent build-feature
  "Builds a feature from a goal description."
  {:initial-state [:goal]
   :budget {:max-usd 0.50}}

  (plan goal)
  (implement)
  (review)
  (while needs-work? {:max 3}
    (implement)
    (review))

  (on-error :transient        (retry {:max 3}))
  (on-error :malformed-output (re-run {:hint "Check contract schema."}))
  (on-error :contract-violation (snapshot-and-halt)))
```

### Primitives

Each primitive declares its contract inline — no separate `defcontract`. Primitives either spawn a CLI agent session (default) or execute locally (`:session false`).

```clojure
(defprimitive implement
  "Writes code, runs tests, fixes failures."
  {:reads  [:goal :plan]
   :optional-reads [:reflection :review-feedback]
   :writes [:code :test-results]
   :session true}
  (fn [state context adapter]
    ;; implementation
    ))
```

Built-in primitives:

| Primitive | Granularity | Session | Reads | Writes |
|-----------|-------------|---------|-------|--------|
| `plan` | Fine | Yes | `[:goal]` | `[:plan]` |
| `implement` | Coarse | Yes | `[:goal :plan]` | `[:code :test-results]` |
| `review` | Fine | Yes | `[:code :test-results]` | `[:review-feedback]` |
| `reflect` | Fine | Yes | configurable | `[:reflection]` |
| `research` | Fine | Yes | `[:goal]` | `[:research-context]` |
| `run-tests` | Fine | No | `[:code]` | `[:test-results]` |

**Granularity principle:** Strategic primitives (plan, reflect, review, research) are fine-grained — one focused session. Execution primitives (implement) are coarse — the agent writes code, runs tests, and fixes failures within a single session. The DSL controls the outer loop; the agent controls the inner loop.

### Predicates

```clojure
;; Built-in
failed?          ;; (:test-results state) has :status :fail
needs-work?      ;; (:review-feedback state) has :verdict :needs-work

;; Custom
(defpred coverage-low?
  (< (get-in state [:test-results :coverage]) 80))
```

### Control Flow

```clojure
(if failed? (fix-code))
(if poor-quality? (improve) (finalize))
(while failed? {:max 5} (implement))
(when needs-research? (research))
```

When a loop hits its max, it exits and sets `:loop-exhausted true` in state. It does not throw.

### Composition

```clojure
(defagent ship-feature
  {:initial-state [:goal]}
  (invoke build-feature)
  (invoke write-docs))
```

- Child receives parent state filtered to child's `:initial-state` keys
- Child's final state merges back — child's `:writes` keys overwrite parent's
- **Strict:** child can only write keys declared in its contracts; parent must declare those keys too
- Key conflicts in conditional branches are compile-time warnings
- No parallel composition in Phase 1

---

## State Model

State is an immutable Clojure map. Each step produces a new version.

```clojure
;; After (plan goal)
{:goal "Build a CSV to JSON converter"
 :plan [{:step 1 :desc "Parse CLI args"}
        {:step 2 :desc "Read CSV with streaming"}
        {:step 3 :desc "Write JSON output"}]}

;; After (implement)
{:goal "Build a CSV to JSON converter"
 :plan [...]
 :code {:files ["converter.go" "converter_test.go"]
        :language "go"}
 :test-results {:status :pass :failures []}}
```

**Rules:**
- State keys are keywords, max two levels of nesting
- Primitives can only write keys declared in `:writes` — runtime rejects extras
- `:reads` keys must be present; `:optional-reads` may be absent
- `:code` holds metadata (file paths, language) — actual files live in the working directory

**Persistence:** EDN format, not YAML. No type coercion issues.

```
runs/
  run-042/
    state-v0.edn       # initial
    state-v1.edn       # after plan
    state-v2.edn       # after implement
    state-v3.edn       # after review
    log.edn            # execution log
    graph.edn          # registered graph structure
    program.clj        # source agent program
    sessions/          # raw session transcripts
```

---

## Session Adapter

```clojure
(defprotocol SessionAdapter
  (spawn [this prompt working-dir opts]
    "Launch a session. Returns an opaque handle.")
  (wait [this handle timeout-ms]
    "Block until complete. Returns {:exit-code N}")
  (read-output [this handle]
    "Read session output. Returns map of state keys."))
```

### Claude Code Adapter

Calls the Go binary: `golem session --prompt <file> --dir <working-dir> [--sandbox] [--plugin-dir ...]`

The Go binary handles sandbox, plugins, MCP config, and spawns `claude -p` with the right flags.

### Output Parsing

Two mechanisms combined:
- **Filesystem diff:** compare working directory before/after for code files
- **Structured output:** prompt instructs Claude to write `session-output.edn` with state keys

Code lives on disk naturally. Structured state (plans, test results, review verdicts) goes through `session-output.edn`. Contract validation runs against the merged result.

---

## Prompt Templates

Each primitive has a template in `resources/prompts/`. The runtime renders it with the contract's `:reads` keys from state.

```markdown
<!-- resources/prompts/implement.md -->
# Task
Implement the following plan. Write working code with tests.

## Goal
{{goal}}

## Plan
{{plan}}

{{#if reflection}}
## Previous Reflection
{{reflection}}
{{/if}}

{{#if review-feedback}}
## Review Feedback to Address
{{review-feedback}}
{{/if}}

## Instructions
- Write code files to the working directory
- Run tests and fix failures before finishing
- Write a session-output.edn file with:
  {:code {:files ["file1.go" ...] :language "go"}
   :test-results {:status :pass|:fail :failures [...]}}
```

Only `:reads` and `:optional-reads` keys are available to templates. Custom templates via `:prompt-template` metadata on primitives.

---

## Error Recovery

### Error Types

| Type | Trigger | Default Handler |
|------|---------|-----------------|
| `:transient` | Session crash, timeout, non-zero exit | Retry max 3 |
| `:malformed-output` | `session-output.edn` missing or schema mismatch | Re-run with hint, max 2 |
| `:contract-violation` | Wrong keys/types after re-run attempts exhausted | Snapshot + halt |
| `:unrecoverable` | Adapter can't spawn, working dir gone | Snapshot + halt |

### Handler DSL

```clojure
;; Agent-level
(on-error :transient        (retry {:max 3}))
(on-error :malformed-output (re-run {:hint "Write session-output.edn"}))
(on-error :contract-violation (snapshot-and-halt))

;; Per-step override
(plan goal
  (on-error :malformed-output
    (re-run {:hint "Output must be a vector of step maps." :max 3})))
```

**Resolution order:** per-step override, then agent-level handler, then global default.

**Re-run behavior:** amends the prompt with the contract violation message (not the full previous output). If re-runs exhaust, halts.

**Diagnostic snapshots on halt:**

```
runs/run-042/errors/
  error-v2.edn    # node, error type, contract expected vs actual
```

---

## Graph Registry & Inspection

`defagent` registers graph structure in a global atom at macroexpand time.

### REPL

```clojure
(graph/nodes :build-feature)        ;; list nodes
(graph/edges :build-feature)        ;; list edges
(graph/contracts :build-feature)    ;; show contract chain
(graph/validate :build-feature)     ;; re-run compile-time checks
(graph/viz :build-feature)          ;; ASCII graph

(runs/list)                         ;; list runs
(runs/state :run-042 3)             ;; state at version 3
(runs/log :run-042)                 ;; execution log
(runs/errors :run-042)              ;; error diagnostics
(runs/diff :run-041 :run-042)       ;; structural comparison
```

### CLI

```bash
golem-dsl run build-feature.clj --goal "Build CSV to JSON converter"
golem-dsl run build-feature.clj --state initial-state.edn
golem-dsl compile build-feature.clj
golem-dsl inspect build-feature
golem-dsl inspect run-042 --version 3
golem-dsl inspect run-042 --errors
golem-dsl diff run-041 run-042
golem-dsl resume run-042 --from v2
golem-dsl runs
```

### Node IDs

Auto-suffixed when a primitive appears multiple times: `:implement-1`, `:implement-2`.

---

## Execution Log

Append-only, written to `runs/run-NNN/log.edn`.

```clojure
[{:node-id :plan
  :primitive :plan
  :source-line 5
  :timestamp "2026-03-10T14:22:45Z"
  :duration-ms 12400
  :session {:adapter :claude-code :exit-code 0}
  :status :success
  :state-version 1
  :contract {:reads-provided [:goal]
             :writes-produced [:plan]}}]
```

Captures: node, timing, session metadata, status, state version, contract keys. Does not capture token usage (invisible to orchestrator). Raw session transcripts saved separately.

---

## Self-Modification

Agents can emit modified `.clj` files as output. A human reviews before using. No runtime graph mutation.

An `(optimize-agent)` primitive could analyze execution history and suggest improvements — but the output is a file, not a live change.

---

## Module Structure

```
golem-dsl/
  deps.edn
  src/golem/dsl/
    core.clj                      ;; defagent, defpred, defprimitive macros
    registry.clj                  ;; graph registry atom, query functions
    primitives/
      builtins.clj                ;; plan, implement, review, reflect, research
    predicates/
      builtins.clj                ;; failed?, needs-work?, coverage-low?
    engine/
      core.clj                    ;; graph walker, execution loop
      state.clj                   ;; immutable state management
      snapshot.clj                ;; versioned EDN persistence
      context.clj                 ;; prompt template rendering
      output.clj                  ;; session output + filesystem diff parsing
    session/
      protocol.clj                ;; SessionAdapter protocol
      claude.clj                  ;; Claude Code adapter (golem session)
    errors/
      types.clj                   ;; error type definitions
      handler.clj                 ;; on-error dispatch, resolution order
      retry.clj                   ;; retry/re-run strategies
      diagnostic.clj              ;; error snapshot capture
    cli/
      main.clj                    ;; run, compile, inspect, diff, resume, runs
  resources/prompts/
    plan.md
    implement.md
    review.md
    reflect.md
    research.md
  agents/
    build_feature.clj
    fix_bug.clj
    write_docs.clj
  test/golem/dsl/
    core_test.clj
    registry_test.clj
    engine_test.clj
    session_test.clj
    errors_test.clj
    integration_test.clj
```

---

## What Comes Later

Deferred until the DSL core is proven:

- **Memory system.** Task-scoped retrieval and extraction.
- **Parallel composition.** `(invoke a)` and `(invoke b)` concurrently via worktrees.
- **Cost optimization.** Per-primitive model routing.
- **Visualization dashboard.** Real-time execution graphs in golem-dash.
- **Dynamic agent planning.** Agent decides its own workflow at runtime.
