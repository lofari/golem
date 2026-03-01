# Golem Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the golem CLI tool (Phase 1+2) — scaffolding, inspection commands, builder loop, and reviewer.

**Architecture:** Go CLI using Cobra for subcommands. State persists in `.ctx/state.yaml` (YAML). Templates embedded via `go:embed`. Five internal packages: `ctx` (state/log), `scaffold` (init), `runner` (builder/reviewer/prompt/validate), `git` (diff), `display` (formatting).

**Tech Stack:** Go 1.22+, Cobra, gopkg.in/yaml.v3, go:embed

---

## Task 1: Go Module & Root Command

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `cmd/root.go`

**Step 1: Initialize Go module**

Run:
```bash
cd /home/winler/projects/golem
go mod init github.com/winler/golem
```
Expected: `go.mod` created

**Step 2: Add Cobra dependency**

Run:
```bash
go get github.com/spf13/cobra@latest
```
Expected: `go.sum` created, cobra added to `go.mod`

**Step 3: Write main.go**

```go
// main.go
package main

import "github.com/winler/golem/cmd"

func main() {
	cmd.Execute()
}
```

**Step 4: Write cmd/root.go**

```go
// cmd/root.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "golem",
	Short: "Goal-Oriented Loop Execution Manager",
	Long:  "golem runs autonomous AI coding agent loops with persistent state across iterations.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Version = version
}
```

**Step 5: Verify it compiles and runs**

Run: `go build -o golem . && ./golem --version`
Expected: prints version

**Step 6: Commit**

```bash
git add go.mod go.sum main.go cmd/root.go
git commit -m "feat: initialize Go module with Cobra root command"
```

---

## Task 2: Template Files

**Files:**
- Create: `templates/state.yaml`
- Create: `templates/log.yaml`
- Create: `templates/prompt.md`
- Create: `templates/review-prompt.md`
- Create: `templates/claude.md`
- Create: `templates/embed.go`

These are the default files that `golem init` writes into `.ctx/`. They are embedded into the binary via `go:embed`.

**Step 1: Write templates/state.yaml**

```yaml
# Project identity — written once during init/planning
project:
  name: ""
  summary: ""
  stack: ""
  docs_path: "docs/"

# Current session focus — updated every iteration
status:
  current_focus: ""
  phase: planning
  last_session: ""

# Architectural decisions — append only, never delete
decisions: []

# Completed modules — agent must not modify these paths
locked: []

# Work items — agent picks from these, updates status
tasks: []

# Lessons learned — things that went wrong, gotchas discovered
pitfalls: []
```

**Step 2: Write templates/log.yaml**

```yaml
sessions: []
```

**Step 3: Write templates/prompt.md**

This is the builder prompt from the design doc. Copy it exactly as specified in `golem-design.md` lines 188-231. The content is:

```markdown
You are working on this project autonomously as part of a loop.
Each iteration you get fresh context — you have no memory of previous iterations.
All persistent state is in `.ctx/`.

{{ITERATION_CONTEXT}}

## Start of Session
1. Read all design and implementation docs in `{{DOCS_PATH}}` for project context.
2. Read `.ctx/state.yaml` for current progress, decisions, and constraints.
3. Respect ALL entries in `decisions` — do not contradict them without exceptional reason.
4. Do NOT modify files under paths listed in `locked`.
5. Review `pitfalls` before making implementation choices.

## During Session
{{TASK_OVERRIDE}}
1. Pick ONE task from `tasks` (prefer `in-progress` over `todo`).
2. If a task depends on another task that isn't `done`, skip it.
3. Find the matching `## Task` section in the implementation doc for detailed steps and code.
4. Follow the implementation doc's steps for this task. Write tests. Make sure they pass.
5. Commit your work with clear commit messages.

## End of Session
Before exiting, update `.ctx/state.yaml`:
1. Update the task you worked on (status, notes).
2. Mark task as `done` if fully complete and tested.
3. Add any new `decisions` with `what`, `why`, and `when`.
4. Add any new `pitfalls` discovered.
5. Add to `locked` any completed, tested modules that should not be modified.
6. Update `status.current_focus` and `status.last_session` with today's date and summary.

Then append a session entry to `.ctx/log.yaml` under `sessions:`:
- iteration: (increment from last entry)
- timestamp: (current ISO timestamp)
- task: (what you worked on)
- outcome: done | partial | blocked | unproductive
- summary: (brief description)
- files_changed: (list of files you modified)
- decisions_made: (list, if any)
- pitfalls_found: (list, if any)

## Completion
If ALL tasks in `state.yaml` have status `done`, output:
<promise>COMPLETE</promise>
```

**Step 4: Write templates/review-prompt.md**

Copy from `golem-design.md` lines 257-299 exactly.

```markdown
You are reviewing this project. You are a QA/code reviewer, NOT a builder.
DO NOT modify any code, tests, or project files.
Your only job is to read, analyze, and write findings.

## What to Read
1. All design and implementation docs in `{{DOCS_PATH}}` — the original design intent
2. `.ctx/state.yaml` — what the builder claims is done
3. `.ctx/log.yaml` — what happened during building
4. The actual codebase

## What to Check
1. **Plan alignment:** Does the implementation match the design doc? Are requirements missed?
2. **Implementation completeness:** For each task marked `done` in state.yaml, check the corresponding `## Task` section in the implementation doc — were all steps completed?
3. **Task accuracy:** Are tasks marked `done` actually complete? Is state.yaml honest?
4. **Test quality:** Are tests meaningful or just coverage padding? Missing edge cases?
5. **Code quality:** Bugs, inconsistencies, error handling gaps, security issues?
6. **Decision consistency:** Are architectural decisions from state.yaml respected throughout?
7. **Pitfall awareness:** Did the builder fall into any known pitfalls?

## What NOT to Flag
- **Style preferences.** Do not flag formatting, naming conventions, or code organization unless they cause functional problems.
- **Intentional decisions.** If something looks unusual but is explained in `decisions`, it's not an issue. Respect the rationale.
- **Speculative refactors.** Do not suggest rewrites or alternative architectures unless the current implementation has a concrete bug or fails a requirement from the plan.
- **Missing features not in the docs.** Only flag what was specified in the design and implementation docs. Don't invent new requirements.
- **Minor TODOs.** Small improvements that don't affect correctness are not review issues.

## Output
For each issue found, add a task to `.ctx/state.yaml` under `tasks:`:
- Prefix the name with `[review]`
- Set status to `todo`
- Include a clear description in `notes`

Append a session entry to `.ctx/log.yaml`:
- task: "code review"
- outcome: done
- summary: describe what you found (or "no issues found")

If you found issues that need builder attention:
  output <promise>NEEDS_WORK</promise>

If everything looks good:
  output <promise>APPROVED</promise>
```

**Step 5: Write templates/claude.md**

This is the CLAUDE.md section injected by `golem init`:

```markdown
<!-- golem:start -->
## Context Engineering (auto-managed by golem)

This project uses `.ctx/` for persistent state across sessions.

- **Design & implementation docs** — See `project.docs_path` in `.ctx/state.yaml` for location. Read ALL docs for project intent and architecture.
- **`.ctx/state.yaml`** — Current state: tasks, decisions, locked paths, pitfalls. Read at start, update at end of every session.
- **`.ctx/log.yaml`** — Session history. Append an entry at the end of every session.

### Task Mapping Convention
Each `## Task` section in the implementation doc must have a corresponding entry in `state.yaml` tasks. Use the task title from the implementation doc as the task name in state.yaml. The implementation doc contains the detailed steps and code — state.yaml tracks progress.

When planning: create tasks in state.yaml that match 1:1 with the implementation doc sections.
When building: find the matching implementation doc section for your current task and follow its steps.

### Rules
- Respect all `decisions` in state.yaml — do not contradict without exceptional reason.
- Do not modify files under `locked` paths.
- Check `pitfalls` before implementation choices.
- Update state.yaml and log.yaml at the end of every session.
<!-- golem:end -->
```

**Step 6: Write templates/embed.go**

```go
// templates/embed.go
package templates

import "embed"

//go:embed state.yaml log.yaml prompt.md review-prompt.md claude.md
var FS embed.FS
```

**Step 7: Verify it compiles**

Run: `go build -o golem .`
Expected: compiles without error

**Step 8: Commit**

```bash
git add templates/
git commit -m "feat: add embedded template files for .ctx/ scaffolding"
```

---

## Task 3: State Types & Read/Write/Validate

**Files:**
- Create: `internal/ctx/state.go`
- Create: `internal/ctx/state_test.go`

**Step 1: Write the state types and read/write/validate functions**

```go
// internal/ctx/state.go
package ctx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type State struct {
	Project   Project    `yaml:"project"`
	Status    Status     `yaml:"status"`
	Decisions []Decision `yaml:"decisions"`
	Locked    []Lock     `yaml:"locked"`
	Tasks     []Task     `yaml:"tasks"`
	Pitfalls  []string   `yaml:"pitfalls"`
}

