package graph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempDB(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	s, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenStore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	s, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Verify file was created
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database file not created: %v", err)
	}
}

func TestInsertAndQueryNodes(t *testing.T) {
	s := tempDB(t)

	nodes := []Node{
		{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go"},
		{ID: "fn:main.go:main", Type: "function", Name: "main", Path: "main.go", Line: 10},
		{ID: "fn:main.go:helper", Type: "function", Name: "helper", Path: "main.go", Line: 25},
	}
	edges := []Edge{
		{From: "file:main.go", To: "fn:main.go:main", Type: "DEFINES"},
		{From: "file:main.go", To: "fn:main.go:helper", Type: "DEFINES"},
		{From: "fn:main.go:main", To: "fn:main.go:helper", Type: "CALLS"},
	}

	if err := s.InsertBatch(nodes, edges); err != nil {
		t.Fatal(err)
	}

	// Query nodes by type
	fns, err := s.NodesByType("function")
	if err != nil {
		t.Fatal(err)
	}
	if len(fns) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(fns))
	}

	// Query edges from a node
	outEdges, err := s.EdgesFrom("fn:main.go:main")
	if err != nil {
		t.Fatal(err)
	}
	if len(outEdges) != 1 || outEdges[0].Type != "CALLS" {
		t.Fatalf("expected 1 CALLS edge, got %v", outEdges)
	}

	// Query edges to a node (reverse)
	inEdges, err := s.EdgesTo("fn:main.go:helper")
	if err != nil {
		t.Fatal(err)
	}
	if len(inEdges) != 2 { // DEFINES + CALLS
		t.Fatalf("expected 2 inbound edges, got %d", len(inEdges))
	}
}

func TestDeleteByPath(t *testing.T) {
	s := tempDB(t)

	nodes := []Node{
		{ID: "file:a.go", Type: "file", Name: "a.go", Path: "a.go"},
		{ID: "fn:a.go:Foo", Type: "function", Name: "Foo", Path: "a.go", Line: 1},
		{ID: "file:b.go", Type: "file", Name: "b.go", Path: "b.go"},
		{ID: "fn:b.go:Bar", Type: "function", Name: "Bar", Path: "b.go", Line: 1},
	}
	edges := []Edge{
		{From: "file:a.go", To: "fn:a.go:Foo", Type: "DEFINES"},
		{From: "fn:a.go:Foo", To: "fn:b.go:Bar", Type: "CALLS"},
	}
	s.InsertBatch(nodes, edges)

	// Delete nodes for a.go
	if err := s.DeleteByPath("a.go"); err != nil {
		t.Fatal(err)
	}

	// a.go nodes should be gone
	fns, _ := s.NodesByPath("a.go")
	if len(fns) != 0 {
		t.Fatalf("expected 0 nodes for a.go, got %d", len(fns))
	}

	// b.go nodes should remain
	fns, _ = s.NodesByPath("b.go")
	if len(fns) != 2 {
		t.Fatalf("expected 2 nodes for b.go, got %d", len(fns))
	}

	// Dangling edges from deleted nodes should be gone
	edges2, _ := s.EdgesFrom("fn:a.go:Foo")
	if len(edges2) != 0 {
		t.Fatalf("expected 0 edges from deleted node, got %d", len(edges2))
	}
}

func TestSetAndGetMeta(t *testing.T) {
	s := tempDB(t)

	if err := s.SetMeta("last_commit", "abc123"); err != nil {
		t.Fatal(err)
	}
	val, err := s.GetMeta("last_commit")
	if err != nil {
		t.Fatal(err)
	}
	if val != "abc123" {
		t.Fatalf("expected abc123, got %q", val)
	}

	// Update
	s.SetMeta("last_commit", "def456")
	val, _ = s.GetMeta("last_commit")
	if val != "def456" {
		t.Fatalf("expected def456, got %q", val)
	}

	// Missing key
	val, _ = s.GetMeta("nonexistent")
	if val != "" {
		t.Fatalf("expected empty string, got %q", val)
	}
}

