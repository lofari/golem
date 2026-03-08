package embed

import (
	"context"
	"testing"
)

type mockEmbedder struct {
	dims int
}

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = make([]float32, m.dims)
		for j := range out[i] {
			out[i][j] = float32(len(texts[i])) / 100.0
		}
	}
	return out, nil
}

func (m *mockEmbedder) Dimensions() int { return m.dims }
func (m *mockEmbedder) Close() error    { return nil }

func TestMockEmbedder(t *testing.T) {
	e := &mockEmbedder{dims: 384}
	vecs, err := e.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vecs))
	}
	if len(vecs[0]) != 384 {
		t.Fatalf("expected 384 dims, got %d", len(vecs[0]))
	}
	if e.Dimensions() != 384 {
		t.Fatal("expected Dimensions() == 384")
	}
}