type Project struct {
	Name     string `yaml:"name"`
	Summary  string `yaml:"summary"`
	Stack    string `yaml:"stack"`
	DocsPath string `yaml:"docs_path"`
}

type Status struct {
	CurrentFocus string `yaml:"current_focus"`
	Phase        string `yaml:"phase"`
	LastSession  string `yaml:"last_session"`
}

type Decision struct {
	What string `yaml:"what"`
	Why  string `yaml:"why"`
	When string `yaml:"when"`
}

type Lock struct {
	Path string `yaml:"path"`
	Note string `yaml:"note"`
}

type Task struct {
	Name          string `yaml:"name"`
	Status        string `yaml:"status"`
	Notes         string `yaml:"notes,omitempty"`
	DependsOn     string `yaml:"depends_on,omitempty"`
	BlockedReason string `yaml:"blocked_reason,omitempty"`
}

var validPhases = map[string]bool{
	"planning": true, "building": true, "fixing": true, "polishing": true,
}

var validTaskStatuses = map[string]bool{
	"todo": true, "in-progress": true, "done": true, "blocked": true,
}

func StatePath(dir string) string {
	return filepath.Join(dir, ".ctx", "state.yaml")
}

func ReadState(dir string) (State, error) {
	data, err := os.ReadFile(StatePath(dir))
	if err != nil {
		return State{}, fmt.Errorf("reading state.yaml: %w", err)
	}
	var s State
	if err := yaml.Unmarshal(data, &s); err != nil {
		return State{}, fmt.Errorf("parsing state.yaml: %w", err)
	}
	return s, nil
}

func WriteState(dir string, s State) error {
	data, err := yaml.Marshal(&s)
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	return os.WriteFile(StatePath(dir), data, 0644)
}

