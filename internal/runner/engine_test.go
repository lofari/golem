package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEngine_RunID(t *testing.T) {
	e := NewEngine(EngineConfig{
		Dir:       t.TempDir(),
		AgentName: "test",
		Goal:      "test goal",
	})
	if e.RunID == "" {
		t.Error("RunID should not be empty")
	}
	if !strings.Contains(e.RunID, "run-") {
		t.Errorf("RunID should start with run-, got: %s", e.RunID)
	}
}

func TestEngine_InitialState(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx", "runs"), 0755)

	e := NewEngine(EngineConfig{
		Dir:       dir,
		AgentName: "test",
		Goal:      "Add authentication",
	})

	state := e.State()
	goal, ok := state["goal"].(string)
	if !ok || goal != "Add authentication" {
		t.Errorf("state[goal] = %v, want 'Add authentication'", state["goal"])
	}
}
