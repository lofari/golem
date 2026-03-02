# TUI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a terminal UI to `golem run` (live output + sidebar) and `golem status` (live-watching dashboard) using Bubbletea/Lipgloss.

**Architecture:** TUI is a display layer wrapping the existing builder loop. The loop emits events through a channel; the TUI consumes them. A `--no-tui` flag preserves plain-text mode. Output streaming uses a custom `io.Writer` on `ClaudeRunner` that sends lines to a channel.

**Tech Stack:** bubbletea, lipgloss, bubbles/viewport, golang.org/x/term

---

### Task 1: Add dependencies

**Files:**
- Modify: `go.mod`

**Step 1: Install bubbletea, lipgloss, bubbles, and x/term**

Run:
```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/charmbracelet/bubbles@latest
go get golang.org/x/term@latest
```

**Step 2: Verify build**

Run: `go build ./...`
Expected: compiles with no errors

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add bubbletea, lipgloss, bubbles, x/term dependencies"
```

---

### Task 2: Create event types

**Files:**
- Create: `internal/tui/events.go`

**Step 1: Create the events file**

```go
// internal/tui/events.go
package tui

import (
	"github.com/lofari/golem/internal/runner"
)

// EventType identifies the kind of TUI event.
type EventType int

const (
	EventIterStart  EventType = iota // Iteration N beginning
	EventOutputLine                  // A line of claude output
	EventIterEnd                     // Iteration N finished
	EventLoopDone                    // Loop finished
)

// Event carries information from the builder loop to the TUI.
type Event struct {
	Type    EventType
	Iter    int                   // EventIterStart, EventIterEnd
	MaxIter int                   // EventIterStart
	Task    string                // EventIterEnd
	Outcome string                // EventIterEnd
	Line    string                // EventOutputLine
	Err     error                 // EventIterEnd (if failed), EventLoopDone
	Result  *runner.BuilderResult // EventLoopDone
}
```

**Step 2: Verify build**

Run: `go build ./...`
Expected: compiles

**Step 3: Commit**

```bash
git add internal/tui/events.go
git commit -m "feat(tui): add event types for builder-to-TUI communication"
```

---

### Task 3: Create lineWriter for output streaming

**Files:**
- Create: `internal/tui/writer.go`
- Create: `internal/tui/writer_test.go`

**Step 1: Write the failing test**

```go
// internal/tui/writer_test.go
package tui

import (
	"testing"
)