func ValidateState(s State) error {
	var errs []string
	if s.Project.Name == "" {
		errs = append(errs, "project.name is required")
	}
	if s.Status.Phase != "" && !validPhases[s.Status.Phase] {
		errs = append(errs, fmt.Sprintf("invalid phase %q (must be planning|building|fixing|polishing)", s.Status.Phase))
	}
	for i, t := range s.Tasks {
		if !validTaskStatuses[t.Status] {
			errs = append(errs, fmt.Sprintf("task[%d] %q has invalid status %q", i, t.Name, t.Status))
		}
		if t.Status == "blocked" && t.BlockedReason == "" {
			errs = append(errs, fmt.Sprintf("task[%d] %q is blocked but has no blocked_reason", i, t.Name))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("state validation failed:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// RemainingTasks returns count of tasks not in "done" status.
func (s State) RemainingTasks() int {
	count := 0
	for _, t := range s.Tasks {
		if t.Status != "done" {
			count++
		}
	}
	return count
}

// FindTask returns a pointer to the task with the given name, or nil.
func (s *State) FindTask(name string) *Task {
	for i := range s.Tasks {
		if s.Tasks[i].Name == name {
			return &s.Tasks[i]
		}
	}
	return nil
}
```

**Step 2: Write tests**

```go
// internal/ctx/state_test.go
package ctx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadWriteState(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	original := State{
		Project: Project{Name: "test", DocsPath: "docs/"},
		Status:  Status{Phase: "building"},
		Tasks: []Task{
			{Name: "task1", Status: "done"},
			{Name: "task2", Status: "todo"},
		},
		Decisions: []Decision{{What: "use Go", Why: "fast", When: "2026-03-01"}},
		Pitfalls:  []string{"watch out for X"},
	}

	if err := WriteState(dir, original); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	got, err := ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}

	if got.Project.Name != "test" {
		t.Errorf("Project.Name = %q, want %q", got.Project.Name, "test")
	}
	if len(got.Tasks) != 2 {
		t.Errorf("len(Tasks) = %d, want 2", len(got.Tasks))
	}
	if got.Tasks[0].Status != "done" {
		t.Errorf("Tasks[0].Status = %q, want %q", got.Tasks[0].Status, "done")
	}
}

func TestValidateState(t *testing.T) {
	tests := []struct {
		name    string
		state   State
		wantErr bool
	}{
		{
			name:    "valid state",
			state:   State{Project: Project{Name: "test"}, Status: Status{Phase: "building"}},
			wantErr: false,
		},
		{
			name:    "missing project name",
			state:   State{Status: Status{Phase: "building"}},
			wantErr: true,
		},
		{
			name:    "invalid phase",
			state:   State{Project: Project{Name: "test"}, Status: Status{Phase: "invalid"}},
			wantErr: true,
		},
		{
			name: "invalid task status",
			state: State{
				Project: Project{Name: "test"},
				Tasks:   []Task{{Name: "t", Status: "invalid"}},
			},
			wantErr: true,
		},
		{
			name: "blocked without reason",
			state: State{
				Project: Project{Name: "test"},
				Tasks:   []Task{{Name: "t", Status: "blocked"}},
			},
			wantErr: true,
		},
		{
			name: "blocked with reason",
			state: State{
				Project: Project{Name: "test"},
				Tasks:   []Task{{Name: "t", Status: "blocked", BlockedReason: "waiting"}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateState(tt.state)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateState() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRemainingTasks(t *testing.T) {
	s := State{
		Tasks: []Task{
			{Name: "a", Status: "done"},
			{Name: "b", Status: "todo"},
			{Name: "c", Status: "in-progress"},
			{Name: "d", Status: "done"},
		},
	}
	if got := s.RemainingTasks(); got != 2 {
		t.Errorf("RemainingTasks() = %d, want 2", got)
	}
}

func TestFindTask(t *testing.T) {
	s := State{
		Tasks: []Task{
			{Name: "first", Status: "todo"},
			{Name: "second", Status: "done"},
		},
	}
	task := s.FindTask("second")
	if task == nil {
		t.Fatal("FindTask returned nil")
	}
	if task.Status != "done" {
		t.Errorf("task.Status = %q, want %q", task.Status, "done")
	}
	if s.FindTask("nonexistent") != nil {
		t.Error("FindTask should return nil for nonexistent task")
	}
}
```

**Step 3: Add yaml.v3 dependency**

Run: `go get gopkg.in/yaml.v3@latest`

**Step 4: Run tests**

Run: `go test ./internal/ctx/ -v -run TestReadWriteState`
Expected: PASS

Run: `go test ./internal/ctx/ -v -run TestValidateState`
Expected: PASS

Run: `go test ./internal/ctx/ -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add internal/ctx/state.go internal/ctx/state_test.go go.mod go.sum
git commit -m "feat: add state types with read/write/validate"
```

---

## Task 4: Log Types & Read/Append

**Files:**
- Create: `internal/ctx/log.go`
- Create: `internal/ctx/log_test.go`

**Step 1: Write the log types and operations**

```go
// internal/ctx/log.go
package ctx

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Log struct {
	Sessions []Session `yaml:"sessions"`
}

type Session struct {
	Iteration     int      `yaml:"iteration"`
	Timestamp     string   `yaml:"timestamp"`
	Task          string   `yaml:"task"`
	Outcome       string   `yaml:"outcome"`
	Summary       string   `yaml:"summary"`
	FilesChanged  []string `yaml:"files_changed,omitempty"`
	DecisionsMade []string `yaml:"decisions_made,omitempty"`
	PitfallsFound []string `yaml:"pitfalls_found,omitempty"`
}

func LogPath(dir string) string {
	return filepath.Join(dir, ".ctx", "log.yaml")
}

func ReadLog(dir string) (Log, error) {
	data, err := os.ReadFile(LogPath(dir))
	if err != nil {
		return Log{}, fmt.Errorf("reading log.yaml: %w", err)
	}
	var l Log
	if err := yaml.Unmarshal(data, &l); err != nil {
		return Log{}, fmt.Errorf("parsing log.yaml: %w", err)
	}
	return l, nil
}

func WriteLog(dir string, l Log) error {
	data, err := yaml.Marshal(&l)
	if err != nil {
		return fmt.Errorf("marshaling log: %w", err)
	}
	return os.WriteFile(LogPath(dir), data, 0644)
}

func AppendSession(dir string, sess Session) error {
	l, err := ReadLog(dir)
	if err != nil {
		return err
	}
	l.Sessions = append(l.Sessions, sess)
	return WriteLog(dir, l)
}

// LastNSessions returns the last n sessions, or all if n <= 0 or n > len.
func (l Log) LastNSessions(n int) []Session {
	if n <= 0 || n >= len(l.Sessions) {
		return l.Sessions
	}
	return l.Sessions[len(l.Sessions)-n:]
}

// FailedSessions returns sessions with outcome "blocked" or "unproductive".
func (l Log) FailedSessions() []Session {
	var result []Session
	for _, s := range l.Sessions {
		if s.Outcome == "blocked" || s.Outcome == "unproductive" {
			result = append(result, s)
		}
	}
	return result
}
```

**Step 2: Write tests**

```go
// internal/ctx/log_test.go
package ctx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadWriteLog(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	original := Log{
		Sessions: []Session{
			{Iteration: 1, Task: "setup", Outcome: "done", Summary: "did setup"},
		},
	}

	if err := WriteLog(dir, original); err != nil {
		t.Fatalf("WriteLog: %v", err)
	}

	got, err := ReadLog(dir)
	if err != nil {
		t.Fatalf("ReadLog: %v", err)
	}

	if len(got.Sessions) != 1 {
		t.Fatalf("len(Sessions) = %d, want 1", len(got.Sessions))
	}
	if got.Sessions[0].Task != "setup" {
		t.Errorf("Sessions[0].Task = %q, want %q", got.Sessions[0].Task, "setup")
	}
}

func TestAppendSession(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	// Start with empty log
	if err := WriteLog(dir, Log{Sessions: []Session{}}); err != nil {
		t.Fatal(err)
	}

	// Append two sessions
	if err := AppendSession(dir, Session{Iteration: 1, Task: "first", Outcome: "done"}); err != nil {
		t.Fatal(err)
	}
	if err := AppendSession(dir, Session{Iteration: 2, Task: "second", Outcome: "partial"}); err != nil {
		t.Fatal(err)
	}

	l, err := ReadLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Sessions) != 2 {
		t.Fatalf("len(Sessions) = %d, want 2", len(l.Sessions))
	}
	if l.Sessions[1].Task != "second" {
		t.Errorf("Sessions[1].Task = %q, want %q", l.Sessions[1].Task, "second")
	}
}

func TestLastNSessions(t *testing.T) {
	l := Log{Sessions: []Session{
		{Iteration: 1}, {Iteration: 2}, {Iteration: 3}, {Iteration: 4}, {Iteration: 5},
	}}

	got := l.LastNSessions(3)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Iteration != 3 {
		t.Errorf("first.Iteration = %d, want 3", got[0].Iteration)
	}

	// n=0 returns all
	if len(l.LastNSessions(0)) != 5 {
		t.Error("LastNSessions(0) should return all")
	}

	// n > len returns all
	if len(l.LastNSessions(100)) != 5 {
		t.Error("LastNSessions(100) should return all")
	}
}

func TestFailedSessions(t *testing.T) {
	l := Log{Sessions: []Session{
		{Iteration: 1, Outcome: "done"},
		{Iteration: 2, Outcome: "blocked"},
		{Iteration: 3, Outcome: "partial"},
		{Iteration: 4, Outcome: "unproductive"},
	}}

	got := l.FailedSessions()
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Iteration != 2 {
		t.Errorf("got[0].Iteration = %d, want 2", got[0].Iteration)
	}
	if got[1].Iteration != 4 {
		t.Errorf("got[1].Iteration = %d, want 4", got[1].Iteration)
	}
}
```

**Step 3: Run tests**

Run: `go test ./internal/ctx/ -v`
Expected: all PASS

**Step 4: Commit**

```bash
git add internal/ctx/log.go internal/ctx/log_test.go
git commit -m "feat: add log types with read/append/filter"
```

---

## Task 5: Scaffold & Init Command

**Files:**
- Create: `internal/scaffold/scaffold.go`
- Create: `internal/scaffold/scaffold_test.go`
- Create: `cmd/init.go`

**Step 1: Write the scaffold package**

```go
// internal/scaffold/scaffold.go
package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/winler/golem/internal/ctx"
	"github.com/winler/golem/templates"
)

type InitOptions struct {
	Name     string
	Stack    string
	DocsPath string // default "docs/"
}

type InitResult struct {
	Created []string
	Skipped []string
	Updated []string
}

func Init(dir string, opts InitOptions) (InitResult, error) {
	var result InitResult

	if opts.DocsPath == "" {
		opts.DocsPath = "docs/"
	}

	// Create .ctx/ directory
	ctxDir := filepath.Join(dir, ".ctx")
	if err := os.MkdirAll(ctxDir, 0755); err != nil {
		return result, fmt.Errorf("creating .ctx/: %w", err)
	}

	// Write template files (skip if they exist)
	templateFiles := map[string]string{
		"state.yaml":       "state.yaml",
		"log.yaml":         "log.yaml",
		"prompt.md":        "prompt.md",
		"review-prompt.md": "review-prompt.md",
	}

	for destName, tmplName := range templateFiles {
		destPath := filepath.Join(ctxDir, destName)
		if _, err := os.Stat(destPath); err == nil {
			result.Skipped = append(result.Skipped, ".ctx/"+destName)
			continue
		}

		data, err := templates.FS.ReadFile(tmplName)
		if err != nil {
			return result, fmt.Errorf("reading template %s: %w", tmplName, err)
		}

		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return result, fmt.Errorf("writing %s: %w", destPath, err)
		}
		result.Created = append(result.Created, ".ctx/"+destName)
	}

	// Pre-fill state.yaml with options
	state, err := ctx.ReadState(dir)
	if err != nil {
		return result, fmt.Errorf("reading state for pre-fill: %w", err)
	}
	if opts.Name != "" {
		state.Project.Name = opts.Name
	}
	if opts.Stack != "" {
		state.Project.Stack = opts.Stack
	}
	state.Project.DocsPath = opts.DocsPath
	if err := ctx.WriteState(dir, state); err != nil {
		return result, fmt.Errorf("writing state: %w", err)
	}

	// Inject CLAUDE.md section
	action, err := injectClaudeMD(dir)
	if err != nil {
		return result, fmt.Errorf("injecting CLAUDE.md: %w", err)
	}
	result.Updated = append(result.Updated, "CLAUDE.md ("+action+")")

	return result, nil
}

func injectClaudeMD(dir string) (string, error) {
	section, err := templates.FS.ReadFile("claude.md")
	if err != nil {
		return "", fmt.Errorf("reading claude.md template: %w", err)
	}
	sectionStr := string(section)

	claudePath := filepath.Join(dir, "CLAUDE.md")
	data, err := os.ReadFile(claudePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create new file with just the section
			return "created", os.WriteFile(claudePath, section, 0644)
		}
		return "", err
	}

	content := string(data)
	startMarker := "<!-- golem:start -->"
	endMarker := "<!-- golem:end -->"

	startIdx := strings.Index(content, startMarker)
	endIdx := strings.Index(content, endMarker)

	if startIdx >= 0 && endIdx >= 0 {
		// Replace between markers (inclusive)
		newContent := content[:startIdx] + sectionStr + content[endIdx+len(endMarker):]
		return "updated", os.WriteFile(claudePath, []byte(newContent), 0644)
	}

	// Append to existing file
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "\n" + sectionStr
	return "appended", os.WriteFile(claudePath, []byte(content), 0644)
}

// CtxExists checks if the .ctx/ directory exists.
func CtxExists(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".ctx"))
	return err == nil && info.IsDir()
}
```

**Step 2: Write scaffold tests**

```go
// internal/scaffold/scaffold_test.go
package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/winler/golem/internal/ctx"
)

func TestInit(t *testing.T) {
	dir := t.TempDir()

	result, err := Init(dir, InitOptions{
		Name:     "TestProject",
		Stack:    "Go",
		DocsPath: "docs/plans",
	})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Check files created
	if len(result.Created) != 4 {
		t.Errorf("Created %d files, want 4: %v", len(result.Created), result.Created)
	}

	// Check state.yaml was pre-filled
	state, err := ctx.ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if state.Project.Name != "TestProject" {
		t.Errorf("Project.Name = %q, want %q", state.Project.Name, "TestProject")
	}
	if state.Project.DocsPath != "docs/plans" {
		t.Errorf("DocsPath = %q, want %q", state.Project.DocsPath, "docs/plans")
	}

	// Check CLAUDE.md created
	claudeData, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("reading CLAUDE.md: %v", err)
	}
	if !strings.Contains(string(claudeData), "<!-- golem:start -->") {
		t.Error("CLAUDE.md missing golem markers")
	}
}

func TestInitIdempotent(t *testing.T) {
	dir := t.TempDir()

	// First init
	_, err := Init(dir, InitOptions{Name: "First"})
	if err != nil {
		t.Fatalf("first Init: %v", err)
	}

	// Modify state to verify it's preserved
	state, _ := ctx.ReadState(dir)
	state.Tasks = []ctx.Task{{Name: "existing", Status: "done"}}
	ctx.WriteState(dir, state)

	// Second init — should skip existing files
	result, err := Init(dir, InitOptions{Name: "Second"})
	if err != nil {
		t.Fatalf("second Init: %v", err)
	}

	// state.yaml should be in skipped (not overwritten)
	hasSkipped := false
	for _, s := range result.Skipped {
		if s == ".ctx/state.yaml" {
			hasSkipped = true
		}
	}
	if !hasSkipped {
		t.Error("second init should skip existing state.yaml")
	}

	// But the state should still have our task (file wasn't overwritten)
	state2, _ := ctx.ReadState(dir)
	if len(state2.Tasks) != 1 {
		t.Errorf("existing tasks lost after second init")
	}
}

