# DSL Integration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Integrate the Clojure DSL as golem's orchestration engine (Phase 1 — coexistence), so `golem run <agent>` delegates to the DSL while `golem code` retains the Go loop by default.

**Architecture:** Two sidecar binaries (`golem` Go + `golem-dsl` GraalVM native) communicate via CLI calls and shared filesystem (`.ctx/`). The DSL emits NDJSON events on stdout; Go parses and displays them. State syncs from DSL's EDN to `.ctx/state.yaml`.

**Tech Stack:** Go (cobra CLI), Clojure (deps.edn, Stencil templates, GraalVM native-image)

**Design doc:** `docs/plans/2026-03-10-dsl-integration-design.md`

---

## Task 1: Port `golem session` Command to Main Branch

The DSL's `claude.clj` adapter calls `golem session --prompt <file> --dir <dir>` to spawn individual Claude sessions. This command exists on `feature/agent-dsl` but not on main.

**Files:**
- Create: `cmd/session.go`
- Test: `cmd/session_test.go`

**Step 1: Write the test**

```go
// cmd/session_test.go
package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionCmd_RequiresPromptFlag(t *testing.T) {
	cmd := rootCmd()
	cmd.SetArgs([]string{"session"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --prompt not provided")
	}
}

func TestSessionCmd_ReadsPromptFile(t *testing.T) {
	dir := t.TempDir()
	prompt := filepath.Join(dir, "prompt.md")
	os.WriteFile(prompt, []byte("test prompt"), 0644)

	cmd := rootCmd()
	cmd.SetArgs([]string{"session", "--prompt", prompt, "--dir", dir, "--dry-run"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/ -run TestSessionCmd -v`
Expected: FAIL — `session` command not defined

**Step 3: Implement the session command**

Port from `/home/winler/projects/golem/.worktrees/agent-dsl/cmd/session.go` (59 lines). The command:
1. Reads `--prompt` file path (required)
2. Reads `--dir` working directory (default: `.`)
3. Reads `--max-turns` (default from config)
4. Creates a `ClaudeRunner` and calls `Run()` with the prompt contents
5. Prints output to stdout
6. Supports `--dry-run` that prints the prompt and exits

Wire it into `cmd/root.go` via `rootCmd.AddCommand(sessionCmd)`.

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/ -run TestSessionCmd -v`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/session.go cmd/session_test.go
git commit -m "feat(cmd): add golem session command for DSL adapter"
```

---

## Task 2: Add `engine` and `dsl_command` Config Options

The config system needs two new fields: `engine` (values: `"go"`, `"dsl"`, default `"go"`) and `dsl_command` (default: `"golem-dsl"`, overridable for dev).

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Step 1: Write the test**

```go
func TestConfig_EngineDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Engine != "go" {
		t.Fatalf("expected engine=go, got %s", cfg.Engine)
	}
	if cfg.DSLCommand != "golem-dsl" {
		t.Fatalf("expected dsl_command=golem-dsl, got %s", cfg.DSLCommand)
	}
}

func TestConfig_EngineFromYAML(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("engine: dsl\ndsl_command: clj -M:run\n"), 0644)
	cfg, err := LoadFrom(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Engine != "dsl" {
		t.Fatalf("expected engine=dsl, got %s", cfg.Engine)
	}
	if cfg.DSLCommand != "clj -M:run" {
		t.Fatalf("expected dsl_command='clj -M:run', got %s", cfg.DSLCommand)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestConfig_Engine -v`
Expected: FAIL — `Engine` field not defined

**Step 3: Add fields to Config struct**

In `internal/config/config.go`, add to the `Config` struct:

```go
Engine     string   `yaml:"engine"`      // "go" or "dsl"
DSLCommand string   `yaml:"dsl_command"` // path to golem-dsl binary
```

In `DefaultConfig()`, set `Engine: "go"` and `DSLCommand: "golem-dsl"`.

Add `"engine"` and `"dsl_command"` to `SetValue()`/`GetValue()` switch statements.

**Step 4: Run tests**

Run: `go test ./internal/config/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add engine and dsl_command config options"
```

---

## Task 3: Add `agent` and `agent_opts` Config Options

Config needs `agent` (default: `"build-feature"`) and `agent_opts` map for DSL agent configuration.

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Step 1: Write the test**

```go
func TestConfig_AgentDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Agent != "build-feature" {
		t.Fatalf("expected agent=build-feature, got %s", cfg.Agent)
	}
}

func TestConfig_AgentOptsFromYAML(t *testing.T) {
	dir := t.TempDir()
	yaml := "agent: fix-bug\nagent_opts:\n  max_iterations: 3\n"
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0644)
	cfg, _ := LoadFrom(dir)
	if cfg.Agent != "fix-bug" {
		t.Fatalf("expected agent=fix-bug, got %s", cfg.Agent)
	}
	if cfg.AgentOpts["max_iterations"] != 3 {
		t.Fatalf("expected max_iterations=3, got %v", cfg.AgentOpts["max_iterations"])
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestConfig_Agent -v`
Expected: FAIL — `Agent` field not defined

