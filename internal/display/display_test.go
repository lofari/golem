// internal/display/display_test.go
package display

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lofari/golem/internal/ctx"
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
