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
