package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
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