**Step 3: Add fields**

```go
Agent     string                 `yaml:"agent"`
AgentOpts map[string]interface{} `yaml:"agent_opts"`
```

Default: `Agent: "build-feature"`, `AgentOpts: nil`.

**Step 4: Run tests**

Run: `go test ./internal/config/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add agent and agent_opts config options"
```

---

## Task 4: Define DSL Event Types and JSON Parser

The Go binary needs to parse NDJSON events emitted by the DSL on stdout and map them to the existing `Event` system.

**Files:**
- Create: `internal/runner/dsl_events.go`
- Create: `internal/runner/dsl_events_test.go`

**Step 1: Write the test**

```go
// internal/runner/dsl_events_test.go
package runner

import (
	"strings"
	"testing"
)

func TestParseDSLEvent_StepStart(t *testing.T) {
	line := `{"type":"step-start","step":"plan","iteration":1,"agent":"build-feature"}`
	evt, err := ParseDSLEvent(line)
	if err != nil {
		t.Fatal(err)
	}
	if evt.Type != "step-start" {
		t.Fatalf("expected step-start, got %s", evt.Type)
	}
	if evt.Step != "plan" {
		t.Fatalf("expected plan, got %s", evt.Step)
	}
}

func TestParseDSLEvent_AgentDone(t *testing.T) {
	line := `{"type":"agent-done","agent":"build-feature","outcome":"complete","total-steps":5}`
	evt, err := ParseDSLEvent(line)
	if err != nil {
		t.Fatal(err)
	}
	if evt.Outcome != "complete" {
		t.Fatalf("expected complete, got %s", evt.Outcome)
	}
}

func TestParseDSLEvents_Stream(t *testing.T) {
	input := strings.NewReader(
		`{"type":"step-start","step":"plan","iteration":1,"agent":"build-feature"}
{"type":"step-end","step":"plan","state-version":1}
{"type":"agent-done","agent":"build-feature","outcome":"complete","total-steps":2}
`)
	events, err := ParseDSLEventStream(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
}

func TestMapDSLEventToEvent_StepStart(t *testing.T) {
	dsl := DSLEvent{Type: "step-start", Step: "plan", Iteration: 1, Agent: "build-feature"}
	evt := MapDSLEvent(dsl, 5)
	if evt.Type != EventIterStart {
		t.Fatalf("expected EventIterStart, got %v", evt.Type)
	}
	if evt.Iter != 1 {
		t.Fatalf("expected iter=1, got %d", evt.Iter)
	}
}

func TestMapDSLEventToEvent_AgentDone(t *testing.T) {
	dsl := DSLEvent{Type: "agent-done", Outcome: "complete", TotalSteps: 5}
	evt := MapDSLEvent(dsl, 5)
	if evt.Type != EventLoopDone {
		t.Fatalf("expected EventLoopDone, got %v", evt.Type)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/runner/ -run TestParseDSLEvent -v`
Expected: FAIL — types not defined

**Step 3: Implement DSL event types and parser**

```go
// internal/runner/dsl_events.go
package runner

import (
	"bufio"
	"encoding/json"
	"io"
)

// DSLEvent represents a JSON event emitted by golem-dsl on stdout.
type DSLEvent struct {
	Type       string `json:"type"`
	Step       string `json:"step,omitempty"`
	Iteration  int    `json:"iteration,omitempty"`
	Agent      string `json:"agent,omitempty"`
	SessionID  string `json:"session-id,omitempty"`
	Outcome    string `json:"outcome,omitempty"`
	DurationMs int    `json:"duration-ms,omitempty"`
	StateVer   int    `json:"state-version,omitempty"`
	ErrorType  string `json:"error-type,omitempty"`
	Action     string `json:"action,omitempty"`
	Attempt    int    `json:"attempt,omitempty"`
	TotalSteps int    `json:"total-steps,omitempty"`
}

func ParseDSLEvent(line string) (DSLEvent, error) {
	var evt DSLEvent
	err := json.Unmarshal([]byte(line), &evt)
	return evt, err
}

func ParseDSLEventStream(r io.Reader) ([]DSLEvent, error) {
	var events []DSLEvent
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		evt, err := ParseDSLEvent(scanner.Text())
		if err != nil {
			return events, err
		}
		events = append(events, evt)
	}
	return events, scanner.Err()
}

// MapDSLEvent converts a DSLEvent to the existing Event type for display.
func MapDSLEvent(dsl DSLEvent, maxIter int) Event {
	switch dsl.Type {
	case "step-start":
		return Event{Type: EventIterStart, Iter: dsl.Iteration, MaxIter: maxIter, Task: dsl.Step}
	case "step-end":
		return Event{Type: EventIterEnd, Iter: dsl.Iteration, MaxIter: maxIter, Task: dsl.Step, Outcome: "done"}
	case "session-end":
		return Event{Type: EventIterEnd, Iter: dsl.Iteration, Task: dsl.Step, Outcome: dsl.Outcome}
	case "error":
		return Event{Type: EventIterEnd, Iter: dsl.Iteration, Task: dsl.Step, Outcome: dsl.ErrorType}
	case "agent-done":
		return Event{Type: EventLoopDone, Outcome: dsl.Outcome, Result: &BuilderResult{Outcome: dsl.Outcome}}
	default:
		return Event{Type: EventOutputLine, Line: dsl.Type}
	}
}
```

