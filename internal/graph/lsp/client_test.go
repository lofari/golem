package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestClientStartShutdown(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}

	cfg := ServerConfig{
		Language: "go",
		Binary:   "gopls",
		Args:     []string{"serve"},
	}

	client, err := StartClient(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	if client.ServerName() == "" {
		t.Error("expected server name after initialize")
	}

	if err := client.Shutdown(); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

func TestClientDocumentSymbols(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}

	dir := t.TempDir()

	// Create a minimal Go module
	writeTestFile(t, dir, "go.mod", "module testmod\n\ngo 1.21\n")
	writeTestFile(t, dir, "main.go", `package main

func main() {}

func helper() string {
	return "hello"
}

type Config struct {
	Name string
}
`)

	cfg := ServerConfig{
		Language: "go",
		Binary:   "gopls",
		Args:     []string{"serve"},
	}

	client, err := StartClient(cfg, dir)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer client.Shutdown()

	symbols, err := client.DocumentSymbols(dir + "/main.go")
	if err != nil {
		t.Fatalf("symbols: %v", err)
	}

	names := make(map[string]bool)
	for _, s := range symbols {
		names[s.Name] = true
	}

	for _, expected := range []string{"main", "helper", "Config"} {
		if !names[expected] {
			t.Errorf("missing symbol %q in %v", expected, names)
		}
	}
}

func TestClientDefinition(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}

	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module testmod\n\ngo 1.21\n")
	writeTestFile(t, dir, "main.go", `package main

func main() {
	helper()
}

func helper() {}
`)

	cfg := ServerConfig{
		Language: "go",
		Binary:   "gopls",
		Args:     []string{"serve"},
	}

	client, err := StartClient(cfg, dir)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer client.Shutdown()

	// Line 3 (0-indexed), col for "helper" in helper() call — col 1
	locs, err := client.Definition(dir+"/main.go", 3, 1)
	if err != nil {
		t.Fatalf("definition: %v", err)
	}

	if len(locs) == 0 {
		t.Fatal("expected at least 1 definition location")
	}

	// Should point to line 6 (0-indexed) where helper is defined
	if locs[0].Line != 6 {
		t.Errorf("expected definition at line 6, got %d", locs[0].Line)
	}
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := dir + "/" + name
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
