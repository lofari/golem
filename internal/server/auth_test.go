package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddleware_RejectsWithoutToken(t *testing.T) {
	srv := New(Config{Token: "secret-token"})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAuthMiddleware_AcceptsValidToken(t *testing.T) {
	srv := New(Config{Token: "secret-token"})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/health", nil)
	req.Header.Set("Authorization", "Bearer secret-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAuthMiddleware_HealthPublicWhenNoToken(t *testing.T) {
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
}

func TestAuthMiddleware_TokenInQueryParam(t *testing.T) {
	srv := New(Config{Token: "secret-token"})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/health?token=secret-token")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestCORS_RestrictsOrigin(t *testing.T) {
	srv := New(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/health", nil)
	req.Header.Set("Origin", "http://evil.com")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	got := resp.Header.Get("Access-Control-Allow-Origin")
	if got != "" {
		t.Fatalf("expected no Access-Control-Allow-Origin for evil.com, got %q", got)
	}
}

func TestCORS_AllowsLocalhost(t *testing.T) {
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
		t.Fatalf("expected http://localhost:8314, got %q", got)
	}
}

func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 64 { // 32 bytes = 64 hex chars
		t.Fatalf("expected 64 char hex token, got %d chars", len(token))
	}

	// Ensure uniqueness
	token2, _ := GenerateToken()
	if token == token2 {
		t.Fatal("two generated tokens should not be equal")
	}
}