**Step 4: Run tests**

Run: `go test ./internal/runner/ -run TestParseDSLEvent -v && go test ./internal/runner/ -run TestMapDSLEvent -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/runner/dsl_events.go internal/runner/dsl_events_test.go
git commit -m "feat(runner): add DSL NDJSON event parser and mapper"
```

---

## Task 5: Add DSL Runner — Go-side Orchestrator

Create `DSLRunner` that spawns `golem-dsl run <agent>` as a subprocess and streams its NDJSON events to the Go event channel.

**Files:**
- Create: `internal/runner/dsl_runner.go`
- Create: `internal/runner/dsl_runner_test.go`

**Step 1: Write the test**

```go
// internal/runner/dsl_runner_test.go
package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDSLRunner_BuildArgs(t *testing.T) {
	r := &DSLRunner{
		DSLCommand: "golem-dsl",
		Agent:      "build-feature",
		Goal:       "add auth",
		StateDir:   "/tmp/test",
	}
	args := r.buildArgs()
	expected := []string{"golem-dsl", "run", "build-feature", "--goal", "add auth", "--state-dir", "/tmp/test"}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, a := range expected {
		if args[i] != a {
			t.Fatalf("arg[%d]: expected %s, got %s", i, a, args[i])
		}
	}
}

func TestDSLRunner_BuildArgsWithOpts(t *testing.T) {
	r := &DSLRunner{
		DSLCommand: "golem-dsl",
		Agent:      "fix-bug",
		Goal:       "fix login",
		StateDir:   "/tmp/test",
		AgentOpts:  map[string]interface{}{"max_iterations": 3},
	}
	args := r.buildArgs()
	// Should include --opt max_iterations=3
	found := false
	for i, a := range args {
		if a == "--opt" && i+1 < len(args) && args[i+1] == "max_iterations=3" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected --opt max_iterations=3 in args: %v", args)
	}
}

func TestDSLRunner_CheckBinary_Missing(t *testing.T) {
	r := &DSLRunner{DSLCommand: "nonexistent-binary-xyz"}
	err := r.CheckBinary()
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/runner/ -run TestDSLRunner -v`
Expected: FAIL — `DSLRunner` not defined

**Step 3: Implement DSLRunner**

```go
// internal/runner/dsl_runner.go
package runner

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type DSLRunner struct {
	DSLCommand string
	Agent      string
	Goal       string
	StateDir   string
	AgentOpts  map[string]interface{}
	Events     chan<- Event
	MaxIter    int
}

func (r *DSLRunner) buildArgs() []string {
	args := []string{r.DSLCommand, "run", r.Agent, "--goal", r.Goal, "--state-dir", r.StateDir}
	for k, v := range r.AgentOpts {
		args = append(args, "--opt", fmt.Sprintf("%s=%v", k, v))
	}
	return args
}

func (r *DSLRunner) CheckBinary() error {
	_, err := exec.LookPath(r.DSLCommand)
	if err != nil {
		return fmt.Errorf("golem-dsl binary not found: %s\nInstall it or set dsl_command in config", r.DSLCommand)
	}
	return nil
}

func (r *DSLRunner) Run(ctx context.Context) (*BuilderResult, error) {
	args := r.buildArgs()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = r.StateDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stderr // inherit stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start golem-dsl: %w", err)
	}

	var lastEvent DSLEvent
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		evt, err := ParseDSLEvent(scanner.Text())
		if err != nil {
			continue // skip non-JSON lines
		}
		lastEvent = evt
		if r.Events != nil {
			r.Events <- MapDSLEvent(evt, r.MaxIter)
		}
	}

	if err := cmd.Wait(); err != nil {
		return &BuilderResult{Outcome: "error"}, fmt.Errorf("golem-dsl exited: %w", err)
	}

	outcome := lastEvent.Outcome
	if outcome == "" {
		outcome = "complete"
	}
	return &BuilderResult{Outcome: outcome}, nil
}
```