func TestLineWriter_CompleteLine(t *testing.T) {
	ch := make(chan string, 10)
	w := NewLineWriter(ch)

	n, err := w.Write([]byte("hello world\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 12 {
		t.Errorf("expected n=12, got %d", n)
	}

	select {
	case line := <-ch:
		if line != "hello world" {
			t.Errorf("expected %q, got %q", "hello world", line)
		}
	default:
		t.Error("expected a line on the channel")
	}
}

func TestLineWriter_MultipleLines(t *testing.T) {
	ch := make(chan string, 10)
	w := NewLineWriter(ch)

	w.Write([]byte("line1\nline2\nline3\n"))

	expected := []string{"line1", "line2", "line3"}
	for _, exp := range expected {
		select {
		case line := <-ch:
			if line != exp {
				t.Errorf("expected %q, got %q", exp, line)
			}
		default:
			t.Errorf("expected line %q on channel", exp)
		}
	}
}

func TestLineWriter_PartialLine(t *testing.T) {
	ch := make(chan string, 10)
	w := NewLineWriter(ch)

	// Write partial line
	w.Write([]byte("hel"))
	select {
	case <-ch:
		t.Error("should not send partial line")
	default:
	}

	// Complete the line
	w.Write([]byte("lo\n"))
	select {
	case line := <-ch:
		if line != "hello" {
			t.Errorf("expected %q, got %q", "hello", line)
		}
	default:
		t.Error("expected completed line on channel")
	}
}

func TestLineWriter_Flush(t *testing.T) {
	ch := make(chan string, 10)
	w := NewLineWriter(ch)

	w.Write([]byte("no newline"))
	w.Flush()

	select {
	case line := <-ch:
		if line != "no newline" {
			t.Errorf("expected %q, got %q", "no newline", line)
		}
	default:
		t.Error("expected flushed line on channel")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -v`
Expected: FAIL (NewLineWriter not defined)

**Step 3: Write minimal implementation**

```go
// internal/tui/writer.go
package tui

import (
	"bytes"
	"strings"
)

// LineWriter is an io.Writer that buffers input and sends complete lines
// (stripped of trailing newline) to a channel.
type LineWriter struct {
	ch  chan<- string
	buf bytes.Buffer
}

// NewLineWriter creates a LineWriter that sends lines to ch.
func NewLineWriter(ch chan<- string) *LineWriter {
	return &LineWriter{ch: ch}
}

func (w *LineWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.buf.Write(p)

	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			// Incomplete line — put it back
			w.buf.WriteString(line)
			break
		}
		w.ch <- strings.TrimRight(line, "\n\r")
	}

	return n, nil
}

// Flush sends any remaining buffered text as a final line.
func (w *LineWriter) Flush() {
	if w.buf.Len() > 0 {
		w.ch <- w.buf.String()
		w.buf.Reset()
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/writer.go internal/tui/writer_test.go
git commit -m "feat(tui): add LineWriter for streaming output to channel"
```

---

### Task 4: Add OutputWriter to ClaudeRunner

**Files:**
- Modify: `internal/runner/command.go`

Currently `ClaudeRunner.Run()` hardcodes `cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)` and `cmd.Stderr = os.Stderr`. We add configurable writers so the TUI can intercept output.

**Step 1: Add fields and update Run()**

In `internal/runner/command.go`, add `OutputWriter` and `ErrWriter` fields to `ClaudeRunner`:

```go
type ClaudeRunner struct {
	Verbose      bool
	OutputWriter io.Writer // stdout destination; defaults to os.Stdout
	ErrWriter    io.Writer // stderr destination; defaults to os.Stderr
}
```

Update `Run()` to use them:

```go
func (c *ClaudeRunner) Run(ctx context.Context, dir string, prompt string, maxTurns int, model string) (string, error) {
	args := []string{"-p", prompt, "--max-turns", fmt.Sprintf("%d", maxTurns)}
	if model != "" {
		args = append(args, "--model", model)
	}
	if c.Verbose {
		args = append(args, "--verbose")
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout := c.OutputWriter
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := c.ErrWriter
	if stderr == nil {
		stderr = os.Stderr
	}

	cmd.Stderr = stderr

	var outputBuf strings.Builder
	cmd.Stdout = io.MultiWriter(stdout, &outputBuf)

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return outputBuf.String(), fmt.Errorf("interrupted: %w", ctx.Err())
		}
		return outputBuf.String(), fmt.Errorf("claude exited with error: %w", err)
	}

	return outputBuf.String(), nil
}
```

**Step 2: Verify build and existing tests pass**

Run: `go build ./... && go test ./internal/runner/ -v`
Expected: compiles, all existing tests pass (mockRunner doesn't use these fields)

**Step 3: Commit**

```bash
git add internal/runner/command.go
git commit -m "feat(runner): add configurable OutputWriter and ErrWriter to ClaudeRunner"
```

---

### Task 5: Add Events channel to BuilderConfig and emit events

**Files:**
- Modify: `internal/runner/builder.go`
- Modify: `internal/runner/builder_test.go`

**Step 1: Write the failing test**

Add to `internal/runner/builder_test.go`:

```go
func TestBuilderLoop_EmitsEvents(t *testing.T) {
	dir := setupTestProject(t)
	mock := &mockRunner{outputs: []string{"done <promise>COMPLETE</promise>"}}

	events := make(chan Event, 100)
	result, err := RunBuilderLoop(context.Background(), BuilderConfig{
		Dir:           dir,
		MaxIterations: 5,
		MaxTurns:      10,
		Runner:        mock,
		Events:        events,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Completed {
		t.Error("expected Completed=true")
	}

	// Collect all events
	close(events)
	var types []EventType
	for ev := range events {
		types = append(types, ev.Type)
	}

	// Should have: IterStart, IterEnd, LoopDone
	if len(types) < 3 {
		t.Fatalf("expected at least 3 events, got %d: %v", len(types), types)
	}
	if types[0] != EventIterStart {
		t.Errorf("first event should be EventIterStart, got %d", types[0])
	}
	// Last event should be LoopDone
	if types[len(types)-1] != EventLoopDone {
		t.Errorf("last event should be EventLoopDone, got %d", types[len(types)-1])
	}
}
```

Run: `go test ./internal/runner/ -run TestBuilderLoop_EmitsEvents -v`
Expected: FAIL (Event type not defined, Events field not on BuilderConfig)

**Step 2: Add Event types and Events field**

First, add the event types to `internal/runner/builder.go` (we'll keep them in the runner package since builder_test.go needs them without import cycles; the tui package will re-export or reference them):

```go
// EventType identifies the kind of TUI event.
type EventType int

const (
	EventIterStart  EventType = iota // Iteration beginning
	EventOutputLine                  // A line of claude output
	EventIterEnd                     // Iteration finished
	EventLoopDone                    // Loop finished
)

// Event carries information from the builder loop to the TUI.
type Event struct {
	Type    EventType
	Iter    int            // EventIterStart, EventIterEnd
	MaxIter int            // EventIterStart
	Task    string         // EventIterEnd
	Outcome string         // EventIterEnd
	Line    string         // EventOutputLine
	Err     error          // EventIterEnd (if failed), EventLoopDone
	Result  *BuilderResult // EventLoopDone
}
```

Add `Events chan<- Event` to `BuilderConfig`:

```go
type BuilderConfig struct {
	Dir           string
	MaxIterations int
	MaxTurns      int
	Model         string
	TaskOverride  string
	DryRun        bool
	Verbose       bool
	Runner        CommandRunner
	Events        chan<- Event
}
```

Add a helper to send events (no-op when Events is nil):

```go
func (cfg *BuilderConfig) emit(ev Event) {
	if cfg.Events != nil {
		cfg.Events <- ev
	}
}
```

**Step 3: Emit events from RunBuilderLoop**

Add event emissions at natural points in `RunBuilderLoop`. These go alongside the existing `fmt.Fprintf` calls (not replacing them):

At iteration start (after `fmt.Fprintf(os.Stderr, "golem: iteration %d starting...\n", i)`):
```go
cfg.emit(Event{Type: EventIterStart, Iter: i, MaxIter: cfg.MaxIterations})
```

After error from runner (inside the `if err != nil` block, before `continue`):
```go
cfg.emit(Event{Type: EventIterEnd, Iter: i, Err: err})
```

After COMPLETE promise detected (before `break`):
```go
cfg.emit(Event{Type: EventIterEnd, Iter: i, Task: "all tasks", Outcome: "complete"})
```

After successful iteration summary (after the `lastSession` print block):
```go
if lastSession != nil {
	cfg.emit(Event{Type: EventIterEnd, Iter: i, Task: lastSession.Task, Outcome: lastSession.Outcome})
} else {
	cfg.emit(Event{Type: EventIterEnd, Iter: i})
}
```

At end of function (before `return result, nil`):
```go
cfg.emit(Event{Type: EventLoopDone, Result: &result})
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/runner/ -v`
Expected: all tests PASS (existing tests unaffected since Events is nil)

**Step 5: Commit**

```bash
git add internal/runner/builder.go internal/runner/builder_test.go
git commit -m "feat(runner): emit events from builder loop for TUI consumption"
```

---

### Task 6: Move event types to dedicated file and update tui/events.go

**Files:**
- Create: `internal/runner/events.go` (move event types from builder.go)
- Modify: `internal/runner/builder.go` (remove event types, keep emit helper)
- Modify: `internal/tui/events.go` (delete — types live in runner package to avoid import cycles)

Since `builder_test.go` is in package `runner` and references `Event`/`EventType`, these types must live in the `runner` package. The `tui` package imports `runner`, so it has access.

**Step 1: Extract event types to events.go**

Create `internal/runner/events.go` with the Event and EventType types (cut from builder.go).

```go
// internal/runner/events.go
package runner

// EventType identifies the kind of TUI event.
type EventType int

const (
	EventIterStart  EventType = iota // Iteration beginning
	EventOutputLine                  // A line of claude output
	EventIterEnd                     // Iteration finished
	EventLoopDone                    // Loop finished
)

// Event carries information from the builder loop to the TUI.
type Event struct {
	Type    EventType
	Iter    int            // EventIterStart, EventIterEnd
	MaxIter int            // EventIterStart
	Task    string         // EventIterEnd
	Outcome string         // EventIterEnd
	Line    string         // EventOutputLine
	Err     error          // EventIterEnd (if failed), EventLoopDone
	Result  *BuilderResult // EventLoopDone
}
```

Remove the same types from `builder.go`. Delete `internal/tui/events.go` (no longer needed — `tui` imports `runner` for the types).

**Step 2: Verify build and tests**

Run: `go build ./... && go test ./internal/runner/ -v`
Expected: compiles, all tests pass

**Step 3: Commit**

```bash
git add internal/runner/events.go internal/runner/builder.go
git rm -f internal/tui/events.go 2>/dev/null; true
git commit -m "refactor(runner): extract event types to dedicated file"
```

---

### Task 7: Create shared lipgloss styles

**Files:**
- Create: `internal/tui/styles.go`

**Step 1: Create styles**

```go
// internal/tui/styles.go
package tui

import "github.com/charmbracelet/lipgloss"

const sidebarWidth = 24

var (
	// Sidebar styles
	sidebarStyle = lipgloss.NewStyle().
			Width(sidebarWidth).
			BorderStyle(lipgloss.NormalBorder()).
			BorderLeft(true).
			PaddingLeft(1).
			PaddingRight(1)

	sidebarHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				MarginBottom(1)

	// Task icon styles
	doneStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))  // green
	inProgressStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))  // yellow
	todoStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))  // dim
	blockedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))  // red

	// Footer
	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	// Output pane
	outputStyle = lipgloss.NewStyle().
			PaddingLeft(1)
)