func TestInjectClaudeMDReplace(t *testing.T) {
	dir := t.TempDir()
	claudePath := filepath.Join(dir, "CLAUDE.md")

	// Create CLAUDE.md with existing markers and surrounding content
	existing := "# My Project\n\nSome content.\n\n<!-- golem:start -->\nold section\n<!-- golem:end -->\n\nMore content.\n"
	os.WriteFile(claudePath, []byte(existing), 0644)

	action, err := injectClaudeMD(dir)
	if err != nil {
		t.Fatalf("injectClaudeMD: %v", err)
	}
	if action != "updated" {
		t.Errorf("action = %q, want %q", action, "updated")
	}

	data, _ := os.ReadFile(claudePath)
	content := string(data)
	if !strings.Contains(content, "# My Project") {
		t.Error("lost content before markers")
	}
	if !strings.Contains(content, "More content.") {
		t.Error("lost content after markers")
	}
	if strings.Contains(content, "old section") {
		t.Error("old section should have been replaced")
	}
	if !strings.Contains(content, "Context Engineering") {
		t.Error("new section not injected")
	}
}

func TestCtxExists(t *testing.T) {
	dir := t.TempDir()
	if CtxExists(dir) {
		t.Error("CtxExists should be false before init")
	}
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)
	if !CtxExists(dir) {
		t.Error("CtxExists should be true after creating .ctx/")
	}
}
```

**Step 3: Run tests**

Run: `go test ./internal/scaffold/ -v`
Expected: all PASS

**Step 4: Write cmd/init.go**

```go
// cmd/init.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/winler/golem/internal/scaffold"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize .ctx/ directory and CLAUDE.md",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		name, _ := cmd.Flags().GetString("name")
		stack, _ := cmd.Flags().GetString("stack")
		docs, _ := cmd.Flags().GetString("docs")

		result, err := scaffold.Init(dir, scaffold.InitOptions{
			Name:     name,
			Stack:    stack,
			DocsPath: docs,
		})
		if err != nil {
			return err
		}

		fmt.Printf("Initialized .ctx/ in %s\n", dir)
		for _, f := range result.Created {
			fmt.Printf("  created %s\n", f)
		}
		for _, f := range result.Skipped {
			fmt.Printf("  skipped %s (already exists)\n", f)
		}
		for _, f := range result.Updated {
			fmt.Printf("  %s\n", f)
		}
		fmt.Println("\nRun `golem plan` to start an interactive planning session.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().String("name", "", "project name")
	initCmd.Flags().String("stack", "", "tech stack")
	initCmd.Flags().String("docs", "docs/", "path to design/implementation docs")
}
```

**Step 5: Build and test manually**

Run: `go build -o golem . && ./golem init --help`
Expected: shows help with --name, --stack, --docs flags

**Step 6: Commit**

```bash
git add internal/scaffold/ cmd/init.go
git commit -m "feat: add golem init with .ctx/ scaffolding and CLAUDE.md injection"
```

---

## Task 6: Display Package & Status Command

**Files:**
- Create: `internal/display/display.go`
- Create: `internal/display/display_test.go`
- Create: `cmd/status.go`

**Step 1: Write the display package**

```go
// internal/display/display.go
package display

import (
	"fmt"
	"io"
	"strings"

	"github.com/winler/golem/internal/ctx"
)

func PrintStatus(w io.Writer, state ctx.State, logEntries int) {
	fmt.Fprintf(w, "Project: %s\n", state.Project.Name)
	fmt.Fprintf(w, "Phase: %s\n", state.Status.Phase)
	if state.Status.CurrentFocus != "" {
		fmt.Fprintf(w, "Focus: %s\n", state.Status.CurrentFocus)
	}

	if len(state.Tasks) > 0 {
		fmt.Fprintln(w, "\nTasks:")
		for _, t := range state.Tasks {
			icon := taskIcon(t.Status)
			line := fmt.Sprintf("  %s %s", icon, t.Name)
			if t.DependsOn != "" {
				line += fmt.Sprintf(" (depends on: %s)", t.DependsOn)
			}
			if t.Status == "in-progress" && t.Notes != "" {
				line += fmt.Sprintf(" — %q", t.Notes)
			}
			if t.Status == "blocked" && t.BlockedReason != "" {
				line += fmt.Sprintf(" — blocked: %q", t.BlockedReason)
			}
			fmt.Fprintln(w, line)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Decisions: %d recorded\n", len(state.Decisions))
	fmt.Fprintf(w, "Pitfalls: %d noted\n", len(state.Pitfalls))
	fmt.Fprintf(w, "Locked paths: %d\n", len(state.Locked))
	fmt.Fprintf(w, "Sessions: %d logged\n", logEntries)
}

func taskIcon(status string) string {
	switch status {
	case "done":
		return "✓"
	case "in-progress":
		return "◐"
	case "todo":
		return "○"
	case "blocked":
		return "✗"
	default:
		return "?"
	}
}

func PrintLog(w io.Writer, sessions []ctx.Session) {
	if len(sessions) == 0 {
		fmt.Fprintln(w, "No sessions logged.")
		return
	}
	for _, s := range sessions {
		ts := s.Timestamp
		if len(ts) >= 16 {
			ts = ts[:16] // trim to YYYY-MM-DDTHH:MM
		}
		ts = strings.Replace(ts, "T", " ", 1)
		fmt.Fprintf(w, "#%-3d %-16s %-14s %q\n", s.Iteration, ts, s.Outcome, s.Task)
	}
}

func PrintDecisions(w io.Writer, decisions []ctx.Decision) {
	if len(decisions) == 0 {
		fmt.Fprintln(w, "No decisions recorded.")
		return
	}
	for i, d := range decisions {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "%s  %s\n", d.When, d.What)
		fmt.Fprintf(w, "            → %s\n", d.Why)
	}
}

func PrintPitfalls(w io.Writer, pitfalls []string) {
	if len(pitfalls) == 0 {
		fmt.Fprintln(w, "No pitfalls noted.")
		return
	}
	for _, p := range pitfalls {
		fmt.Fprintf(w, "• %s\n", p)
	}
}
```

**Step 2: Write display tests**

```go
// internal/display/display_test.go
package display

import (
	"bytes"
	"strings"
	"testing"

	"github.com/winler/golem/internal/ctx"
)

func TestPrintStatus(t *testing.T) {
	var buf bytes.Buffer
	state := ctx.State{
		Project: ctx.Project{Name: "TestProject"},
		Status:  ctx.Status{Phase: "building", CurrentFocus: "auth module"},
		Tasks: []ctx.Task{
			{Name: "auth module", Status: "done"},
			{Name: "API endpoints", Status: "in-progress", Notes: "half done"},
			{Name: "frontend", Status: "todo", DependsOn: "API endpoints"},
			{Name: "deploy", Status: "blocked", BlockedReason: "need CI"},
		},
		Decisions: []ctx.Decision{{What: "d1"}, {What: "d2"}},
		Pitfalls:  []string{"p1"},
		Locked:    []ctx.Lock{{Path: "src/auth/"}},
	}

	PrintStatus(&buf, state, 5)
	out := buf.String()

	if !strings.Contains(out, "Project: TestProject") {
		t.Error("missing project name")
	}
	if !strings.Contains(out, "✓ auth module") {
		t.Error("missing done icon")
	}
	if !strings.Contains(out, "◐ API endpoints") {
		t.Error("missing in-progress icon")
	}
	if !strings.Contains(out, "○ frontend") {
		t.Error("missing todo icon")
	}
	if !strings.Contains(out, "✗ deploy") {
		t.Error("missing blocked icon")
	}
	if !strings.Contains(out, "Decisions: 2") {
		t.Error("wrong decision count")
	}
	if !strings.Contains(out, "Sessions: 5") {
		t.Error("wrong session count")
	}
}

func TestPrintLog(t *testing.T) {
	var buf bytes.Buffer
	sessions := []ctx.Session{
		{Iteration: 1, Timestamp: "2026-03-01T14:30:00Z", Outcome: "done", Task: "auth"},
		{Iteration: 2, Timestamp: "2026-03-01T15:00:00Z", Outcome: "partial", Task: "API"},
	}

	PrintLog(&buf, sessions)
	out := buf.String()

	if !strings.Contains(out, "#1") {
		t.Error("missing iteration 1")
	}
	if !strings.Contains(out, "done") {
		t.Error("missing outcome")
	}
	if !strings.Contains(out, `"auth"`) {
		t.Error("missing task name")
	}
}

func TestPrintDecisions(t *testing.T) {
	var buf bytes.Buffer
	decisions := []ctx.Decision{
		{What: "Use Go", Why: "fast and simple", When: "2026-03-01"},
	}

	PrintDecisions(&buf, decisions)
	out := buf.String()

	if !strings.Contains(out, "Use Go") {
		t.Error("missing decision")
	}
	if !strings.Contains(out, "fast and simple") {
		t.Error("missing rationale")
	}
}

func TestPrintPitfalls(t *testing.T) {
	var buf bytes.Buffer
	PrintPitfalls(&buf, []string{"avoid X", "watch for Y"})
	out := buf.String()

	if !strings.Contains(out, "• avoid X") {
		t.Error("missing pitfall")
	}
}
```

**Step 3: Run tests**

Run: `go test ./internal/display/ -v`
Expected: all PASS

**Step 4: Write cmd/status.go**

```go
// cmd/status.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/winler/golem/internal/ctx"
	"github.com/winler/golem/internal/display"
	"github.com/winler/golem/internal/scaffold"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current project state",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		state, err := ctx.ReadState(dir)
		if err != nil {
			return err
		}

		log, err := ctx.ReadLog(dir)
		if err != nil {
			return err
		}

		display.PrintStatus(os.Stdout, state, len(log.Sessions))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
```

**Step 5: Commit**

```bash
git add internal/display/ cmd/status.go
git commit -m "feat: add display package and golem status command"
```

---

## Task 7: Log, Decisions, Pitfalls Commands

**Files:**
- Create: `cmd/log.go`
- Create: `cmd/decisions.go`
- Create: `cmd/pitfalls.go`

**Step 1: Write cmd/log.go**

```go
// cmd/log.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/winler/golem/internal/ctx"
	"github.com/winler/golem/internal/display"
	"github.com/winler/golem/internal/scaffold"
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Show iteration history",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		l, err := ctx.ReadLog(dir)
		if err != nil {
			return err
		}

		failures, _ := cmd.Flags().GetBool("failures")
		last, _ := cmd.Flags().GetInt("last")

		sessions := l.Sessions
		if failures {
			sessions = l.FailedSessions()
		}
		if last > 0 {
			log := ctx.Log{Sessions: sessions}
			sessions = log.LastNSessions(last)
		}

		display.PrintLog(os.Stdout, sessions)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logCmd)
	logCmd.Flags().Int("last", 0, "show only the last N entries")
	logCmd.Flags().Bool("failures", false, "show only blocked/unproductive sessions")
}
```

**Step 2: Write cmd/decisions.go**

```go
// cmd/decisions.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/winler/golem/internal/ctx"
	"github.com/winler/golem/internal/display"
	"github.com/winler/golem/internal/scaffold"
)

