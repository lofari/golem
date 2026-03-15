package git

import "testing"

func TestValidateGitRef(t *testing.T) {
	valid := []string{"HEAD~1", "main", "origin/main", "v1.0.0", "abc123", "feat/my-branch"}
	for _, ref := range valid {
		if err := ValidateGitRef(ref); err != nil {
			t.Errorf("expected %q to be valid, got: %v", ref, err)
		}
	}
	invalid := []string{"--flag", "-x", "; rm -rf /", "$(whoami)", "ref\nnewline", ""}
	for _, ref := range invalid {
		if err := ValidateGitRef(ref); err == nil {
			t.Errorf("expected %q to be invalid", ref)
		}
	}
}

func TestValidateFilePath(t *testing.T) {
	valid := []string{"main.go", "internal/runner/engine.go", "docs/README.md"}
	for _, p := range valid {
		if err := ValidateFilePath(p); err != nil {
			t.Errorf("expected %q to be valid, got: %v", p, err)
		}
	}
	invalid := []string{"../../../etc/passwd", "--flag", "/absolute/path", ""}
	for _, p := range invalid {
		if err := ValidateFilePath(p); err == nil {
			t.Errorf("expected %q to be invalid", p)
		}
	}
}