**Step 4: Run tests**

Run: `go test ./internal/runner/ -run TestDSLRunner -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/runner/dsl_runner.go internal/runner/dsl_runner_test.go
git commit -m "feat(runner): add DSLRunner to spawn and stream golem-dsl"
```

---

## Task 6: Add `golem run` Command

New command that delegates to the DSL for a specific agent. This is the primary user-facing DSL command in Phase 1.

**Files:**
- Create: `cmd/run.go`
- Create: `cmd/run_test.go`
- Modify: `cmd/root.go` (add command)

**Step 1: Write the test**

```go
// cmd/run_test.go
package cmd

import (
	"testing"
)

func TestRunCmd_RequiresAgentArg(t *testing.T) {
	cmd := rootCmd()
	cmd.SetArgs([]string{"run"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when agent arg not provided")
	}
}

func TestRunCmd_RequiresGoalFlag(t *testing.T) {
	cmd := rootCmd()
	cmd.SetArgs([]string{"run", "build-feature"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --goal not provided")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/ -run TestRunCmd -v`
Expected: FAIL — `run` command not recognized (note: `run` was an alias for `code` — rename that alias to avoid conflict, or use `agent` as the command name)

**Important:** Check if `run` is already an alias for `golem code`. If so, the new command should be `golem agent run` or we should remove the `run` alias from `code`. The design says `golem run <agent>`, so remove the `run` alias from `golem code` and claim it for DSL.

**Step 3: Implement**

```go
// cmd/run.go
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var runAgentCmd = &cobra.Command{
	Use:   "run <agent-name>",
	Short: "Run a DSL-defined agent",
	Long:  "Run a named agent (built-in or project-local from .ctx/agents/)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		agentName := args[0]
		goal, _ := cmd.Flags().GetString("goal")
		if goal == "" {
			return fmt.Errorf("--goal is required")
		}

		cfg := resolveConfig()

		dsl := &runner.DSLRunner{
			DSLCommand: cfg.DSLCommand,
			Agent:      agentName,
			Goal:       goal,
			StateDir:   cfg.Dir,
			AgentOpts:  cfg.AgentOpts,
			Events:     nil, // TODO: wire to TUI
			MaxIter:    cfg.MaxIterations,
		}

		if err := dsl.CheckBinary(); err != nil {
			return err
		}

		result, err := dsl.Run(context.Background())
		if err != nil {
			return err
		}

		fmt.Printf("Agent %s completed: %s\n", agentName, result.Outcome)
		return nil
	},
}

func init() {
	runAgentCmd.Flags().String("goal", "", "Goal description for the agent")
}
```

Remove `run` alias from `cmd/code.go` (currently `Aliases: []string{"run"}`).

**Step 4: Run tests**

Run: `go test ./cmd/ -run TestRunCmd -v`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/run.go cmd/run_test.go cmd/code.go cmd/root.go
git commit -m "feat(cmd): add golem run command for DSL agents"
```

---

## Task 7: Add `golem agents` Command

Lists available agents from built-in and project-local sources.

**Files:**
- Create: `cmd/agents.go`
- Create: `cmd/agents_test.go`

**Step 1: Write the test**

```go
// cmd/agents_test.go
package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListAgents_FindsProjectLocal(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".ctx", "agents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "my-flow.clj"), []byte("(defagent my-flow)"), 0644)

	agents, err := findProjectAgents(agentsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0] != "my-flow" {
		t.Fatalf("expected [my-flow], got %v", agents)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/ -run TestListAgents -v`
Expected: FAIL

**Step 3: Implement**

```go
// cmd/agents.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var builtinAgents = []struct {
	Name string
	Desc string
}{
	{"build-feature", "Plan → implement → review loop"},
	{"fix-bug", "Research → fix → test loop"},
	{"write-docs", "Documentation generator"},
	{"review", "Single-pass code review"},
}

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "List available DSL agents",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Built-in agents:")
		for _, a := range builtinAgents {
			fmt.Printf("  %-20s %s\n", a.Name, a.Desc)
		}

		agentsDir := filepath.Join(".ctx", "agents")
		local, err := findProjectAgents(agentsDir)
		if err == nil && len(local) > 0 {
			fmt.Println("\nProject agents:")
			for _, name := range local {
				fmt.Printf("  %-20s .ctx/agents/%s.clj\n", name, name)
			}
		}
		return nil
	},
}

