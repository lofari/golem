package lsp

import (
	"os/exec"
	"testing"
)

func TestManagerStartShutdown(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}

	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module testmod\n\ngo 1.21\n")
	writeTestFile(t, dir, "main.go", "package main\nfunc main() {}\n")

	mgr := NewManager(dir)

	configs := []ServerConfig{
		{Language: "go", Binary: "gopls", Args: []string{"serve"}},
	}

	if err := mgr.Start(configs); err != nil {
		t.Fatalf("start: %v", err)
	}

	client := mgr.ClientFor("go")
	if client == nil {
		t.Fatal("expected go client")
	}

	if mgr.ClientFor("python") != nil {
		t.Error("expected nil for unavailable language")
	}

	if err := mgr.Shutdown(); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

func TestManagerStartSkipsMissing(t *testing.T) {
	dir := t.TempDir()

	mgr := NewManager(dir)
	configs := []ServerConfig{
		{Language: "fake", Binary: "nonexistent-lsp-binary-12345"},
	}

	// Start should not error — it skips servers that fail to start
	err := mgr.Start(configs)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if mgr.ClientFor("fake") != nil {
		t.Error("expected nil for failed server")
	}
}