func taskIcon(status string) string {
	switch status {
	case "done":
		return doneStyle.Render("✓")
	case "in-progress":
		return inProgressStyle.Render("◐")
	case "todo":
		return todoStyle.Render("○")
	case "blocked":
		return blockedStyle.Render("✗")
	default:
		return "?"
	}
}
```

**Step 2: Verify build**

Run: `go build ./...`
Expected: compiles

**Step 3: Commit**

```bash
git add internal/tui/styles.go
git commit -m "feat(tui): add shared lipgloss styles"
```

---

### Task 8: Create shared components (task list, stats)

**Files:**
- Create: `internal/tui/components.go`
- Create: `internal/tui/components_test.go`

**Step 1: Write failing tests**

```go
// internal/tui/components_test.go
package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/lofari/golem/internal/ctx"
)

func TestRenderTaskList(t *testing.T) {
	tasks := []ctx.Task{
		{Name: "auth module", Status: "done"},
		{Name: "price API", Status: "in-progress"},
		{Name: "charts", Status: "todo"},
		{Name: "shipping", Status: "blocked", BlockedReason: "API pending"},
	}

	result := renderTaskList(tasks, 22)
	if !strings.Contains(result, "auth module") {
		t.Error("should contain task name 'auth module'")
	}
	if !strings.Contains(result, "price API") {
		t.Error("should contain task name 'price API'")
	}
	if !strings.Contains(result, "shipping") {
		t.Error("should contain task name 'shipping'")
	}
}

