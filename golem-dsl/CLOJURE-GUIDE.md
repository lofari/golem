# Clojure for Go Developers: The Golem DSL Guide

You built the Go CLI. Now you need to understand and maintain the Clojure DSL that orchestrates agents. This guide maps Clojure concepts to Go equivalents, then walks through the actual code in this project.

**What You'll Learn:**
- Clojure syntax and semantics through a Go developer's lens
- How to read and understand the macros in `src/golem/dsl/core.clj`
- The architecture: registry → contracts → engine execution
- How to extend the DSL with new primitives and predicates

**Prerequisites:**
- You understand the Go CLI architecture (golem, runners, state)
- You can read basic function definitions

**Time:** 45 minutes. Code is real from this project.

---

## 1. Clojure in 5 Minutes for Go Developers

### The Big Picture Differences

**Go** is statically typed, compiled, imperative. **Clojure** is dynamically typed, interpreted, functional-first. But both are practical languages. Here's the mental model shift:

| Go Concept | Clojure Equivalent | Key Difference |
|---|---|---|
| `type struct { ... }` | `defrecord` or just `map` | Clojure uses maps (like maps in Go) everywhere |
| `func doThing(x int) int { return x + 1 }` | `(defn do-thing [x] (+ x 1))` | Functions are data; prefix notation (function first) |
| `if x > 0 { ... } else { ... }` | `(if (> x 0) ... ...)` | Everything is an expression; parentheses matter |
| `var state = map[string]interface{}{}` | `(atom {})` | Atoms hold mutable state (Go: pointers; Clojure: atoms) |
| `state["key"] = value` | `(swap! state assoc :key value)` | Immutable updates; keywords (`:key`) instead of strings |
| Package + exported functions | Namespace + public fns | `(ns my.namespace)` is like `package mypackage` |

### Functions and Basic Forms

```clojure
;; Function definition (Go: func add(x int, y int) int { return x + y })
(defn add [x y]
  (+ x y))

;; Anonymous function (Go: func(x int) int { return x * 2 })
(fn [x] (* x 2))

;; Call function: (function arg1 arg2 ...)
(add 2 3)        ; => 5
(map inc [1 2 3]) ; => (2 3 4)

;; Nested calls read inside-out (Go: outer(inner(x)))
(println (str "Value: " (+ 1 2)))  ; prints "Value: 3"
```

### Data Structures: Maps, Vectors, Keywords

```clojure
;; Map (like Go's map[string]interface{})
{:name "Alice" :age 30}

;; Vector (like Go's []interface{})
[1 2 3]

;; Keywords are like string constants. Start with `:`, very common
:goal :plan :code  ; Keywords used as keys in maps and for dispatch

;; Extract from map
(get {:name "Alice" :age 30} :name)  ; => "Alice"
(get-in {:user {:name "Alice"}} [:user :name]) ; => "Alice" (nested)

;; Build/update maps
(assoc {:name "Alice"} :age 30)  ; => {:name "Alice" :age 30}
(select-keys {:a 1 :b 2 :c 3} [:a :c]) ; => {:a 1 :c 3}
```

### Mutable State: Atoms

Go uses pointers and goroutines. Clojure uses **atoms** for shared mutable state.

```clojure
;; Create atom (like: var state = &map[string]interface{}{})
(def my-state (atom {}))

;; Read atom (like: *state)
@my-state  ; => current value

;; Update atom (like: state["key"] = value, but atomic)
;; swap! calls a function with current value, returns new value
(swap! my-state assoc :count 5)  ; => {:count 5}

;; In this project:
(defonce ^:private primitives (atom {}))  ; Define once (like sync.Once)
(swap! primitives assoc :plan {...})      ; Register a primitive
(get @primitives :plan)                   ; Read it back
```

`swap!` is like "read current value, apply function, write atomically." Perfect for a global registry.

### Destructuring: Unpack Maps and Vectors

Clojure lets you unpack nested structures inline. No equivalent in Go; closest is `struct` unpacking.

