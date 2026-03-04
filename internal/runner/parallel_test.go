package runner

import (
	"testing"

	golemctx "github.com/lofari/golem/internal/ctx"
)

func TestEligibleTasks(t *testing.T) {
	tasks := []golemctx.Task{
		{Name: "auth", Status: "done"},
		{Name: "api", Status: "todo"},
		{Name: "ui", Status: "todo", DependsOn: golemctx.FlexString{"api"}},
		{Name: "tests", Status: "todo"},
		{Name: "deploy", Status: "blocked", BlockedReason: "waiting"},
		{Name: "docs", Status: "in-progress"},
	}

	eligible := EligibleTasks(tasks)

	// api and tests are eligible (todo, no unresolved deps)
	// ui depends on api which isn't done — not eligible
	// deploy is blocked — not eligible
	// docs is in-progress — eligible (should be picked up)
	// auth is done — not eligible
	names := make(map[string]bool)
	for _, task := range eligible {
		names[task.Name] = true
	}

	if !names["api"] {
		t.Error("api should be eligible")
	}
	if !names["tests"] {
		t.Error("tests should be eligible")
	}
	if !names["docs"] {
		t.Error("docs (in-progress) should be eligible")
	}
	if names["ui"] {
		t.Error("ui should NOT be eligible (depends on api)")
	}
	if names["deploy"] {
		t.Error("deploy should NOT be eligible (blocked)")
	}
	if names["auth"] {
		t.Error("auth should NOT be eligible (done)")
	}
	if len(eligible) != 3 {
		t.Errorf("got %d eligible, want 3", len(eligible))
	}
}

func TestSanitizeTaskName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Simple Task", "simple-task"},
		{"[review] Fix the bug!", "review-fix-the-bug"},
		{"Task with    spaces", "task-with-spaces"},
		{"A very long task name that exceeds the maximum length allowed for worktree directory names and should be truncated", "a-very-long-task-name-that-exceeds-the-maximum-length-allowed-for-work"},
	}
	for _, tt := range tests {
		got := SanitizeTaskName(tt.input)
		if got != tt.want {
			t.Errorf("SanitizeTaskName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
