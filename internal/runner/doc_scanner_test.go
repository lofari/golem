package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindDocSection(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0755)

	doc := "# Implementation Plan\n\n" +
		"## Task 1: Setup Database\n" +
		"Steps for setting up the database...\n\n" +
		"## Task 2: Add Authentication\n" +
		"Steps for auth...\n\n" +
		"## 3. Build API Endpoints\n" +
		"Steps for API...\n\n" +
		"## Task 4: Write Tests\n" +
		"Steps for tests...\n"

	os.WriteFile(filepath.Join(docsDir, "impl.md"), []byte(doc), 0644)

	tests := []struct {
		taskName string
		wantHint string
		wantOK   bool
	}{
		{"Setup Database", "docs/impl.md", true},
		{"Add Authentication", "docs/impl.md", true},
		{"Build API Endpoints", "docs/impl.md", true},
		{"Write Tests", "docs/impl.md", true},
		{"Nonexistent Task", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.taskName, func(t *testing.T) {
			hint := findDocSection(dir, "docs/", tt.taskName)
			if tt.wantOK {
				if hint == "" {
					t.Errorf("expected hint for %q, got empty", tt.taskName)
				}
				if !strings.Contains(hint, tt.wantHint) {
					t.Errorf("hint %q should reference %q", hint, tt.wantHint)
				}
			} else {
				if hint != "" {
					t.Errorf("expected no hint for %q, got %q", tt.taskName, hint)
				}
			}
		})
	}
}

func TestFindDocSection_PrefersRecentFile(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0755)

	os.WriteFile(filepath.Join(docsDir, "old.md"), []byte("## Task: Setup\nold content\n"), 0644)
	os.WriteFile(filepath.Join(docsDir, "new.md"), []byte("## Task: Setup\nnew content\n"), 0644)

	hint := findDocSection(dir, "docs/", "Setup")
	if hint == "" {
		t.Fatal("expected a hint")
	}
	if !strings.Contains(hint, "new.md") {
		t.Errorf("hint = %q, expected reference to new.md", hint)
	}
}
