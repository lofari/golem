package embed

import "context"

// Embedder generates vector embeddings from text.
type Embedder interface {
	// Embed converts texts to float32 vectors. Returns one vector per input text.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// Dimensions returns the output vector dimensionality (e.g. 384).
	Dimensions() int
	// Close releases model resources.
	Close() error
}
