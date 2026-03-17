package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLaunchAndListProcesses(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	srv.RegisterProject(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	pid := srv.projectID(dir)

	// Launch a process (use "echo" as a test command instead of real golem)
	body, _ := json.Marshal(LaunchRequest{
		Command: "code",
		Config:  LaunchConfig{MaxIterations: 5},
	})
	resp, err := http.Post(ts.URL+"/api/projects/"+pid+"/processes", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["id"] == "" {
		t.Fatal("expected process ID")
	}

	// List processes
	resp2, err := http.Get(ts.URL + "/api/projects/" + pid + "/processes")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	var procs []ProcessInfo
	json.NewDecoder(resp2.Body).Decode(&procs)
	if len(procs) != 1 {
		t.Fatalf("expected 1 process, got %d", len(procs))
	}
	if procs[0].Command != "code" {
		t.Fatalf("expected command 'code', got %q", procs[0].Command)
	}
}

func TestStopProcess(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	srv.RegisterProject(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	pid := srv.projectID(dir)

	// Launch
	body, _ := json.Marshal(LaunchRequest{Command: "code"})
	resp, _ := http.Post(ts.URL+"/api/projects/"+pid+"/processes", "application/json", bytes.NewReader(body))
	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	procID := result["id"]

	// Stop
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/projects/"+pid+"/processes/"+procID, nil)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}

	// Wait a moment and verify it's gone
	time.Sleep(100 * time.Millisecond)
	resp3, _ := http.Get(ts.URL + "/api/projects/" + pid + "/processes")
	var procs []ProcessInfo
	json.NewDecoder(resp3.Body).Decode(&procs)
	resp3.Body.Close()

	for _, p := range procs {
		if p.ID == procID && p.Status == "running" {
			t.Fatal("process should not be running after stop")
		}
	}
}

func TestStopAllProcesses(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	srv.RegisterProject(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	pid := srv.projectID(dir)

	// Launch a process
	body, _ := json.Marshal(LaunchRequest{Command: "code"})
	resp, err := http.Post(ts.URL+"/api/projects/"+pid+"/processes", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	// StopAll
	srv.StopAll()

	// Wait for processes to terminate
	time.Sleep(200 * time.Millisecond)

	// Verify no processes have status "running"
	srv.mu.RLock()
	defer srv.mu.RUnlock()
	for _, mp := range srv.processes {
		mp.mu.Lock()
		status := mp.info.Status
		mp.mu.Unlock()
		if status == "running" {
			t.Fatalf("expected no running processes after StopAll, found %s", mp.info.ID)
		}
	}
}

func TestProcessInfoIncludesAgentNameGoalRunID(t *testing.T) {
	// Verify ProcessInfo has the new fields and they serialize correctly.
	info := ProcessInfo{
		ID:        "test-id",
		Command:   "run",
		Status:    "running",
		StartedAt: time.Now(),
		AgentName: "build-feature",
		Goal:      "add login page",
		RunID:     "test-id",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["agentName"] != "build-feature" {
		t.Fatalf("expected agentName 'build-feature', got %v", parsed["agentName"])
	}
	if parsed["goal"] != "add login page" {
		t.Fatalf("expected goal 'add login page', got %v", parsed["goal"])
	}
	if parsed["runId"] != "test-id" {
		t.Fatalf("expected runId 'test-id', got %v", parsed["runId"])
	}
}

func TestProcessInfoOmitsEmptyOptionalFields(t *testing.T) {
	// Verify omitempty works: fields should not appear when empty.
	info := ProcessInfo{
		ID:      "test-id",
		Command: "code",
		Status:  "running",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if _, exists := parsed["agentName"]; exists {
		t.Fatal("agentName should be omitted when empty")
	}
	if _, exists := parsed["goal"]; exists {
		t.Fatal("goal should be omitted when empty")
	}
	if _, exists := parsed["runId"]; exists {
		t.Fatal("runId should be omitted when empty")
	}
}

func TestProcessGC(t *testing.T) {
	srv := New(Config{ProcessRetention: 1 * time.Millisecond})

	// Manually add a stopped process with an old StartedAt
	mp := &managedProcess{
		info: ProcessInfo{
			ID:        "test-gc-proc",
			Command:   "code",
			Status:    "stopped",
			StartedAt: time.Now().Add(-1 * time.Hour),
		},
		cancel: func() {},
		subs:   make(map[chan []byte]struct{}),
	}
	srv.mu.Lock()
	srv.processes["test-gc-proc"] = mp
	srv.mu.Unlock()

	// Verify the process exists
	srv.mu.RLock()
	if _, ok := srv.processes["test-gc-proc"]; !ok {
		srv.mu.RUnlock()
		t.Fatal("process should exist before reap")
	}
	srv.mu.RUnlock()

	// Reap
	srv.reapProcesses()

	// Verify it's removed
	srv.mu.RLock()
	defer srv.mu.RUnlock()
	if _, ok := srv.processes["test-gc-proc"]; ok {
		t.Fatal("expected process to be reaped")
	}
}
