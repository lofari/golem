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
	b.WriteString(fmt.Sprintf(" Elapsed    %s\n", tuiFormatDuration(elapsed)))
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
	b.WriteString(fmt.Sprintf("   running %s\n", tuiFormatDuration(elapsed)))
	return b.String()
}

func tuiFormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%02ds", mins, secs)
}