var decisionsCmd = &cobra.Command{
	Use:   "decisions",
	Short: "List architectural decisions",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		state, err := ctx.ReadState(dir)
		if err != nil {
			return err
		}

		display.PrintDecisions(os.Stdout, state.Decisions)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(decisionsCmd)
}
```

**Step 3: Write cmd/pitfalls.go**

```go
// cmd/pitfalls.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/winler/golem/internal/ctx"
	"github.com/winler/golem/internal/display"
	"github.com/winler/golem/internal/scaffold"
)

var pitfallsCmd = &cobra.Command{
	Use:   "pitfalls",
	Short: "List discovered pitfalls",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		state, err := ctx.ReadState(dir)
		if err != nil {
			return err
		}

		display.PrintPitfalls(os.Stdout, state.Pitfalls)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pitfallsCmd)
}
```

**Step 4: Build and verify**

Run: `go build -o golem . && ./golem log --help && ./golem decisions --help && ./golem pitfalls --help`
Expected: each shows its help text

**Step 5: Commit**

```bash
git add cmd/log.go cmd/decisions.go cmd/pitfalls.go
git commit -m "feat: add golem log, decisions, and pitfalls commands"
```

---

## Task 8: State Manipulation Commands (lock, add-task, block)

**Files:**
- Create: `cmd/lock.go`
- Create: `cmd/addtask.go`
- Create: `cmd/block.go`

**Step 1: Write cmd/lock.go**

```go
// cmd/lock.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/winler/golem/internal/ctx"
	"github.com/winler/golem/internal/scaffold"
)

var lockCmd = &cobra.Command{
	Use:   "lock <path>",
	Short: "Lock a path to prevent agent modification",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		state, err := ctx.ReadState(dir)
		if err != nil {
			return err
		}

		note, _ := cmd.Flags().GetString("note")
		path := args[0]

		// Check for duplicates
		for _, l := range state.Locked {
			if l.Path == path {
				return fmt.Errorf("path %q is already locked", path)
			}
		}

		state.Locked = append(state.Locked, ctx.Lock{Path: path, Note: note})
		if err := ctx.WriteState(dir, state); err != nil {
			return err
		}

		fmt.Printf("Locked: %s\n", path)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(lockCmd)
	lockCmd.Flags().String("note", "", "reason for locking")
}
```

**Step 2: Write cmd/addtask.go**

```go
// cmd/addtask.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/winler/golem/internal/ctx"
	"github.com/winler/golem/internal/scaffold"
)

var addTaskCmd = &cobra.Command{
	Use:   "add-task <description>",
	Short: "Add a task to state.yaml",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		state, err := ctx.ReadState(dir)
		if err != nil {
			return err
		}

		dependsOn, _ := cmd.Flags().GetString("depends-on")

		task := ctx.Task{
			Name:      args[0],
			Status:    "todo",
			DependsOn: dependsOn,
		}
		state.Tasks = append(state.Tasks, task)
		if err := ctx.WriteState(dir, state); err != nil {
			return err
		}

		fmt.Printf("Added task: %s\n", args[0])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(addTaskCmd)
	addTaskCmd.Flags().String("depends-on", "", "task this depends on")
}
```

**Step 3: Write cmd/block.go**

```go
// cmd/block.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/winler/golem/internal/ctx"
	"github.com/winler/golem/internal/scaffold"
)

var blockCmd = &cobra.Command{
	Use:   "block <task-name> <reason>",
	Short: "Mark a task as blocked",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		state, err := ctx.ReadState(dir)
		if err != nil {
			return err
		}

		task := state.FindTask(args[0])
		if task == nil {
			return fmt.Errorf("task %q not found", args[0])
		}

		task.Status = "blocked"
		task.BlockedReason = args[1]

		if err := ctx.WriteState(dir, state); err != nil {
			return err
		}

		fmt.Printf("Blocked: %s — %q\n", args[0], args[1])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(blockCmd)
}
```

**Step 4: Build and verify**

Run: `go build -o golem . && ./golem lock --help && ./golem add-task --help && ./golem block --help`
Expected: each shows help text

**Step 5: Commit**

```bash
git add cmd/lock.go cmd/addtask.go cmd/block.go
git commit -m "feat: add lock, add-task, and block commands"
```

---

## Task 9: Prompt Rendering

**Files:**
- Create: `internal/runner/prompt.go`
- Create: `internal/runner/prompt_test.go`

**Step 1: Write the prompt renderer**

```go
// internal/runner/prompt.go
package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RenderPrompt reads a prompt template from disk and replaces template variables.
func RenderPrompt(dir string, templateFile string, vars PromptVars) (string, error) {
	tmplPath := filepath.Join(dir, ".ctx", templateFile)
	data, err := os.ReadFile(tmplPath)
	if err != nil {
		return "", fmt.Errorf("reading prompt template %s: %w", templateFile, err)
	}

	content := string(data)
	content = strings.ReplaceAll(content, "{{DOCS_PATH}}", vars.DocsPath)
	content = strings.ReplaceAll(content, "{{ITERATION_CONTEXT}}", vars.IterationContext)
	content = strings.ReplaceAll(content, "{{TASK_OVERRIDE}}", vars.TaskOverride)

	return content, nil
}

type PromptVars struct {
	DocsPath         string
	IterationContext string
	TaskOverride     string
}

// BuildIterationContext generates the iteration context string.
func BuildIterationContext(iteration, maxIterations, tasksRemaining int) string {
	ctx := fmt.Sprintf("You are on iteration %d of %d. There are %d tasks remaining.", iteration, maxIterations, tasksRemaining)
	if float64(iteration)/float64(maxIterations) > 0.7 {
		ctx += "\nIf you are running low on iterations, prioritize finishing in-progress tasks cleanly over starting new ones."
	}
	return ctx
}

