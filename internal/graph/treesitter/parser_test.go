package treesitter

import (
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"server.py", "python"},
		{"app.ts", "typescript"},
		{"index.js", "javascript"},
		{"unknown.xyz", ""},
		{"Makefile", ""},
	}
	for _, tt := range tests {
		got := DetectLanguage(tt.path)
		if got != tt.want {
			t.Errorf("DetectLanguage(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestParseGo(t *testing.T) {
	src := []byte(`package main

import "fmt"

func Hello() {
	fmt.Println("hello")
}

func main() {
	Hello()
}
`)
	tree, lang, err := ParseBytes(src, "go")
	if err != nil {
		t.Fatal(err)
	}
	if tree == nil {
		t.Fatal("expected non-nil tree")
	}
	if lang != "go" {
		t.Fatalf("expected lang 'go', got %q", lang)
	}
	root := tree.RootNode()
	if root.Type() != "source_file" {
		t.Fatalf("expected root type 'source_file', got %q", root.Type())
	}
}

func TestParseUnsupportedLanguage(t *testing.T) {
	_, _, err := ParseBytes([]byte("hello"), "unknown")
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
}
