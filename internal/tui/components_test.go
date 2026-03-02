package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/lofari/golem/internal/ctx"
)

func TestRenderTaskList(t *testing.T) {
	tasks := []ctx.Task{
		{Name: "auth module", Status: "done"},
		{Name: "price API", Status: "in-progress"},
		{Name: "charts", Status: "todo"},
		{Name: "shipping", Status: "blocked", BlockedReason: "API pending"},
	}

	result := renderTaskList(tasks, 22)
	if !strings.Contains(result, "auth module") {
		t.Error("should contain task name 'auth module'")
	}
	if !strings.Contains(result, "price API") {
		t.Error("should contain task name 'price API'")
	}
	if !strings.Contains(result, "shipping") {
		t.Error("should contain task name 'shipping'")
	}
}

func TestRenderStats(t *testing.T) {
	result := renderStats(3, 20, 4*time.Minute+32*time.Second, 2, 5, 12, 22)
	if !strings.Contains(result, "3/20") {
		t.Error("should show iteration count")
	}
	if !strings.Contains(result, "2/5") {
		t.Error("should show task progress")
	}
}

func TestRenderCurrentTask(t *testing.T) {
	result := renderCurrentTask("price API", 90*time.Second, 22)
	if !strings.Contains(result, "price API") {
		t.Error("should contain current task name")
	}
	if !strings.Contains(result, "1m30s") {
		t.Error("should show elapsed time")
	}
}
