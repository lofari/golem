package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/graph/model"
	"github.com/lofari/golem/internal/graph/query"
)

func setupGraphProject(t *testing.T) (*Server, string) {
	t.Helper()
	dir := setupTestProject(t)

	// Create and populate graph
	dbPath := filepath.Join(dir, ".ctx", "graph.db")
	store, err := graph.OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	store.InsertBatch(
		[]graph.Node{
			{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go"},
			{ID: "fn:main.go:Foo", Type: "function", Name: "Foo", Path: "main.go", Line: 10},
			{ID: "fn:main.go:Bar", Type: "function", Name: "Bar", Path: "main.go", Line: 20},
		},
		[]graph.Edge{
			{From: "file:main.go", To: "fn:main.go:Foo", Type: "DEFINES"},
			{From: "fn:main.go:Foo", To: "fn:main.go:Bar", Type: "CALLS"},
		},
	)

	// Add execution data
	store.InsertExecution(model.Execution{SessionID: "s1", StartedAt: 1000, Status: "completed"})
	store.InsertCommand(model.Command{ID: "cmd:s1:1", SessionID: "s1", Seq: 1, Command: "go build", ExitCode: 0})
	store.FinalizeExecution("s1", 2000, "completed")

	store.Close()

	srv := New(Config{})
	srv.RegisterProject(dir)

	return srv, dir
}

func TestGraphRelated(t *testing.T) {
	srv, dir := setupGraphProject(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	id := srv.projectID(dir)
	resp, err := http.Get(ts.URL + "/api/projects/" + id + "/graph/related?name=Foo&direction=dependencies")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result query.RelatedResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	// Foo calls Bar, so Bar should appear
	foundBar := false
	for _, n := range result.Nodes {
		if n.Name == "Bar" {
			foundBar = true
		}
	}
	if !foundBar {
		t.Fatal("expected Bar in dependencies of Foo")
	}
}

func TestGraphRelated_MissingName(t *testing.T) {
	srv, dir := setupGraphProject(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	id := srv.projectID(dir)
	resp, err := http.Get(ts.URL + "/api/projects/" + id + "/graph/related")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGraphRelated_ProjectNotFound(t *testing.T) {
	srv := New(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/projects/nonexistent/graph/related?name=Foo")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGraphRuntimePath_Trace(t *testing.T) {
	srv, dir := setupGraphProject(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	id := srv.projectID(dir)
	resp, err := http.Get(ts.URL + "/api/projects/" + id + "/graph/runtime-path?mode=trace")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result query.TraceResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result.SessionID != "s1" {
		t.Fatalf("expected session s1, got %s", result.SessionID)
	}
	if len(result.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(result.Commands))
	}
}

func TestGraphRuntimePath_InvalidMode(t *testing.T) {
	srv, dir := setupGraphProject(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	id := srv.projectID(dir)
	resp, err := http.Get(ts.URL + "/api/projects/" + id + "/graph/runtime-path?mode=invalid")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