func TestRenderStats(t *testing.T) {
	result := renderStats(3, 20, 4*time.Minute+32*time.Second, 2, 5, 12, 22)
	if !strings.Contains(result, "3/20") {
		t.Error("should show iteration count")
	}
	if !strings.Contains(result, "2/5") {
		t.Error("should show task progress")
	}
}

func TestRenderCurrentTask(t *testing.T) {
	result := renderCurrentTask("price API", 90*time.Second, 22)
	if !strings.Contains(result, "price API") {
		t.Error("should contain current task name")
	}
	if !strings.Contains(result, "1m30s") {
		t.Error("should show elapsed time")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -v`
Expected: FAIL (functions not defined)

**Step 3: Write implementation**

```go
// internal/tui/components.go
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/lofari/golem/internal/ctx"
)

func renderTaskList(tasks []ctx.Task, width int) string {
	var b strings.Builder

	done := 0
	for _, t := range tasks {
		if t.Status == "done" {
			done++
		}
	}

	header := fmt.Sprintf("Tasks %d/%d", done, len(tasks))
	b.WriteString(sidebarHeaderStyle.Render(header))
	b.WriteString("\n")

	for _, t := range tasks {
		icon := taskIcon(t.Status)
		name := t.Name
		// Truncate long names
		maxName := width - 4
		if maxName > 0 && len(name) > maxName {
			name = name[:maxName-1] + "…"
		}
		b.WriteString(fmt.Sprintf(" %s %s\n", icon, name))
	}

	return b.String()
}

func renderStats(iter, maxIter int, elapsed time.Duration, tasksDone, tasksTotal int, filesChanged int, width int) string {
	var b strings.Builder
	b.WriteString(sidebarHeaderStyle.Render("Stats"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(" Iteration  %d/%d\n", iter, maxIter))
	b.WriteString(fmt.Sprintf(" Elapsed    %s\n", formatDuration(elapsed)))
	b.WriteString(fmt.Sprintf(" Tasks      %d/%d\n", tasksDone, tasksTotal))
	if filesChanged > 0 {
		b.WriteString(fmt.Sprintf(" Files      %d\n", filesChanged))
	}
	return b.String()
}

func renderCurrentTask(taskName string, elapsed time.Duration, width int) string {
	if taskName == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(sidebarHeaderStyle.Render("Current"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(" ◐ %s\n", taskName))
	b.WriteString(fmt.Sprintf("   running %s\n", formatDuration(elapsed)))
	return b.String()
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

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/components.go internal/tui/components_test.go
git commit -m "feat(tui): add shared rendering components for task list, stats, current task"
```

---

### Task 9: Create golem run TUI model

**Files:**
- Create: `internal/tui/run.go`

This is the bubbletea `Model` for `golem run`. It reads from the `Events` channel, renders the split layout (output pane + sidebar), and auto-scrolls the output viewport.

**Step 1: Write the model**

```go
// internal/tui/run.go
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	golemctx "github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/runner"
)

// RunModel is the bubbletea model for `golem run`.
type RunModel struct {
	// Channels
	events   <-chan runner.Event
	outputCh <-chan string

	// State
	dir          string
	outputLines  []string
	viewport     viewport.Model
	state        golemctx.State
	iter         int
	maxIter      int
	currentTask  string
	iterStart    time.Time
	loopStart    time.Time
	done         bool
	finalResult  *runner.BuilderResult
	finalErr     error
	filesChanged int

	// Layout
	width  int
	height int
	ready  bool
}

// NewRunModel creates a new TUI model for the builder loop.
func NewRunModel(dir string, events <-chan runner.Event, outputCh <-chan string) RunModel {
	return RunModel{
		events:    events,
		outputCh:  outputCh,
		dir:       dir,
		loopStart: time.Now(),
		iterStart: time.Now(),
	}
}

// Messages
type tickMsg time.Time

type outputLineMsg string

type outputDoneMsg struct{}

func waitForOutput(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return outputDoneMsg{}
		}
		return outputLineMsg(line)
	}
}

func waitForEvent(ch <-chan runner.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return runner.Event{Type: runner.EventLoopDone}
		}
		return ev
	}
}

func doTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m RunModel) Init() tea.Cmd {
	return tea.Batch(
		waitForEvent(m.events),
		waitForOutput(m.outputCh),
		doTick(),
	)
}

func (m RunModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		outputWidth := m.width - sidebarWidth - 3 // border + padding
		vpHeight := m.height - 2                   // footer
		if !m.ready {
			m.viewport = viewport.New(outputWidth, vpHeight)
			m.ready = true
		} else {
			m.viewport.Width = outputWidth
			m.viewport.Height = vpHeight
		}
		m.viewport.SetContent(strings.Join(m.outputLines, "\n"))
		m.viewport.GotoBottom()

	case tickMsg:
		cmds = append(cmds, doTick())

	case outputLineMsg:
		m.outputLines = append(m.outputLines, string(msg))
		if m.ready {
			m.viewport.SetContent(strings.Join(m.outputLines, "\n"))
			m.viewport.GotoBottom()
		}
		cmds = append(cmds, waitForOutput(m.outputCh))

	case outputDoneMsg:
		// Output channel closed, no more output

	case runner.Event:
		switch msg.Type {
		case runner.EventIterStart:
			m.iter = msg.Iter
			m.maxIter = msg.MaxIter
			m.iterStart = time.Now()
			m.currentTask = ""
			// Refresh state from disk
			if s, err := golemctx.ReadState(m.dir); err == nil {
				m.state = s
			}

		case runner.EventIterEnd:
			m.currentTask = msg.Task
			// Refresh state from disk
			if s, err := golemctx.ReadState(m.dir); err == nil {
				m.state = s
			}
			// Count total files changed from log
			if l, err := golemctx.ReadLog(m.dir); err == nil {
				total := 0
				for _, s := range l.Sessions {
					total += len(s.FilesChanged)
				}
				m.filesChanged = total
			}

		case runner.EventLoopDone:
			m.done = true
			m.finalResult = msg.Result
			m.finalErr = msg.Err
			// Refresh state one last time
			if s, err := golemctx.ReadState(m.dir); err == nil {
				m.state = s
			}
		}
		cmds = append(cmds, waitForEvent(m.events))
	}

	// Update viewport scrolling
	if m.ready {
		var vpCmd tea.Cmd
		m.viewport, vpCmd = m.viewport.Update(msg)
		cmds = append(cmds, vpCmd)
	}

	return m, tea.Batch(cmds...)
}

