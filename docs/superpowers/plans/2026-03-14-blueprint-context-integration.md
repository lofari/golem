# Blueprint Context Integration — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the legacy context management system (MCP server, knowledge graph sync, execution collector, project-level state) into the blueprint engine so that blueprint runs have the same rich context as the legacy builder loop.

**Architecture:** Add three new fields to `EngineConfig` (`MCPEnabled`, `LSPEnabled`, `GraphPath`). At the start of `engine.Run()`, conditionally: (1) write MCP config and set it on `ClaudeRunner`, (2) sync the knowledge graph + embeddings, (3) wire execution collector callbacks, (4) inject project-level decisions/pitfalls from `state.yaml` into initial pipeline state. CLI commands pass the new config fields from resolved config.

**Tech Stack:** Go, stdlib testing, existing `internal/graph`, `internal/ctx`, `internal/runner` packages.

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/runner/engine.go` | Modify | Add `MCPEnabled`/`LSPEnabled`/`GraphPath` to `EngineConfig`. Add `setupContext()` method called at start of `Run()`. |
| `internal/runner/engine_context.go` | Create | Contains `setupMCP()`, `syncGraph()`, `setupCollector()`, `injectProjectContext()` — extracted from builder.go patterns. |
| `internal/runner/engine_context_test.go` | Create | Tests for each context integration function. |
| `internal/runner/engine_test.go` | Modify | Add integration test for engine with MCP/graph/project-context. |
| `cmd/code.go` | Modify | Pass `MCPEnabled`, `LSPEnabled` from resolved config to `EngineConfig`. |
| `cmd/run.go` | Modify | Pass `MCPEnabled`, `LSPEnabled` from resolved config to `EngineConfig`. |
| `cmd/helpers.go` | Modify | Move `--no-lsp` flag into `addAgentFlags` so all agent commands support it. |

---

## Chunk 1: EngineConfig Fields and Project Context Injection

### Task 1: Add new EngineConfig fields

**Files:**
- Modify: `internal/runner/engine.go:21-31`

- [ ] **Step 1: Add fields to EngineConfig**

In `internal/runner/engine.go`, add three fields to the `EngineConfig` struct:

```go
type EngineConfig struct {
	Dir       string
	AgentName string
	Goal      string
	Blueprint *Blueprint
	Config    map[string]any
	Runner    CommandRunner
	Model     string
	Events    chan<- EngineEvent
	Verbose   bool
	// Context integration
	MCPEnabled bool
	LSPEnabled bool
	GraphPath  string // defaults to ".ctx/graph.db" if empty
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: clean build, no errors.

- [ ] **Step 3: Run existing tests**

Run: `go test ./internal/runner/ -count=1`
Expected: all existing tests pass (new fields are zero-valued, no behavior change).

- [ ] **Step 4: Commit**

```bash
git add internal/runner/engine.go
git commit -m "feat(runner): add MCPEnabled, LSPEnabled, GraphPath to EngineConfig"
```

---

### Task 2: Inject project context from state.yaml

**Files:**
- Create: `internal/runner/engine_context.go`
- Create: `internal/runner/engine_context_test.go`

- [ ] **Step 1: Write the test for project context injection**

Create `internal/runner/engine_context_test.go`:

```go
package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInjectProjectContext(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	stateYAML := `project:
  name: testproj
  summary: a test project
  stack: go
decisions:
  - what: use Go
    why: fast
    when: "2026-03-01"
  - what: use SQLite
    why: simple
    when: "2026-03-02"
pitfalls:
  - what: watch for nil maps
    fix: always initialize
  - watch for race conditions
tasks: []
`
	os.WriteFile(filepath.Join(dir, ".ctx", "state.yaml"), []byte(stateYAML), 0644)

	state := map[string]any{"goal": "test"}
	injectProjectContext(dir, state)

	pc, ok := state["project-context"].(string)
	if !ok || pc == "" {
		t.Fatal("project-context should be a non-empty string in state")
	}
	// Should mention both decisions and pitfalls
	if !containsStr2(pc, "use Go") {
		t.Error("project-context should contain decision 'use Go'")
	}
	if !containsStr2(pc, "nil maps") {
		t.Error("project-context should contain pitfall about nil maps")
	}
}

func TestInjectProjectContext_NoStateFile(t *testing.T) {
	dir := t.TempDir()
	state := map[string]any{"goal": "test"}
	injectProjectContext(dir, state)

	if _, ok := state["project-context"]; ok {
		t.Error("project-context should not be set when state.yaml doesn't exist")
	}
}

func TestInjectProjectContext_EmptyDecisionsAndPitfalls(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	stateYAML := `project:
  name: testproj
  summary: a test project
  stack: go
decisions: []
pitfalls: []
tasks: []
`
	os.WriteFile(filepath.Join(dir, ".ctx", "state.yaml"), []byte(stateYAML), 0644)

	state := map[string]any{"goal": "test"}
	injectProjectContext(dir, state)

	if _, ok := state["project-context"]; ok {
		t.Error("project-context should not be set when decisions and pitfalls are empty")
	}
}

// containsStr2 checks if haystack contains needle.
func containsStr2(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/runner/ -run TestInjectProjectContext -count=1 -v`
Expected: FAIL — `injectProjectContext` undefined.

- [ ] **Step 3: Implement injectProjectContext**

Create `internal/runner/engine_context.go`:

```go
package runner

import (
	"fmt"
	"strings"

	golemctx "github.com/lofari/golem/internal/ctx"
)

// injectProjectContext reads decisions and pitfalls from state.yaml and
// injects them into the pipeline state as the "project-context" key.
// Steps can access this via optional-reads: [project-context] and ${project-context}.
func injectProjectContext(dir string, state map[string]any) {
	s, err := golemctx.ReadState(dir)
	if err != nil {
		return
	}
	if len(s.Decisions) == 0 && len(s.Pitfalls) == 0 {
		return
	}

	var b strings.Builder
	if len(s.Decisions) > 0 {
		b.WriteString("## Project Decisions\n\n")
		for _, d := range s.Decisions {
			b.WriteString(fmt.Sprintf("- **%s** — %s (%s)\n", d.What, d.Why, d.When))
		}
		b.WriteString("\n")
	}
	if len(s.Pitfalls) > 0 {
		b.WriteString("## Known Pitfalls\n\n")
		for _, p := range s.Pitfalls {
			b.WriteString(fmt.Sprintf("- %s\n", p.String()))
		}
		b.WriteString("\n")
	}

	state["project-context"] = b.String()
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/runner/ -run TestInjectProjectContext -count=1 -v`
Expected: PASS (all three test cases).

- [ ] **Step 5: Run full test suite**

Run: `go test ./internal/runner/ -count=1`
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/runner/engine_context.go internal/runner/engine_context_test.go
git commit -m "feat(runner): inject project-context from state.yaml into engine state"
```

---

### Task 3: Call injectProjectContext from Engine.Run()

**Files:**
- Modify: `internal/runner/engine.go:84-121`

- [ ] **Step 1: Write test for project context in engine run**

Add to `internal/runner/engine_test.go`:

```go
func TestEngine_ProjectContextInjection(t *testing.T) {
	dir := setupGitRepo(t)
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	stateYAML := `project:
  name: testproj
  summary: test
  stack: go
decisions:
  - what: use Go
    why: fast
    when: "2026-03-01"
pitfalls:
  - watch out for X
tasks: []
`
	os.WriteFile(filepath.Join(dir, ".ctx", "state.yaml"), []byte(stateYAML), 0644)

	step := &Step{
		Name: "implement", Type: StepTypeAgentic,
		Reads: []string{"goal"}, OptionalReads: []string{"project-context"},
		Writes: []string{"code"}, Prompt: "Goal: ${goal}\nContext: ${project-context}",
	}
	bp := &Blueprint{
		Name: "test", InitialState: []string{"goal"}, Config: map[string]any{},
		Errors: ErrorHandlers{ContractViolation: ErrorHandler{Action: "halt"}},
	}
	bp.pipeline = &Pipeline{
		Nodes:    []PipelineNode{{Step: step}},
		StepDefs: map[string]*Step{},
	}

	mock := &smartMockRunner{
		responses: func(step string, callNum int) MockResponse {
			return MockResponse{SessionOutput: map[string]any{"code": "done"}}
		},
	}

	e := NewEngine(EngineConfig{
		Dir: dir, AgentName: "test", Goal: "Test goal",
		Blueprint: bp, Config: map[string]any{}, Runner: mock, Model: "test",
	})

	_, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("engine error: %v", err)
	}

	// Verify project-context was injected into state
	state := e.State()
	pc, ok := state["project-context"].(string)
	if !ok {
		t.Fatal("project-context should be in engine state")
	}
	if !strings.Contains(pc, "use Go") {
		t.Error("project-context should contain decision 'use Go'")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/runner/ -run TestEngine_ProjectContextInjection -count=1 -v`
Expected: FAIL — project-context not in state (not yet called).

- [ ] **Step 3: Wire injectProjectContext into Engine.Run()**

In `internal/runner/engine.go`, in the `Run()` method, add the call right after creating the run directory and before emitting `pipeline-start`:

```go
func (e *Engine) Run(ctx context.Context) (map[string]any, error) {
	e.runDir = filepath.Join(e.cfg.Dir, ".ctx", "runs", e.RunID)
	if err := os.MkdirAll(filepath.Join(e.runDir, "sessions"), 0755); err != nil {
		return nil, fmt.Errorf("create run dir: %w", err)
	}

	// ... symlink code stays the same ...

	// ... log file creation stays the same ...

	// Context integration
	injectProjectContext(e.cfg.Dir, e.state)

	e.saveState()
	e.emit(EngineEvent{Type: "pipeline-start", Agent: e.cfg.AgentName, Goal: e.cfg.Goal, RunID: e.RunID})
	// ... rest of Run() stays the same ...
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/runner/ -run TestEngine_ProjectContextInjection -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Run full test suite**

Run: `go test ./internal/runner/ -count=1`
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/runner/engine.go internal/runner/engine_test.go
git commit -m "feat(runner): wire project-context injection into engine.Run()"
```

---

## Chunk 2: MCP Server Wiring

### Task 4: Wire MCP config into engine

**Files:**
- Modify: `internal/runner/engine_context.go`
- Modify: `internal/runner/engine_context_test.go`
- Modify: `internal/runner/engine.go`

- [ ] **Step 1: Write the test for setupMCP**

Add to `internal/runner/engine_context_test.go`:

```go
func TestSetupMCP(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	cr := &ClaudeRunner{}
	err := setupMCP(dir, cr, true) // LSP enabled
	if err != nil {
		t.Fatalf("setupMCP error: %v", err)
	}

	// MCP config file should exist
	configPath := filepath.Join(dir, ".ctx", "mcp_servers.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("mcp_servers.json should be created")
	}

	// ClaudeRunner should have MCPConfig set
	if cr.MCPConfig == "" {
		t.Error("ClaudeRunner.MCPConfig should be set")
	}
	if cr.MCPConfig != configPath {
		t.Errorf("MCPConfig = %q, want %q", cr.MCPConfig, configPath)
	}
}

func TestSetupMCP_NoLSP(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	cr := &ClaudeRunner{}
	err := setupMCP(dir, cr, false) // LSP disabled
	if err != nil {
		t.Fatalf("setupMCP error: %v", err)
	}

	// Read the config and verify --no-lsp is present
	data, _ := os.ReadFile(filepath.Join(dir, ".ctx", "mcp_servers.json"))
	if !strings.Contains(string(data), "no-lsp") {
		t.Error("MCP config should include --no-lsp flag when LSP disabled")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/runner/ -run TestSetupMCP -count=1 -v`
Expected: FAIL — `setupMCP` undefined.

- [ ] **Step 3: Implement setupMCP**

Add to `internal/runner/engine_context.go`:

```go
// setupMCP writes the MCP config file and configures the runner.
// lspEnabled controls whether the MCP server starts LSP servers.
func setupMCP(dir string, cr *ClaudeRunner, lspEnabled bool) error {
	mcpPath, err := WriteMCPConfig(dir, !lspEnabled)
	if err != nil {
		return err
	}
	cr.MCPConfig = mcpPath
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/runner/ -run TestSetupMCP -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Wire setupMCP into Engine.Run()**

In `internal/runner/engine.go`, in `Run()`, after the `injectProjectContext` call:

```go
	// Context integration
	injectProjectContext(e.cfg.Dir, e.state)

	if e.cfg.MCPEnabled {
		if cr, ok := e.cfg.Runner.(*ClaudeRunner); ok {
			if err := setupMCP(e.cfg.Dir, cr, e.cfg.LSPEnabled); err != nil {
				log.Printf("golem: warning: MCP setup failed: %v", err)
			}
		}
	}
```

- [ ] **Step 6: Run full test suite**

Run: `go test ./internal/runner/ -count=1`
Expected: all tests pass. Existing tests use mock runners (not `*ClaudeRunner`), so the MCP branch is safely skipped.

- [ ] **Step 7: Commit**

```bash
git add internal/runner/engine.go internal/runner/engine_context.go internal/runner/engine_context_test.go
git commit -m "feat(runner): wire MCP server config into blueprint engine"
```

---

### Task 5: Pass MCPEnabled/LSPEnabled from CLI to EngineConfig

**Files:**
- Modify: `cmd/code.go:55-65`
- Modify: `cmd/run.go:59-69`

- [ ] **Step 1: Update golem code blueprint path**

In `cmd/code.go`, in the blueprint engine branch, add `MCPEnabled` and `LSPEnabled` to the `EngineConfig` construction:

```go
			e := runner.NewEngine(runner.EngineConfig{
				Dir:        dir,
				AgentName:  agentName,
				Goal:       rc.Goal,
				Blueprint:  bp,
				Config:     mergedConfig,
				Runner:     cr,
				Model:      rc.Model,
				Events:     events,
				Verbose:    rc.Verbose,
				MCPEnabled: rc.MCP,
				LSPEnabled: rc.LSP,
			})
```

- [ ] **Step 2: Update golem run**

In `cmd/run.go`, same change to the `EngineConfig` construction:

```go
		e := runner.NewEngine(runner.EngineConfig{
			Dir:        dir,
			AgentName:  agentName,
			Goal:       goal,
			Blueprint:  bp,
			Config:     mergedConfig,
			Runner:     cr,
			Model:      rc.Model,
			Events:     events,
			Verbose:    rc.Verbose,
			MCPEnabled: rc.MCP,
			LSPEnabled: rc.LSP,
		})
```

- [ ] **Step 3: Move --no-lsp flag into addAgentFlags**

The `--no-lsp` flag is currently only registered on `codeCmd` in `cmd/code.go:170`, but `resolveConfig` in `cmd/helpers.go` checks for it on any command. Move the flag registration into `addAgentFlags` in `cmd/helpers.go` so both `golem code` and `golem run` support it:

In `cmd/helpers.go`, add to `addAgentFlags`:
```go
	cmd.Flags().Bool("no-lsp", false, "disable LSP servers during sessions")
```

Then remove the duplicate from `cmd/code.go`'s `init()`:
```go
// Remove this line from code.go init():
// codeCmd.Flags().Bool("no-lsp", false, "disable LSP servers during sessions")
```

Also add `cmd/helpers.go` to the commit in Step 6.

- [ ] **Step 4: Verify build**

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 5: Run full test suite**

Run: `go test ./... -count=1`
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add cmd/code.go cmd/run.go cmd/helpers.go
git commit -m "feat(cmd): pass MCPEnabled/LSPEnabled to blueprint EngineConfig, move --no-lsp to addAgentFlags"
```

---

## Chunk 3: Graph Sync and Execution Collector

### Task 6: Graph sync at engine start

**Files:**
- Modify: `internal/runner/engine_context.go`
- Modify: `internal/runner/engine_context_test.go`
- Modify: `internal/runner/engine.go`

- [ ] **Step 1: Write the test for syncGraph**

Add to `internal/runner/engine_context_test.go`:

```go
func TestSyncGraph_NoGraphDB(t *testing.T) {
	dir := t.TempDir()
	// No graph.db — should be a no-op, no error
	err := syncGraph(dir, "")
	if err != nil {
		t.Errorf("syncGraph with no graph.db should not error, got: %v", err)
	}
}

func TestSyncGraph_ExplicitPathMissing(t *testing.T) {
	dir := t.TempDir()
	err := syncGraph(dir, filepath.Join(dir, "nonexistent.db"))
	if err != nil {
		t.Errorf("syncGraph with missing explicit path should not error, got: %v", err)
	}
}
```

Note: Testing with a real graph.db requires the `graph` package to be available. The pattern in `builder.go` shows the graph sync is best-effort — failures are logged but don't halt execution. We test the "not found" paths here; the happy path with a real graph is an integration test that requires a git repo with source files, which is tested by the existing graph package tests.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/runner/ -run TestSyncGraph -count=1 -v`
Expected: FAIL — `syncGraph` undefined.

- [ ] **Step 3: Implement syncGraph**

Add to `internal/runner/engine_context.go`:

```go
import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	golemctx "github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/graph/embed"
)

// syncGraph updates the knowledge graph from current source files.
// If graphPath is empty, defaults to .ctx/graph.db.
// If the graph doesn't exist, this is a no-op.
func syncGraph(dir, graphPath string) error {
	if graphPath == "" {
		graphPath = filepath.Join(dir, ".ctx", "graph.db")
	}
	if _, err := os.Stat(graphPath); os.IsNotExist(err) {
		return nil
	}

	store, err := graph.OpenStore(graphPath)
	if err != nil {
		log.Printf("golem: warning: could not open graph: %v", err)
		return nil
	}
	defer store.Close()

	builder := graph.NewBuilder(store)
	if err := builder.Sync(dir); err != nil {
		log.Printf("golem: warning: graph sync failed: %v", err)
		return nil
	}
	fmt.Fprintf(os.Stderr, "golem: graph synced\n")

	// Incremental embed if embeddings exist
	eCount, _ := store.EmbeddingCount()
	if eCount > 0 {
		modelDir, mErr := embed.EnsureModel(embed.DefaultModelID, embed.DefaultModelDir())
		if mErr != nil {
			return nil
		}
		embedder, oErr := embed.NewONNXEmbedder(modelDir)
		if oErr != nil {
			return nil
		}
		defer embedder.Close()

		p := embed.NewPipeline(store, embedder)
		if _, eErr := p.EmbedAll(context.Background()); eErr != nil {
			log.Printf("golem: warning: embed sync failed: %v", eErr)
		} else {
			fmt.Fprintf(os.Stderr, "golem: embeddings synced\n")
		}
	}

	return nil
}
```

- [ ] **Step 4: Run to verify tests pass**

Run: `go test ./internal/runner/ -run TestSyncGraph -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Wire syncGraph into Engine.Run()**

In `internal/runner/engine.go`, in `Run()`, after the MCP setup block:

```go
	// Graph sync
	if err := syncGraph(e.cfg.Dir, e.cfg.GraphPath); err != nil {
		log.Printf("golem: warning: graph sync: %v", err)
	}
```

- [ ] **Step 6: Run full test suite**

Run: `go test ./internal/runner/ -count=1`
Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/runner/engine.go internal/runner/engine_context.go internal/runner/engine_context_test.go
git commit -m "feat(runner): add graph sync at blueprint engine start"
```

---

### Task 7: Execution collector for agentic steps

**Files:**
- Modify: `internal/runner/engine_context.go`
- Modify: `internal/runner/engine_context_test.go`
- Modify: `internal/runner/engine.go`

- [ ] **Step 1: Write the test for setupCollector**

Add to `internal/runner/engine_context_test.go`:

```go
func TestSetupCollector_NonClaudeRunner(t *testing.T) {
	// When runner is not a *ClaudeRunner, setupCollector should return nil cleanup
	mock := &smartMockRunner{
		responses: func(step string, callNum int) MockResponse {
			return MockResponse{}
		},
	}
	cleanup := setupCollector(t.TempDir(), "", mock, 5)
	if cleanup != nil {
		t.Error("cleanup should be nil for non-ClaudeRunner")
	}
}

func TestSetupCollector_NoGraphDB(t *testing.T) {
	dir := t.TempDir()
	cr := &ClaudeRunner{StreamJSON: true}
	cleanup := setupCollector(dir, filepath.Join(dir, "nonexistent.db"), cr, 5)
	if cleanup != nil {
		t.Error("cleanup should be nil when graph.db doesn't exist")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/runner/ -run TestSetupCollector -count=1 -v`
Expected: FAIL — `setupCollector` undefined.

- [ ] **Step 3: Implement setupCollector**

Add to `internal/runner/engine_context.go`:

```go
import (
	"time"

	"github.com/lofari/golem/internal/graph/execution"
)

// setupCollector wires an execution collector into the ClaudeRunner's stream parser.
// If graphPath is empty, defaults to .ctx/graph.db relative to dir.
// Returns a cleanup function that should be deferred, or nil if setup was skipped.
func setupCollector(dir, graphPath string, runner CommandRunner, keepSessions int) func(status string) {
	cr, ok := runner.(*ClaudeRunner)
	if !ok || !cr.StreamJSON {
		return nil
	}

	if graphPath == "" {
		graphPath = filepath.Join(dir, ".ctx", "graph.db")
	}
	if _, err := os.Stat(graphPath); os.IsNotExist(err) {
		return nil
	}

	store, err := graph.OpenStore(graphPath)
	if err != nil {
		log.Printf("golem: warning: could not open graph for collector: %v", err)
		return nil
	}

	sessionID := fmt.Sprintf("blueprint-%d", time.Now().Unix())
	collector := execution.NewCollector(store, sessionID)

	if keepSessions < 1 {
		keepSessions = 5
	}
	if _, pErr := execution.PruneSessions(store, keepSessions); pErr != nil {
		log.Printf("golem: warning: prune sessions: %v", pErr)
	}

	collector.Start()

	cr.SetupStreamCallbacks = func(parser *StreamParser) {
		parser.OnBashCommand = collector.OnBashCommand
		parser.OnBashResult = collector.OnBashResult
	}

	return func(status string) {
		collector.Finish(status)
		store.Close()
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/runner/ -run TestSetupCollector -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Wire setupCollector into Engine.Run()**

In `internal/runner/engine.go`, in `Run()`, after graph sync:

```go
	// Execution collector
	collectorCleanup := setupCollector(e.cfg.Dir, e.cfg.GraphPath, e.cfg.Runner, 5)
```

Then, before the pipeline loop, set up a deferred cleanup that captures the final status:

```go
	var pipelineStatus string
	if collectorCleanup != nil {
		defer func() {
			collectorCleanup(pipelineStatus)
		}()
	}
```

And update the pipeline completion to set `pipelineStatus`:

```go
	for _, node := range e.cfg.Blueprint.pipeline.Nodes {
		if err := e.execNode(ctx, node); err != nil {
			pipelineStatus = "failed"
			e.emit(EngineEvent{Type: "pipeline-end", Status: "error", Duration: time.Since(start).Milliseconds(), RunID: e.RunID})
			return e.state, err
		}
	}

	pipelineStatus = "completed"
	e.emit(EngineEvent{Type: "pipeline-end", Status: "success", Duration: time.Since(start).Milliseconds(), RunID: e.RunID})
	return e.state, nil
```

- [ ] **Step 6: Run full test suite**

Run: `go test ./internal/runner/ -count=1`
Expected: all tests pass. Existing tests use mock runners, so collector setup is safely skipped.

- [ ] **Step 7: Commit**

```bash
git add internal/runner/engine.go internal/runner/engine_context.go internal/runner/engine_context_test.go
git commit -m "feat(runner): wire execution collector into blueprint engine"
```

---

## Chunk 4: Integration Test and Final Verification

### Task 8: Integration test — engine with full context

**Files:**
- Modify: `internal/runner/engine_test.go`

- [ ] **Step 1: Write integration test**

Add to `internal/runner/engine_test.go`:

```go
func TestEngine_Integration_WithProjectContext(t *testing.T) {
	dir := setupGitRepo(t)
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	// Write a state.yaml with decisions and pitfalls
	stateYAML := `project:
  name: testproj
  summary: test
  stack: go
decisions:
  - what: use blueprint engine
    why: better execution semantics
    when: "2026-03-14"
pitfalls:
  - what: MCP server needs .ctx/ dir
    fix: ensure init is run first
tasks: []
`
	os.WriteFile(filepath.Join(dir, ".ctx", "state.yaml"), []byte(stateYAML), 0644)

	// Use build-feature agent template
	data, err := templates.FS.ReadFile("agents/build-feature.yaml")
	if err != nil {
		t.Fatalf("read agent: %v", err)
	}
	bp, err := ParseBlueprint(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	mock := &smartMockRunner{
		responses: func(step string, callNum int) MockResponse {
			if step == "review" {
				return MockResponse{SessionOutput: map[string]any{"review-feedback": map[string]any{"verdict": "approved"}}}
			}
			if step == "plan" {
				return MockResponse{SessionOutput: map[string]any{"plan": []any{map[string]any{"step": 1, "desc": "do it"}}}}
			}
			return MockResponse{SessionOutput: map[string]any{"test-results": map[string]any{"status": "pass"}}}
		},
	}

	config := map[string]any{"lint-cmd": "true", "test-cmd": "true", "ci-enabled": false}
	e := NewEngine(EngineConfig{
		Dir: dir, AgentName: "build-feature", Goal: "Add auth",
		Blueprint: bp, Config: config, Runner: mock, Model: "test",
	})

	state, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("engine error: %v", err)
	}

	// Verify project-context was injected
	pc, ok := state["project-context"].(string)
	if !ok {
		t.Fatal("project-context should be in final state")
	}
	if !strings.Contains(pc, "blueprint engine") {
		t.Error("project-context should contain decision 'use blueprint engine'")
	}
	if !strings.Contains(pc, "MCP server") {
		t.Error("project-context should contain pitfall about MCP server")
	}
}
```

- [ ] **Step 2: Run to verify it passes**

Run: `go test ./internal/runner/ -run TestEngine_Integration_WithProjectContext -count=1 -v`
Expected: PASS.

- [ ] **Step 3: Run full project test suite**

Run: `go test ./... -count=1`
Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/runner/engine_test.go
git commit -m "test(runner): add integration test for engine with project context"
```

---

### Task 9: Final verification and cleanup

- [ ] **Step 1: Verify build**

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 2: Run full test suite**

Run: `go test ./... -count=1`
Expected: all tests pass.

- [ ] **Step 3: Verify no unused imports or variables**

Run: `go vet ./...`
Expected: no warnings.

- [ ] **Step 4: Review diff**

Run: `git diff main --stat`

Verify the changes are scoped to:
- `internal/runner/engine.go` — new fields + context calls in Run()
- `internal/runner/engine_context.go` — new file with 4 functions
- `internal/runner/engine_context_test.go` — new test file
- `internal/runner/engine_test.go` — new integration tests
- `cmd/code.go` — MCPEnabled/LSPEnabled passed to EngineConfig
- `cmd/run.go` — MCPEnabled/LSPEnabled passed to EngineConfig
- `cmd/helpers.go` — --no-lsp moved into addAgentFlags