func TestStats(t *testing.T) {
	s := tempDB(t)

	nodes := []Node{
		{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go"},
		{ID: "fn:main.go:main", Type: "function", Name: "main", Path: "main.go", Line: 10},
	}
	edges := []Edge{
		{From: "file:main.go", To: "fn:main.go:main", Type: "DEFINES"},
	}
	s.InsertBatch(nodes, edges)

	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalNodes != 2 {
		t.Fatalf("expected 2 nodes, got %d", stats.TotalNodes)
	}
	if stats.TotalEdges != 1 {
		t.Fatalf("expected 1 edge, got %d", stats.TotalEdges)
	}
}

func TestInsertAndSearchEmbeddings(t *testing.T) {
	s := tempDB(t)

	// Insert some nodes first
	nodes := []Node{
		{ID: "fn:main.go:foo", Type: "function", Name: "foo", Path: "main.go", Line: 1},
		{ID: "fn:main.go:bar", Type: "function", Name: "bar", Path: "main.go", Line: 10},
		{ID: "fn:util.go:baz", Type: "function", Name: "baz", Path: "util.go", Line: 1},
	}
	if err := s.InsertBatch(nodes, nil); err != nil {
		t.Fatal(err)
	}

	// Insert embeddings — foo and baz are similar, bar is different
	entries := []EmbeddingEntry{
		{NodeID: "fn:main.go:foo", Vector: makeVec(384, 0.1)},
		{NodeID: "fn:main.go:bar", Vector: makeVec(384, 0.9)},
		{NodeID: "fn:util.go:baz", Vector: makeVec(384, 0.11)},
	}
	if err := s.InsertEmbeddingsBatch(entries); err != nil {
		t.Fatal(err)
	}

	// Search for vectors similar to foo's
	results, err := s.SearchSimilar(makeVec(384, 0.1), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// foo should be closest (distance 0), then baz
	if results[0].NodeID != "fn:main.go:foo" {
		t.Errorf("expected foo first, got %s", results[0].NodeID)
	}

	// Test count
	count, err := s.EmbeddingCount()
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("expected 3 embeddings, got %d", count)
	}

	// Test delete by path
	if err := s.DeleteEmbeddingsByPath("main.go"); err != nil {
		t.Fatal(err)
	}
	count, err = s.EmbeddingCount()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 embedding after delete, got %d", count)
	}
}

func TestInsertCommitBatch(t *testing.T) {
	s := tempDB(t)

	commits := []Commit{
		{SHA: "aaa111", Message: "first commit", AuthorEmail: "alice@example.com", Timestamp: 1000, Additions: 10, Deletions: 2},
		{SHA: "bbb222", Message: "second commit", AuthorEmail: "bob@example.com", Timestamp: 2000, Additions: 5, Deletions: 0},
	}

	if err := s.InsertCommitBatch(commits); err != nil {
		t.Fatal(err)
	}

	count, err := s.CommitCount()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 commits, got %d", count)
	}
}

func TestInsertAuthorBatch(t *testing.T) {
	s := tempDB(t)

	authors := []Author{
		{Email: "alice@example.com", Name: "Alice"},
		{Email: "bob@example.com", Name: "Bob"},
	}

	if err := s.InsertAuthorBatch(authors); err != nil {
		t.Fatal(err)
	}

	count, err := s.AuthorCount()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 authors, got %d", count)
	}

	// Upsert: update Bob's name
	updated := []Author{
		{Email: "bob@example.com", Name: "Robert"},
	}
	if err := s.InsertAuthorBatch(updated); err != nil {
		t.Fatal(err)
	}
	count, _ = s.AuthorCount()
	if count != 2 {
		t.Fatalf("expected 2 authors after upsert, got %d", count)
	}
}