func (m RunModel) View() string {
	if !m.ready {
		return "Initializing..."
	}

	// Sidebar content
	var sidebar strings.Builder

	// Task list
	sidebar.WriteString(renderTaskList(m.state.Tasks, sidebarWidth-2))

	// Stats
	tasksDone := 0
	for _, t := range m.state.Tasks {
		if t.Status == "done" {
			tasksDone++
		}
	}
	elapsed := time.Since(m.loopStart)
	sidebar.WriteString("\n")
	sidebar.WriteString(renderStats(m.iter, m.maxIter, elapsed, tasksDone, len(m.state.Tasks), m.filesChanged, sidebarWidth-2))

	// Current task
	if m.currentTask != "" || m.iter > 0 {
		iterElapsed := time.Since(m.iterStart)
		taskName := m.currentTask
		if taskName == "" {
			taskName = fmt.Sprintf("iteration %d", m.iter)
		}
		sidebar.WriteString("\n")
		sidebar.WriteString(renderCurrentTask(taskName, iterElapsed, sidebarWidth-2))
	}

	sidebarRendered := sidebarStyle.Height(m.height - 2).Render(sidebar.String())
	outputRendered := outputStyle.Render(m.viewport.View())

	main := lipgloss.JoinHorizontal(lipgloss.Top, outputRendered, sidebarRendered)

	// Footer
	footerLeft := " q quit"
	footerRight := ""
	if m.done {
		if m.finalResult != nil && m.finalResult.Completed {
			footerRight = "all tasks done!"
		} else {
			footerRight = "loop finished"
		}
	} else if m.iter > 0 {
		footerRight = fmt.Sprintf("iter %d/%d", m.iter, m.maxIter)
	}
	gap := m.width - len(footerLeft) - len(footerRight)
	if gap < 0 {
		gap = 0
	}
	footer := footerStyle.Render(footerLeft + strings.Repeat(" ", gap) + footerRight)

	return main + "\n" + footer
}
```

**Step 2: Verify build**

Run: `go build ./...`
Expected: compiles

**Step 3: Commit**

```bash
git add internal/tui/run.go
git commit -m "feat(tui): add bubbletea model for golem run"
```

---

### Task 10: Create golem status TUI model

**Files:**
- Create: `internal/tui/status.go`

This model polls `state.yaml` and `log.yaml` every 2 seconds and renders a full-width status display.

**Step 1: Write the model**

```go
// internal/tui/status.go
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	golemctx "github.com/lofari/golem/internal/ctx"
)

var (
	statusHeaderStyle = lipgloss.NewStyle().Bold(true)
	statusLabelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// StatusModel is the bubbletea model for `golem status`.
type StatusModel struct {
	dir    string
	state  golemctx.State
	log    golemctx.Log
	width  int
	height int
	err    error
}

// NewStatusModel creates a new status TUI model.
func NewStatusModel(dir string) StatusModel {
	m := StatusModel{dir: dir}
	m.refresh()
	return m
}

type statusTickMsg time.Time

func statusTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return statusTickMsg(t)
	})
}

func (m *StatusModel) refresh() {
	if s, err := golemctx.ReadState(m.dir); err == nil {
		m.state = s
		m.err = nil
	} else {
		m.err = err
	}
	if l, err := golemctx.ReadLog(m.dir); err == nil {
		m.log = l
	}
}

func (m StatusModel) Init() tea.Cmd {
	return statusTick()
}

func (m StatusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case statusTickMsg:
		m.refresh()
		return m, statusTick()
	}

	return m, nil
}

