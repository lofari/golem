package context

import (
	gocontext "context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lofari/golem/internal/graph"
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