func findProjectAgents(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var agents []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".clj") {
			agents = append(agents, strings.TrimSuffix(e.Name(), ".clj"))
		}
	}
	return agents, nil
}
```

**Step 4: Run tests**

Run: `go test ./cmd/ -run TestListAgents -v`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/agents.go cmd/agents_test.go cmd/root.go
git commit -m "feat(cmd): add golem agents command to list available agents"
```

---

## Task 8: Wire `golem code` to Delegate to DSL When `engine: dsl`

When config has `engine: dsl`, `golem code` should delegate to the DSL instead of running the Go builder loop.

**Files:**
- Modify: `cmd/code.go`
- Test: `cmd/code_test.go` (if exists, otherwise create)

**Step 1: Write the test**

```go
func TestCodeCmd_EngineSelection(t *testing.T) {
	tests := []struct {
		engine   string
		wantsDSL bool
	}{
		{"go", false},
		{"dsl", true},
		{"", false}, // default is go
	}
	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			result := shouldUseDSL(tt.engine)
			if result != tt.wantsDSL {
				t.Fatalf("engine=%q: expected shouldUseDSL=%v, got %v", tt.engine, tt.wantsDSL, result)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/ -run TestCodeCmd_Engine -v`
Expected: FAIL

**Step 3: Implement engine selection in code.go**

Add a helper function and branch in the `code` command's `RunE`:

```go
func shouldUseDSL(engine string) bool {
	return engine == "dsl"
}
```

In the command's `RunE`, after `resolveConfig()`:

```go
if shouldUseDSL(cfg.Engine) {
    dsl := &runner.DSLRunner{
        DSLCommand: cfg.DSLCommand,
        Agent:      cfg.Agent,
        Goal:       cfg.TaskOverride, // --task flag becomes the goal
        StateDir:   cfg.Dir,
        AgentOpts:  cfg.AgentOpts,
        Events:     events,
        MaxIter:    cfg.MaxIterations,
    }
    if err := dsl.CheckBinary(); err != nil {
        return err
    }
    result, err := dsl.Run(ctx)
    // handle result/err
    return nil
}
// else: existing RunBuilderLoop path
```

**Step 4: Run tests**

Run: `go test ./cmd/ -run TestCodeCmd -v`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/code.go cmd/code_test.go
git commit -m "feat(cmd): golem code delegates to DSL when engine=dsl"
```

---

## Task 9: DSL — Add NDJSON Event Emission to Engine

The DSL execution engine must emit NDJSON events on stdout so the Go binary can stream them.

**Files:**
- Create: `golem-dsl/src/golem/dsl/engine/events.clj`
- Modify: `golem-dsl/src/golem/dsl/engine/core.clj`
- Test: `golem-dsl/test/golem/dsl/engine/events_test.clj`

**Step 1: Write the test**

```clojure
;; test/golem/dsl/engine/events_test.clj
(ns golem.dsl.engine.events-test
  (:require [clojure.test :refer :all]
            [clojure.data.json :as json]
            [golem.dsl.engine.events :as events]))

(deftest emit-step-start-test
  (let [output (with-out-str (events/emit! :step-start {:step "plan" :iteration 1 :agent "build-feature"}))]
    (is (= "step-start" (get (json/read-str output) "type")))
    (is (= "plan" (get (json/read-str output) "step")))))

(deftest emit-agent-done-test
  (let [output (with-out-str (events/emit! :agent-done {:agent "build-feature" :outcome "complete" :total-steps 5}))]
    (is (= "agent-done" (get (json/read-str output) "type")))
    (is (= "complete" (get (json/read-str output) "outcome")))))
```

**Step 2: Run test to verify it fails**

Run: `cd golem-dsl && clj -M:test -n golem.dsl.engine.events-test`
Expected: FAIL — namespace not found

**Step 3: Implement event emission**

```clojure
;; src/golem/dsl/engine/events.clj
(ns golem.dsl.engine.events
  (:require [clojure.data.json :as json]))

(defn emit! [event-type data]
  (println (json/write-str (assoc data :type (name event-type)))))
