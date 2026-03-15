package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	cfg := Load("", "")
	if cfg.MaxIterations != 20 {
		t.Errorf("expected MaxIterations=20, got %d", cfg.MaxIterations)
	}
	if cfg.MaxToolCalls != 200 {
		t.Errorf("expected MaxToolCalls=200, got %d", cfg.MaxToolCalls)
	}
	if cfg.MCP != true {
		t.Error("expected MCP=true")
	}
	if cfg.Parallel != 1 {
		t.Errorf("expected Parallel=1, got %d", cfg.Parallel)
	}
}

func TestLoad_GlobalOverrides(t *testing.T) {
	globalDir := t.TempDir()
	globalFile := filepath.Join(globalDir, "config.yaml")
	os.WriteFile(globalFile, []byte("verbose: true\nmax-tool-calls: 300\nsandbox: true\n"), 0644)

	cfg := Load(globalFile, "")
	if !cfg.Verbose {
		t.Error("expected Verbose=true from global config")
	}
	if cfg.MaxToolCalls != 300 {
		t.Errorf("expected MaxToolCalls=300, got %d", cfg.MaxToolCalls)
	}
	if !cfg.Sandbox {
		t.Error("expected Sandbox=true from global config")
	}
	// Unset values should remain default
	if cfg.MaxIterations != 20 {
		t.Errorf("expected MaxIterations=20 (default), got %d", cfg.MaxIterations)
	}
}

func TestLoad_ProjectOverridesGlobal(t *testing.T) {
	globalDir := t.TempDir()
	globalFile := filepath.Join(globalDir, "config.yaml")
	os.WriteFile(globalFile, []byte("verbose: true\nmax-iterations: 30\n"), 0644)

	projectDir := t.TempDir()
	projectFile := filepath.Join(projectDir, "config.yaml")
	os.WriteFile(projectFile, []byte("max-iterations: 10\n"), 0644)

	cfg := Load(globalFile, projectFile)
	if cfg.MaxIterations != 10 {
		t.Errorf("expected MaxIterations=10 (project override), got %d", cfg.MaxIterations)
	}
	// Global value should still be present for unoverridden fields
	if !cfg.Verbose {
		t.Error("expected Verbose=true from global config")
	}
}

func TestLoad_PluginDirMerge(t *testing.T) {
	globalDir := t.TempDir()
	globalFile := filepath.Join(globalDir, "config.yaml")
	os.WriteFile(globalFile, []byte("plugin-dir:\n  - /global/plugin\n"), 0644)

	projectDir := t.TempDir()
	projectFile := filepath.Join(projectDir, "config.yaml")
	os.WriteFile(projectFile, []byte("plugin-dir:\n  - /project/plugin\n"), 0644)

	cfg := Load(globalFile, projectFile)
	// Project replaces global for slices
	if len(cfg.PluginDir) != 1 || cfg.PluginDir[0] != "/project/plugin" {
		t.Errorf("expected project plugin-dir to override, got %v", cfg.PluginDir)
	}
}

func TestDefaults_ContextMap(t *testing.T) {
	cfg := Defaults()
	if !cfg.ContextMap {
		t.Error("expected ContextMap to default to true")
	}
	if cfg.ContextMapLimit != 15 {
		t.Errorf("expected ContextMapLimit to default to 15, got %d", cfg.ContextMapLimit)
	}
}

func TestDefaults_Engine(t *testing.T) {
	cfg := Defaults()
	if cfg.Engine != "go" {
		t.Fatalf("expected engine=go, got %s", cfg.Engine)
	}
	if cfg.DSLCommand != "golem-dsl" {
		t.Fatalf("expected dsl_command=golem-dsl, got %s", cfg.DSLCommand)
	}
}

func TestLoad_EngineFromYAML(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yaml")
	os.WriteFile(f, []byte("engine: dsl\ndsl-command: clj -M:run\n"), 0644)
	cfg := Load("", f)
	if cfg.Engine != "dsl" {
		t.Fatalf("expected engine=dsl, got %s", cfg.Engine)
	}
	if cfg.DSLCommand != "clj -M:run" {
		t.Fatalf("expected dsl-command='clj -M:run', got %s", cfg.DSLCommand)
	}
}

func TestDefaults_Agent(t *testing.T) {
	cfg := Defaults()
	if cfg.Agent != "build-feature" {
		t.Fatalf("expected agent=build-feature, got %s", cfg.Agent)
	}
}

func TestLoad_AgentOptsFromYAML(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yaml")
	os.WriteFile(f, []byte("agent: fix-bug\nagent-opts:\n  max_iterations: 3\n"), 0644)
	cfg := Load("", f)
	if cfg.Agent != "fix-bug" {
		t.Fatalf("expected agent=fix-bug, got %s", cfg.Agent)
	}
	if cfg.AgentOpts["max_iterations"] != 3 {
		t.Fatalf("expected max_iterations=3, got %v", cfg.AgentOpts["max_iterations"])
	}
}

func TestGetValue_Agent(t *testing.T) {
	cfg := Defaults()
	v, err := GetValue(cfg, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if v != "build-feature" {
		t.Errorf("expected 'build-feature', got %q", v)
	}
}

func TestGetValue_Engine(t *testing.T) {
	cfg := Defaults()
	v, err := GetValue(cfg, "engine")
	if err != nil {
		t.Fatal(err)
	}
	if v != "go" {
		t.Errorf("expected 'go', got %q", v)
	}
	v, err = GetValue(cfg, "dsl-command")
	if err != nil {
		t.Fatal(err)
	}
	if v != "golem-dsl" {
		t.Errorf("expected 'golem-dsl', got %q", v)
	}
}

func TestGetValue_ContextMap(t *testing.T) {
	cfg := Defaults()
	v, err := GetValue(cfg, "context-map")
	if err != nil {
		t.Fatal(err)
	}
	if v != "true" {
		t.Errorf("expected 'true', got %q", v)
	}

	v, err = GetValue(cfg, "context-map-limit")
	if err != nil {
		t.Fatal(err)
	}
	if v != "15" {
		t.Errorf("expected '15', got %q", v)
	}
}
