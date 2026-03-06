package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func setupTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ctxDir := filepath.Join(dir, ".ctx")
	os.MkdirAll(ctxDir, 0755)
	os.WriteFile(filepath.Join(ctxDir, "state.yaml"), []byte(`
project:
  name: test-project
  summary: test
  stack: Go
  docs_path: docs/
status:
  phase: building
  current_focus: auth
tasks:
  - name: auth
    status: in-progress
  - name: api
    status: todo
`), 0644)
	os.WriteFile(filepath.Join(ctxDir, "log.yaml"), []byte(`
sessions:
  - iteration: 1
    timestamp: "2026-03-06T10:00:00Z"
    task: auth
    outcome: partial
    summary: started auth
`), 0644)
	return dir
}

func TestRegisterAndListProjects(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Register project
	body, _ := json.Marshal(map[string]string{"path": dir})
	resp, err := http.Post(ts.URL+"/api/projects", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d", resp.StatusCode)
	}

	// List projects
	resp, err = http.Get(ts.URL + "/api/projects")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var projects []ProjectInfo
	json.NewDecoder(resp.Body).Decode(&projects)
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Name != "test-project" {
		t.Fatalf("expected name test-project, got %q", projects[0].Name)
	}
}

func TestGetProjectState(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	srv.RegisterProject(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/projects/" + srv.projectID(dir) + "/state")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestGetProjectLog(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	srv.RegisterProject(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/projects/" + srv.projectID(dir) + "/log")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRegisterInvalidPath(t *testing.T) {
	srv := New(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body, _ := json.Marshal(map[string]string{"path": "/nonexistent"})
	resp, _ := http.Post(ts.URL+"/api/projects", "application/json", bytes.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
