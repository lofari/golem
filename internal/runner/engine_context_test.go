package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupMCP(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	cr := &ClaudeRunner{}
	err := setupMCP(dir, cr, true) // LSP enabled
	if err != nil {
		t.Fatalf("setupMCP error: %v", err)
	}

	configPath := filepath.Join(dir, ".ctx", "mcp_servers.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("mcp_servers.json should be created")
	}

	if cr.MCPConfig == "" {
		t.Error("ClaudeRunner.MCPConfig should be set")
	}
	if cr.MCPConfig != configPath {
		t.Errorf("MCPConfig = %q, want %q", cr.MCPConfig, configPath)
	}
}

func TestSetupMCP_NoLSP(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	cr := &ClaudeRunner{}
	err := setupMCP(dir, cr, false) // LSP disabled
	if err != nil {
		t.Fatalf("setupMCP error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, ".ctx", "mcp_servers.json"))
	if !strings.Contains(string(data), "no-lsp") {
		t.Error("MCP config should include --no-lsp flag when LSP disabled")
	}
}

func TestSetupCollector_NonClaudeRunner(t *testing.T) {
	mock := &smartMockRunner{
		responses: func(step string, callNum int) MockResponse {
			return MockResponse{}
		},
	}
	cleanup := setupCollector(t.TempDir(), "", mock, 5)
	if cleanup != nil {
		t.Error("cleanup should be nil for non-ClaudeRunner")
	}
}

func TestSetupCollector_NoGraphDB(t *testing.T) {
	dir := t.TempDir()
	cr := &ClaudeRunner{StreamJSON: true}
	cleanup := setupCollector(dir, filepath.Join(dir, "nonexistent.db"), cr, 5)
	if cleanup != nil {
		t.Error("cleanup should be nil when graph.db doesn't exist")
	}
}

func TestSyncGraph_NoGraphDB(t *testing.T) {
	dir := t.TempDir()
	err := syncGraph(dir, "")
	if err != nil {
		t.Errorf("syncGraph with no graph.db should not error, got: %v", err)
	}
}

func TestSyncGraph_ExplicitPathMissing(t *testing.T) {
	dir := t.TempDir()
	err := syncGraph(dir, filepath.Join(dir, "nonexistent.db"))
	if err != nil {
		t.Errorf("syncGraph with missing explicit path should not error, got: %v", err)
	}
}

func TestInjectProjectContext(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	stateYAML := `project:
  name: testproj
  summary: a test project
  stack: go
decisions:
  - what: use Go
    why: fast
    when: "2026-03-01"
  - what: use SQLite
    why: simple
    when: "2026-03-02"
pitfalls:
  - what: watch for nil maps
    fix: always initialize
  - watch for race conditions
tasks: []
`
	os.WriteFile(filepath.Join(dir, ".ctx", "state.yaml"), []byte(stateYAML), 0644)

	state := map[string]any{"goal": "test"}
	injectProjectContext(dir, state)

	pc, ok := state["project-context"].(string)
	if !ok || pc == "" {
		t.Fatal("project-context should be a non-empty string in state")
	}
	if !strings.Contains(pc, "use Go") {
		t.Error("project-context should contain decision 'use Go'")
	}
	if !strings.Contains(pc, "nil maps") {
		t.Error("project-context should contain pitfall about nil maps")
	}
}

func TestInjectProjectContext_NoStateFile(t *testing.T) {
	dir := t.TempDir()
	state := map[string]any{"goal": "test"}
	injectProjectContext(dir, state)

	if _, ok := state["project-context"]; ok {
		t.Error("project-context should not be set when state.yaml doesn't exist")
	}
}

func TestInjectProjectContext_EmptyDecisionsAndPitfalls(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	stateYAML := `project:
  name: testproj
  summary: a test project
  stack: go
decisions: []
pitfalls: []
tasks: []
`
	os.WriteFile(filepath.Join(dir, ".ctx", "state.yaml"), []byte(stateYAML), 0644)

	state := map[string]any{"goal": "test"}
	injectProjectContext(dir, state)

	if _, ok := state["project-context"]; ok {
		t.Error("project-context should not be set when decisions and pitfalls are empty")
	}
}
