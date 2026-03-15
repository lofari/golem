package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lofari/golem/internal/config"
	golemctx "github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/scaffold"
)

func TestApplySetupOutput_WritesConfig(t *testing.T) {
	dir := t.TempDir()
	scaffold.Init(dir, scaffold.InitOptions{Name: "test"})

	output := setupOutput{
		Config: map[string]interface{}{
			"test-cmd": "go test ./...",
			"lint-cmd": "golangci-lint run",
			"agent":    "build-feature",
		},
	}
	output.State.Stack = "go"
	output.State.Name = "myproject"

	data, _ := json.Marshal(output)
	os.WriteFile(filepath.Join(dir, "session-output.json"), data, 0644)

	if err := applySetupOutput(dir); err != nil {
		t.Fatalf("applySetupOutput failed: %v", err)
	}

	// Verify config was written
	cfg := config.Load("", config.ProjectPath(dir))
	if v, _ := config.GetValue(cfg, "agent"); v != "build-feature" {
		t.Errorf("expected agent=build-feature, got %q", v)
	}

	// Verify state was written
	state, err := golemctx.ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState failed: %v", err)
	}
	if state.Project.Stack != "go" {
		t.Errorf("expected stack=go, got %q", state.Project.Stack)
	}
	if state.Project.Name != "myproject" {
		t.Errorf("expected name=myproject, got %q", state.Project.Name)
	}
}

func TestApplySetupOutput_SkipsNullValues(t *testing.T) {
	dir := t.TempDir()
	scaffold.Init(dir, scaffold.InitOptions{Name: "test"})

	output := setupOutput{
		Config: map[string]interface{}{
			"test-cmd": "go test ./...",
			"lint-cmd": nil, // should be skipped
		},
	}

	data, _ := json.Marshal(output)
	os.WriteFile(filepath.Join(dir, "session-output.json"), data, 0644)

	if err := applySetupOutput(dir); err != nil {
		t.Fatalf("applySetupOutput failed: %v", err)
	}
}

func TestApplySetupOutput_NoFileGraceful(t *testing.T) {
	dir := t.TempDir()
	scaffold.Init(dir, scaffold.InitOptions{Name: "test"})

	// No session-output.json — should not error
	if err := applySetupOutput(dir); err != nil {
		t.Fatalf("expected no error when file missing, got: %v", err)
	}
}

func TestApplySetupOutput_CleansUpFile(t *testing.T) {
	dir := t.TempDir()
	scaffold.Init(dir, scaffold.InitOptions{Name: "test"})

	output := setupOutput{Config: map[string]interface{}{"agent": "one-shot"}}
	data, _ := json.Marshal(output)
	path := filepath.Join(dir, "session-output.json")
	os.WriteFile(path, data, 0644)

	applySetupOutput(dir)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected session-output.json to be deleted after processing")
	}
}
