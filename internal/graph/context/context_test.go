package context

import (
	gocontext "context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/graph/model"
)

func setupTestStore(t *testing.T) *graph.Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	store, err := graph.OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// fakeEmbedder returns deterministic vectors for testing.
type fakeEmbedder struct{}

func (f *fakeEmbedder) Embed(_ gocontext.Context, texts []string) ([][]float32, error) {
	vecs := make([][]float32, len(texts))
	for i, text := range texts {
		vecs[i] = f.vectorFor(text)
	}
	return vecs, nil
}

func (f *fakeEmbedder) Dimensions() int { return 384 }
func (f *fakeEmbedder) Close() error    { return nil }

// vectorFor produces a deterministic 384-dim vector based on text content.
// Similar texts produce similar vectors via simple character frequency hashing.
func (f *fakeEmbedder) vectorFor(text string) []float32 {
	vec := make([]float32, 384)
	for i, c := range text {
		idx := (int(c) + i) % 384
		vec[idx] += 1.0
	}
	// Normalize
	var norm float32
	for _, v := range vec {
		norm += v * v
	}
	if norm > 0 {
		norm = float32(1.0 / float64(norm))
		for i := range vec {
			vec[i] *= norm
		}
	}
	return vec
}

func TestContextMap_Format_Empty(t *testing.T) {
	cm := &ContextMap{Task: "fix login"}
	got := cm.Format()
	if got != "" {
		t.Errorf("expected empty string for no symbols, got %q", got)
	}
}

func TestContextMap_Format(t *testing.T) {
	cm := &ContextMap{
		Task: "fix login",
		Symbols: []SymbolEntry{
			{
				Name:      "ValidateCredentials",
				Kind:      "function",
				Path:      "auth/login.go",
				Line:      45,
				Relations: []string{"calls CheckPassword", "called by LoginHandler"},
			},
			{
				Name:      "SessionMiddleware",
				Kind:      "method",
				Path:      "middleware/session.go",
				Line:      12,
				Relations: []string{"calls ValidateToken"},
			},
		},
	}
	got := cm.Format()

	if !strings.Contains(got, "## Relevant Context") {
		t.Error("missing header")
	}
	if !strings.Contains(got, "`ValidateCredentials` function (auth/login.go:45)") {
		t.Error("missing first symbol")
	}
	if !strings.Contains(got, "calls CheckPassword, called by LoginHandler") {
		t.Error("missing relations for first symbol")
	}
	if !strings.Contains(got, "`SessionMiddleware` method (middleware/session.go:12)") {
		t.Error("missing second symbol")
	}
}

func TestSemanticCandidates(t *testing.T) {
	store := setupTestStore(t)
	embedder := &fakeEmbedder{}

	// Insert nodes with embeddings
	nodes := []graph.Node{
		{ID: "func:auth/login.go:ValidateCredentials", Type: "function", Name: "ValidateCredentials", Path: "auth/login.go", Line: 45},
		{ID: "func:auth/crypto.go:CheckPassword", Type: "function", Name: "CheckPassword", Path: "auth/crypto.go", Line: 30},
		{ID: "func:util/logger.go:LogInfo", Type: "function", Name: "LogInfo", Path: "util/logger.go", Line: 10},
	}
	store.InsertBatch(nodes, nil)

	// Embed with known vectors — ValidateCredentials is closest to query
	store.InsertEmbedding(nodes[0].ID, embedder.vectorFor("validate credentials login"))
	store.InsertEmbedding(nodes[1].ID, embedder.vectorFor("check password hash"))
	store.InsertEmbedding(nodes[2].ID, embedder.vectorFor("log info message"))

	candidates, err := semanticCandidates(store, embedder, "fix login validation", 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected candidates")
	}
	// First candidate should be the most semantically similar
	if candidates[0].Node.Name != "ValidateCredentials" {
		t.Errorf("expected ValidateCredentials first, got %s", candidates[0].Node.Name)
	}
}

