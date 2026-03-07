package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFullWorkflow(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 1. Health check
	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatal("health check failed")
	}
	resp.Body.Close()

	// 2. Register project
	body, _ := json.Marshal(map[string]string{"path": dir})
	resp, err = http.Post(ts.URL+"/api/projects", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var regResult map[string]string
	json.NewDecoder(resp.Body).Decode(&regResult)
	resp.Body.Close()
	projectID := regResult["id"]
	if projectID == "" {
		t.Fatal("expected project ID from registration")
	}

	// 3. Get state
	resp, err = http.Get(ts.URL + "/api/projects/" + projectID + "/state")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("state endpoint returned %d: %s", resp.StatusCode, b)
	}
	var state map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&state)
	resp.Body.Close()
	if proj, ok := state["project"].(map[string]interface{}); ok {
		if proj["name"] != "test-project" {
			t.Fatalf("expected name test-project, got %v", proj["name"])
		}
	} else {
		t.Fatalf("unexpected state shape: %v", state)
	}

	// 4. Get log
	resp, err = http.Get(ts.URL + "/api/projects/" + projectID + "/log")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("log endpoint returned %d: %s", resp.StatusCode, b)
	}
	var log map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&log)
	resp.Body.Close()
	if sessions, ok := log["sessions"].([]interface{}); ok {
		if len(sessions) != 1 {
			t.Fatalf("expected 1 session, got %d", len(sessions))
		}
	} else {
		t.Fatalf("unexpected log shape: %v", log)
	}

	// 5. Get config
	resp, err = http.Get(ts.URL + "/api/projects/" + projectID + "/config")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatal("config endpoint failed")
	}
	resp.Body.Close()

	// 6. List processes (empty)
	resp, err = http.Get(ts.URL + "/api/projects/" + projectID + "/processes")
	if err != nil {
		t.Fatal(err)
	}
	var procs []ProcessInfo
	json.NewDecoder(resp.Body).Decode(&procs)
	resp.Body.Close()
	if len(procs) != 0 {
		t.Fatalf("expected 0 processes, got %d", len(procs))
	}
}
