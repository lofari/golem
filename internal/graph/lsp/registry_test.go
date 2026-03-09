package lsp

import (
	"testing"
)

func TestDetectLanguages(t *testing.T) {
	files := []string{
		"main.go",
		"lib.py",
		"app.ts",
		"index.js",
		"server.rs",
		"App.kt",
		"widget.dart",
		"Main.java",
		"README.md",
	}

	langs := DetectLanguages(files)

	expected := map[string]bool{
		"go":         true,
		"python":     true,
		"typescript": true,
		"javascript": true,
		"rust":       true,
		"kotlin":     true,
		"dart":       true,
		"java":       true,
	}

	if len(langs) != len(expected) {
		t.Fatalf("expected %d languages, got %d: %v", len(expected), len(langs), langNames(langs))
	}

	for _, cfg := range langs {
		if !expected[cfg.Language] {
			t.Errorf("unexpected language: %s", cfg.Language)
		}
	}
}

func TestDetectLanguages_duplicates(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go"}
	langs := DetectLanguages(files)
	if len(langs) != 1 {
		t.Fatalf("expected 1 language, got %d", len(langs))
	}
	if langs[0].Language != "go" {
		t.Errorf("expected go, got %s", langs[0].Language)
	}
}

func TestServerConfigForExt(t *testing.T) {
	tests := []struct {
		ext  string
		lang string
	}{
		{".go", "go"},
		{".py", "python"},
		{".ts", "typescript"},
		{".tsx", "typescript"},
		{".js", "javascript"},
		{".jsx", "javascript"},
		{".rs", "rust"},
		{".kt", "kotlin"},
		{".kts", "kotlin"},
		{".dart", "dart"},
		{".java", "java"},
		{".txt", ""},
	}

	for _, tt := range tests {
		cfg := ConfigForExt(tt.ext)
		if tt.lang == "" {
			if cfg != nil {
				t.Errorf("ext %s: expected nil, got %s", tt.ext, cfg.Language)
			}
			continue
		}
		if cfg == nil {
			t.Errorf("ext %s: expected %s, got nil", tt.ext, tt.lang)
			continue
		}
		if cfg.Language != tt.lang {
			t.Errorf("ext %s: expected %s, got %s", tt.ext, tt.lang, cfg.Language)
		}
	}
}

func langNames(cfgs []ServerConfig) []string {
	var names []string
	for _, c := range cfgs {
		names = append(names, c.Language)
	}
	return names
}