// BuildTaskOverride generates the task override string for --task flag.
func BuildTaskOverride(taskName string) string {
	if taskName == "" {
		return ""
	}
	return fmt.Sprintf("IMPORTANT: You MUST work on the following task this iteration: %q\nDo not pick a different task.\n", taskName)
}
```

**Step 2: Write tests**

```go
// internal/runner/prompt_test.go
package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderPrompt(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	tmpl := "Docs at {{DOCS_PATH}}.\n{{ITERATION_CONTEXT}}\n{{TASK_OVERRIDE}}"
	os.WriteFile(filepath.Join(dir, ".ctx", "prompt.md"), []byte(tmpl), 0644)

	result, err := RenderPrompt(dir, "prompt.md", PromptVars{
		DocsPath:         "docs/plans",
		IterationContext: "Iteration 3 of 10. 5 tasks remaining.",
		TaskOverride:     "",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Docs at docs/plans") {
		t.Error("DOCS_PATH not replaced")
	}
	if !strings.Contains(result, "Iteration 3 of 10") {
		t.Error("ITERATION_CONTEXT not replaced")
	}
	// TaskOverride empty => replaced with empty string
	if strings.Contains(result, "{{TASK_OVERRIDE}}") {
		t.Error("TASK_OVERRIDE not replaced")
	}
}

func TestBuildIterationContext(t *testing.T) {
	// Not low on iterations
	ctx := BuildIterationContext(3, 20, 8)
	if !strings.Contains(ctx, "iteration 3 of 20") {
		t.Error("missing iteration info")
	}
	if strings.Contains(ctx, "running low") {
		t.Error("should not warn when not low")
	}

	// Low on iterations (>70%)
	ctx = BuildIterationContext(15, 20, 3)
	if !strings.Contains(ctx, "running low") {
		t.Error("should warn when low on iterations")
	}
}

func TestBuildTaskOverride(t *testing.T) {
	if got := BuildTaskOverride(""); got != "" {
		t.Errorf("empty task should produce empty string, got %q", got)
	}

	got := BuildTaskOverride("fix auth")
	if !strings.Contains(got, "fix auth") {
		t.Error("task name not in override")
	}
	if !strings.Contains(got, "MUST work on") {
		t.Error("missing MUST directive")
	}
}
```

**Step 3: Run tests**

Run: `go test ./internal/runner/ -v`
Expected: all PASS

**Step 4: Commit**

```bash
git add internal/runner/prompt.go internal/runner/prompt_test.go
git commit -m "feat: add prompt template rendering with variable substitution"
```

---

## Task 10: Git Diff for Locked Path Detection

**Files:**
- Create: `internal/git/git.go`
- Create: `internal/git/git_test.go`

**Step 1: Write the git package**

```go
// internal/git/git.go
package git

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// ChangedFiles returns the list of files changed in the most recent commit.
// Returns empty slice if not in a git repo or no commits.
func ChangedFiles(dir string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", "HEAD~1", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		// Not a git repo or no previous commit — not an error for us
		return nil, nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// CheckLockedPaths returns files that were changed under locked paths.
func CheckLockedPaths(changedFiles []string, lockedPaths []string) []string {
	var violations []string
	for _, file := range changedFiles {
		for _, locked := range lockedPaths {
			// Normalize: ensure locked path ends with / for directory matching
			locked = strings.TrimSuffix(locked, "/") + "/"
			if strings.HasPrefix(file, locked) || file == strings.TrimSuffix(locked, "/") {
				violations = append(violations, file)
				break
			}
		}
	}
	return violations
}

// HasUncommittedChanges checks if there are uncommitted changes in .ctx/.
func HasUncommittedChanges(dir string, path string) bool {
	cmd := exec.Command("git", "diff", "--name-only", "--", path)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// IsGitRepo checks if the directory is inside a git repository.
func IsGitRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	return cmd.Run() == nil
}

// StateFileModified checks if .ctx/state.yaml was modified (staged or unstaged).
func StateFileModified(dir string) bool {
	statePath := filepath.Join(".ctx", "state.yaml")
	// Check unstaged
	cmd := exec.Command("git", "diff", "--name-only", "--", statePath)
	cmd.Dir = dir
	out, _ := cmd.Output()
	if strings.TrimSpace(string(out)) != "" {
		return true
	}
	// Check staged
	cmd = exec.Command("git", "diff", "--cached", "--name-only", "--", statePath)
	cmd.Dir = dir
	out, _ = cmd.Output()
	return strings.TrimSpace(string(out)) != ""
}
```

**Step 2: Write tests (unit-testable parts only)**

```go
// internal/git/git_test.go
package git

import (
	"testing"
)

func TestCheckLockedPaths(t *testing.T) {
	tests := []struct {
		name     string
		changed  []string
		locked   []string
		wantLen  int
	}{
		{
			name:    "no violations",
			changed: []string{"src/main.go", "tests/main_test.go"},
			locked:  []string{"src/auth/"},
			wantLen: 0,
		},
		{
			name:    "file under locked path",
			changed: []string{"src/auth/handler.go", "src/main.go"},
			locked:  []string{"src/auth/"},
			wantLen: 1,
		},
		{
			name:    "multiple violations",
			changed: []string{"src/auth/handler.go", "src/auth/middleware.go", "src/main.go"},
			locked:  []string{"src/auth/"},
			wantLen: 2,
		},
		{
			name:    "locked path without trailing slash",
			changed: []string{"src/auth/handler.go"},
			locked:  []string{"src/auth"},
			wantLen: 1,
		},
		{
			name:    "multiple locked paths",
			changed: []string{"src/auth/handler.go", "src/db/schema.go", "src/main.go"},
			locked:  []string{"src/auth/", "src/db/"},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckLockedPaths(tt.changed, tt.locked)
			if len(got) != tt.wantLen {
				t.Errorf("CheckLockedPaths() returned %d violations, want %d: %v", len(got), tt.wantLen, got)
			}
		})
	}
}
```

**Step 3: Run tests**

Run: `go test ./internal/git/ -v`
Expected: all PASS

**Step 4: Commit**

```bash
git add internal/git/
git commit -m "feat: add git diff helpers for locked path detection"
```

---

## Task 11: Post-Iteration Validation

**Files:**
- Create: `internal/runner/validate.go`
- Create: `internal/runner/validate_test.go`

**Step 1: Write the validation logic**

```go
// internal/runner/validate.go
package runner

import (
	"fmt"

	"github.com/winler/golem/internal/ctx"
	gitpkg "github.com/winler/golem/internal/git"
)

type ValidationResult struct {
	Halted   bool     // If true, the loop should stop
	Warnings []string // Non-fatal warnings to print
}

// ValidatePostIteration runs all post-iteration checks.
func ValidatePostIteration(dir string, stateBefore, stateAfter ctx.State, log ctx.Log) ValidationResult {
	var result ValidationResult

	// 1. Schema validation
	if err := ctx.ValidateState(stateAfter); err != nil {
		result.Halted = true
		result.Warnings = append(result.Warnings, fmt.Sprintf("FATAL: state.yaml validation failed: %v", err))
		return result
	}

	// 2. Locked path violation detection
	lockedPaths := make([]string, len(stateAfter.Locked))
	for i, l := range stateAfter.Locked {
		lockedPaths[i] = l.Path
	}
	if len(lockedPaths) > 0 {
		changedFiles, err := gitpkg.ChangedFiles(dir)
		if err == nil && len(changedFiles) > 0 {
			violations := gitpkg.CheckLockedPaths(changedFiles, lockedPaths)
			for _, v := range violations {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("WARNING — modified %s which is under a locked path", v))
			}
		}
	}

	// 3. Task regression detection
	beforeStatuses := make(map[string]string)
	for _, t := range stateBefore.Tasks {
		beforeStatuses[t.Name] = t.Status
	}
	for _, t := range stateAfter.Tasks {
		prev, exists := beforeStatuses[t.Name]
		if exists && prev == "done" && t.Status != "done" {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("WARNING — task %q regressed from done to %s", t.Name, t.Status))
		}
	}

	// 4. Thrashing detection
	thrashing := detectThrashing(log)
	for _, taskName := range thrashing {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("WARNING — task %q has been in-progress for 3+ consecutive iterations", taskName))
	}

	return result
}

// detectThrashing checks if any task has been the subject of 3+ consecutive
// sessions in the log.
func detectThrashing(l ctx.Log) []string {
	if len(l.Sessions) < 3 {
		return nil
	}

	var thrashing []string
	last3 := l.Sessions[len(l.Sessions)-3:]

	// Check if the same task appears in the last 3 consecutive entries
	task := last3[0].Task
	if task != "" && last3[1].Task == task && last3[2].Task == task {
		thrashing = append(thrashing, task)
	}

	return thrashing
}
```

**Step 2: Write tests**

```go
// internal/runner/validate_test.go
package runner

import (
	"strings"
	"testing"

	"github.com/winler/golem/internal/ctx"
)

func TestValidatePostIteration_SchemaFailure(t *testing.T) {
	// State with invalid task status should halt
	before := ctx.State{Project: ctx.Project{Name: "test"}}
	after := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks:   []ctx.Task{{Name: "bad", Status: "invalid"}},
	}

	result := ValidatePostIteration(t.TempDir(), before, after, ctx.Log{})
	if !result.Halted {
		t.Error("should halt on schema validation failure")
	}
}

