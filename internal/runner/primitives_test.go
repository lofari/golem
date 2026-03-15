package runner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)


func TestPrimitiveGitSetup(t *testing.T) {
	dir := setupGitRepo(t)
	result, err := primitiveGitSetup(context.Background(), dir, "test-agent", map[string]any{})
	if err != nil {
		t.Fatalf("git-setup error: %v", err)
	}
	branch, ok := result["branch"].(string)
	if !ok || !strings.HasPrefix(branch, "golem/test-agent-") {
		t.Errorf("branch = %v, want prefix golem/test-agent-", result["branch"])
	}
	base, ok := result["base"].(string)
	if !ok || base == "" {
		t.Errorf("base = %v, want non-empty", result["base"])
	}
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = dir
	out, _ := cmd.Output()
	if strings.TrimSpace(string(out)) != branch {
		t.Errorf("current branch = %q, want %q", strings.TrimSpace(string(out)), branch)
	}
}

func TestPrimitiveLint_NotConfigured(t *testing.T) {
	result, err := primitiveLint(context.Background(), t.TempDir(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	status, _ := result["status"].(string)
	if status != "skipped" {
		t.Errorf("status = %q, want %q", status, "skipped")
	}
}

func TestPrimitiveLint_Pass(t *testing.T) {
	config := map[string]any{"lint-cmd": "true"}
	result, err := primitiveLint(context.Background(), t.TempDir(), config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	status, _ := result["status"].(string)
	if status != "pass" {
		t.Errorf("status = %q, want %q", status, "pass")
	}
}

func TestPrimitiveLint_Fail(t *testing.T) {
	config := map[string]any{"lint-cmd": "false"}
	result, err := primitiveLint(context.Background(), t.TempDir(), config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	status, _ := result["status"].(string)
	if status != "fail" {
		t.Errorf("status = %q, want %q", status, "fail")
	}
}

func TestPrimitiveRunTests_NotConfigured(t *testing.T) {
	result, err := primitiveRunTests(context.Background(), t.TempDir(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	status, _ := result["status"].(string)
	if status != "skipped" {
		t.Errorf("status = %q, want %q", status, "skipped")
	}
}

func TestPrimitiveRunTests_Pass(t *testing.T) {
	config := map[string]any{"test-cmd": "true"}
	result, err := primitiveRunTests(context.Background(), t.TempDir(), config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	status, _ := result["status"].(string)
	if status != "pass" {
		t.Errorf("status = %q, want %q", status, "pass")
	}
	if _, ok := result["duration-ms"]; !ok {
		t.Error("missing duration-ms")
	}
}

func TestPrimitiveCITests_GhNotFound(t *testing.T) {
	// Hide gh from PATH so exec.LookPath fails
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	defer os.Setenv("PATH", origPath)

	config := map[string]any{}
	state := map[string]any{"branch": "golem/test-123", "base": "main"}
	_, err := primitiveCITests(context.Background(), t.TempDir(), config, state)
	if err == nil {
		t.Fatal("expected error when gh not found")
	}
	var unrecov *UnrecoverableError
	if !errors.As(err, &unrecov) {
		t.Errorf("expected UnrecoverableError, got %T", err)
	}
}

func TestPrimitiveCreatePR_NoChanges(t *testing.T) {
	dir := setupGitRepo(t)
	state := map[string]any{
		"branch": "golem/test-123",
		"base":   "main",
		"goal":   "Test goal",
		"code":   map[string]any{"files": []string{}},
	}
	result, err := primitiveCreatePR(context.Background(), dir, map[string]any{}, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	status, _ := result["status"].(string)
	if status != "skipped" {
		t.Errorf("status = %q, want %q", status, "skipped")
	}
}

func TestBuildGHPRArgs_NoShellInjection(t *testing.T) {
	title := `feat: add "quoted" feature & more`
	body := "line1\nline2\n$(whoami)"
	base := "main"
	branch := "golem/build$(rm)-test"

	args := buildGHPRArgs(title, body, base, branch)
	expected := []string{"pr", "create", "--title", title, "--body", body, "--base", base, "--head", branch}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, a := range expected {
		if args[i] != a {
			t.Errorf("arg[%d]: expected %q, got %q", i, a, args[i])
		}
	}
}

func TestGeneratePRTitle(t *testing.T) {
	tests := []struct {
		goal string
		want string
	}{
		{"Add auth", "Add auth"},
		{"Short", "Short"},
		{"Refactor the authentication middleware to support OAuth2 and OIDC flows with token refresh", "Refactor the authentication middleware to support OAuth2 and OIDC..."},
		{strings.Repeat("a", 100), strings.Repeat("a", 67) + "..."},
	}
	for _, tt := range tests {
		got := generatePRTitle(tt.goal)
		if got != tt.want {
			t.Errorf("generatePRTitle(%q) = %q, want %q", tt.goal, got, tt.want)
		}
	}
}
