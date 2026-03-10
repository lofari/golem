package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionCmd_RequiresPromptFlag(t *testing.T) {
	cmd := rootCmd
	cmd.SetArgs([]string{"session"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --prompt not provided")
	}
}

func TestSessionCmd_ReadsPromptFile(t *testing.T) {
	dir := t.TempDir()
	prompt := filepath.Join(dir, "prompt.md")
	os.WriteFile(prompt, []byte("test prompt"), 0644)

	cmd := rootCmd
	cmd.SetArgs([]string{"session", "--prompt", prompt, "--dir", dir, "--dry-run"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
