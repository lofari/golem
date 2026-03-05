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
	if cfg.MaxTurns != 200 {
		t.Errorf("expected MaxTurns=200, got %d", cfg.MaxTurns)
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
	os.WriteFile(globalFile, []byte("verbose: true\nmax-turns: 300\nsandbox: true\n"), 0644)

	cfg := Load(globalFile, "")
	if !cfg.Verbose {
		t.Error("expected Verbose=true from global config")
	}
	if cfg.MaxTurns != 300 {
		t.Errorf("expected MaxTurns=300, got %d", cfg.MaxTurns)
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
