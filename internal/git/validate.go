package git

import (
	"fmt"
	"strings"
)

// ValidateGitRef checks that a git ref is safe to pass as a command argument.
func ValidateGitRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("empty git ref")
	}
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("git ref must not start with dash: %q", ref)
	}
	for _, ch := range ref {
		if ch < 0x20 || ch == 0x7f || ch == '$' || ch == '`' || ch == ';' || ch == '|' || ch == '&' {
			return fmt.Errorf("git ref contains unsafe character: %q", ref)
		}
	}
	return nil
}

// ValidateFilePath checks that a file path is relative and safe.
func ValidateFilePath(path string) error {
	if path == "" {
		return fmt.Errorf("empty file path")
	}
	if strings.HasPrefix(path, "-") {
		return fmt.Errorf("file path must not start with dash: %q", path)
	}
	if strings.HasPrefix(path, "/") {
		return fmt.Errorf("file path must be relative: %q", path)
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("file path must not contain '..': %q", path)
	}
	return nil
}
