package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// MockCall records a single runner invocation.
type MockCall struct {
	Prompt string
	Tools  []string
	Dir    string
}

// MockResponse defines canned behavior for a mock runner call.
type MockResponse struct {
	Output        string
	Err           error
	SessionOutput map[string]any
}

// containsStr checks if a string slice contains a value.
func containsStr(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// setupGitRepo creates a temporary git repo with an initial commit.
func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %s\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0644)
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

// smartMockRunner dispatches responses based on step name and call number.
type smartMockRunner struct {
	responses func(step string, callNum int) MockResponse
	calls     []MockCall
	callCount int
}

func (m *smartMockRunner) Run(ctx context.Context, dir, prompt string, maxTurns int, model string) (string, error) {
	return m.RunWithTools(ctx, dir, prompt, maxTurns, model, nil)
}

func (m *smartMockRunner) RunWithTools(ctx context.Context, dir, prompt string, maxTurns int, model string, tools []string) (string, error) {
	m.callCount++
	call := MockCall{Prompt: prompt, Tools: tools, Dir: dir}
	m.calls = append(m.calls, call)

	stepName := "unknown"
	for _, name := range []string{"plan", "implement", "review", "research", "reflect"} {
		if strings.Contains(strings.ToLower(prompt), name) {
			stepName = name
			break
		}
	}

	resp := m.responses(stepName, m.callCount)

	if resp.SessionOutput != nil {
		data, _ := json.Marshal(resp.SessionOutput)
		os.WriteFile(filepath.Join(dir, "session-output.json"), data, 0644)
	}

	return resp.Output, resp.Err
}