func TestInsertEdgeWithWeight(t *testing.T) {
	s := tempDB(t)

	if err := s.InsertEdgeWithWeight("file:a.go", "file:b.go", "CO_CHANGED", 5); err != nil {
		t.Fatal(err)
	}

	count, err := s.CoChangedCount()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 CO_CHANGED edge, got %d", count)
	}

	// Overwrite with new weight
	if err := s.InsertEdgeWithWeight("file:a.go", "file:b.go", "CO_CHANGED", 10); err != nil {
		t.Fatal(err)
	}
	count, _ = s.CoChangedCount()
	if count != 1 {
		t.Fatalf("expected 1 CO_CHANGED edge after upsert, got %d", count)
	}
}

func TestQueryCommitBySHA(t *testing.T) {
	s := tempDB(t)

	commits := []Commit{
		{SHA: "abc123", Message: "fix bug", AuthorEmail: "dev@example.com", Timestamp: 3000, Additions: 1, Deletions: 1},
	}
	s.InsertCommitBatch(commits)

	c, err := s.QueryCommitBySHA("abc123")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("expected commit, got nil")
	}
	if c.SHA != "abc123" || c.Message != "fix bug" || c.AuthorEmail != "dev@example.com" {
		t.Fatalf("unexpected commit: %+v", c)
	}
	if c.Timestamp != 3000 || c.Additions != 1 || c.Deletions != 1 {
		t.Fatalf("unexpected commit fields: %+v", c)
	}

	// Non-existent SHA
	c, err = s.QueryCommitBySHA("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if c != nil {
		t.Fatalf("expected nil for missing SHA, got %+v", c)
	}
}

func TestQueryCommitsByFile(t *testing.T) {
	s := tempDB(t)

	commits := []Commit{
		{SHA: "c1", Message: "first", AuthorEmail: "a@b.com", Timestamp: 1000, Additions: 5, Deletions: 0},
		{SHA: "c2", Message: "second", AuthorEmail: "a@b.com", Timestamp: 2000, Additions: 3, Deletions: 1},
		{SHA: "c3", Message: "third", AuthorEmail: "a@b.com", Timestamp: 3000, Additions: 1, Deletions: 1},
	}
	s.InsertCommitBatch(commits)

	// Create MODIFIES edges: c1 and c2 modify main.go, c3 modifies other.go
	edges := []Edge{
		{From: "commit:c1", To: "file:main.go", Type: "MODIFIES"},
		{From: "commit:c2", To: "file:main.go", Type: "MODIFIES"},
		{From: "commit:c3", To: "file:other.go", Type: "MODIFIES"},
	}
	s.InsertBatch(nil, edges)

	// Query commits for main.go
	result, err := s.QueryCommitsByFile("main.go", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 commits for main.go, got %d", len(result))
	}
	// Should be ordered by timestamp DESC
	if result[0].SHA != "c2" {
		t.Fatalf("expected c2 first (newest), got %s", result[0].SHA)
	}
	if result[1].SHA != "c1" {
		t.Fatalf("expected c1 second, got %s", result[1].SHA)
	}

	// Test limit
	result, err = s.QueryCommitsByFile("main.go", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 commit with limit=1, got %d", len(result))
	}
}

func TestDeleteHistory(t *testing.T) {
	s := tempDB(t)

	// Insert commits, authors, and history edges
	s.InsertCommitBatch([]Commit{
		{SHA: "c1", Message: "msg", AuthorEmail: "a@b.com", Timestamp: 1000},
	})
	s.InsertAuthorBatch([]Author{
		{Email: "a@b.com", Name: "Alice"},
	})
	s.InsertEdgeWithWeight("file:a.go", "file:b.go", "CO_CHANGED", 5)

	edges := []Edge{
		{From: "commit:c1", To: "file:a.go", Type: "MODIFIES"},
		{From: "commit:c1", To: "author:a@b.com", Type: "AUTHORED_BY"},
		{From: "file:a.go", To: "fn:a.go:Foo", Type: "DEFINES"}, // Should NOT be deleted
	}
	s.InsertBatch(nil, edges)

	if err := s.DeleteHistory(); err != nil {
		t.Fatal(err)
	}

	commitCount, _ := s.CommitCount()
	if commitCount != 0 {
		t.Fatalf("expected 0 commits after DeleteHistory, got %d", commitCount)
	}
	authorCount, _ := s.AuthorCount()
	if authorCount != 0 {
		t.Fatalf("expected 0 authors after DeleteHistory, got %d", authorCount)
	}
	coCount, _ := s.CoChangedCount()
	if coCount != 0 {
		t.Fatalf("expected 0 CO_CHANGED edges after DeleteHistory, got %d", coCount)
	}

	// DEFINES edge should remain
	defEdges, _ := s.EdgesFrom("file:a.go")
	if len(defEdges) != 1 || defEdges[0].Type != "DEFINES" {
		t.Fatalf("expected DEFINES edge to remain, got %v", defEdges)
	}
}

