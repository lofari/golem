package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCommand_NoSandbox(t *testing.T) {
	cr := &ClaudeRunner{}
	args := []string{"-p", "hello", "--max-turns", "50", "--dangerously-skip-permissions"}

	name, got := cr.buildCommand("/tmp/project", args)

	if name != "claude" {
		t.Fatalf("expected command 'claude', got %q", name)
	}
	if len(got) != len(args) {
		t.Fatalf("expected %d args, got %d", len(args), len(got))
	}
	for i, a := range args {
		if got[i] != a {
			t.Errorf("arg[%d]: expected %q, got %q", i, a, got[i])
		}
	}
}

func TestBuildCommand_Sandbox(t *testing.T) {
	homeDir, _ := os.UserHomeDir()
	projectDir := "/tmp/project"

	cr := &ClaudeRunner{Sandbox: true}
	claudeArgs := []string{"-p", "hello", "--max-turns", "50"}

	name, got := cr.buildCommand(projectDir, claudeArgs)

	if name != "warden" {
		t.Fatalf("expected command 'warden', got %q", name)
	}

	joined := strings.Join(got, " ")

	// Must start with warden subcommand and flags
	if got[0] != "run" {
		t.Errorf("expected first arg 'run', got %q", got[0])
	}
	if !strings.Contains(joined, "--network") {
		t.Error("missing --network flag")
	}
	if !strings.Contains(joined, "--tools claude") {
		t.Error("missing --tools claude")
	}
	if !strings.Contains(joined, "--mount "+homeDir+"/.claude:ro") {
		t.Errorf("missing home .claude mount, got: %s", joined)
	}
	if !strings.Contains(joined, "--mount "+projectDir+":rw") {
		t.Errorf("missing project dir mount, got: %s", joined)
	}

	// Everything after "--" should be "claude" + original args
	dashIdx := -1
	for i, a := range got {
		if a == "--" {
			dashIdx = i
			break
		}
	}
	if dashIdx == -1 {
		t.Fatal("missing -- separator in warden args")
	}
	tail := got[dashIdx+1:]
	if tail[0] != "claude" {
		t.Errorf("expected 'claude' after --, got %q", tail[0])
	}
	for i, a := range claudeArgs {
		if tail[1+i] != a {
			t.Errorf("claude arg[%d]: expected %q, got %q", i, a, tail[1+i])
		}
	}
}

func TestBuildCommand_SandboxWithPluginDirs(t *testing.T) {
	projectDir := "/tmp/project"
	pluginDir := "/home/user/plugins/my-plugin"

	cr := &ClaudeRunner{
		Sandbox:    true,
		PluginDirs: []string{pluginDir},
	}
	claudeArgs := []string{"-p", "hello"}

	name, got := cr.buildCommand(projectDir, claudeArgs)

	if name != "warden" {
		t.Fatalf("expected command 'warden', got %q", name)
	}

	joined := strings.Join(got, " ")

	// Plugin dir should be mounted read-only
	absPlugin, _ := filepath.Abs(pluginDir)
	if !strings.Contains(joined, "--mount "+absPlugin+":ro") {
		t.Errorf("missing plugin dir mount, got: %s", joined)
	}
}
