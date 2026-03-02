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
	footer := footerStyle.Render(" q quit" + strings.Repeat(" ", maxInt(0, m.width-28)) + "watching state.yaml")

	return b.String() + footer
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
