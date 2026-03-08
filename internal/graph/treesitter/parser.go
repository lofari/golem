package treesitter

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	tsTypescript "github.com/smacker/go-tree-sitter/typescript/typescript"
)

// Supported language extensions.
var extToLang = map[string]string{
	".go":  "go",
	".py":  "python",
	".js":  "javascript",
	".jsx": "javascript",
	".ts":  "typescript",
	".tsx": "typescript",
	".mjs": "javascript",
	".cjs": "javascript",
}

// langToSitter maps language names to tree-sitter language objects.
var langToSitter = map[string]*sitter.Language{
	"go":         golang.GetLanguage(),
	"python":     python.GetLanguage(),
	"javascript": javascript.GetLanguage(),
	"typescript": tsTypescript.GetLanguage(),
}

// DetectLanguage returns the language name for a file path, or "" if unsupported.
func DetectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	return extToLang[ext]
}

// Supported returns true if the given language is supported.
func Supported(lang string) bool {
	_, ok := langToSitter[lang]
	return ok
}

// ParseBytes parses source code bytes for the given language.
// Returns the parsed tree and the language name.
func ParseBytes(src []byte, lang string) (*sitter.Tree, string, error) {
	tsLang, ok := langToSitter[lang]
	if !ok {
		return nil, "", fmt.Errorf("unsupported language: %q", lang)
	}

	parser := sitter.NewParser()
	parser.SetLanguage(tsLang)
	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, "", fmt.Errorf("parsing %s: %w", lang, err)
	}
	return tree, lang, nil
}

// ParseFile reads and parses a file, auto-detecting the language.
// Returns nil tree if the language is unsupported (not an error).
func ParseFile(path string, src []byte) (*sitter.Tree, string, error) {
	lang := DetectLanguage(path)
	if lang == "" {
		return nil, "", nil
	}
	tree, _, err := ParseBytes(src, lang)
	return tree, lang, err
}
