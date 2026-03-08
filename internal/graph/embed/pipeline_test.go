package embed

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lofari/golem/internal/graph"
)

func TestEmbedPipeline(t *testing.T) {
	dir := t.TempDir()
	store, err := graph.OpenStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Insert test nodes
	nodes := []graph.Node{
		{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go", Line: 1},
		{ID: "fn:main.go:Foo", Type: "function", Name: "Foo", Path: "main.go", Line: 5},
		{ID: "fn:main.go:Bar", Type: "function", Name: "Bar", Path: "main.go", Line: 15},
		{ID: "type:main.go:Cfg", Type: "type", Name: "Cfg", Path: "main.go", Line: 25},
	}
	if err := store.InsertBatch(nodes, nil); err != nil {
		t.Fatal(err)
	}

	embedder := &mockEmbedder{dims: 384}
	p := NewPipeline(store, embedder)

	count, err := p.EmbedAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Errorf("expected 4 embedded, got %d", count)
	}

	ec, err := store.EmbeddingCount()
	if err != nil {
		t.Fatal(err)
	}
	if ec != 4 {
		t.Errorf("expected 4 stored embeddings, got %d", ec)
	}
}

func TestEmbedByPath(t *testing.T) {
	dir := t.TempDir()
	store, err := graph.OpenStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	nodes := []graph.Node{
		{ID: "fn:a.go:Foo", Type: "function", Name: "Foo", Path: "a.go", Line: 1},
		{ID: "fn:b.go:Bar", Type: "function", Name: "Bar", Path: "b.go", Line: 1},
	}
	if err := store.InsertBatch(nodes, nil); err != nil {
		t.Fatal(err)
	}

	embedder := &mockEmbedder{dims: 384}
	p := NewPipeline(store, embedder)

	// Embed only a.go
	count, err := p.EmbedByPath(context.Background(), []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 embedded, got %d", count)
	}

	ec, err := store.EmbeddingCount()
	if err != nil {
		t.Fatal(err)
	}
	if ec != 1 {
		t.Errorf("expected 1 stored embedding, got %d", ec)
	}
}
