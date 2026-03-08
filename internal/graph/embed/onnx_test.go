package embed

import (
	"context"
	"os"
	"testing"
)

func TestONNXEmbedder(t *testing.T) {
	modelDir := os.Getenv("GOLEM_TEST_MODEL_DIR")
	if modelDir == "" {
		t.Skip("set GOLEM_TEST_MODEL_DIR to run ONNX embedding tests")
	}

	e, err := NewONNXEmbedder(modelDir)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	if e.Dimensions() != 384 {
		t.Fatalf("expected 384 dims, got %d", e.Dimensions())
	}

	vecs, err := e.Embed(context.Background(), []string{
		"Function StartServer handles HTTP server initialization",
		"password validation and authentication logic",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vecs))
	}
	if len(vecs[0]) != 384 {
		t.Fatalf("expected 384 dims, got %d", len(vecs[0]))
	}

	// Vectors should be non-zero
	allZero := true
	for _, v := range vecs[0] {
		if v != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Fatal("embedding vector is all zeros")
	}
}
