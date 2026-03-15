package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	srv := New(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", body["status"])
	}
}

func TestCORSHeaders(t *testing.T) {
	srv := New(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/health", nil)
	req.Header.Set("Origin", "http://localhost:8314")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	got := resp.Header.Get("Access-Control-Allow-Origin")
	if got != "http://localhost:8314" {
		t.Fatalf("expected CORS origin http://localhost:8314, got %q", got)
	}
}

func TestRequestBodySizeLimit(t *testing.T) {
	srv := New(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Create a valid JSON body larger than 1MB
	padding := bytes.Repeat([]byte("x"), 2<<20)
	body, _ := json.Marshal(map[string]string{"path": string(padding)})
	resp, err := http.Post(ts.URL+"/api/projects", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized body, got %d", resp.StatusCode)
	}
}