```clojure
;; Vector destructuring
(let [[a b c] [1 2 3]]
  (+ a b c))  ; => 6

;; Map destructuring (very useful!)
(let [{:keys [name age]} {:name "Alice" :age 30}]
  (println name age))  ; prints "Alice 30"

;; In function parameters:
(defn greet [{:keys [name age]}]
  (str name " is " age))
(greet {:name "Bob" :age 25})  ; => "Bob is 25"

;; In let bindings with maps:
(let [{{:keys [status]} :test-results} {:test-results {:status :pass}}]
  status)  ; => :pass
```

This saves a lot of `state["key"]` noise.

### Threading Macros: Reducing Nesting

The `->` and `->>` macros thread a value through a chain of functions, reducing nesting.

```clojure
;; Without threading (Go style):
(conj (conj (conj [] 1) 2) 3)  ; => [1 2 3]

;; With -> (thread value as first arg)
(-> []
    (conj 1)
    (conj 2)
    (conj 3))  ; => [1 2 3]

;; With ->> (thread value as last arg)
(->> [1 2 3 4]
     (filter even?)
     (map (fn [x] (* x 2))))  ; => (4 8)

;; cond-> (thread value, apply forms conditionally)
;; From src/golem/dsl/session/claude.clj:
(cond-> ["golem" "session" "--prompt" file]
  sandbox     (conj "--sandbox")
  model       (into ["--model" model])
  (false? mcp) (conj "--mcp=false"))
  ; builds command array conditionally
```

### Namespaces: Like Go Packages

```clojure
;; At top of file (like package golem/dsl)
(ns golem.dsl.core
  (:require [golem.dsl.registry :as registry]))

;; Call function from another namespace
(registry/register-primitive! :plan {...})

;; Refer (like import function names)
(defns [defagent]
  (:require [golem.dsl.core :refer [defagent]]))
```

### Macros: Code That Writes Code

This is the one true Clojure superpower. A macro takes code as data, transforms it, and returns new code.

```clojure
;; Simple macro definition
(defmacro unless [test & body]
  `(if (not ~test)
     (do ~@body)))

;; Usage:
(unless false (println "This prints"))

;; The backtick ` means "quote this code"
;; The ~ means "unquote and insert value here"
;; The ~@ means "unquote and splice"
```

In this project, **macros are the main feature**. `defprimitive` and `defagent` are macros that register code at **compile time**, not runtime.

---

## 2. Reading the DSL: Walking Through build_feature.clj

Let's read the real example agent line by line:

```clojure
(ns agents.build-feature
  (:require [golem.dsl.core :refer [defagent]]
            [golem.dsl.primitives.builtins]
            [golem.dsl.predicates.builtins]))

(defagent build-feature
  "Builds a feature from a goal description."
  {:initial-state [:goal]}

  (plan)
  (implement)
  (review)
  (while needs-work? {:max 3}
    (implement)
    (review))

  (on-error :transient        (retry {:max 3}))
  (on-error :malformed-output (re-run {:hint "Write session-output.edn with contract keys."}))
  (on-error :contract-violation (snapshot-and-halt)))
```

**Line 1-4: Namespace and imports**
- `(ns agents.build-feature)` — Define namespace (like `package agents.build_feature` in Go)
- `(:require [...])` — Import from other namespaces
- `[golem.dsl.core :refer [defagent]]` — Import `defagent` macro so we can use it as `(defagent ...)`

**Line 6-8: Start the macro invocation**
- `(defagent build-feature ...)` — Call the `defagent` macro with name `build-feature`
- `"Builds a feature..."` — Optional docstring
- `{:initial-state [:goal]}` — Metadata map: the agent starts with one piece of state, `:goal`

**Line 10-15: The agent steps**
- `(plan)` — Call the `plan` primitive (just the name, no args)
- `(implement)` — Call `implement` primitive
- `(review)` — Call `review` primitive
- `(while needs-work? {:max 3} ...)` — Loop: while predicate `needs-work?` is true, execute up to 3 times, run implement + review

