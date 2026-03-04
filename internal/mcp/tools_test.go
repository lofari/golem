package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	golemctx "github.com/lofari/golem/internal/ctx"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)
	state := golemctx.State{
		Project: golemctx.Project{Name: "test"},
		Status:  golemctx.Status{Phase: "building"},
		Tasks: []golemctx.Task{
			{Name: "auth", Status: "todo"},
			{Name: "api", Status: "in-progress"},
		},
	}
	golemctx.WriteState(dir, state)
	log := golemctx.Log{Sessions: []golemctx.Session{}}
	golemctx.WriteLog(dir, log)
	return dir
}

func TestHandleMarkTask(t *testing.T) {
	dir := setupTestDir(t)
	gs := NewServer(dir)

	result, err := gs.handleMarkTask(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"name":   "auth",
				"status": "done",
				"notes":  "implemented OAuth2",
			},
		},
	})
	if err != nil {
		t.Fatalf("handleMarkTask: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}

	state, _ := golemctx.ReadState(dir)
	task := state.FindTask("auth")
	if task.Status != "done" {
		t.Errorf("task status = %q, want %q", task.Status, "done")
	}
	if task.Notes != "implemented OAuth2" {
		t.Errorf("task notes = %q", task.Notes)
	}
}

func TestHandleMarkTask_InvalidStatus(t *testing.T) {
	dir := setupTestDir(t)
	gs := NewServer(dir)

	result, err := gs.handleMarkTask(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"name":   "auth",
				"status": "completed",
			},
		},
	})
	if err != nil {
		t.Fatalf("handleMarkTask: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid status")
	}
}

func TestHandleMarkTask_BlockedRequiresReason(t *testing.T) {
	dir := setupTestDir(t)
	gs := NewServer(dir)

	result, _ := gs.handleMarkTask(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"name":   "auth",
				"status": "blocked",
			},
		},
	})
	if !result.IsError {
		t.Fatal("expected error when blocking without reason")
	}
}

func TestHandleSetPhase(t *testing.T) {
	dir := setupTestDir(t)
	gs := NewServer(dir)

	result, err := gs.handleSetPhase(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"phase": "polishing",
			},
		},
	})
	if err != nil {
		t.Fatalf("handleSetPhase: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}

	state, _ := golemctx.ReadState(dir)
	if state.Status.Phase != "polishing" {
		t.Errorf("phase = %q, want %q", state.Status.Phase, "polishing")
	}
}

func TestHandleSetPhase_Invalid(t *testing.T) {
	dir := setupTestDir(t)
	gs := NewServer(dir)

	result, _ := gs.handleSetPhase(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"phase": "reviewing",
			},
		},
	})
	if !result.IsError {
		t.Fatal("expected error for invalid phase")
	}
}

func TestHandleLogSession(t *testing.T) {
	dir := setupTestDir(t)
	gs := NewServer(dir)

	result, err := gs.handleLogSession(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"task":          "auth",
				"outcome":       "done",
				"summary":       "implemented OAuth2 flow",
				"files_changed": []interface{}{"auth.go", "auth_test.go"},
			},
		},
	})
	if err != nil {
		t.Fatalf("handleLogSession: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}

	log, _ := golemctx.ReadLog(dir)
	if len(log.Sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(log.Sessions))
	}
	if log.Sessions[0].Task != "auth" {
		t.Errorf("session task = %q", log.Sessions[0].Task)
	}
}
