package markdown

import "testing"

func TestParseMarkdown(t *testing.T) {
	content := []byte("# My Project\n\nThis is the intro about `StartServer` and `Config`.\n\n## Installation\n\nRun `go build`.\n\n## Usage\n\nCall `StartServer` with a `Config` struct.\n\n### Advanced Usage\n\nSee `internal/graph/store.go` for details.\n")

	sections, err := ParseMarkdown("README.md", content)
	if err != nil {
		t.Fatal(err)
	}

	if len(sections) != 4 {
		t.Fatalf("expected 4 sections, got %d", len(sections))
	}

	// Check first section
	if sections[0].Heading != "My Project" {
		t.Errorf("section 0 heading = %q, want %q", sections[0].Heading, "My Project")
	}
	if sections[0].Level != 1 {
		t.Errorf("section 0 level = %d, want 1", sections[0].Level)
	}
	if sections[0].Line != 1 {
		t.Errorf("section 0 line = %d, want 1", sections[0].Line)
	}
	// Check refs — StartServer and Config (go build filtered out because of space)
	if len(sections[0].Refs) != 2 {
		t.Fatalf("section 0 refs = %v, want [StartServer Config]", sections[0].Refs)
	}
	if sections[0].Refs[0] != "StartServer" || sections[0].Refs[1] != "Config" {
		t.Errorf("section 0 refs = %v", sections[0].Refs)
	}

	// Check section 1 (Installation)
	if sections[1].Heading != "Installation" {
		t.Errorf("section 1 heading = %q, want %q", sections[1].Heading, "Installation")
	}
	if sections[1].Level != 2 {
		t.Errorf("section 1 level = %d, want 2", sections[1].Level)
	}

	// Check section 2 (Usage)
	if sections[2].Heading != "Usage" {
		t.Errorf("section 2 heading = %q, want %q", sections[2].Heading, "Usage")
	}
	if sections[2].Level != 2 {
		t.Errorf("section 2 level = %d, want 2", sections[2].Level)
	}
	// Usage refs should include StartServer and Config
	if len(sections[2].Refs) != 2 {
		t.Fatalf("section 2 refs = %v, want [StartServer Config]", sections[2].Refs)
	}

	// Check section 3 (Advanced Usage)
	if sections[3].Heading != "Advanced Usage" {
		t.Errorf("section 3 heading = %q, want %q", sections[3].Heading, "Advanced Usage")
	}
	if sections[3].Level != 3 {
		t.Errorf("section 3 level = %d, want 3", sections[3].Level)
	}
	// "internal/graph/store.go" should be filtered out (contains /)
	if len(sections[3].Refs) != 0 {
		t.Errorf("section 3 refs = %v, want []", sections[3].Refs)
	}
}

func TestParseMarkdownEmpty(t *testing.T) {
	sections, err := ParseMarkdown("empty.md", []byte("No headings here.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 0 {
		t.Errorf("expected 0 sections for headingless doc, got %d", len(sections))
	}
}