**Line 17-19: Error handlers**
- `(on-error :transient (retry {:max 3}))` — If error type is `:transient`, retry up to 3 times
- `(on-error :malformed-output (re-run {:hint "..."}))` — If output malformed, re-run with hint
- `(on-error :contract-violation (snapshot-and-halt))` — If contract violated, save state and stop

**What happens when you evaluate this file?**

The `defagent` macro:
1. Parses the body (steps + error handlers)
2. Expands each step into a node in a graph
3. Builds edges (control flow, sequencing)
4. Validates contracts (does each step's `:reads` come from prior `:writes` or initial state?)
5. Registers the agent in the global registry
6. Returns nil (the side effect is the registration)

No code runs yet. The agent is a static graph, ready for the engine to execute.

---

## 3. How the Pieces Fit Together

### The Flow: From Macro to Execution

```
defagent macro call
  ↓
Parse body (steps, error-handlers)
  ↓
expand-steps (turn [plan implement review while...] into nodes + edges)
  ↓
validate-chain (contracts.clj: verify data flow)
  ↓
register-agent! (store in atoms in registry.clj)
  ↓
[Later, at runtime]
  ↓
engine/run (walks nodes, executes, manages state)
```

### Step 1: The Macro Invocation (src/golem/dsl/core.clj, line 158)

```clojure
(defmacro defagent
  "Define an agent program."
  [name & body]
  (let [{:keys [metadata steps error-handlers]} (parse-body body)
        initial-state (:initial-state metadata [])]
    `(let [nodes# (expand-steps '~steps)
           errors# (contracts/validate-chain nodes# ~initial-state)]
       (when errors#
         (throw (ex-info (str "Contract validation failed for agent " '~name)
                         {:agent '~name :errors errors#})))
       (registry/register-agent!
        ~(keyword name)
        {:name ~(keyword name)
         :metadata ~metadata
         :nodes nodes#
         :edges (build-edges '~steps)
         :error-handlers '~error-handlers}))))
```

**What's happening:**

1. `parse-body` — Split the body into `:metadata`, `:steps`, `:error-handlers`
   - Strips docstring (line 32-33)
   - Extracts metadata map (line 34)
   - Separates `on-error` handlers from steps (line 36-37)

2. `expand-steps` — Convert `[plan implement review while ...]` into nodes
   - Line 171: `nodes#` is a vector of `{:id :plan-1 :primitive :plan :contract {...}}`
   - Each node gets a unique ID based on position
   - Control flow (while, if, when) expands into multiple nodes

3. `contracts/validate-chain` — Check data dependency
   - Start with initial state `[:goal]`
   - For each node, check: does `:reads` exist in available keys?
   - After each node, add `:writes` to available keys
   - Return errors if any `:reads` missing

4. `register-agent!` — Store the graph in the global atoms
   - Name: `:build-feature`
   - Metadata, nodes, edges, error-handlers all stored
   - Ready for the engine to execute

### Step 2: Registry and Atoms (src/golem/dsl/registry.clj)

The global registry is three atoms:

```clojure
(defonce ^:private agents (atom {}))
(defonce ^:private primitives (atom {}))
(defonce ^:private predicates (atom {}))
```

`defonce` means "define this only once, even if the file is reloaded." Like Go's `sync.Once`.

Registration functions use `swap!` to update atomically:

```clojure
(defn register-agent! [name graph]
  (swap! agents assoc name graph))
  ;; equivalent to:
  ;; agents := map with agents[name] = graph
```

Retrieval is just dereferencing and looking up:

```clojure
(defn get-agent [name]
  (get @agents name))
  ;; equivalent to:
  ;; return agents[name]
```

### Step 3: Contracts (src/golem/dsl/contracts.clj)

The contract chain validator walks the nodes and checks data flow:

```clojure
(defn validate-chain [steps initial-state]
  (loop [remaining steps
         available (set initial-state)
         errors []]
    (if (empty? remaining)
      (when (seq errors) errors)
      (let [{:keys [id contract]} (first remaining)
            reads (:reads contract [])
            writes (:writes contract [])
            missing (remove available reads)]
        (recur (rest remaining)
               (into available writes)
               (into errors
                     (map (fn [k] {:node id :missing-key k :available available})
                          missing)))))))
```

This is a recursive loop (like Go's `for`, but functional):
- Start with initial state as available keys
- For each step:
  - Check if all `:reads` are available (no missing keys)
  - If missing, add to errors
  - Add this step's `:writes` to available for the next step
  - Recurse to next step

If any errors, throw and halt at macro-expansion time.

### Step 4: Engine Execution (src/golem/dsl/engine/core.clj)

At runtime, the engine walks the graph:

```clojure
(defn run [agent-name initial-state adapter working-dir]
  ;; Get the agent from registry
  ;; Walk nodes in order, executing each
  ;; For each node, check edges for conditionals
  ;; Manage state (immutable, contract-enforced)
  ;; Handle errors (retry, halt, etc.)
  )
```

The execution loop:
1. Fetch agent from registry
2. Loop through nodes
3. For each node:
   - Validate reads (state has required keys)
   - Execute (session or local)
   - Apply writes (merge output into state, enforce contract)
   - Find next node via edges (handle conditionals)
4. Handle errors (retry, halt, snapshot)
5. Return final state

---

## 4. Key Patterns in This Codebase

### Pattern 1: Atoms for Global Mutable State

Go developers use pointers and mutation. Clojure uses atoms (like a safe wrapper around mutable boxes).

```clojure
(defonce ^:private primitives (atom {}))

;; Register a primitive (from src/golem/dsl/core.clj, line 13)
(registry/register-primitive!
  ~(keyword name)
  {:name ~(keyword name)
   :doc ~docstring
   :contract ~contract
   :execute ~body})

;; Inside registry.clj:
(defn register-primitive! [name definition]
  (swap! primitives assoc name definition))
```

`swap!` is atomic (thread-safe). In Go terms:
```go
// Go equivalent (with mutex)
var mu sync.Mutex
primitives := make(map[string]interface{})

func registerPrimitive(name string, def interface{}) {
  mu.Lock()
  primitives[name] = def
  mu.Unlock()
}
```

In Clojure, `swap!` handles the locking for you.

### Pattern 2: Protocols for Interfaces

Clojure's protocols are like Go's interfaces.

```clojure
;; From src/golem/dsl/session/protocol.clj:
(defprotocol SessionAdapter
  (spawn [this prompt working-dir opts]
    "Launch a session. Returns an opaque handle.")
  (await-result [this handle timeout-ms]
    "Block until session completes. Returns {:exit-code N}")
  (read-output [this handle]
    "Read session output. Returns map of state keys."))
```

In Go:
```go
type SessionAdapter interface {
  Spawn(prompt, workingDir string, opts map[string]interface{}) interface{}
  AwaitResult(handle interface{}, timeoutMs int) map[string]interface{}
  ReadOutput(handle interface{}) map[string]interface{}
}
```

### Pattern 3: Defrecord for Types

A record is like a Go struct that implements protocols.

```clojure
;; From src/golem/dsl/session/claude.clj, line 24:
(defrecord ClaudeAdapter [golem-binary sandbox plugin-dirs max-turns model
                          mcp no-lsp]
  proto/SessionAdapter
  (spawn [this prompt working-dir opts]
    ;; implementation
    )
  (await-result [this handle timeout-ms]
    ;; implementation
    )
  (read-output [this handle]
    ;; implementation
    ))
```

In Go:
```go
type ClaudeAdapter struct {
  GolemBinary string
  Sandbox     bool
  PluginDirs  []string
  MaxTurns    int
  Model       string
  MCP         bool
  NoLSP       bool
}

func (ca *ClaudeAdapter) Spawn(prompt, workingDir string, opts map[string]interface{}) interface{} {
  // implementation
}
```

### Pattern 4: Destructuring in Let Bindings

```clojure
;; From src/golem/dsl/core.clj, line 32-40:
(let [;; Skip optional docstring
      body (if (string? (first body)) (rest body) body)
      metadata (when (map? (first body)) (first body))
      rest-body (if metadata (rest body) body)
      {error-handlers true steps false}
      (group-by #(and (seq? %) (= 'on-error (first %))) rest-body)]
  {:metadata (or metadata {})
   :steps (vec steps)
   :error-handlers (vec error-handlers)})
```

The line `{error-handlers true steps false}` destructures a grouped map:
- `group-by` returns `{true [...errors...] false [...steps...]}`
- Unpack into `:error-handlers` and `:steps` immediately
- Very convenient for parsing

### Pattern 5: Loop/Recur for Iteration

Clojure doesn't have loops like Go. Instead, use `loop/recur` (or `reduce`).

```clojure
;; From src/golem/dsl/contracts.clj:
(loop [remaining steps
       available (set initial-state)
       errors []]
  (if (empty? remaining)
    (when (seq errors) errors)
    (let [{:keys [id contract]} (first remaining)
          reads (:reads contract [])
          writes (:writes contract [])
          missing (remove available reads)]
      (recur (rest remaining)
             (into available writes)
             (into errors
                   (map (fn [k] {...}) missing))))))
```

- `loop` binds initial values: `remaining`, `available`, `errors`
- `recur` jumps back to the top with new values
- It's tail-recursive and compiles to a tight loop (no stack overflow)

Equivalent Go:
```go
var remaining []Node = steps
available := makeSet(initialState)
var errors []Error

for {
  if len(remaining) == 0 {
    break
  }
  // process first(remaining)
  remaining = rest(remaining)
  available = union(available, writes)
  errors = append(errors, ...)
}
```

### Pattern 6: Cond-> Threading Macro

From src/golem/dsl/session/claude.clj (line 14):

```clojure
(cond-> ["golem" "session" "--prompt" prompt-file ...]
  sandbox     (conj "--sandbox")
  model       (into ["--model" model])
  (false? mcp) (conj "--mcp=false")
  no-lsp      (conj "--no-lsp")
  plugin-dirs (into (mapcat #(vector "--plugin-dir" %) plugin-dirs)))
```

`cond->` starts with a value (the vector), then:
- If `sandbox` is truthy, call `(conj ... "--sandbox")`
- If `model` is truthy, call `(into ... ["--model" model])`
- And so on

Equivalent Go:
```go
cmd := []string{"golem", "session", "--prompt", promptFile}

if sandbox {
  cmd = append(cmd, "--sandbox")
}
if model != "" {
  cmd = append(cmd, "--model", model)
}
if !mcp {
  cmd = append(cmd, "--mcp=false")
}
// etc.
```

The Clojure version is more data-driven and scalable.

---

## 5. Extending the DSL

### Adding a New Primitive

A primitive is a unit of work. It has a contract (reads, writes, session flag) and execute function.

**Example: Add a new primitive `summarize` that doesn't need a session**

**File: src/golem/dsl/primitives/builtins.clj**

```clojure
(defprimitive summarize
  "Summarize code review feedback into key points."
  {:reads [:review-feedback]
   :writes [:summary]
   :session false}
  (fn [state context adapter]
    (let [feedback (:review-feedback state)
          key-points (extract-key-points feedback)]
      {:summary key-points})))

(defn- extract-key-points [feedback]
  ;; Local processing logic
  (mapv :key-point feedback))
```

**Then use it in an agent:**

```clojure
(defagent improve-code
  {:initial-state [:goal]}
  (implement)
  (review)
  (summarize)  ; Use the new primitive
  (implement))
```

**How it works:**
1. `defprimitive` macro registers `:summarize` in the global registry
2. When you call `(summarize)` in an agent, `defagent` finds it in the registry
3. At runtime, the engine calls the execute fn with state
4. The fn returns `{:summary [...]}`
5. Engine validates `:writes` (only `:summary` key allowed)
6. Engine merges into state

**Contract validation:**
- `:reviews-feedback` must exist (from prior `review` step)
- `:summary` is added to available keys for next steps

### Adding a New Predicate

Predicates are boolean tests over state, used in control flow.

**Example: Add `has-critical-issues?`**

**File: src/golem/dsl/predicates/builtins.clj**

```clojure
(defpred has-critical-issues?
  (some (fn [issue] (= :critical (:severity issue)))
        (get-in state [:review-feedback :issues] [])))
```

**Usage in an agent:**

```clojure
(defagent improve-code
  {:initial-state [:goal]}
  (implement)
  (review)
  (if has-critical-issues?
    (implement)  ; Loop on critical issues
    (finalize))  ; Move on if all good
)
```

**How it works:**
1. `defpred` registers `:has-critical-issues?` in the registry
2. When you use `(if has-critical-issues? ...)` in an agent, the macro stores the predicate reference in edges
3. At runtime, the engine evaluates the predicate by calling it with current state
4. Evaluates to true/false; engine follows the matching edge

### Adding a New Control Flow Construct

Control flow constructs (while, if, when) are handled in the `defagent` macro.

**Example: Add a `repeat` construct that runs N times without a predicate**

**In src/golem/dsl/core.clj, `expand-steps-inner` function:**

Add a new case in the `case op`:

```clojure
;; (repeat 3 step1 step2)
repeat
(let [times (second step)
      body-forms (drop 2 step)
      body-result (expand-steps-inner (vec body-forms) counter)
      body-nodes (:nodes body-result)
      body-edges (:edges body-result)
      prev-node (last nodes)
      first-body (first body-nodes)
      last-body (last body-nodes)]
  {:nodes (into nodes body-nodes)
   :edges (-> edges
              (into body-edges)
              ;; Entry edge from prev to loop
              (conj {:from (:id prev-node)
                     :to (:id first-body)})
              ;; Loop-back edge (limit to `times` iterations)
              (conj {:from (:id last-body)
                     :to (:id first-body)
                     :max times}))})
```

Then update `control-flow?` to recognize it:

```clojure
(defn- control-flow?
  "Check if a step form is a control flow expression."
  [form]
  (and (seq? form) (#{'while 'if 'when 'repeat} (first form))))
```

**Usage:**

```clojure
(defagent demo
  {:initial-state [:goal]}
  (plan)
  (repeat 3
    (implement)
    (test-code))
)
```

The engine will execute implement/test-code 3 times in a loop.

### Testing Your Extension

Always add tests:

**File: test/golem/dsl/primitives_test.clj**

```clojure
(deftest summarize-primitive-registers
  (defprimitive summarize
    "..."
    {:reads [:review-feedback] :writes [:summary] :session false}
    (fn [state context adapter] {:summary []}))

  (let [p (registry/get-primitive :summarize)]
    (is (some? p))
    (is (= [:review-feedback] (get-in p [:contract :reads])))
    (is (= [:summary] (get-in p [:contract :writes])))))

(deftest summarize-in-agent-validates-contract
  (defprimitive summarize ... (fn ...))

  (let [graph (defagent test-agent
                {:initial-state [:goal]}
                (review)
                (summarize))]
    (is (some? graph))
    ;; Contract should pass: review writes :review-feedback,
    ;; summarize reads it
    ))
```

---

## 6. Quick Reference: Clojure Forms Used in This Project

### Basic Forms

| Form | Usage | Go Equivalent |
|------|-------|---|
| `(defn name [args] body)` | Define function | `func name(args) { body }` |
| `(defmacro name [args] body)` | Define macro | Code generation |
| `(defrecord Name [fields] Protocol (methods))` | Define type impl protocol | `type Name struct {...}` + methods |
| `(defonce ^:private atom-name (atom {}))` | Global state, define-once | `var (once sync.Once); var data = ...` |
| `(atom value)` | Create mutable box | `&value` (pointer) |
| `(swap! atom fn)` | Update atom atomically | Mutex-protected update |
| `@atom` | Read atom | `*pointer` (dereference) |

### Control Flow

| Form | Usage | Go Equivalent |
|------|-------|---|
| `(if test true-val false-val)` | Conditional | `if test { ... } else { ... }` |
| `(when test body...)` | Conditional (no else) | `if test { ... }` |
| `(cond c1 v1 c2 v2 :else default)` | Multi-branch | `switch` or chained if/else |
| `(loop [a init] body (recur new-a))` | Tail-recursive loop | `for { ... }` |
| `(reduce fn init coll)` | Fold over collection | `for _, x := range coll { ... }` |

### Data Operations

| Form | Usage | Go Equivalent |
|------|-------|---|
| `(get map :key)` | Lookup | `map[:key]` |
| `(get-in map [:a :b])` | Nested lookup | `map[a][b]` |
| `(assoc map :k v)` | Add/update key | `map[:k] = v` |
| `(select-keys map [:a :b])` | Subset of keys | `{a: map[a], b: map[b]}` |
| `(into coll items)` | Merge/extend | `append(...); maps.Copy(...)` |
| `(conj coll item)` | Add to collection | `append(coll, item)` |
| `(vec coll)` | Convert to vector | `[]T{...}` |
| `(remove pred coll)` | Filter out | `for _, x := range ... if !pred(x)` |

### Destructuring (Clojure-specific)

| Form | Usage |
|------|-------|
| `(let [[a b] [1 2]] body)` | Unpack vector |
| `(let [{:keys [x y]} map] body)` | Unpack map keys |
| `(let [{:keys [x y]} {:x 1 :y 2 :z 3}] body)` | Pick specific keys from map |
| `(let [{{:keys [s]} :test-results} state] body)` | Nested unpacking |

### Threading Macros

| Form | Usage | Example |
|------|-------|---------|
| `(-> val (f1) (f2) (f3))` | Thread as first arg | `f3(f2(f1(val)))` |
| `(->> val (f1) (f2) (f3))` | Thread as last arg | `f3(f2(f1(val)))` with different semantics |
| `(cond-> val c1 (f1) c2 (f2))` | Thread conditionally | Apply f1 if c1, apply f2 if c2 |

### Quoting and Macros

| Form | Usage | Meaning |
|------|-------|---------|
| `` ` `` | Backtick | Quote the next form (treat as data) |
| `~` | Tilde | Unquote (insert value into quoted form) |
| `~@` | Splice-unquote | Insert and splice collection into quoted form |
| `'` | Single quote | Literal quote (don't evaluate) |

### Functions/Collections

| Form | Usage | Go Equivalent |
|------|-------|---|
| `(map fn coll)` | Transform each | `for _, x := range coll { ... }` with collect |
| `(filter pred coll)` | Keep if pred true | `for _, x := range coll if pred(x)` |
| `(first coll)` | First element | `coll[0]` |
| `(rest coll)` | All but first | `coll[1:]` |
| `(count coll)` | Length | `len(coll)` |
| `(empty? coll)` | Is empty | `len(coll) == 0` |
| `(seq coll)` | Convert to seq (or nil) | Similar to `for _, x := range` |
| `(some pred coll)` | Find first matching | `for _, x := range if pred(x) return x` |

### Protocols and Types

| Form | Usage | Go Equivalent |
|------|-------|---|
| `(defprotocol Name (method [args]))` | Define interface | `type Name interface { Method(...) }` |
| `(defrecord Name [fields] Protocol impl)` | Type + impl | `type Name struct + methods` |
| `(map->RecordName {:field val})` | Create record from map | Struct literal `Name{Field: val}` |

---

## 7. Debugging Tips for Go Developers

### Read Macros as They Expand

When confused, ask: "What code does this macro generate?"

Use `macroexpand-1` to see one level of expansion:

```clojure
;; In a REPL:
(require 'golem.dsl.core)

(macroexpand-1 '(defprimitive plan
                  "..."
                  {:reads [:goal] :writes [:plan] :session true}
                  (fn [s c a] {:plan []})))

;; Expands to:
(registry/register-primitive!
  :plan
  {:name :plan
   :doc "..."
   :contract {:reads [:goal] :writes [:plan] :session true}
   :execute (fn [s c a] {:plan []})})
```

Now you see: it's just a function call to `registry/register-primitive!`. The macro is a wrapper that registers code.

### Understand Data Flow With get-in

When debugging state flow, use `get-in` to navigate nested maps:

```clojure
;; State is:
{:goal "Build feature X"
 :plan [{:step 1 :desc "..."} ...]
 :code {...}
 :test-results {:status :pass :failures []}}

;; Check what's available:
(get-in state [:goal])  ; => "Build feature X"
(get-in state [:test-results :status])  ; => :pass

;; In contracts, this is how you validate:
(:reads contract [])  ; What keys must be present
```

### Trace Atoms

Atoms are mutable, but you can spy on them:

```clojure
;; Current registry state:
@registry/primitives   ; Map of all registered primitives
@registry/predicates   ; Map of all predicates
@registry/agents       ; Map of all agents

;; Print for debugging:
(clojure.pprint/pprint @registry/primitives)
```

In tests, `registry/reset-all!` clears them.

### Read Error Messages

Contract validation errors look like:

```
Contract validation failed for agent build-feature
{:agent build-feature
 :errors [{:node :review-1 :missing-key :code :available #{:goal :plan}}]}
```

This says: node `:review-1` tries to read `:code`, but available keys are only `{:goal :plan}`. Prior step didn't write `:code`.

### Key Gotchas

| Gotcha | Explanation | Fix |
|--------|-------------|-----|
| Keyword vs string | Keywords are `:key`, strings are `"key"`. Maps use keywords. | Use keywords in contracts and state |
| Macro vs function | Macros run at compile time, functions at runtime | If you need runtime dispatch, use a function not a macro |
| Atoms are mutable | `(atom {})` is mutable; changing it affects all readers | Use `swap!` atomically; don't mutate inner structures |
| Immutable by default | Maps/vectors are immutable; `assoc` returns new map | This is a feature: no side effects |
| seq? vs vector | `seq?` checks if form is a list; `vector?` checks if vector | In macros, `(seq? '(plan))` is true, but `(vector? '[plan])` is also true |

---

## 8. Next Steps

Now that you understand the architecture:

1. **Run the tests** to see real examples:
   ```bash
   cd /home/winler/projects/golem/.worktrees/agent-dsl/golem-dsl
   clj -M:test
   ```

2. **Read test files** for concrete usage:
   - `test/golem/dsl/core_test.clj` — How macros are tested
   - `test/golem/dsl/contracts_test.clj` — Contract validation examples
   - `test/golem/dsl/integration_test.clj` — Full agent execution flow

3. **Modify the example agent**:
   - Edit `agents/build_feature.clj`
   - Change step order, add new primitives, modify control flow
   - Run the CLI to see it parse and validate

4. **Trace a full execution**:
   - Look at `src/golem/dsl/engine/core.clj` for the `run` function
   - See how it walks the graph, manages state, handles edges

5. **Add a new primitive**:
   - Follow the pattern in section 5
   - Write a test
   - Use it in an agent

---

## Appendix: File Structure

```
src/golem/dsl/
  core.clj           <- Macros: defprimitive, defpred, defagent
  registry.clj       <- Global atoms + lookup
  contracts.clj      <- Contract validation

  primitives/
    builtins.clj     <- plan, implement, review, etc.

  predicates/
    builtins.clj     <- failed?, needs-work?

  session/
    protocol.clj     <- SessionAdapter interface
    claude.clj       <- ClaudeAdapter implementation

  engine/
    core.clj         <- Execution engine (run function)
    state.clj        <- Immutable state management
    context.clj      <- Prompt rendering
    snapshot.clj     <- State snapshots
    output.clj       <- File collection

test/golem/dsl/
  core_test.clj
  contracts_test.clj
  integration_test.clj
  ... (more tests)

agents/
  build_feature.clj  <- Example agent
  fix_bug.clj
  write_docs.clj
```

---

**You can now read, understand, and extend this Clojure DSL.** The key insight: Clojure's macros and atoms give you a declarative way to define agent graphs that are validated at compile time and executed safely at runtime. No external dependencies, just pure Clojure and the patterns Go developers already understand.

Good luck, and enjoy the functional paradigm shift!