func TestValidatePostIteration_TaskRegression(t *testing.T) {
	before := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks: []ctx.Task{
			{Name: "auth", Status: "done"},
			{Name: "api", Status: "in-progress"},
		},
	}
	after := ctx.State{
		Project: ctx.Project{Name: "test"},
		Tasks: []ctx.Task{
			{Name: "auth", Status: "in-progress"}, // regression!
			{Name: "api", Status: "done"},
		},
	}

	result := ValidatePostIteration(t.TempDir(), before, after, ctx.Log{})
	if result.Halted {
		t.Error("regression should warn, not halt")
	}

	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "auth") && strings.Contains(w, "regressed") {
			found = true
		}
	}
	if !found {
		t.Error("should warn about task regression")
	}
}

func TestDetectThrashing(t *testing.T) {
	// 3 consecutive sessions on the same task
	l := ctx.Log{Sessions: []ctx.Session{
		{Iteration: 1, Task: "payment"},
		{Iteration: 2, Task: "payment"},
		{Iteration: 3, Task: "payment"},
	}}

	thrashing := detectThrashing(l)
	if len(thrashing) != 1 || thrashing[0] != "payment" {
		t.Errorf("expected thrashing on 'payment', got %v", thrashing)
	}

	// Different tasks — no thrashing
	l2 := ctx.Log{Sessions: []ctx.Session{
		{Iteration: 1, Task: "auth"},
		{Iteration: 2, Task: "payment"},
		{Iteration: 3, Task: "auth"},
	}}
	if len(detectThrashing(l2)) != 0 {
		t.Error("should not detect thrashing with different tasks")
	}

	// Less than 3 sessions — no thrashing
	l3 := ctx.Log{Sessions: []ctx.Session{
		{Iteration: 1, Task: "payment"},
		{Iteration: 2, Task: "payment"},
	}}
	if len(detectThrashing(l3)) != 0 {
		t.Error("should not detect thrashing with < 3 sessions")
	}
}
```

**Step 3: Run tests**

Run: `go test ./internal/runner/ -v`
Expected: all PASS

**Step 4: Commit**

```bash
git add internal/runner/validate.go internal/runner/validate_test.go
git commit -m "feat: add post-iteration validation (schema, regression, thrashing)"
```

---

## Task 12: Builder Loop

**Files:**
- Create: `internal/runner/builder.go`
- Create: `cmd/run.go`

This is the core loop. It spawns `claude -p` and iterates.

**Step 1: Write the builder loop**

```go
// internal/runner/builder.go
package runner

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/winler/golem/internal/ctx"
)

type BuilderConfig struct {
	Dir           string
	MaxIterations int
	MaxTurns      int
	TaskOverride  string
	DryRun        bool
	Verbose       bool
}

type BuilderResult struct {
	Iterations int
	Duration   time.Duration
	Completed  bool // All tasks done
	Halted     bool // Stopped due to error
	HaltReason string
}

const completePromise = "<promise>COMPLETE</promise>"

func RunBuilderLoop(cfg BuilderConfig) (BuilderResult, error) {
	startTime := time.Now()
	var result BuilderResult

	state, err := ctx.ReadState(cfg.Dir)
	if err != nil {
		return result, fmt.Errorf("reading initial state: %w", err)
	}

	if len(state.Tasks) == 0 {
		return result, fmt.Errorf("no tasks in state.yaml — run `golem plan` first")
	}

	remaining := state.RemainingTasks()
	fmt.Fprintf(os.Stderr, "golem: starting builder loop (max %d iterations)\n", cfg.MaxIterations)
	fmt.Fprintf(os.Stderr, "golem: %d tasks remaining\n\n", remaining)

	for i := 1; i <= cfg.MaxIterations; i++ {
		// Re-read state at start of each iteration
		state, err = ctx.ReadState(cfg.Dir)
		if err != nil {
			result.Halted = true
			result.HaltReason = fmt.Sprintf("reading state before iteration %d: %v", i, err)
			break
		}

		remaining = state.RemainingTasks()
		if remaining == 0 {
			result.Completed = true
			break
		}

		// Render prompt
		iterCtx := BuildIterationContext(i, cfg.MaxIterations, remaining)
		taskOverride := BuildTaskOverride(cfg.TaskOverride)
		prompt, err := RenderPrompt(cfg.Dir, "prompt.md", PromptVars{
			DocsPath:         state.Project.DocsPath,
			IterationContext: iterCtx,
			TaskOverride:     taskOverride,
		})
		if err != nil {
			result.Halted = true
			result.HaltReason = fmt.Sprintf("rendering prompt: %v", err)
			break
		}

		if cfg.DryRun {
			fmt.Fprintf(os.Stderr, "golem: [dry-run] iteration %d would run with prompt:\n%s\n", i, prompt)
			continue
		}

		fmt.Fprintf(os.Stderr, "golem: iteration %d starting...\n", i)
		iterStart := time.Now()

		// Capture state before for regression detection
		stateBefore := state

		// Spawn claude
		output, err := spawnClaude(cfg.Dir, prompt, cfg.MaxTurns)
		iterDuration := time.Since(iterStart)

		if err != nil {
			fmt.Fprintf(os.Stderr, "golem: iteration %d failed (%v) — continuing\n", i, err)
			result.Iterations = i
			continue
		}

		// Check for COMPLETE promise
		if strings.Contains(output, completePromise) {
			result.Completed = true
			result.Iterations = i
			fmt.Fprintf(os.Stderr, "golem: iteration %d complete (%s) — all tasks done\n", i, formatDuration(iterDuration))
			break
		}

		// Post-iteration: re-read state and validate
		stateAfter, readErr := ctx.ReadState(cfg.Dir)
		if readErr != nil {
			result.Halted = true
			result.HaltReason = fmt.Sprintf("state.yaml unreadable after iteration %d: %v", i, readErr)
			result.Iterations = i
			break
		}

		log, _ := ctx.ReadLog(cfg.Dir)

		validation := ValidatePostIteration(cfg.Dir, stateBefore, stateAfter, log)
		for _, w := range validation.Warnings {
			fmt.Fprintf(os.Stderr, "golem: %s\n", w)
		}
		if validation.Halted {
			result.Halted = true
			result.HaltReason = validation.Warnings[0]
			result.Iterations = i
			break
		}

		// Print iteration summary
		lastSession := lastLogSession(log)
		fmt.Fprintf(os.Stderr, "golem: iteration %d complete (%s)\n", i, formatDuration(iterDuration))
		if lastSession != nil {
			fmt.Fprintf(os.Stderr, "golem:   task: %q\n", lastSession.Task)
			fmt.Fprintf(os.Stderr, "golem:   outcome: %s\n", lastSession.Outcome)
			fmt.Fprintf(os.Stderr, "golem:   files changed: %d\n", len(lastSession.FilesChanged))
		}

		result.Iterations = i
	}

	if !result.Completed && !result.Halted && result.Iterations >= cfg.MaxIterations {
		fmt.Fprintf(os.Stderr, "golem: max iterations (%d) reached\n", cfg.MaxIterations)
	}

	result.Duration = time.Since(startTime)

	// Final summary
	state, _ = ctx.ReadState(cfg.Dir)
	remaining = state.RemainingTasks()
	if result.Completed {
		fmt.Fprintf(os.Stderr, "\ngolem: all tasks done! (%d iterations, %s)\n", result.Iterations, formatDuration(result.Duration))
	} else {
		fmt.Fprintf(os.Stderr, "\ngolem: stopped after %d iterations (%s), %d tasks remaining\n", result.Iterations, formatDuration(result.Duration), remaining)
	}

	return result, nil
}

func spawnClaude(dir string, prompt string, maxTurns int) (string, error) {
	args := []string{"-p", prompt, "--max-turns", fmt.Sprintf("%d", maxTurns)}

	cmd := exec.Command("claude", args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr

	// Stream stdout live while also capturing it
	var outputBuf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)

	if err := cmd.Run(); err != nil {
		return outputBuf.String(), fmt.Errorf("claude exited with error: %w", err)
	}

	return outputBuf.String(), nil
}

