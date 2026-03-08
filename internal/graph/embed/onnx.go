package embed

import (
	"context"
	"fmt"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/pipelines"
)

// ONNXEmbedder generates embeddings using a local ONNX model via hugot.
type ONNXEmbedder struct {
	session  *hugot.Session
	pipeline *pipelines.FeatureExtractionPipeline
	dims     int
}

// NewONNXEmbedder creates an embedder from a downloaded model directory.
// modelDir should contain model.onnx and tokenizer.json.
func NewONNXEmbedder(modelDir string) (*ONNXEmbedder, error) {
	session, err := hugot.NewGoSession()
	if err != nil {
		return nil, fmt.Errorf("create hugot session: %w", err)
	}

	config := hugot.FeatureExtractionConfig{
		ModelPath:    modelDir,
		Name:         "golem-embedder",
		OnnxFilename: "model.onnx",
		Options: []hugot.FeatureExtractionOption{
			pipelines.WithNormalization(),
		},
	}

	pipeline, err := hugot.NewPipeline(session, config)
	if err != nil {
		session.Destroy()
		return nil, fmt.Errorf("create pipeline: %w", err)
	}

	return &ONNXEmbedder{
		session:  session,
		pipeline: pipeline,
		dims:     384, // BGE-small-en-v1.5 output dimension
	}, nil
}

func (e *ONNXEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	result, err := e.pipeline.RunPipeline(texts)
	if err != nil {
		return nil, fmt.Errorf("run pipeline: %w", err)
	}
	return result.Embeddings, nil
}

func (e *ONNXEmbedder) Dimensions() int { return e.dims }

func (e *ONNXEmbedder) Close() error {
	return e.session.Destroy()
}