func TestCoChangeBoost(t *testing.T) {
	store := setupTestStore(t)

	nodes := []graph.Node{
		{ID: "file:a.go", Type: "file", Name: "a.go", Path: "a.go", Line: 0},
		{ID: "file:b.go", Type: "file", Name: "b.go", Path: "b.go", Line: 0},
	}
	store.InsertBatch(nodes, nil)
	store.InsertEdgeWithWeight("file:a.go", "file:b.go", "CO_CHANGED", 5)

	candidates := []candidate{
		{Node: graph.Node{ID: "func:a.go:Foo", Type: "function", Name: "Foo", Path: "a.go", Line: 10}, Score: 0.5},
		{Node: graph.Node{ID: "func:b.go:Bar", Type: "function", Name: "Bar", Path: "b.go", Line: 20}, Score: 0.5},
	}

	boosted := coChangeBoost(store, candidates)

	// Both should be boosted because their files co-change
	for _, c := range boosted {
		if c.Score <= 0.5 {
			t.Errorf("%s score should be boosted above 0.5, got %f", c.Node.Name, c.Score)
		}
	}
}

func TestRecencyBoost(t *testing.T) {
	store := setupTestStore(t)

	// Insert a file node and a recent commit that modifies it
	nodes := []graph.Node{
		{ID: "file:a.go", Type: "file", Name: "a.go", Path: "a.go", Line: 0},
	}
	store.InsertBatch(nodes, nil)
	store.InsertCommitBatch([]graph.Commit{
		{SHA: "abc123", Message: "fix a.go", AuthorEmail: "dev@test.com", Timestamp: 9999999999, Additions: 10, Deletions: 5},
	})
	store.InsertBatch(nil, []graph.Edge{
		{From: "commit:abc123", To: "file:a.go", Type: "MODIFIES"},
	})

	candidates := []candidate{
		{Node: graph.Node{ID: "func:a.go:Foo", Type: "function", Name: "Foo", Path: "a.go", Line: 10}, Score: 0.5},
	}

	boosted := recencyBoost(store, candidates, 10)
	if boosted[0].Score <= 0.5 {
		t.Errorf("expected recency boost, got %f", boosted[0].Score)
	}
}

func TestFailureBoost(t *testing.T) {
	store := setupTestStore(t)

	// Insert a failed test result linked to a session
	store.InsertExecution(model.Execution{SessionID: "session-1", StartedAt: 1000, Status: "failed"})
	store.InsertTestResult(model.TestResult{ID: "test:session-1:TestFoo", SessionID: "session-1", Name: "TestFoo", Passed: false, DurationMs: 100, Output: "assertion failed"})

	candidates := []candidate{
		{Node: graph.Node{ID: "func:a.go:Foo", Type: "function", Name: "Foo", Path: "a.go", Line: 10}, Score: 0.5},
	}

	boosted := failureBoost(store, candidates)
	if boosted[0].Score <= 0.5 {
		t.Errorf("expected failure boost, got %f", boosted[0].Score)
	}
}

func TestStructuralExpansion(t *testing.T) {
	store := setupTestStore(t)

	// Insert nodes
	nodes := []graph.Node{
		{ID: "func:a.go:Foo", Type: "function", Name: "Foo", Path: "a.go", Line: 10},
		{ID: "func:b.go:Bar", Type: "function", Name: "Bar", Path: "b.go", Line: 20},
		{ID: "func:c.go:Baz", Type: "function", Name: "Baz", Path: "c.go", Line: 30},
	}
	edges := []graph.Edge{
		{From: "func:a.go:Foo", To: "func:b.go:Bar", Type: "CALLS"},
		{From: "func:b.go:Bar", To: "func:c.go:Baz", Type: "CALLS"},
	}
	store.InsertBatch(nodes, edges)

	// Start with Foo as a semantic candidate
	seeds := []candidate{
		{Node: nodes[0], Score: 0.9},
	}

	expanded := structuralExpansion(store, seeds)

	// Should include Bar (1-hop from Foo) with reduced score
	found := false
	for _, c := range expanded {
		if c.Node.Name == "Bar" {
			found = true
			expected := 0.9 * structuralDecay
			if c.Score < expected-0.01 || c.Score > expected+0.01 {
				t.Errorf("Bar score = %f, want ~%f", c.Score, expected)
			}
		}
	}
	if !found {
		t.Error("expected Bar in expanded candidates")
	}

	// Should NOT include Baz (2 hops away, we only do 1-hop)
	for _, c := range expanded {
		if c.Node.Name == "Baz" {
			t.Error("did not expect Baz (2 hops away)")
		}
	}
}
