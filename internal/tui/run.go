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