```

Then modify `engine/core.clj` to call `events/emit!` at each step boundary:
- Before executing a node: `(emit! :step-start {...})`
- After session spawns: `(emit! :session-start {...})`
- After session completes: `(emit! :session-end {...})`
- After step completes: `(emit! :step-end {...})`
- On error: `(emit! :error {...})`
- At agent completion: `(emit! :agent-done {...})`

**Step 4: Run tests**

Run: `cd golem-dsl && clj -M:test -n golem.dsl.engine.events-test`
Expected: PASS

**Step 5: Commit**

```bash
cd golem-dsl
git add src/golem/dsl/engine/events.clj test/golem/dsl/engine/events_test.clj src/golem/dsl/engine/core.clj
git commit -m "feat(dsl): emit NDJSON events from execution engine"
```

---

## Task 10: DSL — Add State Sync to `.ctx/state.yaml`

After each step, the DSL writes a summary projection to `.ctx/state.yaml` so `golem status` works.

**Files:**
- Create: `golem-dsl/src/golem/dsl/sync.clj`
- Test: `golem-dsl/test/golem/dsl/sync_test.clj`

**Step 1: Write the test**

```clojure
;; test/golem/dsl/sync_test.clj
(ns golem.dsl.sync-test
  (:require [clojure.test :refer :all]
            [golem.dsl.sync :as sync]
            [clj-yaml.core :as yaml]))

(deftest project-state-from-dsl-state-test
  (let [dsl-state {:goal "add auth"
                   :plan [{:step 1 :desc "implement"}]
                   :code {:files ["auth.go"]}
                   :test-results {:status :pass}}
        yaml-str (sync/project-state-yaml dsl-state "build-feature" "building")]
    (is (.contains yaml-str "phase: building"))
    (is (.contains yaml-str "current_focus: add auth"))))

(deftest write-state-yaml-test
  (let [dir (System/getProperty "java.io.tmpdir")
        path (str dir "/state.yaml")]
    (sync/write-state-yaml! path {:goal "test"} "build-feature" "building")
    (is (.exists (java.io.File. path)))))
```

**Step 2: Run test to verify it fails**

Run: `cd golem-dsl && clj -M:test -n golem.dsl.sync-test`
Expected: FAIL

**Step 3: Implement state sync**

```clojure
;; src/golem/dsl/sync.clj
(ns golem.dsl.sync
  (:require [clj-yaml.core :as yaml]))

(defn project-state-yaml
  "Convert DSL state to a state.yaml compatible string."
  [dsl-state agent-name phase]
  (yaml/generate-string
    {:project {:name "" :summary ""}
     :status {:current_focus (:goal dsl-state "")
              :phase phase
              :last_session (str agent-name)}
     :tasks []
     :decisions []
     :pitfalls []}
    :dumper-options {:flow-style :block}))

(defn write-state-yaml! [path dsl-state agent-name phase]
  (spit path (project-state-yaml dsl-state agent-name phase)))
```

Then modify `engine/core.clj` to call `sync/write-state-yaml!` after each step, using the state-dir from opts.

**Step 4: Run tests**

Run: `cd golem-dsl && clj -M:test -n golem.dsl.sync-test`
Expected: PASS

**Step 5: Commit**

```bash
cd golem-dsl
git add src/golem/dsl/sync.clj test/golem/dsl/sync_test.clj src/golem/dsl/engine/core.clj
git commit -m "feat(dsl): sync state to .ctx/state.yaml after each step"
```

---

## Task 11: DSL — Add Agent Resolution (Built-in + Project-Local)

The DSL CLI needs to resolve agent names by checking `.ctx/agents/` first, then built-in agents.

**Files:**
- Create: `golem-dsl/src/golem/dsl/resolve.clj`
- Test: `golem-dsl/test/golem/dsl/resolve_test.clj`
- Modify: `golem-dsl/src/golem/dsl/cli/main.clj`

**Step 1: Write the test**

```clojure
;; test/golem/dsl/resolve_test.clj
(ns golem.dsl.resolve-test
  (:require [clojure.test :refer :all]
            [golem.dsl.resolve :as resolve]))

(deftest resolve-builtin-test
  (let [path (resolve/resolve-agent "build-feature" nil)]
    (is (some? path))
    (is (.contains (str path) "build_feature"))))

(deftest resolve-project-local-test
  (let [dir (System/getProperty "java.io.tmpdir")
        agents-dir (str dir "/agents")
        _ (.mkdirs (java.io.File. agents-dir))
        _ (spit (str agents-dir "/my-flow.clj") "(defagent my-flow)")
        path (resolve/resolve-agent "my-flow" agents-dir)]
    (is (some? path))
    (is (.contains (str path) "my-flow.clj"))))

(deftest resolve-project-overrides-builtin-test
  (let [dir (System/getProperty "java.io.tmpdir")
        agents-dir (str dir "/agents2")
        _ (.mkdirs (java.io.File. agents-dir))
        _ (spit (str agents-dir "/build-feature.clj") "(defagent build-feature :custom)")
        path (resolve/resolve-agent "build-feature" agents-dir)]
    (is (.contains (str path) agents-dir))
    (is (not (.contains (str path) "resources")))))

