package context

import (
	"strings"
	"testing"
)

func TestContextMap_Format_Empty(t *testing.T) {
	cm := &ContextMap{Task: "fix login"}
	got := cm.Format()
	if got != "" {
		t.Errorf("expected empty string for no symbols, got %q", got)
	}
}

func TestContextMap_Format(t *testing.T) {
	cm := &ContextMap{
		Task: "fix login",
		Symbols: []SymbolEntry{
			{
				Name:      "ValidateCredentials",
				Kind:      "function",
				Path:      "auth/login.go",
				Line:      45,
				Relations: []string{"calls CheckPassword", "called by LoginHandler"},
			},
			{
				Name:      "SessionMiddleware",
				Kind:      "method",
				Path:      "middleware/session.go",
				Line:      12,
				Relations: []string{"calls ValidateToken"},
			},
		},
	}
	got := cm.Format()

	if !strings.Contains(got, "## Relevant Context") {
		t.Error("missing header")
	}
	if !strings.Contains(got, "`ValidateCredentials` function (auth/login.go:45)") {
		t.Error("missing first symbol")
	}
	if !strings.Contains(got, "calls CheckPassword, called by LoginHandler") {
		t.Error("missing relations for first symbol")
	}
	if !strings.Contains(got, "`SessionMiddleware` method (middleware/session.go:12)") {
		t.Error("missing second symbol")
	}
}