func (m StatusModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error reading state: %v\n\nPress q to quit.", m.err)
	}

	var b strings.Builder

	// Header
	b.WriteString(statusHeaderStyle.Render(fmt.Sprintf("Project: %s", m.state.Project.Name)))
	b.WriteString(statusLabelStyle.Render(fmt.Sprintf("          Phase: %s", m.state.Status.Phase)))
	b.WriteString("\n")
	if m.state.Status.CurrentFocus != "" {
		b.WriteString(statusLabelStyle.Render(fmt.Sprintf("Focus: %s", m.state.Status.CurrentFocus)))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Tasks
	done := 0
	for _, t := range m.state.Tasks {
		if t.Status == "done" {
			done++
		}
	}
	b.WriteString(sidebarHeaderStyle.Render(fmt.Sprintf("Tasks %d/%d", done, len(m.state.Tasks))))
	b.WriteString("\n")
	for _, t := range m.state.Tasks {
		icon := taskIcon(t.Status)
		line := fmt.Sprintf(" %s %s", icon, t.Name)
		if t.DependsOn != "" {
			line += statusLabelStyle.Render(fmt.Sprintf(" (depends on: %s)", t.DependsOn))
		}
		if t.Status == "in-progress" && t.Notes != "" {
			line += statusLabelStyle.Render(fmt.Sprintf(" — %q", t.Notes))
		}
		if t.Status == "blocked" && t.BlockedReason != "" {
			line += blockedStyle.Render(fmt.Sprintf(" — blocked: %q", t.BlockedReason))
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")

	// Summary line
	b.WriteString(fmt.Sprintf("Decisions: %d    Pitfalls: %d    Locked: %d\n",
		len(m.state.Decisions), len(m.state.Pitfalls), len(m.state.Locked)))
	b.WriteString(fmt.Sprintf("Sessions: %d logged\n", len(m.log.Sessions)))

	// Fill remaining height
	lines := strings.Count(b.String(), "\n")
	remaining := m.height - lines - 2 // footer
	if remaining > 0 {
		b.WriteString(strings.Repeat("\n", remaining))
	}

	// Footer
	footer := footerStyle.Render(" q quit" + strings.Repeat(" ", max(0, m.width-28)) + "watching state.yaml")

	return b.String() + footer
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

**Step 2: Verify build**

Run: `go build ./...`
Expected: compiles

**Step 3: Commit**

```bash
git add internal/tui/status.go
git commit -m "feat(tui): add bubbletea model for golem status (live-watching)"
```

---

### Task 11: Wire cmd/run.go with TUI mode

**Files:**
- Modify: `cmd/run.go`

**Step 1: Add --no-tui flag and TUI path**

Replace the RunE function body in `cmd/run.go` to support TUI mode:

```go
// cmd/run.go
package cmd

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lofari/golem/internal/runner"
	"github.com/lofari/golem/internal/scaffold"
	"github.com/lofari/golem/internal/tui"
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

		ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		maxIter, _ := cmd.Flags().GetInt("max-iterations")
		maxTurns, _ := cmd.Flags().GetInt("max-turns")
		task, _ := cmd.Flags().GetString("task")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		verbose, _ := cmd.Flags().GetBool("verbose")
		review, _ := cmd.Flags().GetBool("review")
		model, _ := cmd.Flags().GetString("model")
		noTUI, _ := cmd.Flags().GetBool("no-tui")

		useTUI := !noTUI && !dryRun && term.IsTerminal(int(os.Stdout.Fd()))

		if useTUI {
			return runWithTUI(ctx, dir, maxIter, maxTurns, task, verbose, review, model)
		}

		return runWithoutTUI(ctx, dir, maxIter, maxTurns, task, dryRun, verbose, review, model)
	},
}

func runWithoutTUI(ctx context.Context, dir string, maxIter, maxTurns int, task string, dryRun, verbose, review bool, model string) error {
	claudeRunner := &runner.ClaudeRunner{Verbose: verbose}

	result, err := runner.RunBuilderLoop(ctx, runner.BuilderConfig{
		Dir:           dir,
		MaxIterations: maxIter,
		MaxTurns:      maxTurns,
		Model:         model,
		TaskOverride:  task,
		DryRun:        dryRun,
		Verbose:       verbose,
		Runner:        claudeRunner,
	})
	if err != nil {
		return err
	}

	if result.Halted {
		return fmt.Errorf("loop halted: %s", result.HaltReason)
	}

	if review {
		fmt.Fprintln(os.Stderr, "\ngolem: chaining review pass...")
		_, err := runner.RunReview(ctx, dir, maxTurns, model, claudeRunner)
		return err
	}

	return nil
}

func runWithTUI(ctx context.Context, dir string, maxIter, maxTurns int, task string, verbose, review bool, model string) error {
	events := make(chan runner.Event, 100)
	outputCh := make(chan string, 1000)

	outputWriter := tui.NewLineWriter(outputCh)
	claudeRunner := &runner.ClaudeRunner{
		Verbose:      verbose,
		OutputWriter: outputWriter,
		ErrWriter:    io.Discard,
	}

	// Run builder loop in background goroutine
	go func() {
		defer close(outputCh)
		defer close(events)
		defer outputWriter.Flush()

		result, err := runner.RunBuilderLoop(ctx, runner.BuilderConfig{
			Dir:           dir,
			MaxIterations: maxIter,
			MaxTurns:      maxTurns,
			Model:         model,
			TaskOverride:  task,
			Verbose:       verbose,
			Runner:        claudeRunner,
			Events:        events,
		})
		_ = result
		_ = err
		// EventLoopDone is emitted inside RunBuilderLoop
	}()

	m := tui.NewRunModel(dir, events, outputCh)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().Int("max-iterations", 20, "maximum number of iterations")
	runCmd.Flags().Int("max-turns", 50, "max turns per Claude Code session")
	runCmd.Flags().String("task", "", "force agent to work on a specific task")
	runCmd.Flags().Bool("dry-run", false, "show rendered prompt without executing")
	runCmd.Flags().Bool("verbose", false, "extra output detail")
	runCmd.Flags().Bool("review", false, "run review pass after builder completes")
	runCmd.Flags().Bool("no-tui", false, "disable terminal UI (plain text output)")
}
```

Note: The `context.Context` import needs to come from the standard library. Since `signal.NotifyContext` returns a `context.Context`, the existing import of `"os/signal"` covers it. Add `"context"` to imports for the helper functions.

**Step 2: Verify build**

Run: `go build ./...`
Expected: compiles

**Step 3: Commit**

```bash
git add cmd/run.go
git commit -m "feat(cmd): wire TUI mode into golem run with --no-tui flag"
```

---

### Task 12: Wire cmd/status.go with TUI mode

**Files:**
- Modify: `cmd/status.go`

**Step 1: Update status command to use TUI by default**

```go
// cmd/status.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/display"
	"github.com/lofari/golem/internal/scaffold"
	"github.com/lofari/golem/internal/tui"
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

		noTUI, _ := cmd.Flags().GetBool("no-tui")
		useTUI := !noTUI && term.IsTerminal(int(os.Stdout.Fd()))

		if useTUI {
			m := tui.NewStatusModel(dir)
			p := tea.NewProgram(m, tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				return fmt.Errorf("TUI error: %w", err)
			}
			return nil
		}

		// Plain text fallback
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
	statusCmd.Flags().Bool("no-tui", false, "disable terminal UI (plain text output)")
}
```

**Step 2: Verify build**

Run: `go build ./...`
Expected: compiles

**Step 3: Commit**

```bash
git add cmd/status.go
git commit -m "feat(cmd): wire TUI mode into golem status with --no-tui flag"
```

---

### Task 13: Verify everything builds and tests pass

**Step 1: Full build**

Run: `go build ./...`
Expected: compiles

**Step 2: All tests**

Run: `go test ./... -v`
Expected: all pass

**Step 3: Go vet**

Run: `go vet ./...`
Expected: no issues

---

### Task 14: Manual smoke test

**Step 1: Test `golem status --no-tui`**

Run `golem status --no-tui` in a project with `.ctx/` — should produce identical output to before.

**Step 2: Test `golem status`**

Run `golem status` in a project with `.ctx/` — should show the TUI with live-watching. Press `q` to quit.

**Step 3: Test `golem run --no-tui --dry-run`**

Run `golem run --no-tui --dry-run` — should produce identical output to before.

**Step 4: Test `golem run --dry-run`**

With `--dry-run`, TUI is automatically disabled. Should fall back to plain text.
