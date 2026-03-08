package embed

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/knights-analytics/hugot"
)

const (
	DefaultModel   = "BAAI/bge-small-en-v1.5"
	DefaultModelID = "bge-small-en-v1.5"
)

// DefaultModelDir returns ~/.config/golem/models/
func DefaultModelDir() string {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		cfgDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(cfgDir, "golem", "models")
}

// EnsureModel downloads the model if not already cached. Returns the path to the model directory.
func EnsureModel(modelID, modelDir string) (string, error) {
	modelPath := filepath.Join(modelDir, modelID)
	if _, err := os.Stat(filepath.Join(modelPath, "tokenizer.json")); err == nil {
		return modelPath, nil // already downloaded
	}

	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return "", fmt.Errorf("create model dir: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Downloading embedding model %s...\n", modelID)
	downloadedPath, err := hugot.DownloadModel(DefaultModel, modelDir, hugot.NewDownloadOptions())
	if err != nil {
		return "", fmt.Errorf("download model: %w", err)
	}
	return downloadedPath, nil
}
