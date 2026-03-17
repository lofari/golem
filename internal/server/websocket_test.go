package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nhooyr.io/websocket"
)

func TestWebSocketProcessStream(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	srv.RegisterProject(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	pid := srv.projectID(dir)

	// Create a dummy process with output
	mp := &managedProcess{
		info: ProcessInfo{
			ID:      "test-proc",
			Command: "code",
			Status:  "running",
		},
		output: newRawBuffer(1024),
		subs:   make(map[chan []byte]struct{}),
	}
	srv.mu.Lock()
	srv.processes["test-proc"] = mp
	srv.mu.Unlock()

	// Write backlog
	mp.output.Write([]byte("hello world\n"))

	// Connect WebSocket
	wsURL := "ws" + ts.URL[4:] + "/api/projects/" + pid + "/processes/test-proc/stream"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Read from WebSocket — should get backlog first
	_, msg, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var wsMsg WSMessage
	if err := json.Unmarshal(msg, &wsMsg); err != nil {
		t.Fatal(err)
	}
	if wsMsg.Type != "output" {
		t.Fatalf("expected type 'output', got %q", wsMsg.Type)
	}
	decoded, err := base64.StdEncoding.DecodeString(wsMsg.Data)
	if err != nil {
		t.Fatalf("failed to decode base64: %v", err)
	}
	if string(decoded) != "hello world\n" {
		t.Fatalf("expected 'hello world\\n', got %q", string(decoded))
	}
}

func TestWebSocketEngineEvents(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	srv.RegisterProject(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	pid := srv.projectID(dir)

	// Create .ctx/runs/ directory before connecting WebSocket
	runsDir := filepath.Join(dir, ".ctx", "runs")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Connect to /watch WebSocket
	wsURL := "ws" + ts.URL[4:] + "/api/projects/" + pid + "/watch"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Read initial state_changed message
	_, msg, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var initMsg WSMessage
	if err := json.Unmarshal(msg, &initMsg); err != nil {
		t.Fatal(err)
	}
	if initMsg.Type != "state_changed" {
		t.Fatalf("expected initial 'state_changed', got %q", initMsg.Type)
	}

	// Create a run directory
	runDir := filepath.Join(runsDir, "run-test-001")
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Wait for watcher to pick up the new directory
	time.Sleep(100 * time.Millisecond)

	// Write NDJSON event to log.json
	eventJSON := `{"type":"pipeline-start","run_id":"run-test-001","timestamp":"2026-03-15T00:00:00Z"}` + "\n"
	logPath := filepath.Join(runDir, "log.json")
	if err := os.WriteFile(logPath, []byte(eventJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Read back engine_event message
	_, msg, err = conn.Read(ctx)
	if err != nil {
		t.Fatalf("failed to read engine_event: %v", err)
	}

	// Parse as raw map to verify projectId is included
	var rawMsg map[string]interface{}
	if err := json.Unmarshal(msg, &rawMsg); err != nil {
		t.Fatal(err)
	}
	if rawMsg["type"] != "engine_event" {
		t.Fatalf("expected 'engine_event', got %v", rawMsg["type"])
	}
	if rawMsg["projectId"] == nil || rawMsg["projectId"] == "" {
		t.Fatal("expected projectId to be set in engine_event")
	}
	if rawMsg["projectId"] != pid {
		t.Fatalf("expected projectId %q, got %v", pid, rawMsg["projectId"])
	}
	if rawMsg["event"] == nil {
		t.Fatal("expected event to be non-nil")
	}

	// Verify the event data contains the expected type
	eventData, err := json.Marshal(rawMsg["event"])
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(eventData, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["type"] != "pipeline-start" {
		t.Fatalf("expected event type 'pipeline-start', got %v", parsed["type"])
	}
}

func TestWebSocketEngineEvents_MultipleEvents(t *testing.T) {
	dir := setupTestProject(t)
	srv := New(Config{})
	srv.RegisterProject(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	pid := srv.projectID(dir)
	runsDir := filepath.Join(dir, ".ctx", "runs", "run-multi-001")
	os.MkdirAll(runsDir, 0755)

	wsURL := "ws" + ts.URL[4:] + "/api/projects/" + pid + "/watch"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Read initial state_changed
	_, _, _ = conn.Read(ctx)

	time.Sleep(100 * time.Millisecond)

	// Write multiple events in one batch
	logPath := filepath.Join(runsDir, "log.json")
	events := []map[string]interface{}{
		{"type": "pipeline-start", "timestamp": time.Now().Format(time.RFC3339Nano), "agent": "build-feature", "run-id": "run-multi-001"},
		{"type": "step-start", "timestamp": time.Now().Format(time.RFC3339Nano), "step": "scaffold", "step-type": "builtin", "run-id": "run-multi-001"},
		{"type": "step-end", "timestamp": time.Now().Format(time.RFC3339Nano), "step": "scaffold", "status": "success", "duration-ms": 1200, "run-id": "run-multi-001"},
	}
	f, _ := os.Create(logPath)
	for _, ev := range events {
		data, _ := json.Marshal(ev)
		f.Write(data)
		f.Write([]byte("\n"))
	}
	f.Close()

	// Should receive 3 engine_event messages
	received := 0
	for received < 3 {
		_, msg, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("expected more events, got %d: %v", received, err)
		}
		var wsMsg WSMessage
		json.Unmarshal(msg, &wsMsg)
		if wsMsg.Type == "engine_event" {
			received++
		}
	}
	if received != 3 {
		t.Fatalf("expected 3 engine events, got %d", received)
	}
}
