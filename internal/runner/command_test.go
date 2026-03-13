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
	if !strings.Contains(joined, "--env HOME="+homeDir) {
		t.Errorf("missing HOME env, got: %s", joined)
	}
	if !strings.Contains(joined, "--env CI=true") {
		t.Errorf("missing CI env, got: %s", joined)
	}
	if !strings.Contains(joined, "--mount "+homeDir+"/.claude:rw") {
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
	// Expect: stdbuf -oL claude <args...>
	if len(tail) < 3 || tail[0] != "stdbuf" || tail[1] != "-oL" || tail[2] != "claude" {
		t.Errorf("expected 'stdbuf -oL claude' after --, got %v", tail[:min(3, len(tail))])
	}
	for i, a := range claudeArgs {
		if tail[3+i] != a {
			t.Errorf("claude arg[%d]: expected %q, got %q", i, a, tail[3+i])
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

func TestBuildCommand_SandboxWithExtraTools(t *testing.T) {
	cr := &ClaudeRunner{
		Sandbox:      true,
		SandboxTools: []string{"go", "node"},
	}
	claudeArgs := []string{"-p", "hello"}

	name, got := cr.buildCommand("/tmp/project", claudeArgs)

	if name != "warden" {
		t.Fatalf("expected command 'warden', got %q", name)
	}

	joined := strings.Join(got, " ")

	// Tools should include claude plus extras
	if !strings.Contains(joined, "--tools claude,go,node") {
		t.Errorf("expected --tools claude,go,node, got: %s", joined)
	}
}

func TestBuildCommand_SandboxDefaultToolsClaude(t *testing.T) {
	cr := &ClaudeRunner{Sandbox: true}
	claudeArgs := []string{"-p", "hello"}

	_, got := cr.buildCommand("/tmp/project", claudeArgs)
	joined := strings.Join(got, " ")

	// Without extra tools, should just be "claude"
	if !strings.Contains(joined, "--tools claude") {
		t.Errorf("expected --tools claude, got: %s", joined)
	}
	// Should NOT have a trailing comma
	if strings.Contains(joined, "--tools claude,") {
		t.Errorf("unexpected trailing comma in tools: %s", joined)
	}
}

func TestBuildCommand_SandboxTimeoutAndMemory(t *testing.T) {
	cr := &ClaudeRunner{
		Sandbox:        true,
		SandboxTimeout: "2h",
		SandboxMemory:  "8g",
	}
	claudeArgs := []string{"-p", "hello"}

	name, got := cr.buildCommand("/tmp/project", claudeArgs)

	if name != "warden" {
		t.Fatalf("expected command 'warden', got %q", name)
	}

	joined := strings.Join(got, " ")

	if !strings.Contains(joined, "--timeout 2h") {
		t.Errorf("missing --timeout 2h, got: %s", joined)
	}
	if !strings.Contains(joined, "--memory 8g") {
		t.Errorf("missing --memory 8g, got: %s", joined)
	}
}

func TestBuildCommand_SandboxNoTimeoutMemoryWhenEmpty(t *testing.T) {
	cr := &ClaudeRunner{Sandbox: true}
	claudeArgs := []string{"-p", "hello"}

	_, got := cr.buildCommand("/tmp/project", claudeArgs)
	joined := strings.Join(got, " ")

	if strings.Contains(joined, "--timeout") {
		t.Errorf("should not include --timeout when empty, got: %s", joined)
	}
	if strings.Contains(joined, "--memory") {
		t.Errorf("should not include --memory when empty, got: %s", joined)
	}
}

func TestBuildCommand_WithToolsEnv_Sandbox(t *testing.T) {
	cr := &ClaudeRunner{Sandbox: true, SandboxTools: []string{"go"}}
	toolsEnv := "semantic_search,find_callers"
	args := []string{"-p", "--output-format", "stream-json", "--max-turns", "50"}

	name, gotArgs := cr.buildCommand("/tmp/project", args, toolsEnv)

	if name != "warden" {
		t.Fatalf("expected warden, got %q", name)
	}
	found := false
	for i, arg := range gotArgs {
		if arg == "--env" && i+1 < len(gotArgs) && strings.HasPrefix(gotArgs[i+1], "GOLEM_TOOLS=") {
			found = true
			if gotArgs[i+1] != "GOLEM_TOOLS=semantic_search,find_callers" {
				t.Errorf("GOLEM_TOOLS value = %q, want %q", gotArgs[i+1], "GOLEM_TOOLS=semantic_search,find_callers")
			}
			break
		}
	}
	if !found {
		t.Errorf("--env GOLEM_TOOLS not found in warden args: %v", gotArgs)
	}
}

func TestBuildCommand_WithToolsEnv_NoSandbox(t *testing.T) {
	cr := &ClaudeRunner{}
	toolsEnv := "semantic_search"
	args := []string{"-p"}

	name, _ := cr.buildCommand("/tmp/project", args, toolsEnv)

	if name != "claude" {
		t.Fatalf("expected claude, got %q", name)
	}
}