func lastLogSession(l ctx.Log) *ctx.Session {
	if len(l.Sessions) == 0 {
		return nil
	}
	s := l.Sessions[len(l.Sessions)-1]
	return &s
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%02ds", mins, secs)
}
```

**Step 2: Write cmd/run.go**

```go
// cmd/run.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/winler/golem/internal/runner"
	"github.com/winler/golem/internal/scaffold"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the autonomous builder loop",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		maxIter, _ := cmd.Flags().GetInt("max-iterations")
		maxTurns, _ := cmd.Flags().GetInt("max-turns")
		task, _ := cmd.Flags().GetString("task")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		verbose, _ := cmd.Flags().GetBool("verbose")
		review, _ := cmd.Flags().GetBool("review")

		result, err := runner.RunBuilderLoop(runner.BuilderConfig{
			Dir:           dir,
			MaxIterations: maxIter,
			MaxTurns:      maxTurns,
			TaskOverride:  task,
			DryRun:        dryRun,
			Verbose:       verbose,
		})
		if err != nil {
			return err
		}

		if result.Halted {
			return fmt.Errorf("loop halted: %s", result.HaltReason)
		}

		// Chain review if requested
		if review {
			fmt.Fprintln(os.Stderr, "\ngolem: chaining review pass...")
			return runReview(dir, maxTurns)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().Int("max-iterations", 20, "maximum number of iterations")
	runCmd.Flags().Int("max-turns", 50, "max turns per Claude Code session")
	runCmd.Flags().String("task", "", "force agent to work on a specific task")
	runCmd.Flags().Bool("dry-run", false, "show rendered prompt without executing")
	runCmd.Flags().Bool("verbose", false, "extra output detail")
	runCmd.Flags().Bool("review", false, "run review pass after builder completes")
}
```

**Step 3: Build and verify**

Run: `go build -o golem . && ./golem run --help`
Expected: shows help with all flags

**Step 4: Commit**

```bash
git add internal/runner/builder.go cmd/run.go
git commit -m "feat: add builder loop with iteration management and validation"
```

---

## Task 13: Reviewer

**Files:**
- Create: `internal/runner/reviewer.go`
- Create: `cmd/review.go`

**Step 1: Write the reviewer**

```go
// internal/runner/reviewer.go
package runner

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/winler/golem/internal/ctx"
)

type ReviewResult struct {
	Duration       time.Duration
	Approved       bool
	NeedsWork      bool
	NewReviewTasks int
	OldReviewTasks int
}

const (
	approvedPromise  = "<promise>APPROVED</promise>"
	needsWorkPromise = "<promise>NEEDS_WORK</promise>"
)

func RunReview(dir string, maxTurns int) (ReviewResult, error) {
	var result ReviewResult
	startTime := time.Now()

	// Count existing review tasks
	state, err := ctx.ReadState(dir)
	if err != nil {
		return result, fmt.Errorf("reading state: %w", err)
	}
	result.OldReviewTasks = countReviewTasks(state)

	// Render review prompt
	prompt, err := RenderPrompt(dir, "review-prompt.md", PromptVars{
		DocsPath: state.Project.DocsPath,
	})
	if err != nil {
		return result, fmt.Errorf("rendering review prompt: %w", err)
	}

	fmt.Fprintf(os.Stderr, "golem: starting review...\n")

	output, err := spawnClaude(dir, prompt, maxTurns)
	result.Duration = time.Since(startTime)

	if err != nil {
		return result, fmt.Errorf("review failed: %w", err)
	}

	// Detect result
	result.Approved = strings.Contains(output, approvedPromise)
	result.NeedsWork = strings.Contains(output, needsWorkPromise)

	// Count new review tasks
	stateAfter, err := ctx.ReadState(dir)
	if err != nil {
		return result, fmt.Errorf("reading state after review: %w", err)
	}
	newCount := countReviewTasks(stateAfter)
	result.NewReviewTasks = newCount - result.OldReviewTasks

	// Print results
	fmt.Fprintf(os.Stderr, "golem: review complete (%s)\n", formatDuration(result.Duration))
	if result.Approved {
		fmt.Fprintf(os.Stderr, "golem: result: APPROVED\n")
	} else if result.NeedsWork {
		fmt.Fprintf(os.Stderr, "golem: result: NEEDS_WORK\n")
	} else {
		fmt.Fprintf(os.Stderr, "golem: result: no promise detected (review may have been incomplete)\n")
	}

	if result.NewReviewTasks > 0 {
		fmt.Fprintf(os.Stderr, "golem: %d review tasks added to state.yaml", result.NewReviewTasks)
		if result.OldReviewTasks > 0 {
			fmt.Fprintf(os.Stderr, " (previous review found %d)", result.OldReviewTasks)
		}
		fmt.Fprintln(os.Stderr)

		// Print the new review tasks
		for _, t := range stateAfter.Tasks {
			if strings.HasPrefix(t.Name, "[review]") {
				// Only print if it's new (wasn't in old state)
				if state.FindTask(t.Name) == nil {
					fmt.Fprintf(os.Stderr, "golem:   %s\n", t.Name)
				}
			}
		}
	} else if result.Approved {
		fmt.Fprintf(os.Stderr, "golem: no issues found")
		if result.OldReviewTasks > 0 {
			fmt.Fprintf(os.Stderr, " (previous review found %d)", result.OldReviewTasks)
		}
		fmt.Fprintln(os.Stderr)
	}

	return result, nil
}

func countReviewTasks(state ctx.State) int {
	count := 0
	for _, t := range state.Tasks {
		if strings.HasPrefix(t.Name, "[review]") && t.Status != "done" {
			count++
		}
	}
	return count
}
```

**Step 2: Write cmd/review.go**

```go
// cmd/review.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/winler/golem/internal/runner"
	"github.com/winler/golem/internal/scaffold"
)

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Run a single-pass code review",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		maxTurns, _ := cmd.Flags().GetInt("max-turns")
		return runReview(dir, maxTurns)
	},
}

func runReview(dir string, maxTurns int) error {
	_, err := runner.RunReview(dir, maxTurns)
	return err
}

func init() {
	rootCmd.AddCommand(reviewCmd)
	reviewCmd.Flags().Int("max-turns", 50, "max turns for the review session")
}
```

**Step 3: Build and verify**

Run: `go build -o golem . && ./golem review --help`
Expected: shows help text

**Step 4: Commit**

```bash
git add internal/runner/reviewer.go cmd/review.go
git commit -m "feat: add review command with APPROVED/NEEDS_WORK detection"
```

---

## Task 14: Plan Command

**Files:**
- Create: `cmd/plan.go`

**Step 1: Write cmd/plan.go**

```go
// cmd/plan.go
package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/winler/golem/internal/scaffold"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Open an interactive Claude Code session for planning",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		fmt.Fprintln(os.Stderr, "golem: launching interactive Claude Code session...")
		fmt.Fprintln(os.Stderr, "golem: CLAUDE.md conventions are active — the agent knows about .ctx/")
		fmt.Fprintln(os.Stderr, "golem: exit the session when planning is complete")
		fmt.Fprintln(os.Stderr)

		claude := exec.Command("claude")
		claude.Dir = dir
		claude.Stdin = os.Stdin
		claude.Stdout = os.Stdout
		claude.Stderr = os.Stderr

		return claude.Run()
	},
}

func init() {
	rootCmd.AddCommand(planCmd)
}
```

**Step 2: Build and verify**

Run: `go build -o golem . && ./golem plan --help`
Expected: shows help text

**Step 3: Commit**

```bash
git add cmd/plan.go
git commit -m "feat: add plan command for interactive Claude Code sessions"
```

---

## Task 15: Integration Test & Final Build

**Files:**
- No new files — verify everything works end-to-end

**Step 1: Run all tests**

Run: `go test ./... -v`
Expected: all PASS

**Step 2: Build final binary**

Run: `go build -o golem .`
Expected: compiles successfully

**Step 3: Test golem init in a temp directory**

Run:
```bash
tmpdir=$(mktemp -d)
cd "$tmpdir"
/home/winler/projects/golem/golem init --name "TestProject" --stack "Go" --docs "docs/"
ls -la .ctx/
cat .ctx/state.yaml
cat CLAUDE.md
cd /home/winler/projects/golem
rm -rf "$tmpdir"
```
Expected: `.ctx/` created with all files, state.yaml has project name, CLAUDE.md has golem section

**Step 4: Test golem status with populated state**

Run:
```bash
tmpdir=$(mktemp -d)
cd "$tmpdir"
/home/winler/projects/golem/golem init --name "TestProject"
/home/winler/projects/golem/golem add-task "first task"
/home/winler/projects/golem/golem add-task "second task" --depends-on "first task"
/home/winler/projects/golem/golem lock src/auth/ --note "done and tested"
/home/winler/projects/golem/golem status
cd /home/winler/projects/golem
rm -rf "$tmpdir"
```
Expected: status output shows tasks, locked paths, etc.

**Step 5: Verify golem run --dry-run**

Run:
```bash
tmpdir=$(mktemp -d)
cd "$tmpdir"
/home/winler/projects/golem/golem init --name "TestProject" --docs "docs/"
/home/winler/projects/golem/golem add-task "implement auth"
/home/winler/projects/golem/golem run --dry-run --max-iterations 1
cd /home/winler/projects/golem
rm -rf "$tmpdir"
```
Expected: prints the rendered prompt without executing

**Step 6: Final commit with go install support**

Verify `go install github.com/winler/golem@latest` would work — this requires the `main.go` to be at the module root (it already is).

Run: `go vet ./...`
Expected: no issues

**Step 7: Commit any remaining changes**

```bash
git add -A
git commit -m "feat: finalize Phase 1+2 implementation"
```
