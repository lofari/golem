package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lofari/golem/internal/runner"
	"github.com/lofari/golem/internal/server"
	"github.com/lofari/golem/templates"
	"nhooyr.io/websocket"
)

// setupIntegrationProject creates a temp dir with .ctx/ and a git repo.
func setupIntegrationProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %s\n%s", strings.Join(args, " "), err, out)
		}
	}

	ctxDir := filepath.Join(dir, ".ctx")
	os.MkdirAll(filepath.Join(ctxDir, "runs"), 0755)
	os.WriteFile(filepath.Join(ctxDir, "state.yaml"), []byte(`project:
  name: integration-test
  summary: test project
  stack: Go
status:
  phase: building
  current_focus: test
tasks: []
decisions: []
pitfalls: []
`), 0644)

	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0644)
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	cmd.CombinedOutput()
	cmd = exec.Command("git", "commit", "-m", "init")
	cmd.Dir = dir
	cmd.CombinedOutput()

	return dir
}

// mockRunner returns canned responses for blueprint steps.
type mockRunner struct {
	calls []string
}

func (m *mockRunner) Run(ctx context.Context, dir, prompt string, maxTurns int, model string) (string, error) {
	return m.RunWithTools(ctx, dir, prompt, maxTurns, model, nil)
}

func (m *mockRunner) RunWithTools(ctx context.Context, dir, prompt string, maxTurns int, model string, tools []string) (string, error) {
	step := "unknown"
	for _, name := range []string{"plan", "implement", "review", "research", "reflect"} {
		if strings.Contains(strings.ToLower(prompt), name) {
			step = name
			break
		}
	}
	m.calls = append(m.calls, step)

	out := map[string]any{"test-results": map[string]any{"status": "pass"}}
	if step == "plan" {
		out = map[string]any{"plan": []any{map[string]any{"step": 1, "desc": "do it"}}}
	}
	if step == "review" {
		out = map[string]any{"review-feedback": map[string]any{"verdict": "approved"}}
	}
	data, _ := json.Marshal(out)
	os.WriteFile(filepath.Join(dir, "session-output.json"), data, 0644)
	return "", nil
}

// TestEngineToWebSocket runs the engine and verifies events appear on the server WebSocket.
func TestEngineToWebSocket(t *testing.T) {
	dir := setupIntegrationProject(t)

	// Start server
	srv := server.New(server.Config{})
	srv.RegisterProject(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	pid := projectID(dir)

	// Connect WebSocket
	wsURL := "ws" + ts.URL[4:] + "/api/projects/" + pid + "/watch"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Read initial state_changed
	_, msg, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var initMsg map[string]any
	json.Unmarshal(msg, &initMsg)
	if initMsg["type"] != "state_changed" {
		t.Fatalf("expected initial state_changed, got %v", initMsg["type"])
	}

	// Parse one-shot blueprint
	data, err := templates.FS.ReadFile("agents/one-shot.yaml")
	if err != nil {
		t.Fatalf("read one-shot.yaml: %v", err)
	}
	bp, err := runner.ParseBlueprint(data)
	if err != nil {
		t.Fatalf("parse blueprint: %v", err)
	}

	mock := &mockRunner{}
	config := map[string]any{"lint-cmd": "true", "test-cmd": "true", "ci-enabled": false}

	e := runner.NewEngine(runner.EngineConfig{
		Dir:       dir,
		AgentName: "one-shot",
		Goal:      "Integration test goal",
		Blueprint: bp,
		Config:    config,
		Runner:    mock,
		Model:     "test",
	})

	// Run engine (writes log.json to .ctx/runs/{runId}/)
	if _, err := e.Run(context.Background()); err != nil {
		t.Fatalf("engine error: %v", err)
	}

	// Give the file watcher time to detect log.json changes
	time.Sleep(500 * time.Millisecond)

	// Collect engine events from WebSocket
	var engineEvents []map[string]any
	collectCtx, collectCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer collectCancel()

	for {
		_, msg, err := conn.Read(collectCtx)
		if err != nil {
			break
		}
		var wsMsg map[string]any
		json.Unmarshal(msg, &wsMsg)
		if wsMsg["type"] == "engine_event" {
			engineEvents = append(engineEvents, wsMsg)
		}
	}

	if len(engineEvents) == 0 {
		t.Fatal("expected engine events on WebSocket, got none")
	}

	// Verify pipeline-start and pipeline-end
	types := make(map[string]bool)
	for _, ev := range engineEvents {
		if event, ok := ev["event"].(map[string]any); ok {
			if evType, ok := event["type"].(string); ok {
				types[evType] = true
			}
		}
	}

	if !types["pipeline-start"] {
		t.Error("expected pipeline-start event")
	}
	if !types["pipeline-end"] {
		t.Error("expected pipeline-end event")
	}

	t.Logf("received %d engine events, types: %v", len(engineEvents), types)
}

func projectID(dir string) string {
	h := sha256.Sum256([]byte(dir))
	return hex.EncodeToString(h[:8])
}
