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
	var evtMsg WSMessage
	if err := json.Unmarshal(msg, &evtMsg); err != nil {
		t.Fatal(err)
	}
	if evtMsg.Type != "engine_event" {
		t.Fatalf("expected 'engine_event', got %q", evtMsg.Type)
	}
	if evtMsg.Event == nil {
		t.Fatal("expected Event to be non-nil")
	}

	// Verify the event data contains the expected type
	eventData, err := json.Marshal(evtMsg.Event)
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
