package mcp

import (
	"testing"
)

func TestNewServer(t *testing.T) {
	s := NewServer(t.TempDir(), nil)
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
	tools := s.ListTools()
	if len(tools) < 15 {
		t.Errorf("got %d tools, want at least 15", len(tools))
	}
}

func TestRegisterTools_WithGOLEM_TOOLS(t *testing.T) {
	t.Setenv("GOLEM_TOOLS", "mark_task,set_phase")

	gs := NewServer(t.TempDir(), nil)
	tools := gs.ListTools()

	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d: %v", len(tools), tools)
	}
	toolSet := make(map[string]bool)
	for _, name := range tools {
		toolSet[name] = true
	}
	if !toolSet["mark_task"] {
		t.Error("mark_task should be registered")
	}
	if !toolSet["set_phase"] {
		t.Error("set_phase should be registered")
	}
}

func TestRegisterTools_EmptyGOLEM_TOOLS(t *testing.T) {
	t.Setenv("GOLEM_TOOLS", "")

	gs := NewServer(t.TempDir(), nil)
	tools := gs.ListTools()

	if len(tools) < 5 {
		t.Fatalf("expected all tools (>5), got %d", len(tools))
	}
}
