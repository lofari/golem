// internal/display/display.go
package display

import (
	"fmt"
	"io"
	"strings"

	"github.com/lofari/golem/internal/ctx"
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
			if !t.DependsOn.IsEmpty() {
				line += fmt.Sprintf(" (depends on: %s)", t.DependsOn.String())
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

func PrintPitfalls(w io.Writer, pitfalls []ctx.Pitfall) {
	if len(pitfalls) == 0 {
		fmt.Fprintln(w, "No pitfalls noted.")
		return
	}
	for _, p := range pitfalls {
		fmt.Fprintf(w, "• %s\n", p.String())
	}
}