func TestQueryCoChanged(t *testing.T) {
	s := tempDB(t)

	// a.go co-changed with b.go (weight 5) and c.go (weight 2)
	s.InsertEdgeWithWeight("file:a.go", "file:b.go", "CO_CHANGED", 5)
	s.InsertEdgeWithWeight("file:a.go", "file:c.go", "CO_CHANGED", 2)
	// d.go co-changed with a.go (weight 3) — reversed direction
	s.InsertEdgeWithWeight("file:d.go", "file:a.go", "CO_CHANGED", 3)

	// Query all co-changed files for a.go with minCount=1
	results, err := s.QueryCoChanged("a.go", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 co-changed results, got %d", len(results))
	}

	// Query with minCount=3 — should exclude c.go (weight 2)
	results, err = s.QueryCoChanged("a.go", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 co-changed results with minCount=3, got %d", len(results))
	}

	// Verify file paths have prefix stripped
	for _, r := range results {
		if r.File == "" {
			t.Fatal("expected non-empty file path")
		}
		if strings.HasPrefix(r.File, "file:") {
			t.Fatalf("file prefix not stripped: %s", r.File)
		}
	}

	// Results should be ordered by weight DESC
	if results[0].Count < results[1].Count {
		t.Fatalf("expected results ordered by count DESC, got %d then %d", results[0].Count, results[1].Count)
	}
}

func TestCoChangedCount(t *testing.T) {
	s := tempDB(t)

	s.InsertEdgeWithWeight("file:a.go", "file:b.go", "CO_CHANGED", 3)
	s.InsertEdgeWithWeight("file:a.go", "file:c.go", "CO_CHANGED", 1)
	// Insert a non-CO_CHANGED edge
	s.InsertBatch(nil, []Edge{{From: "file:a.go", To: "fn:a.go:Foo", Type: "DEFINES"}})

	count, err := s.CoChangedCount()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 CO_CHANGED edges, got %d", count)
	}
}

func TestCommitAndAuthorCount(t *testing.T) {
	s := tempDB(t)

	// Empty initially
	cc, _ := s.CommitCount()
	if cc != 0 {
		t.Fatalf("expected 0 commits, got %d", cc)
	}
	ac, _ := s.AuthorCount()
	if ac != 0 {
		t.Fatalf("expected 0 authors, got %d", ac)
	}

	s.InsertCommitBatch([]Commit{
		{SHA: "a", Message: "m", AuthorEmail: "e@e.com", Timestamp: 1},
		{SHA: "b", Message: "m", AuthorEmail: "e@e.com", Timestamp: 2},
		{SHA: "c", Message: "m", AuthorEmail: "f@f.com", Timestamp: 3},
	})
	s.InsertAuthorBatch([]Author{
		{Email: "e@e.com", Name: "E"},
		{Email: "f@f.com", Name: "F"},
	})

	cc, _ = s.CommitCount()
	if cc != 3 {
		t.Fatalf("expected 3 commits, got %d", cc)
	}
	ac, _ = s.AuthorCount()
	if ac != 2 {
		t.Fatalf("expected 2 authors, got %d", ac)
	}
}

func makeVec(dims int, val float32) []float32 {
	v := make([]float32, dims)
	for i := range v {
		v[i] = val
	}
	return v
}
