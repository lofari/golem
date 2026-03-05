package mcp

import (
	"testing"
)

func TestNewServer(t *testing.T) {
	s := NewServer(t.TempDir())
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
	tools := s.ListTools()
	expected := []string{"mark_task", "set_phase", "set_status", "add_decision", "add_pitfall", "add_locked", "log_session"}
	if len(tools) != len(expected) {
		t.Errorf("got %d tools, want %d", len(tools), len(expected))
	}
}