(deftest list-agents-test
  (let [agents (resolve/list-agents nil)]
    (is (>= (count agents) 4))
    (is (some #(= (:name %) "build-feature") agents))))
```

**Step 2: Run test to verify it fails**

Run: `cd golem-dsl && clj -M:test -n golem.dsl.resolve-test`
Expected: FAIL

**Step 3: Implement**

```clojure
;; src/golem/dsl/resolve.clj
(ns golem.dsl.resolve
  (:require [clojure.java.io :as io]
            [clojure.string :as str]))

(def builtin-agents
  [{:name "build-feature" :desc "Plan → implement → review loop" :resource "agents/build_feature.clj"}
   {:name "fix-bug"       :desc "Research → fix → test loop"     :resource "agents/fix_bug.clj"}
   {:name "write-docs"    :desc "Documentation generator"        :resource "agents/write_docs.clj"}
   {:name "review"        :desc "Single-pass code review"        :resource "agents/review.clj"}])

(defn resolve-agent
  "Resolve agent name to file path. Project-local (.ctx/agents/) takes priority."
  [agent-name agents-dir]
  (let [local-file (when agents-dir
                     (let [f (io/file agents-dir (str agent-name ".clj"))]
                       (when (.exists f) (.getPath f))))]
    (or local-file
        (some (fn [{:keys [name resource]}]
                (when (= name agent-name)
                  (if-let [r (io/resource resource)]
                    (.getPath r)
                    resource)))
              builtin-agents))))

(defn list-agents
  "List all available agents with source."
  [agents-dir]
  (let [builtins (map #(assoc % :source :built-in) builtin-agents)
        locals (when agents-dir
                 (let [dir (io/file agents-dir)]
                   (when (.isDirectory dir)
                     (->> (.listFiles dir)
                          (filter #(str/ends-with? (.getName %) ".clj"))
                          (map (fn [f]
                                 {:name (str/replace (.getName f) ".clj" "")
                                  :desc (.getPath f)
                                  :source :project}))))))]
    (concat locals builtins)))
```

Modify `cli/main.clj` to use `resolve/resolve-agent` before loading and running an agent.

**Step 4: Run tests**

Run: `cd golem-dsl && clj -M:test -n golem.dsl.resolve-test`
Expected: PASS

**Step 5: Commit**

```bash
cd golem-dsl
git add src/golem/dsl/resolve.clj test/golem/dsl/resolve_test.clj src/golem/dsl/cli/main.clj
git commit -m "feat(dsl): add agent resolution with project-local priority"
```

---

## Task 12: DSL — Add `--state-dir` and `--goal` Flags to CLI

The DSL CLI needs the flags that Go passes when delegating: `--goal`, `--state-dir`, `--opt`.

**Files:**
- Modify: `golem-dsl/src/golem/dsl/cli/main.clj`
- Test: `golem-dsl/test/golem/dsl/cli/main_test.clj`

**Step 1: Write the test**

```clojure
;; test/golem/dsl/cli/main_test.clj
(ns golem.dsl.cli.main-test
  (:require [clojure.test :refer :all]
            [golem.dsl.cli.main :as cli]))

(deftest parse-args-run-test
  (let [parsed (cli/parse-args ["run" "build-feature" "--goal" "add auth" "--state-dir" "/tmp/test"])]
    (is (= :run (:command parsed)))
    (is (= "build-feature" (:agent parsed)))
    (is (= "add auth" (:goal parsed)))
    (is (= "/tmp/test" (:state-dir parsed)))))

(deftest parse-args-opts-test
  (let [parsed (cli/parse-args ["run" "fix-bug" "--goal" "fix" "--state-dir" "." "--opt" "max_iterations=3"])]
    (is (= {"max_iterations" "3"} (:opts parsed)))))

(deftest parse-args-list-test
  (let [parsed (cli/parse-args ["list"])]
    (is (= :list (:command parsed)))))
```

**Step 2: Run test to verify it fails**

Run: `cd golem-dsl && clj -M:test -n golem.dsl.cli.main-test`
Expected: FAIL

**Step 3: Implement argument parsing**

Refactor `cli/main.clj` to have a `parse-args` function that returns a structured map, then dispatch on `:command`. Add `--goal`, `--state-dir`, `--opt` flag parsing.

**Step 4: Run tests**

Run: `cd golem-dsl && clj -M:test -n golem.dsl.cli.main-test`
Expected: PASS

**Step 5: Commit**

```bash
cd golem-dsl
git add src/golem/dsl/cli/main.clj test/golem/dsl/cli/main_test.clj
git commit -m "feat(dsl): add --goal, --state-dir, --opt flags to CLI"
```

---

## Task 13: DSL — GraalVM Native Image Build Setup

Configure the Clojure project for GraalVM native-image compilation.

**Files:**
- Modify: `golem-dsl/deps.edn` (add native-image alias)
- Create: `golem-dsl/graalvm/reflect-config.json`
- Create: `golem-dsl/Makefile`

**Step 1: Add native-image alias to deps.edn**

```clojure
;; Add to deps.edn :aliases
:native-image
{:main-opts ["-m" "golem.dsl.cli.main"]
 :jvm-opts ["-Dclojure.compiler.direct-linking=true"]}
```

**Step 2: Create Makefile**

```makefile
.PHONY: build test native clean

GRAALVM_HOME ?= $(shell which native-image | xargs dirname | xargs dirname)
CLJ = clj

test:
	$(CLJ) -M:test

build:
	$(CLJ) -M:run --help

native: target/golem-dsl
	@echo "Built target/golem-dsl"

target/golem-dsl: $(shell find src -name '*.clj') deps.edn
	mkdir -p target
	$(CLJ) -M:native-image -e "(compile 'golem.dsl.cli.main)"
	native-image \
		--no-fallback \
		--initialize-at-build-time \
		-H:ReflectionConfigurationFiles=graalvm/reflect-config.json \
		-jar target/golem-dsl.jar \
		-o target/golem-dsl

clean:
	rm -rf target .cpcache
```

**Step 3: Create minimal reflect-config.json**

```json
[
  {
    "name": "clojure.lang.RT",
    "allDeclaredMethods": true,
    "allDeclaredConstructors": true
  }
]
```

Note: The reflect-config will need expansion as we discover reflection needs during native-image compilation. This is iterative.

**Step 4: Verify build works**

Run: `cd golem-dsl && make test`
Expected: All tests pass

**Step 5: Commit**

```bash
cd golem-dsl
git add deps.edn Makefile graalvm/reflect-config.json
git commit -m "build(dsl): add GraalVM native-image build configuration"
```

---

## Task 14: Integration Test — Full Binary Communication

End-to-end test that verifies Go → DSL → Go session → output flow.

**Files:**
- Create: `test/integration/dsl_integration_test.go`

**Step 1: Write the test**

```go
// test/integration/dsl_integration_test.go
//go:build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDSL_EndToEnd_DryRun(t *testing.T) {
	// Skip if golem-dsl not on PATH
	if _, err := exec.LookPath("golem-dsl"); err != nil {
		t.Skip("golem-dsl not found on PATH")
	}

	dir := t.TempDir()
	// Create minimal .ctx structure
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	cmd := exec.Command("golem-dsl", "list")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("golem-dsl list failed: %v", err)
	}
	if !strings.Contains(string(out), "build-feature") {
		t.Fatalf("expected build-feature in agent list, got: %s", out)
	}
}

func TestDSL_EventStream_Format(t *testing.T) {
	if _, err := exec.LookPath("golem-dsl"); err != nil {
		t.Skip("golem-dsl not found on PATH")
	}

	// Run with a mock/dry-run mode that emits events without spawning sessions
	cmd := exec.Command("golem-dsl", "run", "build-feature", "--goal", "test", "--state-dir", t.TempDir(), "--dry-run")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("golem-dsl run --dry-run failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		t.Fatal("expected NDJSON event output")
	}

	// First event should be step-start
	if !strings.Contains(lines[0], `"type":"step-start"`) {
		t.Fatalf("expected step-start event, got: %s", lines[0])
	}
}
```

**Step 2: Verify test structure compiles**

Run: `go test -tags integration ./test/integration/ -v -list '.*' 2>&1 | head -5`
Expected: Lists test names (may skip if binary not available)

**Step 3: Commit**

```bash
git add test/integration/dsl_integration_test.go
git commit -m "test: add DSL integration test for binary communication"
```

---

## Summary

| Task | Component | Description |
|------|-----------|-------------|
| 1 | Go | Port `golem session` command |
| 2 | Go | Add `engine`/`dsl_command` config |
| 3 | Go | Add `agent`/`agent_opts` config |
| 4 | Go | DSL event types and NDJSON parser |
| 5 | Go | DSLRunner — spawn and stream golem-dsl |
| 6 | Go | `golem run <agent>` command |
| 7 | Go | `golem agents` command |
| 8 | Go | Wire `golem code` engine delegation |
| 9 | DSL | NDJSON event emission from engine |
| 10 | DSL | State sync to `.ctx/state.yaml` |
| 11 | DSL | Agent resolution (built-in + project-local) |
| 12 | DSL | CLI flags (`--goal`, `--state-dir`, `--opt`) |
| 13 | DSL | GraalVM native-image build setup |
| 14 | Both | Integration test for binary communication |

Tasks 1-8 are Go-side changes (can be done on main branch). Tasks 9-13 are DSL-side changes (done in `golem-dsl/` on the feature branch). Task 14 ties them together.
